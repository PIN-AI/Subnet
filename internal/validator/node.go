package validator

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	rootpb "rootlayer/proto"
	"subnet/internal/config"
	"subnet/internal/consensus"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/metrics"
	"subnet/internal/registry"
	"subnet/internal/rootlayer"
	"subnet/internal/storage"
	"subnet/internal/types"
	pb "subnet/proto/subnet"
)

// Node represents a validator node in the subnet
type Node struct {
	mu sync.RWMutex

	// Core components
	id           string
	signer       crypto.Signer
	validatorSet *types.ValidatorSet
	logger       logging.Logger

	// Consensus components
	fsm           *consensus.StateMachine
	leaderTracker *consensus.LeaderTracker
	chain         *consensus.Chain

	// Storage
	store storage.Store

	// Network
	broadcaster consensus.Broadcaster
	natsConn    *nats.Conn
	natsSubs    []*nats.Subscription

	// RootLayer client
	rootlayerClient rootlayer.Client

	// Agent registry
	agentRegistry *registry.Registry
	validatorHB   context.CancelFunc

	// Metrics
	metrics       metrics.Provider
	metricsServer *http.Server

	// Execution reports
	pendingReports map[string]*pb.ExecutionReport
	reportScores   map[string]int32

	// Checkpoint management
	currentEpoch      uint64
	currentCheckpoint *pb.CheckpointHeader
	signatures        map[string]*pb.Signature // validator_id -> signature

	// Configuration
	config *Config

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// Config holds validator configuration
type Config struct {
	ValidatorID               string
	SubnetID                  string
	PrivateKey                string
	ValidatorSet              *types.ValidatorSet
	StoragePath               string
	NATSUrl                   string
	GRPCPort                  int
	MetricsPort               int
	RegistryEndpoint          string
	RegistryHeartbeatInterval time.Duration

	// Use unified config structures
	Timeouts *config.TimeoutConfig
	Network  *config.NetworkConfig
	Identity *config.IdentityConfig
	Limits   *config.LimitsConfig

	// Legacy fields for backward compatibility
	ProposeTimeout     time.Duration
	CollectTimeout     time.Duration
	FinalizeTimeout    time.Duration
	CheckpointInterval time.Duration

	// Execution report config
	MaxReportsPerEpoch int
	ReportScoreDecay   float32

	// RootLayer config
	RootLayerEndpoint     string
	EnableRootLayerSubmit bool

	// Validation policy
	ValidationPolicy *ValidationPolicyConfig
}

// ValidationPolicyConfig defines validation policy configuration
type ValidationPolicyConfig struct {
	PolicyID                string
	Version                 string
	MinExecutionTime        int64   // Minimum execution time in seconds
	MaxExecutionTime        int64   // Maximum execution time in seconds
	RequireProofOfExecution bool    // Whether to require proof of execution
	MinConfidenceScore      float32 // Minimum confidence score (0-1)
	MaxRetries              int     // Maximum retry attempts
}

// NewNode creates a new validator node
func NewNode(config *Config, logger logging.Logger, agentReg *registry.Registry) (*Node, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Initialize signer
	signer, err := crypto.NewECDSASignerFromHex(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Initialize storage
	store, err := storage.NewLevelDB(config.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create consensus components with timeout config
	fsm := consensus.NewStateMachineWithConfig(config.ValidatorSet, signer, config.Timeouts)
	chain := consensus.NewChain()
	leaderTracker := consensus.NewLeaderTracker(config.ValidatorSet, config.ProposeTimeout)

	// Create RootLayer client if enabled
	var rootClient rootlayer.Client
	if config.EnableRootLayerSubmit && config.RootLayerEndpoint != "" {
		rootConfig := &rootlayer.Config{
			Endpoint:  config.RootLayerEndpoint,
			SubnetID:  config.SubnetID,
			MatcherID: config.ValidatorID, // Use validator ID as aggregator ID
		}
		rootClient = rootlayer.NewGRPCClient(rootConfig, logger)
	}

	ctx, cancel := context.WithCancel(context.Background())

	node := &Node{
		id:              config.ValidatorID,
		signer:          signer,
		validatorSet:    config.ValidatorSet,
		logger:          logger,
		fsm:             fsm,
		leaderTracker:   leaderTracker,
		chain:           chain,
		store:           store,
		rootlayerClient: rootClient,
		agentRegistry:   agentReg,
		metrics:         metrics.Noop{},
		pendingReports:  make(map[string]*pb.ExecutionReport),
		reportScores:    make(map[string]int32),
		signatures:      make(map[string]*pb.Signature),
		config:          config,
		ctx:             ctx,
		cancel:          cancel,
	}

	// Load latest checkpoint from storage
	if err := node.loadCheckpoint(); err != nil {
		logger.Warn("Failed to load checkpoint", "error", err)
	}

	return node, nil
}

// Start starts the validator node
func (n *Node) Start(ctx context.Context) error {
	n.logger.Info("Starting validator node", "id", n.id)

	// Connect to RootLayer if configured
	if n.rootlayerClient != nil {
		if err := n.rootlayerClient.Connect(); err != nil {
			n.logger.Warn("Failed to connect to RootLayer", "error", err)
			// Continue without RootLayer for now
		} else {
			n.logger.Info("Connected to RootLayer")
		}
	}

	// Connect to NATS
	if err := n.connectNATS(); err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Start metrics server if configured
	if n.config.MetricsPort > 0 {
		n.startMetricsServer()
	}

	// Register validator in registry and start heartbeat
	n.startValidatorRegistry()

	// Start consensus loop
	go n.consensusLoop()

	// Start checkpoint creation timer
	go n.checkpointTimer()

	n.logger.Info("Validator node started successfully")
	return nil
}

// Stop gracefully stops the validator node
func (n *Node) Stop() error {
	n.logger.Info("Stopping validator node")

	n.cancel()

	if n.validatorHB != nil {
		n.validatorHB()
		n.validatorHB = nil
	}

	// Close broadcaster
	if n.broadcaster != nil {
		if err := n.broadcaster.Close(); err != nil {
			n.logger.Error("Failed to close broadcaster", "error", err)
		}
	}

	// Unsubscribe from NATS (if using old method)
	for _, sub := range n.natsSubs {
		sub.Unsubscribe()
	}

	// Close NATS connection (if using old method)
	if n.natsConn != nil {
		n.natsConn.Close()
	}

	// Close RootLayer connection
	if n.rootlayerClient != nil {
		if err := n.rootlayerClient.Close(); err != nil {
			n.logger.Error("Failed to close RootLayer connection", "error", err)
		}
	}

	if n.agentRegistry != nil {
		if err := n.agentRegistry.RemoveValidator(n.id); err != nil {
			n.logger.Warn("Failed to remove validator from registry", "error", err)
		}
	}

	if n.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := n.metricsServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
			n.logger.Warn("Failed to shutdown metrics server", "error", err)
		}
	}

	// Close storage
	if n.store != nil {
		if err := n.store.Close(); err != nil {
			n.logger.Error("Failed to close storage", "error", err)
		}
	}

	n.logger.Info("Validator node stopped")
	return nil
}

// connectNATS establishes NATS connection and subscriptions
func (n *Node) connectNATS() error {
	// Create NATS broadcaster
	subnetID := n.config.SubnetID
	if subnetID == "" {
		subnetID = "default"
	}

	broadcaster, err := consensus.NewNATSBroadcaster(
		n.config.NATSUrl,
		n.id,
		subnetID,
		n.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create NATS broadcaster: %w", err)
	}
	n.broadcaster = broadcaster

	// Subscribe to checkpoint proposals
	if err := broadcaster.SubscribeToProposals(func(header *pb.CheckpointHeader) {
		if err := n.HandleProposal(header); err != nil {
			n.logger.Error("Failed to handle proposal", "error", err)
		}
	}); err != nil {
		return fmt.Errorf("failed to subscribe to proposals: %w", err)
	}

	// Subscribe to signatures
	if err := broadcaster.SubscribeToSignatures(func(sig *pb.Signature) {
		if err := n.AddSignature(sig); err != nil {
			n.logger.Error("Failed to add signature", "error", err)
		}
	}); err != nil {
		return fmt.Errorf("failed to subscribe to signatures: %w", err)
	}

	// Subscribe to execution reports
	if err := broadcaster.SubscribeToExecutionReports(func(report *pb.ExecutionReport) {
		if _, err := n.ProcessExecutionReport(report); err != nil {
			n.logger.Error("Failed to process execution report", "error", err)
		}
	}); err != nil {
		return fmt.Errorf("failed to subscribe to execution reports: %w", err)
	}

	n.logger.Info("Connected to NATS with broadcaster", "url", n.config.NATSUrl, "subnet", subnetID)
	return nil
}

// consensusLoop runs the main consensus state machine
func (n *Node) consensusLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			n.checkConsensusState()
		}
	}
}

