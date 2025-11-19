package consensus

// This file re-exports types from raft for backward compatibility
// This allows existing code to continue using consensus.RaftConsensus, etc.
//
// Note: StateMachine, Chain, LeaderTracker, State are already in this package
// and don't need to be re-exported.

import (
	raft "subnet/internal/consensus/raft"
	"subnet/internal/logging"
)

// Re-export Raft types from raft package
type (
	RaftConsensus    = raft.RaftConsensus
	RaftApplyHandler = raft.RaftApplyHandler
	RaftFSM          = raft.RaftFSM
	RaftLogEntry     = raft.RaftLogEntry
)

// Re-export Gossip types from raft package
type (
	GossipManager           = raft.GossipManager
	SignatureGossipDelegate = raft.SignatureGossipDelegate
	SignatureHandler        = raft.SignatureHandler
)

// Re-export constructor functions from raft package

func NewRaftConsensus(cfg RaftConfig, handler RaftApplyHandler, logger logging.Logger) (*RaftConsensus, error) {
	return raft.NewRaftConsensus(cfg, handler, logger)
}

func NewSignatureGossipDelegate(nodeID string, handler SignatureHandler, logger logging.Logger) *SignatureGossipDelegate {
	return raft.NewSignatureGossipDelegate(nodeID, handler, logger)
}

func NewGossipManager(cfg GossipConfig, delegate *SignatureGossipDelegate, logger logging.Logger) (*GossipManager, error) {
	return raft.NewGossipManager(cfg, delegate, logger)
}
