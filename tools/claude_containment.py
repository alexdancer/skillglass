"""Fail-closed macOS containment for disposable native verification."""

import hashlib
import json
import os
import pathlib
import pwd
import subprocess
import sys
import tempfile


CONTROL = '''import json, pathlib, socket, subprocess, sys
root, modeled, outside, system = map(pathlib.Path, sys.argv[1:5])
results = {}
for label, action in [
    ("fixture_read", lambda: (root / "control-input").read_text()),
    ("fixture_write", lambda: (root / "control-output").write_text("fixture")),
    ("modeled_live_home_read", lambda: (modeled / "settings").read_text()),
    ("modeled_system_settings_read", lambda: (system / "managed-settings.json").read_text()),
    ("outside_write", lambda: (outside / "escape").write_text("escape")),
    ("symlink_write", lambda: (root / "escape-link" / "symlink-escape").write_text("escape")),
    ("network", lambda: socket.socket().bind(("127.0.0.1", 0))),
]:
    try:
        action()
        results[label] = "allowed"
    except OSError:
        results[label] = "denied"
if len(sys.argv) == 5:
    child = subprocess.run([sys.executable] + sys.argv + ["--child"], capture_output=True,
                           text=True, timeout=10, check=True)
    results.update({"inherited_child_" + label: value for label, value in json.loads(child.stdout).items()})
print(json.dumps(results))
'''


class NativeContainment:
    def __init__(self, root, cwd, environment, binary, output):
        self.root = root.resolve()
        self.cwd = cwd.resolve()
        self.environment = environment
        self.binary = pathlib.Path(binary).resolve(strict=True)
        self.output = output
        self.calls = []
        self.ready = False
        # Both the OS account home and any caller-selected home are denied.
        live_homes = {pathlib.Path.home().resolve(),
                      pathlib.Path(pwd.getpwuid(os.getuid()).pw_dir).resolve()}
        self.controls = pathlib.Path(tempfile.mkdtemp(prefix="ale29-controls-", dir="/private/tmp")).resolve()
        modeled, outside = self.controls / "modeled-live-home", self.controls / "outside"
        system = self.controls / "modeled-system-settings"
        modeled.mkdir()
        outside.mkdir()
        system.mkdir()
        (system / "managed-settings.json").write_text("inert managed-settings model")
        (modeled / "settings").write_text("inert live-home model")
        (self.root / "control-input").write_text("fixture")
        (self.root / "escape-link").symlink_to(outside, target_is_directory=True)
        system_paths = {pathlib.Path("/Library/Application Support/ClaudeCode"),
                        pathlib.Path("/Library/Managed Preferences"),
                        pathlib.Path("/Library/Preferences/com.anthropic.claudecode.plist")}
        denied_reads = live_homes | system_paths | {modeled, system}
        if any(self.root.is_relative_to(home) for home in denied_reads):
            raise RuntimeError("fixture must be outside caller and OS home")
        self.profile = self.root / "native-contained.sb"
        quote = json.dumps
        profile = ('(version 1)\n(allow default)\n(deny network*)\n'
                   '(deny file-write*)\n'
                   f'(allow file-write* (subpath {quote(str(self.root))}))\n')
        profile += ''.join(f'(deny file-read* (subpath {quote(str(home))}))\n'
                           for home in sorted(denied_reads))
        # Only the selected executable may be read from a denied home.
        profile += f'(allow file-read* (literal {quote(str(self.binary))}))\n'
        self.profile.write_text(profile)
        (output / "native-contained.sb").write_text(profile)
        self.prefix = ["/usr/bin/sandbox-exec", "-f", str(self.profile)]
        control = self.root / "containment-control.py"
        control.write_text(CONTROL)
        arguments = [sys.executable, str(control), str(self.root), str(modeled), str(outside), str(system)]
        positive = subprocess.run(arguments, cwd=self.cwd, env=self.environment,
                                  capture_output=True, text=True, timeout=10, check=True)
        positive_results = json.loads(positive.stdout)
        if set(positive_results.values()) != {"allowed"}:
            raise RuntimeError("inert positive controls failed")
        (outside / "escape").unlink()
        (outside / "symlink-escape").unlink()
        contained = subprocess.run(self.prefix + arguments, cwd=self.cwd,
                                   env=self.environment, capture_output=True,
                                   text=True, timeout=10, check=True)
        results = json.loads(contained.stdout)
        expected = {label: "allowed" if label.removeprefix("inherited_child_") in {"fixture_read", "fixture_write"}
                    else "denied" for label in positive_results}
        receipt = {"fixture": str(self.root), "cwd": str(self.cwd),
                   "write_boundary": str(self.root), "denied_live_homes": list(map(str, sorted(live_homes))),
                   "modeled_live_home": str(modeled), "outside": str(outside),
                   "denied_system_inputs": list(map(str, sorted(system_paths))),
                   "modeled_system_settings": str(system),
                   "positive": positive_results, "contained": results,
                   "profile_sha256": hashlib.sha256(profile.encode()).hexdigest(),
                   "commands": [arguments, self.prefix + arguments],
                   "exit_codes": [positive.returncode, contained.returncode]}
        (output / "containment.json").write_text(json.dumps(receipt, indent=2) + "\n")
        if results != expected or (outside / "escape").exists() or (outside / "symlink-escape").exists():
            raise RuntimeError("native containment controls failed; native launch refused")
        self.ready = True

    def command(self, arguments):
        if not self.ready:
            raise RuntimeError("native containment was not validated")
        self.calls.append({"argv": self.prefix + [str(self.binary)] + arguments,
                           "cwd": str(self.cwd), "environment": dict(self.environment),
                           "binary": str(self.binary),
                           "binary_sha256": hashlib.sha256(self.binary.read_bytes()).hexdigest(),
                           "profile_sha256": hashlib.sha256(self.profile.read_bytes()).hexdigest()})
        (self.output / "invocations.json").write_text(json.dumps(self.calls, indent=2) + "\n")
        return self.calls[-1]["argv"]
