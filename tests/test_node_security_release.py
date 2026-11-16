"""Offline release-metadata checks. Never download or execute Node binaries."""

import importlib.util
import io
from pathlib import Path
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "node_security_release", ROOT / "scripts/check-node-security-release.py")
CHECK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECK)


class MetadataResponse(io.BytesIO):
    def __init__(self, payload, destination=CHECK.INDEX_URL):
        super().__init__(payload)
        self.destination = destination

    def geturl(self):
        return self.destination


class NodeSecurityReleaseTests(unittest.TestCase):
    def setUp(self):
        self.releases = [
            {"version": "v26.13.0", "security": False},
            {"version": "v26.12.1", "security": True},
            {"version": "v24.99.0", "security": True},
        ]

    def test_same_major_security_floor_ignores_newer_nonsecurity_and_other_lines(self):
        self.assertEqual(CHECK.security_floor(self.releases, "v26.13.0"), (26, 12, 1))

    def test_new_security_release_raises_the_floor(self):
        self.releases.append({"version": "v26.14.0", "security": True})
        self.assertEqual(CHECK.security_floor(self.releases, "v26.13.0"), (26, 14, 0))

    def test_official_prerelease_rows_are_not_stable_security_floors(self):
        self.releases.extend([
            {"version": "v27.0.0-alpha.0.0.0", "security": False},
            {"version": "v26.14.0-rc.1", "security": True},
        ])
        self.assertEqual(CHECK.security_floor(self.releases, "v26.13.0"), (26, 12, 1))

    def test_stable_version_parser_rejects_malformed_and_prerelease_values(self):
        for value in [None, "26.13", "v26.14.0-rc.1", "v26.13.0 extra", "$(command)",
                      "v026.13.0", "v26.013.0", "v٢٦.13.0"]:
            with self.subTest(value=value), self.assertRaises(ValueError):
                CHECK.version_tuple(value)

    def test_missing_or_malformed_metadata_fails_closed(self):
        for releases in [{}, [], [None], [{"version": "v26.14.0"}],
                         [{"version": "v26.14.0", "security": "true"}],
                         [{"version": "v26.13.0", "security": False}]]:
            with self.subTest(releases=releases), self.assertRaises(ValueError):
                CHECK.security_floor(releases, "v26.13.0")

    def test_main_fails_for_an_older_installed_security_version(self):
        self.releases.append({"version": "v26.14.0", "security": True})
        with patch.object(CHECK, "fetch_index", return_value=(self.releases, "fixture")), \
                patch("sys.stdout", new_callable=io.StringIO) as output:
            self.assertEqual(CHECK.main(["v26.13.0"]), 1)
        self.assertIn('"meets_published_security_floor": false', output.getvalue())

    def test_main_accepts_an_installed_version_above_the_security_floor(self):
        with patch.object(CHECK, "fetch_index", return_value=(self.releases, "fixture")), \
                patch("sys.stdout", new_callable=io.StringIO):
            self.assertEqual(CHECK.main(["v26.13.0"]), 0)

    def test_fetch_failure_is_not_a_clean_result(self):
        with patch.object(CHECK, "fetch_index", side_effect=OSError("fixture")), \
                patch("sys.stderr", new_callable=io.StringIO):
            self.assertEqual(CHECK.main(["v26.13.0"]), 2)

    def test_unexpected_metadata_destination_is_rejected(self):
        for destination in ["http://nodejs.org/dist/index.json", "https://example.invalid/index.json"]:
            with self.subTest(destination=destination), \
                    patch.object(CHECK.urllib.request, "urlopen",
                                 return_value=MetadataResponse(b"[]", destination)), \
                    self.assertRaises(ValueError):
                CHECK.fetch_index()

    def test_malformed_metadata_json_is_rejected(self):
        with patch.object(CHECK.urllib.request, "urlopen", return_value=MetadataResponse(b"{")), \
                self.assertRaises(ValueError):
            CHECK.fetch_index()

    def test_oversized_metadata_is_rejected_before_parsing(self):
        with patch.object(CHECK, "MAX_INDEX_BYTES", 4), \
                patch.object(CHECK.urllib.request, "urlopen", return_value=MetadataResponse(b"[0,0]")), \
                self.assertRaises(ValueError):
            CHECK.fetch_index()


if __name__ == "__main__":
    unittest.main()
