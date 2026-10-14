package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

type codexEvidence struct {
	Kind, Rule, Reason string
	InputPath          string `json:"input_path"`
	AgentVersion       string `json:"agent_version"`
	Setting            string `json:"setting"`
}
type codexAvailability struct {
	NativeNameEvidence codexEvidence `json:"native_name_evidence"`
	NativeName         string        `json:"native_name"`

	State, Reason string
	Evidence      codexEvidence
	References    []struct {
		ReferenceID   string `json:"reference_id"`
		NativeName    string `json:"native_name"`
		Participation string
		Evidence      codexEvidence
		Permission    struct {
			State    string
			Evidence codexEvidence
			Layers   []struct{ Path, State string }
		}
		Provenance struct {
			Kind                          string
			PluginID                      string `json:"plugin_id"`
			Version                       string
			Evidence, Setting, Activation codexEvidence
			Records                       []struct {
				Kind, Path, Scope, Applicability string
				InputPath                        string          `json:"input_path"`
				ProjectPath                      json.RawMessage `json:"project_path"`
				Version                          json.RawMessage
				RegistrationInput                string          `json:"registration_input"`
				LastUpdated                      json.RawMessage `json:"last_updated"`
				DeclaredSource                   string          `json:"declared_source"`
			}
		}
	}
	Precedence struct {
		State, Winner  string
		RelatedSources []string `json:"related_sources"`
		Evidence       codexEvidence
	}
	Invocation, Session, Reload struct {
		State    string
		Evidence codexEvidence
	}
	Capabilities []struct{ Operation, Scope, State string }
}
type codexSource struct {
	Ownership struct{ State string }

	ID, Name     string
	SourcePath   string `json:"source_path"`
	Availability codexAvailability
}
type codexList struct {
	Sources    []codexSource
	References []struct {
		SourceID       string `json:"source_id"`
		Context        string
		DiscoveredPath string `json:"discovered_path"`
	}
	Scope struct {
		Completeness string
		Agent        struct {
			Application      string
			ValidatedVersion string                         `json:"validated_version"`
			ConfigLayers     []struct{ Path, State string } `json:"config_layers"`
			Gaps             []string
		}
		Roots []struct {
			RequestedPath          string `json:"requested_path"`
			Context, Origin, State string
		}
	}
	Problems []struct{ Code string }
}

func codexFixture(t *testing.T) (fixture, string) {
	t.Helper()
	fx := newFixture(t)
	// Generic fixture's fallback canary becomes a valid, intentional native user skill.
	codexSkill(t, filepath.Join(fx.home, ".codex", "skills"), "home-canary", "home-native")
	command := exec.Command("/usr/bin/git", "init", "-q", fx.project)
	command.Env = fx.environment()
	if raw, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", raw, err)
	}
	root, err := filepath.EvalSymlinks(fx.project)
	if err != nil {
		t.Fatal(err)
	}
	fx.project = filepath.Join(root, "nested", "leaf")
	if err := os.MkdirAll(fx.project, 0755); err != nil {
		t.Fatal(err)
	}
	return fx, root
}
func codexSkill(t *testing.T, root, folder, name string) string {
	t.Helper()
	path := filepath.Join(root, folder, "SKILL.md")
	writeFile(t, path, []byte(fmt.Sprintf("---\nname: %s\ndescription: Fixture %s.\n---\nInert instructions.\n", name, folder)), 0600)
	return path
}
func codexListResult(t *testing.T, fx fixture, exit int, extra ...string) codexList {
	t.Helper()
	args := append([]string{"list", "--agent", "codex", "--project", fx.project, "--json"}, extra...)
	result := fx.run(t, args...)
	assertExit(t, result, exit)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	return list
}
func codexNamed(t *testing.T, list codexList, name string) codexSource {
	t.Helper()
	for _, source := range list.Sources {
		if source.Name == name {
			return source
		}
	}
	t.Fatalf("source %s absent from %+v", name, list.Sources)
	return codexSource{}
}
func selector(path string, enabled bool) string {
	encoded, _ := json.Marshal(path)
	return fmt.Sprintf("[[skills.config]]\npath = %s\nenabled = %t\n", encoded, enabled)
}
func trustProject(root string) string {
	encoded, _ := json.Marshal(root)
	return fmt.Sprintf("[projects.%s]\ntrust_level = 'trusted'\n", encoded)
}

