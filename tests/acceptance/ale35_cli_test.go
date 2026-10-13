package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func opencodeListResult(t *testing.T, fx fixture, exit int, extra ...string) codexList {
	t.Helper()
	args := append([]string{"list", "--agent", "opencode", "--project", fx.project, "--json"}, extra...)
	result := fx.run(t, args...)
	assertExit(t, result, exit)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	return list
}
func opencodeConfig(t *testing.T, path, text string) {
	t.Helper()
	writeFile(t, path, []byte(text), 0600)
}
func TestALE35OpenCodeDiscoverySharingPermissionAndExplain(t *testing.T) {
	fx, repo := codexFixture(t)
	native := filepath.Join(fx.xdgConfig, "opencode", "skills")
	for _, root := range []string{filepath.Join(fx.home, ".claude", "skills"), filepath.Join(fx.home, ".agents", "skills"), native, filepath.Join(repo, ".opencode", "skills"), filepath.Join(fx.project, ".agents", "skills")} {
		codexSkill(t, root, "collision", "collision")
	}
	codexSkill(t, filepath.Join(filepath.Dir(repo), ".agents", "skills"), "outside", "outside-repo")
	for _, name := range []string{"allowed", "ask-me", "denied"} {
		codexSkill(t, native, name, name)
	}
	shared := codexSkill(t, filepath.Join(fx.base, "allowed", "shared"), "source", "shared")
	for _, alias := range []string{filepath.Join(fx.home, ".agents", "skills", "alias"), filepath.Join(repo, ".opencode", "skills", "alias")} {
		if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
			t.Fatal(err)
		}
	}
	codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), ".hidden/deep", "external-hidden")
	codexSkill(t, native, ".hidden/deep", "native-hidden")
	codexSkill(t, native, "parent", "parent")
	codexSkill(t, native, "parent/nested", "nested-bundle")
	codexSkill(t, filepath.Join(fx.home, ".opencode", "skill"), "singular", "home-native")
	codexSkill(t, fx.cleanRoot, "inspection", "inspection-only")
	configured := filepath.Join(fx.base, "allowed", "configured")
	codexSkill(t, configured, "deep/nested", "configured")
	opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"skills":{"paths":["`+filepath.ToSlash(configured)+`"]},"permission":{"skill":{"*":"allow","ask-*":"ask","denied":"deny"}}}`)
	before := treeManifest(t, fx.base)
	list := opencodeListResult(t, fx, 0, "--root", fx.cleanRoot)
	if list.Scope.Agent.Application != "opencode" || list.Scope.Agent.ValidatedVersion != "OpenCode 1.17.15" {
		t.Fatalf("scope=%+v", list.Scope)
	}
	states := map[string]string{"allowed": "available", "ask-me": "ask", "denied": "disabled", "shared": "available", "external-hidden": "available", "parent": "available", "nested-bundle": "available", "home-native": "available", "configured": "available", "inspection-only": "unknown"}
	for name, state := range states {
		source := codexNamed(t, list, name)
		if source.Availability.State != state {
			t.Fatalf("%s=%+v", name, source.Availability)
		}
	}
	count := 0
	for _, source := range list.Sources {
		if source.Name == "outside-repo" || source.Name == "native-hidden" {
			t.Fatalf("unexpected source: %s", source.Name)
		}
		if source.Name == "collision" {
			count++
			if source.Availability.State != "unknown" || source.Availability.Precedence.State != "unknown" || source.Availability.Precedence.Winner != "" || len(source.Availability.Precedence.RelatedSources) != 4 {
				t.Fatalf("collision=%+v", source)
			}
		}
	}
	if count != 5 {
		t.Fatalf("copies=%d", count)
	}
	s := codexNamed(t, list, "shared")
	if len(s.Availability.References) != 2 {
		t.Fatalf("aliases=%+v", s)
	}
	denied := codexNamed(t, list, "denied")
	if denied.Availability.References[0].Permission.State != "denied" {
		t.Fatal("deny distinction lost")
	}
	ask := codexNamed(t, list, "ask-me")
	if ask.Availability.References[0].Permission.State != "ask" {
		t.Fatal("ask flattened")
	}
	filtered := opencodeListResult(t, fx, 0, "--availability", "ask")
	if len(filtered.Sources) != 1 || !reflect.DeepEqual(filtered.Sources[0].Availability, ask.Availability) {
		t.Fatal("filter changed native explanation")
	}
	result := fx.run(t, "explain", "--agent", "opencode", "--project", fx.project, "--json", ask.ID)
	assertExit(t, result, 0)
	var detail struct{ Source codexSource }
	decodeJSON(t, result.stdout, &detail)
	if !reflect.DeepEqual(detail.Source.Availability, ask.Availability) {
		t.Fatal("explain differs")
	}
	human := fx.run(t, "explain", "--agent", "opencode", "--project", fx.project, ask.ID)
	assertExit(t, human, 0)
	for _, needle := range []string{"Permission: ask (inferred)", "ordered-skill-permission", "permission.skill.ask-*", "Session: unknown", "unsupported"} {
		if !strings.Contains(string(human.stdout), needle) {
			t.Fatalf("missing %q", needle)
		}
	}
	for _, cap := range ask.Availability.Capabilities {
		if cap.State != "unsupported" {
			t.Fatal("enabled writer")
		}
	}
	global := fx.run(t, "list", "--agent", "opencode", "--json")
	assertExit(t, global, 0)
	var globals codexList
	decodeJSON(t, global.stdout, &globals)
	for _, ref := range globals.References {
		if ref.Context == "project" {
			t.Fatal("implicit cwd selection")
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE35OpenCodeLayerOrderAndNativeContext(t *testing.T) {
	fx, repo := codexFixture(t)
	root := filepath.Join(fx.xdgConfig, "opencode", "skills")
	codexSkill(t, root, "allowed", "allowed")
	codexSkill(t, root, "item", "item")
	global := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
	opencodeConfig(t, global, `{"permission":{"skill":{"*":"allow","i*":"deny"}}}`)
	opencodeConfig(t, filepath.Join(repo, "opencode.json"), `{"permission":{"skill":{"i*":"ask","item":"deny"}}}`)
	opencodeConfig(t, filepath.Join(fx.project, "opencode.jsonc"), "{ // comment\n\"permission\":{\"skill\":{\"item\":\"allow\",},},}\n")
	list := opencodeListResult(t, fx, 0)
	if codexNamed(t, list, "item").Availability.State != "available" {
		t.Fatal("project merge/JSONC failed")
	}
	opencodeConfig(t, filepath.Join(fx.project, ".opencode", "opencode.json"), `{"permission":{"skill":{"item":"ask"}}}`)
	opencodeConfig(t, filepath.Join(repo, ".opencode", "opencode.json"), `{"permission":{"skill":{"item":"deny"}}}`)
	if codexNamed(t, opencodeListResult(t, fx, 0), "item").Availability.State != "disabled" {
		t.Fatal("native directory order should be nearest then root")
	}
	opencodeConfig(t, filepath.Join(repo, ".opencode", "opencode.json"), `{"agent":{"build":{"permission":{"skill":{"item":"ask"}}}}}`)
	if codexNamed(t, opencodeListResult(t, fx, 0), "item").Availability.State != "ask" {
		t.Fatal("agent override did not append after user rules")
	}
	opencodeConfig(t, filepath.Join(repo, ".opencode", "opencode.json"), `{"default_agent":"plan","agent":{"plan":{"permission":{"skill":{"item":"allow"}}}}}`)
	if codexNamed(t, opencodeListResult(t, fx, 0), "item").Availability.State != "available" {
		t.Fatal("configured default agent not selected")
	}
	opencodeConfig(t, filepath.Join(repo, ".opencode", "opencode.json"), `{"agent":{"build":{"tools":{"skill":false}}}}`)
	if codexNamed(t, opencodeListResult(t, fx, 0), "item").Availability.State != "disabled" {
		t.Fatal("legacy agent tools not normalized")
	}
	opencodeConfig(t, global, `{"permission":"deny"}`)
	opencodeConfig(t, filepath.Join(repo, ".opencode", "opencode.json"), `{}`)
	if codexNamed(t, opencodeListResult(t, fx, 0), "allowed").Availability.State != "disabled" {
		t.Fatal("top-level permission action not normalized")
	}
}
func TestALE35OpenCodeOfflineCacheInputsAndImmutability(t *testing.T) {
	fx, _ := codexFixture(t)
	native := filepath.Join(fx.xdgConfig, "opencode", "skills")
	manifest := codexSkill(t, native, "safe", "safe")
	opencodeConfig(t, filepath.Join(native, "explicit", "SKILL.md"), "---\nname: explicit\n---\nInert body\n")
	cached := codexSkill(t, filepath.Join(fx.xdgCache, "opencode", "skills"), "stale", "stale-cache")
	alias := filepath.Join(fx.home, ".agents", "skills", "cache-alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(cached), alias); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	list := opencodeListResult(t, fx, 0)
	if codexNamed(t, list, "explicit").Availability.State != "explicit-only" {
		t.Fatal("missing native description must remain explicit-only")
	}
	cache := codexNamed(t, list, "stale-cache")
	if cache.Availability.State != "available" || cache.Ownership.State != "managed" || len(cache.Availability.References) != 2 {
		t.Fatalf("native alias independent of cache route: %+v", cache)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if codexNamed(t, opencodeListResult(t, fx, 0), "stale-cache").Availability.State != "unavailable" {
		t.Fatal("cache folder promoted without activation")
	}
	before = treeManifest(t, fx.base)
	profile := filepath.Join(fx.base, "allowed", "opencode.sb")
	writeFile(t, profile, []byte(runtimeProfile(canonicalPath(t, skillglassBinary), canonicalPath(t, filepath.Join(fx.base, "allowed")))), 0600)
	before = treeManifest(t, fx.base)
	result := runSandboxCommand(t, fx, profile, canonicalPath(t, skillglassBinary), "list", "--agent", "opencode", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	var offline codexList
	decodeJSON(t, result.stdout, &offline)
	if codexNamed(t, offline, "safe").Availability.State != "available" {
		t.Fatal("offline native browsing failed")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	config := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
	for _, text := range []string{`{"permission":{"skill":{"*":"maybe"}}}`, `{"skills":{"urls":["https://127.0.0.1:9/index"]}}`, `{"plugin":["never-executed"]}`, `{"permission":{"skill":"{env:PRIVATE}"}}`, `{"permission":{"skill":"deny","skill":"allow"}}`} {
		opencodeConfig(t, config, text)
		bad := opencodeListResult(t, fx, 1)
		if codexNamed(t, bad, "safe").Availability.State != "unknown" {
			t.Fatal("bad or dynamic input became available")
		}
	}
	opencodeConfig(t, config, `{"permission":{"skill":{"*":"allow"}}}`)
	plugin := filepath.Join(fx.xdgConfig, "opencode", "plugins", "never.js")
	writeFile(t, plugin, []byte(`throw new Error("PLUGIN_EXECUTED");`), 0600)
	before = treeManifest(t, fx.base)
	if codexNamed(t, opencodeListResult(t, fx, 1), "safe").Availability.State != "unknown" {
		t.Fatal("dynamic plugin hook not represented")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if err := os.Remove(plugin); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"fifo", "oversized", "broken"} {
		if err := os.Remove(config); err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "fifo":
			if err := syscall.Mkfifo(config, 0600); err != nil {
				t.Fatal(err)
			}
		case "oversized":
			writeFile(t, config, []byte(strings.Repeat(" ", (4<<20)+1)), 0600)
		case "broken":
			if err := os.Symlink(filepath.Join(fx.base, "absent"), config); err != nil {
				t.Fatal(err)
			}
		}
		if codexNamed(t, opencodeListResult(t, fx, 1), "safe").Availability.State != "unknown" {
			t.Fatal("unreadable native input defaulted allow")
		}
	}
	missing := fx.run(t, "list", "--agent", "opencode", "--project", filepath.Join(fx.base, "missing"), "--json")
	assertExit(t, missing, 1)
	raw, err := os.ReadFile(manifest)
	if err != nil || !strings.Contains(string(raw), "Inert instructions") {
		t.Fatal("read operation changed content")
	}
}

func opencodeEnvRun(t *testing.T, fx fixture, env []string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, skillglassBinary, args...)
	cmd.Dir = fx.project
	cmd.Env = append(fx.environment(), env...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	result := commandResult{code: code, stdout: out.Bytes(), stderr: stderr.Bytes()}
	recordEvidenceCommand(t, "opencode-env-cli", skillglassBinary, args, fx.project, result)
	return result
}
func TestALE35OpenCodeRuntimeLayersAndFlags(t *testing.T) {
	fx, repo := codexFixture(t)
	root := filepath.Join(fx.xdgConfig, "opencode", "skills")
	codexSkill(t, root, "safe", "safe")
	codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "external", "external")
	codexSkill(t, filepath.Join(fx.home, ".claude", "skills"), "claude", "claude")
	custom := filepath.Join(fx.base, "allowed", "custom")
	codexSkill(t, filepath.Join(custom, "skills"), "extra", "custom-dir")
	config := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
	opencodeConfig(t, config, `{"permission":{"skill":{"*":"deny","safe":"allow"}}}`)
	opencodeConfig(t, filepath.Join(repo, "opencode.json"), `{"permission":{"skill":{"safe":"deny"}}}`)
	opencodeConfig(t, filepath.Join(custom, "opencode.json"), `{"permission":{"skill":{"safe":"ask"}}}`)
	managed := filepath.Join(fx.base, "allowed", "managed")
	opencodeConfig(t, filepath.Join(managed, "opencode.json"), `{"permission":{"skill":{"safe":"deny"}}}`)
	run := func(env []string, exit int) codexList {
		r := opencodeEnvRun(t, fx, env, "list", "--agent", "opencode", "--project", fx.project, "--json")
		assertExit(t, r, exit)
		var list codexList
		decodeJSON(t, r.stdout, &list)
		return list
	}
	if codexNamed(t, run([]string{"OPENCODE_CONFIG_DIR=" + custom}, 0), "safe").Availability.State != "ask" {
		t.Fatal("custom dir override lost")
	}
	inline := `OPENCODE_CONFIG_CONTENT={"permission":{"skill":{"safe":"allow"}}}`
	if codexNamed(t, run([]string{"OPENCODE_CONFIG_DIR=" + custom, inline}, 0), "safe").Availability.State != "available" {
		t.Fatal("inline ordering lost")
	}
	if codexNamed(t, run([]string{inline, "OPENCODE_TEST_MANAGED_CONFIG_DIR=" + managed}, 0), "safe").Availability.State != "disabled" {
		t.Fatal("managed ordering lost")
	}
	if codexNamed(t, run([]string{inline, `OPENCODE_PERMISSION={"skill":{"safe":"ask"}}`}, 0), "safe").Availability.State != "ask" {
		t.Fatal("environment permission precedence lost")
	}
	if codexNamed(t, run([]string{"OPENCODE_DISABLE_PROJECT_CONFIG=true"}, 0), "safe").Availability.State != "available" {
		t.Fatal("project disable flag lost")
	}
	off := run([]string{"OPENCODE_DISABLE_EXTERNAL_SKILLS=true"}, 0)
	for _, name := range []string{"external", "claude"} {
		if codexNamed(t, off, name).Availability.State != "unavailable" {
			t.Fatal("external disable flag ignored")
		}
	}
	only := run([]string{"OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=true"}, 0)
	if codexNamed(t, only, "external").Availability.State != "disabled" || codexNamed(t, only, "claude").Availability.State != "unavailable" {
		t.Fatal("claude-only flag broadened")
	}
}

func TestALE35InvalidDefaultAgentRemainsUnknown(t *testing.T) {
	for _, setting := range []string{`"hidden":true`, `"mode":"subagent"`} {
		t.Run(setting, func(t *testing.T) {
			fx, _ := codexFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"default_agent":"custom","agent":{"custom":{`+setting+`}}}`)
			list := opencodeListResult(t, fx, 1)
			if codexNamed(t, list, "safe").Availability.State != "unknown" {
				t.Fatal("invalid default agent received availability")
			}
		})
	}
}

func TestALE35UnverifiedCacheActivationRemainsUnknown(t *testing.T) {
	fx, _ := codexFixture(t)
	codexSkill(t, filepath.Join(fx.xdgCache, "opencode", "skills"), "cached", "cached")
	opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"skills":{"urls":["https://127.0.0.1:9/index"]}}`)
	before := treeManifest(t, fx.base)
	list := opencodeListResult(t, fx, 1)
	if got := codexNamed(t, list, "cached").Availability.State; got != "unknown" {
		t.Fatalf("unverified native activation became %s", got)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
