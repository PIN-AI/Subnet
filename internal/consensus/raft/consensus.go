package raft

import (
	"context"
	"fmt"
	"sync"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"
)

// ConsensusHandlers defines callbacks for consensus events
// Re-exported from parent package for convenience
type ConsensusHandlers struct {
	OnCheckpointCommitted      func(header *pb.CheckpointHeader)
	OnExecutionReportCommitted func(report *pb.ExecutionReport, reportKey string)
	OnSignatureReceived        func(sig *pb.Signature)
	OnCheckpointFinalized      func(header *pb.CheckpointHeader, signatures []*pb.Signature)
	OnLeaderChanged            func(leaderID string)
}

// Consensus implements the ConsensusEngine interface using Raft+Gossip
type Consensus struct {
	mu sync.RWMutex

	logger   logging.Logger
	handlers ConsensusHandlers

	// Core components
	raft   *RaftConsensus
	gossip *GossipManager

	// Runtime state
	ctx    context.Context
	cancel context.CancelFunc
	ready  bool

	// Node information
	nodeID      string
	validatorID string

	// Signature collection
	signatures map[uint64]map[string]*pb.Signature // epoch -> validator_id -> signature
}

// NewConsensus creates a new Raft+Gossip consensus engine
func NewConsensus(
	raftCfg *RaftConfig,
	gossipCfg *GossipConfig,
	handlers ConsensusHandlers,
	logger logging.Logger,
) (*Consensus, error) {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	if raftCfg == nil {
		return nil, fmt.Errorf("raft config is required")
	}
	if gossipCfg == nil {
		return nil, fmt.Errorf("gossip config is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Consensus{
		logger:      logger,
		handlers:    handlers,
		ctx:         ctx,
		cancel:      cancel,
		nodeID:      raftCfg.NodeID,
		validatorID: gossipCfg.NodeName,
		signatures:  make(map[uint64]map[string]*pb.Signature),
	}

	// Create Raft consensus
	raftHandler := &raftHandler{consensus: c}
	raft, err := NewRaftConsensus(*raftCfg, raftHandler, logger)
	if err != nil {
		return nil, fmt.Errorf("create raft consensus: %w", err)
	}
	c.raft = raft

	// Create Gossip delegate with signature handler adapter
	signatureHandler := func(sig *pb.Signature, checkpointHash []byte) {
		// Extract epoch from current checkpoint (since pb.Signature doesn't have epoch)
		// This is a limitation - we need to track epoch separately
		// For now, use the latest checkpoint's epoch
		header := c.GetLatestCheckpoint()
		epoch := uint64(0)
		if header != nil {
			epoch = header.Epoch
		}
		c.handleSignature(sig, epoch)
	}

	gossipDelegate := NewSignatureGossipDelegate(gossipCfg.NodeName, signatureHandler, logger)

	// Create Gossip manager
	gossip, err := NewGossipManager(*gossipCfg, gossipDelegate, logger)
	if err != nil {
		return nil, fmt.Errorf("create gossip manager: %w", err)
	}
	c.gossip = gossip

	return c, nil
}

// Start initializes and starts the consensus engine
func (c *Consensus) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ready {
		return nil
	}

	c.logger.Info("Starting Raft+Gossip consensus engine", "node_id", c.nodeID)

	// Gossip is already started in NewGossipManager
	// Raft is already started in NewRaftConsensus
	c.ready = true
	c.logger.Info("Raft+Gossip consensus engine started")

	return nil
}

// Stop gracefully stops the consensus engine
func (c *Consensus) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.ready {
		return nil
	}

	c.logger.Info("Stopping Raft+Gossip consensus engine")
	c.cancel()

	// Stop Gossip
	if c.gossip != nil {
		if err := c.gossip.Shutdown(); err != nil {
			c.logger.Error("Failed to stop gossip", "error", err)
		}
	}

	// Stop Raft
	if c.raft != nil {
		if err := c.raft.Shutdown(); err != nil {
			c.logger.Error("Failed to stop raft", "error", err)
		}
	}

	c.ready = false
	return nil
}

// IsReady returns whether the consensus engine is ready
func (c *Consensus) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// IsLeader returns whether this node is the current Raft leader
func (c *Consensus) IsLeader() bool {
	if c.raft == nil {
		return false
	}
	return c.raft.IsLeader()
}

// GetLeaderID returns the current leader's node ID
func (c *Consensus) GetLeaderID() string {
	if c.IsLeader() {
		return c.nodeID
	}
	return ""
}

// GetLeaderAddress returns the current leader's address
func (c *Consensus) GetLeaderAddress() string {
	if c.raft == nil {
		return ""
	}
	return c.raft.LeaderAddress()
}

