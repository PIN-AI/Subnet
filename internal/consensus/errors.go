package consensus

import "errors"

var (
	// Configuration errors
	ErrMissingRaftConfig        = errors.New("raft configuration is required for raft-gossip engine")
	ErrMissingGossipConfig      = errors.New("gossip configuration is required for raft-gossip engine")
	ErrMissingCometBFTConfig    = errors.New("cometbft configuration is required for cometbft engine")
	ErrUnsupportedConsensusType = errors.New("unsupported consensus engine type")

	// Runtime errors
	ErrNotLeader               = errors.New("not the leader")
	ErrEngineNotReady          = errors.New("consensus engine not ready")
	ErrCheckpointAlreadyExists = errors.New("checkpoint already exists")
	ErrInvalidSignature        = errors.New("invalid signature")
	ErrInsufficientSignatures  = errors.New("insufficient signatures")
)
