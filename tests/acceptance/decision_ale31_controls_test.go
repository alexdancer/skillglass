package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestDecisionALE31PluginGateControls(t *testing.T) {
	for _, test := range []struct {
		name          string
		env           []string
		config        string
		user, project string
	}{
		{"project-opt-out", nil, "plugins:\n  enabled: [portable]\n", "explicit-only", "unavailable"},
		{"project-opt-in", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "plugins:\n  enabled: [portable]\n", "unavailable", "explicit-only"},
		{"safe-mode", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1", "HERMES_SAFE_MODE=1"}, "plugins:\n  enabled: [portable]\n", "unavailable", "unavailable"},
		{"disabled-wins", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "plugins:\n  enabled: [portable]\n  disabled: [portable]\n", "unavailable", "unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			user := filepath.Join(profile, "plugins", "portable")
			project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
			for _, plugin := range []string{user, project} {
				codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
				writeFile(t, filepath.Join(plugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
				writeFile(t, filepath.Join(plugin, "__init__.py"), []byte("raise RuntimeError('decision fixture must never execute')\n"), 0600)
			}
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte(test.config), 0600)
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, 0, test.env)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if len(list.Sources) != 2 {
				t.Fatalf("sources = %d; want 2", len(list.Sources))
			}
			want := map[string]string{filepath.Join(user, "skills", "explicit"): test.user, filepath.Join(project, "skills", "explicit"): test.project}
			for _, source := range list.Sources {
				path, err := filepath.EvalSymlinks(source.SourcePath)
				if err != nil {
					t.Fatal(err)
				}
				state, found := want[path]
				if !found || source.Availability.State != state {
					t.Fatalf("source %s: state %q, want %q", path, source.Availability.State, state)
				}
				delete(want, path)
			}
			if len(want) != 0 {
				t.Fatalf("missing sources: %v", want)
			}
		})
	}
}
