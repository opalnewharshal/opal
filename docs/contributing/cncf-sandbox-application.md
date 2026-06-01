# OPAL CNCF Sandbox Application

> This document is the draft submission for the CNCF Sandbox.
> Submit at: https://github.com/cncf/sandbox/issues/new?template=sandbox-application.md

---

## Application

**Project Name:** OPAL — Outcome-Predictive Agent Layer

**Project Repo:** https://github.com/HarshalSant/opal
*(Transfer target: https://github.com/opal-io/opal after org creation)*

**Website:** https://opal.io *(planned)*

**License:** Apache 2.0

**Preferred Maturity Level:** Sandbox

---

## Project Description

OPAL (Outcome-Predictive Agent Layer) is a Kubernetes-native control plane
extension that introduces two capabilities that have never existed in the
cloud native ecosystem:

**1. Outcome-Native Kubernetes API**

Today, every Kubernetes operator must specify *how* to run workloads — exact
replica counts, CPU requests, memory limits, HPA thresholds. This requires
deep Kubernetes expertise and creates configuration drift as business
requirements evolve. OPAL introduces `WorkloadOutcome` as a first-class
Kubernetes CRD, allowing operators to declare *what they want to achieve*:

```yaml
apiVersion: opal.io/v1alpha1
kind: WorkloadOutcome
spec:
  slo:
    latencyP99Ms: 80
    availability: "99.95"
  cost:
    monthlyCeiling: "400"
  compliance: [PCI-DSS]
```

OPAL derives, owns, and continuously maintains all Kubernetes configuration
required to achieve the declared outcomes.

**2. Predictive Reconciliation**

Every Kubernetes controller today is reactive: it detects drift *after* it
occurs and corrects it. OPAL introduces `ClusterForecast` — the first
Kubernetes resource designed to hold *future* state predictions — and a
multi-agent control loop that acts *before* drift occurs.

OPAL is built on [kagent](https://kagent.dev) (CNCF Sandbox), using its
agent runtime, A2A protocol, and Kubernetes-native tooling as the foundation.

---

## Alignment with CNCF Mission

OPAL directly advances the CNCF mission to *make cloud native computing
ubiquitous* by:

- **Lowering the Kubernetes expertise barrier**: Operators declare outcomes in
  business language, not infrastructure language. This opens cloud native
  technology to teams without deep Kubernetes knowledge.

- **Advancing the AI-native cloud native ecosystem**: OPAL is the first project
  to use AI agents as the Kubernetes control plane itself (not as an external
  tool), extending the CNCF AI/ML landscape.

- **Building on existing CNCF projects**: OPAL is built on kagent (CNCF Sandbox)
  and uses controller-runtime, the same framework used by the majority of CNCF
  operator projects. This demonstrates ecosystem reuse and composability.

- **Introducing new primitives for the ecosystem**: `WorkloadOutcome`,
  `ClusterForecast`, and `ClusterMemory` are net-new Kubernetes primitives that
  other CNCF projects can build on.

---

## Relationship to kagent

OPAL and kagent are complementary, not competing:

| | kagent | OPAL |
|---|---|---|
| **Purpose** | Framework for running AI agents in Kubernetes | AI agents as the Kubernetes control plane |
| **User** | Platform engineers building custom agents | Platform engineers managing workload outcomes |
| **Agents trigger** | Human operators | Autonomously, on a continuous schedule |
| **Primary output** | Whatever the user asks for | `WorkloadOutcome` reconciliation + `ClusterForecast` |

OPAL ships kagent as a Helm dependency and all four OPAL agents are standard
kagent `Agent` CRDs. We intend to maintain active collaboration with the
kagent maintainers.

---

## Sponsors

*We are actively seeking CNCF TOC sponsors. Natural candidates include:*

- kagent maintainers (Solo.io) — OPAL builds directly on kagent
- KubeEdge / Karmada maintainers — related control-plane extension work
- Any TOC member with interest in AI-native cloud native infrastructure

*We will reach out to the kagent community before formal submission.*

---

## Maintainers

| Name | GitHub | Affiliation |
|---|---|---|
| HarshalSant | @HarshalSant | Independent / OPAL Author |

*We are actively seeking additional maintainers from the community.*

---

## Infrastructure Requests

- GitHub repository under `cncf` or new `opal-io` org (or move existing repo)
- Artifact Hub listing
- CNCF Slack channel: `#opal`
- Mailing list: `cncf-opal-dev@lists.cncf.io`
- Zoom/calendar entry for bi-weekly community meetings

---

## External Dependencies

All dependencies are Apache 2.0 or compatible:

| Dependency | License | Usage |
|---|---|---|
| sigs.k8s.io/controller-runtime | Apache 2.0 | Controller framework |
| k8s.io/api, apimachinery, client-go | Apache 2.0 | Kubernetes types and client |
| kagent (kagent.dev/v1alpha1) | Apache 2.0 | Agent runtime |
| prometheus/client_golang | Apache 2.0 | Metrics |

---

## Communication Channels

- **GitHub Issues**: https://github.com/HarshalSant/opal/issues
- **GitHub Discussions**: https://github.com/HarshalSant/opal/discussions
- **CNCF Slack (planned)**: `#opal`
- **Mailing list (planned)**: cncf-opal-dev@lists.cncf.io

---

## Release Methodology

- Semantic versioning (v0.1.0, v0.2.0, ...)
- Releases on GitHub with CHANGELOG
- Container images published to `ghcr.io/opal-io/opal`
- Helm chart published to `charts.opal.io`
- Release cadence: every 4-6 weeks during active development

---

## Statement on Security

OPAL follows the security practices described in [SECURITY.md](../../SECURITY.md).
Vulnerabilities are reported to `cncf-opal-security@lists.cncf.io` and
addressed within 48 hours.

---

## Why Now

The AI-native infrastructure moment is here. kagent's acceptance into CNCF
Sandbox (May 2025) with 1000+ stars in 100 days demonstrates the community's
appetite for AI-powered Kubernetes tooling. OPAL takes the natural next step:
not just running AI agents *in* Kubernetes, but making AI agents the Kubernetes
control plane itself.

No existing project occupies this space. OPAL creates a new category.

---

## Checklist Before Submission

- [x] Apache 2.0 LICENSE file present
- [x] GOVERNANCE.md present
- [x] CODE_OF_CONDUCT.md present (CNCF standard)
- [x] CONTRIBUTING.md present
- [x] SECURITY.md present
- [x] MAINTAINERS.md present
- [x] CI/CD pipeline (GitHub Actions)
- [x] Working code with unit tests
- [x] Helm chart for installation
- [x] Documentation (getting-started, architecture, concepts)
- [ ] Find at least one CNCF TOC sponsor
- [ ] Create `opal-io` GitHub org
- [ ] First community meeting scheduled
- [ ] Submit issue at github.com/cncf/sandbox
