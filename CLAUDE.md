# CLAUDE.md

## Project

Playwright Prometheus Exporter — a single Go binary that runs Playwright test suites on independent schedules and exposes results as Prometheus metrics for synthetic monitoring.

## Build & Test Commands

```bash
make build          # compile binary to ./playwright-exporter
make test           # go test -race ./...
make lint           # golangci-lint run
make fmt            # gofmt + goimports
make vet            # go vet ./...
make check          # fmt + vet + lint + test (pre-commit gate)
make clean          # remove binary
```

Run a single test file: `go test -race -run TestConfigValidation ./...`

## Architecture

- **Single binary, single systemd service.** Replaces a previous three-service stack (playwright oneshot + nginx + json-exporter).
- **Multiple independent schedule goroutines**, each with its own ticker, timeout, and "already running" atomic flag. Schedules never block each other.
- **Shared metrics cache** protected by `sync.RWMutex`. Schedule goroutines write-lock, the HTTP `/metrics` handler read-locks.
- **Subprocess execution** via `exec.CommandContext`. Playwright is invoked as `npx playwright test --reporter=json` in the schedule's configured directory. Stdout is captured and parsed as JSON. Stderr is captured for error logging.
- **Playwright exits non-zero when tests fail but still produces valid JSON.** A test failure is NOT an exporter failure. Only parse/exec errors set `playwright_up=0`.

## Key Design Decisions

- **Directory = schedule.** Each schedule maps to one Playwright test directory. One `npx playwright test` invocation per directory per tick.
- **No shared login/session state between schedules.** Each schedule's tests handle their own auth independently. This is intentional for failure isolation.
- **Self-describing staleness.** The exporter exposes `playwright_schedule_interval_seconds` so Prometheus alert rules can compute staleness thresholds dynamically without hardcoded values.
- **Gauges, not histograms.** Tests run once per interval, not sampled from a distribution.
- **Timeout < interval enforced at startup.** If omitted, timeout defaults to 80% of interval.
- **Two-step install.** The deb/rpm package handles binary, config, systemd unit, user creation, and directories. `playwright-exporter-setup` is a separate manual step that installs Chromium and initializes test directories. This split exists because dpkg holds a lock during postinstall, preventing nested package manager calls.

## Code Layout

| File | Responsibility |
|---|---|
| `config.go` | YAML parsing, validation, defaults |
| `executor.go` | Subprocess exec, JSON parsing, result flattening |
| `scheduler.go` | Per-schedule goroutine, ticker, atomic skip flag, context cancellation |
| `metrics.go` | Prometheus metric definitions, shared cache, update logic |
| `main.go` | Wiring: config → metrics → schedulers → HTTP server → signal handling |

## Conventions

- Go 1.22+. Use `log/slog` for structured logging.
- Dependencies: `prometheus/client_golang`, `gopkg.in/yaml.v3`. No unnecessary third-party libraries.
- All exported functions and types must have doc comments.
- No global mutable state outside the Prometheus registry.
- Test data lives in `testdata/`. Unit tests mock command output rather than executing real subprocesses.
- The `G204` gosec exclusion is intentional — subprocess execution is core functionality.

## Metrics Prefix

All metrics use the `playwright_` prefix. Reserved label names that must not appear in user-defined schedule labels: `schedule`, `test`, `step`.

## CI/CD

Three workflow files:

- `ci.yml` — lint, test (60% coverage gate), cross-compile check, govulncheck, shellcheck, actionlint. Runs on every push/PR.
- `security.yml` — gosec, Trivy, CodeQL, dependency review, OpenSSF Scorecard. Runs on PRs + weekly schedule.
- `release.yml` — GoReleaser builds + packages, post-release Trivy scan, SHA256 checksums. Runs on version tags.

All SARIF results upload to GitHub Security tab. G204 is excluded from gosec (subprocess exec is core functionality).
- Dependabot monitors Go modules and GitHub Actions weekly.
- Release artifacts are signed with Sigstore cosign (keyless, via Fulcio CA).
- Docker images are published to ghcr.io on every release.
- Fuzz tests exist for config parsing and JSON report parsing. Run `make fuzz` locally.
