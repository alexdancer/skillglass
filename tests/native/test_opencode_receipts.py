"""Public helper interruption and inert durable launch controls; no native startup."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import time
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

    def test_launch_record_failure_retains_cleanup_and_persistence_errors(self):
        class Child:
            pid = 987654321
            returncode = 0
            def communicate(self, timeout=None):
                return b'inert output', b''
        for cleanup_fails, failure_record_fails in [(False, False), (True, False), (False, True)]:
            real_write = native_receipts.durable_json
            def fail_record(path, value):
                if Path(path).name == 'launched.json':
                    raise OSError('original launch-record failure')
                if Path(path).name == 'failed.json' and failure_record_fails:
                    raise OSError('failure-record persistence denied')
                return real_write(path, value)
            kill_error = PermissionError('owned group denied') if cleanup_fails else ProcessLookupError()
            with patch.object(native_receipts, 'durable_json', side_effect=fail_record), \
                    patch.object(native_receipts.os, 'killpg', side_effect=kill_error), \
                    patch.object(native_receipts.subprocess, 'Popen', return_value=Child()):
                with self.assertRaisesRegex(OSError, 'original launch-record failure') as raised:
                    native_receipts.recorded_run(self.out, 'secondary-failure-control',
                        [str(self.binary)], cwd=self.base, env=self.env)
            current = max((self.out/'invocations').glob('*/prepared.json'), key=lambda p: p.stat().st_mtime_ns).parent
            if failure_record_fails:
                self.assertFalse((current/'failed.json').exists())
                self.assertIn('failure-record persistence denied', str(raised.exception.__notes__))
            else:
                failed = json.loads((current/'failed.json').read_text())
                self.assertEqual(failed['error'], 'original launch-record failure')
                self.assertEqual(failed['pid'], Child.pid)
                self.assertEqual(bool(failed['cleanup_errors']), cleanup_fails)
                self.assertEqual(failed['group_cleanup'],
                    ('owned process group cleanup failed' if cleanup_fails else 'owned process group already absent') + '; child reaped')
            self.assertEqual((current/'stdout').read_bytes(), b'inert output')

    def test_record_failure_after_leader_exit_stops_owned_descendant(self):
        base = Path(tempfile.mkdtemp(prefix='ale35-cleanup-inert-', dir='/private/tmp'))
        self.addCleanup(lambda: __import__('shutil').rmtree(base, ignore_errors=True))
        for name in ['home', 'tmp', 'config', 'data', 'cache', 'state']:
            (base/name).mkdir()
        self.out.mkdir(exist_ok=True)
        script = base/'inert.py'
        program = '''import os, sys, time
from pathlib import Path
base=Path(sys.argv[1])
child=os.fork()
if child:
    while not (base/'ready').exists(): time.sleep(.005)
    os._exit(0)
os.close(1); os.close(2)
(base/'ready').write_text(str(os.getpid()))
deadline=time.monotonic()+3
while time.monotonic()<deadline:
    if (base/'returned').exists():
        (base/'continued-after-return').write_text('OWNED_DESCENDANT_CONTINUED')
        break
    time.sleep(.01)
time.sleep(.5)
os._exit(0)
'''
        script.write_text(program)
        (self.out/'inert.py').write_text(program)
        env={'PATH':'/usr/bin:/bin','HOME':str(base/'home'),'TMPDIR':str(base/'tmp'),
             'XDG_CONFIG_HOME':str(base/'config'),'XDG_DATA_HOME':str(base/'data'),
             'XDG_CACHE_HOME':str(base/'cache'),'XDG_STATE_HOME':str(base/'state'),
             'PYTHONDONTWRITEBYTECODE':'1'}
        real_write=native_receipts.durable_json
        real_popen=native_receipts.subprocess.Popen
        owned=[]
        observed={}
        def launch(*args, **kwargs):
            child=real_popen(*args, **kwargs)
            owned.append(child)
            return child
        def fail_launched(path, value):
            if Path(path).name=='launched.json':
                child=owned[0]
                deadline=time.monotonic()+2
                while child.poll() is None and time.monotonic()<deadline: time.sleep(.005)
                self.assertEqual(child.poll(), 0, 'inert leader must already have exited')
                self.assertTrue((base/'ready').exists(), 'owned descendant ready positive control')
                observed.update(leader_exit=child.returncode, group=child.pid,
                    descendant=int((base/'ready').read_text()), original_error='injected launched-record persistence error')
                raise OSError(observed['original_error'])
            return real_write(path,value)
        argv=[sys.executable,str(script),str(base)]
        try:
            with patch.object(native_receipts.subprocess,'Popen',side_effect=launch), \
                 patch.object(native_receipts,'durable_json',side_effect=fail_launched):
                with self.assertRaisesRegex(OSError,'injected launched-record persistence error'):
                    native_receipts.recorded_run(self.out, 'leader-exited-write-failure',argv,cwd=base,env=env,timeout=1)
            (base/'returned').write_text('helper already returned')
            deadline=time.monotonic()+.5
            marker=base/'continued-after-return'
            while not marker.exists() and time.monotonic()<deadline: time.sleep(.01)
            observed.update(descendant_continued_work=marker.exists(),
                            continued_marker=marker.read_text() if marker.exists() else None)
            record=next(p for p in (self.out/'invocations').glob('*/failed.json')
                        if json.loads(p.read_text()).get('pid')==owned[0].pid)
            observed['failure_record']=json.loads(record.read_text())
            (self.out/'observation.json').write_text(json.dumps(observed,indent=2)+'\n')
            self.assertFalse(marker.exists(),'owned descendant must stop before recorded_run returns on persistence failure')
            self.assertEqual(observed['failure_record']['group_cleanup'], 'SIGKILL sent to owned process group; child reaped')
            self.assertEqual(owned[0].returncode, 0)
        finally:
            for child in owned:
                try: os.killpg(child.pid,signal.SIGKILL)
                except ProcessLookupError: pass
                child.wait(timeout=1)
                deadline=time.monotonic()+.5
                absent=False
                while time.monotonic()<deadline:
                    try: os.killpg(child.pid,0)
                    except ProcessLookupError: absent=True; break
                    except PermissionError as error:
                        observed['cleanup_presence_probe_error']=str(error); break
                    time.sleep(.01)
                observed['investigator_cleanup_sigkill_sent']=True
                observed['investigator_cleanup_group_absent']=absent
            (self.out/'observation.json').write_text(json.dumps(observed,indent=2)+'\n')


if __name__ == '__main__':
    unittest.main(verbosity=2)
