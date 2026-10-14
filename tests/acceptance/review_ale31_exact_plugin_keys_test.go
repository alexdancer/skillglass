package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestReviewALE31ExactPluginPolicyKeys(t *testing.T) {
	for _, test := range []struct{ name, policy, state string }{
		{"exact-enable", "enabled: [portable]", "explicit-only"},
		{"quoted-whitespace-enable", "enabled: [' portable ']", "unavailable"},
		{"exact-disable", "enabled: [portable]\n  disabled: [portable]", "unavailable"},
		{"quoted-whitespace-disable", "enabled: [portable]\n  disabled: [' portable ']", "explicit-only"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			plugin := filepath.Join(profile, "plugins", "portable")
			codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
			writeFile(t, filepath.Join(plugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  "+test.policy+"\n"), 0600)
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, 0, nil)
			selected := codexNamed(t, list, "explicit")
			explain := hermesRun(t, fx, nil, "explain", "--agent", "hermes", "--project", fx.project, selected.ID)
			assertExit(t, explain, 0)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if selected.Availability.State != test.state {
				t.Fatalf("native exact-key policy: state %q, want literal %q\n%s", selected.Availability.State, test.state, explain.stdout)
			}
		})
	}
}
