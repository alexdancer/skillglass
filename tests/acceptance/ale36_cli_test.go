package acceptance_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func opencodeChange(t *testing.T, fx fixture, scope, choice, id, token string, want int) changeReceipt {
	t.Helper()
	args := []string{choice, "--agent", "opencode", "--scope", scope, "--project", fx.project, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	args = append(args, id)
	r := changeCommand(t, fx, "", args...)
	assertExit(t, r, want)
	var out changeReceipt
	decodeJSON(t, r.stdout, &out)
	return out
}
func opencodeApply(t *testing.T, fx fixture, scope, choice, id string) changeReceipt {
	t.Helper()
	before := treeManifest(t, fx.base)
	p := opencodeChange(t, fx, scope, choice, id, "", 0)
	if p.State != "ready" || p.Confirmation == "" {
		t.Fatal(p)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	out := opencodeChange(t, fx, scope, choice, id, p.Confirmation, 0)
	assertSavedChange(t, out)
	return out
}
func opencodeTarget(t *testing.T) (fixture, string, string) {
	t.Helper()
	fx, _ := codexFixture(t)
	path := codexSkill(t, filepath.Join(fx.xdgConfig, "opencode/skills"), "target", "target")
	id := codexNamed(t, opencodeListResult(t, fx, 0), "target").ID
	return fx, path, id
}

func TestALE36GlobalProjectInheritPreserveAndIdempotence(t *testing.T) {
	fx, target, id := opencodeTarget(t)
	config := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
	project := filepath.Join(fx.project, "opencode.json")
	raw := []byte("{ // top , comment\n\"permission\": {\"bash\":\"ask\",\"skill\":{\"*\":\"allow\",/* rule , */\"sibling\":\"ask\",},},\n\"other\":{\"number\":1e3,\"escaped\":\"// /*\"},}\n")
	writeFile(t, config, raw, 0600)
	codexSkill(t, filepath.Join(fx.xdgConfig, "opencode/skills"), "sibling", "sibling")
	other := filepath.Join(fx.home, ".codex/config.toml")
	writeFile(t, other, []byte("sentinel\n"), 0600)
	body := readBytes(t, target)
	opencodeApply(t, fx, "global", "disable", id)
	saved := readBytes(t, config)
	for _, needle := range []string{"// top , comment", "/* rule , */", `"sibling":"ask"`, `"number":1e3`, `"bash":"ask"`} {
		if !bytes.Contains(saved, []byte(needle)) {
			t.Fatal("unrelated bytes lost", string(saved))
		}
	}
	p := opencodeChange(t, fx, "global", "disable", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal(p)
	}
	before := treeManifest(t, fx.base)
	opencodeChange(t, fx, "global", "disable", id, p.Confirmation, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	opencodeApply(t, fx, "project", "enable", id)
	if !bytes.Equal(saved, readBytes(t, config)) {
		t.Fatal("project changed global")
	}
	opencodeApply(t, fx, "project", "inherit", id)
	if codexNamed(t, opencodeListResult(t, fx, 0), "target").Availability.State != "disabled" {
		t.Fatal("inherit lost global deny")
	}
	opencodeApply(t, fx, "global", "enable", id)
	opencodeApply(t, fx, "project", "disable", id)
	refused := opencodeChange(t, fx, "global", "enable", id, "", 1)
	if refused.State != "refused" || !strings.Contains(refused.Reason, "higher-priority") {
		t.Fatal(refused)
	}
	opencodeApply(t, fx, "project", "inherit", id)
	if strings.Contains(string(readBytes(t, project)), `"target"`) {
		t.Fatal("exact project rule remains")
	}
	if !bytes.Equal(body, readBytes(t, target)) || string(readBytes(t, other)) != "sentinel\n" {
		t.Fatal("unrelated state changed")
	}
}
func TestALE36AskPreservedAndInherited(t *testing.T) {
	fx, _, id := opencodeTarget(t)
	global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
	project := filepath.Join(fx.project, "opencode.jsonc")
	opencodeConfig(t, global, `{"permission":{"skill":{"*":"ask","other":"deny"}}}`)
	before := treeManifest(t, fx.base)
	p := opencodeChange(t, fx, "global", "enable", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal("ask weakened", p)
	}
	opencodeChange(t, fx, "global", "enable", id, p.Confirmation, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	opencodeConfig(t, project, "{\"permission\":{\"skill\":{/*a,*/\"target\":\"deny\",/*b,*/\"other-project\":\"ask\",}},}\n")
	opencodeApply(t, fx, "project", "inherit", id)
	if codexNamed(t, opencodeListResult(t, fx, 0), "target").Availability.State != "ask" {
		t.Fatal("ask inheritance flattened")
	}
	saved := string(readBytes(t, project))
	if !strings.Contains(saved, "/*a,*/") || !strings.Contains(saved, "/*b,*/") {
		t.Fatal("comments deleted", saved)
	}
	opencodeApply(t, fx, "global", "disable", id)
	opencodeApply(t, fx, "global", "enable", id)
	if strings.Contains(string(readBytes(t, global)), `"target":"ask"`) {
		t.Fatal("historical ask invented")
	}
}
func TestALE36ScalarAndOrderedRuleSafety(t *testing.T) {
	for _, kind := range []string{"scalar-skill", "scalar-permission", "resurrection", "existing-position", "tool-wildcard", "integer-keys"} {
		t.Run(kind, func(t *testing.T) {
			fx, _, id := opencodeTarget(t)
			global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
			project := filepath.Join(fx.project, "opencode.json")
			scope, choice := "global", "disable"
			want := 0
			switch kind {
			case "scalar-skill":
				opencodeConfig(t, global, `{"permission":{"skill":"ask"}}`)
			case "scalar-permission":
				opencodeConfig(t, global, `{"permission":"ask"}`)
			case "resurrection":
				opencodeConfig(t, global, `{"permission":{"skill":{"*":"allow","undiscovered":"deny"}}}`)
				opencodeConfig(t, project, `{"permission":{"skill":"ask"}}`)
				scope = "project"
				want = 1
			case "existing-position":
				opencodeConfig(t, global, `{"permission":{"skill":{"target":"deny","*":"deny"}}}`)
				scope = "project"
				choice = "enable"
				want = 1
			case "tool-wildcard":
				opencodeConfig(t, global, `{"permission":{"skill":{"target":"allow"},"s*":"allow"}}`)
				want = 1
			case "integer-keys":
				opencodeConfig(t, global, `{"permission":{"skill":{"42":"deny","2":"ask","*":"allow"}}}`)
			}
			before := treeManifest(t, fx.base)
			p := opencodeChange(t, fx, scope, choice, id, "", want)
			if want == 1 {
				if p.State != "refused" {
					t.Fatal(p)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			out := opencodeChange(t, fx, scope, choice, id, p.Confirmation, 0)
			assertSavedChange(t, out)
		})
	}
}
func TestALE36RefuseUnknownAmbiguousAndUnsupported(t *testing.T) {
	for _, kind := range []string{"duplicate", "wildcard", "builtin", "managed", "inspection", "project-only", "no-project", "project-off", "duplicate-json", "malformed", "null", "surrogate", "substitution", "plugin", "multiple-overrides", "nested-override", "alias-output", "missing-alias-output", "hardlink", "source-output"} {
		t.Run(kind, func(t *testing.T) {
			fx, _, id := opencodeTarget(t)
			global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
			scope, choice := "global", "disable"
			switch kind {
			case "duplicate":
				codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "other", "target")
			case "wildcard", "builtin":
				name := "target*"
				if kind == "builtin" {
					name = "customize-opencode"
				}
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode/skills"), "extra", name)
				id = codexNamed(t, opencodeListResult(t, fx, 0), name).ID
			case "managed":
				path := codexSkill(t, filepath.Join(fx.xdgCache, "opencode/skills"), "cache", "cache")
				alias := filepath.Join(fx.xdgConfig, "opencode/skills/cache")
				if e := os.Symlink(filepath.Dir(path), alias); e != nil {
					t.Fatal(e)
				}
				id = codexNamed(t, opencodeListResult(t, fx, 0), "cache").ID
			case "inspection":
				codexSkill(t, fx.cleanRoot, "inspection", "inspection")
				r := fx.run(t, "list", "--agent", "opencode", "--root", fx.cleanRoot, "--project", fx.project, "--json")
				assertExit(t, r, 0)
				var list codexList
				decodeJSON(t, r.stdout, &list)
				id = codexNamed(t, list, "inspection").ID
			case "project-only":
				codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "local", "local")
				id = codexNamed(t, opencodeListResult(t, fx, 0), "local").ID
			case "no-project":
				choice = "inherit"
			case "project-off":
				scope = "project"
			case "duplicate-json":
				opencodeConfig(t, global, `{"permission":{},"permission":{}}`)
			case "malformed":
				opencodeConfig(t, global, `{"permission":`)
			case "null":
				opencodeConfig(t, global, `null`)
			case "surrogate":
				opencodeConfig(t, global, `{"other":"\ud800"}`)
			case "substitution":
				opencodeConfig(t, global, `{"other":"{env:SECRET}"}`)
			case "plugin":
				opencodeConfig(t, global, `{"plugin":["never-executed"]}`)
			case "multiple-overrides":
				scope = "project"
				choice = "inherit"
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":{"target":"deny"}}}`)
				opencodeConfig(t, filepath.Join(fx.project, "opencode.jsonc"), `{"permission":{"skill":{"target":"ask"}}}`)
			case "nested-override":
				scope = "project"
				choice = "inherit"
				opencodeConfig(t, filepath.Join(fx.project, ".opencode/opencode.json"), `{"permission":{"skill":{"target":"deny"}}}`)
			case "alias-output", "missing-alias-output":
				scope = "project"
				if kind == "alias-output" {
					opencodeConfig(t, global, `{}`)
				}
				alias := filepath.Join(fx.base, "allowed/global-alias")
				if e := os.Symlink(filepath.Dir(global), alias); e != nil {
					t.Fatal(e)
				}
				fx.project = alias
			case "source-output":
				scope = "project"
				fx.project = filepath.Join(fx.xdgConfig, "opencode/skills/target")
			case "hardlink":
				opencodeConfig(t, global, `{}`)
				if e := os.Link(global, filepath.Join(fx.base, "allowed/config-alias")); e != nil {
					t.Fatal(e)
				}
			}
			before := treeManifest(t, fx.base)
			if kind == "project-off" {
				r := opencodeEnvRun(t, fx, []string{"OPENCODE_DISABLE_PROJECT_CONFIG=true"}, "disable", "--agent", "opencode", "--scope", "project", "--project", fx.project, "--json", id)
				assertExit(t, r, 1)
			} else {
				p := opencodeChange(t, fx, scope, choice, id, "", 1)
				if p.State != "refused" && p.State != "unsupported" {
					t.Fatal(p)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE36HigherPriorityAndStaleInputs(t *testing.T) {
	for _, kind := range []string{"agent", "inline", "runtime", "managed", "native-directory", "external", "source", "identity", "write"} {
		t.Run(kind, func(t *testing.T) {
			fx, target, id := opencodeTarget(t)
			global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
			opencodeConfig(t, global, `{"keep":1}`)
			env := []string{}
			blocked := true
			switch kind {
			case "agent":
				opencodeConfig(t, global, `{"agent":{"build":{"tools":{"skill":true}}}}`)
			case "inline":
				env = []string{`OPENCODE_CONFIG_CONTENT={"permission":{"skill":{"target":"allow"}}}`}
			case "runtime":
				env = []string{`OPENCODE_PERMISSION={"skill":{"target":"allow"}}`}
			case "managed":
				dir := filepath.Join(fx.base, "allowed/managed")
				opencodeConfig(t, filepath.Join(dir, "opencode.json"), `{"permission":{"skill":{"target":"allow"}}}`)
				env = []string{"OPENCODE_TEST_MANAGED_CONFIG_DIR=" + dir}
			case "native-directory":
				opencodeConfig(t, filepath.Join(fx.project, ".opencode/opencode.json"), `{"permission":{"skill":{"target":"allow"}}}`)
			default:
				blocked = false
			}
			if blocked {
				before := treeManifest(t, fx.base)
				r := opencodeEnvRun(t, fx, env, "disable", "--agent", "opencode", "--scope", "global", "--project", fx.project, "--json", id)
				assertExit(t, r, 1)
				var p changeReceipt
				decodeJSON(t, r.stdout, &p)
				if p.State != "refused" {
					t.Fatal(p)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			p := opencodeChange(t, fx, "global", "disable", id, "", 0)
			switch kind {
			case "external":
				opencodeConfig(t, global, `{"keep":2}`)
			case "source":
				writeFile(t, target, append(readBytes(t, target), []byte("External\n")...), 0600)
			case "identity":
				old := readBytes(t, global)
				if e := os.Remove(global); e != nil {
					t.Fatal(e)
				}
				writeFile(t, global, old, 0600)
			}
			before := treeManifest(t, fx.base)
			if kind == "write" {
				args := []string{"disable", "--agent", "opencode", "--scope", "global", "--project", fx.project, "--json", "--confirm", p.Confirmation, id}
				r := changeCommand(t, fx, "(deny file-write*)", args...)
				assertExit(t, r, 1)
				var out changeReceipt
				decodeJSON(t, r.stdout, &out)
				if out.State != "failed" || out.Persistence != "not-written" {
					t.Fatal(out)
				}
			} else {
				out := opencodeChange(t, fx, "global", "disable", id, p.Confirmation, 1)
				if out.State != "refused" {
					t.Fatal(out)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE36RepeatedNativeRoleSharedSourceAndVirtualStaleness(t *testing.T) {
	fx, target, id := opencodeTarget(t)
	alias := filepath.Join(fx.home, ".agents/skills/shared")
	if e := os.MkdirAll(filepath.Dir(alias), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Dir(target), alias); e != nil {
		t.Fatal(e)
	}
	globalDir := filepath.Join(fx.xdgConfig, "opencode")
	run := func(env []string, token string, want int) changeReceipt {
		args := []string{"disable", "--agent", "opencode", "--scope", "global", "--project", fx.project, "--json"}
		if token != "" {
			args = append(args, "--confirm", token)
		}
		args = append(args, id)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		argv := fx.launchArgs(t, skillglassBinary, true, "", args...)
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir, cmd.Env = fx.project, append(fx.environment(), env...)
		r, _ := executeFinalizedCommand(t, cmd)
		recordEvidenceCommand(t, "opencode-mutation-with-native-environment", r)
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		assertExit(t, r, want)
		var receipt changeReceipt
		decodeJSON(t, r.stdout, &receipt)
		return receipt
	}
	env := []string{"OPENCODE_CONFIG_DIR=" + globalDir}
	p := run(env, "", 0)
	assertSavedChange(t, run(env, p.Confirmation, 0))
	if codexNamed(t, opencodeListResult(t, fx, 0), "target").Availability.State != "disabled" {
		t.Fatal("repeat role overlay did not persist")
	}
	opencodeApply(t, fx, "global", "enable", id)
	p = run([]string{`OPENCODE_CONFIG_CONTENT={"theme":"before"}`}, "", 0)
	before := treeManifest(t, fx.base)
	stale := run([]string{`OPENCODE_CONFIG_CONTENT={"theme":"after"}`}, p.Confirmation, 1)
	if stale.State != "refused" {
		t.Fatal(stale)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// A project chosen at HOME/.opencode is also a global native directory.
	fx.project = filepath.Join(fx.home, ".opencode")
	if e := os.MkdirAll(fx.project, 0700); e != nil {
		t.Fatal(e)
	}
	before = treeManifest(t, fx.base)
	out := opencodeChange(t, fx, "project", "disable", id, "", 1)
	if out.State != "refused" {
		t.Fatal(out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE36MissingConfigDirectoryAndProjectIsolation(t *testing.T) {
	fx, _ := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	id := codexNamed(t, opencodeListResult(t, fx, 0), "target").ID
	global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
	preview := opencodeChange(t, fx, "global", "disable", id, "", 0)
	if preview.CreateDirectory != filepath.Dir(global) {
		t.Fatal("missing directory not disclosed", preview)
	}
	assertSavedChange(t, opencodeChange(t, fx, "global", "disable", id, preview.Confirmation, 0))
	saved := readBytes(t, global)
	second := fx
	second.project = filepath.Join(fx.base, "allowed/second-project")
	writeFile(t, filepath.Join(second.project, ".git/marker"), []byte("fixture"), 0600)
	opencodeApply(t, fx, "project", "enable", id)
	if codexNamed(t, opencodeListResult(t, fx, 0), "target").Availability.State != "available" || codexNamed(t, opencodeListResult(t, second, 0), "target").Availability.State != "disabled" {
		t.Fatal("project choice escaped its selected context")
	}
	if !bytes.Equal(saved, readBytes(t, global)) {
		t.Fatal("project choice rewrote global config")
	}
	before := treeManifest(t, fx.base)
	p := opencodeChange(t, second, "project", "inherit", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal("missing exact override was not a no-op", p)
	}
	assertSavedChange(t, opencodeChange(t, second, "project", "inherit", id, p.Confirmation, 0))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// Project-only content has a supported local control and no global fallback.
	local := codexSkill(t, filepath.Join(fx.project, ".opencode/skills"), "local", "local")
	localID := codexNamed(t, opencodeListResult(t, fx, 0), "local").ID
	body := readBytes(t, local)
	opencodeApply(t, fx, "project", "disable", localID)
	opencodeApply(t, fx, "project", "inherit", localID)
	if codexNamed(t, opencodeListResult(t, fx, 0), "local").Availability.State != "available" || !bytes.Equal(body, readBytes(t, local)) {
		t.Fatal("project-only inheritance or content changed")
	}
	before = treeManifest(t, fx.base)
	result := changeCommand(t, fx, "", "disable", "--agent", "opencode", "--scope", "project", "--json", id)
	assertExit(t, result, 1)
	var refused changeReceipt
	decodeJSON(t, result.stdout, &refused)
	if refused.State != "unsupported" || !strings.Contains(refused.Reason, "explicit project") {
		t.Fatal(refused)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if !bytes.Contains(readBytes(t, target), []byte("Inert instructions")) {
		t.Fatal("source content changed")
	}
}

func TestALE36AskEnableWithMissingConfigurationDirectory(t *testing.T) {
	fx, _ := codexFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	id := codexNamed(t, opencodeListResult(t, fx, 0), "target").ID
	before := treeManifest(t, fx.base)
	args := []string{"enable", "--agent", "opencode", "--scope", "global", "--project", fx.project, "--json"}
	env := []string{`OPENCODE_PERMISSION={"skill":"ask"}`}
	result := opencodeEnvRun(t, fx, env, append(args, id)...)
	assertExit(t, result, 0)
	var preview changeReceipt
	decodeJSON(t, result.stdout, &preview)
	if !preview.NoWriteNeeded || preview.CreateDirectory != "" {
		t.Fatal("effective ask should not create missing configuration", preview)
	}
	args = append(args, "--confirm", preview.Confirmation, id)
	result = opencodeEnvRun(t, fx, env, args...)
	assertExit(t, result, 0)
	var out changeReceipt
	decodeJSON(t, result.stdout, &out)
	assertSavedChange(t, out)
	var facts map[string]any
	decodeJSON(t, result.stdout, &facts)
	if _, exists := facts["persisted_choice"]; exists {
		t.Fatal("no-write ask invented an exact saved choice", facts)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
