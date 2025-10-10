package blockchain

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsOnce           sync.Once
	verificationCounter   *prometheus.CounterVec
	cacheEventCounter     *prometheus.CounterVec
	missingAddressCounter *prometheus.CounterVec
)

func initMetrics() {
	metricsOnce.Do(func() {
		verificationCounter = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "chain_verification_total",
				Help: "Total chain verification attempts by role and result",
			},
			[]string{"role", "result"},
		)
		cacheEventCounter = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "chain_verification_cache_events_total",
				Help: "Cache events for chain verification",
			},
			[]string{"role", "event"},
		)
		missingAddressCounter = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "chain_verification_missing_address_total",
				Help: "Count of verification attempts without a chain address",
			},
			[]string{"role"},
		)

		prometheus.MustRegister(verificationCounter, cacheEventCounter, missingAddressCounter)
	})
}

func observeVerification(role, result string) {
	initMetrics()
	verificationCounter.WithLabelValues(role, result).Inc()
}

func observeCacheEvent(role, event string) {
	initMetrics()
	cacheEventCounter.WithLabelValues(role, event).Inc()
}

func observeMissingAddress(role string) {
	initMetrics()
	missingAddressCounter.WithLabelValues(role).Inc()
}
