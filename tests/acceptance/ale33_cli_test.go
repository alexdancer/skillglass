package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func piFixture(t *testing.T) fixture {
	t.Helper()
	fx := newFixture(t)
	if err := os.MkdirAll(filepath.Join(fx.project, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), []byte(`{"defaultProjectTrust":"always"}`), 0600)
	return fx
}
func piList(t *testing.T, fx fixture, exit int, extra ...string) codexList {
	t.Helper()
	args := append([]string{"list", "--agent", "pi", "--project", fx.project, "--json"}, extra...)
	r := fx.run(t, args...)
	assertExit(t, r, exit)
	var list codexList
	decodeJSON(t, r.stdout, &list)
	return list
}
func TestALE33PiAvailabilityResourcesAndIdentity(t *testing.T) {
	fx := piFixture(t)
	global := filepath.Join(fx.home, ".pi", "agent", "skills")
	project := filepath.Join(fx.project, ".pi", "skills")
	codexSkill(t, global, "duplicate", "duplicate")
	codexSkill(t, project, "duplicate", "duplicate")
	codexSkill(t, global, "inherit", "inherit")
	explicit := codexSkill(t, global, "explicit", "explicit")
	writeFile(t, explicit, []byte("---\nname: explicit\ndescription: Explicit native skill.\ndisable-model-invocation: true\n---\nExplicit body\n"), 0600)
	shared := codexSkill(t, filepath.Join(fx.base, "allowed", "shared"), "bundle", "shared")
	alias := filepath.Join(project, "shared-alias")
	if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
		t.Fatal(err)
	}
	configured := filepath.Join(fx.base, "allowed", "configured")
	codexSkill(t, configured, "selected", "selected")
	codexSkill(t, configured, "excluded", "excluded")
	pkg := filepath.Join(fx.base, "allowed", "package")
	codexSkill(t, filepath.Join(pkg, "skills"), "managed", "managed")
	writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"name":"fixture-package","version":"1.2.3","pi":{"skills":["skills"]}}`), 0600)
	settings := map[string]any{"skills": []string{configured, "!excluded"}, "packages": []any{map[string]any{"source": pkg, "skills": []string{}}}}
	raw, _ := json.Marshal(settings)
	writeFile(t, filepath.Join(fx.project, ".pi", "settings.json"), raw, 0600)
	before := treeManifest(t, fx.base)
	list := piList(t, fx, 0, "--root", filepath.Dir(shared))
	if list.Scope.Agent.Application != "pi" || list.Scope.Agent.ValidatedVersion != "pi 1.1.0" {
		t.Fatalf("scope: %+v", list.Scope)
	}
	states := map[string]string{}
	var duplicate []codexSource
	for _, s := range list.Sources {
		states[s.Name] = s.Availability.State
		if s.Name == "duplicate" {
			duplicate = append(duplicate, s)
		}
	}
	for n, want := range map[string]string{"inherit": "available", "explicit": "explicit-only", "selected": "available", "excluded": "disabled", "managed": "disabled", "shared": "available"} {
		if states[n] != want {
			t.Fatalf("%s got %s want %s", n, states[n], want)
		}
	}
	if len(duplicate) != 2 {
		t.Fatalf("copies merged: %+v", duplicate)
	}
	var winner string
	for _, s := range duplicate {
		if s.Availability.State == "available" {
			winner = s.ID
		}
	}
	if winner == "" {
		t.Fatal("missing winner")
	}
	for _, s := range duplicate {
		if s.ID != winner && (s.Availability.State != "shadowed" || s.Availability.Precedence.Winner != winner) {
			t.Fatalf("bad shadow: %+v", s)
		}
	}
	sharedSource := codexNamed(t, list, "shared")
	refs := 0
	for _, r := range list.References {
		if r.SourceID == sharedSource.ID {
			refs++
		}
	}
	if refs != 2 {
		t.Fatalf("shared references=%d", refs)
	}
	managed := codexNamed(t, list, "managed")
	if managed.Ownership.State != "managed" || managed.Availability.References[0].Provenance.Kind != "plugin" {
		t.Fatalf("provenance %+v", managed)
	}
	for _, op := range []string{"read", "explain"} {
		r := fx.run(t, op, "--agent", "pi", "--project", fx.project, "--json", sharedSource.ID)
		assertExit(t, r, 0)
	}
	r := fx.run(t, "list", "--agent", "pi", "--project", fx.project, "--availability", "shadowed", "--json")
	assertExit(t, r, 0)
	var filtered codexList
	decodeJSON(t, r.stdout, &filtered)
	if len(filtered.Sources) != 1 {
		t.Fatalf("filtered=%d", len(filtered.Sources))
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE33PiProjectFiltersDoNotDisableGlobalRoots(t *testing.T) {
	fx := piFixture(t)
	global := filepath.Join(fx.home, ".pi", "agent", "skills")
	project := filepath.Join(fx.project, ".pi", "skills")
	codexSkill(t, global, "same", "same")
	codexSkill(t, project, "same", "same")
	writeFile(t, filepath.Join(fx.project, ".pi", "settings.json"), []byte(`{"skills":["!same"]}`), 0600)
	list := piList(t, fx, 0)
	for _, s := range list.Sources {
		if s.Name == "same" {
			want := "available"
			if strings.Contains(s.SourcePath, fx.project) {
				want = "disabled"
			}
			if s.Availability.State != want {
				t.Fatalf("%s %+v", want, s)
			}
		}
	}
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), []byte(`{}`), 0600)
	list = piList(t, fx, 0)
	for _, s := range list.Sources {
		if s.Name == "same" && strings.Contains(s.SourcePath, fx.project) && s.Availability.State != "ask" {
			t.Fatalf("unremembered trust: %+v", s)
		}
	}
	canonicalProject, err := filepath.EvalSymlinks(fx.project)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]bool{canonicalProject: false})
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "trust.json"), raw, 0600)
	list = piList(t, fx, 0)
	for _, s := range list.Sources {
		if s.Name == "same" && strings.Contains(s.SourcePath, fx.project) && s.Availability.State != "unavailable" {
			t.Fatalf("denied trust: %+v", s)
		}
	}
}
func TestALE33PiMissingSettingsAndExplicitInspection(t *testing.T) {
	fx := piFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".pi", "agent", "skills"), "safe", "safe")
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), []byte(`{"skills":"UNRELATED_PRIVATE_CONTENT"}`), 0600)
	list := piList(t, fx, 1)
	if codexNamed(t, list, "safe").Availability.State != "unknown" {
		t.Fatal("invalid input inferred available")
	}
	r := fx.run(t, "list", "--agent", "pi", "--json")
	if strings.Contains(string(r.stdout), "UNRELATED_PRIVATE_CONTENT") {
		t.Fatal("leaked settings")
	}
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), []byte(`{"skills":["missing"]}`), 0600)
	piList(t, fx, 1)
}

func TestALE33PiStandaloneFilesNativeFallbackAndReferenceFilters(t *testing.T) {
	fx := piFixture(t)
	auto := filepath.Join(fx.home, ".pi", "agent", "skills")
	file := filepath.Join(auto, "standalone.md")
	writeFile(t, file, []byte("---\nname: standalone\ndescription: Native file skill.\n---\nFull file instructions.\n"), 0600)
	fallback := filepath.Join(auto, "fallback", "SKILL.md")
	writeFile(t, fallback, []byte("---\ndescription: Native name fallback.\n---\nBody.\n"), 0600)
	codexSkill(t, auto, "a", "same-tier")
	codexSkill(t, auto, "b", "same-tier")
	shared := codexSkill(t, filepath.Join(fx.base, "allowed", "shared"), "bundle", "shared")
	alias := filepath.Join(fx.project, ".pi", "skills", "alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
		t.Fatal(err)
	}
	// Canonical-target exclusion does not match the independently discovered alias.
	raw, _ := json.Marshal(map[string]any{"skills": []string{"-" + filepath.Dir(shared)}})
	settings := filepath.Join(fx.project, ".pi", "settings.json")
	writeFile(t, settings, raw, 0600)
	before := treeManifest(t, fx.base)
	list := piList(t, fx, 0)
	for _, name := range []string{"standalone", "fallback", "shared"} {
		if codexNamed(t, list, name).Availability.State != "available" {
			t.Fatalf("%s unavailable", name)
		}
	}
	for _, s := range list.Sources {
		if s.Name == "same-tier" {
			want := "available"
			if filepath.Base(s.SourcePath) == "b" {
				want = "shadowed"
			}
			if s.Availability.State != want {
				t.Fatalf("same-tier %+v", s)
			}
		}
	}
	standalone := codexNamed(t, list, "standalone")
	read := fx.run(t, "read", "--agent", "pi", "--project", fx.project, standalone.ID)
	assertExit(t, read, 0)
	if !strings.Contains(string(read.stdout), "Full file instructions.") {
		t.Fatal("file body not readable")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	raw, _ = json.Marshal(map[string]any{"skills": []string{"-" + alias}})
	writeFile(t, settings, raw, 0600)
	if codexNamed(t, piList(t, fx, 0), "shared").Availability.State != "disabled" {
		t.Fatal("alias exclusion did not apply")
	}
}
func TestALE33PiOfflineBrowseAndCompare(t *testing.T) {
	fx := piFixture(t)
	root := filepath.Join(fx.home, ".pi", "agent", "skills")
	codexSkill(t, root, "copy-a", "copy-a")
	codexSkill(t, root, "copy-b", "copy-b")
	script := filepath.Join(root, "copy-a", "scripts", "danger.sh")
	writeFile(t, script, []byte("#!/bin/sh\nexit 91\n"), 0700)
	before := treeManifest(t, fx.base)
	canonicalProduct := canonicalPath(t, skillglassBinary)
	canonicalAllowed := canonicalPath(t, filepath.Join(fx.base, "allowed"))
	profile := filepath.Join(fx.base, "allowed", "pi-read.sb")
	policy := runtimeProfile(canonicalProduct, canonicalAllowed)
	var ancestors []string
	for dir := filepath.Dir(canonicalAllowed); dir != "/"; dir = filepath.Dir(dir) {
		ancestors = append(ancestors, "(literal "+sandboxString(dir)+")")
	}
	policy = strings.Replace(policy, `(literal "/")`, `(literal "/")`+strings.Join(ancestors, " "), 1)
	writeFile(t, profile, []byte(policy), 0600)
	writeEvidence(t, "ale33/offline.sb", []byte(policy))
	before = treeManifest(t, fx.base)
	r := runSandboxCommand(t, fx, profile, canonicalProduct, "list", "--agent", "pi", "--project", fx.project, "--json")
	assertExit(t, r, 0)
	var list codexList
	decodeJSON(t, r.stdout, &list)
	a := codexNamed(t, list, "copy-a")
	b := codexNamed(t, list, "copy-b")
	for _, op := range []string{"read", "explain"} {
		r = runSandboxCommand(t, fx, profile, canonicalProduct, op, "--agent", "pi", "--project", fx.project, a.ID)
		assertExit(t, r, 0)
	}
	r = runSandboxCommand(t, fx, profile, canonicalProduct, "compare", "--agent", "pi", "--project", fx.project, "--json", a.ID, b.ID)
	assertExit(t, r, 0)
	writeEvidence(t, "ale33/offline-list.json", listJSON(t, list))
	writeEvidence(t, "ale33/offline-compare.json", r.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func listJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestALE33PiConfiguredAgentDirectoryAndInactiveManagedCache(t *testing.T) {
	fx := piFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".pi", "agent", "skills"), "ignored", "ignored-default-root")
	agentDir := filepath.Join(fx.base, "allowed", "separate-native-root")
	codexSkill(t, filepath.Join(agentDir, "skills"), "selected", "configured-agent-dir")
	codexSkill(t, filepath.Join(agentDir, "npm", "node_modules", "stale", "skills"), "cached", "inactive-cached")
	before := treeManifest(t, fx.base)
	command := exec.Command(skillglassBinary, "list", "--agent", "pi", "--project", fx.project, "--json")
	command.Dir = filepath.Join(fx.base, "allowed")
	command.Env = append(fx.environment(), "PI_CODING_AGENT_DIR=separate-native-root")
	raw, err := command.Output()
	if err != nil {
		t.Fatalf("independent root: %v %s", err, raw)
	}
	var list codexList
	decodeJSON(t, raw, &list)
	for _, s := range list.Sources {
		if s.Name == "ignored-default-root" {
			t.Fatal("configured agent dir fell back to default")
		}
	}
	if codexNamed(t, list, "configured-agent-dir").Availability.State != "available" {
		t.Fatal("selected native root unavailable")
	}
	cached := codexNamed(t, list, "inactive-cached")
	if cached.Availability.State != "unavailable" || cached.Ownership.State != "managed" {
		t.Fatalf("cache enabled: %+v", cached)
	}
	writeEvidence(t, "ale33/independent-agent-dir.json", raw)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE33PiFiveTierPrecedence(t *testing.T) {
	fx := piFixture(t)
	global := filepath.Join(fx.home, ".pi", "agent", "skills")
	local := filepath.Join(fx.project, ".pi", "skills")
	globalConfig := filepath.Join(fx.base, "allowed", "global-config")
	projectConfig := filepath.Join(fx.base, "allowed", "project-config")
	pkg := filepath.Join(fx.base, "allowed", "precedence-package")
	names := []string{"project-settings-win", "project-auto-win", "global-settings-win", "global-auto-win"}
	for _, name := range names {
		codexSkill(t, global, name, name)
		codexSkill(t, filepath.Join(pkg, "skills"), name, name)
	}
	for _, name := range names[:3] {
		codexSkill(t, globalConfig, name, name)
	}
	for _, name := range names[:2] {
		codexSkill(t, local, name, name)
	}
	codexSkill(t, projectConfig, names[0], names[0])
	writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"name":"precedence","version":"1.0.0","pi":{"skills":["skills"]}}`), 0600)
	raw, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "skills": []string{globalConfig}, "packages": []string{pkg}})
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), raw, 0600)
	raw, _ = json.Marshal(map[string]any{"skills": []string{projectConfig}})
	writeFile(t, filepath.Join(fx.project, ".pi", "settings.json"), raw, 0600)
	before := treeManifest(t, fx.base)
	list := piList(t, fx, 0)
	expected := map[string]string{names[0]: projectConfig, names[1]: local, names[2]: globalConfig, names[3]: global}
	for name, root := range expected {
		winnerPath := canonicalPath(t, filepath.Join(root, name))
		var winner string
		for _, s := range list.Sources {
			if s.Name == name && s.SourcePath == winnerPath {
				winner = s.ID
				if s.Availability.State != "available" {
					t.Fatalf("%s winner unavailable %+v", name, s)
				}
			}
		}
		if winner == "" {
			t.Fatalf("%s winner missing", name)
		}
		for _, s := range list.Sources {
			if s.Name == name && s.ID != winner {
				if s.Availability.State != "shadowed" || s.Availability.Precedence.Winner != winner {
					t.Fatalf("%s lower tier %+v", name, s)
				}
			}
		}
	}
	writeEvidence(t, "ale33/precedence.json", listJSON(t, list))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE33PiEmptyProjectDoesNotRequireTrust(t *testing.T) {
	fx := piFixture(t)
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), []byte(`{"defaultProjectTrust":"never"}`), 0600)
	codexSkill(t, filepath.Join(fx.home, ".pi", "agent", "skills"), "global", "global")
	list := piList(t, fx, 0)
	for _, layer := range list.Scope.Agent.ConfigLayers {
		if filepath.Base(layer.Path) == "trust.json" && layer.State != "allowed" {
			t.Fatalf("empty project unnecessarily gated %+v", layer)
		}
	}
}

func TestALE33PiDeniedProjectAliasDoesNotBlockGlobalReference(t *testing.T) {
	fx := piFixture(t)
	global := filepath.Join(fx.home, ".pi", "agent", "skills")
	shared := codexSkill(t, global, "shared", "shared")
	alias := filepath.Join(fx.project, ".pi", "skills", "alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
		t.Fatal(err)
	}
	project := canonicalPath(t, fx.project)
	raw, _ := json.Marshal(map[string]bool{project: false})
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "trust.json"), raw, 0600)
	list := piList(t, fx, 0)
	source := codexNamed(t, list, "shared")
	if source.Availability.State != "available" {
		t.Fatalf("blocked alias suppresses global reference %+v", source)
	}
	if len(source.Availability.References) != 2 {
		t.Fatal("lost shared-reference provenance")
	}
}
