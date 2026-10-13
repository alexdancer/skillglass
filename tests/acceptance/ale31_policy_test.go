package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestALE31ProfileEnvironmentPolicyControls(t *testing.T) {
	for _, test := range []struct {
		name, envFile string
		env           []string
		state         string
		exit          int
	}{
		{"absent", "", []string{"HERMES_PLATFORM=telegram"}, "disabled", 0},
		{"profile-over-shell", "HERMES_PLATFORM=telegram\n", []string{"HERMES_PLATFORM=discord"}, "disabled", 0},
		{"quoted-export", "export HERMES_PLATFORM=\"telegram\" # inert\n", []string{"HERMES_PLATFORM=discord"}, "disabled", 0},
		{"single-quote-bom", "\ufeffHERMES_PLATFORM='telegram'\r\n", nil, "disabled", 0},
		{"session-fallback", "HERMES_PLATFORM=\nHERMES_SESSION_PLATFORM=telegram\n", []string{"HERMES_PLATFORM=discord"}, "disabled", 0},
		{"platform-wins", "HERMES_PLATFORM=discord\nHERMES_SESSION_PLATFORM=telegram\n", nil, "available", 0},
		{"unsupported-interpolation", "HERMES_PLATFORM=${PRIVATE_POLICY}\n", []string{"PRIVATE_POLICY=telegram"}, "unknown", 1},
		{"irrelevant-multiline", "INERT_CERT=\"inert\nHERMES_PLATFORM=telegram\n\"\n", []string{"HERMES_PLATFORM=discord"}, "unknown", 1},
		{"quoted-policy-key", "'HERMES_PLATFORM'=telegram\n", []string{"HERMES_PLATFORM=discord"}, "unknown", 1},
		{"tab-export", "export\tHERMES_PLATFORM=telegram\n", []string{"HERMES_PLATFORM=discord"}, "disabled", 0},
		{"unsupported-multiline", "HERMES_PLATFORM=\"telegram\n\"\n", nil, "unknown", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			codexSkill(t, filepath.Join(profile, "skills"), "platform-denied", "platform-denied")
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  platform_disabled:\n    telegram: [platform-denied]\n"), 0600)
			if test.envFile != "" {
				writeFile(t, filepath.Join(profile, ".env"), []byte(test.envFile+"OPENAI_API_KEY=INERT_CREDENTIAL_MUST_NOT_APPEAR\n"), 0600)
			}
			before := treeManifest(t, fx.base)
			result := hermesRun(t, fx, test.env, "list", "--agent", "hermes", "--json")
			assertExit(t, result, test.exit)
			if strings.Contains(string(result.stdout)+string(result.stderr), "INERT_CREDENTIAL_MUST_NOT_APPEAR") {
				t.Fatal("unrelated credential value exposed")
			}
			var list codexList
			decodeJSON(t, result.stdout, &list)
			selected := codexNamed(t, list, "platform-denied")
			if selected.Availability.State != test.state {
				t.Fatalf("state %q want %q", selected.Availability.State, test.state)
			}
			explanation := hermesRun(t, fx, test.env, "explain", "--agent", "hermes", selected.ID)
			assertExit(t, explanation, test.exit)
			if !strings.Contains(string(explanation.stdout), filepath.Join(profile, ".env")) {
				t.Fatal("environment provenance missing")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE31ManagedPluginPolicyControls(t *testing.T) {
	for _, test := range []struct {
		name, user, managed, managedEnv string
		env                             []string
		state, raw                      string
		exit                            int
	}{
		{"missing-explicit", "enabled: [portable]", "", "", nil, "explicit-only", "available", 0},
		{"deny-over-enable", "enabled: [portable]", "disabled: [portable]", "", nil, "unavailable", "available", 0},
		{"list-replacement", "enabled: [portable]\n  disabled: [portable]", "disabled: []", "", nil, "explicit-only", "available", 0},
		{"enable-replacement", "enabled: [portable]", "enabled: []", "", nil, "unavailable", "available", 0},
		{"supported-env-ref", "enabled: [portable]", "disabled: ['${env:HERMES_SESSION_PLATFORM}']", "", []string{"HERMES_SESSION_PLATFORM=portable"}, "unavailable", "available", 0},
		{"managed-environment", "enabled: [portable]", "disabled: ['${HERMES_SESSION_PLATFORM}']", "HERMES_SESSION_PLATFORM=portable\n", []string{"HERMES_SESSION_PLATFORM=discord"}, "unavailable", "available", 0},
		{"unsupported-ref", "enabled: [portable]", "disabled: ['${vault:private}']", "", nil, "unknown", "available", 1},
		{"unreadable-file-kind", "enabled: [portable]", "!directory", "", nil, "unknown", "available", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			plugin := filepath.Join(profile, "plugins", "portable")
			codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
			codexSkill(t, filepath.Join(profile, "skills"), "raw-independent", "raw-independent")
			writeFile(t, filepath.Join(plugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  "+test.user+"\n"), 0600)
			managed := filepath.Join(fx.base, "allowed", "managed-policy")
			if test.managed == "!directory" {
				writeFile(t, filepath.Join(managed, "config.yaml", "not-a-file"), []byte("inert"), 0600)
			} else if test.managed != "" {
				writeFile(t, filepath.Join(managed, "config.yaml"), []byte("skills:\n  disabled: [raw-independent]\nplugins:\n  "+test.managed+"\n"), 0600)
			}
			if test.managedEnv != "" {
				writeFile(t, filepath.Join(managed, ".env"), []byte(test.managedEnv), 0600)
			}
			env := append([]string{"HERMES_MANAGED_DIR=" + managed}, test.env...)
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, test.exit, env)
			selected := codexNamed(t, list, "explicit")
			if selected.Availability.State != test.state {
				t.Fatalf("plugin state %q want %q", selected.Availability.State, test.state)
			}
			if raw := codexNamed(t, list, "raw-independent"); raw.Availability.State != test.raw {
				t.Fatalf("managed config altered raw skills state: %q", raw.Availability.State)
			}
			explain := hermesRun(t, fx, env, "explain", "--agent", "hermes", selected.ID)
			assertExit(t, explain, test.exit)
			if test.managed != "" && !strings.Contains(string(explain.stdout), filepath.Join(managed, "config.yaml")) {
				t.Fatal("managed layer provenance missing")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE31ExternalRootUsesResolvedBehavioralPolicy(t *testing.T) {
	fx, profile := hermesFixture(t)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  external_dirs: ['$HERMES_PLATFORM']\n"), 0600)
	writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=telegram\n"), 0600)
	codexSkill(t, filepath.Join(profile, "telegram"), "resolved", "profile-external")
	codexSkill(t, filepath.Join(profile, "discord"), "stale", "shell-external")
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, []string{"HERMES_PLATFORM=discord"})
	if len(list.Sources) != 1 || list.Sources[0].Name != "profile-external" {
		t.Fatalf("external root uses stale shell policy: %+v", list.Sources)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
