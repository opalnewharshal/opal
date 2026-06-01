# ClusterForecast

## What is a ClusterForecast?

`ClusterForecast` is the world's first Kubernetes resource designed to hold
**future state predictions**. It stores AI-generated forecasts about what will
happen to your cluster over the next N hours.

This is fundamentally different from:
- **Metrics** (Prometheus): what happened in the past
- **Current state** (etcd): what is happening now
- **ClusterForecast** (OPAL): what will happen next

---

## How It Works

Every `refreshInterval` (default: 15 minutes), the OPAL ForecastAgent:

1. Fetches current metrics for all workloads with `WorkloadOutcome` CRDs
2. Queries ClusterMemory for historical patterns
3. Runs statistical analysis (linear trends, seasonal patterns)
4. Updates `ClusterForecast.status` with per-workload predictions

The ReconcilerAgent then acts on high-confidence predictions before they occur.

---

## Configuration

```yaml
apiVersion: opal.io/v1alpha1
kind: ClusterForecast
metadata:
  name: cluster-forecast   # Convention: one per cluster
  namespace: opal-system
spec:
  horizon: 72h             # How far ahead to forecast
  refreshInterval: 15m     # How often to regenerate
  minConfidence: "0.70"    # Minimum confidence to include predictions
```

---

## Reading the Forecast

```bash
kubectl get clusterforecast cluster-forecast -n opal-system -o yaml
```

Example status:

```yaml
status:
  overallClusterHealth: AtRisk
  lastUpdated: "2025-12-20T19:45:00Z"
  modelVersion: statistical-v1
  forecasts:
    - workloadRef: production/checkout
      confidence: 0.87
      riskScore: 65
      predictedEvents:
        - type: CPUSpike
          predictedAt: "2025-12-20T20:00:00Z"
          confidence: 0.87
          severity: High
          description: >
            CPU projected to exceed 800m in 0.25 hours based on current
            growth trend. Pattern consistent with last 3 Friday evenings.
          supportingEvidence:
            - "Same time last Friday: CPU hit 950m at 20:05"
            - "2 Fridays ago: CPU hit 870m at 19:58"
      recommendedActions:
        - action: pre-scale-replicas
          scheduledFor: "2025-12-20T19:30:00Z"
          reasoning: Pre-scale before predicted CPU spike at 20:00
          status: Applied
      resourceForecast:
        - time: "2025-12-20T20:00:00Z"
          cpuMillicores: 850
          memoryMB: 420
          replicas: 6
```

---

## Predicted Event Types

| Type | Description |
|---|---|
| `CPUSpike` | CPU will exceed threshold |
| `MemoryPressure` | Memory will approach OOM |
| `LatencyDegradation` | Response times will increase |
| `ScaleRequired` | Replicas need to increase |
| `NodePressure` | Node resources becoming scarce |
| `SLOViolationRisk` | SLO will be breached |
| `CostOverrun` | Monthly budget on track to exceed ceiling |

---

## Confidence Scores

Every prediction includes a `confidence` score from 0.0 to 1.0:

| Score | Meaning | Action |
|---|---|---|
| ≥ 0.85 | Very confident | ReconcilerAgent acts immediately |
| ≥ 0.70 | Confident | ReconcilerAgent acts with standard lead time |
| 0.50–0.70 | Moderate | Shown in forecast, no automatic action |
| < 0.50 | Low confidence | Filtered out (below `minConfidence` default) |

Confidence improves over time as ClusterMemory accumulates more historical data.
