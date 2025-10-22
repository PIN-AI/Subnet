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
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"github.com/PIN-AI/intent-protocol-contract-sdk/sdk/addressbook"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/protobuf/proto"
	rootpb "subnet/proto/rootlayer"
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
	raftConsensus  *consensus.RaftConsensus
	gossipManager  *consensus.GossipManager
	gossipDelegate *consensus.SignatureGossipDelegate
	fsm            *consensus.StateMachine
	leaderTracker  *consensus.LeaderTracker
	chain          *consensus.Chain

	// Storage
	store storage.Store

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
	EnableChainSubmit bool   // Enable on-chain ValidationBundle submission
	ChainRPCURL       string // Blockchain RPC URL
	ChainNetwork      string // Network name (e.g., "base_sepolia")
	IntentManagerAddr string // IntentManager contract address

	// Validation policy
	ValidationPolicy *ValidationPolicyConfig

	// Raft consensus configuration
	Raft *RaftConfig

	// Gossip signature propagation configuration
	Gossip *GossipConfig
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

// RaftConfig configures the embedded Raft consensus instance.
type RaftConfig struct {
	Enable           bool
	DataDir          string
	BindAddress      string
	Bootstrap        bool
	Peers            []RaftPeerConfig
	HeartbeatTimeout time.Duration
	ElectionTimeout  time.Duration
	CommitTimeout    time.Duration
	MaxPool          int
}

