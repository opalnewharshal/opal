// Package forecast implements OPAL's predictive engine.
// It combines statistical time-series analysis with LLM-based reasoning
// (via kagent agents) to produce ClusterForecast predictions.
package forecast

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	opalv1alpha1 "github.com/opal-io/opal/api/v1alpha1"
	"github.com/opal-io/opal/internal/memory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Predictor generates WorkloadForecast predictions for a given workload.
type Predictor interface {
	// Predict generates a forecast for a workload over the given horizon.
	Predict(ctx context.Context, req PredictRequest) (*opalv1alpha1.WorkloadForecast, error)
}

// PredictRequest encapsulates all inputs needed to generate a forecast.
type PredictRequest struct {
	// WorkloadRef identifies the workload (namespace/name).
	WorkloadRef string
	// Horizon is how far into the future to forecast.
	Horizon time.Duration
	// Now is the reference time for the forecast. Defaults to time.Now().
	Now time.Time
	// MinConfidence filters out predictions below this threshold.
	MinConfidence float64
}

// StatisticalPredictor uses historical memory entries to generate forecasts
// using seasonal decomposition and moving averages. It does not require
// an external LLM call, making it fast and deterministic.
type StatisticalPredictor struct {
	store  *memory.Store
	window time.Duration // lookback window for historical analysis
}

// NewStatisticalPredictor creates a StatisticalPredictor backed by the given store.
func NewStatisticalPredictor(store *memory.Store, window time.Duration) *StatisticalPredictor {
	return &StatisticalPredictor{store: store, window: window}
}

