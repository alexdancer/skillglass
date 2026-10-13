package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestDecisionALE29InvocationControls(t *testing.T) {
	cases := []struct{ label, extra, state, invocation string }{
		{"default", "", "available", "automatic-and-user"},
		{"canonical-false", "user-invocable: false\n", "available", "model-only"},
		{"canonical-true", "user-invocable: true\n", "available", "automatic-and-user"},
		{"canonical-explicit", "disable-model-invocation: true\n", "explicit-only", "explicit-only"},
		{"canonical-neither", "disable-model-invocation: true\nuser-invocable: false\n", "unavailable", "no-invoker"},
		{"alias-no", "user-invocable: no\n", "available", "model-only"},
		{"alias-yes", "user-invocable: yes\n", "available", "automatic-and-user"},
		{"alias-disable-yes", "disable-model-invocation: yes\n", "explicit-only", "explicit-only"},
		{"invalid", "user-invocable: sometimes\n", "unknown", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			fx, _ := codexFixture(t)
			path := claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "action", "action", tc.extra)
			before := treeManifest(t, fx.base)
			source := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(path))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if source.Availability.State != tc.state || source.Availability.Invocation.State != tc.invocation {
				t.Fatalf("got %s/%s, want %s/%s", source.Availability.State, source.Availability.Invocation.State, tc.state, tc.invocation)
			}
		})
	}
}

func TestDecisionALE29RootPluginExplicitSkillsControl(t *testing.T) {
	fx, _ := codexFixture(t)
	market := filepath.Join(fx.base, "allowed", "market")
	plugin := filepath.Join(market, "demo")
	cached := filepath.Join(fx.home, ".claude", "plugins", "cache", "market", "demo", "1")
	path := claudeSkill(t, market, "demo", "root-action", "")
	for _, directory := range []string{plugin, cached} {
		claudeSettings(t, filepath.Join(directory, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo", "version": "1.0.0", "skills": "."})
	}
	claudeSettings(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), map[string]any{"name": "market", "plugins": []map[string]string{{"name": "demo", "source": "./demo"}}})
	plugins := filepath.Join(fx.home, ".claude", "plugins")
	claudeSettings(t, filepath.Join(plugins, "known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-10-13T00:00:00Z"}})
	claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": cached, "version": "1.0.0"}}}})
	claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}})
	before := treeManifest(t, fx.base)
	source := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(path))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if source.Availability.State != "available" || source.Availability.NativeName != "demo:root-action" || source.Ownership.State != "managed" {
		t.Fatalf("explicit plugin root should be recognized: %+v", source)
	}
}