// RaftPeerConfig describes a known Raft peer.
type RaftPeerConfig struct {
	ID      string
	Address string
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

	// Initialise Raft consensus if enabled (transitional; will replace legacy path).
	if config.Raft != nil && config.Raft.Enable {
		raftCfg, err := config.buildRaftConsensusConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build raft config: %w", err)
		}
		raftConsensus, err := consensus.NewRaftConsensus(raftCfg, node, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create raft consensus: %w", err)
		}
		node.raftConsensus = raftConsensus
		logger.Info("Raft consensus initialised",
			"bind_addr", raftCfg.BindAddress,
			"data_dir", raftCfg.DataDir,
			"bootstrap", raftCfg.Bootstrap)
	}

	// Initialise Gossip for signature propagation if enabled
	if config.Gossip != nil && config.Gossip.Enable {
		// Create signature handler
		gossipDelegate := consensus.NewSignatureGossipDelegate(
			config.ValidatorID,
			node.onGossipSignatureReceived,
			logger,
		)

		// Create gossip manager
		gossipCfg := config.Gossip.ToConsensusConfig(config.ValidatorID)
		gossipManager, err := consensus.NewGossipManager(gossipCfg, gossipDelegate, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create gossip manager: %w", err)
		}

		node.gossipManager = gossipManager
		node.gossipDelegate = gossipDelegate

		logger.Info("Gossip initialized",
			"bind_addr", fmt.Sprintf("%s:%d", gossipCfg.BindAddress, gossipCfg.BindPort),
			"seeds", gossipCfg.Seeds)
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

// OnCheckpointCommitted satisfies consensus.RaftApplyHandler; full integration will replace legacy path.
func (n *Node) OnCheckpointCommitted(header *pb.CheckpointHeader) {
	if header == nil {
		return
	}
	if n.raftConsensus == nil {
		return
	}
	n.handleCommittedCheckpoint(header)
}

// OnExecutionReportCommitted satisfies consensus.RaftApplyHandler.
func (n *Node) OnExecutionReportCommitted(report *pb.ExecutionReport, reportKey string) {
	if report == nil || reportKey == "" {
		return
	}
	n.mu.Lock()
	if _, exists := n.pendingReports[reportKey]; !exists {
		n.pendingReports[reportKey] = report
		score := n.scoreReport(report)
		n.reportScores[reportKey] = score
	}
	n.mu.Unlock()
	n.logger.Debugf("Raft committed execution report intent=%s assignment=%s", report.IntentId, report.AssignmentId)
}

// onGossipSignatureReceived handles signatures received via gossip
func (n *Node) onGossipSignatureReceived(sig *pb.Signature, checkpointHash []byte) {
	if sig == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Basic validation
	if sig.SignerId == "" || len(sig.Der) == 0 {
		n.logger.Warn("Invalid gossip signature: missing fields",
			"signer", sig.SignerId)
		return
	}

	// Check if signer is in validator set
	if n.validatorSet == nil {
		n.logger.Warn("No validator set configured, ignoring gossip signature")
		return
	}

	var validatorInSet *types.Validator
	for _, v := range n.validatorSet.Validators {
		if v.ID == sig.SignerId {
			validatorInSet = &v
			break
		}
	}
	if validatorInSet == nil {
		n.logger.Warn("Signature from unknown validator, rejecting",
			"signer", sig.SignerId)
		return
	}

	// Verify checkpoint hash matches current checkpoint
	if n.currentCheckpoint == nil {
		n.logger.Warn("No current checkpoint, cannot verify gossip signature")
		return
	}

	currentHash := n.computeCheckpointHash(n.currentCheckpoint)
	if len(checkpointHash) > 0 && !bytesEqual(currentHash[:], checkpointHash) {
		n.logger.Warn("Checkpoint hash mismatch, rejecting signature",
			"signer", sig.SignerId,
			"expected_epoch", n.currentCheckpoint.Epoch)
		return
	}

	// Cryptographically verify the signature
	if err := n.verifySignature(sig); err != nil {
		n.logger.Warn("Signature verification failed",
			"signer", sig.SignerId,
			"error", err)
		return
	}

	// Skip if we already have this signature
	if n.signatures == nil {
		n.signatures = make(map[string]*pb.Signature)
	}
	if _, exists := n.signatures[sig.SignerId]; exists {
		n.logger.Debug("Duplicate signature ignored",
			"signer", sig.SignerId)
		return
	}

	// Add verified signature to map
	n.signatures[sig.SignerId] = sig

	n.logger.Debug("Received and verified signature via gossip",
		"signer", sig.SignerId,
		"total_sigs", len(n.signatures),
		"epoch", n.currentCheckpoint.Epoch)

	// Check if we've reached threshold (use weight-based if validators have weights)
	thresholdReached := false
	totalWeight := n.validatorSet.TotalWeight()
	if totalWeight > 0 {
		// Use weight-based threshold
		thresholdReached = n.validatorSet.CheckWeightedThreshold(n.signatures)
		if thresholdReached {
			n.logger.Info("Weighted signature threshold reached via gossip",
				"sigs", len(n.signatures),
				"required_weight", n.validatorSet.RequiredWeight(),
				"total_weight", totalWeight,
				"epoch", n.currentCheckpoint.Epoch)
		}
	} else {
		// Fall back to count-based threshold
		thresholdReached = n.validatorSet.CheckThreshold(len(n.signatures))
		if thresholdReached {
			n.logger.Info("Signature threshold reached via gossip",
				"sigs", len(n.signatures),
				"required", n.validatorSet.RequiredSignatures(),
				"epoch", n.currentCheckpoint.Epoch)
		}
	}

	if thresholdReached {
		// If we're the leader and using Raft, finalize checkpoint
		if n.raftConsensus != nil && n.raftConsensus.IsLeader() {
			go n.finalizeCheckpointAfterGossip()
		}
	}
}

// bytesEqual compares two byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// finalizeCheckpointAfterGossip finalizes checkpoint after gossip threshold is reached
func (n *Node) finalizeCheckpointAfterGossip() {
	n.mu.RLock()
	header := n.currentCheckpoint
	if header == nil {
		n.mu.RUnlock()
		return
	}
	headerCopy := proto.Clone(header).(*pb.CheckpointHeader)

	// Copy signatures and reports for async submission
	signaturesCopy := make(map[string]*pb.Signature, len(n.signatures))
	for k, v := range n.signatures {
		signaturesCopy[k] = proto.Clone(v).(*pb.Signature)
	}

	pendingCopy := make(map[string]*pb.ExecutionReport, len(n.pendingReports))
	for k, v := range n.pendingReports {
		pendingCopy[k] = proto.Clone(v).(*pb.ExecutionReport)
	}
	n.mu.RUnlock()

	// Submit to RootLayer (will fill header.Signatures with collected sigs)
	n.submitToRootLayerWithData(headerCopy, pendingCopy, signaturesCopy)
}

func (n *Node) handleCommittedCheckpoint(header *pb.CheckpointHeader) {
	headerCopy := proto.Clone(header).(*pb.CheckpointHeader)

	n.mu.Lock()
	if err := n.chain.AddCheckpoint(headerCopy); err != nil {
		n.logger.Warnf("Failed to append checkpoint to chain error=%v", err)
	}
	if err := n.saveCheckpoint(headerCopy); err != nil {
		n.logger.Warnf("Failed to persist checkpoint error=%v", err)
	}
	n.currentCheckpoint = headerCopy
	if headerCopy.Epoch >= n.currentEpoch {
		n.currentEpoch = headerCopy.Epoch + 1
	}
	n.lastCheckpointAt = time.Now()

	// Reset signatures for new checkpoint
	if n.gossipManager != nil {
		n.signatures = make(map[string]*pb.Signature)
	}

	// Sign the checkpoint locally
	sig, err := n.signCheckpoint(headerCopy)
	if err != nil {
		n.logger.Errorf("Failed to sign checkpoint error=%v", err)
		n.mu.Unlock()
		return
	}

	// Add own signature
	if n.signatures == nil {
		n.signatures = make(map[string]*pb.Signature)
	}
	n.signatures[n.id] = sig

	// Compute checkpoint hash for verification
	checkpointHash := n.computeCheckpointHash(headerCopy)

	n.mu.Unlock()

	// Broadcast signature via gossip (if enabled)
	if n.gossipManager != nil {
		if err := n.gossipManager.BroadcastSignature(sig, headerCopy.Epoch, checkpointHash[:]); err != nil {
			n.logger.Errorf("Failed to gossip signature error=%v", err)
		} else {
			n.logger.Debugf("Broadcasted signature via gossip epoch=%d validator=%s", headerCopy.Epoch, n.id)
		}

		// Check if we've already reached threshold (important for single-node mode)
		// In single-node mode, we already have the only signature needed
		n.mu.RLock()
		thresholdReached := false
		totalWeight := n.validatorSet.TotalWeight()
		if totalWeight > 0 {
			thresholdReached = n.validatorSet.CheckWeightedThreshold(n.signatures)
		} else {
			thresholdReached = n.validatorSet.CheckThreshold(len(n.signatures))
		}
		isLeader := n.raftConsensus != nil && n.raftConsensus.IsLeader()
		n.mu.RUnlock()

		// If threshold already reached and we're leader, submit immediately
		// (handles single-node case where gossip won't deliver own message back)
		if thresholdReached && isLeader {
			n.logger.Infof("Signature threshold already reached after local sign (single-node mode), finalizing epoch=%d", headerCopy.Epoch)
			go n.finalizeCheckpointAfterGossip()
		}

		// Otherwise wait for gossip to collect signatures (handled by onGossipSignatureReceived)
		return
	}

	// Legacy path: if no gossip, submit directly (for backward compatibility)
	n.mu.RLock()
	pendingCopy := make(map[string]*pb.ExecutionReport, len(n.pendingReports))
	for k, v := range n.pendingReports {
		pendingCopy[k] = proto.Clone(v).(*pb.ExecutionReport)
	}
	signaturesCopy := make(map[string]*pb.Signature, len(n.signatures))
	for k, v := range n.signatures {
		signaturesCopy[k] = proto.Clone(v).(*pb.Signature)
	}
	isLeader := n.raftConsensus != nil && n.raftConsensus.IsLeader()
	n.mu.RUnlock()

	if isLeader {
		n.logger.Infof("Raft leader submitting checkpoint to RootLayer epoch=%d", headerCopy.Epoch)
		n.submitToRootLayerWithData(headerCopy, pendingCopy, signaturesCopy)
	}
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

	// Raft consensus is required
	if n.raftConsensus == nil {
		return fmt.Errorf("Raft consensus is required - legacy NATS mode has been removed")
	}

	// Start metrics server if configured
	if n.config.MetricsPort > 0 {
		n.startMetricsServer()
	}

	// Register validator in registry and start heartbeat
	n.startValidatorRegistry()

	// Initialize leadership status
	if n.raftConsensus != nil {
		n.isLeader = n.raftConsensus.IsLeader()
		n.logger.Infof("Initial Raft leadership status is_leader=%t epoch=%d", n.isLeader, n.currentEpoch)
	} else {
		_, leader := n.leaderTracker.Leader(n.currentEpoch)
		n.isLeader = leader != nil && leader.ID == n.id
		n.logger.Infof("Initial leadership status is_leader=%t epoch=%d", n.isLeader, n.currentEpoch)
	}

	// Start consensus loop - this will handle checkpoint creation dynamically
	go n.consensusLoop()

	n.logger.Info("Validator node started successfully")
	return nil
}

// Stop gracefully stops the validator node
func (n *Node) Stop() error {
	n.logger.Info("Stopping validator node")

	n.cancel()

	if n.raftConsensus != nil {
		if err := n.raftConsensus.Shutdown(); err != nil {
			n.logger.Errorf("Failed to shutdown Raft consensus error=%v", err)
		}
	}

	// Shutdown gossip manager
	if n.gossipManager != nil {
		if err := n.gossipManager.Shutdown(); err != nil {
			n.logger.Errorf("Failed to shutdown gossip manager error=%v", err)
		}
	}

	if n.validatorHB != nil {
		n.validatorHB()
		n.validatorHB = nil
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

// consensusLoop runs the main consensus state machine
func (n *Node) consensusLoop() {
	n.logger.Infof("Consensus loop started validator_id=%s", n.id)
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
			if n.raftConsensus != nil {
				n.checkRaftConsensusState()
				n.checkRaftCheckpointTrigger()
			} else {
				n.checkConsensusState()
				n.checkCheckpointTrigger() // Check if leader should create checkpoint
			}
		}
	}
}

func (n *Node) checkRaftConsensusState() {
	if n.raftConsensus == nil {
		return
	}
	isLeader := n.raftConsensus.IsLeader()
	n.mu.Lock()
	if n.isLeader != isLeader {
		n.isLeader = isLeader
		if isLeader {
			n.lastCheckpointAt = time.Time{}
			n.logger.Infof("Became Raft leader epoch=%d", n.currentEpoch)
		} else {
			n.logger.Infof("Lost Raft leadership epoch=%d", n.currentEpoch)
		}
	}
	n.mu.Unlock()
}

func (n *Node) checkRaftCheckpointTrigger() {
	if n.raftConsensus == nil {
		return
	}

	n.mu.Lock()
	if !n.isLeader {
		n.mu.Unlock()
		return
	}
	pendingCount := len(n.pendingReports)
	if pendingCount == 0 {
		n.mu.Unlock()
		return
	}

	checkpointInterval := n.getCheckpointInterval()
	now := time.Now()
	shouldPropose := false
	if n.lastCheckpointAt.IsZero() || now.Sub(n.lastCheckpointAt) >= checkpointInterval {
		shouldPropose = true
		n.lastCheckpointAt = now
	}
	n.mu.Unlock()

	if shouldPropose {
		n.proposeCheckpointRaft()
	}
}

// checkConsensusState checks and updates consensus state
func (n *Node) checkConsensusState() {
	if n.raftConsensus != nil {
		n.checkRaftConsensusState()
		return
	}
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

		n.logger.Infof("checkConsensusState: StateCollecting epoch=%d has_threshold=%t sign_count=%d required=%d progress=%.2f timed_out=%t is_leader=%t node_signatures=%d", n.currentEpoch, hasThreshold, signCount, required, progress, isTimedOut, isLeader, len(n.signatures))

		// Check if leader has failed (timeout failover mechanism)
		shouldFailover := n.leaderTracker.ShouldFailover(n.currentEpoch, time.Now())

		// Fix: Only finalize once when StateCollecting AND threshold reached
		// After finalize, state becomes StateFinalized, won't enter this branch again
		if hasThreshold {
			// Fix: Record leader activity (reaching threshold means leader is working properly)
			n.leaderTracker.RecordActivity(n.currentEpoch, time.Now())
			n.logger.Infof("Threshold reached; calling finalizeCheckpoint() epoch=%d sign_count=%d", n.currentEpoch, signCount)
			n.mu.Unlock()
			n.logger.Infof("About to call finalizeCheckpoint() epoch=%d", n.currentEpoch)
			n.finalizeCheckpoint()
			n.logger.Infof("Returned from finalizeCheckpoint() epoch=%d", n.currentEpoch)
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
				n.logger.Warnf("Leader timeout detected, forcing epoch rotation epoch=%d failed_leader=%s", n.currentEpoch, failedLeaderID)

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

				n.logger.Infof("Epoch rotated due to leader timeout old_epoch=%d new_epoch=%d new_leader=%s i_am_new_leader=%t", oldEpoch, n.currentEpoch, newLeaderID, n.isLeader)
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
	if n.raftConsensus != nil {
		n.proposeCheckpointRaft()
		return
	}

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

	// Proposals are now handled by Raft - no separate broadcast needed
	// Raft will replicate the checkpoint to all followers

	// Fix: Record leader activity when proposing checkpoint
	n.leaderTracker.RecordActivity(header.Epoch, time.Now())

	n.logger.Infof("Proposed checkpoint epoch=%d fsm_state=%s", header.Epoch, n.fsm.GetState())
}

func (n *Node) proposeCheckpointRaft() {
	if n.raftConsensus == nil || !n.raftConsensus.IsLeader() {
		return
	}

	header := n.buildCheckpointHeader()

	sig, err := n.signCheckpoint(header)
	if err != nil {
		n.logger.Errorf("Failed to sign checkpoint error=%v", err)
		return
	}

	n.mu.Lock()
	if n.signatures == nil {
		n.signatures = make(map[string]*pb.Signature)
	}
	// Reset signatures for the new checkpoint
	n.signatures = map[string]*pb.Signature{
		n.id: sig,
	}
	bitmap := n.createSignersBitmapLocked()

	if header.Signatures == nil {
		header.Signatures = &pb.CheckpointSignatures{}
	}
	header.Signatures.EcdsaSignatures = [][]byte{sig.Der}
	header.Signatures.SignersBitmap = bitmap
	header.Signatures.SignatureCount = 1
	if validator := n.validatorSet.GetValidator(n.id); validator != nil {
		header.Signatures.TotalWeight = validator.Weight
	} else {
		header.Signatures.TotalWeight = 1
	}

	n.currentCheckpoint = header
	n.lastCheckpointAt = time.Now()
	n.mu.Unlock()

	if err := n.raftConsensus.ApplyCheckpoint(header, n.id); err != nil {
		n.logger.Errorf("Failed to replicate checkpoint via Raft error=%v", err)
		return
	}
	n.logger.Infof("Replicated checkpoint via Raft epoch=%d", header.Epoch)
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

	// Finalized checkpoints are now propagated via Raft - no broadcast needed
	// All nodes will be notified through Raft log replication

	// Submit to RootLayer if we are the leader
	if isLeader {
		n.submitToRootLayerWithData(headerCopy, pendingReportsCopy, signaturesCopy)
	}
}

// checkCheckpointTrigger checks if it's time for the leader to create a checkpoint
// ONLY creates checkpoints when there are pending execution reports (on-demand checkpointing)
// Fix Problem 8: Removed double unlock bug by avoiding manual unlock/lock inside defer scope
func (n *Node) checkCheckpointTrigger() {
	if n.raftConsensus != nil {
		n.checkRaftCheckpointTrigger()
		return
	}
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
		n.logger.Infof("Leader triggering first checkpoint with execution reports epoch=%d pending_reports=%d", n.currentEpoch, len(n.pendingReports))
		n.lastCheckpointAt = now
		shouldPropose = true
	} else if now.Sub(n.lastCheckpointAt) >= checkpointInterval {
		// Time for next checkpoint - we have reports
		n.logger.Infof("Leader triggering checkpoint with execution reports epoch=%d pending_reports=%d interval=%s", n.currentEpoch, len(n.pendingReports), checkpointInterval)
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

// broadcastProposal removed - proposals are now handled by Raft

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
	var removedKeys []string
	for reportID, report := range n.pendingReports {
		// Check if this report belongs to the Intent group
		key := fmt.Sprintf("%s:%s:%s", report.IntentId, report.AssignmentId, report.AgentId)
		if key == intentKey {
			delete(n.pendingReports, reportID)
			delete(n.reportScores, reportID)
			removedKeys = append(removedKeys, reportID)
			removed++
		}
	}

	if n.raftConsensus != nil && len(removedKeys) > 0 {
		n.raftConsensus.ClearPending(removedKeys)
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
	n.logger.Infof("Waiting %v for RootLayer state synchronization before submitting ValidationBundles epoch=%d (see node.go:1450 for rationale)",
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
			n.logger.Errorf("Failed to build ValidationBundle for Intent group %s epoch=%d", intentKey, header.Epoch)
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
		n.logger.Infof("Submitting %d ValidationBundles in batch batch_id=%s epoch=%d", len(bundles), batchID, header.Epoch)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := batchClient.SubmitValidationBundleBatch(ctx, bundles, batchID, true)
		if err != nil {
			// Batch submission failed - fall back to individual submission
			n.logger.Warnf("Batch submission failed, falling back to individual submission error=%v", err)
			return n.submitIndividualValidationBundles(header, bundles, intentKeys)
		}

		// Process batch response
		n.logger.Infof("Batch submission completed batch_id=%s total=%d success=%d failed=%d",
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
			n.logger.Infof("Successfully submitted ValidationBundle to RootLayer epoch=%d intent_id=%s intent_key=%s attempt=%d",
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
				n.logger.Warnf("ValidationBundle submission failed due to RootLayer sync delay, will retry in %v attempt=%d/%d error=%v",
					retryDelay, attempt, maxRetries, err)
				time.Sleep(retryDelay)
				continue
			} else {
				n.logger.Errorf("ValidationBundle submission failed after %d attempts due to RootLayer sync issue error=%v epoch=%d intent_id=%s",
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
		n.logger.Errorf("ValidationBundle construction failed: missing metadata intent_id=%s assignment_id=%s agent_id=%s",
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
				n.logger.Infof("Generated EIP-191 ValidationBundle signature validator=%s signature_len=%d",
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

	n.logger.Infof("ValidationBundle constructed for Intent group intent_id=%s assignment_id=%s agent_id=%s epoch=%d signatures=%d",
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
	n.logger.Warnf("DEPRECATED: buildValidationBundleWithData called - use submitToRootLayerWithData for multi-intent support epoch=%d", header.Epoch)

	// If there are no pending reports, this is an empty consensus checkpoint - skip it
	if len(pendingReports) == 0 {
		n.logger.Warnf("ValidationBundle construction skipped: no pending execution reports epoch=%d", header.Epoch)
		return nil
	}

	// Use the new grouping logic to properly handle multiple Intents
	groupedReports := n.groupReportsByIntent(pendingReports)

	// Warn if multiple Intent groups exist (backward compatibility issue)
	if len(groupedReports) > 1 {
		n.logger.Errorf("CRITICAL: Multiple Intent groups detected (%d) but buildValidationBundleWithData can only return ONE! Other Intents will be LOST! Use submitToRootLayerWithData instead! epoch=%d",
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
	if c.Raft != nil && c.Raft.Enable {
		if _, err := c.buildRaftConsensusConfig(); err != nil {
			return err
		}
	}
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
	if c.Raft == nil {
		c.Raft = &RaftConfig{}
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
	if c.Raft.DataDir == "" {
		if c.StoragePath != "" {
			c.Raft.DataDir = filepath.Join(c.StoragePath, "raft")
		} else {
			c.Raft.DataDir = "./data/raft"
		}
	}
	if c.Raft.BindAddress == "" {
		c.Raft.BindAddress = "127.0.0.1:7000"
	}
	if c.Raft.HeartbeatTimeout == 0 {
		c.Raft.HeartbeatTimeout = 1 * time.Second
	}
	if c.Raft.ElectionTimeout == 0 {
		c.Raft.ElectionTimeout = 1 * time.Second
	}
	if c.Raft.CommitTimeout == 0 {
		c.Raft.CommitTimeout = 50 * time.Millisecond
	}
	if c.Raft.MaxPool <= 0 {
		c.Raft.MaxPool = 3
	}

	if c.RegistryHeartbeatInterval <= 0 {
		c.RegistryHeartbeatInterval = 30 * time.Second
	}
}

func (c *Config) buildRaftConsensusConfig() (consensus.RaftConfig, error) {
	if c.Raft == nil || !c.Raft.Enable {
		return consensus.RaftConfig{}, fmt.Errorf("raft not enabled")
	}
	if c.Raft.DataDir == "" {
		return consensus.RaftConfig{}, fmt.Errorf("raft data_dir is required when raft is enabled")
	}
	if c.Raft.BindAddress == "" {
		return consensus.RaftConfig{}, fmt.Errorf("raft bind_address is required when raft is enabled")
	}
	peers := make([]consensus.RaftPeer, 0, len(c.Raft.Peers))
	for _, peer := range c.Raft.Peers {
		if peer.ID == "" || peer.Address == "" {
			return consensus.RaftConfig{}, fmt.Errorf("raft peer configuration requires both id and address")
		}
		peers = append(peers, consensus.RaftPeer{
			ID:      peer.ID,
			Address: peer.Address,
		})
	}
	return consensus.RaftConfig{
		NodeID:           c.ValidatorID,
		DataDir:          c.Raft.DataDir,
		BindAddress:      c.Raft.BindAddress,
		Bootstrap:        c.Raft.Bootstrap,
		Peers:            peers,
		HeartbeatTimeout: c.Raft.HeartbeatTimeout,
		ElectionTimeout:  c.Raft.ElectionTimeout,
		CommitTimeout:    c.Raft.CommitTimeout,
		MaxPool:          c.Raft.MaxPool,
	}, nil
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
