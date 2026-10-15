"""Exercise the developer helper's real containment with an inert executable."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

TOOLS = Path(__file__).resolve().parents[2] / "tools"
SPEC = importlib.util.spec_from_file_location("controls", TOOLS / "claude_containment.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ClaudeControlsContainmentTests(unittest.TestCase):
    def test_inert_execution_and_inherited_isolation(self):
        with tempfile.TemporaryDirectory(prefix="ale30-inert-", dir="/private/tmp") as scratch:
            root = Path(scratch).resolve()
            out = root / "receipts"
            out.mkdir()
            environment = {"HOME": str(root), "PATH": "/usr/bin:/bin", "TMPDIR": str(root)}
            boundary = MODULE.NativeContainment(root, root, environment, "/usr/bin/true", out)
            receipt = json.loads((out / "containment.json").read_text())
            self.assertEqual(len(receipt["contained"]), 14)
            for label, state in receipt["contained"].items():
                expected = "allowed" if label.removeprefix("inherited_child_") in {"fixture_read", "fixture_write"} else "denied"
                self.assertEqual(state, expected, label)
            result = subprocess.run(boundary.command([]), cwd=root, env=environment, check=False)
            self.assertEqual(result.returncode, 0)
            invocation = json.loads((out / "invocations.json").read_text())[0]
            self.assertEqual(invocation["binary"], "/usr/bin/true")
            self.assertIn("binary_sha256", invocation)
            self.assertIn("profile_sha256", invocation)

    def test_unvalidated_boundary_cannot_launch(self):
        boundary = object.__new__(MODULE.NativeContainment)
        boundary.ready = False
        with self.assertRaisesRegex(RuntimeError, "not validated"):
            boundary.command([])


if __name__ == "__main__":
    unittest.main()
