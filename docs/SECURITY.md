# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| latest  | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. Do NOT open a public GitHub issue.
2. Email security concerns to [me@maravexa.com].
3. Include steps to reproduce and impact assessment.
4. Allow 90 days for a fix before public disclosure.

## Security Scanning

This project runs automated security scans on every PR and weekly:

- **gosec** — Go static security analysis
- **Trivy** — Dependency and filesystem vulnerability scanning
- **CodeQL** — Semantic code analysis
- **OpenSSF Scorecard** — Repository security posture
- **Dependency Review** — New dependency vetting on PRs

Results are available in the GitHub Security tab.

## Supply Chain

- Dependencies are kept minimal: `prometheus/client_golang` and `gopkg.in/yaml.v3`
- `go.sum` provides integrity verification for all dependencies
- Release artifacts include SHA256 checksums
- License compliance is enforced (no AGPL/GPL dependencies)
