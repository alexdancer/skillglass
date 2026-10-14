"""Verify the public retired entrypoint without importing installed Hermes."""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

REPOSITORY = Path(__file__).resolve().parents[2]
DIAGNOSTIC = (
    "verify_hermes.py is retired: uncontained native startup validation is disabled. "
    "Use tools/verify_hermes_policy.py for bounded environment/config helper validation only. "
    "Full startup/catalog/plugins/sessions/writers/reload remain unverified.\n"
)


def manifest(root):
    return {str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in root.rglob("*") if path.is_file()}


class RetiredHermesVerifier(unittest.TestCase):
    def test_exact_refusal_before_import_arguments_or_filesystem_effects(self):
        with tempfile.TemporaryDirectory(prefix="ale31-retired-", dir="/private/tmp") as temporary:
            root = Path(temporary).resolve()
            self.assertFalse(any((parent / ".git").exists() for parent in (root, *root.parents)))
            source = root / "owned-source"
            package = source / "hermes_cli"
            package.mkdir(parents=True)
            (package / "__init__.py").write_text('__version__ = "INERT-NOT-PINNED"\n')
            (source / ".env").write_text("OWNED_SOURCE_ENV_CANARY\n")
            (source / "config.yaml").write_text("inert: true\n")
            (package / "main.py").write_text(
                "import os, pathlib, subprocess, sys\n"
                "root = pathlib.Path(__file__).resolve().parents[2]\n"
                "(root / 'import-marker').write_text('imported')\n"
                "(root / 'read-marker').write_text((root / 'owned-source/.env').read_text())\n"
                "(root / 'outside-fixture-write').write_text('write')\n"
                "subprocess.run([sys.executable, '-c', "
                "'import pathlib; pathlib.Path(' + repr(str(root / 'child-marker')) + "
                "').write_text(\"child\")'], check=True)\n"
                "raise SystemExit(86)\n"
            )
            (root / "home").mkdir()
            (root / "tmp").mkdir()
            script = root / "verify_hermes.py"
            shutil.copyfile(REPOSITORY / "tools/verify_hermes.py", script)
            script.chmod(0o444)
            env = {"HOME": str(root / "home"), "TMPDIR": str(root / "tmp"),
                   "GOTMPDIR": str(root / "tmp"), "PATH": "/usr/bin:/bin",
                   "PYTHONPATH": str(source), "PYTHONDONTWRITEBYTECODE": "1"}
            before = manifest(root)
            directories = sorted(str(path.relative_to(root)) for path in root.rglob("*") if path.is_dir())
            cases = [[], [str(source)], [str(source), str(root / "output")],
                     [str(root / "missing"), str(root / "output")],
                     [str(source), str(root / "output"), "unexpected"], ["--help"]]
            for args in cases:
                with self.subTest(args=args):
                    completed = subprocess.run([sys.executable, str(script), *args], cwd=root,
                                               env=env, capture_output=True, text=True, timeout=10)
                    self.assertEqual(completed.returncode, 2)
                    self.assertEqual(completed.stdout, "")
                    self.assertEqual(completed.stderr, DIAGNOSTIC)
                    self.assertEqual(manifest(root), before)
                    self.assertEqual(sorted(str(path.relative_to(root)) for path in root.rglob("*") if path.is_dir()), directories)
                    self.assertEqual(list((root / "tmp").iterdir()), [])
                    for name in ("import-marker", "read-marker", "outside-fixture-write", "child-marker", "output"):
                        self.assertFalse((root / name).exists(), name)
                    print({"args": args, "exit": completed.returncode, "stdout": completed.stdout,
                           "stderr": completed.stderr, "fixture_unchanged": True})


if __name__ == "__main__":
    unittest.main(verbosity=2)
