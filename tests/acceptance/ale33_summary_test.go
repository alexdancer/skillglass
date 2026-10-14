package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func piSummaryFixture(t *testing.T, scenario string) (fixture, string, string, string) {
	t.Helper()
	fx := piFixture(t)
	pkg := filepath.Join(fx.base, "allowed", "package")
	manifest := codexSkill(t, filepath.Join(pkg, "skills"), "shared", "summary-shared")
	writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"pi":{"skills":["skills"]}}`), 0600)
	registration := map[string]any{"source": pkg}
	state, permission := "available", "allowed"
	if scenario == "package-disabled" || scenario == "project-enabled" || scenario == "explicit-only" || scenario == "shadowed" {
		registration["skills"] = []string{}
	}
	piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), map[string]any{"defaultProjectTrust": "always", "packages": []any{registration}})
	if scenario == "package-disabled" {
		state, permission = "disabled", "denied"
	}
	if !strings.HasPrefix(scenario, "package-") {
		alias := filepath.Join(fx.project, "selected-alias")
		if err := os.Symlink(filepath.Dir(manifest), alias); err != nil {
			t.Fatal(err)
		}
		patterns := []string{alias}
		if scenario == "project-disabled" {
			patterns = append(patterns, "-"+alias)
			state, permission = "disabled", "denied"
		}
		if scenario == "explicit-only" {
			writeFile(t, manifest, []byte("---\nname: summary-shared\ndescription: Inert summary fixture.\ndisable-model-invocation: true\n---\nBody.\n"), 0600)
			state = "explicit-only"
		}
		if scenario == "shadowed" {
			winner := codexSkill(t, filepath.Join(fx.project, "winner"), "bundle", "summary-shared")
			patterns = append([]string{filepath.Dir(winner)}, patterns...)
			state = "shadowed"
		}
		piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), map[string]any{"skills": patterns})
	}
	return fx, filepath.Dir(manifest), state, permission
}

func piSummarySource(t *testing.T, fx fixture, directory, state string) codexSource {
	t.Helper()
	r := fx.run(t, "list", "--agent", "pi", "--project", fx.project, "--json")
	assertExit(t, r, 0)
	var list codexList
	decodeJSON(t, r.stdout, &list)
	var contract struct {
		Sources []struct {
			ID           string
			Availability collection.AvailabilityExplanation
		}
	}
	decodeJSON(t, r.stdout, &contract)
	writeEvidence(t, "summary/"+strings.ReplaceAll(t.Name(), "/", "-")+"/list.json", r.stdout)
	if list.Scope.Completeness != "complete" {
		t.Fatalf("incomplete fixture: %+v", list.Scope)
	}
	for _, source := range list.Sources {
		if source.SourcePath == directory {
			if source.Availability.State != state {
				t.Fatalf("availability: %+v", source)
			}
			for _, item := range contract.Sources {
				if item.ID == source.ID && string(item.Availability.ReferenceID) != source.Availability.References[len(source.Availability.References)-1].ReferenceID {
					t.Errorf("list governing reference: %+v", item.Availability)
				}
			}
			return source
		}
	}
	t.Fatalf("shared source not found: %+v", list.Sources)
	return codexSource{}
}

func assertSummary(t *testing.T, output, state, permission string) {
	t.Helper()
	for _, line := range []string{"Availability: " + state + " (inferred)", "Configured permission: " + permission + " (inferred)"} {
		if !strings.Contains(output, line) {
			t.Errorf("summary lacks %q:\n%s", line, output)
		}
	}
}

func TestALE33GoverningPermissionSummaryCLI(t *testing.T) {
	for _, scenario := range []string{"project-disabled", "project-enabled", "package-enabled", "package-disabled", "explicit-only", "shadowed"} {
		t.Run(scenario, func(t *testing.T) {
			fx, directory, state, permission := piSummaryFixture(t, scenario)
			before := treeManifest(t, fx.base)
			source := piSummarySource(t, fx, directory, state)
			wantRefs := 2
			if strings.HasPrefix(scenario, "package-") {
				wantRefs = 1
			}
			if len(source.Availability.References) != wantRefs {
				t.Fatalf("reference facts lost: %+v", source)
			}
			if wantRefs == 2 && source.Availability.References[0].Permission.State == permission {
				t.Fatal("fixture does not exercise opposing permissions")
			}
			for _, operation := range []string{"read", "explain"} {
				r := fx.run(t, operation, "--agent", "pi", "--project", fx.project, source.ID)
				assertExit(t, r, 0)
				assertSummary(t, string(r.stdout), state, permission)
				for _, fact := range source.Availability.References {
					if !strings.Contains(string(r.stdout), "Reference "+fact.ReferenceID+": "+fact.Participation) || !strings.Contains(string(r.stdout), "Permission: "+fact.Permission.State+" (inferred)") {
						t.Errorf("reference detail lost: %s", r.stdout)
					}
				}
				jsonResult := fx.run(t, operation, "--agent", "pi", "--project", fx.project, "--json", source.ID)
				assertExit(t, jsonResult, 0)
				var detail struct{ Source codexSource }
				decodeJSON(t, jsonResult.stdout, &detail)
				if detail.Source.ID != source.ID || !reflect.DeepEqual(detail.Source.Availability, source.Availability) {
					t.Errorf("snapshot explanation changed: %+v", detail)
				}
				var contract struct {
					Source struct {
						Availability collection.AvailabilityExplanation
					}
				}
				decodeJSON(t, jsonResult.stdout, &contract)
				governing := contract.Source.Availability.ReferenceID
				want := source.Availability.References[wantRefs-1].ReferenceID
				if string(governing) != want {
					t.Errorf("governing reference = %q, want %q", governing, want)
				}
				writeEvidence(t, "summary/"+scenario+"/"+operation+".txt", r.stdout)
				writeEvidence(t, "summary/"+scenario+"/"+operation+".json", jsonResult.stdout)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE33GoverningPermissionSummaryCodexFallback(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			fx, _ := codexFixture(t)
			manifest := codexSkill(t, filepath.Join(fx.home, ".codex", "skills"), "summary", "summary-control")
			writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte(selector(manifest, enabled)), 0600)
			before := treeManifest(t, fx.base)
			source := codexNamed(t, codexListResult(t, fx, 0), "summary-control")
			state, permission := "available", "allowed"
			if !enabled {
				state, permission = "disabled", "denied"
			}
			for _, operation := range []string{"read", "explain"} {
				r := fx.run(t, operation, "--agent", "codex", "--project", fx.project, source.ID)
				assertExit(t, r, 0)
				assertSummary(t, string(r.stdout), state, permission)
				jsonResult := fx.run(t, operation, "--agent", "codex", "--project", fx.project, "--json", source.ID)
				assertExit(t, jsonResult, 0)
				var detail struct {
					Source struct{ Availability map[string]any }
				}
				decodeJSON(t, jsonResult.stdout, &detail)
				if _, present := detail.Source.Availability["reference_id"]; present {
					t.Error("Codex unexpectedly adopted governing-reference contract")
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE33GoverningPermissionSummaryInspection(t *testing.T) {
	fx := piFixture(t)
	before := treeManifest(t, fx.base)
	r := fx.run(t, "list", "--agent", "pi", "--project", fx.project, "--root", fx.cleanRoot, "--json")
	assertExit(t, r, 0)
	var list publicList
	decodeJSON(t, r.stdout, &list)
	source := sourceNamed(t, list.Sources, "normal-skill")
	for _, operation := range []string{"read", "explain"} {
		r := fx.run(t, operation, "--agent", "pi", "--project", fx.project, "--root", fx.cleanRoot, source.ID)
		assertExit(t, r, 0)
		if !strings.Contains(string(r.stdout), "Availability: unknown (unknown)") || strings.Contains(string(r.stdout), "Configured permission:") {
			t.Errorf("inspection fabricated permission: %s", r.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE33GoverningPermissionSummaryBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"project-disabled", "project-enabled", "explicit-only"} {
		t.Run(scenario, func(t *testing.T) {
			fx, directory, state, permission := piSummaryFixture(t, scenario)
			before := treeManifest(t, fx.base)
			piSummarySource(t, fx, directory, state)
			socket := fmt.Sprintf("ale33-summary-%d-%s", os.Getpid(), scenario)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse --agent pi --project "+shellQuote(fx.project))
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "summary-shared")
			frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Read lines", "Availability: "+state)
			assertSummary(t, frame, state, permission)
			assertPaneCells(t, frame, 80, 30)
			writeEvidence(t, "summary/"+scenario+"/browser.txt", []byte(frame))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
