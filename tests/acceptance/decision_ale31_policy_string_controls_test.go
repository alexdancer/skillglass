package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDecisionALE31PolicyStringControls(t *testing.T) {
	for _, test := range []struct {
		name, user, managed, profileEnv, managedEnv, state string
		exit                                               int
	}{
		{"managed-enable-replaces-exact", "enabled: [portable]", "enabled: [' portable ']", "", "", "unavailable", 0},
		{"managed-disable-replaces-exact", "enabled: [portable]\n  disabled: [portable]", "disabled: [' portable ']", "", "", "explicit-only", 0},
		{"profile-persisted-expanded-whitespace", "enabled: [portable]\n  disabled: ['${HERMES_ENABLE_PROJECT_PLUGINS}']", "enabled: [portable]", "HERMES_ENABLE_PROJECT_PLUGINS=' portable '\n", "", "explicit-only", 0},
		{"managed-persisted-expanded-whitespace", "enabled: [portable]", "disabled: ['${env:HERMES_ENABLE_PROJECT_PLUGINS}']", "HERMES_ENABLE_PROJECT_PLUGINS=portable\n", "HERMES_ENABLE_PROJECT_PLUGINS=' portable '\n", "explicit-only", 0},
		{"managed-persisted-expanded-exact-control", "enabled: [portable]", "disabled: ['${env:HERMES_ENABLE_PROJECT_PLUGINS}']", "HERMES_ENABLE_PROJECT_PLUGINS=' portable '\n", "HERMES_ENABLE_PROJECT_PLUGINS=portable\n", "unavailable", 0},
		{"managed-malformed-list-remains-unknown", "enabled: [portable]", "enabled: [portable, false]", "", "", "unknown", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			plugin := filepath.Join(profile, "plugins", "portable")
			codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
			codexSkill(t, filepath.Join(profile, "skills"), "raw-independent", "raw-independent")
			writeFile(t, filepath.Join(plugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  disabled: [' raw-independent ']\nplugins:\n  "+test.user+"\n"), 0600)
			managed := filepath.Join(fx.base, "allowed", "managed-policy")
			writeFile(t, filepath.Join(managed, "config.yaml"), []byte("skills:\n  disabled: []\nplugins:\n  "+test.managed+"\n"), 0600)
			if test.profileEnv != "" {
				writeFile(t, filepath.Join(profile, ".env"), []byte(test.profileEnv), 0600)
			}
			if test.managedEnv != "" {
				writeFile(t, filepath.Join(managed, ".env"), []byte(test.managedEnv), 0600)
			}
			env := []string{"HERMES_MANAGED_DIR=" + managed}
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, test.exit, env)
			selected := codexNamed(t, list, "explicit")
			explain := hermesRun(t, fx, env, "explain", "--agent", "hermes", "--project", fx.project, selected.ID)
			assertExit(t, explain, test.exit)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if raw := codexNamed(t, list, "raw-independent"); raw.Availability.State != "disabled" {
				t.Errorf("raw skill normalization/profile-only boundary: got %q want disabled", raw.Availability.State)
			}
			if !strings.Contains(string(explain.stdout), filepath.Join(managed, "config.yaml")) {
				t.Error("managed provenance missing")
			}
			if selected.Availability.State != test.state {
				t.Errorf("exact native plugin policy: got %q want literal %q", selected.Availability.State, test.state)
			}
			if !strings.Contains(string(explain.stdout), "Availability: "+test.state+" (") {
				t.Errorf("human explanation does not show literal %q", test.state)
			}
		})
	}
}
