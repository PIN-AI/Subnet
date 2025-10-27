package consensus

import (
	"context"

	pb "subnet/proto/subnet"
)

// ConsensusEngine defines the unified interface for consensus implementations.
// Current implementations:
// - RaftGossipConsensus: Uses Raft for log replication + Gossip for signature propagation (CFT)
//
// Planned future implementations:
// - CometBFTConsensus: Uses CometBFT for Byzantine Fault Tolerant consensus (BFT)
// - HotStuffConsensus: Uses HotStuff for high-performance BFT consensus
type ConsensusEngine interface {
	// Lifecycle management
	Start(ctx context.Context) error
	Stop() error
	IsReady() bool

	// Leadership
	IsLeader() bool
	GetLeaderID() string
	GetLeaderAddress() string

	// Checkpoint consensus
	ProposeCheckpoint(header *pb.CheckpointHeader) error
	GetLatestCheckpoint() *pb.CheckpointHeader

	// Signature handling
	// For Raft+Gossip: Broadcasts signature via Gossip and stores in StateMachine
	// For CometBFT: This is a no-op (signatures are collected automatically)
	BroadcastSignature(sig *pb.Signature, epoch uint64, checkpointHash []byte) error

	// GetSignatures returns collected signatures for an epoch
	// For Raft+Gossip: Returns signatures from StateMachine
	// For CometBFT: Returns signatures from block Commit
	GetSignatures(epoch uint64) []*pb.Signature

	// CheckThreshold checks if we have enough signatures
	// For Raft+Gossip: Checks against validator set threshold (3/4)
	// For CometBFT: Always returns true after block is committed (2/3+ guaranteed)
	CheckThreshold(epoch uint64) bool

	// Execution report handling
	ProposeExecutionReport(report *pb.ExecutionReport, reportKey string) error
	GetPendingReports() map[string]*pb.ExecutionReport
	ClearPendingReports(keys []string)

	// State queries
	GetCurrentEpoch() uint64
	GetValidatorSet() []string
}

// ConsensusHandlers defines callbacks for consensus events
type ConsensusHandlers struct {
	// Called when a checkpoint is committed by consensus
	OnCheckpointCommitted func(header *pb.CheckpointHeader)

	// Called when an execution report is committed
	OnExecutionReportCommitted func(report *pb.ExecutionReport, reportKey string)

	// Called when a signature is received from another validator
	OnSignatureReceived func(sig *pb.Signature)

	// Called when a checkpoint is finalized (reached threshold signatures)
	OnCheckpointFinalized func(header *pb.CheckpointHeader, signatures []*pb.Signature)

	// Called when leadership changes
	OnLeaderChanged func(leaderID string)
}

// ConsensusType represents the type of consensus engine
type ConsensusType string

const (
	ConsensusTypeRaftGossip ConsensusType = "raft-gossip"
	ConsensusTypeCometBFT   ConsensusType = "cometbft"
	// Future BFT consensus types (not yet implemented):
	// ConsensusTypeHotStuff   ConsensusType = "hotstuff"
)

// Config holds the unified consensus configuration
type ConsensusConfig struct {
	// Engine type: "raft-gossip", "cometbft", "hotstuff"
	Type ConsensusType

	// Node information
	NodeID      string
	ValidatorID string

	// Raft+Gossip configuration
	Raft   *RaftConfig
	Gossip *GossipConfig

	// CometBFT configuration
	CometBFT *CometBFTConfig

	// Future BFT configurations (not yet implemented):
	// HotStuff *HotStuffConfig
}

// Validate checks if the consensus configuration is valid
func (c *ConsensusConfig) Validate() error {
	switch c.Type {
	case ConsensusTypeRaftGossip:
		if c.Raft == nil {
			return ErrMissingRaftConfig
		}
		if c.Gossip == nil {
			return ErrMissingGossipConfig
		}
		return nil
	case ConsensusTypeCometBFT:
		if c.CometBFT == nil {
			return ErrMissingCometBFTConfig
		}
		return c.CometBFT.Validate()
	default:
		return ErrUnsupportedConsensusType
	}
}
