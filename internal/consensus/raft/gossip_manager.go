package raft

import (
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	pb "subnet/proto/subnet"
	"subnet/internal/logging"
)

// GossipConfig configures the gossip layer
type GossipConfig struct {
	NodeName       string
	BindAddress    string
	BindPort       int
	AdvertiseAddr  string
	AdvertisePort  int
	Seeds          []string
	GossipInterval time.Duration
	ProbeInterval  time.Duration
}

// GossipManager manages the memberlist instance and gossip propagation
type GossipManager struct {
	config       GossipConfig
	memberlist   *memberlist.Memberlist
	delegate     *SignatureGossipDelegate
	logger       logging.Logger
	shutdownOnce sync.Once
	shutdown     bool
	mu           sync.RWMutex
}

// NewGossipManager creates a new gossip manager
func NewGossipManager(cfg GossipConfig, delegate *SignatureGossipDelegate, logger logging.Logger) (*GossipManager, error) {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	// Create memberlist config
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Name = cfg.NodeName
	mlConfig.BindAddr = cfg.BindAddress
	mlConfig.BindPort = cfg.BindPort

	if cfg.AdvertiseAddr != "" {
		mlConfig.AdvertiseAddr = cfg.AdvertiseAddr
	}
	if cfg.AdvertisePort > 0 {
		mlConfig.AdvertisePort = cfg.AdvertisePort
	}

	// Set gossip intervals
	if cfg.GossipInterval > 0 {
		mlConfig.GossipInterval = cfg.GossipInterval
	} else {
		mlConfig.GossipInterval = 200 * time.Millisecond // Default fast gossip
	}

	if cfg.ProbeInterval > 0 {
		mlConfig.ProbeInterval = cfg.ProbeInterval
	} else {
		mlConfig.ProbeInterval = 1 * time.Second
	}

	// Set delegate
	mlConfig.Delegate = delegate
	mlConfig.Events = delegate // delegate implements EventDelegate

	// Disable memberlist logging (we use our own logger)
	mlConfig.LogOutput = nil

	// Create memberlist
	ml, err := memberlist.Create(mlConfig)
	if err != nil {
		return nil, fmt.Errorf("create memberlist: %w", err)
	}

	logger.Info("Created gossip memberlist",
		"node", cfg.NodeName,
		"bind_addr", fmt.Sprintf("%s:%d", cfg.BindAddress, cfg.BindPort))

	manager := &GossipManager{
		config:     cfg,
		memberlist: ml,
		delegate:   delegate,
		logger:     logger,
	}

	// Join seed nodes if provided
	if len(cfg.Seeds) > 0 {
		if err := manager.JoinSeeds(cfg.Seeds); err != nil {
			logger.Warn("Failed to join some gossip seeds",
				"error", err,
				"seeds", cfg.Seeds)
			// Don't fail - node can join later through other nodes
		}
	}

	return manager, nil
}

// JoinSeeds attempts to join the provided seed nodes
func (g *GossipManager) JoinSeeds(seeds []string) error {
	if len(seeds) == 0 {
		return nil
	}

	g.logger.Info("Joining gossip seeds",
		"seeds", seeds)

	numJoined, err := g.memberlist.Join(seeds)
	if err != nil {
		return fmt.Errorf("join seeds: %w", err)
	}

	g.logger.Info("Joined gossip cluster",
		"num_joined", numJoined,
		"total_seeds", len(seeds))

	return nil
}

// Members returns the current list of cluster members
func (g *GossipManager) Members() []*memberlist.Node {
	return g.memberlist.Members()
}

// NumMembers returns the number of cluster members
func (g *GossipManager) NumMembers() int {
	return g.memberlist.NumMembers()
}

// BroadcastSignature broadcasts a signature via gossip
func (g *GossipManager) BroadcastSignature(sig *pb.Signature, epoch uint64, checkpointHash []byte) error {
	return g.delegate.BroadcastSignature(sig, epoch, checkpointHash)
}

// BroadcastValidationBundleSignature broadcasts a ValidationBundle signature via gossip
func (g *GossipManager) BroadcastValidationBundleSignature(vbSig *pb.ValidationBundleSignature) error {
	return g.delegate.BroadcastValidationBundleSignature(vbSig)
}

// SetValidationBundleSignatureHandler sets the handler for ValidationBundle signatures
func (g *GossipManager) SetValidationBundleSignatureHandler(handler ValidationBundleSignatureHandler) {
	g.delegate.SetValidationBundleSignatureHandler(handler)
}

// Shutdown gracefully shuts down the gossip layer
func (g *GossipManager) Shutdown() error {
	var shutdownErr error

	g.shutdownOnce.Do(func() {
		g.logger.Info("Shutting down gossip manager")

		g.mu.Lock()
		g.shutdown = true
		g.mu.Unlock()

		// Try to leave gracefully first
		if g.memberlist != nil {
			if err := g.memberlist.Leave(5 * time.Second); err != nil {
				g.logger.Warn("Failed to leave memberlist gracefully", "error", err)
				// Continue with shutdown even if leave fails
			} else {
				g.logger.Info("Node left gossip cluster", "addr", g.config.BindAddress+":"+fmt.Sprint(g.config.BindPort), "node", g.config.NodeName)
			}

			// Now shutdown the memberlist
			if err := g.memberlist.Shutdown(); err != nil {
				shutdownErr = fmt.Errorf("shutdown memberlist: %w", err)
				g.logger.Errorf("Failed to shutdown memberlist error=%v", err)
			}
		}

		g.logger.Info("Gossip manager shut down successfully")
	})

	return shutdownErr
}

// LocalNode returns the local node information
func (g *GossipManager) LocalNode() *memberlist.Node {
	return g.memberlist.LocalNode()
}
