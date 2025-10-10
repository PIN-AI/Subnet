package consensus

type State int

const (
	StateIdle State = iota
	StateProposed
	StateCollecting
	StateThreshold
	StatePreFinalized
	StateFinalized
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateProposed:
		return "proposed"
	case StateCollecting:
		return "collecting"
	case StateThreshold:
		return "threshold"
	case StatePreFinalized:
		return "pre_finalized"
	case StateFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}