// checkConsensusState checks and updates consensus state
func (n *Node) checkConsensusState() {
	n.mu.Lock()
	state := n.fsm.GetState()
	n.recordMetrics(state)

	switch state {
	case consensus.StateIdle:
		// Check if we're the leader for current epoch
		_, leader := n.leaderTracker.Leader(n.currentEpoch)
		shouldPropose := leader != nil && leader.ID == n.id
		n.mu.Unlock()

		// Call proposeCheckpoint without holding the lock
		if shouldPropose {
			n.proposeCheckpoint()
		}

	case consensus.StateCollecting:
		// Check if we have threshold signatures
		hasThreshold := n.fsm.HasThreshold()
		isTimedOut := n.fsm.IsTimedOut(n.config.CollectTimeout)
		n.mu.Unlock()

		// Call finalize or reset without holding the lock
		if hasThreshold {
			n.finalizeCheckpoint()
		} else if isTimedOut {
			n.logger.Warn("Signature collection timeout", "epoch", n.currentEpoch)
			n.mu.Lock()
			n.fsm.Reset()
			n.mu.Unlock()
		}

	case consensus.StateFinalized:
		// Move to next epoch
		n.currentEpoch++
		n.fsm.Reset()
		n.clearEpochData()
		n.mu.Unlock()

	default:
		n.mu.Unlock()
	}
}

