package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29ReviewLocalSourceConfidenceWithIndependentCache(t *testing.T) {
	for _, homepage := range []string{"https://例え.invalid", "https:", "https://example.invalid:99999"} {
		t.Run(homepage, func(t *testing.T) {
			fx, _ := codexFixture(t)
			plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
			cache := filepath.Join(fx.home, ".claude/plugins/cache/market/demo/1")
			claudeSkill(t, filepath.Dir(cache), filepath.Base(cache), "root-action", "")
			claudeSettings(t, filepath.Join(cache, ".claude-plugin/plugin.json"), map[string]string{"name": "demo", "version": "1.0.0"})
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]string{"name": "demo", "version": "1.0.0", "homepage": homepage})
			claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": cache, "version": "1.0.0"}}}})
			before := treeManifest(t, fx.base)
			list := claudeListResult(t, fx, 0)
			source := claudeAt(t, list, plugin)
			if source.Availability.State != "unknown" || source.Availability.Evidence.Rule != "plugin-manifest-unverified" {
				t.Fatalf("local source confidence lost: %+v", source.Availability)
			}
			if claudeAt(t, list, cache).Availability.State != "unavailable" {
				t.Fatal("independent cache participates")
			}
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
			assertExit(t, detail, 0)
			if !strings.Contains(string(detail.stdout), "plugin-manifest-unverified") || !strings.Contains(string(detail.stdout), "Availability: unknown") {
				t.Fatalf("diagnostic lost: %s", detail.stdout)
			}
			t.Logf("public explanation:\n%s", detail.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE29ReviewInstallationConfidence(t *testing.T) {
	cases := []struct {
		name, field string
		value       any
		omit        bool
	}{
		{"missing-scope", "scope", nil, true},
		{"null-scope", "scope", nil, false},
		{"unsupported-scope", "scope", "enterprise", false},
		{"numeric-scope", "scope", 7, false},
		{"missing-install", "installPath", nil, true},
		{"null-install", "installPath", nil, false},
		{"relative-install", "installPath", "cache", false},
		{"numeric-install", "installPath", 7, false},
		{"null-version", "version", nil, false},
		{"numeric-version", "version", 7, false},
		{"null-project", "projectPath", nil, false},
		{"relative-project", "projectPath", "project", false},
		{"numeric-project", "projectPath", 7, false},
	}
	for _, scope := range []string{"user", "project", "local"} {
		for _, tc := range cases {
			t.Run(scope+"/"+tc.name, func(t *testing.T) {
				fx, _ := codexFixture(t)
				plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
				cache := filepath.Join(fx.home, ".claude/plugins/cache/market/demo/1")
				claudeSkill(t, filepath.Dir(cache), filepath.Base(cache), "root-action", "")
				claudeSettings(t, filepath.Join(cache, ".claude-plugin/plugin.json"), map[string]string{"name": "demo", "version": "1.0.0"})
				entry := map[string]any{"scope": scope, "installPath": cache, "version": "1.0.0", "projectPath": fx.project}
				if tc.omit {
					delete(entry, tc.field)
				} else {
					entry[tc.field] = tc.value
				}
				claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []any{entry}}})
				before := treeManifest(t, fx.base)
				result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
				var list codexList
				decodeJSON(t, result.stdout, &list)
				for _, path := range []string{plugin, cache} {
					if source := claudeAt(t, list, path); source.Availability.State != "unknown" {
						t.Errorf("incomplete registry asserts state: %+v", source.Availability)
					}
				}
				assertExit(t, result, 1)
				if list.Scope.Completeness != "incomplete" || len(list.Problems) == 0 {
					t.Fatalf("missing registry diagnostic: %s", result.stdout)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
	for _, scope := range []string{"project", "local"} {
		t.Run(scope+"/missing-project", func(t *testing.T) {
			fx, _ := codexFixture(t)
			plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
			claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": scope, "installPath": plugin}}}})
			result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			if source := claudeAt(t, list, plugin); source.Availability.State != "unknown" {
				t.Errorf("missing project asserts inactivity: %+v", source.Availability)
			}
			assertExit(t, result, 1)
		})
	}
}

func TestALE29ReviewUnknownUserInvocationTransitions(t *testing.T) {
	for _, control := range []string{"none", "deny", "user-invocable-only", "off"} {
		t.Run(control, func(t *testing.T) {
			fx, _ := codexFixture(t)
			path := claudeSkill(t, filepath.Join(fx.home, ".claude/skills"), "uncertain", "uncertain", "user-invocable: sometimes\n")
			settings := map[string]any{}
			if control == "deny" {
				settings["permissions"] = map[string]any{"deny": []string{"Skill(uncertain)"}}
			} else if control != "none" {
				settings["skillOverrides"] = map[string]string{"uncertain": control}
			}
			claudeSettings(t, filepath.Join(fx.project, ".claude/settings.local.json"), settings)
			before := treeManifest(t, fx.base)
			source := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(path))
			state, invocation := "unknown", "unknown"
			if control == "off" {
				state, invocation = "disabled", "disabled"
			}
			if source.Availability.State != state || source.Availability.Invocation.State != invocation {
				t.Fatalf("unknown invoker converted to certainty: %+v", source.Availability)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE29ReviewSettingsNullShapes(t *testing.T) {
	for _, key := range []string{"allow", "ask", "deny", "strictPluginOnlyCustomization", "enabledPlugins"} {
		t.Run(key, func(t *testing.T) {
			fx, _ := codexFixture(t)
			path := claudeSkill(t, filepath.Join(fx.home, ".claude/skills"), "ordinary", "ordinary", "")
			settings := map[string]any{}
			switch key {
			case "strictPluginOnlyCustomization":
				settings[key] = nil
			case "enabledPlugins":
				settings[key] = map[string]any{"demo@market": nil}
			default:
				settings["permissions"] = map[string]any{key: []any{"Skill(ordinary)", nil}}
			}
			claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), settings)
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			source := claudeAt(t, list, filepath.Dir(path))
			if source.Availability.State != "unknown" || source.Availability.References[0].Permission.State != "unknown" {
				t.Errorf("null interpreted as supported settings: %+v", source.Availability)
			}
			assertExit(t, result, 1)
			if list.Scope.Completeness != "incomplete" {
				t.Fatal("invalid settings reported complete scope")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE29ReviewSupportedInstallationScopes(t *testing.T) {
	for _, scope := range []string{"user", "project", "local"} {
		for _, mode := range []string{"applicable", "nonapplicable", "no-project", "version-absent", "version-empty"} {
			t.Run(scope+"/"+mode, func(t *testing.T) {
				fx, _ := codexFixture(t)
				plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
				entry := map[string]any{"scope": scope, "installPath": plugin, "version": "1.0.0"}
				if scope != "user" {
					entry["projectPath"] = fx.project
					if mode == "nonapplicable" {
						entry["projectPath"] = filepath.Join(fx.base, "allowed", "other-project")
					}
				}
				if mode == "version-absent" {
					delete(entry, "version")
				} else if mode == "version-empty" {
					entry["version"] = ""
				}
				claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []any{entry}}})
				before := treeManifest(t, fx.base)
				args := []string{"list", "--agent", "claude-code", "--json"}
				if mode != "no-project" {
					args = append(args, "--project", fx.project)
				}
				result := fx.run(t, args...)
				assertExit(t, result, 0)
				var list codexList
				decodeJSON(t, result.stdout, &list)
				want := "available"
				if scope != "user" && (mode == "nonapplicable" || mode == "no-project") {
					want = "unavailable"
				}
				if source := claudeAt(t, list, plugin); source.Availability.State != want {
					t.Fatalf("supported registry changed: %+v, want %s", source.Availability, want)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE29ReviewApplicableInstallationIsOrderIndependent(t *testing.T) {
	for _, versions := range [][]string{{"1.0.0", "2.0.0"}, {"2.0.0", "1.0.0"}} {
		t.Run(strings.Join(versions, "/"), func(t *testing.T) {
			fx, _ := codexFixture(t)
			cache := filepath.Join(fx.home, ".claude/plugins/cache/market/demo/1")
			path := claudeSkill(t, filepath.Join(cache, "skills"), "action", "action", "")
			claudeSettings(t, filepath.Join(cache, ".claude-plugin/plugin.json"), map[string]string{"name": "demo", "version": "1.0.0"})
			entries := []map[string]string{}
			for _, version := range versions {
				entries = append(entries, map[string]string{"scope": "user", "installPath": cache, "version": version})
			}
			claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": entries}})
			before := treeManifest(t, fx.base)
			if source := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(path)); source.Availability.State != "unknown" {
				t.Fatalf("matching record erased by sibling: %+v", source.Availability)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE29ReviewModelRestrictionsResolveUnknownModelInvocation(t *testing.T) {
	for _, user := range []string{"true", "false"} {
		for _, control := range []string{"deny", "user-invocable-only", "off"} {
			t.Run(user+"/"+control, func(t *testing.T) {
				fx, _ := codexFixture(t)
				path := claudeSkill(t, filepath.Join(fx.home, ".claude/skills"), "uncertain", "uncertain", "disable-model-invocation: sometimes\nuser-invocable: "+user+"\n")
				settings := map[string]any{}
				if control == "deny" {
					settings["permissions"] = map[string]any{"deny": []string{"Skill(uncertain)"}}
				} else {
					settings["skillOverrides"] = map[string]string{"uncertain": control}
				}
				claudeSettings(t, filepath.Join(fx.project, ".claude/settings.local.json"), settings)
				before := treeManifest(t, fx.base)
				source := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(path))
				state, invocation := "explicit-only", "explicit-only"
				if user == "false" {
					state, invocation = "unavailable", "no-invoker"
				}
				if control == "off" {
					state, invocation = "disabled", "disabled"
				}
				if source.Availability.State != state || source.Availability.Invocation.State != invocation {
					t.Fatalf("independent restriction failed: %+v", source.Availability)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE29ReviewCanonicalAgentSelection(t *testing.T) {
	fx, _ := codexFixture(t)
	claudeListResult(t, fx, 0)
	result := fx.run(t, "list", "--agent", "claude", "--project", fx.project, "--json")
	if result.code == 0 || !strings.Contains(string(result.stderr), "unknown agent") {
		t.Fatalf("unrequired alias accepted: %+v", result)
	}
}
