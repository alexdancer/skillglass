package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// The public refusal preview must retain the exact selected project.
func TestDecisionALE28UnsupportedPreviewRetainsProject(t *testing.T) {
	fx, root := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "project-context", "project-context")
	writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte(trustProject(root)), 0600)
	id := changeSourceID(t, fx, target)
	before := treeManifest(t, fx.base)
	for _, choice := range []string{"enable", "disable", "inherit"} {
		t.Run(choice, func(t *testing.T) {
			result := changeCommand(t, fx, "", choice, "--agent", "codex", "--scope", "project", "--project", fx.project, id)
			assertExit(t, result, 1)
			writeEvidence(t, "decision-project-context/"+choice+".txt", result.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			want := "Inspection project: " + fx.project
			if !strings.Contains(string(result.stdout), want) {
				t.Fatalf("unsupported preview must identify selected project; missing literal %q in:\n%s", want, result.stdout)
			}
			result = changeCommand(t, fx, "", choice, "--agent", "codex", "--scope", "project", "--project", fx.project, "--json", id)
			assertExit(t, result, 1)
			var receipt struct {
				InspectionProject struct {
					Selected bool
					Path     string
				} `json:"inspection_project"`
			}
			decodeJSON(t, result.stdout, &receipt)
			if !receipt.InspectionProject.Selected || receipt.InspectionProject.Path != fx.project {
				t.Fatalf("JSON refusal lost selected project: %+v", receipt)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestDecisionALE28UnsupportedPreviewAbsentAndEscapedProject(t *testing.T) {
	for _, selected := range []bool{false, true} {
		name := "absent"
		if selected {
			name = "escaped"
		}
		t.Run(name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "context", "context")
			id := changeSourceID(t, fx, target)
			args := []string{"inherit", "--agent", "codex", "--scope", "project"}
			want := "Inspection project: none selected"
			if selected {
				project := filepath.Join(fx.project, "project\x1b[31m\ncontext")
				writeFile(t, filepath.Join(project, "keep.txt"), []byte("inert"), 0600)
				args = append(args, "--project", project)
				want = "Inspection project: " + filepath.Join(fx.project, `project\x1b[31m\ncontext`)
			}
			args = append(args, id)
			before := treeManifest(t, fx.base)
			result := changeCommand(t, fx, "", args...)
			assertExit(t, result, 1)
			if strings.ContainsRune(string(result.stdout), '\x1b') || !strings.Contains(string(result.stdout), want) {
				t.Fatalf("missing safe explicit project context %q in %s", want, result.stdout)
			}
			writeEvidence(t, "decision-project-context/"+name+".txt", result.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
