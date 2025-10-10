package consensus

import (
	"subnet/internal/types"
	"sync"
	"time"
)

// LeaderTracker keeps per-epoch timing data to decide when backup should take over.
type LeaderTracker struct {
	mu       sync.Mutex
	set      *types.ValidatorSet
	timeout  time.Duration
	epoch    uint64
	lastBeat time.Time
}

func NewLeaderTracker(set *types.ValidatorSet, timeout time.Duration) *LeaderTracker {
	return &LeaderTracker{set: set, timeout: timeout}
}

func (lt *LeaderTracker) SetTimeout(timeout time.Duration) {
	lt.mu.Lock()
	lt.timeout = timeout
	lt.mu.Unlock()
}

func (lt *LeaderTracker) Leader(epoch uint64) (idx int, v *types.Validator) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	idx = SelectLeader(epoch, lt.set)
	if idx >= 0 {
		v = &lt.set.Validators[idx]
	}
	if epoch != lt.epoch {
		lt.epoch = epoch
		lt.lastBeat = time.Time{}
	}
	return idx, v
}

func (lt *LeaderTracker) Backup(epoch uint64) (idx int, v *types.Validator) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	idx = SelectBackup(epoch, lt.set)
	if idx >= 0 {
		v = &lt.set.Validators[idx]
	}
	if epoch != lt.epoch {
		lt.epoch = epoch
		lt.lastBeat = time.Time{}
	}
	return idx, v
}

// RecordActivity marks leader heartbeat (proposal, signature collection, etc.).
func (lt *LeaderTracker) RecordActivity(epoch uint64, ts time.Time) {
	lt.mu.Lock()
	if lt.epoch != epoch {
		lt.epoch = epoch
	}
	lt.lastBeat = ts
	lt.mu.Unlock()
}

// ShouldFailover reports whether backup should attempt takeover.
func (lt *LeaderTracker) ShouldFailover(epoch uint64, now time.Time) bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if lt.timeout <= 0 {
		return false
	}
	if epoch != lt.epoch {
		lt.epoch = epoch
		lt.lastBeat = time.Time{}
		return false
	}
	if lt.lastBeat.IsZero() {
		return false
	}
	return now.Sub(lt.lastBeat) >= lt.timeout
}

// UpdateSet swaps the validator set (assumes already validated/sorted).
func (lt *LeaderTracker) UpdateSet(set *types.ValidatorSet) {
	lt.mu.Lock()
	lt.set = set
	lt.lastBeat = time.Time{}
	lt.mu.Unlock()
}
