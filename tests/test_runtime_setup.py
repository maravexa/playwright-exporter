"""Read-only fixture/static checks. Never execute either installation script."""
import base64
import json
from pathlib import Path
import re
import subprocess
import tempfile
import unittest
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
POLICY = ROOT / 'scripts/runtime-policy.sh'


class RuntimePolicyTests(unittest.TestCase):
    def check_policy(self, node_version=None, npm_present=True, node_exit=0):
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            if node_version is not None:
                node = folder / 'node'
                node.write_text('#!/bin/bash\nprintf "%s\\n" ' + repr(node_version)
                                + '\nexit ' + str(node_exit) + '\n')
                node.chmod(0o755)
            if npm_present:
                npm = folder / 'npm'
                npm.write_text('#!/bin/bash\nexit 0\n')
                npm.chmod(0o755)
            return subprocess.run(
                ['/bin/bash', '-c', 'source "$1"; runtime_check_node', 'fixture', str(POLICY)],
                env={'PATH': tmp}, text=True, capture_output=True, check=False)

    def test_supported_node_lines(self):
        for version in ['v22.0.0', 'v22.20.1', 'v24.0.0', 'v24.9.3']:
            with self.subTest(version=version):
                result = self.check_policy(version)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_missing_old_odd_future_and_malformed_node(self):
        for version in [None, '', 'v18.20.0', 'v20.19.0', 'v23.1.0', 'v26.0.0',
                        'v24.1.0-rc.1', 'v24.1', 'v24.1.0 extra', 'v24.x.0']:
            with self.subTest(version=version):
                result = self.check_policy(version)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('provision a maintained Node.js', result.stderr)

    def test_failed_node_and_missing_npm_fail(self):
        self.assertNotEqual(self.check_policy('v24.1.0', node_exit=1).returncode, 0)
        result = self.check_policy('v24.1.0', npm_present=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('provision npm', result.stderr)


class RuntimeStaticTests(unittest.TestCase):
    def test_lock_is_complete_exact_and_integrity_protected(self):
        manifest = json.loads((ROOT / 'runtime/package.json').read_text())
        lock = json.loads((ROOT / 'runtime/package-lock.json').read_text())
        packages = lock['packages']
        self.assertEqual(lock['lockfileVersion'], 3)
        self.assertTrue(manifest['private'])
        self.assertEqual(packages['']['dependencies'], manifest['dependencies'])
        self.assertEqual(packages['']['engines'], manifest['engines'])
        reachable = set()

        def visit(name, version):
            self.assertRegex(version, r'^\d+\.\d+\.\d+$')
            key = 'node_modules/' + name
            package = packages[key]
            self.assertEqual(package['version'], version)
            self.assertEqual(urlparse(package['resolved']).hostname, 'registry.npmjs.org')
            self.assertEqual(urlparse(package['resolved']).scheme, 'https')
            kind, digest = package['integrity'].split('-', 1)
            self.assertEqual(kind, 'sha512')
            self.assertEqual(len(base64.b64decode(digest, validate=True)), 64)
            if key in reachable:
                return
            reachable.add(key)
            for dep, dep_version in package.get('dependencies', {}).items():
                visit(dep, dep_version)
            self.assertFalse(package.get('optionalDependencies'))

        for name, version in manifest['dependencies'].items():
            visit(name, version)
        self.assertEqual(set(packages), reachable | {''})

    def test_host_setup_cannot_bootstrap_node_or_modify_projects(self):
        setup = (ROOT / 'scripts/playwright-exporter-setup.sh').read_text()
        legacy = (ROOT / 'scripts/install-deps.sh').read_text()
        code = '\n'.join(line for line in (setup + legacy).splitlines()
                         if not line.lstrip().startswith('#'))
        self.assertNotRegex(code, r'\b(curl|wget|npx|apt-get|dnf|pacman|useradd|chown)\b')
        self.assertNotIn('/opt/playwright-tests', code)
        self.assertNotIn('npm init', code)
        self.assertNotIn('npm install', code)
        self.assertNotIn('install-deps chromium', code)
        self.assertNotIn('--with-deps', code)
        self.assertIn('npm ci --engine-strict --ignore-scripts --no-audit --no-fund', code)
        self.assertIn('runuser -u playwright-exporter', code)
        self.assertIn('flock -n 9', code)
        self.assertIn('exec bash', legacy)
        self.assertLess(code.index('runuser -u playwright-exporter -- \"$NODE_BIN\" --version'), code.index('install -d'))
        self.assertLess(code.index('runtime_check_node'), code.index('install -d'))
        self.assertLess(code.index('npm ci'), code.index('mv -T'))
        self.assertIn('cmp -s "$MANIFEST_DIR/package-lock.json"', code)

    def test_packaging_ships_policy_and_both_manifests(self):
        config = (ROOT / '.goreleaser.yml').read_text()
        for path in ['runtime/package.json', 'runtime/package-lock.json', 'scripts/runtime-policy.sh']:
            self.assertIn('src: ' + path, config)
        self.assertNotIn('nodejs (>=', config)
        self.assertNotIn('nodejs >=', config)
        examples = (ROOT / 'config.example.yaml').read_text()
        self.assertNotIn('npx', examples)
        self.assertEqual(examples.count('./node_modules/.bin/playwright'), 3)

    def test_package_upgrade_preserves_existing_project_ownership(self):
        postinstall = (ROOT / 'scripts/postinstall.sh').read_text()
        self.assertNotIn('chown', postinstall)
        self.assertNotIn('chmod', postinstall)
        self.assertIn('[[ -e "$dir" || -L "$dir" ]]', postinstall)
        self.assertIn('continue', postinstall)
        self.assertLess(postinstall.index('continue'), postinstall.index('install -d'))
        config = (ROOT / '.goreleaser.yml').read_text()
        for path in ['/opt/playwright-tests', '/opt/playwright-browsers', '/tmp/playwright-exporter']:
            self.assertNotIn('dst: ' + path, config)

    def test_container_base_matches_locked_runtime(self):
        dockerfile = (ROOT / 'Dockerfile').read_text()
        manifest = json.loads((ROOT / 'runtime/package.json').read_text())
        version = manifest['dependencies']['@playwright/test']
        self.assertRegex(dockerfile, r'FROM mcr\.microsoft\.com/playwright:v' + re.escape(version)
                         + r'-noble@sha256:[0-9a-f]{64}')
        self.assertIn('COPY runtime/package.json runtime/package-lock.json', dockerfile)
        self.assertIn('npm ci --engine-strict --ignore-scripts --no-audit --no-fund', dockerfile)
        self.assertNotIn('RUN npx', dockerfile)
        self.assertNotIn('install chromium', dockerfile)
        self.assertIn('ln -s /ms-playwright /opt/playwright-browsers', dockerfile)
        self.assertIn('USER playwright-exporter', dockerfile)

    def test_shell_syntax_only(self):
        for path in (ROOT / 'scripts').glob('*.sh'):
            result = subprocess.run(['/bin/bash', '-n', str(path)],
                                    text=True, capture_output=True, check=False)
            self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == '__main__':
    unittest.main()
