package acceptance_test

import (
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestDecisionALE42RootGroup(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "native"
		if explicit {
			name = "explicit-root"
		}
		t.Run(name, func(t *testing.T) {
			c := newALE42Collection(t)
			fx := c.fixture
			id := changeSourceID(t, fx, c.sharedManifest)
			args := []string{"disable", "--scope", "global", "--project", fx.project, "--json"}
			for _, application := range []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini"} {
				args = append(args, "--agents", application)
			}
			if explicit {
				args = append(args, "--root", filepath.Join(fx.home, ".agents/skills"))
			}
			args = append(args, id)
			before := treeManifest(t, fx.base)
			result := changeCommand(t, fx, "", args...)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			assertExit(t, result, 0)
			var view collection.GroupView
			decodeJSON(t, result.stdout, &view)
			if view.State != "ready" || len(view.Members) != 6 {
				t.Fatalf("proven supported group = %+v, want six ready members", view)
			}
			for _, member := range view.Members {
				if member.Preview.State != "ready" {
					t.Fatalf("member %s: %s - %s", member.Application, member.Preview.State, member.Preview.Reason)
				}
				if explicit && member.Application == "pi" {
					inspection := false
					for _, ref := range member.Preview.Before.References {
						if ref.Participation == collection.ParticipationInspection && ref.Provenance.Kind == "custom" && ref.Permission.State == collection.PermissionUnknown {
							inspection = true
						}
					}
					if !inspection {
						t.Fatal("Pi inspection-only permission diagnostic lost")
					}
				}
			}
		})
	}
}

func TestALE50PiInspectionControls(t *testing.T) {
	for _, scenario := range []string{"custom-root", "project-root", "native-permission", "native-participation", "native-ignore", "viewer-only", "prospective-effect"} {
		t.Run(scenario, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.home, ".pi/agent/skills")
			if scenario == "viewer-only" {
				root = filepath.Join(fx.base, "allowed", "viewer-only")
			}
			target := codexSkill(t, root, "target", "target")
			switch scenario {
			case "native-permission":
				piSettingsFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), map[string]any{"defaultProjectTrust": "always", "skills": []string{"!{target,other}"}})
			case "native-participation":
				piSettingsFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), map[string]any{"defaultProjectTrust": "ask"})
				piSettingsFile(t, filepath.Join(fx.project, ".pi/settings.json"), map[string]any{"skills": []string{target}})
			case "native-ignore":
				writeFile(t, filepath.Join(root, ".ignore"), []byte("**\n"), 0600)
			case "prospective-effect":
				codexSkill(t, filepath.Join(fx.project, ".pi/skills"), "winner", "target")
			}
			flag := "--root"
			if scenario == "project-root" {
				flag = "--project-root"
			}
			list := fx.run(t, "list", "--agent", "pi", "--project", fx.project, flag, root, "--json")
			var inventory collection.View
			decodeJSON(t, list.stdout, &inventory)
			var selected collection.Source
			for _, source := range inventory.Sources {
				if source.Directory == filepath.Dir(target) {
					selected = source
				}
			}
			if selected.ID == "" {
				t.Fatal("target absent")
			}
			inspection, unknownPermission, unknownParticipation := false, false, false
			for _, ref := range selected.Availability.References {
				if ref.Participation == collection.ParticipationInspection {
					inspection = ref.Permission.State == collection.PermissionUnknown
				} else {
					unknownPermission = unknownPermission || ref.Permission.State == collection.PermissionUnknown
					unknownParticipation = unknownParticipation || ref.Participation == collection.ParticipationUnknown
				}
			}
			if !inspection || ((scenario == "native-permission" || scenario == "native-ignore") && !unknownPermission) || (scenario == "native-participation" && !unknownParticipation) {
				t.Fatalf("fixture lacks required inspection/native uncertainty: %+v", selected.Availability)
			}
			before := treeManifest(t, fx.base)
			for _, operation := range []struct{ choice, scope string }{{"enable", "global"}, {"disable", "global"}, {"disable", "project"}, {"inherit", "project"}} {
				result := changeCommand(t, fx, "", operation.choice, "--agent", "pi", "--scope", operation.scope, "--project", fx.project, flag, root, "--json", string(selected.ID))
				var preview collection.ChangeView
				decodeJSON(t, result.stdout, &preview)
				want := scenario == "custom-root" || scenario == "project-root"
				if scenario == "prospective-effect" {
					want = operation.choice != "enable" || operation.scope != "global"
				}
				if (preview.State == "ready") != want {
					t.Fatalf("%s/%s preview = %+v, want ready=%t", operation.scope, operation.choice, preview, want)
				}
				code := 1
				if want {
					code = 0
				}
				assertExit(t, result, code)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
