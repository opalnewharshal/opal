// Package metrics exposes Prometheus metrics for the OPAL control plane.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	OutcomeReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "opal",
			Subsystem: "controller",
			Name:      "outcome_reconcile_total",
			Help:      "Total number of WorkloadOutcome reconciliations.",
		},
		[]string{"namespace", "outcome", "result"},
	)

	OutcomeReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "opal",
			Subsystem: "controller",
			Name:      "outcome_reconcile_duration_seconds",
			Help:      "Duration of WorkloadOutcome reconciliation in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"namespace", "outcome"},
	)

	ForecastGenerateTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "opal",
			Subsystem: "forecast",
			Name:      "generate_total",
			Help:      "Total number of ClusterForecast generation runs.",
		},
		[]string{"result"},
	)

	ForecastPredictedEvents = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "opal",
			Subsystem: "forecast",
			Name:      "predicted_events_total",
			Help:      "Number of predicted events in the latest forecast.",
		},
		[]string{"workload", "event_type", "severity"},
	)

	SLOViolationsActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "opal",
			Subsystem: "slo",
			Name:      "violations_active",
			Help:      "Number of currently active SLO violations.",
		},
		[]string{"namespace", "outcome", "slo_type"},
	)

	AgentDecisionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "opal",
			Subsystem: "agent",
			Name:      "decisions_total",
			Help:      "Total decisions made by OPAL agents.",
		},
		[]string{"agent", "action", "applied"},
	)

	ConfigAppliedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "opal",
			Subsystem: "config",
			Name:      "applied_total",
			Help:      "Total configurations applied by OPAL.",
		},
		[]string{"namespace", "workload", "result"},
	)

	MemoryStoreSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "opal",
			Subsystem: "memory",
			Name:      "entries_total",
			Help:      "Number of entries in the ClusterMemory store per workload.",
		},
		[]string{"workload"},
	)

	RollbacksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "opal",
			Subsystem: "rollback",
			Name:      "total",
			Help:      "Total number of automatic rollbacks triggered.",
		},
		[]string{"namespace", "workload", "outcome"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		OutcomeReconcileTotal,
		OutcomeReconcileDuration,
		ForecastGenerateTotal,
		ForecastPredictedEvents,
		SLOViolationsActive,
		AgentDecisionsTotal,
		ConfigAppliedTotal,
		MemoryStoreSize,
		RollbacksTotal,
	)
}