// proposeCheckpoint creates and broadcasts a new checkpoint proposal
func (n *Node) proposeCheckpoint() {
	header := n.buildCheckpointHeader()

	// Update FSM state
	if err := n.fsm.ProposeHeader(header); err != nil {
		n.logger.Error("Failed to propose header", "error", err)
		return
	}

	// Sign the header
	sig, err := n.signCheckpoint(header)
	if err != nil {
		n.logger.Error("Failed to sign checkpoint", "error", err)
		return
	}
	n.signatures[n.id] = sig

	// Broadcast proposal
	if err := n.broadcastProposal(header); err != nil {
		n.logger.Error("Failed to broadcast proposal", "error", err)
		return
	}

	n.logger.Info("Proposed checkpoint", "epoch", header.Epoch)
}

// buildCheckpointHeader creates a new checkpoint header
func (n *Node) buildCheckpointHeader() *pb.CheckpointHeader {
	// Get parent checkpoint
	parent := n.chain.GetLatest()
	var parentHash []byte
	if parent != nil {
		// Use canonical parent hash
		parentHash = crypto.ComputeParentHash(parent)
	}

	// Compute merkle roots
	stateRoot := n.computeStateRoot()
	agentRoot := n.computeAgentRoot()
	eventRoot := n.computeEventRoot()

	header := &pb.CheckpointHeader{
		Epoch:        n.currentEpoch,
		ParentCpHash: parentHash,
		Timestamp:    time.Now().Unix(),
		SubnetId:     n.config.SubnetID,
		// Set merkle roots - all roots are now properly populated
		Roots: &pb.CommitmentRoots{
			StateRoot: stateRoot,
			AgentRoot: agentRoot,
			EventRoot: eventRoot,
		},
	}

	return header
}

// signCheckpoint creates ECDSA signature for checkpoint
func (n *Node) signCheckpoint(header *pb.CheckpointHeader) (*pb.Signature, error) {
	// Use the new canonical hasher
	hasher := crypto.NewCheckpointHasher()
	msgHash := hasher.ComputeHash(header)

	// Sign the hash
	sigBytes, err := n.signer.Sign(msgHash[:])
	if err != nil {
		return nil, err
	}

	return &pb.Signature{
		Algo:     string(crypto.ECDSA_SECP256K1),
		Der:      sigBytes,
		Pubkey:   n.signer.PublicKey(),
		MsgHash:  msgHash[:],
		SignerId: n.id,
	}, nil
}

