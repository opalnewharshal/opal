---
name: Bug Report
about: Report a bug in OPAL
title: '[BUG] '
labels: 'kind/bug'
assignees: ''
---

## Describe the Bug

A clear description of what the bug is.

## To Reproduce

Steps to reproduce:
1. Apply WorkloadOutcome with spec: ...
2. Wait for reconciliation
3. Observe ...

## Expected Behaviour

What you expected to happen.

## Actual Behaviour

What actually happened.

## Environment

- OPAL version: (e.g., v0.1.0)
- Kubernetes version: (e.g., v1.29.0)
- kagent version: (e.g., v0.3.0)
- LLM provider: (e.g., anthropic claude-sonnet-4-6)
- Cloud provider / local: (e.g., kind, EKS, GKE)

## Logs

```
# kubectl logs -n opal-system deploy/opal-controller-manager
```

## WorkloadOutcome YAML (if relevant)

```yaml
```
