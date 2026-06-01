# OPAL — Outcome-Predictive Agent Layer

[![CNCF Sandbox Candidate](https://img.shields.io/badge/CNCF-Sandbox%20Candidate-blue)](https://github.com/cncf/sandbox)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/HarshalSant/opal)](https://goreportcard.com/report/github.com/HarshalSant/opal)
[![CI](https://github.com/HarshalSant/opal/actions/workflows/ci.yaml/badge.svg)](https://github.com/HarshalSant/opal/actions/workflows/ci.yaml)
[![GitHub Stars](https://img.shields.io/github/stars/HarshalSant/opal?style=social)](https://github.com/HarshalSant/opal)

> **Current home:** [github.com/HarshalSant/opal](https://github.com/HarshalSant/opal)
> — will transfer to `github.com/opal-io/opal` once the `opal-io` org is established.

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

## Key Differentiators

| Capability | Traditional K8s | kagent alone | OPAL |
|---|---|---|---|
| Outcomes as CRDs | No | No | **Yes** |
| Predictive reconciliation | No | No | **Yes** |
| Autonomous persistent agents | No | Partial | **Yes** |
| Temporal cluster memory | No | No | **Yes** |
| Multi-agent control loop | No | A2A exists | **Yes — as control plane** |
| Human triggers required | Yes | Yes | **No** |
| AI owns k8s manifests | No | No | **Yes** |

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
kubectl apply -f https://github.com/opal-io/opal/releases/latest/download/crds.yaml

# Install controller
kubectl apply -f https://github.com/opal-io/opal/releases/latest/download/opal.yaml

# Install agents
kubectl apply -f https://github.com/opal-io/opal/releases/latest/download/agents.yaml
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

- **CNCF Slack**: [#opal](https://cloud-native.slack.com/archives/opal)
- **Mailing List**: cncf-opal-dev@lists.cncf.io
- **Community Meetings**: Bi-weekly — details in Slack
- **GitHub Discussions**: [Discussions](https://github.com/opal-io/opal/discussions)

---

## CNCF Membership

OPAL is a [CNCF Sandbox](https://www.cncf.io/projects/) project. The
[Cloud Native Computing Foundation](https://cncf.io) (CNCF) is part of
the Linux Foundation and provides support, oversight, and direction for
fast-growing, cloud native open source projects.

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
