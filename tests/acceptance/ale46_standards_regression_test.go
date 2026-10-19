package acceptance_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

// Tests the human public CLI boundary at the point that requests approval.
func TestALE46HumanSelectionRetainsExactSourceIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		scope   collection.ControlScope
		escaped bool
	}{
		{"global", collection.ControlGlobal, false},
		{"project", collection.ControlProject, false},
		{"project-escaped-paths", collection.ControlProject, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			expectedSourcePath := filepath.Dir(target)
			if scenario.escaped {
				fx.project = filepath.Join(fx.base, "allowed", "project-\x1b[2J\n")
				writeFile(t, filepath.Join(fx.project, ".git", "marker"), []byte("fixture"), 0600)
				expectedSourcePath = filepath.Join(fx.base, "allowed", "source-\x1b[2J\n", "candidate")
				writeSkill(t, expectedSourcePath, "candidate", "Source identity evidence.", "Retained source instructions.")
				if err := os.Symlink(expectedSourcePath, filepath.Join(fx.home, ".claude/skills/candidate")); err != nil {
					t.Fatal(err)
				}
				inventory := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
				assertExit(t, inventory, 0)
				var view collection.View
				decodeJSON(t, inventory.stdout, &view)
				id = ""
				for _, source := range view.Sources {
					if source.Directory == expectedSourcePath {
						id = string(source.ID)
					}
				}
				if id == "" {
					t.Fatalf("fixture exact source not discovered: %s", inventory.stdout)
				}
			}
			project := ""
			if scenario.scope == collection.ControlProject {
				project = fx.project
			}
			draft := ale46Draft([]string{"claude-code"}, scenario.scope, project)
			draft.Sources = []collection.ReviewAssessment{ale46Assess(id)}
			draft.Changes = []collection.ReviewRecommendation{ale46Change("choice-one", id, "claude-code", collection.ControlDisable)}
			draftPath := ale46File(t, fx, "identity-draft.json", draft)
			proposal := ale46Prepare(t, fx, draftPath, 0)
			if proposal.Changes[0].Baseline.SourcePath != expectedSourcePath {
				t.Fatal("fixture/proposal source path differs from literal expectation")
			}
			proposalPath := ale46File(t, fx, "identity-proposal.json", proposal)
			before := treeManifest(t, fx.base)
			human := fx.run(t, "review", "select", "--proposal", proposalPath, "--choose", "choice-one")
			assertExit(t, human, 0)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeEvidence(t, "repair/identity-"+scenario.name+"-human-selection.txt", human.stdout)
			escapedSource := strings.NewReplacer("\x1b", `\x1b`, "\n", `\n`).Replace(expectedSourcePath)
			escapedProject := "(none)"
			if project != "" {
				escapedProject = strings.NewReplacer("\x1b", `\x1b`, "\n", `\n`).Replace(project)
			}
			expected := []string{
				"Source ID: " + id + "\n",
				"Source: " + escapedSource + "\n",
				fmt.Sprintf("Selected choice-one: claude-code disable | scope: %s\n", scenario.scope),
				fmt.Sprintf("Scope: %s | project: %s\n", scenario.scope, escapedProject),
			}
			approvalAt := bytes.Index(human.stdout, []byte("After explicit user approval"))
			for _, literal := range expected {
				at := bytes.Index(human.stdout, []byte(literal))
				if at < 0 || approvalAt < 0 || at >= approvalAt {
					t.Fatalf("human approval preview omits expected identity/context before approval %q:\n%s", literal, human.stdout)
				}
			}
			if bytes.Contains(human.stdout, []byte{27}) {
				t.Fatalf("untrusted path emitted a raw terminal escape:\n%q", human.stdout)
			}
			writeEvidence(t, "repair/identity-"+scenario.name+"-tree-before.json", before)
			writeEvidence(t, "repair/identity-"+scenario.name+"-tree-after.json", treeManifest(t, fx.base))
		})
	}
}
