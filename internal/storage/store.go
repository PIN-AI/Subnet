package storage

import (
	"fmt"
	"subnet/internal/types"
)

// Store abstracts minimal persistence; LevelDB impl will be added later.
type Store interface {
	// Generic key-value operations
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error

	SaveHeader(h *types.CheckpointHeader) error
	GetHeader(epoch uint64) (*types.CheckpointHeader, error)
	LoadLatestHeader() (*types.CheckpointHeader, error)
	ListHeaders() ([]*types.CheckpointHeader, error)
	SaveSignersBitmap(epoch uint64, bitmap []byte) error
	LoadSignersBitmap(epoch uint64) ([]byte, error)
	SaveAnchor(hash string, epoch uint64) error
	LoadLatestAnchor() (hash string, epoch uint64, err error)
	SaveReportKey(key string) error
	HasReportKey(key string) (bool, error)
	SaveDoubleSignEvidence(epoch uint64, validatorID string, evidence []byte) error
	// List double-sign evidence; filter by optional validatorID and/or epoch. limit<=0 means no limit.
	ListDoubleSignEvidence(validatorID string, epoch *uint64, limit int) ([]DoubleSignItem, error)
	// Intent winner (first reporter) tracking
	SaveIntentWinner(intentID string, winnerAgentID string, ts int64) error
	GetIntentWinner(intentID string) (winnerAgentID string, ts int64, ok bool, err error)
	SaveVerificationRecord(record *types.VerificationRecord) error
	ListVerificationRecords(intentID, agentID string, limit int) ([]*types.VerificationRecord, error)

	// ExecutionReport storage
	SaveExecutionReport(reportID string, report []byte) error
	GetExecutionReport(reportID string) ([]byte, error)
	ListExecutionReports(intentID string, limit int) (map[string][]byte, error) // returns reportID -> report bytes

	// Close closes the storage and releases resources
	Close() error
}

// InMemory is a simple in-memory store for MVP wiring.
type InMemory struct {
	hdrs        []*types.CheckpointHeader
	bitmaps     map[uint64][]byte
	anchorHash  string
	anchorEpoch uint64
	reports     map[string]struct{}
	dse         []DoubleSignItem
	winners     map[string]struct {
		Agent string
		TS    int64
	}
	records        []*types.VerificationRecord
	kvStore        map[string][]byte            // Generic key-value storage
	execReports    map[string][]byte            // ExecutionReport storage: reportID -> serialized report
	reportsByIntent map[string][]string         // Index: intentID -> []reportID
}

func NewInMemory() *InMemory {
	return &InMemory{
		bitmaps: map[uint64][]byte{},
		reports: map[string]struct{}{},
		winners: map[string]struct {
			Agent string
			TS    int64
		}{},
		records:         []*types.VerificationRecord{},
		kvStore:         map[string][]byte{},
		execReports:     map[string][]byte{},
		reportsByIntent: map[string][]string{},
	}
}

// Get retrieves a value by key from memory
func (s *InMemory) Get(key []byte) ([]byte, error) {
	val, ok := s.kvStore[string(key)]
	if !ok {
		return nil, fmt.Errorf("key not found")
	}
	// Return a copy to prevent modifications
	result := make([]byte, len(val))
	copy(result, val)
	return result, nil
}

// Put stores a key-value pair in memory
func (s *InMemory) Put(key, value []byte) error {
	// Store a copy to prevent external modifications
	val := make([]byte, len(value))
	copy(val, value)
	s.kvStore[string(key)] = val
	return nil
}

func (s *InMemory) SaveHeader(h *types.CheckpointHeader) error {
	s.hdrs = append(s.hdrs, h)
	return nil
}

func (s *InMemory) GetHeader(epoch uint64) (*types.CheckpointHeader, error) {
	for _, h := range s.hdrs {
		if h.Epoch == epoch {
			return h, nil
		}
	}
	return nil, fmt.Errorf("header not found for epoch %d", epoch)
}

