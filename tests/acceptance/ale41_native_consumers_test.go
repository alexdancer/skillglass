package acceptance_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41NativeConsumerStatesRemainVisible(t *testing.T) {
	for _, scenario := range []string{"disabled", "shadowed", "inactive"} {
		t.Run(scenario, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			agent := "codex"
			switch scenario {
			case "disabled":
				apps := []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini"}
				p := ale40Preview(t, fx, apps, "global", id, 0)
				ale40Apply(t, fx, apps, id, p.Confirmation, "", 0)
			case "shadowed":
				target = codexSkill(t, filepath.Join(fx.project, ".claude/skills"), "priority", "ale40-target")
				agent = "claude-code"
				r := fx.run(t, "list", "--agent", agent, "--project", fx.project, "--json")
				var listed collection.View
				decodeJSON(t, r.stdout, &listed)
				found := false
				for _, source := range listed.Sources {
					if source.Directory == filepath.Dir(target) {
						id = string(source.ID)
						found = true
					}
				}
				if !found {
					t.Fatal("shadowed source missing from native inventory")
				}
			case "inactive":
				root := filepath.Join(fx.home, ".pi/agent/npm/node_modules/pkg/skills")
				target = codexSkill(t, root, "inactive", "inactive")
				listed := piList(t, fx, 0)
				id = codexNamed(t, listed, "inactive").ID
				agent = "pi"
			}
			before := treeManifest(t, fx.base)
			r := changeCommand(t, fx, "", "delete", "--agent", agent, "--project", fx.project, "--json", id)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if scenario == "inactive" {
				if p.State != "refused" {
					t.Fatal("managed cache was accepted", p.Reason)
				}
			} else if p.State != "ready" {
				t.Fatal(p.State, p.Reason)
			}
			observed := false
			for _, consumer := range p.Consumers {
				ids := map[collection.ReferenceID]bool{}
				for _, ref := range consumer.References {
					ids[ref.ID] = true
				}
				if consumer.Availability == nil {
					continue
				}
				for _, fact := range consumer.Availability.References {
					if !ids[fact.Reference] || fact.Participation == collection.ParticipationInspection {
						t.Fatal("availability projection includes a non-native inspection", consumer.Application, fact)
					}
					if scenario == "inactive" && consumer.Application == "pi" && fact.Participation == collection.ParticipationExcluded {
						observed = true
					}
				}
				if scenario == "disabled" && consumer.Application != "codeaf" {
					if len(consumer.References) == 0 || consumer.Availability.State != collection.AvailabilityDisabled {
						t.Fatal("disabled native route disappeared", consumer.Application, consumer.Availability.State)
					}
					observed = true
				}
				if scenario == "shadowed" && consumer.Application == "claude-code" {
					if len(consumer.References) == 0 || consumer.Availability.State != collection.AvailabilityShadowed {
						t.Fatal("shadowed native route disappeared", consumer.Availability.State)
					}
					observed = true
				}
			}
			if !observed {
				t.Fatal("expected native state was not exposed", scenario)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if _, err := os.Stat(target); err != nil {
				t.Fatal("preview changed content", err)
			}
		})
	}
}
