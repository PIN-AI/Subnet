package validator

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"github.com/PIN-AI/intent-protocol-contract-sdk/sdk/addressbook"
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

	// SDK client for blockchain operations
	sdkClient *sdk.Client

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
	lastCheckpointAt  time.Time                // Track last checkpoint time
	isLeader          bool                     // Track current leadership status

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

	// Blockchain config for ValidationBundle signing
	EnableChainSubmit  bool   // Enable on-chain ValidationBundle submission
	ChainRPCURL        string // Blockchain RPC URL
	ChainNetwork       string // Network name (e.g., "base_sepolia")
	IntentManagerAddr  string // IntentManager contract address

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
		// Use CompleteClient for full batch submission support
		completeConfig := &rootlayer.CompleteClientConfig{
			Endpoint:   config.RootLayerEndpoint,
			SubnetID:   config.SubnetID,
			NodeID:     config.ValidatorID,
			NodeType:   "validator",
			PrivateKey: config.PrivateKey,
		}
		var err error
		rootClient, err = rootlayer.NewCompleteClient(completeConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create RootLayer client: %w", err)
		}
	}

	// Initialize SDK client if on-chain submission is enabled
	var sdkClient *sdk.Client
	if config.EnableChainSubmit {
		if config.ChainRPCURL == "" {
			return nil, fmt.Errorf("chain_rpc_url is required when enable_chain_submit is true")
		}
		if config.ChainNetwork == "" {
			return nil, fmt.Errorf("chain_network is required when enable_chain_submit is true")
		}
		if config.IntentManagerAddr == "" {
			return nil, fmt.Errorf("intent_manager_addr is required when enable_chain_submit is true")
		}

		// Create SDK config
		sdkConfig := sdk.Config{
			RPCURL:        config.ChainRPCURL,
			PrivateKeyHex: config.PrivateKey,
			Network:       config.ChainNetwork,
			Addresses: &addressbook.Addresses{
				IntentManager: common.HexToAddress(config.IntentManagerAddr),
			},
		}

		// Initialize SDK client
		var err error
		sdkClient, err = sdk.NewClient(context.Background(), sdkConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize SDK client: %w", err)
		}

		logger.Info("SDK client initialized for ValidationBundle signing",
			"rpc_url", config.ChainRPCURL,
			"network", config.ChainNetwork,
			"intent_manager", config.IntentManagerAddr)
	} else {
		logger.Info("ValidationBundle blockchain signing disabled")
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
		sdkClient:       sdkClient,
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
		logger.Warnf("Failed to load checkpoint error=%v", err)
	}

	return node, nil
}

// GetID returns the validator node's ID
func (n *Node) GetID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.id
}

// Start starts the validator node
func (n *Node) Start(ctx context.Context) error {
	n.logger.Infof("Starting validator node id=%s", n.id)

	// Connect to RootLayer if configured
	if n.rootlayerClient != nil {
		if err := n.rootlayerClient.Connect(); err != nil {
			n.logger.Warnf("Failed to connect to RootLayer error=%v", err)
			// Continue without RootLayer for now
		} else {
			n.logger.Info("Connected to RootLayer")
		}
	}

	// Connect to NATS
	if err := n.connectNATS(); err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Wait for all validators to be ready on the NATS bus before starting consensus
	n.awaitConsensusReadiness()

	// Start metrics server if configured
	if n.config.MetricsPort > 0 {
		n.startMetricsServer()
	}

	// Register validator in registry and start heartbeat
	n.startValidatorRegistry()

	// Initialize leadership status
	_, leader := n.leaderTracker.Leader(n.currentEpoch)
	n.isLeader = leader != nil && leader.ID == n.id
	n.logger.Infof("Initial leadership status is_leader=%t epoch=%d", n.isLeader, n.currentEpoch)

	// Start consensus loop - this will handle checkpoint creation dynamically
	go n.consensusLoop()

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
			n.logger.Errorf("Failed to close broadcaster error=%v", err)
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
			n.logger.Errorf("Failed to close RootLayer connection error=%v", err)
		}
	}

	// Close SDK client
	if n.sdkClient != nil {
		n.sdkClient.Close()
	}

	if n.agentRegistry != nil {
		if err := n.agentRegistry.RemoveValidator(n.id); err != nil {
			n.logger.Warnf("Failed to remove validator from registry error=%v", err)
		}
	}

	if n.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := n.metricsServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
			n.logger.Warnf("Failed to shutdown metrics server error=%v", err)
		}
	}

	// Close storage
	if n.store != nil {
		if err := n.store.Close(); err != nil {
			n.logger.Errorf("Failed to close storage error=%v", err)
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
			n.logger.Errorf("Failed to handle proposal error=%v", err)
		}
	}); err != nil {
		return fmt.Errorf("failed to subscribe to proposals: %w", err)
	}

	// Subscribe to signatures
	if err := broadcaster.SubscribeToSignatures(func(sig *pb.Signature) {
		if err := n.AddSignature(sig); err != nil {
			n.logger.Errorf("Failed to add signature error=%v", err)
		}
	}); err != nil {
		return fmt.Errorf("failed to subscribe to signatures: %w", err)
	}

	// Subscribe to execution reports
	if err := broadcaster.SubscribeToExecutionReports(func(report *pb.ExecutionReport) {
		if _, err := n.ProcessExecutionReport(report); err != nil {
			n.logger.Errorf("Failed to process execution report error=%v", err)
		}
	}); err != nil {
		return fmt.Errorf("failed to subscribe to execution reports: %w", err)
	}

	// Subscribe to finalized checkpoints
	if err := broadcaster.SubscribeToFinalized(func(header *pb.CheckpointHeader) {
		if err := n.HandleFinalized(header); err != nil {
			n.logger.Errorf("Failed to handle finalized checkpoint error=%v", err)
		}
	}); err != nil {
		return fmt.Errorf("failed to subscribe to finalized checkpoints: %w", err)
	}

	n.logger.Infof("Connected to NATS with broadcaster url=%s subnet=%s", n.config.NATSUrl, subnetID)
	return nil
}