// ProposeCheckpoint submits a checkpoint proposal through Raft
func (c *Consensus) ProposeCheckpoint(header *pb.CheckpointHeader) error {
	if !c.IsReady() {
		return fmt.Errorf("consensus engine not ready")
	}

	if c.raft == nil {
		return fmt.Errorf("raft not initialized")
	}

	return c.raft.ApplyCheckpoint(header, c.validatorID)
}

// GetLatestCheckpoint returns the latest committed checkpoint
func (c *Consensus) GetLatestCheckpoint() *pb.CheckpointHeader {
	if c.raft == nil {
		return nil
	}
	return c.raft.LatestCheckpoint()
}

// BroadcastSignature broadcasts a validator signature via Gossip
func (c *Consensus) BroadcastSignature(sig *pb.Signature, epoch uint64, checkpointHash []byte) error {
	if !c.IsReady() {
		return fmt.Errorf("consensus engine not ready")
	}

	if c.gossip == nil {
		return fmt.Errorf("gossip not initialized")
	}

	// Store locally first (index by epoch + validator ID)
	c.storeSignature(sig, epoch)

	// Broadcast via gossip
	return c.gossip.BroadcastSignature(sig, epoch, checkpointHash)
}

// GetSignatures returns all signatures for a given epoch
func (c *Consensus) GetSignatures(epoch uint64) []*pb.Signature {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sigs := c.signatures[epoch]
	result := make([]*pb.Signature, 0, len(sigs))
	for _, sig := range sigs {
		result = append(result, sig)
	}
	return result
}

// CheckThreshold checks if we have enough signatures for the given epoch
func (c *Consensus) CheckThreshold(epoch uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sigs := c.signatures[epoch]
	if sigs == nil {
		return false
	}

	// For Raft+Gossip, we need to check against validator set
	// This is a simplified check - in production you'd check weighted voting
	signCount := len(sigs)

	// Assuming 3/4 threshold (you should get this from validatorSet)
	// TODO: Get actual validator count and threshold from config
	requiredSignatures := 3 // Placeholder - should be (totalValidators * 3 + 3) / 4

	return signCount >= requiredSignatures
}

// ProposeExecutionReport submits an execution report through Raft
func (c *Consensus) ProposeExecutionReport(report *pb.ExecutionReport, reportKey string) error {
	if !c.IsReady() {
		return fmt.Errorf("consensus engine not ready")
	}

	if c.raft == nil {
		return fmt.Errorf("raft not initialized")
	}

	return c.raft.ApplyExecutionReport(report, c.validatorID)
}

// GetPendingReports returns all pending execution reports
func (c *Consensus) GetPendingReports() map[string]*pb.ExecutionReport {
	if c.raft == nil {
		return nil
	}
	return c.raft.PendingReports()
}

// ClearPendingReports removes reports from pending state
func (c *Consensus) ClearPendingReports(keys []string) {
	if c.raft != nil {
		c.raft.ClearPending(keys)
	}
}

// GetCurrentEpoch returns the current epoch
func (c *Consensus) GetCurrentEpoch() uint64 {
	header := c.GetLatestCheckpoint()
	if header == nil {
		return 0
	}
	return header.Epoch
}

// GetValidatorSet returns the current validator set
func (c *Consensus) GetValidatorSet() []string {
	// For Raft+Gossip, return gossip members
	if c.gossip == nil {
		return nil
	}

	members := c.gossip.Members()
	validators := make([]string, len(members))
	for i, member := range members {
		validators[i] = member.Name
	}
	return validators
}

// handleSignature is called when a signature is received via Gossip
func (c *Consensus) handleSignature(sig *pb.Signature, epoch uint64) {
	c.storeSignature(sig, epoch)

	// Notify handler
	if c.handlers.OnSignatureReceived != nil {
		c.handlers.OnSignatureReceived(sig)
	}
}

// storeSignature stores a signature in local state
func (c *Consensus) storeSignature(sig *pb.Signature, epoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.signatures[epoch] == nil {
		c.signatures[epoch] = make(map[string]*pb.Signature)
	}
	// Use SignerId from the Signature protobuf
	c.signatures[epoch][sig.SignerId] = sig
}

// raftHandler implements RaftApplyHandler to receive Raft callbacks
type raftHandler struct {
	consensus *Consensus
}

func (h *raftHandler) OnCheckpointCommitted(header *pb.CheckpointHeader) {
	if h.consensus.handlers.OnCheckpointCommitted != nil {
		h.consensus.handlers.OnCheckpointCommitted(header)
	}
}

func (h *raftHandler) OnExecutionReportCommitted(report *pb.ExecutionReport, reportKey string) {
	if h.consensus.handlers.OnExecutionReportCommitted != nil {
		h.consensus.handlers.OnExecutionReportCommitted(report, reportKey)
	}
}
