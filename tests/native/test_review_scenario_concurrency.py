"""Cross-process controller regression; product transport is a bounded test double.

Real sandboxed concurrent CLI reads are a separate recorded surface check.
"""
import argparse
from contextlib import redirect_stderr, redirect_stdout
import fcntl
import io
import json
import multiprocessing
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'tools'))
import review_scenario as provider

BOUND_SECONDS = 10


def controller_worker(state, operation, label, connection, release):
    """Trace real kernel lock contention and hold reads at an explicit barrier."""
    native_flock = fcntl.flock
    output = Path(state['output'])
    stdout, stderr = io.BytesIO(), io.BytesIO()

    def trace(kind, **details):
        connection.send({'event': kind, 'label': label, **details})

    def observed_flock(descriptor, mode):
        if mode != fcntl.LOCK_EX:
            return native_flock(descriptor, mode)
        trace('lock-attempt')
        try:
            native_flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            trace('lock-blocked')
            native_flock(descriptor, fcntl.LOCK_EX)
        trace('lock-acquired', sealed_results=len(list(output.glob('call-*/result.json'))))

    def bounded_read(argv, **kwargs):
        trace('child-entered')
        if not release.wait(BOUND_SECONDS):
            raise RuntimeError('bounded read barrier timed out')
        return subprocess.CompletedProcess(argv, 0, (json.dumps({'read': label}) + '\n').encode(), b'')

    try:
        args = argparse.Namespace(arguments=['help'], save=label + '.json', name=label + '.json')
        with mock.patch.object(provider, 'load_session', return_value=state), \
                mock.patch.object(provider.fcntl, 'flock', side_effect=observed_flock), \
                mock.patch.object(provider.subprocess, 'run', side_effect=bounded_read), \
                mock.patch.object(provider.sys, 'stdin', io.StringIO('{"put": "b"}')), \
                redirect_stdout(io.TextIOWrapper(stdout, write_through=True)), \
                redirect_stderr(io.TextIOWrapper(stderr, write_through=True)):
            try:
                result = getattr(provider, operation)(args)
                captured = {'exit': result or 0, 'stdout': stdout.getvalue().decode(),
                            'stderr': stderr.getvalue().decode()}
            except Exception as error:  # Negative control preserves the precise guard.
                captured = {'error': type(error).__name__, 'message': str(error),
                            'stdout': stdout.getvalue().decode(), 'stderr': stderr.getvalue().decode()}
        (output / (label + '-controller.json')).write_text(json.dumps(captured, indent=2) + '\n')
        trace('done', **captured)
    finally:
        connection.close()


