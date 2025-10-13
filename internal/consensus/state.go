package consensus

type State int

const (
	StateIdle State = iota
	StateCollecting // 合并了 Proposed 和 Collecting,收集签名中
	StateFinalized  // 已完成(达到阈值后直接 finalize)
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateCollecting:
		return "collecting"
	case StateFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}
