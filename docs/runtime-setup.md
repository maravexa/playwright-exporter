# Host runtime setup (local redesign)

## Provisioning contract

The operator supplies a maintained Node.js 22.x, 24.x or 26.x release, npm with lockfile-v3 support, Chromium OS libraries, and standard Linux `runuser`, `flock`, `install`, and `stat` utilities. The installer does not add signing keys, package sources, NodeSource scripts, or OS packages. A package install provides the `playwright-exporter` account and existing browser/test directories; manual installs must provision these explicitly. Node or npm failure stops setup before runtime writes.

Package metadata no longer pulls Node/npm in automatically. It still recommends Chromium as before; an operator/package manager may act on that recommendation independently. Choose an appropriate trusted OS provisioning procedure before adopting this change. The exported service configuration and its sandbox are unchanged.

## Locked runtime

`runtime/package.json` and lockfile v3 pin `@playwright/test`, `playwright`, and `playwright-core` to 1.64.1 with registry-provided SHA-512 package integrity. There are no additional dependencies in the reviewed metadata. The lockfile was prepared from registry metadata and subsequently validated with actual `npm ci` on Node 22.23.4/npm 10.9.9 and Node 24.22.1/npm 11.19.1. Both installed the three locked packages without changing the manifests.

Setup uses `npm ci --engine-strict --ignore-scripts --no-audit --no-fund` in a private staging directory. It does not execute package lifecycle scripts. Successful staging is moved into `/opt/playwright-exporter/runtime-1.64.1`, readable but not writable by the service user. A lock prevents concurrent setup. Existing version directories are reused only when their manifests match and the CLI exists; unknown or partial versions cause failure rather than overwriting them. This is a consistency check, not a full attestation of every previously installed file. Root administrators remain trusted.

The fixed CLI runs as `playwright-exporter` to download Chromium into the existing `/opt/playwright-browsers`. Playwright selects the browser revision through its pinned package contents. The npm lockfile authenticates package tarballs; it is not an independent checksum manifest for browser downloads. No root Playwright CLI, `install-deps`, `--with-deps`, `npx`, or registry-tag resolution is used by the host setup scripts. Browser/OS compatibility must still be smoke-tested on supported distributions before release.

No old runtime or browser directories are automatically removed. A failed browser download leaves the already installed runtime available for retry and may leave Playwright-managed partial browser files; it does not roll back operator projects.

## Operator-owned projects

Setup never walks `/opt/playwright-tests`, runs `npm init`, installs project dependencies, rewrites manifests/lockfiles, or changes ownership of project trees. Each existing project remains operator-owned in the operational sense: the operator chooses its dependencies, lockfile, configuration and migration timing. Package postinstall now creates only missing service directories and never recursively changes existing ownership or permissions. Runtime/project directories are no longer declared with package-manager ownership overrides. Existing symlinks/non-directories cause a clear failure rather than replacement; an operator must resolve them. The service account creation and daemon reload already performed by package postinstall remain unchanged.

To share the new browser, projects must explicitly adopt matching Playwright 1.64.1 dependencies using their own reviewed lockfile. Do not import from the isolated runtime to work around Node module resolution; projects need local dependencies, including for ESM. Projects staying on another version need their matching browser/runtime retained or provisioned separately. New project creation is an explicit operator step, not an installer side effect.

Use `command: "./node_modules/.bin/playwright test --reporter=json"` in schedules. This is now the Go default. Bare `playwright` and legacy plain `npx playwright ...` are resolved to that project's installed binary without executing npx. Existing test arguments are retained. Missing, dangling, non-executable or invalid local binaries fail with instructions to provision the project's reviewed locked dependencies; no fallback to PATH or package download occurs.

Commands containing npx options (including `--yes`, `--package`, or `--no-install`), version-qualified packages, and global/alternate Playwright paths fail with a migration instruction. Use the project-local executable and retain the test arguments. Explicit custom runner commands remain supported and operator-controlled: the exporter does not inspect scripts they invoke or act as a command sandbox. Ordinary npm `.bin` symlinks remain supported.

## Adoption and validation boundaries

Host runtime setup remains an explicit operator step. It does not modify services, initialize projects or rewrite project dependencies. Earlier host and Noble-image fixtures passed CJS/ESM journeys and local/legacy runner commands on Node 22/24, with a failing npx stub never invoked. Those historical results do not qualify a different image or Node line.

The current Dockerfile targets the official Playwright 1.64.1 Ubuntu 26.04 Resolute image by immutable digest. Direct upstream-image inventory reports Node 26.13.0 and Chrome for Testing 156.0.8078.80 at revision 1248. The build updates APT/libAPT and Node point releases from that image's existing signed public repositories, installs standard Tini, and installs pinned npm CLI 12.2.2 before the unchanged three-package runtime lock. Lifecycle scripts remain disabled. These are image-build steps; host setup still leaves Node and OS provisioning to the operator.

The unused-Pebble-removal candidate has not been built or qualified. The following results describe only the accepted pre-removal baseline image.

On 2026-11-10, the isolated Linux/amd64 baseline image passed normal browser completion, worker timeout/shutdown and active-browser timeout/shutdown tests with the actual non-root Tini entrypoint. Every captured browser descendant, including zygotes, was reaped after timeout while the exporter remained alive. The tested renderer had no effective capabilities, no-new-privileges, seccomp filtering and separate user/PID namespaces. These are results for the controlled fixture, not an enforcement guarantee for arbitrary mounted projects.

The exact pre-removal baseline image passed the Trivy 0.70.0 HIGH/CRITICAL advisory gate on 2026-11-10, using the database updated at 19:01 UTC and no ignore or unfixed exclusions. This is a time-specific database result, not clearance for announcements not yet reflected there. Current upstream Node/browser and other native base-binary security review remain open. Do not infer browser patch coverage from a successful journey or package advisory scan. Verify current upstream browser security notices before production use. The complete image is not made static by the exporter's CGO-disabled build.

The image uses non-root Tini as PID 1 to forward signals to Go and reap adopted dead children. It does not enable process-group forwarding or add per-run hard containment. Review project Node engine ranges and native dependencies before migrating to Node 26; host Node 22/24 support remains available. The image runs as an unprivileged service user, preserves `/opt/playwright-browsers` as an alias to `/ms-playwright`, and does not give that user write access to the installed runtime. Mounted projects still need their own reviewed, matching local dependencies. Update the Playwright base version/digest and runtime lock together; the build-time version assertion rejects mismatches. These source changes do not dispatch workflows or publish an image.

Static and fixture tests: `python3 -m unittest discover -s tests -p 'test_runtime_setup.py'`. These execute only the pure policy helper with fake command fixtures, never either installer. Also run `bash -n` on the scripts and the existing GoReleaser configuration checker.
