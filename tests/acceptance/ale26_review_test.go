package acceptance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestALE26ReviewProblemAvailabilityFilters(t *testing.T) {
	fx, root := codexFixture(t)
	global := filepath.Join(fx.home, ".agents", "skills")
	project := filepath.Join(root, ".agents", "skills")
	denied := codexSkill(t, global, "denied", "collision")
	codexSkill(t, project, "allowed", "collision")
	writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte(selector(denied, false)), 0600)
	before := treeManifest(t, fx.base)
	inventory := codexListResult(t, fx, 0)
	for _, state := range []string{"disabled", "available", "explicit-only"} {
		t.Run(state, func(t *testing.T) {
			result := fx.run(t, "problems", "--agent", "codex", "--project", fx.project, "--availability", state, "--problem", "duplicate-name", "--json")
			assertExit(t, result, 0)
			var view publicList
			decodeJSON(t, result.stdout, &view)
			want := 1
			if state == "explicit-only" {
				want = 0
			}
			if len(view.Problems) != want {
				t.Fatalf("%s problems=%+v, want %d", state, view.Problems, want)
			}
			for _, problem := range view.Problems {
				if len(problem.SourceIDs) != 1 {
					t.Fatalf("source relationships: %+v", problem)
				}
				for _, source := range inventory.Sources {
					if source.ID == problem.SourceIDs[0] && source.Availability.State != state {
						t.Fatalf("%s includes %s diagnostic", state, source.Availability.State)
					}
				}
			}
			if view.Scope.Completeness != "complete" {
				t.Fatal("display filter changed coverage")
			}
		})
	}
	result := fx.run(t, "problems", "--agent", "codex", "--project", fx.project, "--availability", "disabled", "--context", "project", "--json")
	assertExit(t, result, 0)
	var view publicList
	decodeJSON(t, result.stdout, &view)
	if len(view.Problems) != 0 {
		t.Fatalf("combined filters include unrelated source: %+v", view.Problems)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE26ReviewUnresolvedManifestPermission(t *testing.T) {
	for _, kind := range []string{"fifo", "broken-link"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			global := filepath.Join(fx.home, ".agents", "skills")
			manifest := codexSkill(t, global, "unresolved", "unresolved")
			if err := os.Remove(manifest); err != nil {
				t.Fatal(err)
			}
			if kind == "fifo" {
				if err := syscall.Mkfifo(manifest, 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(filepath.Join(fx.base, "missing"), manifest); err != nil {
				t.Fatal(err)
			}
			folder := filepath.Join(global, "unrelated-folder")
			if err := os.MkdirAll(folder, 0755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte(selector(folder, false)+selector(filepath.Join(fx.base, "missing", "SKILL.md"), false)), 0600)
			before := treeManifest(t, fx.base)
			list := codexListResult(t, fx, 1)
			found := false
			for _, source := range list.Sources {
				if filepath.Base(source.SourcePath) != "unresolved" {
					continue
				}
				found = true
				for _, fact := range source.Availability.References {
					if fact.Permission.State != "unknown" || fact.Permission.Evidence.Kind != "unknown" || fact.Permission.Evidence.Rule == "manifest-selector" {
						t.Fatalf("unresolved manifest fabricated permission: %+v", fact.Permission)
					}
				}
			}
			if !found {
				t.Fatal("unresolved source absent")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE26ReviewNamespaceProblemRelationships(t *testing.T) {
	for _, kind := range []string{"fifo", "invalid-json"} {
		t.Run(kind, func(t *testing.T) {
			fx, root := codexFixture(t)
			plugin := filepath.Join(root, "plugins", "sample")
			metadata := filepath.Join(plugin, ".codex-plugin", "plugin.json")
			writeFile(t, metadata, []byte(`{"name":"sample","version":"1.0.0","skills":"./skills/"}`), 0600)
			global := filepath.Join(fx.home, ".agents", "skills")
			for _, name := range []string{"first", "second"} {
				manifest := codexSkill(t, filepath.Join(plugin, "skills"), name, name)
				if err := os.MkdirAll(global, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(manifest), filepath.Join(global, name)); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(filepath.Join(plugin, "skills", "first"), filepath.Join(fx.cleanRoot, "shared-alias")); err != nil {
				t.Fatal(err)
			}
			codexSkill(t, global, "healthy", "healthy")
			if kind == "fifo" {
				if err := os.Remove(metadata); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(metadata, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, metadata, []byte("{"), 0600)
			}
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "codex", "--project", fx.project, "--root", fx.cleanRoot, "--problem", "native-input", "--json")
			assertExit(t, result, 1)
			var list publicList
			decodeJSON(t, result.stdout, &list)
			if len(list.Sources) != 2 || len(list.Problems) != 1 {
				t.Fatalf("namespace diagnostics reached unaffected sources: %+v", list)
			}
			problem := list.Problems[0]
			if len(problem.SourceIDs) != 2 || len(problem.ReferenceIDs) != 3 || len(problem.RootIDs) != 2 {
				t.Fatalf("shared namespace relationships: %+v", problem)
			}
			for _, source := range list.Sources {
				if source.Name != "first" && source.Name != "second" {
					t.Fatalf("unaffected source included: %+v", source)
				}
				if len(source.Problems) != 1 || !reflect.DeepEqual(source.Problems[0], problem) {
					t.Fatalf("source diagnostic not aggregated: %+v", source.Problems)
				}
			}
			for _, reference := range list.References {
				if len(reference.Problems) != 1 || !reflect.DeepEqual(reference.Problems[0], problem) {
					t.Fatalf("reference diagnostic not aggregated: %+v", reference)
				}
			}
			for _, report := range list.Scope.Roots {
				if report.ID == problem.RootIDs[0] || report.ID == problem.RootIDs[1] {
					if report.ProblemCount != 1 || report.State != "incomplete" {
						t.Fatalf("affected root stale: %+v", report)
					}
				} else if report.ProblemCount != 0 || report.State == "incomplete" {
					t.Fatalf("unaffected root changed: %+v", report)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE26ReviewNamespaceAncestryLimit(t *testing.T) {
	fx, root := codexFixture(t)
	deep := filepath.Join(root, strings.Repeat("d/", 130))
	manifest := codexSkill(t, deep, "skill", "deep-skill")
	global := filepath.Join(fx.home, ".agents", "skills")
	if err := os.MkdirAll(global, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(manifest), filepath.Join(global, "deep-alias")); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	list := codexListResult(t, fx, 1)
	source := codexNamed(t, list, "deep-skill")
	if source.Availability.NativeName != "" || source.Availability.NativeNameEvidence.Kind != "unknown" || source.Availability.Precedence.State != "unknown" {
		t.Fatal("bounded namespace search guessed a name")
	}
	result := fx.run(t, "list", "--agent", "codex", "--project", fx.project, "--problem", "native-input", "--json")
	assertExit(t, result, 1)
	var view publicList
	decodeJSON(t, result.stdout, &view)
	if len(view.Sources) != 1 || view.Sources[0].ID != source.ID || len(view.Problems) != 1 || len(view.Problems[0].SourceIDs) != 1 || len(view.Problems[0].RootIDs) != 1 {
		t.Fatalf("namespace limit diagnostic reached unaffected sources: %+v", view)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE26ReviewNativeRootSummaries(t *testing.T) {
	for _, kind := range []string{"configuration", "invocation"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			manifest := codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "safe", "safe")
			if kind == "configuration" {
				writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte("skills.config = ["), 0600)
			} else {
				writeFile(t, filepath.Join(filepath.Dir(manifest), "agents", "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: invalid\n"), 0600)
			}
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "codex", "--project", fx.project, "--json")
			assertExit(t, result, 1)
			var list publicList
			decodeJSON(t, result.stdout, &list)
			if list.Scope.Completeness != "incomplete" {
				t.Fatal("native diagnostic lost coverage limitation")
			}
			if kind == "invocation" {
				if len(sourceNamed(t, list.Sources, "safe").Problems) != 1 || len(sourceNamed(t, list.Sources, "home-native").Problems) != 0 {
					t.Fatal("invocation diagnostic reached unaffected source")
				}
			}
			for _, root := range list.Scope.Roots {
				count := 0
				limited := false
				for _, problem := range list.Problems {
					for _, id := range problem.RootIDs {
						if id == root.ID {
							count++
							limited = limited || problem.LimitsCoverage
						}
					}
				}
				sources := 0
				for _, reference := range list.References {
					if reference.RootID == root.ID && reference.SourceID != "" {
						sources++
					}
				}
				if root.SourceCount != sources || root.ProblemCount != count || (limited && root.State != "missing" && root.State != "unreadable" && root.State != "incomplete") {
					t.Fatalf("root summary disagrees with diagnostics: %+v, count=%d limited=%t", root, count, limited)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
