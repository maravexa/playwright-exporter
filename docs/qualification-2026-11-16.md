# Candidate qualification, 2026-11-16

This records development evidence, not release or deployment approval.

The Linux/amd64 candidate uses Go 1.26.11, Node 26.13.1, npm 12.2.2 and
Playwright 1.64.1. Its image config ID is
`sha256:b39e22d99ef8096c67a3046bb8799682932ecc0577c3f992c6d242abb924f5d8`;
its exact OCI manifest is
`sha256:bb063f8745eacc896f1ba9d1cb6d6b1815594649c8144436a9a3da2f54f55b13`.
The retained image was refreshed using signed existing APT sources, a rebuilt
exporter, and the previously reviewed npm tree. This does not establish that a
fresh Dockerfile build produces identical package bytes.

## Passed

- Official govulncheck 1.9.0 found no reachable vulnerabilities in the source or
  rebuilt Linux amd64/arm64 binaries using Go 1.26.11.
- Trivy 0.76.2 full **image** scanning returned exit 0 and zero HIGH/CRITICAL
  findings against its November 16 database. All 18 layer digests and sizes,
  config diff IDs, and the unchanged manifest were independently verified.
  This supersedes the earlier supplementary merged-rootfs-only scan.
- Exact native Node version meets the published same-major security floor;
  two real Playwright CLI fixture tests passed.
- Current-main source reconciliation passed Go race tests, vet, 39 runtime and
  publication contract tests, four asset tests, and actionlint syntax checking.
- Native Linux PID namespace tests passed successful-run and timeout descendant
  cleanup on the isolated amd64 test VM, without changing its namespace policy.

## Remaining limits

The real Chromium test explicitly enabled its sandbox. The browser's zygote
exited prematurely with SIGTRAP before the local DOM journey could run under
the VM's existing container confinement. Browser startup, renderer sandbox and
browser cancellation acceptance are therefore **not passed**. No sandbox,
AppArmor, seccomp or global namespace restriction was disabled.

ARM64 evidence is cross-build and binary scanning, not native execution.
Production service identity, distribution/package acceptance and dependency
trust review remain separate gates. The denied trust operation was not retried.
No release, tag, container publication or production service deployment occurred.
All hosted jobs now require a public repository; private work is checked on
approved local/isolated infrastructure.
