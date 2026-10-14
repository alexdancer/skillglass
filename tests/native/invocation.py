"""Invocation-bound sampling for disposable subprocess evidence."""
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess
import uuid


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def identify(path):
    resolved = Path(path).resolve(strict=True)
    return {"path": str(resolved), "sha256": digest(resolved.read_bytes())}


def persist(directory, launch_id, kind, value):
    path = Path(directory) / f"{launch_id}.{kind}.json"
    with path.open("x", encoding="utf-8") as stream:
        json.dump(value, stream, indent=2)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    return path


def run_recorded(name, command, cwd, env, directory, profile_path, artifacts=(), timeout=90):
    directory = Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    launch_id = uuid.uuid4().hex
    profile = Path(profile_path).read_bytes()
    environment = dict(env)
    environment.update(PI_PROBE_INVOCATIONS=str(directory),
                       PI_PROBE_PROFILE=str(profile_path),
                       PI_PROBE_PARENT_INVOCATION=str(directory / f"{launch_id}.invocation.json"))
    executables = [identify(command[0])]
    if Path(command[0]).name == "sandbox-exec":
        executables.append(identify(command[3]))
    invocation = {"launch_id": launch_id, "name": name, "argv": list(command),
                  "cwd": str(cwd), "environment": environment,
                  "executables": executables, "artifacts": [identify(p) for p in artifacts],
                  "profile": {"path": str(profile_path), "bytes_base64": base64.b64encode(profile).decode(),
                              "sha256": digest(profile)}}
    persist(directory, launch_id, "invocation", invocation)
    try:
        result = subprocess.run(command, cwd=cwd, env=environment, capture_output=True, timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as error:
        stdout = getattr(error, "stdout", None) or b""
        stderr = getattr(error, "stderr", None) or b""
        persist(directory, launch_id, "completion", {"launch_id": launch_id, "exit_code": -1,
                "error": str(error), "stdout_sha256": digest(stdout), "stderr_sha256": digest(stderr)})
        raise
    persist(directory, launch_id, "completion", {"launch_id": launch_id, "exit_code": result.returncode,
            "stdout_sha256": digest(result.stdout), "stderr_sha256": digest(result.stderr)})
    return result, invocation
