package storage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"

	"subnet/internal/types"
)

type LevelDBStore struct{ db *leveldb.DB }

func NewLevelDB(path string) (*LevelDBStore, error) {
	p := filepath.Clean(path)
	db, err := leveldb.OpenFile(p, nil)
	if err != nil {
		return nil, err
	}
	return &LevelDBStore{db: db}, nil
}

func (s *LevelDBStore) Close() error { return s.db.Close() }

// Get retrieves a value by key from the database
func (s *LevelDBStore) Get(key []byte) ([]byte, error) {
	return s.db.Get(key, nil)
}

// Put stores a key-value pair in the database
func (s *LevelDBStore) Put(key, value []byte) error {
	return s.db.Put(key, value, nil)
}

func keyHeader(epoch uint64) []byte         { return []byte(fmt.Sprintf("cp:epoch:%020d", epoch)) }
func keyBitmap(epoch uint64) []byte         { return []byte(fmt.Sprintf("bm:epoch:%020d", epoch)) }
func keyAnchor() []byte                     { return []byte("anchor") }
func keyReport(intent, agent string) []byte { return []byte(fmt.Sprintf("rep:%s:%s", intent, agent)) }
func keyDS(epoch uint64, vid string) []byte { return []byte(fmt.Sprintf("ds:%020d:%s", epoch, vid)) }
func keyWinner(intent string) []byte        { return []byte(fmt.Sprintf("iw:%s", intent)) }
func keyVerificationRecord(intent, agent, record string) []byte {
	return []byte(fmt.Sprintf("vr:%s:%s:%s", intent, agent, record))
}
func keyExecutionReport(reportID string) []byte { return []byte(fmt.Sprintf("er:%s", reportID)) }

func (s *LevelDBStore) SaveHeader(h *types.CheckpointHeader) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return s.db.Put(keyHeader(h.Epoch), b, nil)
}

