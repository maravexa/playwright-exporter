# Playwright Exporter contributor guide

## Project

Playwright Prometheus Exporter — a single Go binary that runs Playwright test suites on independent schedules and exposes results as Prometheus metrics for synthetic monitoring.

## Build & Test Commands

```bash
make build          # compile binary to ./playwright-exporter
make test           # go test -race ./...
make test-runtime   # pure policy fixtures and static installer checks
make test-contracts # runtime, service and publication safety contracts
make lint           # golangci-lint run
make fmt            # rewrites source with gofmt + goimports
make vet            # go vet ./...
make check          # fmt + vet + lint + test; also rewrites source
make clean          # remove binary
```

Run tests whose names match the given pattern: `go test -race -run TestValidateConfig_ ./...`

## Architecture

- **Single binary, single systemd service.** Replaces a previous three-service stack (playwright oneshot + nginx + json-exporter).
- **Multiple independent schedule goroutines**, each with its own ticker, timeout, and "already running" atomic flag. Schedules never block each other.
- **Shared metrics cache** protected by `sync.RWMutex`. Schedule goroutines write-lock, the HTTP `/metrics` handler read-locks.
- **Subprocess execution** via `exec.CommandContext`. Playwright uses the schedule project's `node_modules/.bin/playwright`; the default is `./node_modules/.bin/playwright test --reporter=json`. Bare Playwright and legacy plain `npx playwright ...` normalize to that local binary without executing npx. Missing or invalid local dependencies fail with provisioning instructions. Npx options, package resolution and alternate Playwright paths are rejected; explicit custom runner commands remain operator-controlled, so this is not a command sandbox. Stdout is captured and parsed as JSON. Stderr is captured for error logging.
- **Playwright exits non-zero when tests fail but still produces valid JSON.** A test failure is NOT an exporter failure. Only parse/exec errors set `playwright_up=0`.

## Key Design Decisions

- **Directory = schedule.** Each schedule maps to one Playwright test directory. One project-local Playwright invocation per directory per tick.
- **No shared login/session state between schedules.** Each schedule's tests handle their own auth independently. This is intentional for failure isolation.
- **Self-describing staleness.** The exporter exposes `playwright_schedule_interval_seconds` so Prometheus alert rules can compute staleness thresholds dynamically without hardcoded values.
- **Gauges, not histograms.** Tests run once per interval, not sampled from a distribution.
- **Timeout < interval enforced at startup.** If omitted, timeout defaults to 80% of interval.
- **Two-step install.** The deb/rpm package handles binary, config, systemd unit, user creation, and directories. `playwright-exporter-setup` is a separate manual step that provisions the locked runtime and Chromium. It does not initialize, recursively change ownership of, or install dependencies into operator project trees. This split exists because dpkg holds a lock during postinstall, preventing nested package manager calls.

## Code Layout

| File | Responsibility |
|---|---|
| `config.go` | YAML parsing, validation, defaults |
| `executor.go` | Subprocess exec, JSON parsing, result flattening |
| `scheduler.go` | Per-schedule goroutine, ticker, atomic skip flag, context cancellation |
| `metrics.go` | Prometheus metric definitions, shared cache, update logic |
| `main.go` | Wiring: config → metrics → schedulers → HTTP server → signal handling |

## Conventions

- Use the Go version in `go.mod` (currently 1.26.11). Use `log/slog` for structured logging.
- Dependencies are declared in `go.mod`, including Prometheus client packages and YAML. No unnecessary third-party libraries.
- All exported functions and types must have doc comments.
- No global mutable state outside the Prometheus registry.
- Report-parsing tests use fixture data; command and runtime policy tests cover additional execution/installer contracts. Check each test before claiming real browser, package/service integration or supported-distribution acceptance.
- The security workflow excludes `G204` and `G304` from gosec; subprocess execution is core functionality. Review concrete path and command trust boundaries when changing either area.

## Metrics Prefix

All metrics use the `playwright_` prefix. Reserved label names that must not appear in user-defined schedule labels: `schedule`, `test`, `step`.

## Runtime and dependency boundaries

`runtime/package.json` and lockfile v3 currently pin Playwright 1.64.1. Host policy accepts maintained Node 22.x, 24.x or 26.x; the operator supplies maintained Node/npm and Chromium OS libraries. Setup does not add package sources, trust keys or OS packages. Keep the policy helper, manifest, lockfile, installer documentation and digest-pinned Docker base consistent when changing supported versions. The container uses the digest-pinned Playwright Resolute base, signed existing APT sources and npm 12.2.2; final image advisories and native Node security floors are separate release gates.

The host setup installs with `npm ci --engine-strict --ignore-scripts --no-audit --no-fund` into a staged runtime protected from service-user writes, then invokes the fixed CLI as the service user for browser download. Project dependencies remain operator-owned and must explicitly match the intended browser version. Preserve the no-network-fallback executor behavior, no lifecycle-script execution, no automatic project migration and no old-runtime cleanup. Lockfile package integrity is not independent browser-download attestation. See `docs/runtime-setup.md` for provisioning and validation limits.

## Validation and release boundaries

Runner cancellation sends an interrupt to the direct child, waits a bounded grace period, and bounds captured output and pipe draining. PID namespace isolation is opt-in and fails closed when unsupported; gated native process tests and real sandboxed browsers remain distinct from unit fixtures. A container Tini entrypoint reaps adopted processes. See `docs/service-lifecycle.md`.

Inspect `.github/workflows/` as source without triggering hosted workflows for private-repository work. Every hosted job is guarded to public repositories; source pushes use `[skip ci]`. `ci.yml` includes race/coverage (60%), cross-build, pinned govulncheck 1.9.0, shellcheck and actionlint checks. `security.yml` includes gosec with `-no-fail`, SARIF reporting, CodeQL, dependency review and Scorecard; presence or SARIF upload does not establish that findings block a merge or release.

`release.yml` prepares Linux amd64/arm64 assets and an SPDX SBOM before checksumming and Sigstore-signing the manifest, verifies the bundle, then publishes. See `docs/release-verification.md` and `scripts/prepare-release-assets.py`: source-tag signatures, asset checksums/signatures and container publication are separate evidence. The GHCR image is a subsequent amd64 job and is not authenticated by the package manifest. Do not overwrite existing releases or equate partial publication with a complete release.

Run affected local checks with approved existing tools first. `make fmt` and `make check` rewrite source; `make install-deps` runs a privileged installer; snapshot/package targets create or replace release outputs. Browser provisioning, dependency trust changes, service execution and release publication are separate operational actions requiring their applicable authorization. A documentation review or passing fixture suite does not authorize or establish those actions. Do not claim candidate scans or browser checks as evidence for a different commit, toolchain or image.
