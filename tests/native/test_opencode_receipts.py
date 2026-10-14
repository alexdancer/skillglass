"""Public helper interruption and inert durable launch controls; no native startup."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT/'tools'))
import native_receipts
import verify_opencode


class ReceiptControls(unittest.TestCase):
    def setUp(self):
        self.base = Path(tempfile.mkdtemp(prefix='ale35-receipts-', dir='/private/tmp'))
        self.addCleanup(lambda: shutil.rmtree(self.base, ignore_errors=True))
        self.binary = self.base/'inert'
        self.binary.write_bytes(b'INERT_NOT_EXECUTABLE\n')
        self.profile = self.base/'profile.sb'
        self.profile.write_text('(inert profile one)')
        self.out = self.base/'evidence'
        self.env = {'PATH':'/usr/bin:/bin', 'HOME':str(self.base)}

    def tearDown(self):
        destination = os.environ.get('ALE35_RECEIPTS_OUT')
        if destination and self.out.exists():
            shutil.copytree(self.out, Path(destination)/self.id().split('.')[-1], dirs_exist_ok=True)

    def prepared(self, argv, kwargs):
        records = list((self.out/'invocations').glob('*/prepared.json'))
        matching = [json.loads(path.read_text()) for path in records
                    if json.loads(path.read_text())['wrapped_argv'] == argv]
        record = max(matching, key=lambda item: item['prepared_at'])
        self.assertEqual(record['state'], 'prepared')
        self.assertEqual(record['cwd'], str(kwargs['cwd']))
        self.assertEqual(record['environment'], kwargs['env'])
        self.assertEqual(record['binary_sha256'], hashlib.sha256(Path(record['binary']).read_bytes()).hexdigest())
        self.assertEqual(record['profile'], Path(argv[2]).read_text())
        self.assertEqual(record['profile_sha256'], hashlib.sha256(Path(argv[2]).read_bytes()).hexdigest())
        self.assertNotIn('launch_observed_at', record)
        self.assertNotIn('pid', record)
        return record

    def run_control(self, observer, label='control'):
        argv = ['/usr/bin/sandbox-exec', '-f', str(self.profile), str(self.binary)]
        with patch.object(native_receipts.subprocess, 'Popen', side_effect=observer):
            return native_receipts.recorded_run(self.out, label, argv, cwd=self.base,
                env=self.env, binary=self.binary, profile=self.profile)

    def test_public_main_interrupted_before_first_launch(self):
        git = self.base/'inert-git'
        git.write_bytes(b'INERT_GIT_NOT_EXECUTABLE\n')
        observed = []
        class Interrupted(Exception):
            pass
        def stop(argv, **kwargs):
            observed.append(self.prepared(argv, kwargs))
            raise Interrupted('intercepted before OS launch')
        with patch.object(sys, 'argv', ['verify_opencode.py', '--opencode', str(self.binary),
                    '--git', str(git), '--out', str(self.out)]), \
                patch.object(verify_opencode, 'runtime_data_paths', return_value=[]), \
                patch.object(native_receipts.subprocess, 'Popen', side_effect=stop):
            with self.assertRaises(Interrupted):
                verify_opencode.main()
        self.assertEqual(len(observed), 1)
        start = observed[0]
        self.assertEqual(start['source_binary'], str(git))
        failure = json.loads((Path(start['record_directory'])/'failed.json').read_text())
        self.assertEqual(failure['state'], 'failed')
        self.assertIsNone(failure['exit'])
        self.assertEqual(json.loads((self.out/'receipts.json').read_text())['commands'], [])

    def test_success_rejection_timeout_and_changed_profiles(self):
        observed = []
        class Child:
            pid = 987654321
            def __init__(self, code, timeout):
                self.returncode = code
                self.timeout = timeout
            def communicate(self, timeout=None):
                if timeout and self.timeout:
                    raise subprocess.TimeoutExpired('inert', timeout)
                return b'inert stdout', b'inert stderr'
            def poll(self):
                return self.returncode
        for code, timeout in [(0, False), (7, False), (-9, True)]:
            self.profile.write_text('(inert profile '+str(code)+')')
            def launch(argv, **kwargs):
                observed.append(self.prepared(argv, kwargs))
                return Child(code, timeout)
            with patch.object(native_receipts.os, 'killpg', side_effect=ProcessLookupError):
                result, record = self.run_control(launch)
            self.assertEqual(result.returncode, code)
            self.assertEqual(record['timed_out'], timeout)
            self.assertEqual(Path(record['stdout']).read_bytes(), b'inert stdout')
            self.assertTrue((Path(record['record_directory'])/'launched.json').exists())
            self.assertTrue((Path(record['record_directory'])/'completed.json').exists())
        self.assertEqual(len({r['invocation_id'] for r in observed}), 3)
        self.assertEqual(len({r['profile_sha256'] for r in observed}), 3)

    def test_launch_failure_retains_start(self):
        def fail(argv, **kwargs):
            self.prepared(argv, kwargs)
            raise OSError('inert launch failure')
        with self.assertRaises(OSError):
            self.run_control(fail)
        failed = next((self.out/'invocations').glob('*/failed.json'))
        self.assertEqual(json.loads(failed.read_text())['error_type'], 'OSError')
        self.assertFalse(list((self.out/'invocations').glob('*/launched.json')))

    def test_record_failure_prevents_launch(self):
        with patch.object(native_receipts, 'durable_json', side_effect=OSError('record denied')), \
                patch.object(native_receipts.subprocess, 'Popen') as launch:
            with self.assertRaises(OSError):
                native_receipts.recorded_run(self.out, 'fail-closed', [str(self.binary)],
                                            cwd=self.base, env=self.env)
            launch.assert_not_called()


if __name__ == '__main__':
    unittest.main(verbosity=2)
