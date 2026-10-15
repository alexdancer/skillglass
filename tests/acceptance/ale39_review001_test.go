package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE39Review001UnknownPluginPrecedence(t *testing.T) {
	for _, topology := range []string{"project-plugin-home", "home-plugin-system", "lower-shared-aliases", "same-source-alias-control", "earlier-project-control"} {
		for _, mode := range []string{"enabled", "disabled", "unknown"} {
			t.Run(topology+"/"+mode, func(t *testing.T) {
				fx := codeafFixture(t)
				plugin := filepath.Join(fx.base, "allowed", "plugin")
				pluginSkill := filepath.Dir(codexSkill(t, filepath.Join(plugin, "skills"), "bundle", "collision"))
				lower := ""
				scope := "project"
				switch topology {
				case "home-plugin-system":
					scope = "user"
					lower = filepath.Dir(codexSkill(t, filepath.Join(fx.home, ".codex/skills/.system"), "system-bundle", "collision"))
				case "lower-shared-aliases", "same-source-alias-control":
					lower = pluginSkill
					if topology == "lower-shared-aliases" {
						lower = filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "shared"), "bundle", "collision"))
					}
					for _, root := range []string{".codeaf/skills", ".agents/skills"} {
						alias := filepath.Join(fx.home, root, "alias")
						if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(lower, alias); err != nil {
							t.Fatal(err)
						}
					}
				default:
					lower = filepath.Dir(codexSkill(t, filepath.Join(fx.home, ".codeaf/skills"), "home-bundle", "collision"))
				}
				earlier := ""
				if topology == "earlier-project-control" {
					earlier = filepath.Dir(codexSkill(t, filepath.Join(fx.project, ".codeaf/skills"), "earlier", "collision"))
				}
				registration := map[string]any{"plugins": map[string]any{"example@local": []any{map[string]any{"scope": scope, "projectPath": fx.project, "installPath": plugin, "version": "1"}}}}
				raw, err := json.Marshal(registration)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), raw, 0600)
				writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte(`{"enabledPlugins":{"example@local":true}}`), 0600)
				exit := 0
				local := filepath.Join(fx.project, ".claude/settings.local.json")
				if mode == "disabled" {
					writeFile(t, local, []byte(`{"enabledPlugins":{"example@local":false}}`), 0600)
				}
				if mode == "unknown" {
					writeFile(t, local, []byte(`{"enabledPlugins":`), 0600)
					exit = 1
				}
				before := treeManifest(t, fx.base)
				list := codeafListResult(t, fx, exit)
				lowerSource := codeafPath(t, list, lower)
				pluginSource := codeafPath(t, list, pluginSkill)
				want := "shadowed"
				if mode == "disabled" || topology == "same-source-alias-control" {
					want = "available"
				}
				if mode == "unknown" && topology != "same-source-alias-control" {
					want = "unknown"
				}
				if topology == "earlier-project-control" {
					want = "shadowed"
				}
				if lowerSource.Availability.State != want {
					t.Errorf("lower availability=%q, want literal %q; source=%+v", lowerSource.Availability.State, want, lowerSource)
				}
				if want == "available" {
					controlling := ""
					for _, fact := range lowerSource.Availability.References {
						if fact.Participation == "eligible" {
							controlling = fact.ReferenceID
						}
						if topology == "same-source-alias-control" && fact.Provenance.Kind == "plugin" {
							permission := map[string]string{"enabled": "allowed", "disabled": "denied", "unknown": "unknown"}[mode]
							if fact.Permission.State != permission {
								t.Errorf("plugin permission=%q, want %q", fact.Permission.State, permission)
							}
						}
					}
					if controlling == "" || lowerSource.Availability.ReferenceID != controlling {
						t.Errorf("controlling reference=%q, want eligible reference %q", lowerSource.Availability.ReferenceID, controlling)
					}
					for _, verb := range []string{"read", "explain"} {
						result := fx.run(t, verb, "--agent", "codeaf", "--project", fx.project, lowerSource.ID)
						assertExit(t, result, exit)
						if !strings.Contains(string(result.stdout), "Configured permission: allowed") {
							t.Errorf("%s must summarize winning permission: %s", verb, result.stdout)
						}
					}
				}
				if want == "unknown" && (lowerSource.Availability.Precedence.State != "unknown" || lowerSource.Availability.Precedence.Winner != "") {
					t.Errorf("lower precedence state=%q winner=%q, want literal state=unknown winner=empty", lowerSource.Availability.Precedence.State, lowerSource.Availability.Precedence.Winner)
				}
				if want == "unknown" {
					for _, fact := range lowerSource.Availability.References {
						if fact.Participation != "unknown" || fact.Permission.State != "allowed" {
							t.Errorf("lower reference must retain allowed permission but uncertain participation: %+v", fact)
						}
					}
					if strings.Join(lowerSource.Availability.Precedence.RelatedSources, ",") != pluginSource.ID {
						t.Errorf("uncertain competitor missing from precedence: %+v", lowerSource.Availability.Precedence)
					}
				}
				if mode == "unknown" && topology != "same-source-alias-control" && pluginSource.Availability.State != "unknown" {
					t.Errorf("plugin uncertainty not retained: %+v", pluginSource)
				}
				if topology == "lower-shared-aliases" && len(lowerSource.Availability.References) != 2 {
					t.Errorf("shared references=%d want 2", len(lowerSource.Availability.References))
				}
				if earlier != "" {
					s := codeafPath(t, list, earlier)
					if s.Availability.State != "available" || lowerSource.Availability.Precedence.Winner != s.ID {
						t.Errorf("earlier assured project winner changed: %+v lower=%+v", s, lowerSource)
					}
				}
				unrelated := codexNamed(t, list, "home-native")
				if unrelated.Availability.State != "available" {
					t.Errorf("unrelated name demoted: %+v", unrelated)
				}
				detail := fx.run(t, "explain", "--agent", "codeaf", "--project", fx.project, lowerSource.ID)
				assertExit(t, detail, exit)
				if !strings.Contains(string(detail.stdout), "Availability: "+want) {
					t.Errorf("public explain missing literal Availability: %s; stdout=%s", want, detail.stdout)
				}
				if want == "unknown" && (!strings.Contains(string(detail.stdout), "Precedence: unknown") || strings.Contains(string(detail.stdout), "Winning source:")) {
					t.Errorf("public explain must not assert a definite winner: %s", detail.stdout)
				}
				if mode == "unknown" && list.Scope.Completeness != "incomplete" {
					t.Errorf("uncertain settings should report incomplete coverage: %s", list.Scope.Completeness)
				}
				if topology == "same-source-alias-control" {
					tmux, socket, target := startALE43ReviewBrowser(t, fx, 120, 40, "--agent", "codeaf", "--project", fx.project)
					runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "/")
					waitForPaneAt(t, tmux, fx, socket, target, false, "Enter keep")
					runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "collision")
					waitForPaneAt(t, tmux, fx, socket, target, false, "Focus: 1/1")
					frame := actForPaneAt(t, tmux, fx, socket, target, false, func() {
						runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter", "Enter", "d")
					}, "Source ID: "+lowerSource.ID, "Availability: available", "Configured permission: allowed")
					assertPaneCells(t, frame, 120, 40)
					writeEvidence(t, "codeaf-controlling-reference/"+mode+".txt", []byte(frame))
					runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "q")
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE39Review001InterposedPluginAlias(t *testing.T) {
	fx := codeafFixture(t)
	first := filepath.Join(fx.base, "allowed", "first-plugin")
	second := filepath.Join(fx.base, "allowed", "second-plugin")
	firstSkill := filepath.Dir(codexSkill(t, filepath.Join(first, "skills"), "bundle", "collision"))
	secondSkill := filepath.Dir(codexSkill(t, filepath.Join(second, "skills"), "bundle", "collision"))
	alias := filepath.Join(fx.home, ".codeaf/skills/alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(firstSkill, alias); err != nil {
		t.Fatal(err)
	}
	registration := map[string]any{"plugins": map[string]any{
		"alpha@local": []any{map[string]any{"scope": "project", "projectPath": fx.project, "installPath": first}},
		"beta@local":  []any{map[string]any{"scope": "project", "projectPath": fx.project, "installPath": second}},
	}}
	raw, err := json.Marshal(registration)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), raw, 0600)
	writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte(`{"enabledPlugins":{"alpha@local":true,"beta@local":true}}`), 0600)
	local := filepath.Join(fx.project, ".claude/settings.local.json")
	for _, mode := range []string{"unknown", "disabled", "enabled"} {
		t.Run(mode, func(t *testing.T) {
			settings := `{"enabledPlugins":`
			exit := 1
			if mode == "disabled" {
				settings = `{"enabledPlugins":{"alpha@local":false,"beta@local":false}}`
				exit = 0
			} else if mode == "enabled" {
				settings = `{"enabledPlugins":{"alpha@local":true,"beta@local":true}}`
				exit = 0
			}
			writeFile(t, local, []byte(settings), 0600)
			before := treeManifest(t, fx.base)
			list := codeafListResult(t, fx, exit)
			one := codeafPath(t, list, firstSkill)
			two := codeafPath(t, list, secondSkill)
			if mode == "unknown" {
				for _, source := range []codexSource{one, two} {
					if source.Availability.State != "unknown" || source.Availability.Precedence.State != "unknown" || source.Availability.Precedence.Winner != "" {
						t.Fatalf("interposed distinct plugin prevents a definite alias winner: %+v", source)
					}
					for _, fact := range source.Availability.References {
						if fact.Participation != "unknown" {
							t.Fatalf("reference asserted certain participation: %+v", fact)
						}
					}
				}
			} else if one.Availability.State != "available" {
				t.Fatalf("certain same-source route lost: %+v", one)
			} else if mode == "enabled" && (two.Availability.State != "shadowed" || two.Availability.Precedence.Winner != one.ID) {
				t.Fatalf("known plugin ordering changed: %+v", two)
			} else if mode == "disabled" && two.Availability.State != "unavailable" {
				t.Fatalf("disabled plugin participates: %+v", two)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