// computeStateRoot computes the state merkle root
func (n *Node) computeStateRoot() []byte {
	// Get validators from ValidatorSet
	if n.validatorSet == nil {
		return make([]byte, 32)
	}

	validators := n.validatorSet.Validators

	validatorIDs := make([]string, len(validators))
	balances := make([]uint64, len(validators))

	for i, v := range validators {
		validatorIDs[i] = v.ID
		// In production, get actual stake/balance from state
		balances[i] = v.Weight
	}

	return crypto.ComputeStateRoot(validatorIDs, balances)
}

// computeAgentRoot computes the agent merkle root
func (n *Node) computeAgentRoot() []byte {
	// Get agents from registry
	agents := []string{}
	statuses := []string{}

	if n.agentRegistry != nil {
		agentList := n.agentRegistry.ListAgents()
		for _, agent := range agentList {
			agents = append(agents, agent.ID)

			// Map status to string
			statusStr := "inactive"
			if agent.Status == pb.AgentStatus_AGENT_STATUS_ACTIVE {
				statusStr = "active"
			} else if agent.Status == pb.AgentStatus_AGENT_STATUS_UNHEALTHY {
				statusStr = "unhealthy"
			}
			statuses = append(statuses, statusStr)
		}
	}

	return crypto.ComputeAgentRoot(agents, statuses)
}

// computeEventRoot computes the event/report merkle root
func (n *Node) computeEventRoot() []byte {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Convert pending reports to array
	reports := make([]*pb.ExecutionReport, 0, len(n.pendingReports))
	for _, report := range n.pendingReports {
		reports = append(reports, report)
	}

	return crypto.ComputeEventRoot(reports)
}

// finalizeCheckpoint finalizes the checkpoint with collected signatures
func (n *Node) finalizeCheckpoint() {
	header := n.fsm.GetCurrentHeader()
	if header == nil {
		return
	}

	// Create signature bitmap
	bitmap := n.createSignersBitmap()

	// Update header with signatures
	if header.Signatures == nil {
		header.Signatures = &pb.CheckpointSignatures{}
	}
	header.Signatures.SignersBitmap = bitmap
	header.Signatures.SignatureCount = uint32(len(n.signatures))

	// Collect ECDSA signatures in deterministic order
	ecdsaSigs := make([][]byte, 0, len(n.signatures))
	totalWeight := uint64(0)

	// Process signatures in validator set order for determinism
	for _, validator := range n.validatorSet.Validators {
		if sig, exists := n.signatures[validator.ID]; exists {
			ecdsaSigs = append(ecdsaSigs, sig.Der)
			totalWeight += validator.Weight
		}
	}

	header.Signatures.EcdsaSignatures = ecdsaSigs
	header.Signatures.TotalWeight = totalWeight

	// Store checkpoint in chain
	if err := n.chain.AddCheckpoint(header); err != nil {
		n.logger.Error("Failed to store checkpoint", "error", err)
		return
	}

	// Persist checkpoint to storage
	if err := n.saveCheckpoint(header); err != nil {
		n.logger.Error("Failed to save checkpoint to storage", "error", err)
		// Continue even if save fails
	}

	// Update FSM state
	n.fsm.Finalize()
	n.currentCheckpoint = header

	n.logger.Info("Finalized checkpoint",
		"epoch", header.Epoch,
		"signatures", len(n.signatures))

	// Submit to RootLayer if we are the leader
	_, leader := n.leaderTracker.Leader(n.currentEpoch)
	if leader != nil && leader.ID == n.id {
		n.submitToRootLayer(header)
	}
}

// checkpointTimer periodically triggers checkpoint creation
func (n *Node) checkpointTimer() {
	// Use configured checkpoint interval
	checkpointInterval := n.getCheckpointInterval()

	ticker := time.NewTicker(checkpointInterval)
	defer ticker.Stop()

	n.logger.Info("Checkpoint timer started", "interval", checkpointInterval)

	for {
		select {
		case <-n.ctx.Done():
			n.logger.Info("Checkpoint timer stopped")
			return
		case <-ticker.C:
			n.triggerCheckpoint()
		}
	}
}

