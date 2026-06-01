# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

To report a security vulnerability, please email:
**cncf-opal-security@lists.cncf.io**

You should receive a response within 48 hours. If not, please follow up.

Please include:
- Type of issue (e.g., buffer overflow, privilege escalation, etc.)
- Full paths of source file(s) related to the issue
- Location of the affected source code (tag/branch/commit or direct URL)
- Any special configuration required to reproduce the issue
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue

## Security Considerations

OPAL runs as a Kubernetes controller with cluster-wide read access and
namespace-scoped write access. It communicates with external LLM APIs.
Consider the following:

- **LLM API Keys**: Store LLM provider credentials in Kubernetes Secrets,
  not in plain ConfigMaps or environment variables in pod specs.
- **RBAC**: Use the provided RBAC manifests which follow least-privilege.
- **Network**: Restrict egress to only LLM provider endpoints if possible.
- **Audit**: All agent decisions are emitted as Kubernetes Events for audit.
