# OPAL Architecture

## Overview

OPAL (Outcome-Predictive Agent Layer) extends the Kubernetes control plane with
two fundamental capabilities that have never existed before:

1. **Outcome-Native API**: `WorkloadOutcome` as a first-class Kubernetes resource —
   declare what you want, not how to run it
2. **Predictive Reconciliation**: act before drift occurs, not after

OPAL is built on [kagent](https://kagent.dev) (CNCF Sandbox), using its agent
runtime, A2A protocol, and Kubernetes-native tooling as its foundation.

---

## Core Components

### 1. OPAL Controller Manager

A standard Kubernetes controller manager (built with controller-runtime) that
hosts three reconcilers:

| Reconciler | Watches | Does |
|---|---|---|
| WorkloadOutcomeReconciler | WorkloadOutcome CRDs | Calls Translator, applies derived config |
| ClusterForecastReconciler | ClusterForecast CRDs | Calls Predictor, updates forecast status |
| OutcomeBindingReconciler | OutcomeBinding CRDs | Enforces approval policy, manages rollbacks |

### 2. ClusterMemory Store

An in-process temporal event store that gives OPAL episodic memory.

Unlike etcd (point-in-time state) or Prometheus (raw metrics), ClusterMemory
stores semantically-labelled events that agents query by workload, time range,
and behavioural pattern.

Key capabilities:
- `Record(entry)` — store any cluster event with labels and narrative
- `Query(filter)` — retrieve by workload, time range, type, labels
- `SameTimeLast(workload, t, window, weeks)` — find same-time-last-week patterns
- `RecordPattern / GetPatterns` — store identified recurring behaviours
- `Snapshot / Restore` — persistence to ConfigMap or PVC

### 3. Statistical Forecaster

A deterministic predictor that analyses ClusterMemory entries using:
- Linear regression on CPU/memory time series
- Seasonal pattern detection (same-time-last-week)
- SLO violation history analysis

Produces `WorkloadForecast` objects with confidence-scored `PredictedEvent` lists.

### 4. Outcome Translator

Maps `WorkloadOutcomeSpec` (SLOs + cost + compliance) to concrete Kubernetes
resources:
- Resource requests/limits (derived from latency SLO profile)
- Min/max replicas (derived from availability SLO and cost ceiling)
- HorizontalPodAutoscaler spec
- PodDisruptionBudget spec

---

## The kagent Multi-Agent Control Loop

Four kagent agents form the AI control plane, communicating via A2A:

```
┌─────────────────────────────────────────────────────────────┐
│                    kagent A2A Control Loop                   │
│                                                              │
│  ┌────────────────┐          ┌──────────────────────────┐   │
│  │ ForecastAgent  │──A2A──►  │    ReconcilerAgent        │   │
│  │                │          │                          │   │
│  │ Runs: every    │          │ Receives predictions,    │   │
│  │ 15 minutes     │          │ schedules pre-emptive    │   │
│  │                │          │ actions 30-60min ahead   │   │
│  │ Reads:         │          └──────────┬───────────────┘   │
│  │ - k8s metrics  │                     │ A2A               │
│  │ - ClusterMemory│          ┌──────────▼───────────────┐   │
│  │                │          │    OutcomeAgent           │   │
│  │ Writes:        │          │                          │   │
│  │ ClusterForecast│          │ Validates actions against │   │
│  └────────────────┘          │ WorkloadOutcome SLOs and  │   │
│                               │ cost constraints          │   │
│  ┌────────────────┐           └──────────┬───────────────┘   │
│  │  AuditAgent    │◄──────────────A2A────┘                   │
│  │                │                                           │
│  │ Converts every │                                           │
│  │ decision into  │                                           │
│  │ a k8s Event    │                                           │
│  └────────────────┘                                           │
└─────────────────────────────────────────────────────────────┘
```

### Agent Responsibilities

| Agent | Trigger | Primary Output |
|---|---|---|
| ForecastAgent | Every 15 minutes | ClusterForecast CRD updates |
| ReconcilerAgent | A2A from ForecastAgent | Pre-emptive kubectl patches |
| OutcomeAgent | WorkloadOutcome changes | Validated DerivedConfig |
| AuditAgent | A2A from all agents | Kubernetes Events + ClusterMemory entries |

---

## Data Flow

### Outcome Management (steady state)

```
User creates WorkloadOutcome
        │
        ▼
WorkloadOutcomeReconciler
        │
        ├── calls Translator.Translate(outcome)
        │         │
        │         └── returns DerivedConfig
        │                 (resources, replicas, HPA, PDB)
        │
        ├── if autoApply=true: patches Deployment, creates HPA/PDB
        │
        ├── records decision in ClusterMemory
        │
        └── updates WorkloadOutcome.status (phase, reasoning)
```

### Predictive Loop (every 15 minutes)

```
ClusterForecastReconciler triggers
        │
        ├── lists all WorkloadOutcomes
        │
        ├── for each workload:
        │     └── StatisticalPredictor.Predict(workloadRef, horizon)
        │               │
        │               ├── queries ClusterMemory for historical data
        │               ├── runs linear trend analysis
        │               ├── detects seasonal patterns
        │               └── returns WorkloadForecast with PredictedEvents
        │
        ├── updates ClusterForecast.status
        │
        └── ForecastAgent (kagent) picks up updated forecast
                  │
                  └── A2A → ReconcilerAgent
                                │
                                └── pre-emptive action if confidence ≥ 0.7
```

---

## CRD Design Principles

### WorkloadOutcome

The `WorkloadOutcome` CRD is the primary user-facing API. It is deliberately
**outcome-oriented**, not resource-oriented:

```yaml
spec:
  slo:           # WHAT you want (goals)
    latencyP99Ms: 80
    availability: "99.95"
  cost:          # CONSTRAINT (ceiling)
    monthlyCeiling: "400"
  compliance:    # POLICY (rules)
    - PCI-DSS
```

OPAL owns `status` entirely — users never write to it.

### ClusterForecast

The `ClusterForecast` is a cluster-wide singleton (per namespace) that holds
the latest AI predictions. Its status is updated by the ForecastAgent every
`refreshInterval`. It is the first Kubernetes resource type designed to hold
**future state** predictions.

### OutcomeBinding

`OutcomeBinding` is the approval and governance layer. It decouples *what OPAL
wants to do* from *when and how it is allowed to do it*.

---

## Security Model

OPAL follows least-privilege:

- Controller reads cluster-wide, writes only in namespaces with WorkloadOutcomes
- Agent API keys stored in Kubernetes Secrets, never in CRD specs
- All agent decisions emitted as Kubernetes Events — fully auditable
- `autoApply: false` by default — operators must opt into automation

See [SECURITY.md](../SECURITY.md) for the full threat model.
