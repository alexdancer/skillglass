import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import tempfile
import unittest

NATIVE = Path(__file__).resolve().parents[1] / "native"
spec = importlib.util.spec_from_file_location("invocation", NATIVE / "invocation.py")
invocation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(invocation)


class InvocationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="ale-33-receipt-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.profile = self.root / "profile.sb"
        self.profile.write_bytes(b"(version 1)\n(allow default)\n(deny network*)\n")
        self.records = self.root / "records"
        self.env = {"HOME": str(self.root), "PATH": "/usr/bin:/bin", "TERM": "dumb"}
        self.addCleanup(self.preserve_evidence)

    def preserve_evidence(self):
        evidence = os.environ.get("SKILLGLASS_EVIDENCE")
        if evidence:
            destination = Path(evidence) / "native-recorder" / self.id().split(".")[-1]
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copytree(self.root, destination)

    def run_command(self, name, argv):
        return invocation.run_recorded(name, argv, self.root, self.env, self.records, self.profile, timeout=5)

    def check_completion(self, identity, code, stdout):
        before = json.loads((self.records / f"{identity['launch_id']}.invocation.json").read_text())
        after = json.loads((self.records / f"{identity['launch_id']}.completion.json").read_text())
        self.assertEqual(before, identity)
        self.assertEqual(after["exit_code"], code)
        self.assertEqual(after["stdout_sha256"], hashlib.sha256(stdout).hexdigest())
        self.assertEqual(base64.b64decode(before["profile"]["bytes_base64"]), self.profile.read_bytes())
        self.assertEqual(before["executables"][0], invocation.identify(before["argv"][0]))

    def test_success_and_failed_exit(self):
        for argv, code in [(["/usr/bin/printf", "recorded"], 0), (["/usr/bin/false"], 1)]:
            result, identity = self.run_command("inert", argv)
            self.check_completion(identity, code, result.stdout)

    def test_record_precedes_launch(self):
        program = self.root / "owned.sh"
        program.write_text('test -f "$PI_PROBE_PARENT_INVOCATION" || exit 7\nprintf prelaunch\n')
        result, identity = invocation.run_recorded("ordering", ["/bin/sh", str(program)], self.root,
                                                 self.env, self.records, self.profile, [program], timeout=5)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b"prelaunch")
        self.check_completion(identity, 0, result.stdout)

    def test_start_failure_is_preserved(self):
        denied = self.root / "not-executable"
        denied.write_text("owned inert fixture")
        with self.assertRaises(OSError):
            self.run_command("start-error", [str(denied)])
        completions = list(self.records.glob("*.completion.json"))
        self.assertEqual(len(completions), 1)
        self.assertEqual(json.loads(completions[0].read_text())["exit_code"], -1)

    def test_unavailable_evidence_fails_closed(self):
        self.records.write_text("not a directory")
        marker = self.root / "forbidden"
        with self.assertRaises(OSError):
            self.run_command("must-not-launch", ["/bin/sh", "-c", f"printf forbidden > '{marker}'"])
        self.assertFalse(marker.exists())

    def test_inert_node_inherited_identity(self):
        node = shutil.which("node")
        if not node:
            self.fail("Node is required for inherited recorder verification")
        helper = self.root / "invocation.mjs"
        shutil.copyfile(NATIVE / "invocation.mjs", helper)
        child = self.root / "child.mjs"
        child.write_text("import fs from 'node:fs';if(!fs.existsSync(process.env.PI_PROBE_PARENT_INVOCATION))process.exit(7);process.stdout.write('inherited');")
        entry = self.root / "entry.mjs"
        entry.write_text("import {spawnRecorded} from './invocation.mjs';"
                         "const child=spawnRecorded(process.execPath,[process.argv[2]],{cwd:process.cwd(),env:{...process.env}});"
                         "child.stdout.pipe(process.stdout);child.stderr.pipe(process.stderr);"
                         "child.on('error',()=>process.exitCode=1);child.on('close',code=>process.exitCode=code);")
        result, parent = invocation.run_recorded("inert-node", [node, str(entry), str(child)], self.root,
                                               self.env, self.records, self.profile, [entry], timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, b"inherited")
        records = [json.loads(p.read_text()) for p in self.records.glob("*.invocation.json")]
        self.assertEqual(len(records), 2)
        nested = next(r for r in records if r["launch_id"] != parent["launch_id"])
        self.assertEqual(nested["profile"]["inherited_from"], parent["launch_id"])
        self.assertEqual(nested["profile"]["sha256"], parent["profile"]["sha256"])
        completion = json.loads((self.records / f"{nested['launch_id']}.completion.json").read_text())
        self.assertEqual(completion["stdout_sha256"], hashlib.sha256(b"inherited").hexdigest())
        self.assertEqual(completion["exit_code"], 0)


if __name__ == "__main__":
    unittest.main()
