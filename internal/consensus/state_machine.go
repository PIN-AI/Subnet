package consensus

import (
	"fmt"
	"sync"
	"time"

	pb "subnet/proto/subnet"
	"subnet/internal/config"
	"subnet/internal/crypto"
	"subnet/internal/types"
)

// StateMachine manages the consensus state transitions
type StateMachine struct {
	mu sync.RWMutex

	state         State
	validatorSet  *types.ValidatorSet
	signer        crypto.Signer
	timeoutConfig *config.TimeoutConfig // Configuration for timeouts

	// Current round data
	currentHeader *pb.CheckpointHeader
	signatures    map[string]*pb.Signature // validator_id -> signature
	signersBitmap []byte

	// Timing
	startTime       time.Time
	proposeDeadline time.Time
	collectDeadline time.Time
	thresholdTime   time.Time
}

// NewStateMachine creates a new consensus state machine
func NewStateMachine(validatorSet *types.ValidatorSet, signer crypto.Signer) *StateMachine {
	return &StateMachine{
		state:        StateIdle,
		validatorSet: validatorSet,
		signer:       signer,
		signatures:   make(map[string]*pb.Signature),
	}
}

// NewStateMachineWithConfig creates a new consensus state machine with timeout configuration
func NewStateMachineWithConfig(validatorSet *types.ValidatorSet, signer crypto.Signer, timeoutConfig *config.TimeoutConfig) *StateMachine {
	return &StateMachine{
		state:         StateIdle,
		validatorSet:  validatorSet,
		signer:        signer,
		signatures:    make(map[string]*pb.Signature),
		timeoutConfig: timeoutConfig,
	}
}

// GetState returns current consensus state
func (sm *StateMachine) GetState() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

// GetCurrentHeader returns the current checkpoint header being processed
func (sm *StateMachine) GetCurrentHeader() *pb.CheckpointHeader {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.currentHeader
}

// ProposeHeader starts a new consensus round with proposed header
func (sm *StateMachine) ProposeHeader(header *pb.CheckpointHeader) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StateIdle {
		return fmt.Errorf("cannot propose in state %v", sm.state)
	}

	sm.currentHeader = header
	sm.signatures = make(map[string]*pb.Signature)
	sm.signersBitmap = make([]byte, (len(sm.validatorSet.Validators)+7)/8)
	sm.state = StateProposed
	sm.startTime = time.Now()
	// Use configured timeout or default
	proposeTimeout := 10 * time.Second
	if sm.timeoutConfig != nil {
		proposeTimeout = sm.timeoutConfig.ProposeTimeout
	}
	sm.proposeDeadline = sm.startTime.Add(proposeTimeout)

	return nil
}

// StartCollecting transitions to collecting signatures state
func (sm *StateMachine) StartCollecting(collectTimeout time.Duration) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StateProposed {
		return fmt.Errorf("cannot start collecting in state %v", sm.state)
	}

	sm.state = StateCollecting
	sm.collectDeadline = time.Now().Add(collectTimeout)
	return nil
}

// AddSignature adds a signature from a validator
func (sm *StateMachine) AddSignature(sig *pb.Signature) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StateCollecting && sm.state != StateProposed {
		return fmt.Errorf("cannot add signature in state %v", sm.state)
	}

	// Transition to collecting if needed
	if sm.state == StateProposed {
		sm.state = StateCollecting
		// Use configured timeout or default
		collectTimeout := 15 * time.Second
		if sm.timeoutConfig != nil {
			collectTimeout = sm.timeoutConfig.CollectTimeout
		}
		sm.collectDeadline = time.Now().Add(collectTimeout)
	}

	// Add signature using SignerId field
	sm.signatures[sig.SignerId] = sig

	// Update bitmap
	idx := sm.validatorSet.IndexByID(sig.SignerId)
	if idx >= 0 {
		byteIdx := idx / 8
		bitIdx := uint(idx % 8)
		sm.signersBitmap[byteIdx] |= 1 << bitIdx
	}

	// Check if we reached threshold
	if sm.checkThreshold() {
		sm.state = StateThreshold
		sm.thresholdTime = time.Now()
	}

	return nil
}

// HasThreshold checks if we have enough signatures
func (sm *StateMachine) HasThreshold() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.checkThreshold()
}

// checkThreshold internal check without lock
func (sm *StateMachine) checkThreshold() bool {
	signCount := len(sm.signatures)
	required := sm.validatorSet.RequiredSignatures()
	return signCount >= required
}

// GetSignatures returns collected signatures
func (sm *StateMachine) GetSignatures() map[string]*pb.Signature {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sigs := make(map[string]*pb.Signature)
	for k, v := range sm.signatures {
		sigs[k] = v
	}
	return sigs
}

// GetSignersBitmap returns the bitmap of validators who signed
func (sm *StateMachine) GetSignersBitmap() []byte {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	bitmap := make([]byte, len(sm.signersBitmap))
	copy(bitmap, sm.signersBitmap)
	return bitmap
}

// MoveToPreFinalized transitions to pre-finalized state
func (sm *StateMachine) MoveToPreFinalized() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StateThreshold {
		return fmt.Errorf("cannot pre-finalize from state %v", sm.state)
	}

	sm.state = StatePreFinalized
	return nil
}

// Finalize completes the consensus round
func (sm *StateMachine) Finalize() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StatePreFinalized && sm.state != StateThreshold {
		return fmt.Errorf("cannot finalize from state %v", sm.state)
	}

	sm.state = StateFinalized
	return nil
}

// Reset resets the state machine to idle
func (sm *StateMachine) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.state = StateIdle
	sm.currentHeader = nil
	sm.signatures = make(map[string]*pb.Signature)
	sm.signersBitmap = nil
	sm.startTime = time.Time{}
}

// IsTimedOut checks if current operation has timed out
func (sm *StateMachine) IsTimedOut(timeout time.Duration) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.state == StateIdle {
		return false
	}

	switch sm.state {
	case StateProposed:
		return time.Now().After(sm.proposeDeadline)
	case StateCollecting:
		return time.Now().After(sm.collectDeadline)
	default:
		return time.Since(sm.startTime) > timeout
	}
}

// GetProgress returns current consensus progress info
func (sm *StateMachine) GetProgress() (signCount, required int, progress float64) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	signCount = len(sm.signatures)
	required = sm.validatorSet.RequiredSignatures()
	if required > 0 {
		progress = float64(signCount) / float64(required)
		if progress > 1.0 {
			progress = 1.0
		}
	}
	return
}