// awaitConsensusReadiness blocks startup until a quorum of validators have subscribed to
// the NATS subjects used for consensus. This prevents the first leader from proposing before
// followers have active subscriptions, which previously caused proposals to be dropped.
func (n *Node) awaitConsensusReadiness() {
	if n.broadcaster == nil || n.validatorSet == nil {
		return
	}

	required := make(map[string]struct{})
	for _, v := range n.validatorSet.Validators {
		required[v.ID] = struct{}{}
	}

	if _, ok := required[n.id]; !ok {
		required[n.id] = struct{}{}
	}

	total := len(required)
	if total == 0 {
		return
	}

	ready := make(map[string]struct{}, total)
	ready[n.id] = struct{}{}
	readyCount := len(ready)

	const (
		readinessTimeout  = 8 * time.Second
		readinessInterval = 500 * time.Millisecond
	)

	readyCh := make(chan string, total*2)

	if err := n.broadcaster.SubscribeToReadiness(func(id string) {
		select {
		case readyCh <- id:
		default:
			n.logger.Debug("Dropping readiness signal (channel full)", "validator_id", id)
		}
	}); err != nil {
		n.logger.Warnf("Failed to subscribe to consensus readiness error=%v", err)
		if err := n.broadcaster.BroadcastReadiness(n.id); err != nil {
			n.logger.Warnf("Failed to broadcast readiness error=%v", err)
		}
		return
	}

	start := time.Now()
	if err := n.broadcaster.BroadcastReadiness(n.id); err != nil {
		n.logger.Warnf("Failed to broadcast readiness error=%v", err)
	} else {
		n.logger.Infof("Announced consensus readiness validator_id=%s", n.id)
	}

	if readyCount >= total {
		n.logger.Debug("Consensus readiness satisfied locally", "validators", total)
		return
	}

	ticker := time.NewTicker(readinessInterval)
	defer ticker.Stop()
	timeout := time.NewTimer(readinessTimeout)
	defer timeout.Stop()

	for readyCount < total {
		select {
		case id := <-readyCh:
			if _, ok := required[id]; !ok {
				n.logger.Debug("Ignoring readiness from unknown validator", "validator_id", id)
				continue
			}
			if _, seen := ready[id]; seen {
				continue
			}
			ready[id] = struct{}{}
			readyCount++
			n.logger.Infof("Validator ready for consensus validator_id=%s ready=%d total=%d", id, readyCount, total)
			if readyCount >= total {
				n.logger.Infof("All validators ready for consensus total=%d wait_duration=%s", total, time.Since(start))
				return
			}
		case <-ticker.C:
			if err := n.broadcaster.BroadcastReadiness(n.id); err != nil {
				n.logger.Warnf("Failed to re-broadcast readiness error=%v", err)
			}
		case <-timeout.C:
			n.logger.Warnf("Timed out waiting for validator readiness ready=%d total=%d", readyCount, total)
			return
		case <-n.ctx.Done():
			n.logger.Debug("Stopped waiting for readiness (context cancelled)")
			return
		}
	}
}

