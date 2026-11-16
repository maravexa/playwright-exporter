"""Static release-build contracts; actual ELF checks run in release validation."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]


class ReleaseBuildTests(unittest.TestCase):
    def test_exporter_package_targets_disable_cgo_explicitly(self):
        config = (ROOT / ".goreleaser.yml").read_text()
        builds = config.split("\nbuilds:\n", 1)[1].split("\narchives:\n", 1)[0]
        self.assertEqual(builds.count("  - id:"), 1)
        self.assertIn("  - id: playwright-exporter", builds)
        self.assertRegex(builds, r"(?m)^    env:\n      - CGO_ENABLED=0$")
        self.assertRegex(builds, r"(?m)^    goos:\n      - linux$")
        self.assertRegex(builds, r"(?m)^    goarch:\n      - amd64\n      - arm64$")
        self.assertNotRegex(builds, r"CGO_ENABLED=(?!0(?:\s|$))")

    def test_minimum_go_patch_is_security_updated_and_documented(self):
        lines = (ROOT / "go.mod").read_text().splitlines()
        version = next(line.split()[1] for line in lines if line.startswith("go "))
        self.assertGreaterEqual(tuple(map(int, version.split("."))), (1, 26, 11))
        for document in ["AGENTS.md", "CONTRIBUTING.md"]:
            self.assertIn(version, (ROOT / document).read_text())

    def test_browser_runtime_is_not_claimed_static(self):
        document = (ROOT / "docs/release-verification.md").read_text()
        self.assertIn("Node, Playwright and browsers still", document)
        self.assertIn("does not\nmake the complete browser runtime static", document)


if __name__ == "__main__":
    unittest.main()