// triggerCheckpoint initiates a new checkpoint if conditions are met
func (n *Node) triggerCheckpoint() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if we're already in a checkpoint process
	state := n.fsm.GetState()
	if state != consensus.StateIdle {
		n.logger.Debug("Skipping checkpoint trigger, FSM not idle", "state", state)
		return
	}

	// Check if we have any reports to include
	if len(n.pendingReports) == 0 {
		n.logger.Debug("No pending reports, skipping checkpoint")
		return
	}

	// Check if we're the leader for current epoch
	_, leader := n.leaderTracker.Leader(n.currentEpoch)
	if leader == nil {
		n.logger.Warn("No leader for current epoch", "epoch", n.currentEpoch)
		return
	}

	if leader.ID == n.id {
		n.logger.Info("Triggering checkpoint as leader", "epoch", n.currentEpoch, "reports", len(n.pendingReports))
		n.proposeCheckpoint()
	}
}

func (n *Node) startMetricsServer() {
	prom := metrics.NewProm()
	n.metrics = prom

	mux := http.NewServeMux()
	mux.Handle("/metrics", prom.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/v1/execution-report", n.handleHTTPExecutionReport)

	addr := fmt.Sprintf(":%d", n.config.MetricsPort)
	n.metricsServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := n.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			n.logger.Warn("Metrics server exited with error", "error", err)
		}
	}()

	go func() {
		<-n.ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if n.metricsServer != nil {
			if err := n.metricsServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				n.logger.Warn("Failed to shutdown metrics server", "error", err)
			}
		}
	}()
}

func (n *Node) recordMetrics(state consensus.State) {
	if n.metrics == nil {
		return
	}

	n.metrics.SetGauge("fsm_state", float64(state))
	n.metrics.SetGauge("consensus_epoch", float64(n.currentEpoch))
	n.metrics.SetGauge("fail_queue_depth", float64(len(n.pendingReports)))
}

func (n *Node) startValidatorRegistry() {
	if n.agentRegistry == nil || n.config.RegistryEndpoint == "" {
		return
	}

	info := &registry.ValidatorInfo{
		ID:       n.id,
		Endpoint: n.config.RegistryEndpoint,
		LastSeen: time.Now(),
		Status:   pb.AgentStatus_AGENT_STATUS_ACTIVE,
	}

	if err := n.agentRegistry.RegisterValidator(info); err != nil {
		n.logger.Warn("Failed to register validator in registry", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(n.ctx)
	n.validatorHB = cancel
	go n.validatorHeartbeat(ctx)
}

func (n *Node) validatorHeartbeat(ctx context.Context) {
	interval := n.config.RegistryHeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := n.agentRegistry.UpdateValidatorHeartbeat(n.id); err != nil {
				n.logger.Warn("Failed to update validator heartbeat", "error", err)
			}
		}
	}
}

type httpExecutionReport struct {
	ReportID     string `json:"report_id"`
	AssignmentID string `json:"assignment_id"`
	IntentID     string `json:"intent_id"`
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	ResultData   string `json:"result_data"`
	Timestamp    int64  `json:"timestamp"`
}

type httpExecutionReceipt struct {
	ReportID    string `json:"report_id"`
	IntentID    string `json:"intent_id"`
	ValidatorID string `json:"validator_id"`
	Status      string `json:"status"`
	ReceivedTs  int64  `json:"received_ts"`
	Message     string `json:"message,omitempty"`
}

