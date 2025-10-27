package consensus

// This file defines type aliases for configuration types
// The actual implementations are in the raft and cometbft packages

import (
	"subnet/internal/consensus/cometbft"
	raft "subnet/internal/consensus/raft"
)

// Re-export configuration types from raft package
type (
	RaftConfig   = raft.RaftConfig
	RaftPeer     = raft.RaftPeer
	GossipConfig = raft.GossipConfig
)

// Re-export configuration types from cometbft package
type (
	CometBFTConfig = cometbft.Config
)
