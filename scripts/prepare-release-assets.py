#!/usr/bin/env python3
"""Stage the complete release asset set before checksumming and signing it."""

import hashlib
import json
from pathlib import Path
import shutil
import sys


def prepare(source: Path, destination: Path) -> None:
    artifacts = json.loads((source / "artifacts.json").read_text())
    selected = [a for a in artifacts if a["type"] in ("Archive", "Linux Package")]
    expected = {(arch, suffix) for arch in ("amd64", "arm64")
                for suffix in (".tar.gz", ".tar.zst", ".deb", ".rpm")}
    actual = set()
    paths = []
    for artifact in selected:
        name = artifact["name"]
        if Path(name).name != name:
            raise ValueError("Artifact name must be a basename")
        suffix = next((s for s in (".tar.gz", ".tar.zst", ".deb", ".rpm")
                       if name.endswith(s)), None)
        pair = (artifact.get("goarch"), suffix)
        if artifact.get("goos") != "linux" or pair not in expected or pair in actual:
            raise ValueError(f"Unexpected or duplicate artifact: {name}")
        actual.add(pair)
        paths.append(source / name)
    if actual != expected:
        raise ValueError(f"Missing release artifacts: {expected - actual}")
    sbom = source / "playwright-exporter.sbom.spdx.json"
    document = json.loads(sbom.read_text())
    if not document.get("spdxVersion", "").startswith("SPDX-"):
        raise ValueError("Missing SPDX SBOM")
    paths.append(sbom)
    # Reject stale output instead of signing files left by a previous run.
    destination.mkdir(exist_ok=False)
    lines = []
    for path in sorted(paths):
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"Missing or unsafe asset: {path}")
        target = destination / path.name
        shutil.copyfile(path, target)
        lines.append(f"{hashlib.sha256(target.read_bytes()).hexdigest()}  {target.name}\n")
    (destination / "checksums.txt").write_text("".join(lines))


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: prepare-release-assets.py DIST OUTPUT")
    prepare(Path(sys.argv[1]), Path(sys.argv[2]))