// consensusLoop runs the main consensus state machine
func (n *Node) consensusLoop() {
	n.logger.Infof("🔄 Consensus loop started validator_id=%s", n.id)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	tickCount := 0
	for {
		select {
		case <-n.ctx.Done():
			n.logger.Infof("Consensus loop stopped (context done) ticks=%d", tickCount)
			return
		case <-ticker.C:
			tickCount++
			if tickCount%10 == 0 {
				n.logger.Debug("Consensus loop tick", "count", tickCount)
			}
			n.checkConsensusState()
			n.checkCheckpointTrigger() // Check if leader should create checkpoint
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
		// FIX: Remove auto-proposal logic here!
		// checkCheckpointTrigger() already handles this WITH pending_reports check
		// This was causing empty checkpoints to be created unnecessarily
		n.mu.Unlock()

	case consensus.StateCollecting:
		// Check if we have threshold signatures
		hasThreshold := n.fsm.HasThreshold()
		signCount, required, progress := n.fsm.GetProgress()
		isTimedOut := n.fsm.IsTimedOut(n.config.CollectTimeout)
		isLeader := false
		_, leader := n.leaderTracker.Leader(n.currentEpoch)
		if leader != nil && leader.ID == n.id {
			isLeader = true
		}

		n.logger.Infof("🔍 checkConsensusState: StateCollecting epoch=%d has_threshold=%t sign_count=%d required=%d progress=%.2f timed_out=%t is_leader=%t node_signatures=%d", n.currentEpoch, hasThreshold, signCount, required, progress, isTimedOut, isLeader, len(n.signatures))

		// Check if leader has failed (timeout failover mechanism)
		shouldFailover := n.leaderTracker.ShouldFailover(n.currentEpoch, time.Now())

		// Fix: Only finalize once when StateCollecting AND threshold reached
		// After finalize, state becomes StateFinalized, won't enter this branch again
		if hasThreshold {
			// Fix: Record leader activity (reaching threshold means leader is working properly)
			n.leaderTracker.RecordActivity(n.currentEpoch, time.Now())
			n.logger.Infof("✅ Threshold reached! Calling finalizeCheckpoint() epoch=%d sign_count=%d", n.currentEpoch, signCount)
			n.mu.Unlock()
			n.logger.Infof("🚀 About to call finalizeCheckpoint() epoch=%d", n.currentEpoch)
			n.finalizeCheckpoint()
			n.logger.Infof("🏁 Returned from finalizeCheckpoint() epoch=%d", n.currentEpoch)
			return // IMPORTANT: Return directly after finalize to avoid checking timeout
		} else if isTimedOut {
			n.mu.Unlock()
			if shouldFailover {
				// Fix: Add null check before accessing leader.ID
				var failedLeaderID string
				if leader != nil {
					failedLeaderID = leader.ID
				} else {
					failedLeaderID = "unknown"
				}

				// Leader timeout! Force epoch rotation
				n.logger.Warnf("⚠️ Leader timeout detected, forcing epoch rotation epoch=%d failed_leader=%s", n.currentEpoch, failedLeaderID)

				n.mu.Lock()
				oldEpoch := n.currentEpoch
				n.currentEpoch++ // Force move to next epoch
				n.fsm.Reset()
				n.signatures = make(map[string]*pb.Signature)

				// Fix Problem 10: Clear pendingReports during leader failover
				// When a leader fails and we rotate, the old pending reports are abandoned
				// The new leader should start fresh to avoid submitting stale data
				n.pendingReports = make(map[string]*pb.ExecutionReport)
				n.reportScores = make(map[string]int32)

				// Recalculate leader
				_, newLeader := n.leaderTracker.Leader(n.currentEpoch)
				n.isLeader = newLeader != nil && newLeader.ID == n.id
				n.lastCheckpointAt = time.Time{} // Reset checkpoint timer
				n.mu.Unlock()

				// Fix: Add null check before accessing newLeader.ID
				var newLeaderID string
				if newLeader != nil {
					newLeaderID = newLeader.ID
				} else {
					newLeaderID = "unknown"
				}

				n.logger.Infof("🔄 Epoch rotated due to leader timeout old_epoch=%d new_epoch=%d new_leader=%s i_am_new_leader=%t", oldEpoch, n.currentEpoch, newLeaderID, n.isLeader)
			} else if isLeader {
				// Leader not timed out yet, retry
				n.logger.Warnf("Signature collection timeout, leader will retry epoch=%d", n.currentEpoch)
				n.mu.Lock()
				n.fsm.Reset()
				n.mu.Unlock()
				return
			} else {
				// Follower waiting
				n.logger.Warnf("Signature collection timeout, waiting for leader epoch=%d", n.currentEpoch)
				return
			}
		} else {
			// Neither threshold reached nor timed out, continue collecting
			n.mu.Unlock()
			return
		}

	case consensus.StateFinalized:
		// Move to next epoch
		oldEpoch := n.currentEpoch
		n.currentEpoch++
		n.fsm.Reset()
		// DON'T clear epoch data here anymore - will be cleared after ValidationBundle submission
		// Only clear signatures since they're epoch-specific
		n.signatures = make(map[string]*pb.Signature)

		// Fix Problem 9: Clear currentCheckpoint so new proposals start fresh
		// The finalized checkpoint is already stored in chain and storage
		n.currentCheckpoint = nil

		// Check if leader changed - update leadership status for dynamic rotation
		_, oldLeader := n.leaderTracker.Leader(oldEpoch)
		_, newLeader := n.leaderTracker.Leader(n.currentEpoch)

		wasLeader := n.isLeader
		n.isLeader = newLeader != nil && newLeader.ID == n.id

		if oldLeader != nil && newLeader != nil {
			if oldLeader.ID != newLeader.ID {
				n.logger.Infof("Leader rotation old_epoch=%d old_leader=%s new_epoch=%d new_leader=%s i_am_new_leader=%t", oldEpoch, oldLeader.ID, n.currentEpoch, newLeader.ID, n.isLeader)
			}
		}

		// Reset checkpoint timer if leadership changed
		if wasLeader != n.isLeader {
			n.lastCheckpointAt = time.Time{} // Reset timer
			if n.isLeader {
				n.logger.Infof("Became leader - checkpoint timer reset epoch=%d", n.currentEpoch)
			} else {
				n.logger.Infof("No longer leader epoch=%d", n.currentEpoch)
			}
		}

		n.mu.Unlock()

	default:
		n.mu.Unlock()
	}
}

// proposeCheckpoint creates and broadcasts a new checkpoint proposal
func (n *Node) proposeCheckpoint() {
	// Build checkpoint header first (without lock, as it needs to acquire RLock)
	header := n.buildCheckpointHeader()

	// Now acquire lock for FSM updates and state changes
	n.mu.Lock()
	defer n.mu.Unlock()

	// Update FSM state
	if err := n.fsm.ProposeHeader(header); err != nil {
		n.logger.Errorf("Failed to propose header error=%v", err)
		return
	}

	// Sign the header
	sig, err := n.signCheckpoint(header)
	if err != nil {
		n.logger.Errorf("Failed to sign checkpoint error=%v", err)
		return
	}
	n.signatures[n.id] = sig

	// Set current checkpoint BEFORE broadcasting so it's ready when signatures arrive
	n.currentCheckpoint = header

	// Add leader's own signature to FSM - this transitions FSM to StateCollecting
	// and enables timeout mechanism!
	if err := n.fsm.AddSignature(sig); err != nil {
		n.logger.Errorf("Failed to add leader signature to FSM error=%v", err)
		return
	}

	// Broadcast proposal
	if err := n.broadcastProposal(header); err != nil {
		n.logger.Errorf("Failed to broadcast proposal error=%v", err)
		return
	}

	// Fix: Record leader activity when proposing checkpoint
	n.leaderTracker.RecordActivity(header.Epoch, time.Now())

	n.logger.Infof("Proposed checkpoint epoch=%d fsm_state=%s", header.Epoch, n.fsm.GetState())
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
// Fix: Add lock protection for accessing shared data
func (n *Node) finalizeCheckpoint() {
	n.mu.Lock()

	// Get header and copy needed data while holding lock
	header := n.fsm.GetCurrentHeader()
	if header == nil {
		n.mu.Unlock()
		return
	}

	// Create signature bitmap (while holding lock, so pass data directly to avoid nested lock)
	bitmap := n.createSignersBitmapLocked()

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
		n.logger.Errorf("Failed to store checkpoint error=%v", err)
		n.mu.Unlock()
		return
	}

	// Persist checkpoint to storage
	if err := n.saveCheckpoint(header); err != nil {
		n.logger.Errorf("Failed to save checkpoint to storage error=%v", err)
		// Continue even if save fails
	}

	// Update FSM state
	if err := n.fsm.Finalize(); err != nil {
		n.logger.Errorf("Failed to finalize FSM error=%v", err)
		n.mu.Unlock()
		return
	}
	n.currentCheckpoint = header

	n.logger.Infof("Finalized checkpoint epoch=%d signatures=%d", header.Epoch, len(n.signatures))

	// Fix: Clone header for async broadcast to avoid accessing shared data in goroutine
	headerCopy := proto.Clone(header).(*pb.CheckpointHeader)

	// Check if we are the leader for RootLayer submission
	_, leader := n.leaderTracker.Leader(n.currentEpoch)
	isLeader := leader != nil && leader.ID == n.id

	// CRITICAL FIX: Clone pendingReports and signatures BEFORE releasing lock
	// because submitToRootLayer needs these to build ValidationBundle,
	// but StateFinalized handler may clear them concurrently after we unlock
	pendingReportsCopy := make(map[string]*pb.ExecutionReport, len(n.pendingReports))
	for k, v := range n.pendingReports {
		pendingReportsCopy[k] = proto.Clone(v).(*pb.ExecutionReport)
	}
	signaturesCopy := make(map[string]*pb.Signature, len(n.signatures))
	for k, v := range n.signatures {
		signaturesCopy[k] = proto.Clone(v).(*pb.Signature)
	}

	n.mu.Unlock()

	// Broadcast finalized checkpoint to all validators
	// This helps followers sync their state even if they missed some signatures
	go n.broadcastFinalized(headerCopy)

	// Submit to RootLayer if we are the leader
	if isLeader {
		n.submitToRootLayerWithData(headerCopy, pendingReportsCopy, signaturesCopy)
	}
}

// checkCheckpointTrigger checks if it's time for the leader to create a checkpoint
// ONLY creates checkpoints when there are pending execution reports (on-demand checkpointing)
// Fix Problem 8: Removed double unlock bug by avoiding manual unlock/lock inside defer scope
func (n *Node) checkCheckpointTrigger() {
	n.mu.Lock()

	// Only leaders should create checkpoints
	if !n.isLeader {
		n.mu.Unlock()
		return
	}

	// Check if we're already in a checkpoint process
	state := n.fsm.GetState()
	if state != consensus.StateIdle {
		n.mu.Unlock()
		return
	}

	// SOLUTION B: Only create checkpoint if we have pending reports
	if len(n.pendingReports) == 0 {
		// No reports to validate - skip checkpoint creation
		n.mu.Unlock()
		return
	}

	// Check if enough time has passed since last checkpoint
	checkpointInterval := n.getCheckpointInterval()
	now := time.Now()
	shouldPropose := false

	if n.lastCheckpointAt.IsZero() {
		// First checkpoint for this leader - we have reports, trigger immediately
		n.logger.Infof("🎯 Leader triggering first checkpoint with execution reports epoch=%d pending_reports=%d", n.currentEpoch, len(n.pendingReports))
		n.lastCheckpointAt = now
		shouldPropose = true
	} else if now.Sub(n.lastCheckpointAt) >= checkpointInterval {
		// Time for next checkpoint - we have reports
		n.logger.Infof("🎯 Leader triggering checkpoint with execution reports epoch=%d pending_reports=%d interval=%s", n.currentEpoch, len(n.pendingReports), checkpointInterval)
		n.lastCheckpointAt = now
		shouldPropose = true
	}

	n.mu.Unlock()

	// Call proposeCheckpoint without holding lock
	if shouldPropose {
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
			n.logger.Warnf("Metrics server exited with error error=%v", err)
		}
	}()

	go func() {
		<-n.ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if n.metricsServer != nil {
			if err := n.metricsServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				n.logger.Warnf("Failed to shutdown metrics server error=%v", err)
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
		n.logger.Warnf("Failed to register validator in registry error=%v", err)
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
				n.logger.Warnf("Failed to update validator heartbeat error=%v", err)
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
			n.logger.Warnf("Failed to unmarshal signatures error=%v", err)
		} else {
			n.signatures = sigs
		}
	}

	// Update node state
	n.currentEpoch = epoch
	n.currentCheckpoint = &header

	// Add to chain
	if err := n.chain.AddCheckpoint(&header); err != nil {
		n.logger.Warnf("Failed to add checkpoint to chain error=%v", err)
	}

	n.logger.Infof("Loaded checkpoint from storage epoch=%d timestamp=%d signatures=%d", epoch, header.Timestamp, len(n.signatures))

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

// createSignersBitmapLocked creates a bitmap of signers.
// IMPORTANT: This function assumes the caller already holds n.mu (Lock or RLock).
// This version exists to prevent deadlock when called from within finalizeCheckpoint.
func (n *Node) createSignersBitmapLocked() []byte {
	// NO LOCK - assumes caller holds n.mu.Lock() or n.mu.RLock()

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

// groupReportsByIntent groups execution reports by (IntentID, AssignmentID, AgentID)
// This allows building separate ValidationBundles for each Intent
func (n *Node) groupReportsByIntent(reports map[string]*pb.ExecutionReport) map[string][]*pb.ExecutionReport {
	grouped := make(map[string][]*pb.ExecutionReport)

	for reportID, report := range reports {
		// Create composite key from Intent+Assignment+Agent
		key := fmt.Sprintf("%s:%s:%s", report.IntentId, report.AssignmentId, report.AgentId)
		grouped[key] = append(grouped[key], report)

		n.logger.Debug("Grouping execution report",
			"report_id", reportID,
			"intent_id", report.IntentId,
			"assignment_id", report.AssignmentId,
			"agent_id", report.AgentId,
			"group_key", key)
	}

	n.logger.Infof("Grouped execution reports into %d Intent groups", len(grouped))
	return grouped
}

// clearReportsForIntent removes execution reports for a specific Intent group
// Iterates through all pending reports and removes those matching the intentKey
func (n *Node) clearReportsForIntent(intentKey string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	removed := 0
	for reportID, report := range n.pendingReports {
		// Check if this report belongs to the Intent group
		key := fmt.Sprintf("%s:%s:%s", report.IntentId, report.AssignmentId, report.AgentId)
		if key == intentKey {
			delete(n.pendingReports, reportID)
			delete(n.reportScores, reportID)
			removed++
		}
	}

	n.logger.Infof("Cleared %d execution reports for Intent group %s", removed, intentKey)
}

// submitToRootLayer submits the finalized checkpoint to RootLayer
// DEPRECATED: Use submitToRootLayerWithData instead to avoid race conditions
func (n *Node) submitToRootLayer(header *pb.CheckpointHeader) {
	n.logger.Warn("DEPRECATED: submitToRootLayer called without data - use submitToRootLayerWithData instead")
	// Get data with locks
	n.mu.RLock()
	pendingReportsCopy := make(map[string]*pb.ExecutionReport, len(n.pendingReports))
	for k, v := range n.pendingReports {
		pendingReportsCopy[k] = proto.Clone(v).(*pb.ExecutionReport)
	}
	signaturesCopy := make(map[string]*pb.Signature, len(n.signatures))
	for k, v := range n.signatures {
		signaturesCopy[k] = proto.Clone(v).(*pb.Signature)
	}
	n.mu.RUnlock()

	n.submitToRootLayerWithData(header, pendingReportsCopy, signaturesCopy)
}

// submitToRootLayerWithData submits the finalized checkpoint to RootLayer
// FIXED: Now supports multiple Intents per checkpoint by grouping reports and submitting separate ValidationBundles
func (n *Node) submitToRootLayerWithData(header *pb.CheckpointHeader, pendingReports map[string]*pb.ExecutionReport, signatures map[string]*pb.Signature) {
	n.logger.Infof("Attempting to submit checkpoint to RootLayer epoch=%d pending_reports=%d signatures=%d is_connected=%t", header.Epoch, len(pendingReports), len(signatures), n.rootlayerClient != nil && n.rootlayerClient.IsConnected())

	if n.rootlayerClient == nil || !n.rootlayerClient.IsConnected() {
		n.logger.Warnf("Cannot submit to RootLayer: client not connected client_nil=%t epoch=%d", n.rootlayerClient == nil, header.Epoch)
		return
	}

	if len(pendingReports) == 0 {
		n.logger.Info("No pending reports to submit, skipping ValidationBundle submission epoch=%d", header.Epoch)
		return
	}

	// STEP 1: Group reports by Intent (IntentID, AssignmentID, AgentID)
	groupedReports := n.groupReportsByIntent(pendingReports)
	n.logger.Infof("Grouped %d execution reports into %d Intent groups for epoch %d", len(pendingReports), len(groupedReports), header.Epoch)

	// IMPORTANT: Wait for RootLayer state synchronization before submitting ValidationBundles
	//
	// RATIONALE:
	// After an Assignment is submitted to the blockchain by the Matcher, the RootLayer's
	// indexer needs time to sync the blockchain state and update the Intent's status
	// (e.g., from "PENDING" to "ASSIGNED"). If we submit the ValidationBundle too early,
	// the RootLayer will reject it with "Invalid intent status" because it hasn't seen
	// the Assignment transaction yet.
	//
	// CURRENT IMPLEMENTATION:
	// We use a conservative 15-second blocking sleep to ensure the RootLayer has enough
	// time to process the Assignment transaction. This is a simple, reliable approach
	// that works well in production with typical blockchain confirmation times.
	//
	// FUTURE OPTIMIZATION:
	// This synchronous wait can be replaced with an event-driven mechanism:
	// 1. Listen for Assignment confirmation events from blockchain
	// 2. Poll RootLayer's /intent/{id} endpoint until status changes to "ASSIGNED"
	// 3. Use exponential backoff retry instead of fixed delay
	// 4. Implement callback-based async submission pipeline
	//
	// The current approach is acceptable because:
	// - It's deterministic and easy to debug
	// - 15s is reasonable for blockchain finality
	// - The retry logic in submitSingleValidationBundle handles edge cases
	// - This only blocks the leader validator, not the entire subnet
	syncDelay := 15 * time.Second
	n.logger.Infof("⏳ Waiting %v for RootLayer state synchronization before submitting ValidationBundles epoch=%d (see node.go:1450 for rationale)",
		syncDelay, header.Epoch)
	time.Sleep(syncDelay)

	// STEP 2: Build ValidationBundles for all Intent groups
	bundles := make([]*rootpb.ValidationBundle, 0, len(groupedReports))
	intentKeys := make([]string, 0, len(groupedReports))

	for intentKey, reports := range groupedReports {
		n.logger.Infof("Processing Intent group %s with %d reports epoch=%d", intentKey, len(reports), header.Epoch)

		// Build ValidationBundle for this Intent
		bundle := n.buildValidationBundleForIntent(header, reports, signatures)
		if bundle == nil {
			n.logger.Errorf("⚠️ Failed to build ValidationBundle for Intent group %s epoch=%d", intentKey, header.Epoch)
			continue
		}

		bundles = append(bundles, bundle)
		intentKeys = append(intentKeys, intentKey)
	}

	// STEP 3: Submit all ValidationBundles in a single batch call
	successfulIntents, failedIntents := n.submitValidationBundleBatch(header, bundles, intentKeys)

	// STEP 4: Clear only successfully submitted Intent reports
	for _, intentKey := range successfulIntents {
		n.clearReportsForIntent(intentKey)
	}

	n.logger.Infof("ValidationBundle submission complete epoch=%d total_intents=%d successful=%d failed=%d",
		header.Epoch, len(groupedReports), len(successfulIntents), len(failedIntents))

	if len(failedIntents) > 0 {
		n.logger.Warnf("Some Intents failed ValidationBundle submission and will be retried in next checkpoint epoch=%d failed_intents=%v",
			header.Epoch, failedIntents)
	}
}

// submitValidationBundleBatch submits multiple ValidationBundles to RootLayer in a single batch call
// Returns lists of successful and failed intent keys
func (n *Node) submitValidationBundleBatch(header *pb.CheckpointHeader, bundles []*rootpb.ValidationBundle, intentKeys []string) (successfulIntents []string, failedIntents []string) {
	if len(bundles) == 0 {
		n.logger.Info("No ValidationBundles to submit epoch=%d", header.Epoch)
		return []string{}, []string{}
	}

	// Check if RootLayer client supports batch submission
	type batchSubmitter interface {
		SubmitValidationBundleBatch(ctx context.Context, bundles []*rootpb.ValidationBundle, batchID string, partialOk bool) (*rootpb.ValidationBundleBatchResponse, error)
	}

	batchClient, supportsBatch := n.rootlayerClient.(batchSubmitter)

	if supportsBatch {
		// Use batch submission API
		batchID := fmt.Sprintf("epoch-%d-%d", header.Epoch, time.Now().Unix())
		n.logger.Infof("📦 Submitting %d ValidationBundles in batch batch_id=%s epoch=%d", len(bundles), batchID, header.Epoch)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := batchClient.SubmitValidationBundleBatch(ctx, bundles, batchID, true)
		if err != nil {
			// Batch submission failed - fall back to individual submission
			n.logger.Warnf("⚠️ Batch submission failed, falling back to individual submission error=%v", err)
			return n.submitIndividualValidationBundles(header, bundles, intentKeys)
		}

		// Process batch response
		n.logger.Infof("✅ Batch submission completed batch_id=%s total=%d success=%d failed=%d",
			batchID, len(bundles), resp.Success, resp.Failed)

		// Collect results
		successfulIntents = make([]string, 0, resp.Success)
		failedIntents = make([]string, 0, resp.Failed)

		for i, result := range resp.Results {
			if i >= len(intentKeys) {
				break
			}
			if result.Ok {
				successfulIntents = append(successfulIntents, intentKeys[i])
			} else {
				failedIntents = append(failedIntents, intentKeys[i])
				n.logger.Warnf("Intent %s failed in batch: %s", bundles[i].IntentId, result.Msg)
			}
		}

		return successfulIntents, failedIntents
	}

	// Fallback: Client doesn't support batch submission
	n.logger.Warn("RootLayer client doesn't support batch submission, using individual submission")
	return n.submitIndividualValidationBundles(header, bundles, intentKeys)
}

// submitIndividualValidationBundles submits ValidationBundles one by one (fallback method)
func (n *Node) submitIndividualValidationBundles(header *pb.CheckpointHeader, bundles []*rootpb.ValidationBundle, intentKeys []string) (successfulIntents []string, failedIntents []string) {
	successfulIntents = make([]string, 0, len(bundles))
	failedIntents = make([]string, 0)

	for i, bundle := range bundles {
		if i >= len(intentKeys) {
			break
		}
		intentKey := intentKeys[i]

		if n.submitSingleValidationBundle(header, bundle, intentKey) {
			successfulIntents = append(successfulIntents, intentKey)
		} else {
			failedIntents = append(failedIntents, intentKey)
		}
	}

	return successfulIntents, failedIntents
}

// submitSingleValidationBundle submits a single ValidationBundle to RootLayer with retry logic
// Returns true if submission succeeded, false otherwise
func (n *Node) submitSingleValidationBundle(header *pb.CheckpointHeader, bundle *rootpb.ValidationBundle, intentKey string) bool {
	maxRetries := 5
	retryDelay := 10 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		n.logger.Infof("Submitting ValidationBundle to RootLayer epoch=%d intent_id=%s intent_key=%s attempt=%d/%d",
			header.Epoch, bundle.IntentId, intentKey, attempt, maxRetries)

		err := n.rootlayerClient.SubmitValidationBundle(ctx, bundle)
		cancel()

		if err == nil {
			// Success!
			n.logger.Infof("✅ Successfully submitted ValidationBundle to RootLayer epoch=%d intent_id=%s intent_key=%s attempt=%d",
				header.Epoch, bundle.IntentId, intentKey, attempt)
			return true
		}

		// Check if error is retryable
		isRetryable := strings.Contains(err.Error(), "Invalid intent status") ||
			strings.Contains(err.Error(), "intent status") ||
			strings.Contains(err.Error(), "assignment not found")

		if isRetryable {
			lastErr = err
			if attempt < maxRetries {
				n.logger.Warnf("⚠️ ValidationBundle submission failed due to RootLayer sync delay, will retry in %v attempt=%d/%d error=%v",
					retryDelay, attempt, maxRetries, err)
				time.Sleep(retryDelay)
				continue
			} else {
				n.logger.Errorf("❌ ValidationBundle submission failed after %d attempts due to RootLayer sync issue error=%v epoch=%d intent_id=%s",
					maxRetries, err, header.Epoch, bundle.IntentId)
			}
		} else {
			// Other error - don't retry
			n.logger.Errorf("Failed to submit ValidationBundle to RootLayer (non-retryable error) error=%v epoch=%d intent_id=%s attempt=%d",
				err, header.Epoch, bundle.IntentId, attempt)
			return false
		}
	}

	// All retries exhausted
	n.logger.Errorf("Failed to submit ValidationBundle to RootLayer after all retries error=%v epoch=%d intent_id=%s max_retries=%d",
		lastErr, header.Epoch, bundle.IntentId, maxRetries)
	return false
}

// buildValidationBundleForIntent creates a ValidationBundle for a single Intent group
// This version accepts an array of ExecutionReports for one Intent, not a map of all reports
func (n *Node) buildValidationBundleForIntent(header *pb.CheckpointHeader, reports []*pb.ExecutionReport, signatures map[string]*pb.Signature) *rootpb.ValidationBundle {
	if len(reports) == 0 {
		n.logger.Warn("Cannot build ValidationBundle: no reports provided")
		return nil
	}

	// Extract metadata from first report (all reports in this group have same IntentID/AssignmentID/AgentID)
	firstReport := reports[0]
	intentID := firstReport.IntentId
	assignmentID := firstReport.AssignmentId
	agentID := firstReport.AgentId

	n.logger.Infof("Building ValidationBundle for Intent group intent_id=%s assignment_id=%s agent_id=%s reports_count=%d epoch=%d",
		intentID, assignmentID, agentID, len(reports), header.Epoch)

	// Validate metadata
	if intentID == "" || assignmentID == "" || agentID == "" {
		n.logger.Errorf("⚠️ ValidationBundle construction failed: missing metadata intent_id=%s assignment_id=%s agent_id=%s",
			intentID, assignmentID, agentID)
		return nil
	}

	// Generate ValidationBundle signature using SDK (EIP-191 standard)
	var validationSigs []*rootpb.ValidationSignature

	if n.sdkClient != nil {
		// Prepare data for SDK ValidationBundle signing
		intentIDHash := common.HexToHash(intentID)
		assignmentIDHash := common.HexToHash(assignmentID)
		subnetIDHash := common.HexToHash(n.config.SubnetID)
		agentAddr := common.HexToAddress(agentID)

		// Calculate result hash from checkpoint header
		resultHash := sha256.Sum256([]byte(fmt.Sprintf("%v", header)))

		// Get execution reports root as proof hash
		var proofHash [32]byte
		if header.Roots != nil && len(header.Roots.EventRoot) > 0 {
			copy(proofHash[:], header.Roots.EventRoot)
		}

		// Parse root hash from parent checkpoint hash
		var rootHash [32]byte
		if len(header.ParentCpHash) > 0 {
			copy(rootHash[:], header.ParentCpHash)
		}

		// Create SDK ValidationBundle structure for signing
		bundle := sdk.ValidationBundle{
			IntentID:     intentIDHash,
			AssignmentID: assignmentIDHash,
			SubnetID:     subnetIDHash,
			Agent:        agentAddr,
			ResultHash:   resultHash,
			ProofHash:    proofHash,
			RootHeight:   header.Epoch,
			RootHash:     rootHash,
		}

		// Compute digest using SDK
		digest, err := n.sdkClient.Validation.ComputeDigest(bundle)
		if err != nil {
			n.logger.Errorf("Failed to compute ValidationBundle digest: %v", err)
		} else {
			// Sign digest using SDK (EIP-191 standard)
			signature, err := n.sdkClient.Validation.SignDigest(digest)
			if err != nil {
				n.logger.Errorf("Failed to sign ValidationBundle digest: %v", err)
			} else {
				validatorAddr := n.sdkClient.Signer.Address()
				validationSigs = append(validationSigs, &rootpb.ValidationSignature{
					Validator: validatorAddr.Hex(),
					Signature: signature,
				})
				n.logger.Infof("✅ Generated EIP-191 ValidationBundle signature validator=%s signature_len=%d",
					validatorAddr.Hex(), len(signature))
			}
		}
	} else {
		// Fallback: Use checkpoint signatures if SDK not available
		n.logger.Warn("SDK client not available, using checkpoint signatures as fallback (NOT EIP-191 compliant)")
		for validatorID, sig := range signatures {
			validationSigs = append(validationSigs, &rootpb.ValidationSignature{
				Validator: validatorID,
				Signature: sig.Der,
			})
		}
	}

	// Calculate result hash for ValidationBundle
	resultHash := sha256.Sum256([]byte(fmt.Sprintf("%v", header)))

	// Get execution reports root as proof hash
	var proofHash []byte
	if header.Roots != nil && len(header.Roots.EventRoot) > 0 {
		proofHash = header.Roots.EventRoot
	}

	// Format RootHash with proper handling for epoch 0 (genesis)
	// Use hex.EncodeToString to safely convert []byte to hex string
	var rootHashStr string
	if len(header.ParentCpHash) == 0 {
		// Epoch 0 has no parent - use zero hash
		rootHashStr = "0x0000000000000000000000000000000000000000000000000000000000000000"
	} else {
		// Safely encode bytes to hex string
		rootHashStr = "0x" + hex.EncodeToString(header.ParentCpHash)
	}

	bundle := &rootpb.ValidationBundle{
		SubnetId:     n.config.SubnetID,
		IntentId:     intentID,
		AssignmentId: assignmentID,
		AgentId:      agentID,
		RootHeight:   header.Epoch,
		RootHash:     rootHashStr,
		ExecutedAt:   header.Timestamp,
		ResultHash:   resultHash[:],
		ProofHash:    proofHash,
		Signatures:   validationSigs,
		SignerBitmap: header.Signatures.SignersBitmap,
		TotalWeight:  header.Signatures.TotalWeight,
		AggregatorId: n.id,
		CompletedAt:  time.Now().Unix(),
	}

	n.logger.Infof("✅ ValidationBundle constructed for Intent group intent_id=%s assignment_id=%s agent_id=%s epoch=%d signatures=%d",
		bundle.IntentId, bundle.AssignmentId, bundle.AgentId, header.Epoch, len(bundle.Signatures))

	return bundle
}

// buildValidationBundle creates a ValidationBundle from the checkpoint
// DEPRECATED: Use buildValidationBundleForIntent for multi-intent support
// WARNING: This function has the old single-intent bug and should NOT be used in production
func (n *Node) buildValidationBundle(header *pb.CheckpointHeader) *rootpb.ValidationBundle {
	n.logger.Warn("DEPRECATED: buildValidationBundle called - this function has the single-intent bug! Use submitToRootLayerWithData instead")
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.buildValidationBundleWithData(header, n.pendingReports, n.signatures)
}

// buildValidationBundleWithData creates a ValidationBundle from the checkpoint with provided data
// DEPRECATED: Use buildValidationBundleForIntent + submitToRootLayerWithData for proper multi-intent support
// This function now internally calls the new grouping logic to avoid the single-intent bug
// Returns the FIRST Intent's ValidationBundle for backward compatibility (warns if multiple Intents exist)
func (n *Node) buildValidationBundleWithData(header *pb.CheckpointHeader, pendingReports map[string]*pb.ExecutionReport, signatures map[string]*pb.Signature) *rootpb.ValidationBundle {
	n.logger.Warnf("⚠️ DEPRECATED: buildValidationBundleWithData called - use submitToRootLayerWithData for multi-intent support epoch=%d", header.Epoch)

	// If there are no pending reports, this is an empty consensus checkpoint - skip it
	if len(pendingReports) == 0 {
		n.logger.Warnf("⚠️ ValidationBundle construction skipped: no pending execution reports epoch=%d", header.Epoch)
		return nil
	}

	// Use the new grouping logic to properly handle multiple Intents
	groupedReports := n.groupReportsByIntent(pendingReports)

	// Warn if multiple Intent groups exist (backward compatibility issue)
	if len(groupedReports) > 1 {
		n.logger.Errorf("⚠️ CRITICAL: Multiple Intent groups detected (%d) but buildValidationBundleWithData can only return ONE! Other Intents will be LOST! Use submitToRootLayerWithData instead! epoch=%d",
			len(groupedReports), header.Epoch)
	}

	// For backward compatibility, return the first Intent's bundle
	for intentKey, reports := range groupedReports {
		n.logger.Infof("Building ValidationBundle for first Intent group (backward compat): %s epoch=%d", intentKey, header.Epoch)
		return n.buildValidationBundleForIntent(header, reports, signatures)
	}

	// No reports found - should not reach here due to check at beginning
	return nil
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
