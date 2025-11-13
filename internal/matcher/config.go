package matcher

import (
	"fmt"
	"time"
	"subnet/internal/config"
)

// Config contains configuration for the matcher service
type Config struct {
	// Use unified config structures
	Timeouts *config.TimeoutConfig
	Network  *config.NetworkConfig
	Identity *config.IdentityConfig
	Limits   *config.LimitsConfig

	// Legacy fields for backward compatibility
	BiddingWindowSec int `mapstructure:"bidding_window_sec"`
	MaxBidsPerIntent int `mapstructure:"max_bids_per_intent"`

	// Matching strategy
	MatchingStrategy string `mapstructure:"matching_strategy"`

	// Lease configuration
	DefaultLeaseTTL time.Duration `mapstructure:"default_lease_ttl"`
	MinLeaseTTL     time.Duration `mapstructure:"min_lease_ttl"`
	MaxLeaseTTL     time.Duration `mapstructure:"max_lease_ttl"`

	// Matcher identity
	MatcherID string `mapstructure:"matcher_id"`
	PrivateKey string `mapstructure:"private_key"` // Private key for signing

	// RootLayer configuration
	RootLayerEndpoint string `mapstructure:"rootlayer_endpoint"`
	IntentPullInterval time.Duration `mapstructure:"intent_pull_interval"`

	// On-chain configuration for assignment submission
	ChainRPCURL       string `mapstructure:"chain_rpc_url"`        // RPC URL for blockchain
	ChainNetwork      string `mapstructure:"chain_network"`        // Network name (e.g., "base_sepolia")
	IntentManagerAddr string `mapstructure:"intent_manager_addr"`  // IntentManager contract address
	EnableChainSubmit bool   `mapstructure:"enable_chain_submit"`  // Enable on-chain assignment submission
}

// DefaultConfig returns default matcher configuration
func DefaultConfig() *Config {
	cfg := &Config{
		Timeouts: config.DefaultTimeoutConfig(),
		Network:  config.DefaultNetworkConfig(),
		Identity: config.DefaultIdentityConfig(),
		Limits:   config.DefaultLimitsConfig(),

		// Legacy defaults
		MatchingStrategy: "lowest_price", // Default to lowest price strategy
		DefaultLeaseTTL:  5 * time.Minute,
		MinLeaseTTL:      1 * time.Minute,
		MaxLeaseTTL:      30 * time.Minute,
	}

	// Apply from unified config
	cfg.applyDefaults()
	return cfg
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
	if c.BiddingWindowSec == 0 {
		c.BiddingWindowSec = c.Limits.BiddingWindowSec
	}
	if c.MaxBidsPerIntent == 0 {
		c.MaxBidsPerIntent = c.Limits.MaxBidsPerIntent
	}
	if c.MatcherID == "" {
		c.MatcherID = c.Identity.MatcherID
	}
	if c.IntentPullInterval == 0 {
		c.IntentPullInterval = c.Timeouts.IntentGenerateInterval
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check required IDs
	if c.Identity != nil && c.Identity.SubnetID == "" {
		return fmt.Errorf("identity.subnet_id is required but not configured")
	}
	if c.MatcherID == "" && (c.Identity == nil || c.Identity.MatcherID == "") {
		return fmt.Errorf("matcher_id is required but not configured")
	}

	// Validate private key format (required for signing)
	if c.PrivateKey != "" {
		if err := validatePrivateKey(c.PrivateKey); err != nil {
			return fmt.Errorf("invalid private key: %w", err)
		}
	}

	// Validate other required fields
	if c.BiddingWindowSec <= 0 {
		return fmt.Errorf("bidding_window_sec must be positive")
	}
	if c.MaxBidsPerIntent <= 0 {
		return fmt.Errorf("max_bids_per_intent must be positive")
	}

	// Validate timeouts if configured
	if c.Timeouts != nil {
		if c.Timeouts.TaskSendTimeout < time.Second {
			return fmt.Errorf("task_send_timeout too short: %v", c.Timeouts.TaskSendTimeout)
		}
		if c.Timeouts.TaskResponseTimeout < 5*time.Second {
			return fmt.Errorf("task_response_timeout too short: %v", c.Timeouts.TaskResponseTimeout)
		}
		if c.Timeouts.RootLayerSubmitTimeout < time.Second {
			return fmt.Errorf("rootlayer_submit_timeout too short: %v", c.Timeouts.RootLayerSubmitTimeout)
		}
	}

	return nil
}

// CreateStrategy creates a BidMatchingStrategy instance based on the configuration
// This allows strategies to be configured via config files
func (c *Config) CreateStrategy() (BidMatchingStrategy, error) {
	switch c.MatchingStrategy {
	case "lowest_price", "":
		// Default strategy: lowest price
		return NewLowestPriceBidStrategy(), nil
	default:
		return nil, fmt.Errorf("unknown matching strategy: %s (supported: lowest_price)", c.MatchingStrategy)
	}
}

// validatePrivateKey checks if the private key is in valid format
func validatePrivateKey(key string) error {
	// Remove 0x prefix if present
	if len(key) >= 2 && key[:2] == "0x" {
		key = key[2:]
	}

	// Check if it's a valid hex string
	if len(key) != 64 {
		return fmt.Errorf("private key must be 32 bytes (64 hex characters), got %d chars", len(key))
	}

	// Check if all characters are valid hex
	for _, c := range key {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("private key contains non-hex character: %c", c)
		}
	}

	return nil
}