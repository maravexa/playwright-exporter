# Contributing to playwright-exporter

Thank you for your interest in contributing. This document covers the development workflow, conventions, and CI requirements for this project.

For architectural background and design decisions, see [CLAUDE.md](CLAUDE.md). For security vulnerability reporting, see [docs/SECURITY.md](docs/SECURITY.md).

---

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Code Conventions](#code-conventions)
- [Testing](#testing)
- [Commit Requirements](#commit-requirements)
- [Pull Request Process](#pull-request-process)
- [CI Overview](#ci-overview)

---

## Getting Started

### Prerequisites

- Go 1.22 or later
- `golangci-lint` ([install instructions](https://golangci-lint.run/welcome/install/))
- `goimports` (`go install golang.org/x/tools/cmd/goimports@latest`)
- Node.js v18+ and Playwright (only needed to run real tests against a live install — not required for unit tests)

### Fork and clone

```bash
git clone https://github.com/<your-fork>/playwright-exporter.git
cd playwright-exporter
go mod download
```

### Verify your setup

```bash
make check
```

This runs formatting, `go vet`, `golangci-lint`, and the full test suite with the race detector. It must pass cleanly before any commit.

---

## Development Workflow

| Command | What it does |
|---|---|
| `make build` | Compile binary to `./playwright-exporter` |
| `make test` | `go test -race ./...` |
| `make lint` | `golangci-lint run` |
| `make fmt` | `gofmt` + `goimports` |
| `make vet` | `go vet ./...` |
| `make check` | `fmt` + `vet` + `lint` + `test` (pre-commit gate) |
| `make fuzz` | Run fuzz tests for config and JSON parsing |
| `make clean` | Remove compiled binary |

Run a single test: `go test -race -run TestName ./...`

Always run `make check` before pushing. CI enforces the same checks and will fail if they do not pass.

---

## Code Conventions

These conventions are enforced by CI and code review.

**Go version:** 1.22+.

**Logging:** Use `log/slog` with structured key-value pairs. Do not use `fmt.Print*` or `log.Print*`.

**Dependencies:** Keep the dependency list minimal. This project intentionally uses only `prometheus/client_golang` and `gopkg.in/yaml.v3`. New third-party dependencies require justification.

**Global state:** No global mutable state outside the Prometheus registry.

**Doc comments:** All exported functions, types, and methods must have doc comments.

**Comments:** Default to writing no comments. Only add one when the *why* is non-obvious — a hidden constraint, a subtle invariant, or a workaround for a specific bug. Do not explain what the code does; well-named identifiers do that.

**Metrics:** All metrics use the `playwright_` prefix. The label names `schedule`, `test`, and `step` are reserved and must not appear in user-defined schedule labels.

**Security linting:** `gosec` runs in CI. `G204` (subprocess exec) and `G304` (file path from config) are intentionally excluded — subprocess execution and config-driven paths are core functionality. Do not add new blanket exclusions without discussion.

**Error handling:** Return errors; do not swallow them. A test failure returned by Playwright is not an exporter error — only exec/parse failures set `playwright_up=0`. See [CLAUDE.md](CLAUDE.md) for the full rationale.

---

## Testing

### Unit tests

Unit tests live alongside source files and mock subprocess output rather than executing real Playwright. Test fixtures live in `testdata/`.

```bash
make test          # all tests with race detector
go test -race -run TestConfigValidation ./...   # single test
```

### Coverage gate

CI enforces a **60% line coverage minimum**. Dropping below this threshold fails the build. When adding new functionality, include tests that cover the new code paths.

### Fuzz tests

Fuzz tests exist for config parsing and Playwright JSON report parsing. Run them locally before submitting changes to either area:

```bash
make fuzz
```

### What not to test

Do not write tests that spin up real Playwright or make network calls. Mock command output using the patterns already present in `executor_test.go` and `scheduler_test.go`.

---

## Commit Requirements

**Signed commits are required.** All commits to `main` must be GPG- or SSH-signed.

Configure signing globally:

```bash
git config --global commit.gpgsign true
git config --global user.signingkey <your-key-id>
```

Or per-repository:

```bash
git config commit.gpgsign true
```

**Commit messages:** Use the imperative mood in the subject line (e.g. `Add timeout validation for schedules`, not `Added timeout validation`). Keep the subject under 72 characters. Use the body to explain *why* when the change is non-trivial.

---

## Pull Request Process

1. **Fork** the repository and create a feature branch from `main`.
2. **Run `make check`** and ensure it passes cleanly before pushing.
3. **Write or update tests.** Coverage must not drop below 60%.
4. **Open a PR** against `main` with a clear description of the change and the problem it solves.
5. **One approval** from a maintainer is required before merge.
6. **CI must pass** — all lint, test, security, and build checks must be green.
7. **Security issues** must not be disclosed publicly in a PR. See [docs/SECURITY.md](docs/SECURITY.md) for the responsible disclosure process.

Maintainers may ask for changes. Address review feedback in new commits; do not force-push a branch under active review unless asked.

---

## CI Overview

Three workflow files run automatically:

**`ci.yml`** — Runs on every push and pull request.
- `lint`: golangci-lint
- `test`: `go test -race` with 60% coverage gate
- `build`: Cross-compile for `linux/amd64` and `linux/arm64`
- `govulncheck`: Go vulnerability database check
- `shellcheck`: Lint scripts in `scripts/`
- `actionlint`: Validate GitHub Actions workflow files

**`security.yml`** — Runs on pull requests and weekly.
- `gosec`: Static security analysis (G204 and G304 excluded)
- `trivy_fs`: Filesystem vulnerability scan
- `codeql`: Semantic code analysis
- `dependency_review`: Blocks high-severity vulnerabilities and AGPL/GPL-licensed dependencies
- `scorecard`: OpenSSF Scorecard (weekly only)

All SARIF results upload to the GitHub Security tab.

**`release.yml`** — Runs on version tags (`v*`). Builds deb/rpm packages, Docker images, SPDX SBOM, SHA256 checksums, and Sigstore keyless signatures. You do not need to interact with this workflow during normal development.
