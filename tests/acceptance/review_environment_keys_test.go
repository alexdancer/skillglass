package acceptance_test

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestReviewEnvironmentKeyConfidence(t *testing.T) {
	for _, key := range []string{"HERMES_PLATFORM", "HERMES_SESSION_PLATFORM", "SKILL_ROOT"} {
		for _, assignment := range []string{"quoted", "interpolated", "restored", "inherited"} {
			t.Run(key+"/"+assignment, func(t *testing.T) {
				fx, p := hermesFixture(t)
				file := codexSkill(t, filepath.Join(p, "skills"), "local", "local")
				codexSkill(t, filepath.Join(p, "discord"), "stale", "stale")
				codexSkill(t, filepath.Join(p, "telegram"), "fresh", "fresh")
				codexSkill(t, filepath.Join(p, "${"+key+"}"), "literal", "literal")
				text := "'" + key + "'=telegram\n"
				if assignment == "interpolated" {
					text = key + "=${CHOICE}\n"
				}
				if assignment == "restored" {
					text += key + "=telegram\n"
				}
				if assignment == "inherited" {
					text = ""
				}
				writeFile(t, filepath.Join(p, ".env"), []byte(text), 0600)
				writeFile(t, filepath.Join(p, "config.yaml"), []byte(fmt.Sprintf("skills:\n  external_dirs: ['$%s']\n  platform_disabled:\n    discord: [local]\n    telegram: []\n", key)), 0600)
				env := []string{key + "=discord", "CHOICE=telegram", "HERMES_MANAGED_DIR=" + filepath.Join(p, "absent-managed")}
				if key == "HERMES_PLATFORM" {
					env = append(env, "HERMES_SESSION_PLATFORM=discord")
				}
				defer g2Seal(t, fx)()
				list := d2List(t, fx, env)
				wantRoot := ""
				if assignment == "inherited" {
					wantRoot = "stale"
				}
				if assignment == "restored" && key != "SKILL_ROOT" {
					wantRoot = "fresh"
				}
				for _, s := range list.Sources {
					if s.Name != "local" && s.Name != wantRoot {
						t.Errorf("unexpected external inventory %s; want %q", s.Name, wantRoot)
					}
				}
				if wantRoot != "" {
					found := false
					for _, s := range list.Sources {
						found = found || s.Name == wantRoot
					}
					if !found {
						t.Errorf("missing supported root %s", wantRoot)
					}
				}
				wantPermission := "unknown"
				wantAvailability := "unknown"
				if assignment == "restored" && key != "SKILL_ROOT" {
					wantPermission = "allowed"
				}
				if assignment == "inherited" {
					wantPermission = "allowed"
					wantAvailability = "available"
					if key != "SKILL_ROOT" {
						wantPermission = "denied"
						wantAvailability = "disabled"
					}
				}
				if assignment == "restored" && key != "SKILL_ROOT" {
					wantAvailability = "available"
				}
				s := gateSourceAt(t, list, filepath.Dir(file))
				if s.Availability.References[0].Permission.State != wantPermission {
					t.Errorf("permission=%s want %s", s.Availability.References[0].Permission.State, wantPermission)
				}
				g2Inspect(t, fx, env, list, file, wantAvailability)
			})
		}
	}
}

func TestReviewPlatformPriorityConfidence(t *testing.T) {
	for _, tc := range []struct{ name, env, primary, permission string }{
		{"unknown-primary", "'HERMES_PLATFORM'=telegram\n", "discord", "unknown"},
		{"unknown-fallback", "'HERMES_SESSION_PLATFORM'=telegram\n", "", "unknown"},
		{"unused-fallback", "'HERMES_SESSION_PLATFORM'=telegram\n", "telegram", "allowed"},
		{"independent-global-denial", "'HERMES_PLATFORM'=telegram\n", "discord", "denied"},
		{"restored-empty-primary", "'HERMES_PLATFORM'=telegram\nHERMES_PLATFORM=\n", "telegram", "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := codexSkill(t, filepath.Join(p, "skills"), "local", "local")
			excluded := codexSkill(t, filepath.Join(p, "skills"), "excluded", "excluded")
			writeFile(t, excluded, []byte(g2Text("excluded", "platforms: [never-this-os]\n")), 0600)
			writeFile(t, filepath.Join(p, ".env"), []byte(tc.env), 0600)
			config := "skills:\n  platform_disabled:\n    discord: [local]\n"
			if tc.name == "independent-global-denial" {
				config += "  disabled: [local]\n"
			}
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(config), 0600)
			env := []string{"HERMES_PLATFORM=" + tc.primary, "HERMES_SESSION_PLATFORM=discord", "HERMES_MANAGED_DIR=" + filepath.Join(p, "absent")}
			defer g2Seal(t, fx)()
			list := d2List(t, fx, env)
			s := gateSourceAt(t, list, filepath.Dir(file))
			if s.Availability.References[0].Permission.State != tc.permission {
				t.Errorf("permission=%s want %s", s.Availability.References[0].Permission.State, tc.permission)
			}
			g2Inspect(t, fx, env, list, excluded, "unavailable")
		})
	}
}
