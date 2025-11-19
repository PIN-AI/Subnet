package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	pb "subnet/proto/subnet"

	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"
	"subnet/internal/logging"
)

// RaftApplyHandler receives callbacks when entries are committed.
type RaftApplyHandler interface {
	OnCheckpointCommitted(header *pb.CheckpointHeader)
	OnExecutionReportCommitted(report *pb.ExecutionReport, reportKey string)
}

// RaftFSM implements raft.FSM and stores checkpoint/report snapshots in memory.
type RaftFSM struct {
	mu sync.RWMutex

	logger  logging.Logger
	handler RaftApplyHandler

	checkpoints    map[uint64]*pb.CheckpointHeader
	pendingReports map[string]*pb.ExecutionReport
	currentEpoch   uint64
}

// NewRaftFSM constructs an FSM with the given handler.
func NewRaftFSM(handler RaftApplyHandler, logger logging.Logger) *RaftFSM {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}
	return &RaftFSM{
		logger:         logger,
		handler:        handler,
		checkpoints:    make(map[uint64]*pb.CheckpointHeader),
		pendingReports: make(map[string]*pb.ExecutionReport),
	}
}

// Apply is invoked once a Raft log entry is committed.
func (f *RaftFSM) Apply(logEntry *raft.Log) interface{} {
	entry, err := UnmarshalRaftLogEntry(logEntry.Data)
	if err != nil {
		f.logger.Error("raft fsm: failed to decode log entry", "error", err)
		return err
	}

	switch entry.Type {
	case RaftEntryCheckpoint:
		if err := f.applyCheckpoint(entry.Data); err != nil {
			f.logger.Error("raft fsm: failed to apply checkpoint", "error", err)
			return err
		}
	case RaftEntryExecutionReport:
		if err := f.applyExecutionReport(entry.Data); err != nil {
			f.logger.Error("raft fsm: failed to apply execution report", "error", err)
			return err
		}
	default:
		err = fmt.Errorf("raft fsm: unknown log entry type %d", entry.Type)
		f.logger.Error("raft fsm: unknown entry type", "type", entry.Type)
		return err
	}
	return nil
}

func (f *RaftFSM) applyCheckpoint(raw []byte) error {
	header := &pb.CheckpointHeader{}
	if err := proto.Unmarshal(raw, header); err != nil {
		return fmt.Errorf("decode checkpoint: %w", err)
	}

	f.mu.Lock()
	f.checkpoints[header.Epoch] = header
	f.currentEpoch = header.Epoch
	f.mu.Unlock()

	if f.handler != nil {
		go f.handler.OnCheckpointCommitted(proto.Clone(header).(*pb.CheckpointHeader))
	}
	return nil
}

func (f *RaftFSM) applyExecutionReport(raw []byte) error {
	report := &pb.ExecutionReport{}
	if err := proto.Unmarshal(raw, report); err != nil {
		return fmt.Errorf("decode execution report: %w", err)
	}
	key := makeReportKey(report)

	f.mu.Lock()
	f.pendingReports[key] = report
	f.mu.Unlock()

	if f.handler != nil {
		go f.handler.OnExecutionReportCommitted(proto.Clone(report).(*pb.ExecutionReport), key)
	}
	return nil
}

// Snapshot returns an immutable view for Raft snapshotting.
func (f *RaftFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	payload := raftSnapshot{
		CurrentEpoch: f.currentEpoch,
		Checkpoints:  make(map[uint64]*pb.CheckpointHeader),
		Reports:      make(map[string]*pb.ExecutionReport),
	}
	for epoch, header := range f.checkpoints {
		payload.Checkpoints[epoch] = proto.Clone(header).(*pb.CheckpointHeader)
	}
	for key, rpt := range f.pendingReports {
		payload.Reports[key] = proto.Clone(rpt).(*pb.ExecutionReport)
	}

	return payload, nil
}

// Restore restores FSM state from snapshot.
func (f *RaftFSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	var snap raftSnapshot
	if err := json.NewDecoder(rc).Decode(&snap); err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}

	f.mu.Lock()
	f.currentEpoch = snap.CurrentEpoch
	f.checkpoints = snap.Checkpoints
	f.pendingReports = snap.Reports
	f.mu.Unlock()
	return nil
}

// LatestCheckpoint returns the last committed checkpoint.
func (f *RaftFSM) LatestCheckpoint() *pb.CheckpointHeader {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if header, ok := f.checkpoints[f.currentEpoch]; ok {
		return proto.Clone(header).(*pb.CheckpointHeader)
	}
	return nil
}

// PendingReports returns a copy of pending reports map.
func (f *RaftFSM) PendingReports() map[string]*pb.ExecutionReport {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make(map[string]*pb.ExecutionReport, len(f.pendingReports))
	for k, v := range f.pendingReports {
		out[k] = proto.Clone(v).(*pb.ExecutionReport)
	}
	return out
}

// ClearPending removes the provided keys from the pending map.
func (f *RaftFSM) ClearPending(keys []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		delete(f.pendingReports, k)
	}
}

type raftSnapshot struct {
	CurrentEpoch uint64                          `json:"current_epoch"`
	Checkpoints  map[uint64]*pb.CheckpointHeader `json:"checkpoints"`
	Reports      map[string]*pb.ExecutionReport  `json:"reports"`
}

func (s raftSnapshot) Persist(sink raft.SnapshotSink) error {
	if err := json.NewEncoder(sink).Encode(s); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s raftSnapshot) Release() {}

func makeReportKey(report *pb.ExecutionReport) string {
	if report == nil {
		return ""
	}
	// Use Timestamp since SubmittedAtUnix doesn't exist
	return fmt.Sprintf("%s:%s:%s:%d", report.IntentId, report.AssignmentId, report.AgentId, report.Timestamp)
}
