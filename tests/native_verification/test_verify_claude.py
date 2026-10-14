"""Inert executable checks of the public native verification helper."""
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

REPOSITORY = pathlib.Path(__file__).resolve().parents[2]

# Literal catalogs merely let the helper reach every subprocess route.
# Native semantics are validated independently with actual Claude.
INERT = '''#!INTERPRETER
import json, os, pathlib, sys
home = pathlib.Path(os.environ["HOME"])
with (home / "calls.jsonl").open("a") as out:
 out.write(json.dumps({"args": sys.argv[1:], "HOME": os.environ["HOME"],
  "config": os.environ.get("CLAUDE_CONFIG_DIR"), "cwd": os.getcwd(),
  "sentinel": os.environ.get("ALE29_AMBIENT_SENTINEL")}) + "\\n")
if sys.argv[1:] == ["--version"]:
 print("2.1.282 (Claude Code)")
elif sys.argv[1:3] == ["plugin", "list"]:
 print("[]")
else:
 sys.stdin.readline()
 counter = home / "counter"
 count = int(counter.read_text()) if counter.exists() else 0
 counter.write_text(str(count + 1))
 commands = []
 def add(name, description="fixture"):
  commands.append({"name": name, "description": description})
 if count not in [1]: add("collision", "PERSONAL")
 if count == 0:
  add("nested-collision", "CHILD")
  for index in range(14):
   if index < 7: add("user-invocable-" + str(index))
   add("disable-model-invocation-" + str(index))
 if count == 11: add("alternate")
 if count in [4, 5, 8, 9]: add("demo:action", "LOCAL-SOURCE")
 if count == 15: add("root-demo:root-action", "ROOT-PLUGIN-SKILL")
 print(json.dumps({"response": {"response": {"commands": commands, "account": {}}}}), flush=True)
'''


class NativeHelper(unittest.TestCase):
    def setUp(self):
        self.root = pathlib.Path(tempfile.mkdtemp(prefix="ale29-helper-test-", dir="/private/tmp")).resolve()
        self.native = self.root / "inert-claude"
        self.native.write_text(INERT.replace("INTERPRETER", sys.executable))
        self.native.chmod(0o700)
        self.output = self.root / "evidence"
        self.output.mkdir()
        self.ambient = self.root / "ambient"
        self.ambient.mkdir()
        self.environment = {"HOME": str(self.ambient), "CLAUDE_CONFIG_DIR": str(self.ambient),
                            "PATH": "/usr/bin:/bin", "TMPDIR": str(self.root),
                            "ALE29_AMBIENT_SENTINEL": "inert ambient marker"}

    def test_every_native_child_including_version_uses_fixture(self):
        command = [sys.executable, str(REPOSITORY / "tools/verify_claude.py"),
                   str(self.output), "--claude", str(self.native)]
        result = subprocess.run(command, env=self.environment, cwd=self.root,
                                capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stderr)
        summary = json.loads((self.output / "summary.json").read_text())
        fixture = pathlib.Path(summary["fixture"])
        calls = [json.loads(row) for row in (fixture / "home/calls.jsonl").read_text().splitlines()]
        self.assertEqual(len(calls), 20)
        self.assertEqual(calls[0]["args"], ["--version"])
        self.assertFalse((self.ambient / "calls.jsonl").exists())
        receipts = json.loads((self.output / "invocations.json").read_text())
        self.assertEqual(len(receipts), len(calls))
        for call, receipt in zip(calls, receipts):
            self.assertEqual(call["HOME"], str(fixture / "home"))
            self.assertEqual(call["cwd"], str(fixture / "repo/nested"))
            self.assertIsNone(call["sentinel"])
            self.assertTrue(pathlib.Path(call["config"]).is_relative_to(fixture))
            self.assertEqual(receipt["environment"]["CLAUDE_CONFIG_DIR"], call["config"])
            self.assertEqual(receipt["argv"][:3], ["/usr/bin/sandbox-exec", "-f", str(fixture / "native-contained.sb")])
        controls = json.loads((self.output / "containment.json").read_text())
        self.assertEqual(controls["write_boundary"], str(fixture.resolve()))
        self.assertEqual(set(controls["positive"].values()), {"allowed"})
        expected = {"fixture_read": "allowed", "fixture_write": "allowed",
            "modeled_live_home_read": "denied", "modeled_system_settings_read": "denied", "outside_write": "denied", "symlink_write": "denied", "network": "denied"}
        expected.update({"inherited_child_" + label: value for label, value in dict(expected).items()})
        self.assertEqual(controls["contained"], expected)

    def containment_module(self):
        spec = importlib.util.spec_from_file_location("claude_containment", REPOSITORY / "tools/claude_containment.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def test_failed_controls_refuse_native_launch(self):
        module = self.containment_module()
        fixture = self.root / "fixture"
        fixture.mkdir()
        # A broken policy that reports every access permitted must fail closed.
        with mock.patch.object(module, "CONTROL", module.CONTROL.replace('results[label] = "denied"', 'results[label] = "allowed"')):
            with self.assertRaises(RuntimeError):
                module.NativeContainment(fixture, fixture, self.environment, self.native, self.output)
        self.assertFalse((self.ambient / "calls.jsonl").exists())

    def test_failed_sandbox_never_runs_native(self):
        module = self.containment_module()
        fixture = self.root / "fixture"
        fixture.mkdir()
        environment = dict(self.environment, HOME=str(fixture))
        containment = module.NativeContainment(fixture, fixture, environment, self.native, self.output)
        containment.profile.write_text("invalid sandbox policy")
        result = subprocess.run(containment.command(["--version"]), cwd=fixture, env=environment,
                                capture_output=True, text=True, timeout=10)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((fixture / "calls.jsonl").exists())


if __name__ == "__main__":
    unittest.main()
