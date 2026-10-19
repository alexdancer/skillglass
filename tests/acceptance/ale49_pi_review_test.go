package acceptance_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE49PiProjectRegistrationProgression(t *testing.T) {
	for _, action := range []collection.ControlChoice{collection.ControlEnable, collection.ControlDisable} {
		for _, count := range []int{1, 2} {
			for _, existing := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-%d-existing-%t", action, count, existing), func(t *testing.T) {
					fx := piFixture(t)
					root := filepath.Join(fx.home, ".pi/agent/skills")
					draft := ale46Draft([]string{"pi"}, collection.ControlProject, fx.project)
					manifests := []string{}
					chosen := []string{}
					global := map[string]any{"defaultProjectTrust": "always", "keep": true}
					disabled := []string{}
					for i := 0; i < 3; i++ {
						name := fmt.Sprintf("item-%d", i)
						manifest := codexSkill(t, root, name, name)
						manifests = append(manifests, manifest)
						id := codexNamed(t, piList(t, fx, 0), name).ID
						draft.Sources = append(draft.Sources, ale46Assess(id))
						draft.Changes = append(draft.Changes, ale46Change(name, id, "pi", action))
						if i < count {
							chosen = append(chosen, name)
						}
						if action == collection.ControlEnable {
							disabled = append(disabled, "-"+manifest)
						}
					}
					global["skills"] = disabled
					writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), mustJSON(t, global), 0600)
					codexSkill(t, root, "kept", "kept")
					keptID := codexNamed(t, piList(t, fx, 0), "kept").ID
					draft.Context.KeepEnabled = []collection.SourceID{collection.SourceID(keptID)}
					draft.Sources = append(draft.Sources, ale46Assess(keptID))
					writeFile(t, filepath.Join(filepath.Dir(manifests[0]), "guide.md"), []byte("Reviewed supporting evidence.\n"), 0600)
					draft.Sources[0].SupportingPaths = []string{"guide.md"}
					config := filepath.Join(fx.project, ".pi/settings.json")
					if existing {
						writeFile(t, config, []byte(`{"theme":"retained","skills":[]}`), 0600)
					}
					selection, path := ale47Approved(t, fx, draft, chosen)
					before := ale49PiView(t, fx)
					home := treeManifest(t, fx.home)
					out := ale47Apply(t, fx, path, "", 0)
					if out.State != "applied" || len(out.Results) != count || !reflect.DeepEqual(out.Omitted, selection.Omitted) || len(out.Omitted) != 3-count {
						t.Fatalf("untruthful batch result: %+v", out)
					}
					want := collection.AvailabilityAvailable
					if action == collection.ControlDisable {
						want = collection.AvailabilityDisabled
					}
					for i, receipt := range out.Results {
						if receipt.ID != chosen[i] || receipt.State != "succeeded" || receipt.Outcome == nil || receipt.Outcome.WriteState != "committed" || receipt.Outcome.Persistence != "verified" || receipt.Outcome.Achievement != "achieved" || receipt.FinalObservation != "observed" || receipt.Final == nil || receipt.Final.State != want {
							t.Fatalf("untruthful item: %+v", receipt)
						}
						if i > 0 && selection.Effects[i].BeforeSHA256 != selection.Effects[i-1].AfterSHA256 {
							t.Fatal("effects do not compose")
						}
					}
					raw := readBytes(t, config)
					if fmt.Sprintf("%x", sha256.Sum256(raw)) != selection.Effects[count-1].AfterSHA256 {
						t.Fatal("saved configuration differs from the exact approved postimage")
					}
					var saved struct {
						Theme  string   `json:"theme"`
						Skills []string `json:"skills"`
					}
					if err := json.Unmarshal(raw, &saved); err != nil {
						t.Fatal(err)
					}
					if existing && saved.Theme != "retained" || len(saved.Skills) != 2*count {
						t.Fatalf("unrelated settings lost or omitted registration added: %+v", saved)
					}
					for i := 0; i < count; i++ {
						prefix := "+"
						if action == collection.ControlDisable {
							prefix = "-"
						}
						if saved.Skills[2*i] != manifests[i] || saved.Skills[2*i+1] != prefix+manifests[i] {
							t.Fatalf("unexpected native registration pair: %v", saved.Skills)
						}
					}
					after := ale49PiView(t, fx)
					if len(after.Sources) != len(before.Sources) || len(after.Scope.Roots) != len(before.Scope.Roots)+count || len(after.References) != len(before.References)+count {
						t.Fatal("unexpected discovery expansion")
					}
					for _, old := range before.Scope.Roots {
						found := false
						for _, next := range after.Scope.Roots {
							found = found || reflect.DeepEqual(old, next)
						}
						if !found {
							t.Fatalf("existing root changed: %+v", old)
						}
					}
					for _, old := range before.References {
						found := false
						for _, next := range after.References {
							found = found || reflect.DeepEqual(old, next)
						}
						if !found {
							t.Fatalf("existing reference changed: %+v", old)
						}
					}
					for _, source := range after.Sources {
						selected := false
						for i := 0; i < count; i++ {
							if source.ID != draft.Changes[i].Source {
								continue
							}
							selected = true
							registered := false
							for _, ref := range after.References {
								for _, r := range after.Scope.Roots {
									registered = registered || ref.Source == source.ID && ref.Context == collection.ContextProject && ref.Root == r.ID && r.Origin == "pi-configured-project" && r.Requested == manifests[i]
								}
							}
							if !registered || source.Availability.State != want {
								t.Fatalf("exact project registration or configured result missing: %+v", source)
							}
						}
						if !selected {
							for _, old := range before.Sources {
								if old.ID == source.ID && old.Availability.State != source.Availability.State {
									t.Fatal("omitted or protected source changed")
								}
							}
						}
					}
					assertTreeUnchanged(t, home, treeManifest(t, fx.home))
					stable := treeManifest(t, fx.base)
					stale := ale47Apply(t, fx, path, "", 1)
					if stale.State != "refused" {
						t.Fatal("stale approval accepted")
					}
					assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
					if count == 1 {
						_, path = ale47Approved(t, fx, draft, chosen)
						stable = treeManifest(t, fx.base)
						noop := ale47Apply(t, fx, path, "", 0)
						if noop.Results[0].State != "no-op" || noop.Results[0].Outcome.WriteState != "none" {
							t.Fatalf("unchanged native choice rewritten: %+v", noop)
						}
						assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
					}
					writeEvidence(t, "ale49/pi-outcome.json", mustJSON(t, out))
				})
			}
		}
	}
}

func ale49PiView(t *testing.T, fx fixture) collection.View {
	t.Helper()
	r := fx.run(t, "list", "--agent", "pi", "--project", fx.project, "--json")
	assertExit(t, r, 0)
	var view collection.View
	decodeJSON(t, r.stdout, &view)
	return view
}
