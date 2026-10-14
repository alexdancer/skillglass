"""Durable invocation evidence for the bounded native verification controls."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import uuid


def utc_now():
    return datetime.now(timezone.utc).isoformat()


def durable_file(path, data):
    """Publish new bytes atomically and sync both the file and its directory."""
    path = Path(path)
    temporary = path.with_name('.' + path.name + '.' + uuid.uuid4().hex)
    try:
        with temporary.open('xb') as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        # link is exclusive: historical records cannot be replaced.
        os.link(temporary, path)
        temporary.unlink()
        descriptor = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    finally:
        temporary.unlink(missing_ok=True)


def durable_json(path, value):
    durable_file(path, (json.dumps(value, indent=2) + '\n').encode())


def recorded_run(out, label, argv, *, cwd, env, binary=None, profile=None,
                 source=None, runtime_data=None, timeout=60, child_records=None):
    """Persist final context before Popen; retain starts even if launch is interrupted."""
    out = Path(out).resolve()
    journal = out / 'invocations'
    journal.mkdir(parents=True, exist_ok=True)
    invocation = journal / uuid.uuid4().hex
    invocation.mkdir()
    # Sync the new directory entries before publishing a start record.
    for directory in (out.parent, out, journal):
        descriptor = os.open(directory, os.O_RDONLY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    executable = Path(binary or shutil.which(argv[0], path=env.get('PATH')) or argv[0]).resolve(strict=True)
    launcher = Path(shutil.which(argv[0], path=env.get('PATH')) or argv[0]).resolve(strict=True)
    resolved_cwd = str(Path(cwd).resolve(strict=True))
    record = {'invocation_id': invocation.name, 'label': label, 'state': 'prepared',
              'prepared_at': utc_now(), 'binary': str(executable),
              'binary_sha256': hashlib.sha256(executable.read_bytes()).hexdigest(),
              'launcher': str(launcher), 'launcher_sha256': hashlib.sha256(launcher.read_bytes()).hexdigest(),
              'argv': list(argv[3:] if profile else argv), 'wrapped_argv': list(argv),
              'cwd': resolved_cwd, 'environment': dict(env), 'runtime_data': runtime_data or [],
              'record_directory': str(invocation), 'stdout': str(invocation/'stdout'),
              'stderr': str(invocation/'stderr')}
    if source:
        record.update(source)
    if child_records:
        record['child_records'] = str(Path(child_records).resolve())
    if profile:
        path = Path(profile).resolve(strict=True)
        policy = path.read_bytes()
        record.update(profile_path=str(path), profile=policy.decode(),
                      profile_sha256=hashlib.sha256(policy).hexdigest(),
                      profile_snapshot=str(invocation/'profile.sb'))
        durable_file(invocation/'profile.sb', policy)
    durable_json(invocation/'prepared.json', record)
    child = None
    stdout, stderr = b'', b''
    timed_out = False
    cleanup = 'no child was returned by Popen'
    try:
        child = subprocess.Popen(argv, cwd=resolved_cwd, env=env, stdout=subprocess.PIPE,
                                 stderr=subprocess.PIPE, start_new_session=True)
        durable_json(invocation/'launched.json', {'invocation_id': invocation.name,
                     'state': 'launched', 'launch_observed_at': utc_now(), 'pid': child.pid,
                     'owned_process_group': child.pid})
        try:
            stdout, stderr = child.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
        finally:
            try:
                os.killpg(child.pid, signal.SIGKILL)
                cleanup = 'SIGKILL sent to owned process group; child reaped'
            except ProcessLookupError:
                cleanup = 'owned process group already absent; child reaped'
            stdout, stderr = child.communicate()
        durable_file(invocation/'stdout', stdout)
        durable_file(invocation/'stderr', stderr)
        record.update(state='timed-out' if timed_out else 'completed',
                      finished_at=utc_now(), pid=child.pid, owned_process_group=child.pid,
                      group_cleanup=cleanup, timed_out=timed_out, exit=child.returncode)
        durable_json(invocation/'completed.json', record)
    except BaseException as error:
        if child is not None and child.poll() is None:
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            stdout, stderr = child.communicate()
            cleanup = 'owned process group stopped and child reaped after failure'
        for name, data in [('stdout', stdout), ('stderr', stderr)]:
            if not (invocation/name).exists():
                durable_file(invocation/name, data)
        failure = {**record, 'state': 'failed', 'finished_at': utc_now(),
                   'error_type': type(error).__name__, 'error': str(error),
                   'group_cleanup': cleanup, 'timed_out': timed_out,
                   'exit': child.returncode if child else None}
        if child:
            failure.update(pid=child.pid, owned_process_group=child.pid)
        durable_json(invocation/'failed.json', failure)
        raise
    result = subprocess.CompletedProcess(argv, child.returncode, stdout, stderr)
    return result, record
