"""Delivered installed-companion sessions using the real contained executable."""
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
class InstalledCompanionSession(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix='review-installed-', dir='/private/tmp')
        cls.base = Path(cls.temporary.name)
        cls.sessions = []
        cls.addClassCleanup(cls.cleanup_sessions)
        for case in ('missing-inputs', 'project-broad'):
            output = cls.base / case
            argv = [sys.executable, str(ROOT / 'tools/review_scenario.py'), 'create',
                    '--case', case, '--installed', '--execution',
                    '--binary', os.environ['SKILLGLASS_PROVIDER_TEST_BINARY'],
                    '--probe', os.environ['SKILLGLASS_COLLECTION_PROBE'], '--output', str(output)]
            if case == 'missing-inputs':
                argv.append('--incomplete-evidence')
            result = subprocess.run(argv, cwd=ROOT, capture_output=True, timeout=60)
            (cls.base / (case + '-create.stdout')).write_bytes(result.stdout)
            (cls.base / (case + '-create.stderr')).write_bytes(result.stderr)
            if result.returncode:
                raise RuntimeError('installed fixture setup failed: ' + result.stderr.decode())
            state = json.loads((output / 'session.json').read_text())
            cls.sessions.append((output / 'session.json', state))

    @classmethod
    def cleanup_sessions(cls):
        evidence = os.environ.get('SKILLGLASS_PROVIDER_TEST_EVIDENCE')
        if evidence:
            shutil.copytree(cls.base, Path(evidence) / 'installed')
        for _, state in cls.sessions:
            shutil.rmtree(Path(state['fixture']).parent)
        cls.temporary.cleanup()

    def test_controller_setup_is_explicit_separate_and_exact(self):
        for session, state in self.sessions:
            with self.subTest(case=state['case']):
                self.assertEqual({i['application'] for i in state['installed_companion']},
                                 {'codex', 'claude-code'})
                output = session.parent / 'controller-setup'
                bundle = json.loads((output / 'bundle/stdout').read_text())
                for name in ('bundle', 'preview', 'install'):
                    receipt = json.loads((output / name / 'invocation.json').read_text())
                    self.assertEqual(receipt['actor'], 'fixture-setup-controller')
                    self.assertEqual(receipt['binary_sha256'], state['product_sha256'])
                    result = json.loads((output / name / 'result.json').read_text())
                    self.assertEqual(result['exit_code'], 0)
                    self.assertEqual(result['no_write'], name != 'install')
                install = json.loads((output / 'install/stdout').read_text())
                self.assertEqual(install['applications'], ['codex', 'claude-code'])
                self.assertEqual(install['allowed_shared_applications'], ['opencode', 'codeaf'])
                prompt = (session.parent / 'prompt.txt').read_text()
                for installation in state['installed_companion']:
                    for entry, delivered in zip(installation['files'], bundle['files']):
                        self.assertEqual(Path(entry['installed_path']).read_bytes(), delivered['content'].encode())
                        self.assertEqual(Path(entry['retained_path']).read_bytes(), delivered['content'].encode())
                        self.assertEqual(provider.digest(Path(entry['installed_path'])), delivered['sha256'])
                        self.assertIn(delivered['content'], prompt)
                fixture = Path(state['fixture'])
                self.assertFalse((fixture / 'home/.config/opencode/skills/skillglass-review').exists())
                self.assertFalse((fixture / 'home/.agents/skills/skillglass-review').exists())
                self.assertEqual(provider.manifest(fixture), state['initial_manifest'])

    def test_missing_request_and_incomplete_fixture_are_independent(self):
        session, state = self.sessions[0]
        prompt = (session.parent / 'prompt.txt').read_text()
        request = (session.parent / 'request.txt').read_text()
        context = (session.parent / 'context.txt').read_text()
        self.assertEqual(request, 'Review my skills and suggest what should stay enabled.\n')
        self.assertNotIn(request.strip(), context)
        self.assertTrue(prompt.endswith(request))
        self.assertIn('Review my skills and suggest what should stay enabled.', prompt)
        self.assertNotIn('Review my global Codex and Claude Code collection for a leaner', prompt)
        self.assertNotIn('Keep guardrails and every installed', prompt)
        self.assertTrue(state['incomplete_evidence'])
        fixture = Path(state['fixture'])
        self.assertTrue((fixture / 'home/.agents/skills/research/references/evidence.md').is_symlink())
        self.assertGreater((fixture / 'home/.agents/skills/research/large.txt').stat().st_size, 1 << 20)
        self.assertIn(b'\x00', (fixture / 'home/.agents/skills/research/binary.dat').read_bytes())

    def test_installed_session_startup_is_read_only_and_setup_stays_denied(self):
        for session, state in self.sessions:
            fixture = Path(state['fixture'])
            before = provider.manifest(fixture, artifacts=True)
            argv = [sys.executable, str(ROOT / 'tools/review_scenario.py'), 'run',
                    '--session', str(session), '--']
            result = subprocess.run(argv + ['setup', '--check', '--json'], cwd=ROOT,
                                    capture_output=True, timeout=35)
            self.assertEqual(result.returncode, 0, result.stderr.decode())
            self.assertEqual(json.loads(result.stdout)['state'], 'compatible')
            receipt = json.loads(result.stderr.decode().splitlines()[-1])
            invocation = json.loads((Path(receipt['receipt']) / 'invocation.json').read_text())
            self.assertEqual(invocation['argv'][2], state['profile'])
            calls = list(session.parent.glob('call-*'))
            denied = subprocess.run(argv + ['setup', '--app', 'codex', '--install'], cwd=ROOT,
                                    capture_output=True, timeout=35)
            self.assertNotEqual(denied.returncode, 0)
            self.assertEqual(list(session.parent.glob('call-*')), calls)
            self.assertEqual(provider.manifest(fixture, artifacts=True), before)

    def test_replaced_installed_evidence_refuses_before_product_launch(self):
        session, state = self.sessions[0]
        path = Path(state['installed_companion'][0]['files'][0]['installed_path'])
        original = path.read_bytes()
        calls = list(session.parent.glob('call-*'))
        try:
            path.write_bytes(b'customized copy must never be silently replaced')
            result = subprocess.run([sys.executable, str(ROOT / 'tools/review_scenario.py'),
                                    'run', '--session', str(session), '--', 'help'],
                                    cwd=ROOT, capture_output=True, timeout=35)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'installed companion evidence changed', result.stderr)
            self.assertEqual(list(session.parent.glob('call-*')), calls)
            self.assertEqual(path.read_bytes(), b'customized copy must never be silently replaced')
        finally:
            path.write_bytes(original)


if __name__ == '__main__':
    unittest.main()
