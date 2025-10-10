package matcher

import (
	"testing"

	"subnet/internal/config"
	"subnet/internal/logging"
)

// GenerateTestPrivateKey generates a test private key for testing purposes
func GenerateTestPrivateKey() (string, error) {
	// Use a fixed test private key for reproducible testing
	// This is a well-known test key - NEVER use in production!
	// 32 bytes hex = 64 characters
	return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", nil
}

// NewTestConfig creates a config with a test private key
func NewTestConfig(matcherID string, biddingWindowSec int) *Config {
	// Generate a test private key
	privKey, _ := GenerateTestPrivateKey()

	cfg := DefaultConfig()
	cfg.MatcherID = matcherID
	cfg.PrivateKey = privKey
	cfg.BiddingWindowSec = biddingWindowSec
	cfg.MaxBidsPerIntent = 10

	if cfg.Identity == nil {
		cfg.Identity = config.DefaultIdentityConfig()
	}
	cfg.Identity.SubnetID = "test-subnet"

	if cfg.Limits == nil {
		cfg.Limits = config.DefaultLimitsConfig()
	}
	cfg.Limits.BiddingWindowSec = biddingWindowSec
	cfg.Limits.MaxBidsPerIntent = cfg.MaxBidsPerIntent

	return cfg
}

// NewTestServer creates a server configured for testing
func NewTestServer(t *testing.T, matcherID string, logger logging.Logger) (*Server, error) {
	cfg := NewTestConfig(matcherID, 3) // 3 seconds bidding window for tests
	return NewServer(cfg, logger)
}

// TestPrivateKey returns a fixed test private key for deterministic testing
// This is NOT secure and should ONLY be used in tests
func TestPrivateKey() string {
	// This is a well-known test key - NEVER use in production
	return "0000000000000000000000000000000000000000000000000000000000000001"
}
