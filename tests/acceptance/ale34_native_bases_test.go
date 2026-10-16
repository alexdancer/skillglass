package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestALE34AgentsNativeBaseExactChoices(t *testing.T) {
	for _, location := range []string{"global", "project", "ancestor"} {
		for _, selector := range []string{"skills/target", "skills/target/SKILL.md"} {
			for _, prefix := range []string{"-", "+"} {
				t.Run(location+"/"+selector+"/"+prefix, func(t *testing.T) {
					fx := piFixture(t)
					base := fx.project
					scope := "project"
					config := filepath.Join(fx.project, ".pi/settings.json")
					if location == "global" {
						base = fx.home
						scope = "global"
						config = filepath.Join(fx.home, ".pi/agent/settings.json")
					} else if location == "ancestor" {
						fx.project = filepath.Join(fx.project, "nested")
						writeFile(t, filepath.Join(fx.project, "sentinel"), []byte("project"), 0600)
						config = filepath.Join(fx.project, ".pi/settings.json")
					}
					root := filepath.Join(base, ".agents/skills")
					target := codexSkill(t, root, "target", "target")
					sibling := codexSkill(t, root, "sibling", "sibling")
					raw := []byte(`{"defaultProjectTrust":"always","skills":["` + prefix + selector + `","!unrelated"],"keep":"\u0061"}`)
					writeFile(t, config, raw, 0600)
					source := piSourceAt(t, fx, target)
					want := "available"
					if prefix == "-" {
						want = "disabled"
					}
					if source.Availability.State != want {
						t.Fatal("native relative choice was not observed", source.Availability)
					}
					content, other := readBytes(t, target), readBytes(t, sibling)
					globalConfig := filepath.Join(fx.home, ".pi/agent/settings.json")
					globalBytes := readBytes(t, globalConfig)
					choice := "inherit"
					if scope == "global" {
						choice = "enable"
						if prefix == "+" {
							choice = "disable"
						}
					}
					piApply(t, fx, scope, choice, source.ID)
					var entries []string
					if err := json.Unmarshal(piSettingsValues(t, config)["skills"], &entries); err != nil {
						t.Fatal(err)
					}
					for _, entry := range entries {
						if entry == prefix+selector {
							t.Fatal("relative exact choice survived", string(readBytes(t, config)))
						}
					}
					want = "available"
					if choice == "disable" {
						want = "disabled"
					}
					if piSourceAt(t, fx, target).Availability.State != want {
						t.Fatal("requested result not achieved")
					}
					for _, next := range []string{"disable", "enable"} {
						piApply(t, fx, scope, next, source.ID)
					}
					if scope == "project" {
						piApply(t, fx, scope, "inherit", source.ID)
						if !piChange(t, fx, scope, "inherit", source.ID, "", 0).NoWriteNeeded || !bytes.Equal(globalBytes, readBytes(t, globalConfig)) {
							t.Fatal("inheritance is not idempotent or changed global settings")
						}
					}
					saved := readBytes(t, config)
					if !bytes.Contains(saved, []byte(`"keep":"\u0061"`)) || !bytes.Contains(saved, []byte(`"!unrelated"`)) || !bytes.Equal(content, readBytes(t, target)) || !bytes.Equal(other, readBytes(t, sibling)) || piSourceAt(t, fx, sibling).Availability.State != "available" {
						t.Fatal("unrelated settings or sources changed", string(saved))
					}
				})
			}
		}
	}
}

func TestALE34AgentsNativeBaseConfiguredOccurrenceAndAlias(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	alias := filepath.Join(fx.home, ".pi/agent/skills/alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(target), alias); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(fx.home, ".pi/agent/settings.json")
	raw, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "skills": []string{target, "-skills/target", "-skills/alias"}})
	writeFile(t, config, raw, 0600)
	id := piSourceAt(t, fx, target).ID
	piApply(t, fx, "global", "enable", id)
	var entries []string
	if err := json.Unmarshal(piSettingsValues(t, config)["skills"], &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry == "-skills/target" || entry == "-skills/alias" {
			t.Fatal("one native base or lexical alias was lost", entries)
		}
	}
	if len(entries) != 3 || entries[0] != target {
		t.Fatal("configured registration was lost or exact choices duplicated", entries)
	}
	piApply(t, fx, "global", "disable", id)
	if piSourceAt(t, fx, target).Availability.State != "disabled" {
		t.Fatal("all selected-source occurrences must be disabled")
	}
}

func TestALE34AgentsNativeBaseIndependentEffectsRefused(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			fx := piFixture(t)
			base := fx.home
			piBase := filepath.Join(fx.home, ".pi/agent")
			if scope == "project" {
				base = fx.project
				piBase = filepath.Join(fx.project, ".pi")
			}
			target := codexSkill(t, filepath.Join(base, ".agents/skills"), "target", "selected")
			other := codexSkill(t, filepath.Join(piBase, "skills"), "target", "independent")
			config := filepath.Join(piBase, "settings.json")
			writeFile(t, config, []byte(`{"defaultProjectTrust":"always","skills":["-skills/target"]}`), 0600)
			source := piSourceAt(t, fx, target)
			if source.Availability.State != "disabled" || piSourceAt(t, fx, other).Availability.State != "disabled" {
				t.Fatal("relative directive must govern both independent occurrences")
			}
			before := treeManifest(t, fx.base)
			choices := []string{"enable"}
			if scope == "project" {
				choices = append(choices, "inherit")
			}
			for _, choice := range choices {
				out := piChange(t, fx, scope, choice, source.ID, "", 1)
				if out.State != "refused" || out.Confirmation != "" {
					t.Fatal(out)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
		})
	}
}
