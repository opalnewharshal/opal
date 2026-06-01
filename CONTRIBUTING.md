# Contributing to OPAL

Thank you for your interest in contributing to OPAL. This document provides
guidelines for contributing to the project.

## Prerequisites

- Go 1.22+
- kubectl 1.28+
- Kubernetes cluster (kind or minikube for local development)
- kagent installed in your cluster ([kagent quickstart](https://kagent.dev/docs))
- Docker (for building images)

## Development Setup

```bash
# Clone the repository
git clone https://github.com/opal-io/opal.git
cd opal

# Install dependencies
go mod download

# Install CRDs into your cluster
make install

# Run the controller locally
make run
```

## Project Structure

```
opal/
├── api/v1alpha1/          # CRD type definitions
├── internal/
│   ├── controller/        # Kubernetes reconcilers
│   ├── memory/            # ClusterMemory temporal store
│   ├── forecast/          # Predictive modeling engine
│   ├── outcome/           # Outcome-to-config translator
│   └── metrics/           # OTel metrics collection
├── agents/                # kagent agent YAML specs
├── config/                # Kustomize manifests
├── charts/opal/           # Helm chart
└── docs/                  # Documentation
```

## Making Changes

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes
4. Add tests for new functionality
5. Run tests: `make test`
6. Run linting: `make lint`
7. Commit with a clear message following [Conventional Commits](https://www.conventionalcommits.org/)
8. Open a pull request

## Pull Request Guidelines

- PRs must have a clear description of the change and why it is needed
- All CI checks must pass
- At least one maintainer approval is required
- Breaking changes must be documented

## Running Tests

```bash
# Unit tests
make test

# Integration tests (requires a running cluster)
make test-integration

# End-to-end tests
make test-e2e
```

## Adding a New kagent Agent

1. Create a new YAML file in `agents/`
2. Define the `Agent` CRD spec with appropriate system prompt and tools
3. Register the agent in `agents/kustomization.yaml`
4. Document the agent in `docs/concepts/`

## Reporting Issues

Please use the GitHub issue tracker. For security vulnerabilities, see
[SECURITY.md](SECURITY.md).

## Community

- CNCF Slack: `#opal` channel
- Mailing list: cncf-opal-dev@lists.cncf.io
- Community meetings: bi-weekly (details in Slack)