func TestALE26NativeDiscoveryCoexistenceSharingAndExplanation(t *testing.T) {
	fx, root := codexFixture(t)
	for _, path := range []string{root, filepath.Dir(fx.project), fx.project} {
		codexSkill(t, filepath.Join(path, ".agents", "skills"), "collision", "collision")
		codexSkill(t, filepath.Join(path, ".codex", "skills"), "legacy", filepath.Base(path)+"-legacy")
	}
	codexSkill(t, filepath.Join(filepath.Dir(root), ".agents", "skills"), "above-repo", "outside-boundary")
	codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "user", "collision")
	codexSkill(t, filepath.Join(fx.home, ".codex", "skills"), "legacy-user", "collision")
	shared := codexSkill(t, filepath.Join(fx.base, "allowed", "shared"), "source", "shared")
	for _, alias := range []string{filepath.Join(fx.home, ".agents", "skills", "alias"), filepath.Join(fx.home, ".codex", "skills", "alias")} {
		if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
			t.Fatal(err)
		}
	}
	codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "group/nested", "nested-skill")
	custom := codexSkill(t, fx.cleanRoot, "custom-native", "custom-inspection")
	config := filepath.Join(fx.home, ".codex", "config.toml")
	writeFile(t, config, []byte(selector(filepath.Join(fx.home, ".agents", "skills", "alias", "SKILL.md"), false)), 0600)
	before := treeManifest(t, fx.base)
	list := codexListResult(t, fx, 0, "--root", filepath.Dir(custom))
	if list.Scope.Agent.Application != "codex" || list.Scope.Agent.ValidatedVersion != "codex-cli 0.155.1" || list.Scope.Completeness != "complete" {
		t.Fatalf("scope: %+v", list.Scope)
	}
	count := 0
	for _, source := range list.Sources {
		if source.Name == "outside-boundary" {
			t.Fatal("traversed above Git repo")
		}
		if source.Name == "collision" {
			count++
			if source.Availability.State != "available" || source.Availability.Precedence.State != "coexisting" || source.Availability.Precedence.Winner != "" || len(source.Availability.Precedence.RelatedSources) != 4 {
				t.Fatalf("coexistence: %+v", source)
			}
		}
	}
	if count != 5 {
		t.Fatalf("collision count=%d", count)
	}
	codexNamed(t, list, "nested-skill")
	for _, name := range []string{filepath.Base(root) + "-legacy", "nested-legacy", "leaf-legacy"} {
		codexNamed(t, list, name)
	}
	sharedSource := codexNamed(t, list, "shared")
	if sharedSource.Availability.State != "disabled" || len(sharedSource.Availability.References) != 2 {
		t.Fatalf("shared explanation: %+v", sharedSource)
	}
	for _, fact := range sharedSource.Availability.References {
		if fact.Permission.State != "denied" || fact.Permission.Evidence.Rule != "manifest-selector" {
			t.Fatalf("alias permission: %+v", fact)
		}
	}
	if got := codexNamed(t, list, "custom-inspection").Availability.State; got != "unknown" {
		t.Fatalf("custom state=%s", got)
	}
	filtered := codexListResult(t, fx, 0, "--availability", "disabled", "--source-root", filepath.Join(fx.home, ".agents", "skills"))
	if len(filtered.Sources) != 1 || !reflect.DeepEqual(filtered.Sources[0].Availability, sharedSource.Availability) {
		t.Fatal("display filter changed source explanation")
	}
	explain := fx.run(t, "explain", "--agent", "codex", "--project", fx.project, "--json", sharedSource.ID)
	assertExit(t, explain, 0)
	var detail struct {
		Operation string
		Source    codexSource
	}
	decodeJSON(t, explain.stdout, &detail)
	if detail.Operation != "explain" || !reflect.DeepEqual(detail.Source.Availability, sharedSource.Availability) {
		t.Fatal("explain uses different facts")
	}
	human := fx.run(t, "explain", "--agent", "codex", "--project", fx.project, sharedSource.ID)
	assertExit(t, human, 0)
	for _, needle := range []string{"Permission: denied (inferred)", "manifest-selector", "Session: unknown", "Reload: unknown", "Capability: disable", "unsupported"} {
		if !strings.Contains(string(human.stdout), needle) {
			t.Fatalf("explanation missing %q", needle)
		}
	}
	for _, capability := range sharedSource.Availability.Capabilities {
		if capability.Operation == "enable" || capability.Operation == "disable" {
			if capability.State != "supported" || capability.Scope != "global" {
				t.Fatal("validated global writer missing")
			}
		} else if capability.State != "unsupported" {
			t.Fatal("broadened control scope")
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	global := fx.run(t, "list", "--agent", "codex", "--json")
	assertExit(t, global, 0)
	var globals codexList
	decodeJSON(t, global.stdout, &globals)
	for _, source := range globals.Sources {
		if source.Name == "leaf-legacy" {
			t.Fatal("implicit CWD project selection")
		}
	}
}
func TestALE26TrustedLayerReplacementFolderTargetsAndMissingInputs(t *testing.T) {
	fx, root := codexFixture(t)
	user := codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "user", "user-choice")
	repo := codexSkill(t, filepath.Join(root, ".agents", "skills"), "repo", "repo-choice")
	leaf := codexSkill(t, filepath.Join(fx.project, ".agents", "skills"), "leaf", "leaf-choice")
	config := filepath.Join(fx.home, ".codex", "config.toml")
	writeFile(t, config, []byte(selector(user, false)), 0600)
	writeFile(t, filepath.Join(root, ".codex", "config.toml"), []byte(selector(repo, false)), 0600)
	untrusted := codexListResult(t, fx, 0)
	if codexNamed(t, untrusted, "user-choice").Availability.State != "disabled" || codexNamed(t, untrusted, "repo-choice").Availability.State != "available" {
		t.Fatal("untrusted config changed permission")
	}
	ignored := false
	for _, layer := range untrusted.Scope.Agent.ConfigLayers {
		ignored = ignored || layer.State == "ignored"
	}
	if !ignored {
		t.Fatal("ignored layer absent")
	}
	writeFile(t, config, []byte(selector(user, false)+trustProject(root)), 0600)
	writeFile(t, filepath.Join(fx.project, ".codex", "config.toml"), []byte(selector(leaf, false)), 0600)
	closest := codexListResult(t, fx, 0)
	for _, name := range []string{"user-choice", "repo-choice"} {
		if codexNamed(t, closest, name).Availability.State != "available" {
			t.Fatal("lower arrays were merged")
		}
	}
	if codexNamed(t, closest, "leaf-choice").Availability.State != "disabled" {
		t.Fatal("closest project array not effective")
	}
	replaced := 0
	for _, layer := range closest.Scope.Agent.ConfigLayers {
		if layer.State == "replaced" {
			replaced++
		}
	}
	if replaced != 2 {
		t.Fatalf("replaced=%d", replaced)
	}
	writeFile(t, filepath.Join(fx.project, ".codex", "config.toml"), []byte(selector(filepath.Dir(leaf), false)), 0600)
	if codexNamed(t, codexListResult(t, fx, 0), "leaf-choice").Availability.State != "available" {
		t.Fatal("folder selector disabled skill")
	}
	writeFile(t, filepath.Join(fx.project, ".codex", "config.toml"), []byte("[skills]\nconfig=[]\n"), 0600)
	for _, source := range codexListResult(t, fx, 0).Sources {
		if source.Availability.State == "disabled" {
			t.Fatal("empty array failed to replace")
		}
	}
	writeFile(t, config, []byte("skills.config = [\nSECRET_NOT_PRINTED"), 0600)
	malformed := codexListResult(t, fx, 1)
	if codexNamed(t, malformed, "user-choice").Availability.State != "unknown" {
		t.Fatal("invalid config defaulted enabled")
	}
	result := fx.run(t, "list", "--agent", "codex", "--project", fx.project)
	if strings.Contains(string(result.stdout)+string(result.stderr), "SECRET_NOT_PRINTED") {
		t.Fatal("printed malformed config contents")
	}
	missing := fx.run(t, "list", "--agent", "codex", "--project", filepath.Join(fx.base, "missing"), "--json")
	assertExit(t, missing, 1)
	var missingList codexList
	decodeJSON(t, missing.stdout, &missingList)
	codexNamed(t, missingList, "user-choice")
	usage := fx.run(t, "list", "--agent", "codex", "--availability", "maybe")
	assertExit(t, usage, 2)
}
func TestALE26InvocationEligibilityAndManagedSources(t *testing.T) {
	fx, root := codexFixture(t)
	global := filepath.Join(fx.home, ".agents", "skills")
	explicit := codexSkill(t, global, "explicit", "explicit")
	writeFile(t, filepath.Join(filepath.Dir(explicit), "agents", "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0600)
	codexSkill(t, global, ".hidden/hidden", "hidden")
	linked := codexSkill(t, global, "manifest-link", "manifest-link")
	if err := os.Remove(linked); err != nil {
		t.Fatal(err)
	}
	external := codexSkill(t, filepath.Join(fx.base, "allowed", "external"), "file", "external")
	if err := os.Symlink(external, linked); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(global, "missing-description", "SKILL.md"), []byte("---\nname: missing-description\n---\nBody\n"), 0600)
	pluginSource := filepath.Join(root, "plugins", "sample")
	codexSkill(t, filepath.Join(pluginSource, "skills"), "skill", "plugin-skill")
	manifest := []byte(`{"name":"sample","version":"1.0.0","skills":"./skills/"}`)
	writeFile(t, filepath.Join(pluginSource, ".codex-plugin", "plugin.json"), manifest, 0600)
	writeFile(t, filepath.Join(root, ".agents", "plugins", "marketplace.json"), []byte(`{"name":"local","plugins":[{"name":"sample","source":{"source":"local","path":"./plugins/sample"}}]}`), 0600)
	cache := filepath.Join(fx.home, ".codex", "plugins", "cache", "local", "sample", "1.0.0")
	codexSkill(t, filepath.Join(cache, "skills"), "skill", "cached-plugin")
	writeFile(t, filepath.Join(cache, ".codex-plugin", "plugin.json"), manifest, 0600)
	stale := filepath.Join(fx.home, ".codex", "plugins", "cache", "local", "sample", "0.9.0")
	codexSkill(t, filepath.Join(stale, "skills"), "skill", "stale-plugin")
	writeFile(t, filepath.Join(stale, ".codex-plugin", "plugin.json"), []byte(`{"name":"sample","version":"0.9.0","skills":"./skills/"}`), 0600)
	codexSkill(t, filepath.Join(fx.home, ".codex", "plugins", "cache", "orphan", "orphan", "old", "skills"), "skill", "orphan-cache")

	config := filepath.Join(fx.home, ".codex", "config.toml")
	writeFile(t, config, []byte(trustProject(root)+"[plugins.'sample@local']\nenabled=true\n"), 0600)
	before := treeManifest(t, fx.base)
	list := codexListResult(t, fx, 0)
	for _, source := range list.Sources {
		if source.Name == "hidden" {
			t.Fatal("native hidden subtree loaded")
		}
	}
	if codexNamed(t, list, "explicit").Availability.State != "explicit-only" {
		t.Fatal("explicit policy lost")
	}
	for _, name := range []string{"external", "missing-description", "stale-plugin"} {
		if codexNamed(t, list, name).Availability.State != "unavailable" {
			t.Fatalf("%s not excluded", name)
		}
	}
	if codexNamed(t, list, "orphan-cache").Availability.State != "unknown" {
		t.Fatal("orphan activation invented")
	}
	plugin := codexNamed(t, list, "cached-plugin")
	if plugin.Availability.State != "available" || plugin.Availability.References[0].Provenance.PluginID != "sample@local" {
		t.Fatalf("plugin: %+v", plugin)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, config, []byte(trustProject(root)+"[plugins.'sample@local']\nenabled=false\n"), 0600)
	if codexNamed(t, codexListResult(t, fx, 0), "cached-plugin").Availability.State != "unavailable" {
		t.Fatal("inactive cache still effective")
	}
	writeFile(t, config, []byte(trustProject(root)+"[plugins.'sample@local']\nenabled=true\n"+selector(filepath.Join(cache, "skills", "skill", "SKILL.md"), false)), 0600)
	if codexNamed(t, codexListResult(t, fx, 0), "cached-plugin").Availability.State != "disabled" {
		t.Fatal("plugin skill selector not applied")
	}
}

func TestALE26NativeInputFailuresAreBoundedAndPrivate(t *testing.T) {
	for _, kind := range []string{"fifo", "oversized", "broken-link", "invalid-policy"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			manifest := codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "safe", "safe")
			config := filepath.Join(fx.home, ".codex", "config.toml")
			switch kind {
			case "fifo":
				if err := syscall.Mkfifo(config, 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				writeFile(t, config, []byte(strings.Repeat("#", (4<<20)+1)), 0600)
			case "broken-link":
				if err := os.Symlink(filepath.Join(fx.base, "missing-config"), config); err != nil {
					t.Fatal(err)
				}
			case "invalid-policy":
				writeFile(t, filepath.Join(filepath.Dir(manifest), "agents", "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: 'false'\nprivate: UNRELATED_POLICY_SECRET\n"), 0600)
			}
			result := fx.run(t, "list", "--agent", "codex", "--project", fx.project, "--json")
			assertExit(t, result, 1)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			if codexNamed(t, list, "safe").Availability.State != "unknown" {
				t.Fatal("bad input promoted to available")
			}
			if strings.Contains(string(result.stdout), "UNRELATED_POLICY_SECRET") {
				t.Fatal("leaked private policy contents")
			}
		})
	}
	fx, root := codexFixture(t)
	hostile := codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "hostile-\x1b[2J-END", "hostile-selector")
	writeFile(t, hostile, []byte("---\nname: hostile-selector\ndescription: Valid native fixture.\n---\nBody\n"), 0600)
	config := filepath.Join(fx.home, ".codex", "config.toml")
	writeFile(t, config, []byte(selector(hostile, false)+trustProject(root)+"[model_providers.private]\nexperimental_api_key='UNRELATED_CONFIG_SECRET'\n"), 0600)
	list := codexListResult(t, fx, 0)
	source := codexNamed(t, list, "hostile-selector")
	for _, operation := range []string{"list", "explain"} {
		args := []string{operation, "--agent", "codex", "--project", fx.project}
		if operation == "explain" {
			args = append(args, source.ID)
		}
		result := fx.run(t, args...)
		assertExit(t, result, 0)
		output := string(result.stdout) + string(result.stderr)
		if strings.Contains(output, "\x1b") || strings.Contains(output, "UNRELATED_CONFIG_SECRET") {
			t.Fatal("unsafe human configuration rendering")
		}
	}
	jsonResult := fx.run(t, "explain", "--agent", "codex", "--project", fx.project, "--json", source.ID)
	assertExit(t, jsonResult, 0)
	if strings.Contains(string(jsonResult.stdout), "UNRELATED_CONFIG_SECRET") {
		t.Fatal("exposed unrelated config in JSON")
	}
	fx.project = filepath.Join(fx.base, "allowed", "non-git")
	if err := os.MkdirAll(filepath.Join(fx.project, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	codexSkill(t, filepath.Join(fx.project, ".agents", "skills"), "unbounded", "no-git-context")
	noGit := codexListResult(t, fx, 1)
	if codexNamed(t, noGit, "no-git-context").Availability.State != "unknown" {
		t.Fatal("empty .git marker was treated as a repository")
	}
	// The original native config remains intact after inspection.
	if after, err := os.ReadFile(config); err != nil || !strings.Contains(string(after), "UNRELATED_CONFIG_SECRET") {
		t.Fatal("inspection rewrote configuration")
	}
}
