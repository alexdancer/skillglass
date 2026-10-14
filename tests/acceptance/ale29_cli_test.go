package acceptance_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func claudeSkill(t *testing.T, root, folder, name, extra string) string {
	t.Helper()
	path := filepath.Join(root, folder, "SKILL.md")
	metadata := ""
	if name != "" {
		metadata = "name: " + name + "\n"
	}
	writeFile(t, path, []byte("---\n"+metadata+"description: Inert "+folder+".\n"+extra+"---\nFULL_CLAUDE_INSTRUCTIONS\nNever execute anything.\n"), 0600)
	return path
}
func claudeListResult(t *testing.T, fx fixture, exit int, extra ...string) codexList {
	t.Helper()
	args := append([]string{"list", "--agent", "claude-code", "--project", fx.project, "--json"}, extra...)
	result := fx.run(t, args...)
	assertExit(t, result, exit)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	return list
}
func claudeAt(t *testing.T, list codexList, directory string) codexSource {
	t.Helper()
	directory = canonicalPath(t, directory)
	for _, source := range list.Sources {
		if source.SourcePath == directory {
			return source
		}
	}
	t.Fatalf("source absent at %s: %+v", directory, list.Sources)
	return codexSource{}
}
func claudeSettings(t *testing.T, path string, data any) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, raw, 0600)
}
func TestALE29ClaudeDiscoveryPrecedenceSharingAndReadParity(t *testing.T) {
	fx, repo := codexFixture(t)
	personal := filepath.Join(fx.home, ".claude", "skills")
	project := filepath.Join(repo, ".claude", "skills")
	nested := filepath.Join(fx.project, ".claude", "skills")
	user := claudeSkill(t, personal, "same", "collision", "")
	root := claudeSkill(t, project, "same", "collision", "")
	leaf := claudeSkill(t, nested, "same", "collision", "")
	ancestor := claudeSkill(t, project, "nested", "nested-collision", "")
	nearest := claudeSkill(t, nested, "nested", "nested-collision", "")
	claudeSkill(t, filepath.Join(filepath.Dir(repo), ".claude", "skills"), "outside", "outside-repository", "")
	claudeSkill(t, filepath.Join(repo, "below", ".claude", "skills"), "below", "below-startup", "")
	claudeSkill(t, personal, "group/deep", "recursive-canary", "")
	unnamed := claudeSkill(t, personal, "folder-name", "", "")
	shared := claudeSkill(t, filepath.Join(fx.base, "allowed", "shared"), "bundle", "shared", "")
	for _, alias := range []string{filepath.Join(personal, "alias"), filepath.Join(project, "alias")} {
		if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
			t.Fatal(err)
		}
	}
	custom := claudeSkill(t, fx.cleanRoot, "inspection", "inspection", "")
	additional := filepath.Join(fx.base, "allowed", "extra")
	claudeSkill(t, filepath.Join(additional, ".claude", "skills"), "access-only", "not-a-configured-root", "")
	claudeSettings(t, filepath.Join(fx.project, ".claude", "settings.json"), map[string]any{"permissions": map[string]any{"additionalDirectories": []string{additional}}})
	before := treeManifest(t, fx.base)
	list := claudeListResult(t, fx, 0, "--root", fx.cleanRoot)
	if list.Scope.Agent.Application != "claude-code" {
		t.Fatalf("wrong application: %+v", list.Scope.Agent)
	}
	global := claudeAt(t, list, filepath.Dir(user))
	if global.Availability.State != "available" || global.Availability.Precedence.Winner != global.ID {
		t.Fatalf("personal winner: %+v", global)
	}
	for _, path := range []string{root, leaf} {
		source := claudeAt(t, list, filepath.Dir(path))
		if source.Availability.State != "shadowed" || source.Availability.Precedence.Winner != global.ID || len(source.Availability.Precedence.RelatedSources) != 2 {
			t.Fatalf("shadowed: %+v", source)
		}
	}
	near := claudeAt(t, list, filepath.Dir(nearest))
	far := claudeAt(t, list, filepath.Dir(ancestor))
	if near.Availability.State != "available" || far.Availability.State != "shadowed" || far.Availability.Precedence.Winner != near.ID {
		t.Fatalf("nearest precedence: %+v %+v", near, far)
	}
	for _, source := range list.Sources {
		if source.Name == "outside-repository" || source.Name == "below-startup" || source.Name == "recursive-canary" || source.Name == "not-a-configured-root" {
			t.Fatalf("outside native startup scope: %+v", source)
		}
	}
	fallback := claudeAt(t, list, filepath.Dir(unnamed))
	if fallback.Name != "folder-name" || fallback.Availability.State != "available" {
		t.Fatalf("optional name: %+v", fallback)
	}
	common := claudeAt(t, list, filepath.Dir(shared))
	if len(common.Availability.References) != 2 || common.Availability.State != "available" {
		t.Fatalf("sharing: %+v", common)
	}
	inspect := claudeAt(t, list, filepath.Dir(custom))
	if inspect.Availability.State != "unknown" || inspect.Availability.References[0].Participation != "inspection-only" {
		t.Fatalf("custom inspection: %+v", inspect)
	}
	for _, operation := range []string{"read", "explain"} {
		result := fx.run(t, operation, "--agent", "claude-code", "--project", fx.project, "--root", fx.cleanRoot, global.ID)
		assertExit(t, result, 0)
		for _, needle := range []string{"Agent: claude-code", "FULL_CLAUDE_INSTRUCTIONS", "Winning source: " + global.ID, "startup-name-precedence", "Config layer:", "Capability:", "Session: unknown", "Reload: unknown"} {
			if !strings.Contains(string(result.stdout), needle) {
				t.Fatalf("%s lacks %q:\n%s", operation, needle, result.stdout)
			}
		}
	}
	filtered := claudeListResult(t, fx, 0, "--availability", "shadowed")
	if len(filtered.Sources) != 3 {
		t.Fatalf("shadowed filter: %+v", filtered.Sources)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeEvidence(t, "ale29/identity-list.json", fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json").stdout)
}
func TestALE29ClaudeNativeInvocationSettingsAndPermissionStates(t *testing.T) {
	fx, repo := codexFixture(t)
	personal := filepath.Join(fx.home, ".claude", "skills")
	paths := map[string]string{}
	for name, extra := range map[string]string{"ordinary": "", "explicit": "disable-model-invocation: true\n", "model-only": "user-invocable: false\n", "neither": "disable-model-invocation: true\nuser-invocable: false\n", "denied": "", "ask": "", "prefix": "", "name-only": ""} {
		paths[name] = claudeSkill(t, personal, name, name, extra)
	}
	user := filepath.Join(fx.home, ".claude", "settings.json")
	project := filepath.Join(fx.project, ".claude", "settings.json")
	local := filepath.Join(fx.project, ".claude", "settings.local.json")
	claudeSettings(t, user, map[string]any{"skillOverrides": map[string]string{"ordinary": "off", "name-only": "name-only"}, "permissions": map[string]any{"deny": []string{"Skill(denied)", "Skill(prefix *)"}, "ask": []string{"Skill(ask)"}}})
	claudeSettings(t, project, map[string]any{"skillOverrides": map[string]string{"ordinary": "on"}, "permissions": map[string]any{"allow": []string{"Skill(denied)", "Skill(ask)"}}})
	// Ancestor settings do not join the selected working-directory settings layers.
	claudeSettings(t, filepath.Join(repo, ".claude", "settings.local.json"), map[string]any{"skillOverrides": map[string]string{"ordinary": "off"}})
	assertStates := func(want map[string]string) codexList {
		t.Helper()
		list := claudeListResult(t, fx, 0)
		for name, state := range want {
			source := claudeAt(t, list, filepath.Dir(paths[name]))
			if source.Availability.State != state {
				t.Fatalf("%s: got %+v, want %s", name, source.Availability, state)
			}
		}
		return list
	}
	assertStates(map[string]string{"ordinary": "available", "explicit": "explicit-only", "model-only": "available", "neither": "unavailable", "denied": "explicit-only", "ask": "ask", "prefix": "explicit-only", "name-only": "available"})
	claudeSettings(t, local, map[string]any{"skillOverrides": map[string]string{"ordinary": "user-invocable-only", "model-only": "user-invocable-only", "explicit": "name-only"}})
	list := assertStates(map[string]string{"ordinary": "explicit-only", "model-only": "unavailable", "explicit": "explicit-only"})
	source := claudeAt(t, list, filepath.Dir(paths["ordinary"]))
	detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
	assertExit(t, detail, 0)
	if !strings.Contains(string(detail.stdout), "skillOverrides.ordinary=user-invocable-only") || !strings.Contains(string(detail.stdout), local) {
		t.Fatalf("missing controlling setting: %s", detail.stdout)
	}
	claudeSettings(t, local, map[string]any{"skillOverrides": map[string]string{"ordinary": "off"}})
	assertStates(map[string]string{"ordinary": "disabled"})
	writeFile(t, local, []byte(`{"skillOverrides":{"ordinary":123}}`), 0600)
	broken := claudeListResult(t, fx, 1)
	if claudeAt(t, broken, filepath.Dir(paths["ordinary"])).Availability.State != "unknown" {
		t.Fatal("invalid settings reported an effective state")
	}
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	assertStates(map[string]string{"ordinary": "available"})
	before := treeManifest(t, fx.base)
	assertStates(map[string]string{"explicit": "explicit-only"})
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE29ClaudePluginProvenanceInactiveStaleAndUnverifiedActivation(t *testing.T) {
	fx, _ := codexFixture(t)
	base := filepath.Join(fx.home, ".claude", "plugins")
	paths := map[string]string{}
	for _, version := range []string{"current", "stale", "orphan"} {
		plugin := filepath.Join(base, "cache", "market", "demo", version)
		claudeSettings(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo", "version": version})
		paths[version] = claudeSkill(t, filepath.Join(plugin, "skills"), "action", "action", "")
	}
	claudeSettings(t, filepath.Join(base, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": filepath.Dir(filepath.Dir(filepath.Dir(paths["current"]))), "version": "current"}}}})
	settings := filepath.Join(fx.home, ".claude", "settings.json")
	claudeSettings(t, settings, map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}, "skillOverrides": map[string]string{"demo:action": "off"}})
	list := claudeListResult(t, fx, 0)
	current := claudeAt(t, list, filepath.Dir(paths["current"]))
	if current.Availability.State != "unknown" || current.Availability.NativeName != "demo:action" || current.Ownership.State != "managed" {
		t.Fatalf("unverified activation: %+v", current)
	}
	for _, version := range []string{"stale", "orphan"} {
		if claudeAt(t, list, filepath.Dir(paths[version])).Availability.State != "unavailable" {
			t.Fatalf("inactive version %s included", version)
		}
	}
	for _, fact := range current.Availability.References {
		if fact.Provenance.Kind != "plugin" || fact.Provenance.PluginID != "demo@market" {
			t.Fatalf("plugin provenance: %+v", fact)
		}
	}
	detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, current.ID)
	assertExit(t, detail, 0)
	for _, needle := range []string{"plugin-activation-unverified", "Plugin: demo@market", "whole plugins", "Capability:", "unsupported"} {
		if !strings.Contains(string(detail.stdout), needle) {
			t.Fatalf("plugin detail lacks %q:\n%s", needle, detail.stdout)
		}
	}
	claudeSettings(t, settings, map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}})
	before := treeManifest(t, fx.base)
	list = claudeListResult(t, fx, 0)
	for _, path := range paths {
		if claudeAt(t, list, filepath.Dir(path)).Availability.State != "unavailable" {
			t.Fatal("disabled plugin included")
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeEvidence(t, "ale29/plugin-explain.txt", detail.stdout)
}
func TestALE29ClaudeMissingProjectOfflineAndReadOnly(t *testing.T) {
	fx, _ := codexFixture(t)
	manifest := claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "offline", "offline", "")
	writeFile(t, filepath.Join(filepath.Dir(manifest), "sentinel.sh"), []byte("#!/bin/sh\nprintf executed > SHOULD_NEVER_EXIST\n"), 0700)
	before := treeManifest(t, fx.base)
	profile := filepath.Join(fx.base, "allowed", "claude-offline.sb")
	product := canonicalPath(t, skillglassBinary)
	writeFile(t, profile, []byte(runtimeProfile(product, canonicalPath(t, filepath.Join(fx.base, "allowed")))), 0600)
	before = treeManifest(t, fx.base)
	result := runSandboxCommand(t, fx, profile, product, "list", "--agent", "claude-code", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	source := claudeAt(t, list, filepath.Dir(manifest))
	if source.Availability.State != "available" {
		t.Fatalf("offline: %+v", source)
	}
	result = runSandboxCommand(t, fx, profile, product, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
	assertExit(t, result, 0)
	writeEvidence(t, "ale29/offline-explain.txt", result.stdout)
	writeEvidence(t, "ale29/offline.sb", []byte(runtimeProfile(product, canonicalPath(t, filepath.Join(fx.base, "allowed")))))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	missing := fx.run(t, "list", "--agent", "claude-code", "--project", filepath.Join(fx.project, "does-not-exist"), "--json")
	assertExit(t, missing, 1)
	decodeJSON(t, missing.stdout, &list)
	if len(list.Problems) == 0 || list.Scope.Completeness != "incomplete" {
		t.Fatalf("missing project silently accepted: %s", missing.stdout)
	}
	duplicate := claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "other-folder", "offline", "")
	_ = duplicate
	ambiguous := claudeListResult(t, fx, 0)
	if claudeAt(t, ambiguous, filepath.Dir(manifest)).Availability.State != "unknown" {
		t.Fatal("equal priority collision pretended a winner")
	}
}

func TestALE29ClaudeConfiguredHomeReservedNamesAndUnknownManagedInputs(t *testing.T) {
	fx, _ := codexFixture(t)
	claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "old", "old-root-canary", "")
	configured := filepath.Join(fx.base, "allowed", "configured-claude")
	manifest := claudeSkill(t, filepath.Join(configured, "skills"), "configured", "configured", "")
	claudeSkill(t, filepath.Join(configured, "skills"), "anthropic-skills-guide", "anthropic-skills-guide", "")
	before := treeManifest(t, fx.base)
	args := []string{"list", "--agent", "claude-code", "--project", fx.project, "--json"}
	command := fx.command(t, context.Background(), args...)
	command.Env = append(fx.environment(), "CLAUDE_CONFIG_DIR="+configured)
	command.Dir = fx.project
	started := time.Now()
	launch := recordLaunch(t, command)
	raw, err := command.Output()
	result := commandResult{code: launchExitCode(err), stdout: raw, duration: time.Since(started), launch: launch}
	if exit, ok := err.(*exec.ExitError); ok {
		result.code, result.stderr = exit.ExitCode(), exit.Stderr
	}
	recordEvidenceCommand(t, "configured-home-contained", result)
	if err != nil {
		t.Fatalf("configured scope: %s %v", raw, err)
	}
	var list codexList
	decodeJSON(t, raw, &list)
	if claudeAt(t, list, filepath.Dir(manifest)).Availability.State != "available" {
		t.Fatal("configured root not native")
	}
	foundGuide := false
	for _, source := range list.Sources {
		if source.Name == "old-root-canary" {
			t.Fatal("fell back to ordinary Claude home")
		}
		if source.Name == "anthropic-skills-guide" {
			foundGuide = source.Availability.State == "available"
		}
	}
	if !foundGuide {
		t.Fatal("nonreserved name prefix was excluded")
	}
	writeEvidence(t, "ale29/configured-root.json", raw)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// Unknown schemas cannot establish an inactive registration.
	plugin := filepath.Join(fx.home, ".claude", "plugins", "cache", "market", "demo", "current")
	claudeSettings(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo", "version": "1"})
	native := claudeSkill(t, filepath.Join(plugin, "skills"), "action", "action", "")
	claudeSettings(t, filepath.Join(fx.home, ".claude", "plugins", "installed_plugins.json"), map[string]any{"version": 900, "plugins": map[string]any{"demo@market": []any{}}})
	list = claudeListResult(t, fx, 1)
	source := claudeAt(t, list, filepath.Dir(native))
	if source.Availability.State != "unknown" || source.Availability.References[0].Participation != "unknown" {
		t.Fatalf("unknown registry treated as inactive: %+v", source)
	}
}

func TestALE29ClaudeLocalMarketplaceSourceWinsOverIndependentCache(t *testing.T) {
	fx, _ := codexFixture(t)
	market := filepath.Join(fx.base, "allowed", "market")
	localPlugin := filepath.Join(market, "demo")
	cachedPlugin := filepath.Join(fx.home, ".claude", "plugins", "cache", "market", "demo", "1")
	source := claudeSkill(t, filepath.Join(localPlugin, "skills"), "action", "action", "")
	cached := claudeSkill(t, filepath.Join(cachedPlugin, "skills"), "action", "action", "")
	for _, directory := range []string{localPlugin, cachedPlugin} {
		claudeSettings(t, filepath.Join(directory, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo", "version": "1"})
	}
	claudeSettings(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), map[string]any{"name": "market", "plugins": []map[string]string{{"name": "demo", "source": "./demo"}}})
	plugins := filepath.Join(fx.home, ".claude", "plugins")
	claudeSettings(t, filepath.Join(plugins, "known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-10-13T00:00:00Z"}})
	claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": cachedPlugin, "version": "1"}}}})
	user := filepath.Join(fx.home, ".claude", "settings.json")
	project := filepath.Join(fx.project, ".claude", "settings.json")
	local := filepath.Join(fx.project, ".claude", "settings.local.json")
	claudeSettings(t, user, map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}, "skillOverrides": map[string]string{"demo:action": "off"}})
	before := treeManifest(t, fx.base)
	list := claudeListResult(t, fx, 0)
	active := claudeAt(t, list, filepath.Dir(source))
	cache := claudeAt(t, list, filepath.Dir(cached))
	if active.ID == cache.ID || active.Availability.State != "available" || cache.Availability.State != "unavailable" {
		t.Fatalf("native local source versus cache: %+v %+v", active, cache)
	}
	if active.Availability.NativeName != "demo:action" || active.Availability.References[0].Provenance.PluginID != "demo@market" {
		t.Fatalf("namespace: %+v", active)
	}
	detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, active.ID)
	assertExit(t, detail, 0)
	for _, needle := range []string{"local-marketplace-plugin", localPlugin, "Plugin: demo@market", "plugin | unsupported"} {
		if !strings.Contains(string(detail.stdout), needle) {
			t.Fatalf("local plugin detail missing %q: %s", needle, detail.stdout)
		}
	}
	writeEvidence(t, "ale29/local-plugin-explain.txt", detail.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	claudeSettings(t, user, map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}})
	if claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(source)).Availability.State != "unavailable" {
		t.Fatal("disabled local source loaded")
	}
	claudeSettings(t, project, map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}})
	if claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(source)).Availability.State != "available" {
		t.Fatal("project plugin enable did not override global")
	}
	claudeSettings(t, local, map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}})
	if claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(source)).Availability.State != "unavailable" {
		t.Fatal("local plugin disable did not override project")
	}
}
