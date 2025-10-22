package validator

import (
	"fmt"
	"time"

	"subnet/internal/consensus"
)

// GossipConfig configures the gossip layer for signature propagation
type GossipConfig struct {
	// Enable gossip (if false, falls back to NATS if available)
	Enable bool `mapstructure:"enable"`

	// BindAddress is the address to bind the gossip listener (e.g., "0.0.0.0")
	BindAddress string `mapstructure:"bind_address"`

	// BindPort is the port for gossip communication
	BindPort int `mapstructure:"bind_port"`

	// AdvertiseAddress is the address to advertise to other nodes
	// If empty, uses BindAddress
	AdvertiseAddress string `mapstructure:"advertise_address"`

	// AdvertisePort is the port to advertise to other nodes
	// If 0, uses BindPort
	AdvertisePort int `mapstructure:"advertise_port"`

	// Seeds are the initial gossip nodes to connect to
	Seeds []string `mapstructure:"seeds"`

	// GossipInterval is how often to gossip (default: 200ms)
	GossipInterval time.Duration `mapstructure:"gossip_interval"`

	// ProbeInterval is how often to probe nodes for health (default: 1s)
	ProbeInterval time.Duration `mapstructure:"probe_interval"`
}

// Validate validates the gossip configuration
func (c *GossipConfig) Validate() error {
	if !c.Enable {
		return nil // Disabled, no validation needed
	}

	if c.BindAddress == "" {
		return fmt.Errorf("gossip bind_address is required when gossip is enabled")
	}

	if c.BindPort <= 0 || c.BindPort > 65535 {
		return fmt.Errorf("gossip bind_port must be between 1 and 65535, got %d", c.BindPort)
	}

	if c.AdvertisePort > 65535 {
		return fmt.Errorf("gossip advertise_port must be between 0 and 65535, got %d", c.AdvertisePort)
	}

	// Seeds can be empty for single-node or bootstrap scenarios
	// But warn if empty in multi-node setup
	if len(c.Seeds) == 0 {
		// This is okay for bootstrap node
	}

	return nil
}

// ApplyDefaults applies default values to the configuration
func (c *GossipConfig) ApplyDefaults() {
	if c.BindAddress == "" {
		c.BindAddress = "0.0.0.0"
	}

	if c.BindPort == 0 {
		c.BindPort = 7946 // Default memberlist port
	}

	if c.AdvertiseAddress == "" {
		// Will use BindAddress if not specified
		c.AdvertiseAddress = ""
	}

	if c.AdvertisePort == 0 {
		// Will use BindPort if not specified
		c.AdvertisePort = 0
	}

	if c.GossipInterval == 0 {
		c.GossipInterval = 200 * time.Millisecond
	}

	if c.ProbeInterval == 0 {
		c.ProbeInterval = 1 * time.Second
	}
}

// ToConsensusConfig converts to consensus.GossipConfig
func (c *GossipConfig) ToConsensusConfig(nodeID string) consensus.GossipConfig {
	return consensus.GossipConfig{
		NodeName:       nodeID,
		BindAddress:    c.BindAddress,
		BindPort:       c.BindPort,
		AdvertiseAddr:  c.AdvertiseAddress,
		AdvertisePort:  c.AdvertisePort,
		Seeds:          c.Seeds,
		GossipInterval: c.GossipInterval,
		ProbeInterval:  c.ProbeInterval,
	}
}
