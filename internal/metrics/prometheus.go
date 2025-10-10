package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Prom struct {
	reg *prometheus.Registry
	// Gauges/Counters
	FSMState       prometheus.Gauge
	EpochGauge     prometheus.Gauge
	SigCount       prometheus.Counter
	Reports        prometheus.Counter
	ReportsFailed  prometheus.Counter
	ReportsDropped prometheus.Counter
	FailQueueDepth prometheus.Gauge
	ReportLatency  prometheus.Summary
}

func NewProm() *Prom {
	reg := prometheus.NewRegistry()
	p := &Prom{
		reg:            reg,
		FSMState:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "fsm_state", Help: "FSM state code"}),
		EpochGauge:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "consensus_epoch", Help: "Current epoch"}),
		SigCount:       prometheus.NewCounter(prometheus.CounterOpts{Name: "signatures_total", Help: "Total signatures processed"}),
		Reports:        prometheus.NewCounter(prometheus.CounterOpts{Name: "reports_total", Help: "Total reports processed"}),
		ReportsFailed:  prometheus.NewCounter(prometheus.CounterOpts{Name: "reports_failed_total", Help: "Total report submissions failed"}),
		ReportsDropped: prometheus.NewCounter(prometheus.CounterOpts{Name: "reports_dropped_total", Help: "Total reports dropped (not retried)"}),
		FailQueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{Name: "fail_queue_depth", Help: "Number of reports queued for retry"}),
		ReportLatency:  prometheus.NewSummary(prometheus.SummaryOpts{Name: "report_latency_ms", Help: "Latency of report submissions in ms"}),
	}
	reg.MustRegister(p.FSMState, p.EpochGauge, p.SigCount, p.Reports, p.ReportsFailed, p.ReportsDropped, p.FailQueueDepth, p.ReportLatency)
	return p
}

func (p *Prom) Handler() http.Handler { return promhttp.HandlerFor(p.reg, promhttp.HandlerOpts{}) }

// Implement Provider
func (p *Prom) SetGauge(name string, value float64) {
	switch name {
	case "fsm_state":
		p.FSMState.Set(value)
	case "consensus_epoch":
		p.EpochGauge.Set(value)
	case "fail_queue_depth":
		p.FailQueueDepth.Set(value)
	}
}

func (p *Prom) IncCounter(name string, delta float64) {
	switch name {
	case "signatures_total":
		for i := 0; i < int(delta); i++ {
			p.SigCount.Inc()
		}
	case "reports_total":
		for i := 0; i < int(delta); i++ {
			p.Reports.Inc()
		}
	case "reports_failed_total":
		for i := 0; i < int(delta); i++ {
			p.ReportsFailed.Inc()
		}
	case "reports_dropped_total":
		for i := 0; i < int(delta); i++ {
			p.ReportsDropped.Inc()
		}
	}
}

// Observe supports selected summaries/histograms
func (p *Prom) Observe(name string, value float64) {
	switch name {
	case "report_latency_ms":
		p.ReportLatency.Observe(value)
	default:
		// ignore unknown for now
	}
}
