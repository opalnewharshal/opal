# WorkloadOutcome

## What is a WorkloadOutcome?

A `WorkloadOutcome` is a new Kubernetes resource that lets you declare **what
you want to achieve** for a workload, rather than how to run it.

Traditional Kubernetes (before OPAL):
```yaml
# You must know HOW to run it
spec:
  replicas: 3
  template:
    spec:
      containers:
        - resources:
            requests:
              cpu: "250m"
              memory: "256Mi"
            limits:
              cpu: "1000m"
              memory: "1Gi"
```

With OPAL:
```yaml
# You declare WHAT you want to achieve
spec:
  slo:
    latencyP99Ms: 100
    availability: "99.9"
  cost:
    monthlyCeiling: "200"
```

OPAL derives and owns all the Kubernetes configuration required to achieve your
declared outcomes.

---

## SLO Specification

### Latency Targets

`latencyP99Ms` and `latencyP95Ms` define response time targets.

OPAL maps these to CPU profiles:

| P99 Latency | CPU Profile | Request | Limit |
|---|---|---|---|
| < 50ms | High Performance | 500m | 2000m |
| < 100ms | Standard | 250m | 1000m |
| < 500ms | Normal | 100m | 500m |
| ≥ 500ms | Minimal | 50m | 200m |

### Availability Target

`availability` as a percentage string (e.g., `"99.95"`).

OPAL maps this to minimum replica counts:

| Availability | Min Replicas | Notes |
|---|---|---|
| ≥ 99.99% | 5 | Multi-zone required |
| ≥ 99.95% | 4 | |
| ≥ 99.9% | 3 | |
| ≥ 99.0% | 2 | Basic HA |
| < 99.0% | 1 | Single replica allowed |

### Error Rate and Throughput

`errorRatePercent` sets a maximum acceptable error rate.
`throughputRPS` sets a minimum throughput requirement — OPAL scales resources
proportionally to meet the throughput target.

---

## Cost Constraints

```yaml
cost:
  monthlyCeiling: "400"
  currency: USD
```

OPAL uses cloud pricing estimates to cap the maximum replica count so that
the monthly cost ceiling is never exceeded. This prevents autoscaling from
creating runaway cloud bills.

The calculation:
```
cost_per_replica_hour = (cpu_cores × $0.048) + (memory_gb × $0.006)
max_replicas = floor(monthly_ceiling / (24 × 30 × cost_per_replica_hour))
```

---

## Compliance

```yaml
compliance:
  - PCI-DSS
  - SOC2
```

When compliance requirements are declared, OPAL adds additional constraints:
- **PCI-DSS**: NetworkPolicy isolation, no privilege escalation, resource limits enforced
- **SOC2**: Audit logging labels, resource isolation, RBAC annotations
- **GDPR**: Data locality labels, network isolation

---

## AutoApply and Approval

`autoApply: false` (default) — OPAL calculates derived configuration and shows
it in `.status`, but does not apply it. Use this to review OPAL's reasoning.

`autoApply: true` — OPAL applies configuration immediately. Recommended only
after you have reviewed OPAL's reasoning for the workload.

For production workloads, use an `OutcomeBinding` with a `changeWindow` to
control when OPAL applies changes, regardless of `autoApply`.

---

## Status Fields

OPAL writes all of its reasoning into `WorkloadOutcome.status`:

```yaml
status:
  phase: Active
  lastReconciledAt: "2025-12-20T14:00:00Z"
  currentCostEstimate: "87.50"
  agentDecisions:
    - agentName: outcome-controller
      action: "derived config for production/checkout"
      reasoning: "3 reasoning steps applied"
      timestamp: "2025-12-20T14:00:00Z"
      applied: true
  sloViolations: []
```

---

## Full Example

```yaml
apiVersion: opal.io/v1alpha1
kind: WorkloadOutcome
metadata:
  name: payment-service
  namespace: production
  labels:
    team: payments
    domain: transactions
spec:
  targetRef:
    kind: Deployment
    name: payment-service
    namespace: production
  slo:
    latencyP99Ms: 50        # Strict: < 50ms
    latencyP95Ms: 30
    availability: "99.99"   # Very high: 5 replicas minimum
    errorRatePercent: "0.01"
    throughputRPS: 1000
  cost:
    monthlyCeiling: "800"
    currency: USD
  compliance:
    - PCI-DSS
    - SOC2
  horizon: 72h              # Predict 3 days ahead
  autoApply: true
```
