"""Checks for missing, stale and tampered release assets."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "assets", Path(__file__).with_name("prepare-release-assets.py"))
assets = importlib.util.module_from_spec(spec)
spec.loader.exec_module(assets)


class ReleaseAssetsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.source = Path(self.temp.name) / "dist"
        self.source.mkdir()
        self.output = Path(self.temp.name) / "release"
        self.items = []
        for arch in ("amd64", "arm64"):
            for suffix in (".tar.gz", ".tar.zst", ".deb", ".rpm"):
                name = f"exporter_linux_{arch}{suffix}"
                (self.source / name).write_bytes(b"fixture")
                self.items.append({"name": name, "goarch": arch, "goos": "linux",
                                   "type": "Archive" if suffix.startswith(".tar") else "Linux Package"})
        (self.source / "artifacts.json").write_text(json.dumps(self.items))
        (self.source / "playwright-exporter.sbom.spdx.json").write_text(
            json.dumps({"spdxVersion": "SPDX-2.3"}))
        (self.source / "stale.sig").write_text("must not publish")

    def test_complete_manifest_and_tampering(self):
        assets.prepare(self.source, self.output)
        self.assertEqual(len((self.output / "checksums.txt").read_text().splitlines()), 9)
        self.assertFalse((self.output / "stale.sig").exists())
        subprocess.run(["sha256sum", "--check", "checksums.txt"], cwd=self.output,
                       check=True, capture_output=True)
        (self.output / self.items[0]["name"]).write_text("tampered")
        result = subprocess.run(["sha256sum", "--check", "checksums.txt"],
                                cwd=self.output, capture_output=True)
        self.assertNotEqual(result.returncode, 0)

    def test_missing_platform_rejected(self):
        (self.source / "artifacts.json").write_text(json.dumps(self.items[:-1]))
        with self.assertRaises(ValueError):
            assets.prepare(self.source, self.output)

    def test_missing_sbom_rejected(self):
        (self.source / "playwright-exporter.sbom.spdx.json").unlink()
        with self.assertRaises(FileNotFoundError):
            assets.prepare(self.source, self.output)

    def test_stale_destination_rejected(self):
        self.output.mkdir()
        with self.assertRaises(FileExistsError):
            assets.prepare(self.source, self.output)


if __name__ == "__main__":
    unittest.main()
