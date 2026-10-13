package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func codeafFixture(t *testing.T) fixture {
	t.Helper()
	fx := newFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".codex/skills"), "home-canary", "home-native")
	return fx
}
func codeafListResult(t *testing.T, fx fixture, exit int, extra ...string) codexList {
	t.Helper()
	args := append([]string{"list", "--agent", "codeaf", "--project", fx.project, "--json"}, extra...)
	result := fx.run(t, args...)
	assertExit(t, result, exit)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	return list
}
func codeafPath(t *testing.T, list codexList, path string) codexSource {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range list.Sources {
		if source.SourcePath == resolved {
			return source
		}
	}
	t.Fatalf("source %s absent: %+v", resolved, list.Sources)
	return codexSource{}
}
func codeafPluginFixture(t *testing.T, fx fixture) (string, string) {
	t.Helper()
	plugin := filepath.Join(fx.base, "plugin")
	manifest := codexSkill(t, filepath.Join(plugin, "skills"), "plugin-skill", "plugin-skill")
	registry := map[string]any{"plugins": map[string]any{"example@local": []any{map[string]any{"scope": "user", "installPath": plugin, "version": "1.2"}}}}
	raw, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), raw, 0600)
	settings := filepath.Join(fx.home, ".claude/settings.json")
	writeFile(t, settings, []byte(`{"enabledPlugins":{"example@local":true}}`), 0600)
	return manifest, settings
}
func TestALE39CodeAFPrecedenceSharedRootsAndReadOnly(t *testing.T) {
	fx := codeafFixture(t)
	winner := codexSkill(t, filepath.Join(fx.project, ".codeaf/skills"), "first", "same-name")
	second := codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "second", "same-name")
	third := codexSkill(t, filepath.Join(fx.home, ".codeaf/skills"), "third", "same-name")
	// Ancestors and deeply nested containers are not CodeAF project discovery.
	codexSkill(t, filepath.Join(filepath.Dir(fx.project), ".codeaf/skills"), "ancestor", "not-an-ancestor")
	codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "deep/container", "not-recursive")
	shared := filepath.Dir(codexSkill(t, filepath.Join(fx.base, "shared"), "real", "shared-skill"))
	for _, root := range []string{filepath.Join(fx.home, ".agents/skills"), filepath.Join(fx.home, ".claude/skills")} {
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shared, filepath.Join(root, "alias")); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte(selector(filepath.Join(shared, "SKILL.md"), false)), 0600)
	before := treeManifest(t, fx.base)
	list := codeafListResult(t, fx, 0, "--root", fx.cleanRoot)
	win := codeafPath(t, list, filepath.Dir(winner))
	if list.Scope.Agent.Application != "codeaf" || !strings.Contains(list.Scope.Agent.ValidatedVersion, "installed +dirty") {
		t.Fatalf("scope %+v", list.Scope.Agent)
	}
	if win.Availability.State != "available" || win.Availability.Precedence.State != "winning" || win.Availability.Precedence.Winner != win.ID {
		t.Fatalf("winner %+v", win)
	}
	for _, path := range []string{second, third} {
		source := codeafPath(t, list, filepath.Dir(path))
		if source.Availability.State != "shadowed" || source.Availability.Precedence.Winner != win.ID || source.ID == win.ID {
			t.Fatalf("shadow %+v", source)
		}
	}
	for _, source := range list.Sources {
		if source.Name == "not-recursive" || source.Name == "not-an-ancestor" {
			t.Fatalf("unexpected discovery %+v", source)
		}
	}
	same := codeafPath(t, list, shared)
	if same.Availability.State != "available" || len(same.Availability.References) != 2 {
		t.Fatalf("shared %+v", same)
	}
	if same.Availability.References[0].Permission.State != "allowed" {
		t.Fatalf("inherited Codex setting %+v", same)
	}
	for _, source := range list.Sources {
		if source.Name == "normal-skill" && source.Availability.State != "unknown" {
			t.Fatalf("custom root fabricated native availability %+v", source)
		}
	}
	filtered := codeafListResult(t, fx, 0, "--availability", "shadowed")
	if len(filtered.Sources) != 2 {
		t.Fatalf("shadowed count %d", len(filtered.Sources))
	}
	for _, source := range filtered.Sources {
		original := codeafPath(t, list, source.SourcePath)
		if !reflect.DeepEqual(source.Availability, original.Availability) {
			t.Fatal("filter recalculated availability")
		}
	}
	detail := fx.run(t, "explain", "--agent", "codeaf", "--project", fx.project, win.ID)
	assertExit(t, detail, 0)
	for _, needle := range []string{"Winning source: " + win.ID, "persistent CodeAF-only per-skill control is unsupported", "conversation detachment", "another agent's setting", "Session: unknown"} {
		if !strings.Contains(string(detail.stdout), needle) {
			t.Fatalf("detail missing %q: %s", needle, detail.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeEvidence(t, "ale39/precedence.txt", detail.stdout)
}
func TestALE39CodeAFPluginStateAndAttachmentDistinction(t *testing.T) {
	fx := codeafFixture(t)
	manifest, settings := codeafPluginFixture(t, fx)
	ordinary := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "ordinary", "ordinary")
	// Plugin per-skill invocation metadata is not a native CodeAF disable.
	writeFile(t, filepath.Join(filepath.Dir(manifest), "agents/openai.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0600)
	before := treeManifest(t, fx.base)
	initial := codeafListResult(t, fx, 0)
	plugin := codeafPath(t, initial, filepath.Dir(manifest))
	if plugin.Availability.State != "available" || plugin.Availability.Invocation.State != "automatic-and-attachment" || plugin.Ownership.State != "managed" {
		t.Fatalf("plugin %+v", plugin)
	}
	if fact := plugin.Availability.References[0]; fact.Provenance.PluginID != "example@local" || fact.Provenance.Version != "1.2" || fact.Provenance.Kind != "plugin" {
		t.Fatalf("provenance %+v", fact)
	}
	for _, capability := range plugin.Availability.Capabilities {
		if capability.State != "unsupported" || capability.Scope != "codeaf-only" {
			t.Fatalf("capability %+v", capability)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// A project local setting wins over project and home enablement.
	writeFile(t, filepath.Join(fx.project, ".claude/settings.json"), []byte(`{"enabledPlugins":{"example@local":true}}`), 0600)
	writeFile(t, filepath.Join(fx.project, ".claude/settings.local.json"), []byte(`{"enabledPlugins":{"example@local":false}}`), 0600)
	before = treeManifest(t, fx.base)
	off := codeafListResult(t, fx, 0)
	if source := codeafPath(t, off, filepath.Dir(manifest)); source.Availability.State != "unavailable" || source.Availability.References[0].Permission.State != "denied" {
		t.Fatalf("off %+v", source)
	}
	if source := codeafPath(t, off, filepath.Dir(ordinary)); source.Availability.State != "available" {
		t.Fatalf("ordinary %+v", source)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, settings, []byte("not json"), 0600)
	uncertain := codeafListResult(t, fx, 1)
	if source := codeafPath(t, uncertain, filepath.Dir(manifest)); source.Availability.State != "unknown" {
		t.Fatalf("uncertain %+v", source)
	}
}
func TestALE39CodeAFProfileAndLoginEnvironment(t *testing.T) {
	fx := codeafFixture(t)
	override := filepath.Join(fx.base, "alternate-login")
	profile := filepath.Join(fx.base, "profile")
	codexSkill(t, filepath.Join(override, ".agents/skills"), "alternate", "alternate-home")
	codexSkill(t, filepath.Join(profile, ".agents/skills"), "profile", "profile-not-discovery")
	before := treeManifest(t, fx.base)
	run := func(extra []string) codexList {
		command := exec.Command(skillglassBinary, "list", "--agent", "codeaf", "--json")
		command.Dir = fx.project
		command.Env = append(fx.environment(), extra...)
		raw, err := command.Output()
		if err != nil {
			t.Fatal(err)
		}
		var list codexList
		decodeJSON(t, raw, &list)
		return list
	}
	for _, extra := range [][]string{{"CODEAF_HOME=" + override, "CODEAF_PROFILE_DIR=" + profile}, {"AFORGE_HOME=" + override, "AFORGE_PROFILE_DIR=" + profile}} {
		list := run(extra)
		if len(list.Sources) != 1 || list.Sources[0].Name != "alternate-home" {
			t.Fatalf("environment scope %+v", list.Sources)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE39CodeAFOfflineContainment(t *testing.T) {
	fx := codeafFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "contained", "contained-native")
	before := treeManifest(t, fx.base)
	profile := filepath.Join(fx.base, "deny-network.sb")
	writeFile(t, profile, []byte(runtimeProfile(skillglassBinary, fx.base)), 0600)
	before = treeManifest(t, fx.base)
	result := runSandboxCommand(t, fx, profile, skillglassBinary, "list", "--agent", "codeaf", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	if codexNamed(t, list, "contained-native").Availability.State != "available" {
		t.Fatal("offline native availability")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeEvidence(t, "ale39/offline.json", result.stdout)
}

func TestALE39CodeAFPluginDeclarationsScopesAndVersions(t *testing.T) {
	fx := codeafFixture(t)
	first := filepath.Join(fx.base, "plugins/first")
	second := filepath.Join(fx.base, "plugins/second")
	projectPlugin := filepath.Join(fx.base, "plugins/project")
	firstManifest := codexSkill(t, first, "declared/one", "plugin-conflict")
	secondManifest := codexSkill(t, second, "declared/two", "plugin-conflict")
	localManifest := codexSkill(t, projectPlugin, "skills/local", "project-plugin")
	codexSkill(t, first, "skills/not-declared", "not-declared")
	codexSkill(t, filepath.Join(fx.home, ".claude/plugins/cache/unregistered"), "skills/orphan", "not-installed")
	for _, path := range []string{first, second} {
		writeFile(t, filepath.Join(path, ".claude-plugin/plugin.json"), []byte(`{"skills":"declared"}`), 0600)
	}
	registry := map[string]any{"plugins": map[string]any{
		"alpha@local": []any{map[string]any{"scope": "user", "installPath": first, "version": "1"}, map[string]any{"scope": "user", "installPath": second, "version": "2"}},
		"local@local": []any{map[string]any{"scope": "project", "projectPath": fx.project, "installPath": projectPlugin, "version": "3"}},
	}}
	raw, _ := json.Marshal(registry)
	writeFile(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), raw, 0600)
	writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte(`{"enabledPlugins":{"alpha@local":true,"local@local":true}}`), 0600)
	list := codeafListResult(t, fx, 0)
	one := codeafPath(t, list, filepath.Dir(firstManifest))
	two := codeafPath(t, list, filepath.Dir(secondManifest))
	local := codeafPath(t, list, filepath.Dir(localManifest))
	if one.Availability.State != "available" || two.Availability.State != "shadowed" || two.Availability.Precedence.Winner != one.ID || one.ID == two.ID {
		t.Fatalf("versions one=%+v two=%+v", one, two)
	}
	if one.Availability.References[0].Provenance.Version != "1" || two.Availability.References[0].Provenance.Version != "2" {
		t.Fatal("installation version overwritten")
	}
	if local.Availability.State != "available" {
		t.Fatalf("project plugin %+v", local)
	}
	for _, source := range list.Sources {
		if source.Name == "not-declared" || source.Name == "not-installed" {
			t.Fatalf("unregistered folder loaded %+v", source)
		}
	}
	other := filepath.Join(fx.base, "different-project")
	if err := os.Mkdir(other, 0755); err != nil {
		t.Fatal(err)
	}
	result := fx.run(t, "list", "--agent", "codeaf", "--project", other, "--json")
	assertExit(t, result, 0)
	var otherList codexList
	decodeJSON(t, result.stdout, &otherList)
	for _, source := range otherList.Sources {
		if source.Name == "project-plugin" {
			t.Fatal("project registration leaked")
		}
	}
}

func TestALE39CodeAFNativeManifestRulesAndDirectRoot(t *testing.T) {
	fx := codeafFixture(t)
	root := filepath.Join(fx.project, ".codeaf/skills")
	child := codexSkill(t, root, "child", "native-child")
	writeFile(t, filepath.Join(root, "SKILL.md"), []byte("---\nname: not-a-root-skill\ndescription: Ignored native root manifest.\n---\n"), 0600)
	missing := filepath.Join(root, "missing/SKILL.md")
	writeFile(t, missing, []byte("---\nname: missing-description\n---\nNo native description.\n"), 0600)
	list := codeafListResult(t, fx, 0)
	if codeafPath(t, list, filepath.Dir(child)).Availability.State != "available" {
		t.Fatal("root manifest hid native child")
	}
	if codeafPath(t, list, filepath.Dir(missing)).Availability.State != "unavailable" {
		t.Fatal("missing description counted as eligible")
	}
	for _, source := range list.Sources {
		if source.Name == "not-a-root-skill" {
			t.Fatal("native root was treated as a bundle")
		}
	}
	// Native repairs colon-containing descriptions before YAML parsing, while generic inspection retains its malformed-manifest diagnostic.
	repaired := filepath.Join(root, "repaired/SKILL.md")
	writeFile(t, repaired, []byte("---\nname: repaired\ndescription: Topic: details\n---\nInert.\n"), 0600)
	repairedList := codeafListResult(t, fx, 1)
	if codeafPath(t, repairedList, filepath.Dir(repaired)).Availability.State != "available" {
		t.Fatal("native repaired description was skipped")
	}
}
