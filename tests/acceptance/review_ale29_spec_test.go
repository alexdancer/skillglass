package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestReviewALE29RootPluginSkillIsInventoried(t *testing.T) {
	fx, _ := codexFixture(t)
	market := filepath.Join(fx.base, "allowed", "market")
	plugin := filepath.Join(market, "demo")
	cached := filepath.Join(fx.home, ".claude", "plugins", "cache", "market", "demo", "1")
	manifest := claudeSkill(t, market, "demo", "root-action", "")
	for _, directory := range []string{plugin, cached} {
		claudeSettings(t, filepath.Join(directory, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo", "version": "1.0.0"})
	}
	claudeSettings(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), map[string]any{"name": "market", "owner": map[string]string{"name": "Fixture"}, "plugins": []map[string]string{{"name": "demo", "source": "./demo"}}})
	plugins := filepath.Join(fx.home, ".claude", "plugins")
	claudeSettings(t, filepath.Join(plugins, "known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-10-13T00:00:00Z"}})
	claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": cached, "version": "1.0.0"}}}})
	claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}})
	list := claudeListResult(t, fx, 0)
	source := claudeAt(t, list, filepath.Dir(manifest))
	if source.Availability.State != "available" || source.Availability.NativeName != "demo:root-action" || source.Ownership.State != "managed" {
		t.Fatalf("native root plugin skill not represented: %+v", source)
	}
}

func TestReviewALE29NativeBooleanAliasPreservesModelOnly(t *testing.T) {
	fx, _ := codexFixture(t)
	manifest := claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "alternate-bool", "alternate-bool", "user-invocable: no\n")
	list := claudeListResult(t, fx, 0)
	source := claudeAt(t, list, filepath.Dir(manifest))
	if source.Availability.State != "available" || source.Availability.Invocation.State != "model-only" {
		t.Fatalf("native false alias must leave model-only availability: %+v", source.Availability)
	}
}
