package metrics

// Provider is a placeholder metrics interface.
// MVP keeps a no-op implementation; wiring can be added later.
type Provider interface {
	SetGauge(name string, value float64)
	IncCounter(name string, delta float64)
	Observe(name string, value float64)
}

type Noop struct{}

func (Noop) SetGauge(string, float64)   {}
func (Noop) IncCounter(string, float64) {}
func (Noop) Observe(string, float64)    {}
