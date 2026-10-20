package acceptance_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/setup"
)

// Port of the independent review regression. The ordinary leaf grammar is now
// native-validated by ALE-50; unsupported leaf grammar retains scoped uncertainty.
func TestALE51LeafIgnoreScope(t *testing.T) {
	for _, name := range []string{"without-ignore", "known-leaf", ".gitignore", ".ignore", ".fdignore", "known-root-unknown-leaf"} {
		t.Run(name, func(t *testing.T) {
			fx := ale48Fixture(t)
			root := filepath.Join(fx.home, ".claude/skills")
			raw, err := json.Marshal(map[string]any{"skills": []string{root}})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), raw, 0600)
			writeFile(t, filepath.Join(root, "unrelated/SKILL.md"), []byte("---\nname: unrelated\ndescription: fixture\n---\nfixture\n"), 0600)
			unknown := name != "without-ignore" && name != "known-leaf"
			if name != "without-ignore" {
				input, rule := name, "ignored?/\n"
				if name == "known-leaf" {
					input, rule = ".gitignore", "ignored/\n"
				}
				if name == "known-root-unknown-leaf" {
					input = ".gitignore"
					writeFile(t, filepath.Join(root, ".gitignore"), []byte("unrelated/ignored/\n"), 0600)
				}
				writeFile(t, filepath.Join(root, "unrelated", input), []byte(rule), 0600)
			}
			args := []string{"setup", "--app", "claude-code", "--allow-shared-app", "pi", "--allow-shared-app", "codeaf", "--allow-shared-app", "opencode"}
			before := treeManifest(t, fx.base)
			run := func(want int, extra ...string) setup.SetupReport {
				t.Helper()
				result := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), append(append([]string{}, args...), extra...)...)
				assertExit(t, result, want)
				var report setup.SetupReport
				decodeJSON(t, result.stdout, &report)
				if len(report.Results) != 1 {
					t.Fatal(report)
				}
				warnings, err := json.Marshal(report.Results[0].Audit.Warnings)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(warnings), "Pi ignore-file") != unknown {
					t.Fatalf("Pi warning mismatch: %s", warnings)
				}
				return report
			}
			preview := run(0, "--json")
			if preview.Results[0].State != "ready" {
				t.Fatal(preview)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			human := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), args...)
			assertExit(t, human, 0)
			if unknown && (!strings.Contains(string(human.stdout), "Warning:") || !strings.Contains(string(human.stdout), "Pi ignore-file")) {
				t.Fatalf("missing human Pi warning: %s", human.stdout)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			denied := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--app", "claude-code", "--install", "--json")
			assertExit(t, denied, 1)
			if !strings.Contains(string(denied.stdout), "unacknowledged shared consumers") {
				t.Fatal(string(denied.stdout))
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			installed := run(0, "--install", "--json")
			if installed.Results[0].State != "installed" {
				t.Fatal(installed)
			}
			ale48AssertBundle(t, installed.Results[0].Destination)
			ale48Preserved(t, fx, before, installed.Results)
			stable := treeManifest(t, fx.base)
			again := run(0, "--install", "--json")
			if again.Results[0].State != "already-present" {
				t.Fatal(again)
			}
			assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
		})
	}
}

func TestALE51ProspectiveIgnoreAncestorsRefuseWithoutCurrentReference(t *testing.T) {
	for _, name := range []string{"root", "ancestor", "destination", "root-with-leaf"} {
		t.Run(name, func(t *testing.T) {
			fx := ale48Fixture(t)
			root := filepath.Join(fx.home, ".claude/skills")
			inputDir := root
			if name == "ancestor" {
				root = filepath.Join(fx.home, ".claude")
			}
			if name == "destination" {
				inputDir = filepath.Join(root, "skillglass-review")
			}
			raw, err := json.Marshal(map[string]any{"skills": []string{root}})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), raw, 0600)
			writeFile(t, filepath.Join(inputDir, ".gitignore"), []byte("ignored?/\n"), 0600)
			if name == "root-with-leaf" {
				writeFile(t, filepath.Join(root, "unrelated/SKILL.md"), []byte("---\nname: unrelated\ndescription: fixture\n---\nfixture\n"), 0600)
			}
			before := treeManifest(t, fx.base)
			for _, install := range []bool{false, true} {
				args := []string{"setup", "--app", "claude-code", "--allow-shared-app", "pi", "--allow-shared-app", "codeaf", "--allow-shared-app", "opencode", "--json"}
				if install {
					args = append(args, "--install")
				}
				result := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), args...)
				assertExit(t, result, 1)
				reason := "prospective companion ignore inputs are unverified"
				if name == "destination" {
					reason = "existing companion differs"
				}
				if !strings.Contains(string(result.stdout), reason) {
					t.Fatal(string(result.stdout))
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
		})
	}
}