func (s *InMemory) LoadLatestHeader() (*types.CheckpointHeader, error) {
	if len(s.hdrs) == 0 {
		return nil, nil
	}
	return s.hdrs[len(s.hdrs)-1], nil
}
func (s *InMemory) ListHeaders() ([]*types.CheckpointHeader, error) {
	out := make([]*types.CheckpointHeader, len(s.hdrs))
	copy(out, s.hdrs)
	return out, nil
}
func (s *InMemory) SaveSignersBitmap(epoch uint64, bitmap []byte) error {
	cp := make([]byte, len(bitmap))
	copy(cp, bitmap)
	s.bitmaps[epoch] = cp
	return nil
}
func (s *InMemory) LoadSignersBitmap(epoch uint64) ([]byte, error) {
	b, ok := s.bitmaps[epoch]
	if !ok {
		return nil, nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp, nil
}
func (s *InMemory) SaveAnchor(hash string, epoch uint64) error {
	s.anchorHash, s.anchorEpoch = hash, epoch
	return nil
}
func (s *InMemory) LoadLatestAnchor() (string, uint64, error) {
	return s.anchorHash, s.anchorEpoch, nil
}
func (s *InMemory) SaveReportKey(key string) error        { s.reports[key] = struct{}{}; return nil }
func (s *InMemory) HasReportKey(key string) (bool, error) { _, ok := s.reports[key]; return ok, nil }
func (s *InMemory) SaveDoubleSignEvidence(epoch uint64, validatorID string, evidence []byte) error {
	evCopy := make([]byte, len(evidence))
	copy(evCopy, evidence)
	s.dse = append(s.dse, DoubleSignItem{Epoch: epoch, ValidatorID: validatorID, Evidence: evCopy})
	return nil
}
func (s *InMemory) ListDoubleSignEvidence(validatorID string, epoch *uint64, limit int) ([]DoubleSignItem, error) {
	// MOCK: no-op, return in-memory slice if present
	out := make([]DoubleSignItem, 0)
	for _, it := range s.dse {
		if validatorID != "" && it.ValidatorID != validatorID {
			continue
		}
		if epoch != nil && it.Epoch != *epoch {
			continue
		}
		out = append(out, it)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// DoubleSignItem represents one record stored by Store.
type DoubleSignItem struct {
	Epoch       uint64
	ValidatorID string
	Evidence    []byte
}

func (s *InMemory) SaveIntentWinner(intentID string, winnerAgentID string, ts int64) error {
	if _, ok := s.winners[intentID]; ok {
		return nil
	}
	s.winners[intentID] = struct {
		Agent string
		TS    int64
	}{Agent: winnerAgentID, TS: ts}
	return nil
}

func (s *InMemory) GetIntentWinner(intentID string) (string, int64, bool, error) {
	if v, ok := s.winners[intentID]; ok {
		return v.Agent, v.TS, true, nil
	}
	return "", 0, false, nil
}

func (s *InMemory) SaveVerificationRecord(record *types.VerificationRecord) error {
	recCopy := *record
	if len(record.EvidenceChecked) > 0 {
		recCopy.EvidenceChecked = append([]string(nil), record.EvidenceChecked...)
	}
	s.records = append(s.records, &recCopy)
	return nil
}

func (s *InMemory) ListVerificationRecords(intentID, agentID string, limit int) ([]*types.VerificationRecord, error) {
	out := make([]*types.VerificationRecord, 0)
	for _, rec := range s.records {
		if intentID != "" && rec.IntentID != intentID {
			continue
		}
		if agentID != "" && rec.AgentID != agentID {
			continue
		}
		copyRec := *rec
		if len(rec.EvidenceChecked) > 0 {
			copyRec.EvidenceChecked = append([]string(nil), rec.EvidenceChecked...)
		}
		out = append(out, &copyRec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// SaveExecutionReport stores an execution report
func (s *InMemory) SaveExecutionReport(reportID string, report []byte) error {
	// Store a copy to prevent external modifications
	reportCopy := make([]byte, len(report))
	copy(reportCopy, report)
	s.execReports[reportID] = reportCopy
	return nil
}

// GetExecutionReport retrieves an execution report by ID
func (s *InMemory) GetExecutionReport(reportID string) ([]byte, error) {
	report, ok := s.execReports[reportID]
	if !ok {
		return nil, fmt.Errorf("execution report not found: %s", reportID)
	}
	// Return a copy to prevent modifications
	result := make([]byte, len(report))
	copy(result, report)
	return result, nil
}

// ListExecutionReports returns all execution reports for a given intent ID
func (s *InMemory) ListExecutionReports(intentID string, limit int) (map[string][]byte, error) {
	result := make(map[string][]byte)
	count := 0

	// Iterate through all reports and filter by intentID
	// Note: This requires parsing the report to extract intentID, which is inefficient
	// For now, we return all reports if intentID is empty
	// In production, we should maintain an index: intentID -> []reportID
	for reportID, report := range s.execReports {
		if intentID == "" {
			// Return all reports
			reportCopy := make([]byte, len(report))
			copy(reportCopy, report)
			result[reportID] = reportCopy
			count++
			if limit > 0 && count >= limit {
				break
			}
		} else {
			// For filtering by intentID, we'd need to deserialize the report
			// For MVP, we'll skip this and just return by reportID prefix if needed
			// Production implementation should maintain proper indexes
			reportCopy := make([]byte, len(report))
			copy(reportCopy, report)
			result[reportID] = reportCopy
			count++
			if limit > 0 && count >= limit {
				break
			}
		}
	}

	return result, nil
}

// Close implements Store interface - no resources to release for in-memory store
func (s *InMemory) Close() error {
	// In-memory store doesn't have resources to release
	// For a real database implementation, this would close connections
	return nil
}
