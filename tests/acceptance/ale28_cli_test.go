package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE28ProjectControlRefusesNativeIneffectiveScope(t *testing.T) {
	fx, root := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "project-control", "project-control")
	second := fx
	second.project = filepath.Join(fx.base, "allowed", "second-project")
	writeFile(t, filepath.Join(second.project, "keep.txt"), []byte("second project"), 0600)
	git := exec.Command("/usr/bin/git", "init", "-q", second.project)
	git.Env = fx.environment()
	if raw, err := git.CombinedOutput(); err != nil {
		t.Fatalf("second git init: %s: %v", raw, err)
	}
	config := filepath.Join(fx.home, ".codex/config.toml")
	projectConfig := filepath.Join(root, ".codex/config.toml")
	writeFile(t, config, []byte(selector(target, false)+trustProject(root)+trustProject(second.project)), 0600)
	writeFile(t, projectConfig, []byte("# preserve project\nmodel = 'fixture'\n"+selector(target, true)), 0600)
	selected := codexNamed(t, codexListResult(t, fx, 0), "project-control")
	if selected.Availability.Evidence.Kind != "inferred" || !strings.Contains(selected.Availability.Reason, "project configuration inference only") {
		t.Fatal("project state must not promise native enforcement", selected.Availability)
	}
	secondBefore := codexNamed(t, codexListResult(t, second, 0), "project-control")
	if secondBefore.Availability.State != "disabled" {
		t.Fatal("second project must inherit global disable")
	}
	id := changeSourceID(t, fx, target)
	before := treeManifest(t, fx.base)
	for _, choice := range []string{"enable", "disable", "inherit"} {
		for _, token := range []string{"", "c1:forged"} {
			args := []string{choice, "--agent", "codex", "--scope", "project", "--project", fx.project, "--json"}
			if token != "" {
				args = append(args, "--confirm", token)
			}
			args = append(args, id)
			result := changeCommand(t, fx, "", args...)
			assertExit(t, result, 1)
			var receipt changeReceipt
			decodeJSON(t, result.stdout, &receipt)
			if receipt.State != "unsupported" || !strings.Contains(receipt.Reason, "0.155.1") || !strings.Contains(receipt.Reason, "skills/list") || receipt.Confirmation != "" {
				t.Fatalf("native scope must not be presented as enforceable: %+v", receipt)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		}
	}
	secondAfter := codexNamed(t, codexListResult(t, second, 0), "project-control")
	if secondAfter.Availability.State != secondBefore.Availability.State {
		t.Fatal("other project changed")
	}
	result := changeCommand(t, fx, "", "inherit", "--agent", "codex", "--scope", "global", "--json", id)
	assertExit(t, result, 1)
	if !strings.Contains(string(result.stdout), "global scope has no project override") {
		t.Fatal("global inherit refusal unclear", string(result.stdout))
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
