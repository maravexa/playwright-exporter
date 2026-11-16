"""Static publication-safety checks; actionlint validates workflow YAML separately."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[1]


def block(text, header, indent):
    """Extract one indentation-delimited YAML block from these workflow files."""
    prefix = ' ' * indent
    start = text.index(prefix + header + '\n')
    lines = text[start:].splitlines()
    result = [lines[0]]
    for line in lines[1:]:
        if line.strip() and len(line) - len(line.lstrip()) <= indent:
            break
        result.append(line)
    return '\n'.join(result)


class ContainerWorkflowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.health = (ROOT / '.github/workflows/container-health.yml').read_text()
        cls.release = (ROOT / '.github/workflows/release.yml').read_text()
        cls.ci = (ROOT / '.github/workflows/ci.yml').read_text()

    def test_preflight_gates_asset_release(self):
        preflight = block(self.release, 'container_preflight:', 2)
        release = block(self.release, 'release:', 2)
        self.assertIn('uses: ./.github/workflows/container-health.yml', preflight)
        self.assertIn('needs: [container_preflight]', release)
        self.assertNotIn('if: always()', release)
        self.assertNotIn('continue-on-error', preflight + release)
        self.assertIn('Publish verified assets', release)

    def test_health_is_read_only_and_never_publishes(self):
        for event in ('pull_request:', 'schedule:', 'workflow_dispatch:', 'workflow_call:'):
            self.assertIn('  ' + event, self.health)
        self.assertIn('  contents: read', self.health)
        for forbidden in ('write', 'docker push', 'push: true', 'login-action', 'secrets.'):
            self.assertNotIn(forbidden, self.health)
        self.assertLess(self.health.index('docker build '), self.health.index('scan-type: image'))

    def test_both_image_scans_fail_closed(self):
        for workflow in (self.health, block(self.release, 'docker:', 2)):
            with self.subTest(workflow=workflow.splitlines()[0]):
                for setting in ('scan-type: image', 'scanners: vuln', 'vuln-type: os,library',
                                'severity: HIGH,CRITICAL', "ignore-unfixed: 'false'", "exit-code: '1'"):
                    self.assertIn(setting, workflow)
                self.assertNotIn('continue-on-error', workflow)
                self.assertNotIn('if: always()', workflow)

    def test_exact_scanned_candidate_is_pushed(self):
        docker = block(self.release, 'docker:', 2)
        self.assertIn('needs: [release]', docker)
        self.assertIn('load: true', docker)
        self.assertIn('push: false', docker)
        self.assertNotIn('push: true', docker)
        scan = docker.index('image-ref: playwright-exporter:release-candidate')
        publish = docker.index('name: Push scanned image without rebuilding')
        self.assertLess(scan, publish)
        tail = docker[publish:]
        self.assertNotIn('docker build', tail)
        self.assertIn('docker tag playwright-exporter:release-candidate', tail)
        self.assertEqual(tail.count('docker push '), 2)

    def test_docker_updates_are_daily(self):
        config = (ROOT / '.github/dependabot.yml').read_text()
        docker = config.split('  - package-ecosystem: docker\n', 1)[1].split('  - ', 1)[0]
        self.assertIn('directory: /', docker)
        self.assertIn('interval: daily', docker)

    def test_security_refresh_builds_are_not_cached(self):
        self.assertIn('docker build --pull --no-cache ', self.health)
        self.assertIn('          no-cache: true', block(self.release, 'docker:', 2))
        self.assertIn('needs: [contracts, lint, test]', block(self.ci, 'build:', 2))

    def test_native_node_floor_is_checked_before_both_image_scans(self):
        for workflow, scan in [(self.health, 'name: Gate full image'),
                               (block(self.release, 'docker:', 2), 'name: Gate the exact publication image')]:
            with self.subTest(scan=scan):
                self.assertIn('python3 scripts/check-node-security-release.py "$node_version"', workflow)
                self.assertIn('--network none --read-only --entrypoint node', workflow)
                self.assertLess(workflow.index('python3 scripts/check-node-security-release.py'),
                                workflow.index(scan))

    def test_private_repositories_never_allocate_hosted_jobs(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            jobs = path.read_text().split('\njobs:\n', 1)[1]
            for name in re.findall(r'^  ([a-zA-Z0-9_-]+):$', jobs, re.MULTILINE):
                with self.subTest(workflow=path.name, job=name):
                    job = block(jobs, name + ':', 2)
                    self.assertEqual(len(re.findall(r'^    if:', job, re.MULTILINE)), 1)
                    self.assertIn("github.event.repository.visibility == 'public'", job)

    def test_action_references_are_immutable(self):
        for workflow in (self.health, self.release):
            for action in re.findall(r'uses: ([^\s]+)', workflow):
                if not action.startswith('./'):
                    self.assertRegex(action, r'@\w{40}$')


if __name__ == '__main__':
    unittest.main()