func (n *Node) handleHTTPExecutionReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload httpExecutionReport
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		n.writeHTTPError(w, http.StatusBadRequest, fmt.Sprintf("invalid payload: %v", err))
		return
	}

	if payload.ReportID == "" || payload.AssignmentID == "" || payload.IntentID == "" || payload.AgentID == "" {
		n.writeHTTPError(w, http.StatusBadRequest, "report_id, assignment_id, intent_id, agent_id are required")
		return
	}

	status, err := parseReportStatus(payload.Status)
	if err != nil {
		n.writeHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}

	var resultData []byte
	if payload.ResultData != "" {
		data, err := base64.StdEncoding.DecodeString(payload.ResultData)
		if err != nil {
			// fall back to raw string bytes
			data = []byte(payload.ResultData)
		}
		resultData = data
	}

	report := &pb.ExecutionReport{
		ReportId:     payload.ReportID,
		AssignmentId: payload.AssignmentID,
		IntentId:     payload.IntentID,
		AgentId:      payload.AgentID,
		Status:       status,
		ResultData:   resultData,
		Timestamp:    payload.Timestamp,
	}
	if report.Timestamp == 0 {
		report.Timestamp = time.Now().Unix()
	}

	receipt, err := n.ProcessExecutionReport(report)
	if err != nil {
		n.writeHTTPError(w, http.StatusInternalServerError, fmt.Sprintf("process report failed: %v", err))
		return
	}

	resp := httpExecutionReceipt{
		ReportID:    receipt.GetReportId(),
		IntentID:    receipt.GetIntentId(),
		ValidatorID: receipt.GetValidatorId(),
		Status:      receipt.GetStatus(),
		ReceivedTs:  receipt.GetReceivedTs(),
	}

	n.writeJSON(w, http.StatusOK, resp)
}

func parseReportStatus(status string) (pb.ExecutionReport_Status, error) {
	if status == "" {
		return pb.ExecutionReport_SUCCESS, nil
	}
	s := strings.ToLower(status)
	switch s {
	case "success", "ok", "accepted":
		return pb.ExecutionReport_SUCCESS, nil
	case "failed", "error", "rejected":
		return pb.ExecutionReport_FAILED, nil
	case "partial":
		return pb.ExecutionReport_PARTIAL, nil
	case "status_unspecified", "unspecified":
		return pb.ExecutionReport_STATUS_UNSPECIFIED, nil
	default:
		return pb.ExecutionReport_STATUS_UNSPECIFIED, fmt.Errorf("unknown status: %s", status)
	}
}

