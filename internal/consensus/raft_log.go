package consensus

import (
	"encoding/json"
	"fmt"
	"time"
)

// RaftEntryType enumerates consensus log entry kinds replicated through Raft.
type RaftEntryType uint8

const (
	// RaftEntryCheckpoint encodes a checkpoint proposal.
	RaftEntryCheckpoint RaftEntryType = iota
	// RaftEntryExecutionReport encodes a single execution report.
	RaftEntryExecutionReport
)

// RaftLogEntry is the canonical payload replicated via the Raft WAL.
// Data is a serialized protobuf payload (CheckpointHeader or ExecutionReport).
type RaftLogEntry struct {
	Type       RaftEntryType `json:"type"`
	Data       []byte        `json:"data"`
	Timestamp  int64         `json:"timestamp"`
	ProposerID string        `json:"proposer_id"`
}

// NewCheckpointEntry wraps the serialized checkpoint payload into a Raft log entry.
func NewCheckpointEntry(raw []byte, proposerID string) *RaftLogEntry {
	return &RaftLogEntry{
		Type:       RaftEntryCheckpoint,
		Data:       raw,
		Timestamp:  time.Now().Unix(),
		ProposerID: proposerID,
	}
}

// NewExecutionReportEntry wraps the serialized execution report into a Raft log entry.
func NewExecutionReportEntry(raw []byte, proposerID string) *RaftLogEntry {
	return &RaftLogEntry{
		Type:       RaftEntryExecutionReport,
		Data:       raw,
		Timestamp:  time.Now().Unix(),
		ProposerID: proposerID,
	}
}

// Marshal marshals the log entry to JSON. JSON is human readable and stable for Raft replication.
func (e *RaftLogEntry) Marshal() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("raft log entry is nil")
	}
	return json.Marshal(e)
}

// UnmarshalRaftLogEntry decodes a JSON encoded RaftLogEntry.
func UnmarshalRaftLogEntry(data []byte) (*RaftLogEntry, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("invalid raft log payload: empty")
	}
	var entry RaftLogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("decode raft entry: %w", err)
	}
	return &entry, nil
}
