package config

import (
	"fmt"
	"strings"
	"time"
)

// BlockchainConfig holds on-chain verification settings.
type BlockchainConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	RPCURL          string `mapstructure:"rpc_url"`
	SubnetContract  string `mapstructure:"subnet_contract"`
	CacheTTLRaw     string `mapstructure:"cache_ttl"`
	CacheSize       int    `mapstructure:"cache_size"`
	EnableFallback  bool   `mapstructure:"enable_fallback"`
	AllowUnverified bool   `mapstructure:"allow_unverified"`

	CacheTTL time.Duration `mapstructure:"-"`
}

// DefaultBlockchainConfig returns default blockchain configuration.
func DefaultBlockchainConfig() *BlockchainConfig {
	return &BlockchainConfig{
		Enabled:         false,
		CacheTTLRaw:     "5m",
		CacheSize:       1024,
		EnableFallback:  true,
		AllowUnverified: true,
	}
}

// Normalize applies defaults and parses durations.
func (c *BlockchainConfig) Normalize() error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(c.CacheTTLRaw) == "" {
		c.CacheTTLRaw = "5m"
	}
	ttl, err := time.ParseDuration(c.CacheTTLRaw)
	if err != nil {
		return fmt.Errorf("invalid blockchain.cache_ttl: %w", err)
	}
	if ttl <= 0 {
		return fmt.Errorf("blockchain.cache_ttl must be positive")
	}
	c.CacheTTL = ttl
	if c.CacheSize <= 0 {
		c.CacheSize = 1024
	}
	return nil
}

// SubnetAddress returns the canonical address string.
func (c *BlockchainConfig) SubnetAddress() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.SubnetContract)
}