func (n *Node) writeHTTPError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (n *Node) writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// loadCheckpoint loads the latest checkpoint from storage
func (n *Node) loadCheckpoint() error {
	// Define storage keys
	const (
		keyLatestEpoch      = "checkpoint:latest:epoch"
		keyCheckpointPrefix = "checkpoint:epoch:"
		keySignaturesPrefix = "signatures:epoch:"
	)

	// Load latest epoch
	epochData, err := n.store.Get([]byte(keyLatestEpoch))
	if err != nil {
		if err.Error() == "key not found" || err.Error() == "leveldb: not found" {
			// No checkpoint stored yet, start from epoch 0
			n.currentEpoch = 0
			n.logger.Info("No stored checkpoint found, starting from epoch 0")
			return nil
		}
		return fmt.Errorf("failed to load latest epoch: %w", err)
	}

	// Parse epoch number
	epoch := uint64(0)
	if len(epochData) == 8 {
		epoch = binary.BigEndian.Uint64(epochData)
	}

	// Load checkpoint header
	checkpointKey := fmt.Sprintf("%s%d", keyCheckpointPrefix, epoch)
	checkpointData, err := n.store.Get([]byte(checkpointKey))
	if err != nil {
		return fmt.Errorf("failed to load checkpoint for epoch %d: %w", epoch, err)
	}

	// Unmarshal checkpoint header
	var header pb.CheckpointHeader
	if err := proto.Unmarshal(checkpointData, &header); err != nil {
		return fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	// Load signatures if they exist
	sigKey := fmt.Sprintf("%s%d", keySignaturesPrefix, epoch)
	sigData, err := n.store.Get([]byte(sigKey))
	if err == nil && len(sigData) > 0 {
		// Unmarshal signatures map
		var sigs map[string]*pb.Signature
		if err := json.Unmarshal(sigData, &sigs); err != nil {
			n.logger.Warn("Failed to unmarshal signatures", "error", err)
		} else {
			n.signatures = sigs
		}
	}

	// Update node state
	n.currentEpoch = epoch
	n.currentCheckpoint = &header

	// Add to chain
	if err := n.chain.AddCheckpoint(&header); err != nil {
		n.logger.Warn("Failed to add checkpoint to chain", "error", err)
	}

	n.logger.Info("Loaded checkpoint from storage",
		"epoch", epoch,
		"timestamp", header.Timestamp,
		"signatures", len(n.signatures))

	// Start from next epoch
	n.currentEpoch++

	return nil
}

// saveCheckpoint persists checkpoint to storage
func (n *Node) saveCheckpoint(header *pb.CheckpointHeader) error {
	const (
		keyLatestEpoch      = "checkpoint:latest:epoch"
		keyCheckpointPrefix = "checkpoint:epoch:"
		keySignaturesPrefix = "signatures:epoch:"
	)

	// Marshal checkpoint
	checkpointData, err := proto.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Save checkpoint
	checkpointKey := fmt.Sprintf("%s%d", keyCheckpointPrefix, header.Epoch)
	if err := n.store.Put([]byte(checkpointKey), checkpointData); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	// Save signatures
	if len(n.signatures) > 0 {
		sigData, err := json.Marshal(n.signatures)
		if err != nil {
			return fmt.Errorf("failed to marshal signatures: %w", err)
		}

		sigKey := fmt.Sprintf("%s%d", keySignaturesPrefix, header.Epoch)
		if err := n.store.Put([]byte(sigKey), sigData); err != nil {
			return fmt.Errorf("failed to save signatures: %w", err)
		}
	}

	// Update latest epoch
	epochData := make([]byte, 8)
	binary.BigEndian.PutUint64(epochData, header.Epoch)
	if err := n.store.Put([]byte(keyLatestEpoch), epochData); err != nil {
		return fmt.Errorf("failed to update latest epoch: %w", err)
	}

	n.logger.Debug("Saved checkpoint to storage", "epoch", header.Epoch)
	return nil
}

func (n *Node) broadcastProposal(header *pb.CheckpointHeader) error {
	if n.broadcaster != nil {
		return n.broadcaster.BroadcastProposal(header)
	}
	return fmt.Errorf("no broadcaster configured")
}

// computeReportsRoot is deprecated - use computeEventRoot instead
func (n *Node) computeReportsRoot() []byte {
	return n.computeEventRoot()
}

func (n *Node) createSignersBitmap() []byte {
	// Create bitmap of signers
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.validatorSet == nil || len(n.signatures) == 0 {
		return []byte{}
	}

	// Create bitmap with enough bytes for all validators
	numValidators := len(n.validatorSet.Validators)
	bitmapSize := (numValidators + 7) / 8
	bitmap := make([]byte, bitmapSize)

	// Set bits for validators who have signed
	for validatorID := range n.signatures {
		// Find validator index
		for idx, validator := range n.validatorSet.Validators {
			if validator.ID == validatorID {
				// Set bit for this validator
				byteIdx := idx / 8
				bitIdx := uint(idx % 8)
				if byteIdx < len(bitmap) {
					bitmap[byteIdx] |= 1 << bitIdx
				}
				break
			}
		}
	}

	return bitmap
}

func (n *Node) clearEpochData() {
	n.pendingReports = make(map[string]*pb.ExecutionReport)
	n.reportScores = make(map[string]int32)
	n.signatures = make(map[string]*pb.Signature)
}

// submitToRootLayer submits the finalized checkpoint to RootLayer
func (n *Node) submitToRootLayer(header *pb.CheckpointHeader) {
	if n.rootlayerClient == nil || !n.rootlayerClient.IsConnected() {
		n.logger.Warn("Cannot submit to RootLayer: client not connected")
		return
	}

	// Build ValidationBundle from checkpoint
	bundle := n.buildValidationBundle(header)
	if bundle == nil {
		n.logger.Error("Failed to build validation bundle")
		return
	}

	// Submit to RootLayer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := n.rootlayerClient.SubmitValidationBundle(ctx, bundle); err != nil {
		n.logger.Error("Failed to submit validation bundle to RootLayer", "error", err)
		return
	}

	n.logger.Info("Successfully submitted validation bundle to RootLayer",
		"epoch", header.Epoch,
		"signatures", len(n.signatures))
}

