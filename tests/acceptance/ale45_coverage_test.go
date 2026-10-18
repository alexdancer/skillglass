package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestALE45SnapshotFailureCoverage(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "coverage-review")
	live, replaced := filepath.Join(root, "live"), filepath.Join(root, "replaced")
	writeSkill(t, live, "coverage", "Evidence.", "LIVE_INSTRUCTIONS")
	writeSkill(t, replaced, "coverage", "Evidence.", "OLD_INSTRUCTIONS")
	writeFile(t, filepath.Join(live, "guide.md"), []byte("SUPPORTING_EVIDENCE"), 0600)
	initial := fx.run(t, "list", "--root", root, "--json")
	assertExit(t, initial, 0)
	var inventory publicList
	decodeJSON(t, initial.stdout, &inventory)
	var stale string
	for _, source := range inventory.Sources {
		if source.SourcePath == replaced {
			stale = source.ID
		}
	}
	if stale == "" {
		t.Fatal("missing source to replace")
	}
	if err := os.Rename(replaced, filepath.Join(fx.base, "allowed", "old-kept")); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, replaced, "coverage", "Evidence.", "REPLACEMENT_INSTRUCTIONS")

	for _, incomplete := range []bool{false, true} {
		name, baselineCode := "complete", 0
		roots := []string{"--root", root, "--project", fx.project}
		if incomplete {
			name, baselineCode = "incomplete", 1
			roots = append(roots, "--root", fx.missingRoot)
		}
		t.Run(name, func(t *testing.T) {
			baseline := fx.run(t, append(append([]string{"list"}, roots...), "--json")...)
			assertExit(t, baseline, baselineCode)
			var list publicList
			decodeJSON(t, baseline.stdout, &list)
			if len(list.Sources) != 2 {
				t.Fatalf("baseline sources: %s", baseline.stdout)
			}
			id := ""
			for _, source := range list.Sources {
				if source.SourcePath == live {
					id = source.ID
				}
				if source.ID == stale {
					t.Fatal("replacement reused stale source identity")
				}
			}
			if id == "" {
				t.Fatal("missing live source")
			}
			var baselineRaw map[string]any
			decodeJSON(t, baseline.stdout, &baselineRaw)
			before := treeManifest(t, fx.base)
			checkContext := func(t *testing.T, result commandResult) {
				t.Helper()
				var raw map[string]any
				decodeJSON(t, result.stdout, &raw)
				if raw["scope"] == nil || !reflect.DeepEqual(raw["scope"], baselineRaw["scope"]) {
					t.Errorf("lost normalized scope: %s", result.stdout)
				}
				if incomplete {
					problems, ok := raw["problems"].([]any)
					if !ok || len(problems) == 0 {
						t.Errorf("missing discovery problems array: %s", result.stdout)
					} else if !reflect.DeepEqual(problems, baselineRaw["problems"]) {
						t.Errorf("lost discovery diagnostics: %s", result.stdout)
					}
				}
			}
			for _, operation := range []string{"read", "explain", "read-resource", "compare"} {
				selections := [][]string{{"unknown-id"}, {stale}}
				if operation == "compare" {
					selections = [][]string{{"unknown-id", id}, {id, "unknown-id"}, {stale, id}, {id, stale}, {stale, "unknown-id"}}
				} else if operation == "read-resource" {
					selections = [][]string{{"unknown-id", "guide.md"}, {stale, "guide.md"}}
				}
				for _, selection := range selections {
					t.Run(operation+"/"+strings.Join(selection, "/"), func(t *testing.T) {
						args := append(append([]string{operation}, roots...), "--json")
						result := fx.run(t, append(args, selection...)...)
						assertExit(t, result, 1)
						got := inspectResult(t, result)
						selected := "failed"
						if operation == "read-resource" {
							selected = "incomplete"
						}
						if got.Coverage.Discovery != name || got.Coverage.Selected != selected || got.Outcome != "failed" || got.Failure == nil || got.Failure.Action == "" {
							t.Errorf("selected failure coverage: %s", result.stdout)
						}
						checkContext(t, result)
						if operation == "compare" {
							human := fx.run(t, append(append([]string{operation}, roots...), selection...)...)
							assertExit(t, human, 1)
							if len(human.stdout) != 0 || !bytes.Contains(human.stderr, []byte("not in selected scope")) {
								t.Errorf("human comparison error changed: %s %s", human.stdout, human.stderr)
							}
						}
					})
				}
			}
			for _, operation := range []string{"list", "problems"} {
				for _, filter := range [][]string{{"--context", "invalid-context"}, {"--problem", "invalid-problem"}, {"--availability", "invalid-availability"}, {"--source-root", fx.emptyRoot}} {
					t.Run(operation+"/"+filter[0], func(t *testing.T) {
						args := append(append([]string{operation}, roots...), "--json")
						result := fx.run(t, append(args, filter...)...)
						assertExit(t, result, 2)
						got := inspectResult(t, result)
						if got.Coverage.Discovery != name || got.Coverage.Selected != "not-applicable" || got.Outcome != "failed" || got.Failure == nil || got.Failure.Code != "usage-error" || got.Failure.Action == "" {
							t.Errorf("filter failure coverage: %s", result.stdout)
						}
						var raw struct {
							Failure struct{ Message string }
							Help    string
						}
						decodeJSON(t, result.stdout, &raw)
						if raw.Failure.Message == "" || raw.Help == "" {
							t.Errorf("missing actionable usage failure: %s", result.stdout)
						}
						checkContext(t, result)
						human := fx.run(t, append(append([]string{operation}, roots...), filter...)...)
						assertExit(t, human, 2)
						if len(human.stdout) != 0 || !bytes.Contains(human.stderr, []byte("skillglass:")) || !bytes.Contains(human.stderr, []byte("Usage:")) {
							t.Errorf("human filter error changed: %s %s", human.stdout, human.stderr)
						}
					})
				}
			}
			for _, operation := range []string{"list", "problems", "read", "explain", "read-resource", "compare"} {
				t.Run(operation+"/success", func(t *testing.T) {
					args := append(append([]string{operation}, roots...), "--json")
					selected, code, outcome := "complete", baselineCode, "success"
					if incomplete {
						outcome = "partial"
					}
					switch operation {
					case "list", "problems":
						selected = "not-applicable"
					case "read-resource":
						args = append(args, id, "guide.md")
					case "compare":
						args, code = append(args, id, id), 0
					default:
						args = append(args, id)
					}
					result := fx.run(t, args...)
					assertExit(t, result, code)
					got := inspectResult(t, result)
					if got.Coverage.Discovery != name || got.Coverage.Selected != selected || got.Outcome != outcome {
						t.Errorf("ordinary inspection coverage: %s", result.stdout)
					}
					checkContext(t, result)
				})
			}
			after := treeManifest(t, fx.base)
			assertTreeUnchanged(t, before, after)
			writeEvidence(t, "ale45-coverage/"+name+"-tree-before.json", before)
			writeEvidence(t, "ale45-coverage/"+name+"-tree-after.json", after)
		})
	}
}

