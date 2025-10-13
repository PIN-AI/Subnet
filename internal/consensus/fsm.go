package consensus

import (
	"subnet/internal/types"
	"sync"
	"time"
)

// FSM manages proposal and signature collection with basic timeouts.
type FSM struct {
	mu     sync.Mutex
	state  State
	header *types.CheckpointHeader
	bitmap []byte
	set    *types.ValidatorSet
	// timeouts
	proposeDeadline time.Time
	collectDeadline time.Time
	proposeTO       time.Duration
	collectTO       time.Duration
	// milestones
	thresholdAt    time.Time
	preFinalizedAt time.Time
}

type Params struct {
	ProposeTimeout time.Duration
	CollectTimeout time.Duration
}

func NewFSM(vset *types.ValidatorSet, p Params) *FSM {
	return &FSM{state: StateIdle, set: vset, proposeTO: p.ProposeTimeout, collectTO: p.CollectTimeout}
}

func (f *FSM) State() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *FSM) Bitmap() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.bitmap))
	copy(out, f.bitmap)
	return out
}

func (f *FSM) Header() *types.CheckpointHeader {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.header
}

// StartProposal moves FSM to Collecting and sets deadlines.
func (f *FSM) StartProposal(h *types.CheckpointHeader, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = StateCollecting // 直接进入 Collecting 状态
	f.header = h
	f.bitmap = nil
	f.collectDeadline = now.Add(f.collectTO) // 使用 collect 超时
	f.thresholdAt = time.Time{}
	f.preFinalizedAt = time.Time{}
}

// OnSignature records a signature from signerIdx and updates state.
// Returns (reached=true) when threshold is met and ready to finalize.
func (f *FSM) OnSignature(signerIdx int, now time.Time) (reached bool, count int, required int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == StateIdle || f.header == nil {
		return false, CountBits(f.bitmap), RequiredSigs(f.set)
	}

	// Record signature
	f.bitmap = SetBit(f.bitmap, signerIdx)
	count = CountBits(f.bitmap)
	required = RequiredSigs(f.set)

	// Check if threshold reached (但不自动转状态,由外部调用 MoveToFinalized)
	if count >= required {
		f.thresholdAt = now
		return true, count, required
	}
	return false, count, required
}

// Tick processes time-based transitions. Returns true if state changed.
func (f *FSM) Tick(now time.Time) (changed bool, newState State) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.state {
	case StateCollecting:
		if now.After(f.collectDeadline) {
			// collection window ended without threshold
			f.state = StateIdle
			changed = true
		}
	}
	return changed, f.state
}

// MoveToFinalized transitions from Collecting to Finalized (when threshold is reached).
func (f *FSM) MoveToFinalized() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == StateCollecting {
		f.state = StateFinalized
		f.preFinalizedAt = time.Now() // 记录 finalize 时间
	}
}

// GetFinalizedAt returns the time when Finalized was entered.
func (f *FSM) GetFinalizedAt() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.preFinalizedAt
}

// Reset moves FSM back to Idle, clearing all state.
func (f *FSM) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = StateIdle
	f.header = nil
	f.bitmap = nil
	f.proposeDeadline = time.Time{}
	f.collectDeadline = time.Time{}
	f.thresholdAt = time.Time{}
	f.preFinalizedAt = time.Time{}
}
