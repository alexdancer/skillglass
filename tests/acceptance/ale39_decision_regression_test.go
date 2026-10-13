package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE39DecisionPluginManifestFallback(t *testing.T) {
	for _, mode := range []string{"absent", "intact", "broken"} {
		t.Run(mode, func(t *testing.T) {
			fx := codeafFixture(t)
			manifest, _ := codeafPluginFixture(t, fx)
			container := filepath.Dir(filepath.Dir(manifest))
			rootManifest := filepath.Join(container, "SKILL.md")
			writeFile(t, filepath.Join(fx.base, "plugin/.claude-plugin/plugin.json"), []byte(`{"skills":"skills"}`), 0600)
			switch mode {
			case "intact":
				writeFile(t, rootManifest, []byte("---\nname: root-skill\ndescription: Container is itself a skill.\n---\nInert.\n"), 0600)
			case "broken":
				if err := os.Symlink("missing-target.md", rootManifest); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			exit := 0
			if mode == "broken" {
				exit = 1
			}
			list := codeafListResult(t, fx, exit)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			child, root := false, false
			for _, source := range list.Sources {
				if source.Name == "plugin-skill" {
					child = source.Availability.State == "available"
					if source.Ownership.State != "managed" || len(source.Availability.References) != 1 || source.Availability.References[0].Provenance.PluginID != "example@local" || source.Availability.References[0].Provenance.Version != "1.2" || source.Availability.References[0].Participation != "eligible" {
						t.Errorf("child provenance/participation: %+v", source)
					}
					detail := fx.run(t, "explain", "--agent", "codeaf", "--project", fx.project, source.ID)
					assertExit(t, detail, exit)
					for _, needle := range []string{"Availability: available", "example@local", "Session: unknown", "persistent CodeAF-only per-skill control is unsupported"} {
						if !strings.Contains(string(detail.stdout), needle) {
							t.Errorf("explanation lacks %q: %s", needle, detail.stdout)
						}
					}
				}
				if source.Name == "root-skill" {
					root = source.Availability.State == "available"
				}
			}
			if mode == "broken" && !child {
				// Public explicit inspection exposes the stable source ID for the missing native child.
				inspected := codeafListResult(t, fx, 1, "--root", filepath.Dir(manifest))
				target := codeafPath(t, inspected, filepath.Dir(manifest))
				detail := fx.run(t, "explain", "--agent", "codeaf", "--project", fx.project, target.ID)
				if detail.code != 1 || !strings.Contains(string(detail.stdout), "Availability: available") {
					t.Errorf("native child explanation: exit=%d want 1 with available child; stdout=%s stderr=%s", detail.code, detail.stdout, detail.stderr)
				}
			}
			if mode == "broken" {
				found := false
				for _, problem := range list.Problems {
					found = found || problem.Code == "broken-reference"
				}
				if !found || list.Scope.Completeness != "incomplete" {
					t.Fatal("broken container diagnostic lost")
				}
			}
			wantChild := mode != "intact"
			if child != wantChild || root != !wantChild {
				t.Errorf("native selection: child available=%v root available=%v; want child=%v root=%v; sources=%+v", child, root, wantChild, !wantChild, list.Sources)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE39DecisionGenericExplicitRootControls(t *testing.T) {
	for _, mode := range []string{"absent", "intact", "broken"} {
		t.Run(mode, func(t *testing.T) {
			fx := codeafFixture(t)
			container := filepath.Join(fx.base, "custom")
			codexSkill(t, container, "child", "custom-child")
			manifest := filepath.Join(container, "SKILL.md")
			switch mode {
			case "intact":
				writeFile(t, manifest, []byte("---\nname: custom-root\ndescription: Generic bundle.\n---\nInert.\n"), 0600)
			case "broken":
				if err := os.Symlink("missing-target.md", manifest); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--root", container, "--json")
			exit := 0
			if mode == "broken" {
				exit = 1
			}
			assertExit(t, result, exit)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			child, root := false, false
			for _, s := range list.Sources {
				child = child || s.Name == "custom-child"
				root = root || s.Name == "custom-root"
			}
			if child != (mode == "absent") || root != (mode == "intact") {
				t.Fatalf("generic semantics changed: child=%v root=%v mode=%s sources=%+v", child, root, mode, list.Sources)
			}
			if mode == "broken" && (len(list.Problems) == 0 || list.Scope.Completeness != "incomplete") {
				t.Fatal("broken explicit root lost diagnostic")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE39PluginStatSuccessControls(t *testing.T) {
	for _, mode := range []string{"linked", "malformed", "directory"} {
		t.Run(mode, func(t *testing.T) {
			fx := codeafFixture(t)
			manifest, _ := codeafPluginFixture(t, fx)
			container := filepath.Dir(filepath.Dir(manifest))
			rootManifest := filepath.Join(container, "SKILL.md")
			writeFile(t, filepath.Join(fx.base, "plugin/.claude-plugin/plugin.json"), []byte(`{"skills":"skills"}`), 0600)
			switch mode {
			case "linked":
				target := filepath.Join(fx.base, "root-target.md")
				writeFile(t, target, []byte("---\nname: root-skill\ndescription: Linked root.\n---\nInert.\n"), 0600)
				if err := os.Symlink(target, rootManifest); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				writeFile(t, rootManifest, []byte("---\nname: [unterminated\n---\nInert.\n"), 0600)
			case "directory":
				if err := os.Mkdir(rootManifest, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			exit := 1
			if mode == "linked" {
				exit = 0
			}
			list := codeafListResult(t, fx, exit)
			root := codeafPath(t, list, container)
			for _, source := range list.Sources {
				if source.Name == "plugin-skill" {
					t.Fatalf("Stat-success root exposed child: %+v", source)
				}
			}
			if mode == "linked" {
				if root.Name != "root-skill" || root.Availability.State != "available" {
					t.Fatalf("linked root: %+v", root)
				}
			} else if root.Availability.State != "unavailable" || list.Scope.Completeness != "incomplete" || len(list.Problems) == 0 {
				t.Fatalf("damaged Stat-success root: %+v", root)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
