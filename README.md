# OPAL — Outcome-Predictive Agent Layer

[![Open Source](https://img.shields.io/badge/Open%20Source-Apache%202.0-green)](LICENSE)
[![Built on kagent](https://img.shields.io/badge/Built%20on-kagent%20CNCF-blue)](https://kagent.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/opalnewharshal/opal)](https://goreportcard.com/report/github.com/opalnewharshal/opal)
[![CI](https://github.com/opalnewharshal/opal/actions/workflows/ci.yaml/badge.svg)](https://github.com/opalnewharshal/opal/actions/workflows/ci.yaml)
[![GitHub Stars](https://img.shields.io/github/stars/opalnewharshal/opal?style=social)](https://github.com/opalnewharshal/opal)

**OPAL** is a Kubernetes-native control plane extension that replaces manual
YAML configuration with **declared business outcomes**, and replaces reactive
reconciliation with **predictive pre-reconciliation** — powered by a
multi-agent AI control loop built on [kagent](https://kagent.dev).

> You declare *what you want*. OPAL figures out *how to run it* — today,
> tomorrow, and before problems happen.

---

## The Problem

Kubernetes requires engineers to specify *how* to run workloads:

```yaml
replicas: 3
resources:
  requests:
    cpu: 500m
    memory: 512Mi
```

This means:
- Deep Kubernetes expertise required for every service
- Reactive scaling: problems fixed *after* they occur
- No memory: the cluster forgets everything every restart
- Config drift: YAML diverges from business reality over time

---

## The Solution

OPAL introduces **three new Kubernetes primitives**:

### 1. WorkloadOutcome — Declare Goals, Not Config

```yaml
apiVersion: opal.io/v1alpha1
kind: WorkloadOutcome
metadata:
  name: checkout-service
  namespace: production
spec:
  targetRef:
    kind: Deployment
    name: checkout
  slo:
    latencyP99Ms: 80
    availability: "99.95"
    errorRatePercent: "0.1"
  cost:
    monthlyCeiling: "400"
    currency: USD
  compliance:
    - PCI-DSS
  horizon: 72h
  autoApply: true
```

### 2. ClusterForecast — Predict Future State

```yaml
apiVersion: opal.io/v1alpha1
kind: ClusterForecast
metadata:
  name: cluster-forecast
  namespace: opal-system
status:
  forecasts:
    - workloadRef: production/checkout
      predictedEvents:
        - type: CPUSpike
          predictedAt: "2025-12-20T20:00:00Z"
          confidence: 0.87
      recommendedActions:
        - pre-scale-replicas-to-8
        - increase-cpu-limit-to-2000m
```

### 3. ClusterMemory — Temporal Intelligence

OPAL maintains an episodic memory of your cluster — every pattern, every
incident, every seasonal trend. The longer OPAL runs, the smarter it gets.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        OPAL Control Plane                         │
│                                                                    │
│  ┌─────────────────┐   A2A    ┌──────────────────────────────┐   │
│  │  ForecastAgent  │ ──────►  │      ReconcilerAgent          │   │
│  │  (kagent)       │          │      (kagent)                 │   │
│  │                 │          │                               │   │
│  │  Reads:         │          │  Pre-acts on predictions      │   │
│  │  - Metrics      │          │  before drift occurs          │   │
│  │  - Events       │          └──────────────┬───────────────┘   │
│  │  - ClusterMemory│                          │ A2A               │
│  │                 │          ┌──────────────▼───────────────┐   │
│  │  Writes:        │          │      OutcomeAgent             │   │
│  │  ClusterForecast│          │      (kagent)                 │   │
│  └─────────────────┘          │                               │   │
│                                │  Translates WorkloadOutcome  │   │
│  ┌─────────────────┐           │  → k8s config, validates     │   │
│  │   AuditAgent    │◄── A2A ───│  every change against SLOs   │   │
│  │   (kagent)      │           └──────────────────────────────┘   │
│  │                 │                                               │
│  │  Every decision │   ┌──────────────────────────────────────┐   │
│  │  → K8s Event    │   │         ClusterMemory Store           │   │
│  └─────────────────┘   │  Episodic + Semantic temporal memory  │   │
│                          │  Queried by all agents               │   │
│                          └──────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
         │                              │
         ▼                              ▼
  WorkloadOutcome CRDs          Generated k8s manifests
  (your declared goals)         (owned and maintained by OPAL)
```

OPAL is built **on top of [kagent](https://kagent.dev)** (CNCF Sandbox),
using kagent's agent runtime, A2A protocol, and Kubernetes-native tooling
as its foundation.

---

## How OPAL Differs from Everything Else

### vs. Intel Intent-Driven Orchestration (IDO)

[Intel IDO](https://github.com/intel/intent-driven-orchestration) is the closest existing project —
it also uses SLO declarations to drive Kubernetes decisions. Here is the precise difference:

| | Intel IDO | OPAL |
|---|---|---|
| What you declare | SLO targets | SLO + cost ceiling + compliance |
| What it generates | Scaling decisions only | Full k8s config: resources, replicas, HPA, PDB, NetworkPolicy |
| Predictive | No — reactive only | Yes — acts before problems occur |
| Cluster memory | No | Yes — learns patterns over time |
| AI agents | No | Yes — 4 kagent agents as the control plane |
| Built on CNCF project | No | Yes — built on kagent (CNCF Sandbox) |
| Open source activity | Limited community | Actively building community |

Intel IDO proves the idea is valid. OPAL goes significantly further.

### vs. Keptn (CNCF — Archived September 2025)

Keptn used SLOs for delivery pipeline quality gates. It was archived in September 2025.
OPAL fills the gap Keptn leaves: continuous, runtime outcome management — not just deployment gates.

### vs. KEDA / Karpenter / Flagger

These tools each solve **one dimension** reactively:

| Tool | What it solves | What it misses |
|---|---|---|
| KEDA | Scale replicas on events | Doesn't derive config from outcomes; reactive only |
| Karpenter | Right-size nodes | Node provisioning only; no SLO awareness |
| Flagger | Safe progressive delivery | Deployment strategy only; no predictive capability |

OPAL is not a replacement for these — it sits above them, using outcomes to drive their configuration.

### vs. k8sgpt / Robusta

These are **reactive AI troubleshooting tools** — they help you fix problems after they occur.
OPAL predicts problems before they occur and prevents them. Fundamentally different.

### The Capability No One Else Has

| Capability | Intel IDO | Cast AI | k8sgpt | Keptn | Karpenter | **OPAL** |
|---|---|---|---|---|---|---|
| Outcomes → full k8s config | Partial | No | No | No | No | **Yes** |
| Predict future cluster state | No | Partial | No | No | No | **Yes** |
| Temporal cluster memory | No | No | No | No | No | **Yes — first ever** |
| AI agents as control plane | No | No | No | No | No | **Yes (via kagent)** |
| CNCF native | No | No | Sandbox | Archived | Sandbox | **Building** |
| Pre-emptive action | No | Limited | No | No | No | **Yes** |

---

## Getting Started

### Prerequisites

- Kubernetes 1.28+
- [kagent](https://kagent.dev/docs/kagent/introduction/installation) installed
- Helm 3.x

### Install with Helm

```bash
helm repo add opal https://charts.opal.io
helm repo update

helm install opal opal/opal \
  --namespace opal-system \
  --create-namespace \
  --set llm.provider=anthropic \
  --set llm.apiKeySecret=anthropic-key
```

### Install with kubectl

```bash
# Install CRDs
kubectl apply -f https://github.com/opalnewharshal/opal/releases/latest/download/crds.yaml

# Install controller
kubectl apply -f https://github.com/opalnewharshal/opal/releases/latest/download/opal.yaml

# Install agents
kubectl apply -f https://github.com/opalnewharshal/opal/releases/latest/download/agents.yaml
```

### Deploy Your First WorkloadOutcome

```bash
kubectl apply -f config/samples/checkout-outcome.yaml
```

Watch OPAL derive configuration:

```bash
kubectl get workloadoutcome checkout-service -n production -w
kubectl get events -n production --field-selector reason=OPALOutcomeApplied
```

---

## Documentation

- [Getting Started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [WorkloadOutcome Concept](docs/concepts/workload-outcomes.md)
- [ClusterForecast Concept](docs/concepts/cluster-forecast.md)
- [ClusterMemory Concept](docs/concepts/memory-store.md)
- [Contributing Guide](CONTRIBUTING.md)

---

## Community

- **GitHub Discussions**: [Start a discussion](https://github.com/opalnewharshal/opal/discussions)
- **GitHub Issues**: [Report a bug or request a feature](https://github.com/opalnewharshal/opal/issues)
- **kagent Community**: [Join the conversation](https://github.com/kagent-dev/kagent/discussions/1949) — OPAL is built on kagent

---

## CNCF Ecosystem

OPAL is built on [kagent](https://kagent.dev) — a CNCF Sandbox project.
We are actively building community and plan to apply to CNCF once we have
demonstrated adoption. Contributions and feedback are very welcome.

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
