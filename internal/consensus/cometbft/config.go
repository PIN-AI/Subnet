package cometbft

import (
	"errors"
	"time"
)

// Config holds configuration for CometBFT consensus engine
type Config struct {
	// Home directory for CometBFT data and config
	HomeDir string

	// Node configuration
	Moniker string // Node name
	ChainID string // Chain ID (usually subnet_id)

	// Network configuration
	P2PListenAddress string   // P2P listen address (e.g., "tcp://0.0.0.0:26656")
	RPCListenAddress string   // RPC listen address (e.g., "tcp://127.0.0.1:26657")
	Seeds            []string // Seed nodes (e.g., ["id@ip:port"])
	PersistentPeers  []string // Persistent peers (e.g., ["id@ip:port"])

	// Consensus timeouts
	TimeoutPropose   time.Duration // How long to wait for a proposal (default: 3s)
	TimeoutPrevote   time.Duration // How long to wait for prevote (default: 1s)
	TimeoutPrecommit time.Duration // How long to wait for precommit (default: 1s)
	TimeoutCommit    time.Duration // How long to wait before starting next block (default: 5s)

	// Block configuration
	CreateEmptyBlocks         bool          // Whether to create empty blocks (default: true)
	CreateEmptyBlocksInterval time.Duration // Interval for empty blocks (default: 0s)
	MaxBlockSizeBytes         int64         // Max block size in bytes (default: 22020096 = 21MB)

	// Mempool configuration
	MempoolSize      int  // Maximum number of transactions in mempool (default: 5000)
	MempoolRecheck   bool // Recheck transactions after commit (default: true)
	MempoolBroadcast bool // Broadcast transactions to peers (default: true)

	// State sync
	StateSyncEnable bool // Enable state sync (default: false)

	// Database backend
	DBBackend string // Database backend: "goleveldb", "cleveldb", "boltdb", "rocksdb" (default: "goleveldb")
	DBDir     string // Database directory (default: "$HOME/data")

	// Logging
	LogLevel  string // Log level: "debug", "info", "error" (default: "info")
	LogFormat string // Log format: "plain", "json" (default: "plain")

	// Genesis validators (for initial setup)
	// Format: map[validator_id]voting_power
	GenesisValidators map[string]int64
}

// DefaultConfig returns a default CometBFT configuration
func DefaultConfig() *Config {
	return &Config{
		HomeDir:                   "./data/cometbft",
		Moniker:                   "validator-node",
		P2PListenAddress:          "tcp://0.0.0.0:26656",
		RPCListenAddress:          "tcp://127.0.0.1:26657",
		Seeds:                     []string{},
		PersistentPeers:           []string{},
		TimeoutPropose:            3 * time.Second,
		TimeoutPrevote:            1 * time.Second,
		TimeoutPrecommit:          1 * time.Second,
		TimeoutCommit:             5 * time.Second,
		CreateEmptyBlocks:         true,
		CreateEmptyBlocksInterval: 0,
		MaxBlockSizeBytes:         22020096, // 21MB
		MempoolSize:               5000,
		MempoolRecheck:            true,
		MempoolBroadcast:          true,
		StateSyncEnable:           false,
		DBBackend:                 "goleveldb",
		LogLevel:                  "info",
		LogFormat:                 "plain",
		GenesisValidators:         make(map[string]int64),
	}
}

// Validate checks if the CometBFT configuration is valid
func (c *Config) Validate() error {
	if c.HomeDir == "" {
		return errors.New("cometbft: home directory is required")
	}
	if c.ChainID == "" {
		return errors.New("cometbft: chain ID is required")
	}
	if c.P2PListenAddress == "" {
		return errors.New("cometbft: P2P listen address is required")
	}
	if c.TimeoutPropose <= 0 {
		return errors.New("cometbft: timeout_propose must be positive")
	}
	if c.TimeoutPrevote <= 0 {
		return errors.New("cometbft: timeout_prevote must be positive")
	}
	if c.TimeoutPrecommit <= 0 {
		return errors.New("cometbft: timeout_precommit must be positive")
	}
	if c.TimeoutCommit <= 0 {
		return errors.New("cometbft: timeout_commit must be positive")
	}
	return nil
}
