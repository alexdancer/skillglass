package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func gateR3Targets(t *testing.T, fx fixture) []codexSource {
	t.Helper()
	out := []codexSource{}
	for _, s := range opencodeListResult(t, fx, 0).Sources {
		if s.Name == "target" {
			out = append(out, s)
		}
	}
	for _, s := range out {
		t.Logf("context=%s target=%s path=%s availability=%s permission=%+v", fx.project, s.ID, s.SourcePath, s.Availability.State, s.Availability.References)
	}
	return out
}
func gateR3Control(t *testing.T, fx fixture, id, token string) (commandResult, changeReceipt) {
	t.Helper()
	args := []string{"disable", "--agent", "opencode", "--scope", "global", "--project", fx.project, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	r := changeCommand(t, fx, "", append(args, id)...)
	var out changeReceipt
	decodeJSON(t, r.stdout, &out)
	t.Logf("global disable confirmation=%t exit=%d state=%s persistence=%s achievement=%s reason=%s", token != "", r.code, out.State, out.Persistence, out.Achievement, out.Reason)
	return r, out
}
func TestALE36GateR3HiddenGlobalIndependentCopyMustRefuse(t *testing.T) {
	for _, late := range []bool{false, true} {
		name := "existing-hidden-copy"
		if late {
			name = "hidden-copy-added-after-preview"
		}
		t.Run(name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			target := codexSkill(t, filepath.Join(fx.xdgConfig, "opencode/skills"), "regular", "target")
			extra := filepath.Join(fx.base, "allowed", "global-extra")
			if err := os.MkdirAll(extra, 0700); err != nil {
				t.Fatal(err)
			}
			global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
			raw, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{extra}}, "theme": "global-unchanged"})
			writeFile(t, global, raw, 0600)
			project := filepath.Join(fx.project, "opencode.json")
			projectRaw := []byte(`{"skills":{"paths":[]},"theme":"project-unchanged"}`)
			writeFile(t, project, projectRaw, 0600)
			hidden := ""
			if !late {
				hidden = codexSkill(t, extra, "independent", "target")
			}
			other := fx
			other.project = filepath.Join(fx.base, "allowed", "other-project")
			if err := os.MkdirAll(other.project, 0700); err != nil {
				t.Fatal(err)
			}
			selected := gateR3Targets(t, fx)
			if len(selected) != 1 {
				t.Fatalf("selected project must hide configured extra root: got %d target copies, want 1", len(selected))
			}
			id := selected[0].ID
			outsideBefore := gateR3Targets(t, other)
			wantBefore := 2
			if late {
				wantBefore = 1
			}
			if len(outsideBefore) != wantBefore {
				t.Fatalf("other project before: got %d target copies, want %d", len(outsideBefore), wantBefore)
			}
			before := treeManifest(t, fx.base)
			r, p := gateR3Control(t, fx, id, "")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if !late && p.State != "ready" {
				assertExit(t, r, 1)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			if late && p.State != "ready" {
				t.Fatalf("unique target must permit initial preview: %s", p.Reason)
			}
			if late {
				hidden = codexSkill(t, extra, "independent", "target")
				outsideBefore = gateR3Targets(t, other)
				if len(outsideBefore) != 2 {
					t.Fatal("late global independent copy must be discovered outside masking project")
				}
			}
			targetBytes, hiddenBytes := readBytes(t, target), readBytes(t, hidden)
			a, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.Stat(hidden)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(a, b) {
				t.Fatal("copies must be independent files")
			}
			if len(gateR3Targets(t, fx)) != 1 {
				t.Fatal("project inventory unexpectedly exposed hidden copy")
			}
			applied, out := gateR3Control(t, fx, id, p.Confirmation)
			selectedAfter := gateR3Targets(t, fx)
			outsideAfter := gateR3Targets(t, other)
			t.Logf("independent identities=true; selected target copies=%d outside target copies=%d; global-before=%s global-after=%s project-after=%s", len(selectedAfter), len(outsideAfter), raw, readBytes(t, global), readBytes(t, project))
			if !bytes.Equal(targetBytes, readBytes(t, target)) || !bytes.Equal(hiddenBytes, readBytes(t, hidden)) || !bytes.Equal(projectRaw, readBytes(t, project)) {
				t.Fatal("source content or project settings changed")
			}
			if applied.code == 0 {
				t.Fatalf("global name action must refuse hidden independent copy%s; got exit=0 state=%s achievement=%s", map[bool]string{true: " added after preview", false: ""}[late], out.State, out.Achievement)
			}
			if !bytes.Equal(raw, readBytes(t, global)) {
				t.Fatal("refused/stale confirmation must leave global config unchanged")
			}
		})
	}
}
