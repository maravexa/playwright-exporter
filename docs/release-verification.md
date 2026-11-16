# Verifying release assets

Both Linux exporter binaries are built with `CGO_ENABLED=0`, so the exporter
itself does not require a host glibc version. Node, Playwright and browsers still
require their own compatible operating-system libraries. This setting does not
make the complete browser runtime static or self-contained.

The release workflow builds Linux amd64 and arm64 archives and packages, generates
an SPDX SBOM, and checksums that complete asset set before publication. Cosign
signs `checksums.txt` and records its certificate and transparency-log evidence in
`checksums.txt.sigstore.json`. The workflow verifies that bundle before creating
the GitHub release. An existing release is not overwritten on a rerun.

Download all release assets into an empty directory. For v0.3.0, verify the
manifest with Cosign 3.1.3 and then verify the downloaded files:

```sh
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity https://github.com/maravexa/playwright-exporter/.github/workflows/release.yml@refs/tags/v0.3.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --check checksums.txt
```

Both commands must succeed. Replace the tag in the certificate identity when
verifying another release. The manifest covers the SBOM and eight package/archive
files; the signature bundle is verified separately rather than checksumming itself.
Unlisted files are not authenticated by this manifest.

An SSH-signed annotated Git tag authenticates the source tag separately. It does
not replace artifact verification or constitute a build-provenance attestation.
Container publication remains a subsequent job and currently targets amd64;
this manifest does not sign the GHCR image. Publication can still be partial if
GitHub asset upload or the subsequent container job fails. Inspect the complete
release run before announcing availability; do not automatically overwrite it.

## Local qualification status (2026-11-10)

These are historical results for the accepted image before the proposed unused
Pebble removal. The changed image has not been built, tested or scanned.

An isolated Go 1.26.11 snapshot passed the full and race-enabled Go tests, vet,
format checks and static contracts. Coverage was 77.4%. Both Linux architectures
produced static exporter binaries, and the eight package/archive payloads, SPDX
SBOM and checksum manifest were checked. The arm64 binaries were inspected, not
executed on an arm64 host. No release was signed or published.

The Linux/amd64 image passed all five controlled runtime cases through its real
non-root Tini entrypoint, including browser timeout with adopted zygotes reaped
while the exporter remained running. The tested inputs were Node 26.13.0, npm
12.2.2, Playwright 1.64.1 and Chromium 156.0.8078.80. Runtime results do not prove
that those components contain every current security fix.

The exact pre-removal baseline image passed the Trivy 0.70.0 HIGH/CRITICAL advisory gate
with no ignore or unfixed exclusions on 2026-11-10 at 23:32 UTC. Its database was
updated at 19:01 UTC that day; this does not clear announcements not yet present
in the database or establish native-binary fix coverage. Fresh source/binary
govulncheck remains pending. Current upstream Node/browser and other native
base-binary security review, unresolved installer trust findings, actual package
installation, remote CI, source signing and production deployment are separate
gates. This record is not release clearance
or a build-provenance attestation.
