package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

type piRelatedList struct {
	Scope      collection.Scope
	Sources    []collection.Source
	References []collection.Reference
	Problems   []collection.Problem
}

func piRelatedInventory(t *testing.T, fx fixture, exit int, extra ...string) piRelatedList {
	t.Helper()
	args := append([]string{"list", "--agent", "pi", "--project", fx.project, "--json"}, extra...)
	r := fx.run(t, args...)
	assertExit(t, r, exit)
	var list piRelatedList
	decodeJSON(t, r.stdout, &list)
	return list
}

func piRelatedSource(t *testing.T, list piRelatedList, directory string) collection.Source {
	t.Helper()
	for _, source := range list.Sources {
		if source.Directory == directory {
			return source
		}
	}
	t.Fatalf("source absent: %s", directory)
	return collection.Source{}
}

func piRetainedFixture(t *testing.T, scenario string) (fixture, string, string, string, string) {
	t.Helper()
	fx := piFixture(t)
	pkg := filepath.Join(fx.base, "allowed", "P")
	p := filepath.Dir(codexSkill(t, filepath.Join(pkg, "skills"), "bundle", "collision"))
	x := filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "X"), "bundle", "collision"))
	alias := filepath.Join(fx.project, "alias")
	if err := os.Symlink(p, alias); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"pi":{"skills":["skills"]}}`), 0600)
	piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), map[string]any{"defaultProjectTrust": "always", "packages": []any{map[string]any{"source": pkg, "skills": []string{}}}})
	patterns := []string{filepath.Join(pkg, "skills"), x, alias}
	pState, xState := "shadowed", "available"
	switch scenario {
	case "alias-first":
		patterns = []string{filepath.Join(pkg, "skills"), alias, x}
		pState, xState = "available", "shadowed"
	case "explicit-only":
		writeFile(t, filepath.Join(p, "SKILL.md"), []byte("---\nname: collision\ndescription: Explicit fixture.\ndisable-model-invocation: true\n---\nBody.\n"), 0600)
	case "disabled-alias":
		patterns = append(patterns, "-"+alias)
		pState = "disabled"
	case "unknown-alias":
		writeFile(t, filepath.Join(p, ".ignore"), []byte("*\n"), 0600)
		pState = "unknown"
	case "disabled-X":
		patterns = append(patterns, "-"+x)
		pState, xState = "available", "disabled"
	}
	piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), map[string]any{"skills": patterns})
	return fx, p, x, pState, xState
}

func TestALE33RetainedOccurrenceNameOrder(t *testing.T) {
	for _, scenario := range []string{"X-first", "alias-first", "explicit-only", "disabled-alias", "unknown-alias", "disabled-X"} {
		t.Run(scenario, func(t *testing.T) {
			fx, p, x, pState, xState := piRetainedFixture(t, scenario)
			before := treeManifest(t, fx.base)
			exit := 0
			if scenario == "unknown-alias" {
				exit = 1
			}
			list := piRelatedInventory(t, fx, exit)
			pSource, xSource := piRelatedSource(t, list, p), piRelatedSource(t, list, x)
			if pSource.ID == xSource.ID || string(pSource.Availability.State) != pState || string(xSource.Availability.State) != xState {
				t.Errorf("retained order: P=%s X=%s, want %s/%s", pSource.Availability.State, xSource.Availability.State, pState, xState)
			}
			if len(pSource.Availability.References) != 3 {
				t.Errorf("suppressed facts lost: %+v", pSource.Availability.References)
			}
			for _, ref := range list.References {
				if ref.ID == pSource.Availability.ReferenceID && ref.DiscoveredPath != filepath.Join(fx.project, "alias") {
					t.Errorf("wrong controlling occurrence: %+v", ref)
				}
			}
			if pState == "shadowed" && pSource.Availability.Precedence.Winner != xSource.ID || xState == "shadowed" && xSource.Availability.Precedence.Winner != pSource.ID {
				t.Error("winner identity disagrees with retained order")
			}
			for _, state := range []string{pState, xState} {
				filtered := piRelatedInventory(t, fx, exit, "--availability", state)
				for _, s := range filtered.Sources {
					if string(s.Availability.State) != state {
						t.Error("availability filter diverged")
					}
				}
			}
			for _, source := range []collection.Source{pSource, xSource} {
				for _, op := range []string{"read", "explain"} {
					r := fx.run(t, op, "--agent", "pi", "--project", fx.project, "--json", string(source.ID))
					assertExit(t, r, exit)
					var detail struct{ Source collection.Source }
					decodeJSON(t, r.stdout, &detail)
					if detail.Source.Availability.ReferenceID != source.Availability.ReferenceID || detail.Source.Availability.State != source.Availability.State {
						t.Error("detail lost retained occurrence")
					}
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func piPackageProblemFixture(t *testing.T, scenario string) (fixture, string, int) {
	t.Helper()
	fx := piFixture(t)
	pkg := filepath.Join(fx.base, "allowed", "P")
	p := filepath.Dir(codexSkill(t, filepath.Join(pkg, "skills"), "bundle", "package-P"))
	q := filepath.Join(fx.base, "allowed", "Q")
	codexSkill(t, filepath.Join(q, "skills"), "bundle", "package-Q")
	writeFile(t, filepath.Join(q, "package.json"), []byte(`{"pi":{"skills":["skills"]}}`), 0600)
	global := filepath.Join(fx.home, ".pi", "agent", "skills")
	codexSkill(t, global, "healthy", "healthy")
	manifest, exit := `{"pi":{"skills":["skills",null]}}`, 1
	switch scenario {
	case "malformed":
		manifest = `{`
	case "filters":
		manifest = `{"pi":{"skills":["skills","!absent"]}}`
	case "glob":
		manifest = `{"pi":{"skills":["skills/*"]}}`
	case "invalid-glob":
		manifest = `{"pi":{"skills":["skills","*["]}}`
	case "rootless":
		manifest = `{"pi":{"skills":["missing*"]}}`
	case "empty":
		manifest, exit = `{"pi":{"skills":[]}}`, 0
	case "shared":
		if err := os.Symlink(p, filepath.Join(global, "alias")); err != nil {
			t.Fatal(err)
		}
	case "context":
		manifest = `{"pi":{"skills":["skills"]}}`
	}
	writeFile(t, filepath.Join(pkg, "package.json"), []byte(manifest), 0600)
	settings := map[string]any{"defaultProjectTrust": "always", "packages": []any{map[string]any{"source": pkg}, map[string]any{"source": q}}}
	if scenario == "context" {
		settings["skills"] = 7
	}
	piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), settings)
	return fx, pkg, exit
}

func TestALE33PackageDiagnosticOwnership(t *testing.T) {
	for _, scenario := range []string{"mixed-null", "malformed", "filters", "glob", "invalid-glob", "rootless", "empty", "shared", "context"} {
		t.Run(scenario, func(t *testing.T) {
			fx, pkg, exit := piPackageProblemFixture(t, scenario)
			before := treeManifest(t, fx.base)
			list := piRelatedInventory(t, fx, exit)
			filtered := piRelatedInventory(t, fx, exit, "--problem", "native-input")
			if scenario == "empty" && len(filtered.Sources) != 0 || scenario == "rootless" && len(filtered.Sources) != 0 {
				t.Errorf("rootless/empty package contaminated sources: %+v", filtered.Sources)
			}
			for _, source := range list.Sources {
				owned := strings.HasPrefix(source.Directory, pkg+string(filepath.Separator))
				if scenario != "context" && owned && exit == 1 && len(source.Problems) == 0 {
					t.Errorf("owning source lost diagnostic: %s", source.Name)
				}
				if scenario != "context" && !owned && len(source.Problems) > 0 {
					t.Errorf("unrelated source has package diagnostic: %s", source.Name)
				}
				if scenario != "context" && !owned && source.Availability.State != collection.AvailabilityAvailable {
					t.Errorf("unrelated source availability changed: %s", source.Name)
				}
				for _, op := range []string{"read", "explain"} {
					r := fx.run(t, op, "--agent", "pi", "--project", fx.project, "--json", string(source.ID))
					assertExit(t, r, exit)
					var detail struct{ Source collection.Source }
					decodeJSON(t, r.stdout, &detail)
					if len(detail.Source.Problems) != len(source.Problems) || detail.Source.Availability.State != source.Availability.State {
						t.Error("detail ownership disagrees with list")
					}
				}
			}
			for _, source := range filtered.Sources {
				if scenario != "context" && !strings.HasPrefix(source.Directory, pkg+string(filepath.Separator)) {
					t.Errorf("problem filter included unrelated %s", source.Name)
				}
			}
			for _, problem := range list.Problems {
				if scenario == "rootless" && (len(problem.RootIDs) != 0 || len(problem.ReferenceIDs) != 0 || len(problem.SourceIDs) != 0) {
					t.Errorf("rootless local error expanded globally: %+v", problem)
				}
			}
			for _, ref := range list.References {
				if scenario != "context" && ref.Context != collection.ContextManaged && len(ref.Problems) != 0 {
					t.Errorf("shared/healthy reference contaminated: %+v", ref)
				}
			}
			for _, report := range list.Scope.Roots {
				owned := strings.HasPrefix(report.Requested, pkg+string(filepath.Separator))
				if scenario != "context" && !owned && (report.ProblemCount > 0 || report.State == collection.RootIncomplete) {
					t.Errorf("unrelated root contaminated: %+v", report)
				}
			}
			if exit == 1 && list.Scope.Completeness != collection.Incomplete || exit == 0 && list.Scope.Completeness != collection.Complete {
				t.Error("coverage erased local errors")
			}
			if scenario == "rootless" && len(list.Problems) == 0 {
				t.Error("rootless local error disappeared")
			}
			if scenario == "shared" {
				for _, selector := range [][]string{{"--context", "global"}, {"--source-root", filepath.Join(fx.home, ".pi", "agent", "skills")}} {
					view := piRelatedInventory(t, fx, exit, append([]string{"--problem", "native-input"}, selector...)...)
					if len(view.Sources) != 0 || len(view.Problems) != 0 {
						t.Errorf("alias-only filter inherited package problem: %+v", view)
					}
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE33CodexInvocationProblemOwnership(t *testing.T) {
	fx, _ := codexFixture(t)
	global := filepath.Join(fx.home, ".agents", "skills")
	manifest := codexSkill(t, global, "bad", "bad-policy")
	codexSkill(t, global, "healthy", "healthy-policy")
	writeFile(t, filepath.Join(filepath.Dir(manifest), "agents", "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: 'false'\n"), 0600)
	if err := os.Symlink(filepath.Dir(manifest), filepath.Join(fx.cleanRoot, "alias")); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	r := fx.run(t, "list", "--agent", "codex", "--project", fx.project, "--root", fx.cleanRoot, "--json")
	assertExit(t, r, 1)
	var list piRelatedList
	decodeJSON(t, r.stdout, &list)
	bad := piRelatedSource(t, list, filepath.Dir(manifest))
	if len(bad.Problems) != 1 || len(bad.Problems[0].SourceIDs) != 1 || len(bad.Problems[0].ReferenceIDs) != 2 || len(bad.Problems[0].RootIDs) != 2 {
		t.Errorf("source-local propagation changed: %+v", bad.Problems)
	}
	for _, source := range list.Sources {
		if source.ID != bad.ID && len(source.Problems) != 0 {
			t.Errorf("invocation error reached unrelated %s", source.Name)
		}
	}
	for _, ref := range list.References {
		if ref.Source == bad.ID && len(ref.Problems) != 1 || ref.Source != bad.ID && len(ref.Problems) != 0 {
			t.Errorf("source-local alias propagation changed: %+v", ref)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE33RetainedScopeBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"name-order", "package-problem"} {
		t.Run(scenario, func(t *testing.T) {
			var fx fixture
			var directory string
			state, name, exit := "shadowed", "collision", 0
			if scenario == "name-order" {
				fx, directory, _, _, _ = piRetainedFixture(t, "X-first")
			} else {
				var pkg string
				fx, pkg, _ = piPackageProblemFixture(t, "mixed-null")
				directory = filepath.Join(pkg, "skills", "bundle")
				state, name, exit = "unknown", "package-P", 1
			}
			before := treeManifest(t, fx.base)
			source := piRelatedSource(t, piRelatedInventory(t, fx, exit), directory)
			socket := fmt.Sprintf("ale33-scope-%d-%s", os.Getpid(), scenario)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse --agent pi --project "+shellQuote(fx.project)+" --availability "+state)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", name)
			frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Read lines", "Availability: "+state)
			permission, kind := "allowed", "inferred"
			if state == "unknown" {
				permission, kind = "unknown", "unknown"
			}
			if !strings.Contains(frame, "Configured permission: "+permission+" ("+kind+")") || !strings.Contains(frame, "Selector: "+string(source.ID)) {
				t.Errorf("permission or selected identity disagrees: %s", frame)
			}
			assertPaneCells(t, frame, 80, 30)
			writeEvidence(t, "scope/"+scenario+"/browser.txt", []byte(frame))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
