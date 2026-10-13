package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29ClaudeInvocationAliasesAndInvalidValues(t *testing.T) {
	fx, _ := codexFixture(t)
	root := filepath.Join(fx.home, ".claude", "skills")
	aliases := []struct {
		scalar string
		truth  bool
	}{{"true", true}, {"TRUE", true}, {"yes", true}, {"YeS", true}, {"on", true}, {"ON", true}, {"1", true}, {"\"1\"", true}, {"false", false}, {"FALSE", false}, {"no", false}, {"No", false}, {"off", false}, {"OFF", false}, {"0", false}, {"\"0\"", false}}
	type expected struct{ path, state, invocation, scalar string }
	expectations := []expected{}
	for index, alias := range aliases {
		for _, field := range []string{"user-invocable", "disable-model-invocation", "combined"} {
			extra := field + ": " + alias.scalar + "\n"
			state, invocation := "available", "automatic-and-user"
			switch field {
			case "user-invocable":
				if !alias.truth {
					invocation = "model-only"
				}
			case "disable-model-invocation":
				if alias.truth {
					state, invocation = "explicit-only", "explicit-only"
				}
			case "combined":
				extra = "disable-model-invocation: YeS\nuser-invocable: " + alias.scalar + "\n"
				state, invocation = "explicit-only", "explicit-only"
				if !alias.truth {
					state, invocation = "unavailable", "no-invoker"
				}
			}
			name := fmt.Sprintf("%s-%d", field, index)
			path := claudeSkill(t, root, name, name, extra)
			expectations = append(expectations, expected{filepath.Dir(path), state, invocation, alias.scalar})
		}
	}
	for i, value := range []string{"sometimes", "2", "null", "[]", "{}", "1.5"} {
		for _, field := range []string{"user-invocable", "disable-model-invocation"} {
			name := fmt.Sprintf("invalid-%s-%d", field, i)
			path := claudeSkill(t, root, name, name, field+": "+value+"\n")
			expectations = append(expectations, expected{filepath.Dir(path), "unknown", "automatic-and-user", value})
		}
	}
	before := treeManifest(t, fx.base)
	list := claudeListResult(t, fx, 0)
	for _, want := range expectations {
		source := claudeAt(t, list, want.path)
		if source.Availability.State != want.state || source.Availability.Invocation.State != want.invocation {
			t.Fatalf("scalar %s at %s: got %s/%s, want %s/%s", want.scalar, want.path, source.Availability.State, source.Availability.Invocation.State, want.state, want.invocation)
		}
	}
	modelOnly := claudeAt(t, list, expectations[24].path)
	for _, op := range []string{"read", "explain"} {
		result := fx.run(t, op, "--agent", "claude-code", "--project", fx.project, modelOnly.ID)
		assertExit(t, result, 0)
		for _, needle := range []string{"Invocation: model-only", "Availability: available (inferred)", "Session: unknown", "FULL_CLAUDE_INSTRUCTIONS"} {
			if !strings.Contains(string(result.stdout), needle) {
				t.Fatalf("%s lacks %q: %s", op, needle, result.stdout)
			}
		}
	}
	filtered := claudeListResult(t, fx, 0, "--availability", "unknown")
	if len(filtered.Sources) != 12 {
		t.Fatalf("unknown aliases filter: %+v", filtered.Sources)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE29ClaudeFalseAliasesPreserveSettingsAndPermissionExclusions(t *testing.T) {
	for _, value := range []string{"false", "FALSE", "no", "No", "off", "OFF", "0"} {
		t.Run(value, func(t *testing.T) {
			fx, _ := codexFixture(t)
			root := filepath.Join(fx.home, ".claude", "skills")
			visibility := claudeSkill(t, root, "visibility", "visibility", "user-invocable: "+value+"\n")
			permission := claudeSkill(t, root, "permission", "permission", "user-invocable: "+value+"\n")
			claudeSettings(t, filepath.Join(fx.project, ".claude", "settings.local.json"), map[string]any{"skillOverrides": map[string]string{"visibility": "user-invocable-only"}, "permissions": map[string]any{"deny": []string{"Skill(permission)"}}})
			before := treeManifest(t, fx.base)
			list := claudeListResult(t, fx, 0)
			for _, path := range []string{visibility, permission} {
				source := claudeAt(t, list, filepath.Dir(path))
				if source.Availability.State != "unavailable" {
					t.Fatalf("false alias resurrected an invoker: %+v", source.Availability)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func claudeRootPluginFixture(t *testing.T, fx fixture, skills any, declare bool) string {
	t.Helper()
	market := filepath.Join(fx.base, "allowed", "root-market")
	plugin := filepath.Join(market, "demo")
	path := claudeSkill(t, market, "demo", "root-action", "")
	metadata := map[string]any{"name": "demo", "version": "1.0.0"}
	if declare {
		metadata["skills"] = skills
	}
	claudeSettings(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"), metadata)
	claudeSettings(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), map[string]any{"name": "market", "plugins": []map[string]string{{"name": "demo", "source": "./demo"}}})
	registries := filepath.Join(fx.home, ".claude", "plugins")
	claudeSettings(t, filepath.Join(registries, "known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market}})
	claudeSettings(t, filepath.Join(registries, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": plugin, "version": "1.0.0"}}}})
	claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}})
	return path
}

func TestALE29ClaudeRootPluginFallbackBoundaries(t *testing.T) {
	for _, mode := range []string{"absent", "empty-default", "default-child", "explicit-other", "explicit-empty", "disabled", "stale", "unregistered"} {
		t.Run(mode, func(t *testing.T) {
			fx, _ := codexFixture(t)
			declaration := mode == "explicit-other" || mode == "explicit-empty"
			var skills any = "./other"
			if mode == "explicit-empty" {
				skills = []string{}
			}
			path := claudeRootPluginFixture(t, fx, skills, declaration)
			plugin := filepath.Dir(path)
			if mode == "empty-default" {
				if err := os.Mkdir(filepath.Join(plugin, "skills"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "default-child" {
				claudeSkill(t, filepath.Join(plugin, "skills"), "child", "child-action", "")
			}
			if mode == "explicit-other" {
				claudeSkill(t, filepath.Join(plugin, "other"), "child", "child-action", "")
			}
			if mode == "disabled" {
				claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}})
			}
			if mode == "stale" {
				claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "project", "projectPath": filepath.Join(fx.base, "other-project"), "installPath": plugin, "version": "1.0.0"}}}})
			}
			if mode == "unregistered" {
				claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{}})
			}
			before := treeManifest(t, fx.base)
			list := claudeListResult(t, fx, 0)
			if mode == "empty-default" || mode == "default-child" || declaration {
				for _, source := range list.Sources {
					if source.SourcePath == canonicalPath(t, plugin) {
						t.Fatalf("root fallback escaped selection rule: %+v", source)
					}
				}
			} else {
				source := claudeAt(t, list, plugin)
				want := "available"
				if mode == "disabled" || mode == "stale" {
					want = "unavailable"
				}
				if mode == "unregistered" {
					want = "unknown"
				}
				if source.Availability.State != want || source.Ownership.State != "managed" || source.Availability.NativeName != "demo:root-action" {
					t.Fatalf("root plugin outcome: %+v", source)
				}
				for _, op := range []string{"read", "explain"} {
					result := fx.run(t, op, "--agent", "claude-code", "--project", fx.project, source.ID)
					assertExit(t, result, 0)
					for _, needle := range []string{"FULL_CLAUDE_INSTRUCTIONS", "Plugin: demo@market", "Capability: disable | plugin", "unsupported"} {
						if !strings.Contains(string(result.stdout), needle) {
							t.Fatalf("%s lacks %q: %s", op, needle, result.stdout)
						}
					}
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE29ClaudeRootPluginKeepsIndependentCacheAndSharedReferences(t *testing.T) {
	fx, _ := codexFixture(t)
	path := claudeRootPluginFixture(t, fx, nil, false)
	plugin := filepath.Dir(path)
	cache := filepath.Join(fx.home, ".claude/plugins/cache/market/demo/1")
	cached := claudeSkill(t, filepath.Dir(cache), filepath.Base(cache), "root-action", "")
	claudeSettings(t, filepath.Join(cache, ".claude-plugin/plugin.json"), map[string]string{"name": "demo", "version": "1.0.0"})
	claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": cache, "version": "1.0.0"}}}})
	aliasRoot := filepath.Join(fx.base, "allowed", "root-alias")
	if err := os.MkdirAll(aliasRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(plugin, filepath.Join(aliasRoot, "shared")); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	list := claudeListResult(t, fx, 0, "--root", aliasRoot)
	local := claudeAt(t, list, plugin)
	independent := claudeAt(t, list, filepath.Dir(cached))
	if local.ID == independent.ID || local.Availability.State != "available" || independent.Availability.State != "unavailable" {
		t.Fatalf("plugin source/cache identity lost: %+v %+v", local, independent)
	}
	if len(local.Availability.References) != 2 {
		t.Fatalf("shared reference lost: %+v", local)
	}
	for _, fact := range local.Availability.References {
		if fact.Provenance.PluginID != "demo@market" {
			t.Fatalf("provenance lost: %+v", fact)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE29PluginManifestActivationLimits(t *testing.T) {
	for _, tc := range []struct {
		name              string
		fields            map[string]any
		state, diagnostic string
	}{
		{"absent", nil, "available", "local-marketplace-plugin"},
		{"valid", map[string]any{"homepage": "https://example.invalid/docs"}, "available", "local-marketplace-plugin"},
		{"invalid", map[string]any{"homepage": "not-an-absolute-url"}, "unknown", "homepage"},
		{"invalid-hierarchical", map[string]any{"homepage": "https://"}, "unknown", "homepage"},
		{"invalid-type", map[string]any{"homepage": 17}, "unknown", "homepage"},
		{"invalid-null", map[string]any{"homepage": nil}, "unknown", "homepage"},
		{"repository", map[string]any{"repository": "not-an-absolute-url"}, "available", "local-marketplace-plugin"},
		{"author-unmodeled", map[string]any{"author": map[string]string{"name": "Fixture"}}, "unknown", "unmodeled native activation constraints"},
		{"dependencies-unmodeled", map[string]any{"dependencies": []string{"other"}}, "unknown", "unmodeled native activation constraints"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			path := claudeRootPluginFixture(t, fx, nil, false)
			plugin := filepath.Dir(path)
			fields := map[string]any{"name": "demo", "version": "1.0.0"}
			for key, value := range tc.fields {
				fields[key] = value
			}
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), fields)
			before := treeManifest(t, fx.base)
			source := claudeAt(t, claudeListResult(t, fx, 0), plugin)
			if source.Availability.State != tc.state || source.Ownership.State != "managed" {
				t.Fatalf("activation/provenance: %+v", source)
			}
			if tc.state == "unknown" {
				if source.Availability.Evidence.Rule != "plugin-manifest-unverified" {
					t.Fatalf("manifest diagnostic lost from main explanation: %+v", source.Availability)
				}
				for _, ref := range source.Availability.References {
					if ref.Participation == "eligible" {
						t.Fatal("unvalidated manifest granted eligibility")
					}
				}
			}
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
			assertExit(t, detail, 0)
			for _, needle := range []string{tc.diagnostic, "Plugin: demo@market", "Capability: disable | plugin", "unsupported"} {
				if !strings.Contains(string(detail.stdout), needle) {
					t.Fatalf("explain lacks %q: %s", needle, detail.stdout)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
