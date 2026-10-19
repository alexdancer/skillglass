"""Public provider startup checks using the real contained Skillglass executable."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
import review_scenario as provider


@unittest.skipUnless(os.environ.get('SKILLGLASS_PROVIDER_TEST_BINARY') and
                     os.environ.get('SKILLGLASS_COLLECTION_PROBE'),
                     'requires a built Skillglass and the proved isolation probe')
class ReviewScenarioStartup(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix='review-startup-', dir='/private/tmp')
        cls.base = Path(cls.temporary.name)
        cls.sessions = []
        cls.addClassCleanup(cls.cleanup_sessions)
        for execution in (False, True):
            output = cls.base / ('execution' if execution else 'reader')
            argv = [sys.executable, str(ROOT / 'tools/review_scenario.py'), 'create',
                    '--case', 'global-lean', '--binary', os.environ['SKILLGLASS_PROVIDER_TEST_BINARY'],
                    '--probe', os.environ['SKILLGLASS_COLLECTION_PROBE'], '--output', str(output)]
            if execution:
                argv.append('--execution')
            result = subprocess.run(argv, cwd=ROOT, capture_output=True, timeout=60)
            (cls.base / (output.name + '-create.stdout')).write_bytes(result.stdout)
            (cls.base / (output.name + '-create.stderr')).write_bytes(result.stderr)
            if result.returncode:
                raise RuntimeError('contained fixture creation failed: ' + result.stderr.decode())
            state = json.loads((output / 'session.json').read_text())
            cls.sessions.append((output / 'session.json', state))

    @classmethod
    def cleanup_sessions(cls):
        evidence = os.environ.get('SKILLGLASS_PROVIDER_TEST_EVIDENCE')
        if evidence:
            shutil.copytree(cls.base, Path(evidence) / 'startup')
        for _, state in cls.sessions:
            shutil.rmtree(Path(state['fixture']).parent)
        cls.temporary.cleanup()

    def command(self, session, state, label, argv, environment=None):
        fixture = Path(state['fixture'])
        before = provider.manifest(fixture, artifacts=True)
        result = subprocess.run(argv, cwd=state['cwd'], env=environment,
                                capture_output=True, timeout=35)
        after = provider.manifest(fixture, artifacts=True)
        output = session.parent / label
        output.with_suffix('.stdout').write_bytes(result.stdout)
        output.with_suffix('.stderr').write_bytes(result.stderr)
        output.with_suffix('.json').write_text(json.dumps({
            'argv': argv, 'exit_code': result.returncode, 'environment': environment,
            'binary_sha256': state['product_sha256'], 'profile_sha256': state['profile_sha256'],
            'before': before, 'after': after}, indent=2) + '\n')
        self.assertEqual(before, after, 'startup must not mutate even fixture artifacts')
        return result

    def assert_compatible(self, result, state):
        self.assertEqual(result.returncode, 0, result.stdout.decode() + result.stderr.decode())
        report = json.loads(result.stdout)
        self.assertEqual(report['operation'], 'setup-check')
        self.assertEqual(report['state'], 'compatible')
        self.assertEqual(report['exit_code'], 0)
        self.assertEqual(report['executable']['path'], state['product'])

    def test_documented_provider_startup_uses_reader_in_both_session_modes(self):
        for session, state in self.sessions:
            for index, arguments in enumerate((['--check', '--json'], ['--json', '--check'], ['--check'])):
                with self.subTest(execution=state['execution'], arguments=arguments):
                    result = self.command(session, state, 'provider-startup-' + str(index),
                        [sys.executable, str(ROOT / 'tools/review_scenario.py'), 'run',
                         '--session', str(session), '--', 'setup', *arguments])
                    if '--json' in arguments:
                        self.assert_compatible(result, state)
                    else:
                        self.assertEqual(result.returncode, 0, result.stderr.decode())
                        self.assertIn(b'Executable: compatible', result.stdout)
                    receipt = json.loads(result.stderr.decode().splitlines()[-1])
                    self.assertIs(receipt['no_write'], True)
                    call = Path(receipt['receipt'])
                    invocation = json.loads((call / 'invocation.json').read_text())
                    self.assertEqual(invocation['argv'][2], state['profile'])
                    self.assertEqual(invocation['binary_sha256'], state['product_sha256'])
                    self.assertEqual(invocation['profile_sha256'], state['profile_sha256'])
                    self.assertIs(invocation['availability_execution'], False)
                    self.assertEqual((call / 'tree-before.json').read_bytes(),
                                     (call / 'tree-after.json').read_bytes())

    def test_generated_environment_resolves_exact_staged_executable(self):
        for session, state in self.sessions:
            with self.subTest(execution=state['execution']):
                result = self.command(session, state, 'sealed-path-startup',
                    ['/usr/bin/sandbox-exec', '-f', state['profile'], state['product'],
                     'setup', '--check', '--json'], state['environment'])
                self.assert_compatible(result, state)
                self.assertEqual(shutil.which('skillglass', path=state['environment']['PATH']),
                                 state['product'])
                self.assertEqual(state['environment']['PATH'],
                                 str(Path(state['product']).parent) + ':/usr/bin:/bin:/usr/sbin:/sbin')

    def test_setup_mutations_previews_and_malformed_checks_never_launch(self):
        denied = ([], ['--install', '--app', 'codex'], ['--app', 'codex'],
                  ['--check', '--install=false'], ['--check', '--allow-shared-app', 'codeaf'],
                  ['--check=false', '--json'], ['--check=invalid'], ['--check', 'false'],
                  ['--check', '--check=false'], ['--check', '--json', '--app', 'claude'])
        for session, state in self.sessions:
            for index, arguments in enumerate(denied):
                with self.subTest(execution=state['execution'], arguments=arguments):
                    calls = list(session.parent.glob('call-*'))
                    result = self.command(session, state, 'denied-' + str(index),
                        [sys.executable, str(ROOT / 'tools/review_scenario.py'), 'run',
                         '--session', str(session), '--', 'setup', *arguments])
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(b'read-only public operations', result.stderr)
                    self.assertEqual(list(session.parent.glob('call-*')), calls)


if __name__ == '__main__':
    unittest.main()
