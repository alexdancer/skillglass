package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestALE28ForgedConfirmationRefusalRetainsProject(t *testing.T) {
	fx, root := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "confirmation-context", "confirmation-context")
	writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte(trustProject(root)), 0600)
	id := changeSourceID(t, fx, target)
	before := treeManifest(t, fx.base)
	for _, choice := range []string{"enable", "disable", "inherit"} {
		for _, format := range []string{"human", "json"} {
			t.Run(choice+"/"+format, func(t *testing.T) {
				args := []string{choice, "--agent", "codex", "--scope", "project", "--project", fx.project, "--confirm", "c1:forged"}
				if format == "json" {
					args = append(args, "--json")
				}
				result := changeCommand(t, fx, "", append(args, id)...)
				assertExit(t, result, 1)
				writeEvidence(t, "confirmation-project-context/"+choice+"-"+format+".txt", result.stdout)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if format == "human" {
					if !strings.Contains(string(result.stdout), "Inspection project: "+fx.project) {
						t.Errorf("unsupported confirmation refusal lost selected project context:\n%s", result.stdout)
					}
					return
				}
				var receipt struct {
					InspectionProject struct {
						Selected bool   `json:"selected"`
						Path     string `json:"path"`
					} `json:"inspection_project"`
				}
				decodeJSON(t, result.stdout, &receipt)
				if !receipt.InspectionProject.Selected || receipt.InspectionProject.Path != fx.project {
					t.Errorf("unsupported JSON confirmation refusal lost selected project %q: %+v", fx.project, receipt)
				}
			})
		}
	}
}
