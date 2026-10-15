package acceptance_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func claudeChange(t *testing.T, fx fixture, choice, scope, id, token string, want int) changeReceipt {
	t.Helper()
	args := []string{choice, "--agent", "claude-code", "--project", fx.project, "--scope", scope, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	args = append(args, id)
	result := changeCommand(t, fx, "", args...)
	assertExit(t, result, want)
	var out changeReceipt
	decodeJSON(t, result.stdout, &out)
	return out
}
func claudeTarget(t *testing.T, fx fixture) (string, string) {
	t.Helper()
	target := claudeSkill(t, filepath.Join(fx.home, ".claude/skills"), "target", "ale30-target", "")
	source := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(target))
	return source.ID, target
}
func TestALE30GlobalProjectInheritLossless(t *testing.T) {
	fx, _ := codexFixture(t)
	id, target := claudeTarget(t, fx)
	content := readBytes(t, target)
	config := filepath.Join(fx.home, ".claude/settings.json")
	project := filepath.Join(fx.project, ".claude/settings.json")
	raw := []byte("{\r\n \"number\": 1.2300e+09, \"unknown\": [true,{\"x\":\"keep\"}],\r\n \"skillOverrides\": {\"other\":\"name-only\", \"ale30-target\": \"on\"}\r\n}\r\n")
	writeFile(t, config, raw, 0640)
	writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte("# unrelated\n"), 0600)
	before := treeManifest(t, fx.base)
	preview := claudeChange(t, fx, "disable", "global", id, "", 0)
	if preview.State != "ready" {
		t.Fatalf("preview %+v", preview)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	assertSavedChange(t, claudeChange(t, fx, "disable", "global", id, preview.Confirmation, 0))
	expected := bytes.Replace(raw, []byte("\"ale30-target\": \"on\""), []byte("\"ale30-target\": \"off\""), 1)
	if !bytes.Equal(readBytes(t, config), expected) {
		t.Fatal("unrelated JSON bytes changed")
	}
	writeFile(t, project, []byte("{\"skillOverrides\":{\"other\":\"off\"},\"project\":true}\n"), 0600)
	p := claudeChange(t, fx, "enable", "project", id, "", 0)
	assertSavedChange(t, claudeChange(t, fx, "enable", "project", id, p.Confirmation, 0))
	if !bytes.Equal(readBytes(t, config), expected) {
		t.Fatal("project choice changed global settings")
	}
	p = claudeChange(t, fx, "inherit", "project", id, "", 0)
	out := claudeChange(t, fx, "inherit", "project", id, p.Confirmation, 0)
	assertSavedChange(t, out)
	if out.Effective.State != "disabled" {
		t.Fatalf("inherit should follow disabled global: %+v", out)
	}
	if string(readBytes(t, project)) != "{\"skillOverrides\":{\"other\":\"off\"},\"project\":true}\n" {
		t.Fatal("inherit failed to preserve other settings")
	}
	before = treeManifest(t, fx.base)
	p = claudeChange(t, fx, "inherit", "project", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal("inherit should be no-op")
	}
	assertSavedChange(t, claudeChange(t, fx, "inherit", "project", id, p.Confirmation, 0))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if !bytes.Equal(content, readBytes(t, target)) {
		t.Fatal("skill content changed")
	}
	if string(readBytes(t, filepath.Join(fx.home, ".codex/config.toml"))) != "# unrelated\n" {
		t.Fatal("Codex settings changed")
	}
}
func TestALE30RefusalsAndStale(t *testing.T) {
	for _, scenario := range []string{"collision", "duplicate-root", "duplicate-name", "null", "malformed", "local", "project", "symlink", "hardlink", "manifest-hardlink", "external", "same-byte-replacement", "write-denied", "flags"} {
		t.Run(scenario, func(t *testing.T) {
			fx, _ := codexFixture(t)
			id, target := claudeTarget(t, fx)
			config := filepath.Join(fx.home, ".claude/settings.json")
			raw := []byte("{\"skillOverrides\":{\"ale30-target\":\"on\"},\"untouched\":true}")
			writeFile(t, config, raw, 0600)
			switch scenario {
			case "collision":
				claudeSkill(t, filepath.Join(fx.project, ".claude/skills"), "copy", "ale30-target", "")
			case "duplicate-root":
				writeFile(t, config, []byte("{\"skillOverrides\":{},\"skillOverrides\":{}}"), 0600)
			case "duplicate-name":
				writeFile(t, config, []byte("{\"skillOverrides\":{\"ale30-target\":\"on\",\"ale30-target\":\"off\"}}"), 0600)
			case "null":
				writeFile(t, config, []byte("{\"skillOverrides\":null}"), 0600)
			case "malformed":
				writeFile(t, config, []byte("{"), 0600)
			case "local":
				writeFile(t, filepath.Join(fx.project, ".claude/settings.local.json"), []byte("{\"skillOverrides\":{\"ale30-target\":\"on\"}}"), 0600)
			case "project":
				writeFile(t, filepath.Join(fx.project, ".claude/settings.json"), []byte("{\"skillOverrides\":{\"ale30-target\":\"on\"}}"), 0600)
			case "symlink":
				other := filepath.Join(fx.home, "other.json")
				writeFile(t, other, raw, 0600)
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, config); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(config, filepath.Join(fx.home, "other.json")); err != nil {
					t.Fatal(err)
				}
			case "flags":
				command := exec.Command("/usr/bin/chflags", "uchg", config)
				command.Env = fx.environment()
				if raw, err := command.CombinedOutput(); err != nil {
					t.Fatalf("fixture flags: %s %v", raw, err)
				}
				defer func() {
					command := exec.Command("/usr/bin/chflags", "nouchg", config)
					command.Env = fx.environment()
					if raw, err := command.CombinedOutput(); err != nil {
						t.Errorf("cleanup flags: %s %v", raw, err)
					}
				}()
			case "manifest-hardlink":
				if err := os.Link(target, filepath.Join(fx.home, "manifest-copy")); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			if scenario == "external" || scenario == "same-byte-replacement" || scenario == "write-denied" {
				p := claudeChange(t, fx, "disable", "global", id, "", 0)
				if scenario == "external" {
					writeFile(t, config, append(raw, ' '), 0600)
				} else if scenario == "same-byte-replacement" {
					if err := os.Remove(config); err != nil {
						t.Fatal(err)
					}
					writeFile(t, config, raw, 0600)
				}
				before = treeManifest(t, fx.base)
				if scenario == "write-denied" {
					args := []string{"disable", "--agent", "claude-code", "--project", fx.project, "--scope", "global", "--json", "--confirm", p.Confirmation, id}
					result := changeCommand(t, fx, "(deny file-write* (subpath "+strconv.Quote(filepath.Dir(config))+"))", args...)
					assertExit(t, result, 1)
				} else {
					out := claudeChange(t, fx, "disable", "global", id, p.Confirmation, 1)
					if out.State != "refused" {
						t.Fatalf("stale accepted %+v", out)
					}
				}
			} else {
				p := claudeChange(t, fx, "disable", "global", id, "", 1)
				if p.State == "ready" {
					t.Fatal("unsafe preview ready")
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE30AbsentInheritCreatesNothing(t *testing.T) {
	fx, _ := codexFixture(t)
	id, _ := claudeTarget(t, fx)
	before := treeManifest(t, fx.base)
	p := claudeChange(t, fx, "inherit", "project", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal("absent inherit requires no write")
	}
	assertSavedChange(t, claudeChange(t, fx, "inherit", "project", id, p.Confirmation, 0))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE30JSONInsertionAndRemoval(t *testing.T) {
	for _, raw := range []string{"{}\n", "{\"keep\":1}\n", "{\"skillOverrides\":{}}", "{\"skillOverrides\":{\"ale30-target\":\"on\",\"last\":\"off\"}}", "{\"skillOverrides\":{\"first\":\"off\",\"ale30-target\":\"on\",\"last\":\"off\"}}"} {
		t.Run(strings.ReplaceAll(raw, "/", ""), func(t *testing.T) {
			fx, _ := codexFixture(t)
			id, _ := claudeTarget(t, fx)
			config := filepath.Join(fx.project, ".claude/settings.json")
			writeFile(t, config, []byte(raw), 0600)
			p := claudeChange(t, fx, "disable", "project", id, "", 0)
			assertSavedChange(t, claudeChange(t, fx, "disable", "project", id, p.Confirmation, 0))
			p = claudeChange(t, fx, "inherit", "project", id, "", 0)
			assertSavedChange(t, claudeChange(t, fx, "inherit", "project", id, p.Confirmation, 0))
			if bytes.Contains(readBytes(t, config), []byte("ale30-target")) {
				t.Fatal("override retained")
			}
		})
	}
}

func TestALE30SavedChoiceCanLeaveNoInvoker(t *testing.T) {
	fx, _ := codexFixture(t)
	target := claudeSkill(t, filepath.Join(fx.home, ".claude/skills"), "hidden", "ale30-hidden", "disable-model-invocation: true\nuser-invocable: false\n")
	config := filepath.Join(fx.home, ".claude/settings.json")
	writeFile(t, config, []byte("{\"skillOverrides\":{\"ale30-hidden\":\"off\"}}"), 0600)
	id := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(target)).ID
	p := claudeChange(t, fx, "enable", "global", id, "", 0)
	out := claudeChange(t, fx, "enable", "global", id, p.Confirmation, 1)
	if out.State != "applied" || out.Persistence != "verified" || out.Achievement != "unmet" || out.Effective.State != "unavailable" {
		t.Fatalf("saved choice confused with effective result: %+v", out)
	}
	if !bytes.Contains(readBytes(t, config), []byte("\"on\"")) {
		t.Fatal("choice not saved")
	}
}
func TestALE30GlobalEnableAndCapabilityParity(t *testing.T) {
	fx, _ := codexFixture(t)
	id, _ := claudeTarget(t, fx)
	for _, choice := range []string{"disable", "enable"} {
		list := claudeListResult(t, fx, 0)
		var supported bool
		for _, source := range list.Sources {
			if source.ID == id {
				for _, cap := range source.Availability.Capabilities {
					if cap.Operation == choice && cap.Scope == "global" {
						supported = cap.State == "supported"
					}
				}
			}
		}
		if !supported {
			t.Fatal("ready control not advertised")
		}
		p := claudeChange(t, fx, choice, "global", id, "", 0)
		assertSavedChange(t, claudeChange(t, fx, choice, "global", id, p.Confirmation, 0))
	}
	before := treeManifest(t, fx.base)
	p := claudeChange(t, fx, "enable", "global", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal("saved global choice rewrites")
	}
	assertSavedChange(t, claudeChange(t, fx, "enable", "global", id, p.Confirmation, 0))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	claudeSkill(t, filepath.Join(fx.project, ".claude/skills"), "collision", "ale30-target", "")
	list := claudeListResult(t, fx, 0)
	for _, source := range list.Sources {
		if source.ID == id {
			for _, cap := range source.Availability.Capabilities {
				if cap.Operation == "disable" && cap.State == "supported" {
					t.Fatal("ambiguous source advertises control")
				}
			}
		}
	}
	claudeChange(t, fx, "disable", "global", id, "", 1)
}
func TestALE30PluginAndOrdinaryAliasRefused(t *testing.T) {
	fx, _ := codexFixture(t)
	market := filepath.Join(fx.base, "allowed", "market")
	plugin := filepath.Join(market, "demo")
	cached := filepath.Join(fx.home, ".claude/plugins/cache/market/demo/1")
	target := claudeSkill(t, filepath.Join(plugin, "skills"), "action", "action", "")
	claudeSkill(t, filepath.Join(cached, "skills"), "action", "action", "")
	for _, dir := range []string{plugin, cached} {
		claudeSettings(t, filepath.Join(dir, ".claude-plugin/plugin.json"), map[string]string{"name": "demo", "version": "1"})
	}
	claudeSettings(t, filepath.Join(market, ".claude-plugin/marketplace.json"), map[string]any{"name": "market", "plugins": []map[string]string{{"name": "demo", "source": "./demo"}}})
	plugins := filepath.Join(fx.home, ".claude/plugins")
	claudeSettings(t, filepath.Join(plugins, "known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-10-13T00:00:00Z"}})
	claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []map[string]string{{"scope": "user", "installPath": cached, "version": "1"}}}})
	claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}})
	for _, alias := range []bool{false, true} {
		if alias {
			dir := filepath.Join(fx.home, ".claude/skills")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Dir(target), filepath.Join(dir, "ordinary-alias")); err != nil {
				t.Fatal(err)
			}
		}
		before := treeManifest(t, fx.base)
		id := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(target)).ID
		for _, scope := range []string{"global", "project"} {
			out := claudeChange(t, fx, "disable", scope, id, "", 1)
			if out.State != "unsupported" {
				t.Fatalf("plugin: %+v", out)
			}
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	}
}
func TestALE30StaleManifestAndProject(t *testing.T) {
	for _, scenario := range []string{"manifest", "project"} {
		t.Run(scenario, func(t *testing.T) {
			fx, _ := codexFixture(t)
			id, target := claudeTarget(t, fx)
			p := claudeChange(t, fx, "disable", "project", id, "", 0)
			if scenario == "manifest" {
				writeFile(t, target, append(readBytes(t, target), []byte("changed\n")...), 0600)
			} else {
				original := fx.project + "-original"
				if err := os.Rename(fx.project, original); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(fx.project, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			claudeChange(t, fx, "disable", "project", id, p.Confirmation, 1)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE30ACLMetadataRefused(t *testing.T) {
	for _, kind := range []string{"file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			id, _ := claudeTarget(t, fx)
			config := filepath.Join(fx.home, ".claude/settings.json")
			writeFile(t, config, []byte("{}\n"), 0600)
			path := config
			if kind == "directory" {
				path = filepath.Dir(config)
			}
			output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", path).CombinedOutput()
			if err != nil {
				t.Fatalf("fixture ACL: %v %s", err, output)
			}
			before := treeManifest(t, fx.base)
			out := claudeChange(t, fx, "disable", "global", id, "", 1)
			if !strings.Contains(out.Reason, "ACL") {
				t.Fatalf("ACL not diagnosed: %+v", out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE30UnsupportedProjectKeepsInspectionContext(t *testing.T) {
	fx, _ := codexFixture(t)
	id, _ := claudeTarget(t, fx)
	claudeSkill(t, filepath.Join(fx.project, ".claude/skills"), "copy", "ale30-target", "")
	before := treeManifest(t, fx.base)
	result := changeCommand(t, fx, "", "disable", "--agent", "claude-code", "--scope", "project", "--project", fx.project, id)
	assertExit(t, result, 1)
	if !strings.Contains(string(result.stdout), "Inspection project: "+fx.project) {
		t.Fatalf("refusal lost selected project: %s", result.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
