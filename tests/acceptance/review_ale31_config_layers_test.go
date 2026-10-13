package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestReviewALE31ProfileDotenvPlatform(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "dotenv-denied", "dotenv-denied")
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  platform_disabled:\n    telegram: [dotenv-denied]\n    discord: []\n"), 0600)
	writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=telegram\n"), 0600)
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, []string{"HERMES_PLATFORM=discord"})
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if got := codexNamed(t, list, "dotenv-denied").Availability.State; got != "disabled" {
		t.Fatalf("profile .env selects telegram: state = %q, want literal disabled", got)
	}
}
func TestReviewALE31ManagedPluginDeny(t *testing.T) {
	fx, profile := hermesFixture(t)
	plugin := filepath.Join(profile, "plugins", "portable")
	codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
	writeFile(t, filepath.Join(plugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
	managed := filepath.Join(fx.base, "allowed", "managed")
	writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins:\n  disabled: [portable]\n"), 0600)
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, []string{"HERMES_MANAGED_DIR=" + managed})
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if got := codexNamed(t, list, "explicit").Availability.State; got != "unavailable" {
		t.Fatalf("native managed plugin deny: state = %q, want literal unavailable", got)
	}
}
