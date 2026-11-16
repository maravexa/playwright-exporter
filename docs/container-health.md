# Container dependency health and release gating

All hosted workflow jobs are restricted to public repositories. Private repository
qualification runs on an explicitly approved isolated machine; schedules and manual
dispatches do not allocate hosted runners there. `[skip ci]` is also used for
private source pushes. These controls do not grant release authorization.

For public repositories, `container-health.yml` builds the Dockerfile and scans the complete
local Linux/amd64 image on every pull request to main, daily at 06:47 UTC,
on manual dispatch, and as a reusable release preflight. It has read-only
repository permissions, does not log into a registry, and never publishes.
The preflight runs `make test-contracts` before building. The build pulls the
digest-pinned base, applies public APT/Node point updates and installs standard Tini,
installs a pinned npm CLI and the existing runtime lockfile, and includes the
exporter binary. Builds use no cache so APT metadata is refreshed. Build or
scanner errors fail the job. The npm CLI pin is separate from the three locked
Playwright packages and needs review when its bundled libraries receive fixes.

APT package versions are resolved from the upstream image's existing signed
repositories at build time. A base digest alone therefore does not freeze every
final OS package version. Inventory, SBOM and scanning must identify the actual
resulting image; the publication gate scans the exact image that is pushed.

Trivy scans OS and language-package advisories and exits nonzero for HIGH or
CRITICAL findings, including vulnerabilities without an available fix. Reports
are printed in the Actions job log; this gate does not depend on SARIF uploads
or GitHub Advanced Security. The existing filesystem scan remains separate.

Daily Docker Dependabot checks propose base-image updates. Go and npm updates
have their own daily checks. These are proposals, not auto-merges or deployed
patches. Playwright base tags, browser revisions, runtime lockfiles, and the
Dockerfile's compatibility assertion must stay aligned; a base-only update may
intentionally fail the build. Review and test such updates together rather than
relaxing the build assertion or the vulnerability gate.

## Native Node security-release check

Both preflight and publication read the actual image's Node version without
network access in that container, then compare it with the latest published
security release in the same major line from the official Node distribution
index. Missing, malformed or unavailable metadata fails the check. An image
older than that published security version cannot proceed.

This check is not a native-binary vulnerability scanner or an LTS/end-of-life
assessment. It does not cover preannounced updates before fixed versions are
published, or unknown vulnerabilities. Continue reviewing Node and browser
security announcements. Existing NodeSource point updates are applied during
uncached image builds; a repository publication delay can still block release.

## Release order

1. The reusable container preflight must pass before release artifact generation,
   signing, or publication starts.
2. The existing asset staging, checksums, signatures, and asset publication run.
3. The Docker job builds a local release candidate and scans that exact image.
   Only a successful scan permits tagging and pushing it, without rebuilding.

This is not an atomic publication transaction: if the later Docker build or scan
fails (for example, newly published advisories), already published package assets
remain available, but the container is not pushed. The existing post-publication
package scan is informational and is not converted into a package release gate
by this change. No workflow was dispatched, release signed, or image published
while preparing this change.

## Limits and response to failures

A successful advisory scan is not proof of absence of vulnerabilities. Coverage
and severity depend on Trivy's databases and package detection, particularly for
bundled Node/browser binaries. Browser-specific upstream security notices still
need review. This gate does not exercise browser behavior, installer trust,
service lifecycle, or package deployment, and it does not repair dependencies.
It tests the current single supported publication architecture, Linux/amd64.

A digest pin prevents silent base updates. When a new advisory appears, the daily
scan can fail until a reviewed fixed base/runtime is available, even if no source
changed. Inspect the failing package and advisory, review the upstream fix,
update compatible inputs through the normal review process, then rerun the full
image scan and behavior tests. Do not claim release readiness from YAML lint or
a dependency audit alone. Scans require registry, package, and advisory-database
availability. Scheduled jobs run on the default branch and may be delayed or
disabled by GitHub; required PR checks must be configured separately in repository
branch protection.