// Predict implements Predictor using statistical methods.
func (p *StatisticalPredictor) Predict(ctx context.Context, req PredictRequest) (*opalv1alpha1.WorkloadForecast, error) {
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}

	entries, err := p.store.Query(ctx, memory.QueryFilter{
		WorkloadRef: req.WorkloadRef,
		Since:       req.Now.Add(-p.window),
		Until:       req.Now,
		Types: []memory.EntryType{
			memory.EntryTypeMetricSnapshot,
			memory.EntryTypeScalingEvent,
			memory.EntryTypeSLOViolation,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query memory: %w", err)
	}

	forecast := &opalv1alpha1.WorkloadForecast{
		WorkloadRef: req.WorkloadRef,
	}

	cpuSeries := extractMetricSeries(entries, "cpu_millicores")
	memSeries := extractMetricSeries(entries, "memory_mb")
	rpsSeries := extractMetricSeries(entries, "rps")

	resourceForecast := p.projectResources(req.Now, req.Horizon, cpuSeries, memSeries, rpsSeries)
	forecast.ResourceForecast = resourceForecast

	events := p.detectPredictedEvents(req.Now, req.Horizon, req.MinConfidence, cpuSeries, memSeries, entries)
	forecast.PredictedEvents = events

	actions := p.deriveRecommendedActions(req.Now, events)
	forecast.RecommendedActions = actions

	forecast.RiskScore = p.computeRiskScore(events)
	forecast.Confidence = p.computeOverallConfidence(len(entries), len(events))

	patterns, _ := p.store.GetPatterns(ctx, req.WorkloadRef)
	if len(patterns) > 0 {
		forecast.Confidence = math.Min(forecast.Confidence+0.1, 1.0)
	}

	return forecast, nil
}

// metricPoint is a timestamped scalar measurement.
type metricPoint struct {
	t time.Time
	v float64
}

func extractMetricSeries(entries []*memory.Entry, key string) []metricPoint {
	var series []metricPoint
	for _, e := range entries {
		if v, ok := e.Metrics[key]; ok {
			series = append(series, metricPoint{t: e.Timestamp, v: v})
		}
	}
	sort.Slice(series, func(i, j int) bool { return series[i].t.Before(series[j].t) })
	return series
}

func (p *StatisticalPredictor) projectResources(
	now time.Time,
	horizon time.Duration,
	cpu, mem, rps []metricPoint,
) []opalv1alpha1.ResourcePoint {
	steps := 8
	stepDuration := horizon / time.Duration(steps)
	var points []opalv1alpha1.ResourcePoint

	cpuTrend := linearTrend(cpu)
	memTrend := linearTrend(mem)

	for i := 1; i <= steps; i++ {
		t := now.Add(time.Duration(i) * stepDuration)
		elapsed := float64(i) * stepDuration.Hours()

		projectedCPU := int64(math.Max(cpuTrend.base+cpuTrend.slope*elapsed, 0))
		projectedMem := int64(math.Max(memTrend.base+memTrend.slope*elapsed, 0))

		replicas := int32(1)
		if projectedCPU > 0 {
			replicas = int32(math.Ceil(float64(projectedCPU) / 500.0))
		}

		points = append(points, opalv1alpha1.ResourcePoint{
			Time:          metav1.NewTime(t),
			CPUMillicores: projectedCPU,
			MemoryMB:      projectedMem,
			Replicas:      replicas,
		})
	}
	return points
}

func (p *StatisticalPredictor) detectPredictedEvents(
	now time.Time,
	horizon time.Duration,
	minConf float64,
	cpu, mem []metricPoint,
	entries []*memory.Entry,
) []opalv1alpha1.PredictedEvent {
	var events []opalv1alpha1.PredictedEvent

	// CPU spike detection: if recent trend suggests crossing 80% threshold
	cpuTrend := linearTrend(cpu)
	if cpuTrend.slope > 0 && len(cpu) >= 3 {
		hoursUntilSpike := (800.0 - cpuTrend.base) / cpuTrend.slope
		if hoursUntilSpike > 0 && hoursUntilSpike < horizon.Hours() {
			conf := confidenceFromDataPoints(len(cpu), cpuTrend.r2)
			if conf >= minConf {
				events = append(events, opalv1alpha1.PredictedEvent{
					Type:        "CPUSpike",
					PredictedAt: metav1.NewTime(now.Add(time.Duration(hoursUntilSpike * float64(time.Hour)))),
					Confidence:  conf,
					Severity:    severityFromConfidence(conf),
					Description: fmt.Sprintf("CPU projected to exceed 800m in %.1f hours based on current growth trend", hoursUntilSpike),
				})
			}
		}
	}

	// Memory pressure: similar approach
	memTrend := linearTrend(mem)
	if memTrend.slope > 0 && len(mem) >= 3 {
		hoursUntilPressure := (3000.0 - memTrend.base) / memTrend.slope
		if hoursUntilPressure > 0 && hoursUntilPressure < horizon.Hours() {
			conf := confidenceFromDataPoints(len(mem), memTrend.r2)
			if conf >= minConf {
				events = append(events, opalv1alpha1.PredictedEvent{
					Type:        "MemoryPressure",
					PredictedAt: metav1.NewTime(now.Add(time.Duration(hoursUntilPressure * float64(time.Hour)))),
					Confidence:  conf,
					Severity:    severityFromConfidence(conf),
					Description: fmt.Sprintf("Memory projected to exceed 3000MB in %.1f hours", hoursUntilPressure),
				})
			}
		}
	}

	// SLO violation history: if violations occurred at same time last week
	violationCount := countRecentViolations(entries, 7*24*time.Hour)
	if violationCount >= 2 {
		conf := math.Min(float64(violationCount)*0.15, 0.9)
		if conf >= minConf {
			events = append(events, opalv1alpha1.PredictedEvent{
				Type:        "SLOViolationRisk",
				PredictedAt: metav1.NewTime(now.Add(horizon / 2)),
				Confidence:  conf,
				Severity:    "High",
				Description: fmt.Sprintf("SLO violation pattern detected: %d violations in the past week suggests recurrence risk", violationCount),
				SupportingEvidence: []string{
					fmt.Sprintf("%d SLO violations in past 7 days", violationCount),
				},
			})
		}
	}

	return events
}

func (p *StatisticalPredictor) deriveRecommendedActions(now time.Time, events []opalv1alpha1.PredictedEvent) []opalv1alpha1.RecommendedAction {
	var actions []opalv1alpha1.RecommendedAction
	for _, e := range events {
		leadTime := 30 * time.Minute
		switch e.Type {
		case "CPUSpike":
			actions = append(actions, opalv1alpha1.RecommendedAction{
				Action:       "pre-scale-replicas",
				ScheduledFor: metav1.NewTime(e.PredictedAt.Add(-leadTime)),
				Reasoning:    fmt.Sprintf("Pre-scale before predicted CPU spike at %s", e.PredictedAt.Format(time.RFC3339)),
				Status:       "Pending",
			})
		case "MemoryPressure":
			actions = append(actions, opalv1alpha1.RecommendedAction{
				Action:       "increase-memory-limit",
				ScheduledFor: metav1.NewTime(e.PredictedAt.Add(-leadTime)),
				Reasoning:    "Prevent OOM kill by increasing memory ceiling before pressure event",
				Status:       "Pending",
			})
		case "SLOViolationRisk":
			actions = append(actions, opalv1alpha1.RecommendedAction{
				Action:       "scale-out-and-enable-circuit-breaker",
				ScheduledFor: metav1.NewTime(now.Add(15 * time.Minute)),
				Reasoning:    "Historical pattern indicates SLO risk; scaling out and enabling circuit breaker as precaution",
				Status:       "Pending",
			})
		}
	}
	return actions
}

func (p *StatisticalPredictor) computeRiskScore(events []opalv1alpha1.PredictedEvent) int32 {
	if len(events) == 0 {
		return 0
	}
	score := 0.0
	for _, e := range events {
		weight := 1.0
		switch e.Severity {
		case "Critical":
			weight = 4.0
		case "High":
			weight = 2.5
		case "Medium":
			weight = 1.5
		}
		score += e.Confidence * weight * 20
	}
	return int32(math.Min(score, 100))
}

func (p *StatisticalPredictor) computeOverallConfidence(dataPoints, eventCount int) float64 {
	if dataPoints < 5 {
		return 0.3
	}
	if dataPoints < 20 {
		return 0.55
	}
	if dataPoints < 100 {
		return 0.70
	}
	return 0.85
}

// trendResult holds the result of a linear regression.
type trendResult struct {
	base  float64 // y-intercept (current value estimate)
	slope float64 // change per hour
	r2    float64 // coefficient of determination (fit quality)
}

// linearTrend fits a least-squares line to a metric series.
// Returns slope in units/hour.
func linearTrend(series []metricPoint) trendResult {
	n := float64(len(series))
	if n < 2 {
		if n == 1 {
			return trendResult{base: series[0].v}
		}
		return trendResult{}
	}

	t0 := series[0].t
	var sumX, sumY, sumXY, sumX2 float64
	for _, p := range series {
		x := p.t.Sub(t0).Hours()
		sumX += x
		sumY += p.v
		sumXY += x * p.v
		sumX2 += x * x
	}

	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return trendResult{base: sumY / n}
	}
	slope := (n*sumXY - sumX*sumY) / denom
	base := (sumY - slope*sumX) / n

	// compute R²
	meanY := sumY / n
	var ssTot, ssRes float64
	for _, p := range series {
		x := p.t.Sub(t0).Hours()
		predicted := base + slope*x
		ssTot += (p.v - meanY) * (p.v - meanY)
		ssRes += (p.v - predicted) * (p.v - predicted)
	}
	r2 := 0.0
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}

	return trendResult{base: base, slope: slope, r2: r2}
}

func confidenceFromDataPoints(n int, r2 float64) float64 {
	dataConf := math.Min(float64(n)/50.0, 1.0)
	return math.Min((dataConf*0.5 + math.Max(r2, 0)*0.5), 1.0)
}

func severityFromConfidence(conf float64) string {
	switch {
	case conf >= 0.85:
		return "Critical"
	case conf >= 0.70:
		return "High"
	case conf >= 0.50:
		return "Medium"
	default:
		return "Low"
	}
}

func countRecentViolations(entries []*memory.Entry, window time.Duration) int {
	cutoff := time.Now().Add(-window)
	count := 0
	for _, e := range entries {
		if e.Type == memory.EntryTypeSLOViolation && e.Timestamp.After(cutoff) {
			count++
		}
	}
	return count
}
