package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29ClaudeMarketplaceTimestampConfidence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		timestamp any
		present   bool
		disabled  bool
	}{
		{name: "missing"},
		{name: "valid", timestamp: "2026-10-13T00:00:00Z", present: true},
		{name: "null", present: true},
		{name: "number", timestamp: 17, present: true},
		{name: "empty", timestamp: "", present: true},
		{name: "invalid", timestamp: "not-a-timestamp", present: true},
		{name: "disabled-missing", disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			ordinary := claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "ordinary", "ordinary", "")
			local := filepath.Dir(claudeRootPluginFixture(t, fx, ".", true))
			cache := filepath.Join(fx.home, ".claude", "plugins", "cache", "market", "demo", "1")
			claudeSkill(t, filepath.Dir(cache), filepath.Base(cache), "root-action", "")
			claudeSettings(t, filepath.Join(cache, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo", "version": "1.0.0", "skills": "."})
			for _, directory := range []string{local, cache} {
				claudeSkill(t, filepath.Join(directory, "skills"), "child", "child-action", "")
			}
			plugins := filepath.Join(fx.home, ".claude", "plugins")
			registry := filepath.Join(plugins, "known_marketplaces.json")
			market := filepath.Dir(local)
			record := map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market}
			if tc.present {
				record["lastUpdated"] = tc.timestamp
			}
			claudeSettings(t, registry, map[string]any{"market": record})
			// Installation timestamps do not establish marketplace source selection.
			claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": cache, "version": "1.0.0", "installedAt": "2026-10-13T00:00:00Z", "lastUpdated": "2026-10-13T00:00:00Z"}}}})
			claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": !tc.disabled}, "permissions": map[string]any{"ask": []string{"Skill(ordinary)"}}})
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			valid := tc.name == "valid"
			for _, directory := range []string{local, cache, filepath.Join(local, "skills", "child"), filepath.Join(cache, "skills", "child")} {
				source := claudeAt(t, list, directory)
				want := "unknown"
				if valid {
					want = "available"
					if directory == cache || directory == filepath.Join(cache, "skills", "child") {
						want = "unavailable"
					}
				} else if tc.disabled {
					want = "unavailable"
				}
				if source.Availability.State != want || source.Ownership.State != "managed" {
					t.Fatalf("marketplace timestamp %s: got %s at %s, want %s; CLI output:\n%s", tc.name, source.Availability.State, directory, want, result.stdout)
				}
				if len(source.Availability.References) != 1 || source.Availability.References[0].Permission.State != "allowed" {
					t.Fatalf("marketplace uncertainty changed Skill permission: %+v", source.Availability)
				}
				if !valid && !tc.disabled && (source.Availability.Evidence.Rule != "local-marketplace-unverified" || source.Availability.Evidence.InputPath != registry) {
					t.Fatalf("marketplace activation diagnostic lost: %+v", source.Availability)
				}
			}
			if source := claudeAt(t, list, filepath.Dir(ordinary)); source.Availability.State != "ask" {
				t.Fatalf("marketplace uncertainty changed ordinary skill permission: %+v", source.Availability)
			}
			active := claudeAt(t, list, local)
			cached := claudeAt(t, list, cache)
			if active.ID == cached.ID {
				t.Fatal("independent source and cache identities merged")
			}
			exit, completeness := 1, "incomplete"
			if valid {
				exit, completeness = 0, "complete"
			}
			assertExit(t, result, exit)
			if list.Scope.Completeness != completeness {
				t.Fatalf("coverage: got %s, want %s", list.Scope.Completeness, completeness)
			}
			if !valid {
				detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, active.ID)
				assertExit(t, detail, 1)
				if !strings.Contains(string(detail.stdout), "lastUpdated") || !strings.Contains(string(detail.stdout), registry) {
					t.Fatalf("missing marketplace confidence reason:\n%s", detail.stdout)
				}
				writeEvidence(t, "marketplace-timestamp/"+tc.name+"-explain.txt", detail.stdout)
			}
			writeEvidence(t, "marketplace-timestamp/"+tc.name+"-list.json", result.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
