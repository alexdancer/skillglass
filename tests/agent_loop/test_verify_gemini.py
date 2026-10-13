"""Exercise the public native verifier with an inert package, never Gemini."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import unittest

HELPER = Path(__file__).resolve().parents[2] / "tools/verify_gemini.py"


@unittest.skipUnless(os.uname().sysname == "Darwin", "sandbox-exec requires macOS")
class NativeHelperContainment(unittest.TestCase):
    def test_actual_public_helper_contains_parent_and_inherited_child(self):
        node = shutil.which("node") or "/Users/alex/.hermes/node/bin/node"
        node = Path(node).resolve(strict=True)
        with tempfile.TemporaryDirectory(prefix="ale37-inert-", dir="/private/tmp") as temporary:
            root = Path(temporary)
            package = root / "package"
            (package / "bundle").mkdir(parents=True)
            evidence = root / "evidence"
            excluded = root / "modeled-home/canary.txt"
            excluded.parent.mkdir()
            excluded.write_text("MODELED_HOME_CANARY")
            outside = root / "outside.txt"
            outside.write_text("parent-positive")
            self.assertEqual(excluded.read_text(), "MODELED_HOME_CANARY")
            self.assertEqual(outside.read_text(), "parent-positive")
            outside.unlink()
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                listener.listen()
                port = listener.getsockname()[1]
                with socket.create_connection(("127.0.0.1", port), timeout=1):
                    connection, _ = listener.accept()
                    connection.close()
                fake = package / "bundle/gemini.js"
                fake.write_text(r'''const fs = require('fs');
const path = require('path');
const cp = require('child_process');
const net = require('net');
const outside = OUTSIDE;
const excluded = EXCLUDED;
const result = {mode: process.argv[2] === 'child' ? 'child' : 'parent'};
function attempt(key, fn) {
  try { fn(); result[key] = true; } catch (error) { result[key] = false; result[key+'Error'] = error.code; }
}
attempt('fixtureWrite', () => fs.writeFileSync(path.join(process.env.HOME, result.mode+'.txt'), 'allowed'));
attempt('outsideWrite', () => fs.writeFileSync(outside, 'forbidden'));
attempt('excludedRead', () => fs.readFileSync(excluded));
const escape = path.join(process.env.HOME, result.mode+'-escape');
fs.symlinkSync(path.dirname(outside), escape);
attempt('symlinkWrite', () => fs.writeFileSync(path.join(escape, 'escape.txt'), 'forbidden'));
attempt('symlinkRead', () => fs.readFileSync(path.join(escape, 'modeled-home/canary.txt')));
if (result.mode === 'parent') {
  result.child = JSON.parse(cp.execFileSync(process.execPath, [__filename, 'child'], {encoding: 'utf8'}));
}
const connection = net.connect({host:'127.0.0.1', port:PORT});
let emitted = false;
function done(allowed, error) {
  if (emitted) return; emitted = true;
  result.network = allowed; result.networkError = error;
  connection.destroy(); console.log(JSON.stringify(result));
  process.exit(result.mode === 'parent' ? 17 : 0);
}
connection.once('connect', () => done(true));
connection.once('error', error => done(false, error.code));
setTimeout(() => done(false, 'timeout'), 1000);
'''.replace("OUTSIDE", json.dumps(str(outside))).replace("EXCLUDED", json.dumps(str(excluded))).replace("PORT", str(port)))
                command = ["/usr/bin/python3", str(HELPER), str(package), str(evidence)]
                env = {"HOME": str(root / "launcher-home"), "PATH": str(node.parent)+":/usr/bin:/bin"}
                process = subprocess.run(command, cwd=root, env=env, capture_output=True, text=True, timeout=20)
            self.assertNotEqual(process.returncode, 0, "inert version exits 17 deliberately")
            self.assertTrue((evidence / "version.txt").exists(), process.stderr)
            output = os.environ.get("ALE37_HELPER_EVIDENCE")
            if output:
                target = Path(output)
                shutil.copytree(evidence, target, dirs_exist_ok=True)
                (target / "fake-gemini.js").write_bytes(fake.read_bytes())
                (target / "stdout").write_text(process.stdout)
                (target / "stderr").write_text(process.stderr)
            self.assertTrue((evidence / "version.txt").read_text().startswith("{"), (evidence / "version.txt").read_text())
            observation = json.loads((evidence / "version.txt").read_text().strip().splitlines()[0])
            for result in (observation, observation["child"]):
                self.assertTrue(result["fixtureWrite"], result)
                for key in ("outsideWrite", "excludedRead", "symlinkWrite", "symlinkRead", "network"):
                    self.assertFalse(result[key], result)
                    self.assertIn(result[key+"Error"], ("EPERM", "EACCES"), result)
            self.assertFalse(outside.exists())
            self.assertFalse((root / "escape.txt").exists())
            receipt = json.loads((evidence / "receipt.json").read_text())
            self.assertEqual(receipt["cases"][0]["exit"], 17)
            self.assertEqual(receipt["cases"][0]["wrapper_sha256_at_launch"], hashlib.sha256(fake.read_bytes()).hexdigest())
            self.assertIn("deny file-write*", (evidence / "profile.sb").read_text())



if __name__ == "__main__":
    unittest.main()
