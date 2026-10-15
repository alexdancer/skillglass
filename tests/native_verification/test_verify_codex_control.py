"""Executable regression checks for the Codex verification launch boundary."""
import hashlib
import importlib.util
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest
from types import SimpleNamespace
from unittest import mock

REPOSITORY = pathlib.Path(__file__).resolve().parents[2]

INERT = '''package main
import ("bufio"; "encoding/json"; "os")
func main() {
 cwd, _ := os.Getwd()
 scanner := bufio.NewScanner(os.Stdin)
 for scanner.Scan() {
  var request map[string]any
  json.Unmarshal(scanner.Bytes(), &request)
  if id, ok := request["id"]; ok {
   json.NewEncoder(os.Stdout).Encode(map[string]any{"id": id, "result": map[string]any{
    "argv": os.Args, "cwd": cwd, "environment": os.Environ()}})
  }
 }
}
'''


def load(name):
    spec = importlib.util.spec_from_file_location(name, REPOSITORY / 'tools' / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class CodexBoundary(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix='codex-boundary-test-', dir='/private/tmp')
        cls.build = pathlib.Path(cls.temporary.name).resolve()
        cls.probe = cls.build / 'isolation-probe'
        cls.native = cls.build / 'inert-codex'
        cls.skillglass = cls.build / 'skillglass'
        source = cls.build / 'inert.go'
        source.write_text(INERT)
        go = shutil.which('go')
        if not go:
            raise RuntimeError('Go unavailable on PATH; isolation probe build blocked')
        cache = subprocess.check_output([go, 'env', 'GOMODCACHE'], text=True).strip()
        environment = {'PATH': os.environ['PATH'], 'HOME': str(cls.build / 'home'),
                       'CODEX_HOME': str(cls.build / 'home/.codex'),
                       'CLAUDE_CONFIG_DIR': str(cls.build / 'home/.claude'),
                       'XDG_CONFIG_HOME': str(cls.build / 'home/.config'),
                       'TMPDIR': str(cls.build), 'GOTMPDIR': str(cls.build),
                       'GOCACHE': str(cls.build / 'cache'), 'GOMODCACHE': cache,
                       'GOPROXY': 'off', 'GOSUMDB': 'off',
                       'GOTOOLCHAIN': 'local', 'GOWORK': 'off'}
        for target, path in [(cls.probe, './tests/acceptance/cmd/isolationprobe'),
                             (cls.native, str(source)), (cls.skillglass, './cmd/skillglass')]:
            subprocess.run([go, 'build', '-mod=readonly', '-o', str(target), path], cwd=REPOSITORY,
                           env=environment, check=True, capture_output=True, timeout=120)
        cls.module = load('codex_containment')
        cls.transport = load('codex_probe')

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir=self.build)
        self.addCleanup(self.temp.cleanup)
        self.output = pathlib.Path(self.temp.name)
        self.root = self.output / 'fixture'
        self.cwd = self.root / 'repo/nested'
        self.cwd.mkdir(parents=True)
        self.product = pathlib.Path('/bin/echo').resolve()
        self.environment = {'HOME': str(self.root / 'home'),
                            'CODEX_HOME': str(self.root / 'home/.codex'),
                            'CLAUDE_CONFIG_DIR': str(self.root / 'home/.claude'),
                            'XDG_CONFIG_HOME': str(self.root / 'home/.config'),
                            'TMPDIR': str(self.root), 'GOTMPDIR': str(self.root),
                            'PATH': '/usr/bin:/bin',
                            'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'}
        pathlib.Path(self.environment['CODEX_HOME']).mkdir(parents=True)

    def boundary(self, probe=None):
        with mock.patch.object(self.module, 'PINNED_NATIVE_SHA256', self.module.digest(self.native)):
            return self.module.CodexContainment(self.root, self.cwd, self.environment,
                                                self.native, self.product,
                                                probe or self.probe, self.output)

    def test_public_runner_rejects_unpinned_binary_before_execution(self):
        untrusted = self.output / 'untrusted-codex'
        marker = self.output / 'executed'
        untrusted.write_text('#!/bin/sh\ntouch ' + str(marker) + '\necho "codex-cli 0.155.1"\n')
        untrusted.chmod(0o700)
        output = self.output / 'runner-output'
        argv = [sys.executable, str(REPOSITORY / 'tools/verify_codex_control.py'),
                '--codex', str(untrusted), '--product', str(self.product),
                '--isolation-probe', str(self.probe), '--out', str(output)]
        result = subprocess.run(argv, cwd=REPOSITORY, capture_output=True, text=True,
                                timeout=10, env=dict(self.environment, PYTHONDONTWRITEBYTECODE='1'))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('does not match the pinned 0.155.1 SHA256', result.stderr)
        self.assertFalse(marker.exists())
        self.assertEqual(list(output.glob('invocation-*.json')), [])

    def test_both_modes_prove_containment_before_native_launch(self):
        boundary = self.boundary()
        controls = json.loads((self.output / 'containment.json').read_bytes())
        self.assertTrue(all(controls['positive'].values()))
        self.assertEqual([row['writer'] for row in controls['contained']], [False, False, True, True])
        for row in controls['contained']:
            self.assertEqual(json.loads(row['stdout'])['checks'],
                             dict.fromkeys(self.module.CONTROL_NAMES, True))
        self.assertTrue(boundary.ready)
        self.assertFalse((boundary.controls / 'outside-write').exists())
        self.assertFalse((boundary.controls / 'symlink-write').exists())

    def test_failed_controls_refuse_native_execution(self):
        with self.assertRaisesRegex(RuntimeError, 'containment controls failed'):
            self.boundary(probe=pathlib.Path('/usr/bin/false'))
        for receipt in self.output.glob('invocation-*.json'):
            self.assertNotEqual(json.loads(receipt.read_bytes())['binary'], str(self.native))

    def test_server_and_product_use_sealed_final_contexts(self):
        boundary = self.boundary()
        boundary.initialize_git(self.root / 'repo')
        self.assertTrue((self.root / 'repo/.git').is_dir())
        probe = SimpleNamespace(base=self.root, cwd=self.cwd, env=boundary.environment,
                                command=boundary.native_command, results={})
        server = self.transport._Server(probe, ())
        try:
            response = server.call('skills/list', {'cwds': [str(self.cwd)]})['result']
        finally:
            server.close()
        receipt = pathlib.Path(boundary.calls[-1]['receipt'])
        raw = receipt.read_bytes()
        context = json.loads(raw)
        self.assertEqual(response['argv'], context['argv'][3:])
        self.assertEqual(response['cwd'], context['cwd'])
        self.assertEqual(dict(item.split('=', 1) for item in response['environment']),
                         context['environment'])
        self.assertEqual(hashlib.sha256(raw).hexdigest(), boundary.calls[-1]['sha256'])
        self.assertEqual(context['binary_sha256'], self.module.digest(self.native))
        self.assertEqual(context['profile_sha256'], self.module.digest(pathlib.Path(context['profile'])))
        argv = boundary.command(self.product, [], writer=False)
        product_receipt = pathlib.Path(boundary.calls[-1]['receipt'])
        self.assertEqual(json.loads(product_receipt.read_bytes())['argv'], argv)
        result = subprocess.run(argv, cwd=boundary.cwd, env=boundary.environment, timeout=10)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(receipt.read_bytes(), raw)
        self.assertEqual(receipt.stat().st_mode & 0o777, 0o400)
        self.assertEqual(product_receipt.stat().st_mode & 0o777, 0o400)

    def test_actual_product_preview_and_apply_with_acl_helper(self):
        self.product = self.skillglass
        boundary = self.boundary()
        boundary.initialize_git(self.root / 'repo')
        config = pathlib.Path(self.environment['CODEX_HOME']) / 'config.toml'
        skills = pathlib.Path(self.environment['HOME']) / '.agents/skills'
        manifests = []
        for folder in ('target', 'independent'):
            manifest = skills / folder / 'SKILL.md'
            manifest.parent.mkdir(parents=True)
            manifest.write_text('---\nname: same-name\ndescription: Inert fixture.\n---\nDo not execute.\n')
            manifests.append(manifest)
        target, other = manifests
        head = '# retained settings\nmodel = "offline-fixture"\n'
        original = (head + '\n[[skills.config]]\npath = ' + json.dumps(str(target)) +
                    '\nenabled = true\n\n[[skills.config]]\npath = ' + json.dumps(str(other)) +
                    '\nenabled = false\n')
        config.write_text(original)
        config.chmod(0o640)

        def tree():
            return {str(path.relative_to(self.root)):
                    (path.lstat().st_mode, os.readlink(path) if path.is_symlink()
                     else path.read_bytes() if path.is_file() else None)
                    for path in self.root.rglob('*')}

        def run(args, writer=False):
            argv = boundary.command(self.product, args, writer=writer)
            result = subprocess.run(argv, cwd=boundary.cwd, env=boundary.environment,
                                    capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 0, (result.stdout, result.stderr))
            return json.loads(result.stdout)

        list_args = ['list', '--agent', 'codex', '--project', str(self.cwd), '--json']
        listing = run(list_args)
        source_id = next(source['id'] for source in listing['sources']
                         if source['source_path'] == str(target.parent))
        self.assertFalse(self.root.is_relative_to(REPOSITORY))
        for name in ('TMPDIR', 'GOTMPDIR'):
            self.assertFalse(pathlib.Path(boundary.environment[name]).is_relative_to(REPOSITORY))
        for choice, state, enabled in [('disable', 'disabled', False), ('enable', 'available', True)]:
            with self.subTest(choice=choice):
                args = [choice, '--agent', 'codex', '--scope', 'global',
                        '--project', str(self.cwd), '--json']
                before = tree()
                preview = run([*args, source_id])
                self.assertEqual(preview['state'], 'ready')
                self.assertTrue(preview['confirmation'])
                self.assertEqual(tree(), before)
                applied = run([*args, '--confirm', preview['confirmation'], source_id], writer=True)
                self.assertEqual(applied['state'], 'applied')
                self.assertEqual(applied['persistence'], 'verified')
                self.assertEqual(applied['achievement'], 'achieved')
                expected = original if enabled else original.replace('enabled = true', 'enabled = false', 1)
                self.assertEqual(config.read_text(), expected)
                before[str(config.relative_to(self.root))] = (config.lstat().st_mode, expected.encode())
                self.assertEqual(tree(), before)
                self.assertEqual(config.stat().st_mode & 0o777, 0o640)
                states = {source['source_path']: source['availability']['state']
                          for source in run(list_args)['sources']}
                self.assertEqual(states[str(target.parent)], state)
                self.assertEqual(states[str(other.parent)], 'disabled')
        for receipt in self.output.glob('invocation-*.json'):
            self.assertNotEqual(json.loads(receipt.read_bytes())['binary'], str(self.native))

    def test_changed_profile_refuses_launch(self):
        boundary = self.boundary()
        profile = boundary.profiles[True]
        profile.chmod(0o600)
        profile.write_text('(version 1)\n(allow default)\n')
        count = len(boundary.calls)
        with self.assertRaisesRegex(RuntimeError, 'sealed profile changed'):
            boundary.command(self.native, ['--version'])
        self.assertEqual(len(boundary.calls), count)

    def test_changed_binary_refuses_launch(self):
        native = self.output / 'native-copy'
        shutil.copyfile(self.native, native)
        native.chmod(0o700)
        with mock.patch.object(self, 'native', native):
            boundary = self.boundary()
            native.write_bytes(b'changed executable')
            count = len(boundary.calls)
            with self.assertRaisesRegex(RuntimeError, 'sealed executable changed'):
                boundary.command(self.native, ['--version'])
            self.assertEqual(len(boundary.calls), count)


if __name__ == '__main__':
    unittest.main()
