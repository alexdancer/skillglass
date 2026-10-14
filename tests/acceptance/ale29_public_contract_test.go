package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29ClaudeCompareAndProblemFilterUseNativeScopeReadOnly(t *testing.T) {
	fx, _ := codexFixture(t)
	global := filepath.Dir(claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "copy", "collision", ""))
	project := filepath.Dir(claudeSkill(t, filepath.Join(fx.project, ".claude", "skills"), "copy", "collision", ""))
	before := treeManifest(t, fx.base)
	list := claudeListResult(t, fx, 0)
	left, right := claudeAt(t, list, global), claudeAt(t, list, project)
	if left.ID == right.ID || left.Availability.State != "available" || right.Availability.State != "shadowed" {
		t.Fatalf("native independent copies: %+v %+v", left, right)
	}
	result := fx.run(t, "compare", "--agent", "claude-code", "--project", fx.project, "--json", left.ID, right.ID)
	assertExit(t, result, 0)
	report := compareReport(t, result)
	if report.Report.Relation != "independent-copies" || report.Report.Outcome != "identical" || report.Report.Coverage != "complete" {
		t.Fatalf("Claude comparison: %s", result.stdout)
	}
	writeEvidence(t, "ale29-public-contract/independent-compare.json", result.stdout)
	problems := fx.run(t, "problems", "--agent", "claude-code", "--project", fx.project, "--problem", "duplicate-name", "--json")
	assertExit(t, problems, 0)
	var duplicates publicList
	decodeJSON(t, problems.stdout, &duplicates)
	if len(duplicates.Problems) != 2 {
		t.Fatalf("duplicate filter: %s", problems.stdout)
	}
	for _, problem := range duplicates.Problems {
		if problem.Code != "duplicate-name" {
			t.Fatalf("unrelated problem: %+v", problem)
		}
	}
	writeEvidence(t, "ale29-public-contract/problems.json", problems.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, filepath.Join(project, "notes.txt"), []byte("PROJECT_ONLY_RESOURCE\n"), 0600)
	before = treeManifest(t, fx.base)
	result = fx.run(t, "compare", "--agent", "claude-code", "--project", fx.project, left.ID, right.ID)
	assertExit(t, result, 0)
	for _, needle := range []string{"different", "notes.txt", "PROJECT_ONLY_RESOURCE"} {
		if !strings.Contains(string(result.stdout), needle) {
			t.Fatalf("comparison lacks %q: %s", needle, result.stdout)
		}
	}
	writeEvidence(t, "ale29-public-contract/different-compare.txt", result.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE29ClaudeNoSelectedProjectKeepsPersonalScope(t *testing.T) {
	fx, _ := codexFixture(t)
	global := filepath.Dir(claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "global", "personal-action", ""))
	claudeSkill(t, filepath.Join(fx.project, ".claude", "skills"), "project", "project-canary", "")
	before := treeManifest(t, fx.base)
	result := fx.run(t, "list", "--agent", "claude-code", "--json")
	assertExit(t, result, 0)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	if len(list.Sources) != 1 || claudeAt(t, list, global).Availability.State != "available" {
		t.Fatalf("personal scope inherited an unselected project: %s", result.stdout)
	}
	var scope publicList
	decodeJSON(t, result.stdout, &scope)
	if scope.Scope.Project.Selected {
		t.Fatal("project was implicitly selected")
	}
	writeEvidence(t, "ale29-public-contract/personal-only.json", result.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