class ReviewScenarioConcurrency(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='review-provider-test-', dir='/private/tmp')
        self.base = Path(self.temporary.name)
        self.fixture, self.output = self.base / 'fixture', self.base / 'output'
        (self.fixture / 'artifacts').mkdir(parents=True)
        (self.fixture / 'source').mkdir()
        (self.fixture / 'source/SKILL.md').write_text('fixture instructions\n')
        self.output.mkdir()
        self.state = {'fixture': str(self.fixture), 'output': str(self.output),
                      'profile': 'test-transport-only', 'product': 'test-transport-only',
                      'cwd': str(self.fixture), 'environment': {},
                      'product_sha256': 'test-double', 'profile_sha256': 'test-double',
                      'initial_manifest': provider.manifest(self.fixture)}
        self.context = multiprocessing.get_context('spawn')
        self.processes, self.traces = [], []

    def tearDown(self):
        for process in self.processes:
            process.join(BOUND_SECONDS)
            if process.is_alive():
                process.terminate()
                process.join(BOUND_SECONDS)
        (self.output / 'barrier-trace.json').write_text(json.dumps(self.traces, indent=2) + '\n')
        evidence = os.environ.get('SKILLGLASS_PROVIDER_TEST_EVIDENCE')
        if evidence:
            shutil.copytree(self.base, Path(evidence) / self._testMethodName)
        self.temporary.cleanup()

    def start(self, operation, label, released=False):
        receiver, sender = self.context.Pipe(duplex=False)
        release = self.context.Event()
        if released:
            release.set()
        process = self.context.Process(target=controller_worker,
                                       args=(self.state, operation, label, sender, release))
        process.start()
        sender.close()
        self.processes.append(process)
        return receiver, release, process

    def until(self, receiver, event):
        while True:
            self.assertTrue(receiver.poll(BOUND_SECONDS), 'bounded process barrier timed out: ' + event)
            message = receiver.recv()
            self.traces.append(message)
            if message['event'] == event:
                return message
            self.assertNotEqual(message['event'], 'done', 'worker finished before barrier: ' + str(message))

    def result_receipts(self):
        return [(path, json.loads(path.read_text()))
                for path in sorted(self.output.glob('call-*/result.json'))]

    def check_no_write(self, receipts):
        for path, result in receipts:
            self.assertEqual(result['exit_code'], 0)
            self.assertIs(result['no_write'], True)
            self.assertEqual((path.parent / 'tree-before.json').read_bytes(),
                             (path.parent / 'tree-after.json').read_bytes())
            self.assertEqual(Path(result['saved']).read_bytes(), (path.parent / 'stdout').read_bytes())

    def overlap(self, operation):
        first, release, process_a = self.start('run', 'a')
        self.until(first, 'child-entered')  # A already holds its before snapshot.
        second, _, process_b = self.start(operation, 'b', released=True)
        self.until(second, 'lock-blocked')  # Proven kernel contention, no sleep heuristic.
        self.assertFalse((self.fixture / 'artifacts/b.json').exists())
        release.set()
        completed_a = self.until(first, 'done')
        acquired_b = self.until(second, 'lock-acquired')
        completed_b = self.until(second, 'done')
        self.assertEqual(acquired_b['sealed_results'], 1, 'A result must seal before B enters')
        self.assertEqual(completed_a.get('exit'), 0, completed_a)
        self.assertEqual(completed_b.get('exit'), 0, completed_b)
        for process in (process_a, process_b):
            process.join(BOUND_SECONDS)
            self.assertEqual(process.exitcode, 0)
        return completed_b

    def test_concurrent_runs_serialize_snapshot_save_and_result(self):
        self.overlap('run')
        receipts = self.result_receipts()
        self.assertEqual(len(receipts), 2)
        self.check_no_write(receipts)
        self.assertEqual((self.fixture / 'artifacts/a.json').read_bytes(), b'{"read": "a"}\n')
        self.assertEqual((self.fixture / 'artifacts/b.json').read_bytes(), b'{"read": "b"}\n')
        second = next(path for path, result in receipts if result['saved'].endswith('/b.json'))
        before = json.loads((second.parent / 'tree-before.json').read_text())
        self.assertIn('artifacts/a.json', [row['path'] for row in before])

    def test_put_waits_until_active_run_result_is_sealed(self):
        self.overlap('put')
        receipts = self.result_receipts()
        self.assertEqual(len(receipts), 1)
        self.check_no_write(receipts)
        self.assertEqual(json.loads((self.fixture / 'artifacts/b.json').read_text()), {'put': 'b'})

    def test_status_waits_for_a_coherent_snapshot(self):
        result = self.overlap('status')
        self.assertIs(json.loads(result['stdout'])['product_tree_unchanged'], True)
        self.check_no_write(self.result_receipts())

    def test_real_artifact_mutation_still_triggers_no_write_guard(self):
        receiver, release, process = self.start('run', 'a')
        self.until(receiver, 'child-entered')
        # Deliberately bypass controller locking as the mutation negative control.
        (self.fixture / 'artifacts/unexpected.json').write_text('{"changed": true}\n')
        release.set()
        result = self.until(receiver, 'done')
        self.assertEqual(result.get('error'), 'RuntimeError')
        self.assertEqual(result.get('message'), 'read-only product operation changed the fixture')
        self.assertEqual(self.result_receipts(), [])
        call = next(self.output.glob('call-*'))
        before = json.loads((call / 'tree-before.json').read_text())
        after = json.loads((call / 'tree-after.json').read_text())
        self.assertNotIn('artifacts/unexpected.json', [row['path'] for row in before])
        self.assertIn('artifacts/unexpected.json', [row['path'] for row in after])
        process.join(BOUND_SECONDS)
        self.assertEqual(process.exitcode, 0)


if __name__ == '__main__':
    unittest.main()
