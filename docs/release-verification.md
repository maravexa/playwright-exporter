# Verifying release assets

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
