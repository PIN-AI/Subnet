package consensus

import (
	"fmt"

	"subnet/internal/consensus/cometbft"
	raft "subnet/internal/consensus/raft"
	"subnet/internal/logging"
)

// NewConsensusEngine creates a consensus engine based on the configuration
func NewConsensusEngine(
	cfg *ConsensusConfig,
	handlers ConsensusHandlers,
	logger logging.Logger,
) (ConsensusEngine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid consensus config: %w", err)
	}

	switch cfg.Type {
	case ConsensusTypeRaftGossip:
		return newRaftGossipEngine(cfg, handlers, logger)
	case ConsensusTypeCometBFT:
		return newCometBFTEngine(cfg, handlers, logger)
	default:
		return nil, ErrUnsupportedConsensusType
	}
}

// newRaftGossipEngine creates a Raft+Gossip consensus engine
func newRaftGossipEngine(
	cfg *ConsensusConfig,
	handlers ConsensusHandlers,
	logger logging.Logger,
) (ConsensusEngine, error) {
	// Since RaftConfig and GossipConfig are type aliases, no conversion needed
	raftCfg := cfg.Raft
	gossipCfg := cfg.Gossip

	// Convert handlers
	rgHandlers := raft.ConsensusHandlers{
		OnCheckpointCommitted:      handlers.OnCheckpointCommitted,
		OnExecutionReportCommitted: handlers.OnExecutionReportCommitted,
		OnSignatureReceived:        handlers.OnSignatureReceived,
		OnCheckpointFinalized:      handlers.OnCheckpointFinalized,
		OnLeaderChanged:            handlers.OnLeaderChanged,
	}

	return raft.NewConsensus(raftCfg, gossipCfg, rgHandlers, logger)
}

// newCometBFTEngine creates a CometBFT consensus engine
func newCometBFTEngine(
	cfg *ConsensusConfig,
	handlers ConsensusHandlers,
	logger logging.Logger,
) (ConsensusEngine, error) {
	// Convert handlers to CometBFT format
	cometHandlers := cometbft.ConsensusHandlers{
		OnCheckpointCommitted:      handlers.OnCheckpointCommitted,
		OnExecutionReportCommitted: handlers.OnExecutionReportCommitted,
		OnCheckpointFinalized:      handlers.OnCheckpointFinalized,
		// Note: CometBFT doesn't use OnSignatureReceived and OnLeaderChanged
		// as signatures are automatically collected and proposer rotates
	}

	return cometbft.NewConsensus(cfg.CometBFT, cometHandlers, logger)
}
