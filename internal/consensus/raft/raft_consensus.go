package raft

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	pb "subnet/proto/subnet"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"google.golang.org/protobuf/proto"
	"subnet/internal/logging"
)

// RaftPeer describes a known peer in the Raft cluster.
type RaftPeer struct {
	ID      string
	Address string
}

// RaftConfig defines the parameters required to initialise the Raft node.
type RaftConfig struct {
	NodeID           string
	DataDir          string
	BindAddress      string
	Bootstrap        bool
	Peers            []RaftPeer
	HeartbeatTimeout time.Duration
	ElectionTimeout  time.Duration
	CommitTimeout    time.Duration
	MaxPool          int
}

// RaftConsensus wraps the hashicorp/raft state machine.
type RaftConsensus struct {
	raft   *raft.Raft
	fsm    *RaftFSM
	logger logging.Logger
}

// NewRaftConsensus initialises Raft with the provided configuration.
func NewRaftConsensus(cfg RaftConfig, handler RaftApplyHandler, logger logging.Logger) (*RaftConsensus, error) {
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("raft: node id required")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("raft: data directory required")
	}
	if cfg.BindAddress == "" {
		return nil, fmt.Errorf("raft: bind address required")
	}
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("raft: create data dir: %w", err)
	}

	fsm := NewRaftFSM(handler, logger)

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("raft: create log store: %w", err)
	}
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-stable.db"))
	if err != nil {
		return nil, fmt.Errorf("raft: create stable store: %w", err)
	}
	snapshotStore, err := raft.NewFileSnapshotStore(cfg.DataDir, 3, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("raft: create snapshot store: %w", err)
	}

	maxPool := cfg.MaxPool
	if maxPool <= 0 {
		maxPool = 3
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.BindAddress)
	if err != nil {
		return nil, fmt.Errorf("raft: resolve bind address: %w", err)
	}
	transport, err := raft.NewTCPTransport(cfg.BindAddress, addr, maxPool, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("raft: create transport: %w", err)
	}

	rConfig := raft.DefaultConfig()
	rConfig.LocalID = raft.ServerID(cfg.NodeID)
	if cfg.HeartbeatTimeout > 0 {
		rConfig.HeartbeatTimeout = cfg.HeartbeatTimeout
	}
	if cfg.ElectionTimeout > 0 {
		rConfig.ElectionTimeout = cfg.ElectionTimeout
	}
	if cfg.CommitTimeout > 0 {
		rConfig.CommitTimeout = cfg.CommitTimeout
	}

	rNode, err := raft.NewRaft(rConfig, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, fmt.Errorf("raft: init raft node: %w", err)
	}

	if cfg.Bootstrap && len(cfg.Peers) > 0 {
		servers := make([]raft.Server, 0, len(cfg.Peers))
		for _, peer := range cfg.Peers {
			servers = append(servers, raft.Server{
				ID:      raft.ServerID(peer.ID),
				Address: raft.ServerAddress(peer.Address),
			})
		}
		if err := rNode.BootstrapCluster(raft.Configuration{Servers: servers}).Error(); err != nil && err != raft.ErrCantBootstrap {
			return nil, fmt.Errorf("raft: bootstrap cluster: %w", err)
		}
	}

	return &RaftConsensus{
		raft:   rNode,
		fsm:    fsm,
		logger: logger,
	}, nil
}

// IsLeader returns true if this node currently acts as leader.
func (c *RaftConsensus) IsLeader() bool {
	return c.raft.State() == raft.Leader
}

// ApplyCheckpoint replicates the checkpoint header through Raft.
func (c *RaftConsensus) ApplyCheckpoint(header *pb.CheckpointHeader, proposerID string) error {
	raw, err := proto.Marshal(header)
	if err != nil {
		return fmt.Errorf("raft: marshal checkpoint: %w", err)
	}
	entry := NewCheckpointEntry(raw, proposerID)
	return c.applyEntry(entry)
}

// ApplyExecutionReport replicates an execution report.
func (c *RaftConsensus) ApplyExecutionReport(report *pb.ExecutionReport, proposerID string) error {
	raw, err := proto.Marshal(report)
	if err != nil {
		return fmt.Errorf("raft: marshal execution report: %w", err)
	}
	entry := NewExecutionReportEntry(raw, proposerID)
	return c.applyEntry(entry)
}

func (c *RaftConsensus) applyEntry(entry *RaftLogEntry) error {
	payload, err := entry.Marshal()
	if err != nil {
		return err
	}
	future := c.raft.Apply(payload, 10*time.Second)
	return future.Error()
}

// LatestCheckpoint returns the latest committed header.
func (c *RaftConsensus) LatestCheckpoint() *pb.CheckpointHeader {
	return c.fsm.LatestCheckpoint()
}

// PendingReports exposes committed pending reports.
func (c *RaftConsensus) PendingReports() map[string]*pb.ExecutionReport {
	return c.fsm.PendingReports()
}

// ClearPending removes keys from the pending report map.
func (c *RaftConsensus) ClearPending(keys []string) {
	c.fsm.ClearPending(keys)
}

// LeaderAddress returns the address of the current leader if known.
func (c *RaftConsensus) LeaderAddress() string {
	return string(c.raft.Leader())
}

// Shutdown gracefully stops the Raft node.
func (c *RaftConsensus) Shutdown() error {
	future := c.raft.Shutdown()
	return future.Error()
}