// buildValidationBundle creates a ValidationBundle from the checkpoint
func (n *Node) buildValidationBundle(header *pb.CheckpointHeader) *rootpb.ValidationBundle {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Convert checkpoint signatures to ValidationSignatures
	var validationSigs []*rootpb.ValidationSignature
	for validatorID, sig := range n.signatures {
		validationSigs = append(validationSigs, &rootpb.ValidationSignature{
			Validator: validatorID,  // Changed from ValidatorId to Validator
			Signature: sig.Der,
			// MsgHash field removed - not in new proto definition
		})
	}

	// Calculate result hash from checkpoint header
	resultHash := sha256.Sum256([]byte(fmt.Sprintf("%v", header)))

	// Get execution reports root as proof hash
	var proofHash []byte
	if header.Roots != nil && len(header.Roots.EventRoot) > 0 {
		proofHash = header.Roots.EventRoot
	}

	bundle := &rootpb.ValidationBundle{
		SubnetId:     n.config.SubnetID,
		IntentId:     fmt.Sprintf("checkpoint-%d", header.Epoch), // Use epoch as intent ID for checkpoints
		AssignmentId: fmt.Sprintf("epoch-%d", header.Epoch),
		AgentId:      "subnet-consensus", // Special agent ID for consensus checkpoints
		RootHeight:   header.Epoch,
		RootHash:     fmt.Sprintf("%x", header.ParentCpHash),
		ExecutedAt:   header.Timestamp,
		ResultHash:   resultHash[:],
		ProofHash:    proofHash,
		Signatures:   validationSigs,
		SignerBitmap: header.Signatures.SignersBitmap,
		TotalWeight:  header.Signatures.TotalWeight,
		AggregatorId: n.id, // Current validator as aggregator
		CompletedAt:  time.Now().Unix(),
	}

	return bundle
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.ValidatorID == "" {
		return fmt.Errorf("validator ID is required")
	}
	if c.PrivateKey == "" {
		return fmt.Errorf("private key is required")
	}
	if c.ValidatorSet == nil {
		return fmt.Errorf("validator set is required")
	}
	if err := c.ValidatorSet.Validate(); err != nil {
		return fmt.Errorf("invalid validator set: %w", err)
	}
	// Apply defaults from unified config
	c.applyDefaults()
	return nil
}

// applyDefaults applies default values from unified config structures
func (c *Config) applyDefaults() {
	// Initialize unified configs if not provided
	if c.Timeouts == nil {
		c.Timeouts = config.DefaultTimeoutConfig()
	}
	if c.Network == nil {
		c.Network = config.DefaultNetworkConfig()
	}
	if c.Identity == nil {
		c.Identity = config.DefaultIdentityConfig()
	}
	if c.Limits == nil {
		c.Limits = config.DefaultLimitsConfig()
	}

	// Apply legacy fields from unified config if not set
	if c.ProposeTimeout == 0 {
		c.ProposeTimeout = c.Timeouts.ProposeTimeout
	}
	if c.CollectTimeout == 0 {
		c.CollectTimeout = c.Timeouts.CollectTimeout
	}
	if c.FinalizeTimeout == 0 {
		c.FinalizeTimeout = c.Timeouts.FinalizeTimeout
	}
	if c.CheckpointInterval == 0 {
		c.CheckpointInterval = c.Timeouts.CheckpointInterval
	}
	if c.NATSUrl == "" {
		c.NATSUrl = c.Network.NATSUrl
	}
	if c.SubnetID == "" {
		c.SubnetID = c.Identity.SubnetID
	}

	if c.RegistryHeartbeatInterval <= 0 {
		c.RegistryHeartbeatInterval = 30 * time.Second
	}
}

// computeCheckpointHash computes the canonical hash of a checkpoint header
// DEPRECATED: Use crypto.NewCheckpointHasher().ComputeHash() instead
func (n *Node) computeCheckpointHash(header *pb.CheckpointHeader) [32]byte {
	hasher := crypto.NewCheckpointHasher()
	return hasher.ComputeHash(header)
}

// Helper methods to get configuration values
func (n *Node) getCheckpointInterval() time.Duration {
	if n.config.CheckpointInterval > 0 {
		return n.config.CheckpointInterval
	}
	if n.config.Timeouts != nil {
		return n.config.Timeouts.CheckpointInterval
	}
	return 30 * time.Second
}
