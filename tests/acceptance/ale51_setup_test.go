package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE51SetupProvenDestinationRetainsUnrelatedWarnings(t *testing.T) {
	for _, kind := range []string{"broken-alias", "pi-ignore", "inactive-cache"} {
		t.Run(kind, func(t *testing.T) {
			fx := ale48Fixture(t)
			switch kind {
			case "broken-alias":
				root := filepath.Join(fx.home, ".claude/skills")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(fx.home, "absent"), filepath.Join(root, "broken")); err != nil {
					t.Fatal(err)
				}
			case "pi-ignore":
				writeFile(t, filepath.Join(fx.home, ".pi/agent/skills/.gitignore"), []byte("ignored?/\n"), 0600)
				writeFile(t, filepath.Join(fx.home, ".pi/agent/skills/example/SKILL.md"), []byte("---\nname: example\ndescription: fixture\n---\nfixture\n"), 0600)
			case "inactive-cache":
				root := filepath.Join(fx.home, ".pi/agent/npm/node_modules")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 10005; i++ {
					writeFile(t, filepath.Join(root, fmt.Sprint(i)), []byte("fixture"), 0600)
				}
			}
			before := treeManifest(t, fx.base)
			preview := ale48Setup(t, fx, false, 0, "codex", "opencode", "codeaf")
			raw, err := json.Marshal(preview)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "pi-ignore" && !strings.Contains(string(raw), "Pi ignore-file") {
				t.Fatalf("Pi ignore uncertainty warning missing: %s", raw)
			}
			if !strings.Contains(string(raw), `"warnings":["`) {
				t.Fatalf("unrelated gaps not visible: %s", raw)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			human := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--app", "codex", "--allow-shared-app", "codeaf")
			assertExit(t, human, 0)
			if !strings.Contains(string(human.stdout), "Warning:") {
				t.Fatalf("human warning missing: %s", human.stdout)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			denied := ale48Setup(t, fx, true, 1, "codex")
			if !strings.Contains(denied.Results[0].Reason, "unacknowledged shared consumers") {
				t.Fatal(denied)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			installed := ale48Setup(t, fx, true, 0, "codex", "opencode", "codeaf")
			for _, r := range installed.Results {
				if r.State != "installed" {
					t.Fatal(r)
				}
				ale48AssertBundle(t, r.Destination)
			}
			ale48Preserved(t, fx, before, installed.Results)
		})
	}
}

func TestALE51UnknownRoutesAndBrokenAliasChainsStillRefuse(t *testing.T) {
	for _, kind := range []string{"plugin", "secrets", "broken-related-chain", "parent-traversal-alias", "related-ignore", "inactive-managed-alias", "cycle"} {
		t.Run(kind, func(t *testing.T) {
			fx := ale48Fixture(t)
			switch kind {
			case "plugin":
				writeFile(t, filepath.Join(fx.home, ".config/opencode/opencode.json"), []byte(`{"plugin":["fixture-never-run"]}`), 0600)
			case "secrets":
				writeFile(t, filepath.Join(fx.home, ".hermes/config.yaml"), []byte("secrets:\n  provider: fixture\n"), 0600)
			case "related-ignore":
				root := filepath.Join(fx.home, ".codex/skills")
				raw, err := json.Marshal(map[string]any{"skills": []string{root}})
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), raw, 0600)
				writeFile(t, filepath.Join(root, ".gitignore"), []byte("ignored?/\n"), 0600)
			case "inactive-managed-alias":
				root := filepath.Join(fx.home, ".pi/agent/npm")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(fx.home, ".codex/skills"), filepath.Join(root, "node_modules")); err != nil {
					t.Fatal(err)
				}
			default:
				root := filepath.Join(fx.home, ".claude/skills")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(fx.home, ".codex/skills/skillglass-review")
				if kind == "parent-traversal-alias" {
					container := filepath.Join(fx.home, ".codex/container")
					if err := os.MkdirAll(container, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(container, filepath.Join(fx.home, "outer")); err != nil {
						t.Fatal(err)
					}
					target = fx.home + "/outer/../skills"
					if err := os.Symlink(target, filepath.Join(root, "broken")); err != nil {
						t.Fatal(err)
					}
					break
				}
				if kind == "cycle" {
					target = filepath.Join(fx.home, "hop")
				}
				if err := os.Symlink(target, filepath.Join(fx.home, "hop")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(fx.home, "hop"), filepath.Join(root, "broken")); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			report := ale48Setup(t, fx, true, 1, "codex", "codeaf")
			for _, r := range report.Results {
				if r.State != "refused" {
					t.Fatal(r)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE51SetupReinspectsRoutesAndConflictsAfterPreview(t *testing.T) {
	for _, kind := range []string{"new-alias", "new-plugin", "new-consumer", "new-conflict"} {
		t.Run(kind, func(t *testing.T) {
			fx := ale48Fixture(t)
			ale48Setup(t, fx, false, 0, "codex", "codeaf")
			switch kind {
			case "new-alias":
				root := filepath.Join(fx.home, ".claude/skills")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(fx.home, ".codex/skills"), filepath.Join(root, "future")); err != nil {
					t.Fatal(err)
				}
			case "new-plugin":
				writeFile(t, filepath.Join(fx.home, ".config/opencode/opencode.json"), []byte(`{"plugin":["fixture-never-run"]}`), 0600)
			case "new-consumer":
				raw, err := json.Marshal(map[string]any{"skills": []string{filepath.Join(fx.home, ".codex/skills")}})
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), raw, 0600)
			case "new-conflict":
				writeFile(t, filepath.Join(fx.home, ".codex/skills/skillglass-review/SKILL.md"), []byte("concurrent customized copy"), 0600)
			}
			before := treeManifest(t, fx.base)
			report := ale48Setup(t, fx, true, 1, "codex", "codeaf")
			if report.Results[0].State != "refused" {
				t.Fatal(report)
			}
			if kind == "new-consumer" && !strings.Contains(report.Results[0].Reason, "pi") {
				t.Fatal(report)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