func (s *LevelDBStore) GetHeader(epoch uint64) (*types.CheckpointHeader, error) {
	data, err := s.db.Get(keyHeader(epoch), nil)
	if err == leveldb.ErrNotFound {
		return nil, fmt.Errorf("header not found for epoch %d", epoch)
	}
	if err != nil {
		return nil, err
	}
	var h types.CheckpointHeader
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (s *LevelDBStore) LoadLatestHeader() (*types.CheckpointHeader, error) {
	it := s.db.NewIterator(util.BytesPrefix([]byte("cp:epoch:")), nil)
	defer it.Release()
	var last *types.CheckpointHeader
	for it.Next() {
		// iterate to the last
		var h types.CheckpointHeader
		if err := json.Unmarshal(it.Value(), &h); err != nil {
			continue
		}
		last = &h
	}
	if last == nil {
		return nil, nil
	}
	return last, nil
}

func (s *LevelDBStore) ListHeaders() ([]*types.CheckpointHeader, error) {
	it := s.db.NewIterator(util.BytesPrefix([]byte("cp:epoch:")), nil)
	defer it.Release()
	var out []*types.CheckpointHeader
	for it.Next() {
		var h types.CheckpointHeader
		if err := json.Unmarshal(it.Value(), &h); err != nil {
			continue
		}
		// iterator over prefix is ordered by epoch key suffix (zero-padded), so append keeps chronological order
		cp := h
		out = append(out, &cp)
	}
	return out, it.Error()
}

func (s *LevelDBStore) SaveSignersBitmap(epoch uint64, bitmap []byte) error {
	return s.db.Put(keyBitmap(epoch), bitmap, nil)
}

func (s *LevelDBStore) LoadSignersBitmap(epoch uint64) ([]byte, error) {
	b, err := s.db.Get(keyBitmap(epoch), nil)
	if err == leveldb.ErrNotFound {
		return nil, nil
	}
	return b, err
}

func (s *LevelDBStore) SaveAnchor(hash string, epoch uint64) error {
	val := []byte(fmt.Sprintf("%020d|%s", epoch, hash))
	return s.db.Put(keyAnchor(), val, nil)
}

func (s *LevelDBStore) LoadLatestAnchor() (string, uint64, error) {
	b, err := s.db.Get(keyAnchor(), nil)
	if err == leveldb.ErrNotFound {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	var epoch uint64
	var hash string
	if _, err := fmt.Sscanf(string(b), "%d|%s", &epoch, &hash); err != nil {
		return "", 0, err
	}
	return hash, epoch, nil
}

func (s *LevelDBStore) SaveReportKey(key string) error {
	return s.db.Put([]byte("rep:"+key), []byte{1}, nil)
}
func (s *LevelDBStore) HasReportKey(key string) (bool, error) {
	_, err := s.db.Get([]byte("rep:"+key), nil)
	if err == leveldb.ErrNotFound {
		return false, nil
	}
	return err == nil, err
}

func (s *LevelDBStore) SaveDoubleSignEvidence(epoch uint64, validatorID string, evidence []byte) error {
	return s.db.Put(keyDS(epoch, validatorID), evidence, nil)
}

func (s *LevelDBStore) ListDoubleSignEvidence(validatorID string, epoch *uint64, limit int) ([]DoubleSignItem, error) {
	it := s.db.NewIterator(util.BytesPrefix([]byte("ds:")), nil)
	defer it.Release()
	var out []DoubleSignItem
	for it.Next() {
		// key format: ds:%020d:%s
		var e uint64
		var vid string
		if _, err := fmt.Sscanf(string(it.Key()), "ds:%d:%s", &e, &vid); err != nil {
			continue
		}
		if validatorID != "" && vid != validatorID {
			continue
		}
		if epoch != nil && e != *epoch {
			continue
		}
		ev := make([]byte, len(it.Value()))
		copy(ev, it.Value())
		out = append(out, DoubleSignItem{Epoch: e, ValidatorID: vid, Evidence: ev})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, it.Error()
}

func (s *LevelDBStore) SaveIntentWinner(intentID string, winnerAgentID string, ts int64) error {
	// only set if not exists
	k := keyWinner(intentID)
	_, err := s.db.Get(k, nil)
	if err == nil {
		return nil
	}
	val := []byte(fmt.Sprintf("%s|%d", winnerAgentID, ts))
	return s.db.Put(k, val, nil)
}

func (s *LevelDBStore) GetIntentWinner(intentID string) (string, int64, bool, error) {
	b, err := s.db.Get(keyWinner(intentID), nil)
	if err == leveldb.ErrNotFound {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	var agent string
	var ts int64
	if _, err := fmt.Sscanf(string(b), "%s|%d", &agent, &ts); err != nil {
		return "", 0, false, err
	}
	return agent, ts, true, nil
}

// NOTE(mock): verification record persistence uses simple JSON blobs without indexes.
func (s *LevelDBStore) SaveVerificationRecord(record *types.VerificationRecord) error {
	if record.RecordID == "" {
		return fmt.Errorf("record id required")
	}
	key := keyVerificationRecord(record.IntentID, record.AgentID, record.RecordID)
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.db.Put(key, payload, nil)
}

// NOTE(mock): linear scan over vr:* keys; replace with indexed queries when productionizing.
func (s *LevelDBStore) ListVerificationRecords(intentID, agentID string, limit int) ([]*types.VerificationRecord, error) {
	prefix := []byte("vr:")
	if intentID != "" {
		prefix = []byte(fmt.Sprintf("vr:%s:", intentID))
	}
	iter := s.db.NewIterator(util.BytesPrefix(prefix), nil)
	defer iter.Release()
	out := make([]*types.VerificationRecord, 0)
	for iter.Next() {
		if agentID != "" {
			key := string(iter.Key())
			parts := strings.SplitN(key, ":", 4)
			if len(parts) < 4 || parts[2] != agentID {
				continue
			}
		}
		var rec types.VerificationRecord
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		out = append(out, &rec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveExecutionReport stores an execution report by ID
func (s *LevelDBStore) SaveExecutionReport(reportID string, report []byte) error {
	return s.db.Put(keyExecutionReport(reportID), report, nil)
}

// GetExecutionReport retrieves an execution report by ID
func (s *LevelDBStore) GetExecutionReport(reportID string) ([]byte, error) {
	data, err := s.db.Get(keyExecutionReport(reportID), nil)
	if err == leveldb.ErrNotFound {
		return nil, fmt.Errorf("execution report not found: %s", reportID)
	}
	return data, err
}

// ListExecutionReports returns all execution reports, optionally filtered by intentID
// Note: intentID filtering requires parsing reports, which is not efficient
// For MVP, this returns all reports when intentID is empty
func (s *LevelDBStore) ListExecutionReports(intentID string, limit int) (map[string][]byte, error) {
	iter := s.db.NewIterator(util.BytesPrefix([]byte("er:")), nil)
	defer iter.Release()

	result := make(map[string][]byte)
	count := 0

	for iter.Next() {
		// Extract reportID from key (format: "er:reportID")
		key := string(iter.Key())
		reportID := strings.TrimPrefix(key, "er:")

		// Copy the value
		reportData := make([]byte, len(iter.Value()))
		copy(reportData, iter.Value())

		// For intentID filtering, we'd need to deserialize and check
		// For MVP, we skip filtering by intentID (would require proto unmarshaling)
		result[reportID] = reportData
		count++

		if limit > 0 && count >= limit {
			break
		}
	}

	if err := iter.Error(); err != nil {
		return nil, err
	}

	return result, nil
}
