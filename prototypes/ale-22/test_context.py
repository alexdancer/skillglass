import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent
SCRIPT = ROOT / "terminal_prototype.py"
SPEC = importlib.util.spec_from_file_location("ale22_prototype", SCRIPT)
PROTOTYPE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = PROTOTYPE
SPEC.loader.exec_module(PROTOTYPE)


class ContextContractTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.fixture = Path(self.temporary.name) / "fixture.json"
        self.document = json.loads((ROOT / "fixture.json").read_text())
        self.project = self.document["fixture"]["default_project"]
        project_rule = next(rule for rule in self.document["rules"] if rule.get("project") == self.project)
        unrelated = copy.deepcopy(project_rule)
        unrelated.update(project="/fixture/projects/other", layer="unrelated project override")
        equal_precedence = copy.deepcopy(project_rule)
        equal_precedence["layer"] = "equal-precedence project rule"
        global_rule = next(rule for rule in self.document["rules"] if rule["source_id"] == "shared.research" and rule["application"] == "Codex" and rule.get("project") is None)
        equal_global = copy.deepcopy(global_rule)
        equal_global["layer"] = "equal-precedence global rule"
        self.document["rules"].extend([unrelated, equal_precedence, equal_global])
        self.fixture.write_text(json.dumps(self.document))
        self.original = self.fixture.read_bytes()

    def command(self, *args):
        return [sys.executable, "-B", str(SCRIPT), *args, "--fixture", str(self.fixture)]

    def browser(self, steps):
        code, text = PROTOTYPE.run_pty_dialog(self.command("browse"), os.environ.copy(), steps)
        self.assertEqual(code, 0, text)
        self.assertEqual(self.fixture.read_bytes(), self.original)
        return text.split(PROTOTYPE.BANNER)[1:]

    def test_global_why_omits_inapplicable_and_equal_precedence_rules(self):
        frames = self.browser([b"/", b"research\n", b"\r", b"\t", b"p", b"\x1b", b"q"])
        explanation = next(frame for frame in frames if "SOURCE DETAIL - Why" in frame and "Selected project: none" in frame)
        self.assertIn("Effective state: available", explanation)
        self.assertIn("Winning layer: global", explanation)
        self.assertNotIn("Lower-precedence context:", explanation)

    def test_project_why_shadows_only_global_rules(self):
        frames = self.browser([b"/", b"research\n", b"\r", b"\t", b"\x1b", b"q"])
        explanation = next(frame for frame in frames if "SOURCE DETAIL - Why" in frame)
        self.assertIn("Effective state: explicit-only", explanation)
        self.assertIn("Winning layer: project override", explanation)
        self.assertIn("Lower-precedence context:", explanation)
        self.assertIn("global | global | available", explanation)
        self.assertNotIn("unrelated project override", explanation)
        self.assertNotIn("equal-precedence project rule", explanation)

    def test_other_project_cli_falls_back_without_shadowing_atlas(self):
        result = subprocess.run(self.command("explain", "shared.research", "--agent", "Codex", "--project", "/fixture/projects/no-override"), capture_output=True, text=True, stdin=subprocess.DEVNULL, env={**os.environ, "COLUMNS": "80"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Effective state: available", result.stdout)
        self.assertIn("Winning layer: global", result.stdout)
        self.assertNotIn("Lower-precedence context:", result.stdout)
        self.assertEqual(self.fixture.read_bytes(), self.original)

    def test_tab_navigates_sections_and_e_has_no_alias(self):
        frames = self.browser([b"/", b"research\n", b"\r", b"e", b"\t", b"\t", b"\t", b"\t", b"\x1b", b"q"])
        titles = [next(line for line in frame.splitlines() if line.startswith("SOURCE DETAIL -")) for frame in frames if "SOURCE DETAIL -" in frame]
        self.assertEqual(titles, ["SOURCE DETAIL - Read", "SOURCE DETAIL - Read", "SOURCE DETAIL - Why", "SOURCE DETAIL - Compare", "SOURCE DETAIL - Control", "SOURCE DETAIL - Read"])


if __name__ == "__main__":
    unittest.main()