func TestALE45CommandHelpCoverage(t *testing.T) {
	fx := newFixture(t)
	before := treeManifest(t, fx.base)
	for _, operation := range []string{"list", "problems", "read", "explain", "read-resource", "compare"} {
		for _, help := range []string{"--help", "-h"} {
			t.Run(operation+"/"+help, func(t *testing.T) {
				result := fx.run(t, operation, "--json", "--root", fx.missingRoot, help)
				assertExit(t, result, 0)
				got := inspectResult(t, result)
				var raw map[string]json.RawMessage
				decodeJSON(t, result.stdout, &raw)
				if got.Coverage.Discovery != "not-started" || got.Coverage.Selected != "not-applicable" || got.Outcome != "success" || got.Failure != nil || string(raw["scope"]) != "null" || !bytes.Contains(raw["help"], []byte("read-resource")) {
					t.Errorf("help claims discovery or selection: %s", result.stdout)
				}
				human := fx.run(t, operation, help)
				assertExit(t, human, 0)
				if !bytes.Contains(human.stdout, []byte("Usage:")) || len(human.stderr) != 0 {
					t.Errorf("human help changed: %s %s", human.stdout, human.stderr)
				}
			})
		}
	}
	for _, operation := range []string{"help", "--help", "-h"} {
		result := fx.run(t, operation, "--json")
		assertExit(t, result, 0)
		got := inspectResult(t, result)
		if got.Coverage.Discovery != "not-started" || got.Coverage.Selected != "not-applicable" || got.Outcome != "success" || got.Failure != nil {
			t.Errorf("top-level help coverage: %s", result.stdout)
		}
	}
	for _, args := range [][]string{
		{"read", "--root", fx.cleanRoot, "--json"},
		{"explain", "--root", fx.cleanRoot, "--json"},
		{"read-resource", "--root", fx.cleanRoot, "--json"},
		{"compare", "--root", fx.cleanRoot, "--json"},
		{"read", "--agent", "unknown-agent", "--json", "id"},
		{"explain", "--agent", "unknown-agent", "--json", "id"},
		{"read-resource", "--agent", "unknown-agent", "--json", "id", "guide.md"},
		{"compare", "--agent", "unknown-agent", "--json", "left", "right"},
	} {
		result := fx.run(t, args...)
		assertExit(t, result, 2)
		got := inspectResult(t, result)
		if got.Coverage.Discovery != "not-started" || got.Coverage.Selected != "not-applicable" || got.Outcome != "failed" || got.Failure == nil || got.Failure.Code != "usage-error" || got.Failure.Action == "" {
			t.Errorf("usage failure without selection: %s", result.stdout)
		}
	}
	after := treeManifest(t, fx.base)
	assertTreeUnchanged(t, before, after)
	writeEvidence(t, "ale45-coverage/help-tree-before.json", before)
	writeEvidence(t, "ale45-coverage/help-tree-after.json", after)
}
