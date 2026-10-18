"""Controller authorization and mutation footprint checks, with bounded transport."""
import argparse
from contextlib import redirect_stderr, redirect_stdout
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'tools'))
import review_scenario as provider


class ReviewScenarioExecution(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='review-execution-', dir='/private/tmp')
        self.root = Path(self.temp.name)
        fixture, output = self.root / 'fixture', self.root / 'output'
        (fixture / 'artifacts').mkdir(parents=True)
        (fixture / 'home/.codex').mkdir(parents=True)
        (fixture / 'home/skill').mkdir()
        (fixture / 'home/skill/SKILL.md').write_text('inert')
        (fixture / 'home/.codex/config.toml').write_text('# keep\n')
        output.mkdir()
        self.state = {'fixture': str(fixture), 'output': str(output),
                      'profile': 'reader-test-only', 'profile_sha256': 'reader-hash',
                      'writer_profile': 'writer-test-only', 'writer_profile_sha256': 'writer-hash',
                      'product': 'product-test-only', 'product_sha256': 'test',
                      'cwd': str(fixture), 'environment': {}, 'execution': True,
                      'mutable_paths': ['home/.codex/config.toml']}
        self.fixture, self.output = fixture, output

    def tearDown(self):
        self.temp.cleanup()

    def invoke(self, arguments, changed=None):
        def transport(argv, **kwargs):
            if changed:
                (self.fixture / changed).write_text('changed')
            return subprocess.CompletedProcess(argv, 0, b'{}\n', b'')
        with mock.patch.object(provider, 'load_session', return_value=self.state), \
                mock.patch.object(provider.subprocess, 'run', side_effect=transport) as run, \
                redirect_stdout(io.TextIOWrapper(io.BytesIO(), write_through=True)), \
                redirect_stderr(io.TextIOWrapper(io.BytesIO(), write_through=True)):
            result = provider.run(argparse.Namespace(arguments=arguments, save=None))
            return result, run.call_args.args[0]

    def test_execution_requires_explicit_session(self):
        self.state['execution'] = False
        with self.assertRaisesRegex(ValueError, 'read-only review'):
            self.invoke(['review', 'apply', '--selection', 'fixture-only'])
        self.assertFalse(list(self.output.glob('call-*')))

    def test_apply_uses_writer_and_records_actual_mutation(self):
        result, argv = self.invoke(['review', 'apply', '--selection', 'fixture-only'],
                                   'home/.codex/config.toml')
        self.assertEqual(result, 0)
        self.assertIn('writer-test-only', argv)
        receipt = json.loads(next(self.output.glob('call-*/result.json')).read_text())
        self.assertIs(receipt['no_write'], False)
        invocation = json.loads(next(self.output.glob('call-*/invocation.json')).read_text())
        self.assertIs(invocation['availability_execution'], True)
        self.assertEqual(invocation['profile_sha256'], 'writer-hash')

    def test_reader_keeps_no_write_guard_in_execution_session(self):
        with self.assertRaisesRegex(RuntimeError, 'read-only product operation'):
            self.invoke(['review', 'select'], 'home/.codex/config.toml')

    def test_apply_rejects_content_mutation(self):
        with self.assertRaisesRegex(RuntimeError, 'outside the explicit native settings'):
            self.invoke(['review', 'apply'], 'home/skill/SKILL.md')

    def test_direct_confirmation_stays_outside_surface(self):
        with self.assertRaisesRegex(ValueError, 'read-only public operations'):
            self.invoke(['disable', '--confirm', 'opaque'])
        self.assertFalse(list(self.output.glob('call-*')))


if __name__ == '__main__':
    unittest.main()
