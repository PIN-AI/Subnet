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

// StartProposal moves FSM to Proposed and sets deadlines.
func (f *FSM) StartProposal(h *types.CheckpointHeader, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = StateProposed
	f.header = h
	f.bitmap = nil
	f.proposeDeadline = now.Add(f.proposeTO)
	f.thresholdAt = time.Time{}
	f.preFinalizedAt = time.Time{}
}

// OnSignature records a signature from signerIdx and updates state toward threshold.
func (f *FSM) OnSignature(signerIdx int, now time.Time) (reached bool, count int, required int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == StateIdle || f.header == nil {
		return false, CountBits(f.bitmap), RequiredSigs(f.set)
	}
	// enter collecting on first signature
	if f.state == StateProposed {
		f.state = StateCollecting
		f.collectDeadline = now.Add(f.collectTO)
	}
	f.bitmap = SetBit(f.bitmap, signerIdx)
	count = CountBits(f.bitmap)
	required = RequiredSigs(f.set)
	if count >= required && f.state != StateThreshold {
		f.state = StateThreshold
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
	case StateProposed:
		if now.After(f.proposeDeadline) {
			// proposal expired
			f.state = StateIdle
			changed = true
		}
	case StateCollecting:
		if now.After(f.collectDeadline) {
			// collection window ended without threshold
			f.state = StateIdle
			changed = true
		}
	case StateThreshold:
		// external controller moves to PreFinalized/Finalized (challenge window)
	}
	return changed, f.state
}

// MoveToPreFinalized transitions after threshold met. Controller should run challenge timer outside.
func (f *FSM) MoveToPreFinalized() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == StateThreshold {
		f.state = StatePreFinalized
		f.preFinalizedAt = time.Now()
	}
}

func (f *FSM) MoveToFinalized() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == StatePreFinalized {
		f.state = StateFinalized
	}
}

// GetPreFinalizedAt returns the time when PreFinalized was entered.
func (f *FSM) GetPreFinalizedAt() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.preFinalizedAt
}
