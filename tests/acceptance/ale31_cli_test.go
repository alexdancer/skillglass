package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hermesFixture(t *testing.T) (fixture, string) {
	t.Helper()
	fx := newFixture(t)
	root := filepath.Join(fx.home, ".hermes")
	profile := filepath.Join(root, "profiles", "research")
	writeFile(t, filepath.Join(root, "active_profile"), []byte("research\n"), 0600)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  external_dirs: [shared]\n"), 0600)
	if err := os.MkdirAll(filepath.Join(profile, "shared"), 0755); err != nil {
		t.Fatal(err)
	}
	return fx, profile
}
func hermesRun(t *testing.T, fx fixture, env []string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := fx.fixtureCommand(ctx, skillglassBinary, args...)
	command.Dir = fx.project
	command.Env = append(fx.environment(), env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.Stdin = bytes.NewReader(nil)
	started := time.Now()
	err := command.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	if ctx.Err() != nil {
		t.Fatal("Hermes public command timed out")
	}
	result := commandResult{code: code, stdout: stdout.Bytes(), stderr: stderr.Bytes(), duration: time.Since(started)}
	recordEvidenceCommand(t, "public-cli", skillglassBinary, args, fx.project, result)
	return result
}
func hermesList(t *testing.T, fx fixture, exit int, env []string, extra ...string) codexList {
	t.Helper()
	args := append([]string{"list", "--agent", "hermes", "--project", fx.project, "--json"}, extra...)
	result := hermesRun(t, fx, env, args...)
	assertExit(t, result, exit)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	return list
}
func TestALE31HermesProfileExternalPrecedenceSharingAndReading(t *testing.T) {
	fx, profile := hermesFixture(t)
	local := codexSkill(t, filepath.Join(profile, "skills"), "category/local", "collision")
	external := codexSkill(t, filepath.Join(profile, "shared"), "external", "collision")
	codexSkill(t, filepath.Join(fx.home, ".hermes/skills"), "inactive-default", "inactive-default")
	codexSkill(t, filepath.Join(fx.project, ".hermes/skills"), "not-automatic", "project-not-automatic")
	codexSkill(t, filepath.Join(filepath.Dir(fx.project), ".hermes/skills"), "not-ancestor", "ancestor-not-automatic")
	writeFile(t, filepath.Join(fx.project, ".hermes/config.yaml"), []byte("skills:\n  disabled: [collision]\n"), 0600)
	shared := codexSkill(t, filepath.Join(profile, "skills"), "shared", "shared")
	if err := os.Symlink(filepath.Dir(shared), filepath.Join(profile, "shared/alias")); err != nil {
		t.Fatal(err)
	}
	custom := codexSkill(t, fx.cleanRoot, "inspection", "inspection")
	before := treeManifest(t, fx.base)
	localResolved, _ := filepath.EvalSymlinks(filepath.Dir(local))
	externalResolved, _ := filepath.EvalSymlinks(filepath.Dir(external))
	list := hermesList(t, fx, 0, nil, "--root", filepath.Dir(custom))
	if list.Scope.Agent.Application != "hermes" || !strings.Contains(list.Scope.Agent.ValidatedVersion, "Hermes 0.20.0") || list.Scope.Completeness != "complete" {
		t.Fatalf("scope %+v", list.Scope)
	}
	var winner, shadow codexSource
	for _, source := range list.Sources {
		switch source.Name {
		case "inactive-default", "project-not-automatic", "ancestor-not-automatic":
			t.Fatalf("unloaded source leaked %+v", source)
		}
		switch source.SourcePath {
		case localResolved:
			winner = source
		case externalResolved:
			shadow = source
		}
	}
	if winner.Availability.State != "available" || shadow.Availability.State != "shadowed" || shadow.Availability.Precedence.Winner != winner.ID || winner.Availability.Invocation.State != "ambiguous-bare-name" {
		t.Fatalf("precedence winner=%+v shadow=%+v", winner, shadow)
	}
	if len(codexNamed(t, list, "shared").Availability.References) != 2 {
		t.Fatal("shared aliases were not retained")
	}
	if codexNamed(t, list, "inspection").Availability.State != "unavailable" {
		t.Fatal("explicit root treated as native")
	}
	read := fx.run(t, "read", "--agent", "hermes", "--project", fx.project, winner.ID)
	assertExit(t, read, 0)
	for _, needle := range []string{"Native name: collision", "Precedence:", "ambiguous-bare-name", "Session: unknown", "Capability: disable", "unverified"} {
		if !strings.Contains(string(read.stdout), needle) {
			t.Fatalf("missing %q: %s", needle, read.stdout)
		}
	}
	filtered := hermesList(t, fx, 0, nil, "--availability", "shadowed")
	if len(filtered.Sources) != 1 || filtered.Sources[0].ID != shadow.ID {
		t.Fatalf("shadow filter %+v", filtered)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE31HermesNativeDisabledUnionAndProfileHomeOverride(t *testing.T) {
	fx, profile := hermesFixture(t)
	for _, name := range []string{"global-denied", "platform-denied", "available"} {
		codexSkill(t, filepath.Join(profile, "skills"), name, name)
	}
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  disabled: [global-denied]\n  platform_disabled:\n    telegram: [platform-denied]\n    discord: []\n"), 0600)
	before := treeManifest(t, fx.base)
	telegram := hermesList(t, fx, 0, []string{"HERMES_PLATFORM=telegram", "HERMES_SESSION_PLATFORM=discord", "HERMES_HOME=" + filepath.Join(fx.home, ".hermes")})
	for _, name := range []string{"global-denied", "platform-denied"} {
		source := codexNamed(t, telegram, name)
		if source.Availability.State != "disabled" || source.Availability.References[0].Permission.State != "denied" {
			t.Fatalf("disabled union %+v", source)
		}
	}
	discord := hermesList(t, fx, 0, []string{"HERMES_PLATFORM=discord"})
	if codexNamed(t, discord, "global-denied").Availability.State != "disabled" || codexNamed(t, discord, "platform-denied").Availability.State != "available" {
		t.Fatal("platform disabled replaced global policy")
	}
	writeFile(t, filepath.Join(fx.home, ".hermes/active_profile"), []byte("missing-profile\n"), 0600)
	before = treeManifest(t, fx.base)
	specific := hermesList(t, fx, 0, []string{"HERMES_HOME=" + profile})
	if codexNamed(t, specific, "available").Availability.State != "available" {
		t.Fatal("explicit profile home was redirected by sticky profile")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE31HermesOrgPlatformNestedAndProjectConfiguredScope(t *testing.T) {
	fx, profile := hermesFixture(t)
	skills := filepath.Join(profile, "skills")
	codexSkill(t, skills, "_org/current/live", "org-live")
	codexSkill(t, skills, "_org/old/stale", "org-stale")
	writeFile(t, filepath.Join(skills, "_org/.active_org"), []byte("current\n"), 0600)
	codexSkill(t, skills, "parent", "parent")
	codexSkill(t, skills, "parent/nested", "nested")
	codexSkill(t, skills, "parent/references/archive", "support-hidden")
	codexSkill(t, skills, "node_modules/dependency", "dependency-hidden")
	native := codexSkill(t, skills, "windows", "windows-only")
	writeFile(t, native, []byte("---\nname: windows-only\ndescription: Inert.\nplatforms: [windows]\n---\nInert.\n"), 0600)
	projectRoot := filepath.Join(fx.project, "selected-skills")
	codexSkill(t, projectRoot, "project", "configured-project")
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte(fmt.Sprintf("skills:\n  external_dirs: [%q]\n", projectRoot)), 0600)
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, nil)
	for name, state := range map[string]string{"org-live": "available", "org-stale": "unavailable", "parent": "available", "nested": "available", "windows-only": "unavailable", "configured-project": "available"} {
		if source := codexNamed(t, list, name); source.Availability.State != state {
			t.Fatalf("%s %+v", name, source)
		}
	}
	for _, source := range list.Sources {
		if source.Name == "support-hidden" || source.Name == "dependency-hidden" {
			t.Fatalf("excluded package entered inventory %+v", source)
		}
	}
	project := hermesList(t, fx, 0, nil, "--context", "project")
	if len(project.Sources) != 1 || project.Sources[0].Name != "configured-project" {
		t.Fatalf("project context %+v", project)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE31HermesPluginsProvenanceNamespacesAndInactiveSources(t *testing.T) {
	fx, profile := hermesFixture(t)
	for _, name := range []string{"portable", "inactive", "dynamic"} {
		root := filepath.Join(profile, "plugins/category", name)
		codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
		if name == "dynamic" {
			writeFile(t, filepath.Join(root, "plugin.yaml"), []byte("name: dynamic\nversion: '1'\n"), 0600)
			writeFile(t, filepath.Join(root, "__init__.py"), []byte("raise RuntimeError('must never execute')\n"), 0600)
		} else {
			writeFile(t, filepath.Join(root, "plugin.json"), []byte(fmt.Sprintf(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":%q,"version":"1"}`, name)), 0600)
		}
	}
	projectPlugin := filepath.Join(fx.project, ".hermes/plugins/project")
	codexSkill(t, filepath.Join(projectPlugin, "skills"), "project-skill", "project-skill")
	writeFile(t, filepath.Join(projectPlugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"project"}`), 0600)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [category/portable, category/dynamic, project]\n"), 0600)
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, nil)
	found := make(map[string]string)
	for _, source := range list.Sources {
		for _, ref := range source.Availability.References {
			found[ref.Provenance.PluginID] = source.Availability.State
			if source.Ownership.State != "managed" {
				t.Fatal("plugin ownership lost")
			}
		}
	}
	for name, state := range map[string]string{"category/portable": "explicit-only", "category/inactive": "unavailable", "category/dynamic": "unknown", "project": "unavailable"} {
		if found[name] != state {
			t.Fatalf("plugin %s state %s, want %s", name, found[name], state)
		}
	}
	explicit := hermesList(t, fx, 0, nil, "--availability", "explicit-only")
	if len(explicit.Sources) != 1 || explicit.Sources[0].Availability.NativeName != "agent-plugin-category-portable-aba1f9b5:explicit" {
		t.Fatalf("namespace %+v", explicit)
	}
	project := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "--availability", "explicit-only")
	if len(project.Sources) != 2 {
		t.Fatal("opted-in exact project plugin missing")
	}
	safe := hermesList(t, fx, 0, []string{"HERMES_SAFE_MODE=1"}, "--availability", "explicit-only")
	if len(safe.Sources) != 0 {
		t.Fatal("safe mode plugin remained effective")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE31HermesMissingNativeInputsAndReadOnlyCoverage(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "inert", "inert")
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  external_dirs: [missing-root]\n"), 0600)
	before := treeManifest(t, fx.base)
	missing := hermesList(t, fx, 1, nil)
	if missing.Scope.Completeness != "incomplete" || codexNamed(t, missing, "inert").Availability.State != "unknown" {
		t.Fatalf("missing scope %+v", missing)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills: [broken\n"), 0600)
	before = treeManifest(t, fx.base)
	broken := hermesList(t, fx, 1, nil)
	if codexNamed(t, broken, "inert").Availability.State != "unknown" {
		t.Fatal("unreadable config silently allowed")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE31HermesReadOnlyOfflinePublicExecutableAndPTY(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "offline", "offline-hermes")
	binary := canonicalPath(t, skillglassBinary)
	sandbox := filepath.Join(fx.base, "allowed/hermes.sb")
	writeFile(t, sandbox, []byte(runtimeProfile(binary, canonicalPath(t, filepath.Join(fx.base, "allowed")))), 0600)
	before := treeManifest(t, fx.base)
	result := runSandboxCommand(t, fx, sandbox, binary, "list", "--agent", "hermes", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	selected := codexNamed(t, list, "offline-hermes")
	explanation := runSandboxCommand(t, fx, sandbox, binary, "explain", "--agent", "hermes", "--project", fx.project, selected.ID)
	assertExit(t, explanation, 0)
	if !strings.Contains(string(explanation.stdout), "Session: unknown") || !strings.Contains(string(explanation.stdout), "hermes-profile") {
		t.Fatalf("explanation %s", explanation.stdout)
	}
	read := runSandboxCommand(t, fx, sandbox, binary, "read", "--agent", "hermes", selected.ID)
	assertExit(t, read, 0)
	if !strings.Contains(string(read.stdout), "Inert instructions.") {
		t.Fatalf("full read %s", read.stdout)
	}
	raw := runSandboxBrowser(t, fx, sandbox, binary, "--agent", "hermes", "--project", fx.project)
	if !bytes.Contains(raw, []byte("offline-hermes")) || !bytes.Contains(raw, []byte("Agent: hermes")) {
		t.Fatalf("sandboxed Hermes PTY %q", raw)
	}
	writeEvidence(t, "ale31/isolation/list.json", result.stdout)
	writeEvidence(t, "ale31/isolation/explain.txt", explanation.stdout)
	writeEvidence(t, "ale31/isolation/read.txt", read.stdout)
	writeEvidence(t, "ale31/isolation/browse.ansi", raw)
	writeEvidence(t, "ale31/isolation/runtime.sb", []byte(runtimeProfile(binary, canonicalPath(t, filepath.Join(fx.base, "allowed")))))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE31HermesSameProfileConflictsAndExplicitProjectInspection(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "first", "local-conflict")
	codexSkill(t, filepath.Join(profile, "skills"), "second", "local-conflict")
	projectRoot := filepath.Join(fx.project, ".hermes/skills")
	codexSkill(t, projectRoot, "project", "project-inspection")
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, nil, "--project-root", projectRoot)
	count := 0
	for _, source := range list.Sources {
		if source.Name == "local-conflict" {
			count++
			if source.Availability.State != "unknown" || source.Availability.Evidence.Kind != "unknown" || source.Availability.Precedence.State != "unknown" || source.Availability.Invocation.State != "ambiguous-bare-name" {
				t.Fatalf("false effective winner %+v", source)
			}
		}
	}
	if count != 2 {
		t.Fatalf("independent profile copies merged: %d", count)
	}
	if codexNamed(t, list, "project-inspection").Availability.State != "unavailable" {
		t.Fatal("project inspection root became native")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE31HermesCacheWithoutRegistrationAndBrokenReferences(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "native", "native")
	codexSkill(t, filepath.Join(profile, "plugins/cache"), "orphan", "cache-only")
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, nil)
	if codexNamed(t, list, "cache-only").Availability.State != "unavailable" {
		t.Fatal("cache alone counted as enabled")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if err := os.Symlink("missing-target", filepath.Join(profile, "skills/broken")); err != nil {
		t.Fatal(err)
	}
	before = treeManifest(t, fx.base)
	broken := hermesList(t, fx, 1, nil)
	found := false
	for _, problem := range broken.Problems {
		found = found || problem.Code == "broken-reference"
	}
	if !found || codexNamed(t, broken, "native").Availability.State != "unknown" {
		t.Fatalf("broken reference coverage %+v", broken)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE31HermesDisplayProfileNameNormalizesBeforeResolution(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "normalized-profile", "normalized-profile")
	writeFile(t, filepath.Join(fx.home, ".hermes/active_profile"), []byte("Research\n"), 0600)
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, nil)
	if codexNamed(t, list, "normalized-profile").Availability.State != "available" {
		t.Fatal("display name failed to select canonical profile")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
