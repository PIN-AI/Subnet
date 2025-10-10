package blockchain

import "time"

// VerifierConfig controls cache and fallback behaviour for on-chain checks.
type VerifierConfig struct {
	CacheTTL       time.Duration
	CacheSize      int
	EnableFallback bool
}

// DefaultVerifierConfig provides sensible defaults for verifier behaviour.
func DefaultVerifierConfig() VerifierConfig {
	return VerifierConfig{
		CacheTTL:       5 * time.Minute,
		CacheSize:      1024,
		EnableFallback: true,
	}
}

func (c *VerifierConfig) normalize() {
	if c.CacheTTL <= 0 {
		c.CacheTTL = DefaultVerifierConfig().CacheTTL
	}
	if c.CacheSize <= 0 {
		c.CacheSize = DefaultVerifierConfig().CacheSize
	}
}
