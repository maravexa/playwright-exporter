"""Static service contract checks; never install or start a unit."""

import configparser
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[1]


class ServiceUnitTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        config = configparser.ConfigParser(interpolation=None)
        config.read(ROOT / "playwright-exporter.service")
        cls.service = config["Service"]

    def test_private_tmp_does_not_depend_on_host_temp_directory(self):
        self.assertEqual(self.service["PrivateTmp"], "yes")
        self.assertEqual(self.service["ProtectSystem"], "strict")
        paths = self.service["ReadWritePaths"].split()
        self.assertEqual(paths, ["/opt/playwright-browsers", "/opt/playwright-tests"])
        self.assertNotIn("/tmp/playwright-exporter", paths)

    def test_stop_covers_the_service_cgroup_with_a_final_kill(self):
        self.assertEqual(self.service["KillMode"], "control-group")
        self.assertEqual(self.service["SendSIGKILL"], "yes")
        self.assertEqual(self.service["TimeoutStopSec"], "10")
        self.assertNotIn("Delegate", self.service)

    def test_service_stays_unprivileged(self):
        self.assertEqual(self.service["User"], "playwright-exporter")
        self.assertEqual(self.service["Group"], "playwright-exporter")
        self.assertEqual(self.service["NoNewPrivileges"], "yes")
        self.assertEqual(self.service["ProtectHome"], "yes")
        self.assertEqual(self.service["MemoryMax"], "2G")

    def test_manual_install_matches_service_binary_path(self):
        binary = self.service["ExecStart"].split()[0]
        self.assertEqual(binary, "/usr/bin/playwright-exporter")
        readme = (ROOT / "README.md").read_text()
        self.assertRegex(readme, re.escape("sudo install -m 755 playwright-exporter /usr/bin/"))


if __name__ == "__main__":
    unittest.main()
