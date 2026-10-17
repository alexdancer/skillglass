package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func geminiChange(t *testing.T, fx fixture, scope, choice, id, token string, want int) changeReceipt {
	t.Helper()
	args := []string{choice, "--agent", "gemini", "--scope", scope, "--project", fx.project, "--json"}
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
func geminiApply(t *testing.T, fx fixture, scope, choice, id string) changeReceipt {
	t.Helper()
	before := treeManifest(t, fx.base)
	p := geminiChange(t, fx, scope, choice, id, "", 0)
	if p.State != "ready" || p.Confirmation == "" {
		t.Fatal(p)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	out := geminiChange(t, fx, scope, choice, id, p.Confirmation, 0)
	assertSavedChange(t, out)
	destination := filepath.Join(fx.home, ".gemini/settings.json")
	if scope == "project" {
		destination = filepath.Join(fx.project, ".gemini/settings.json")
	}
	relative, err := filepath.Rel(fx.base, destination)
	if err != nil {
		t.Fatal(err)
	}
	strip := func(raw []byte) []byte {
		var entries []treeEntry
		decodeJSON(t, raw, &entries)
		kept := []treeEntry{}
		for _, entry := range entries {
			if entry.Path != filepath.ToSlash(relative) && !strings.HasPrefix(entry.Mode, "d") {
				kept = append(kept, entry)
			}
		}
		out, e := json.Marshal(kept)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	assertTreeUnchanged(t, strip(before), strip(treeManifest(t, fx.base)))
	return out
}
func TestALE38ScopedControlsPreserveJSONCAndUnion(t *testing.T) {
	fx := geminiFixture(t)
	manifest := codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "target", "target")
	id := codexNamed(t, geminiListResult(t, fx, 0), "target").ID
	config := filepath.Join(fx.home, ".gemini/settings.json")
	raw := []byte("{ // keep top\n\"security\":{\"folderTrust\":{\"enabled\":false}},\"ui\":{\"number\":1e3},\"skills\":{\"disabled\":[\"other\"]}}\n")
	writeFile(t, config, raw, 0600)
	writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte("unrelated = true\n"), 0600)
	body := readBytes(t, manifest)
	geminiApply(t, fx, "global", "disable", id)
	saved := readBytes(t, config)
	for _, needle := range []string{"// keep top", `"number":1e3`, `"other"`} {
		if !bytes.Contains(saved, []byte(needle)) {
			t.Fatal(string(saved))
		}
	}
	before := treeManifest(t, fx.base)
	refused := geminiChange(t, fx, "project", "enable", id, "", 1)
	if refused.State != "refused" || !strings.Contains(refused.Reason, "layer") {
		t.Fatal(refused)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	geminiApply(t, fx, "project", "disable", id)
	geminiApply(t, fx, "project", "inherit", id)
	if codexNamed(t, geminiListResult(t, fx, 0), "target").Availability.State != "disabled" {
		t.Fatal("inherit lost global deny")
	}
	geminiApply(t, fx, "global", "enable", id)
	if codexNamed(t, geminiListResult(t, fx, 0), "target").Availability.State != "ask" {
		t.Fatal("enable lost consent")
	}
	geminiApply(t, fx, "project", "disable", id)
	geminiApply(t, fx, "project", "enable", id)
	geminiApply(t, fx, "project", "inherit", id)
	if !bytes.Equal(body, readBytes(t, manifest)) {
		t.Fatal("content changed")
	}
}

func TestALE38CaseEntriesPreserveTriviaAndSharedReferences(t *testing.T) {
	fx := geminiFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "target", "target")
	alias := filepath.Join(fx.project, ".agents/skills/shared")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(target), alias); err != nil {
		t.Fatal(err)
	}
	id := codexNamed(t, geminiListResult(t, fx, 0), "target").ID
	config := filepath.Join(fx.home, ".gemini/settings.json")
	raw := []byte(`{"security":{"folderTrust":{"enabled":false}},"skills":{"disabled":[/*before*/"TARGET",/*between*/"o\u0074her", "comma,name", "target"/*after*/]},"keep":1e3}`)
	writeFile(t, config, raw, 0600)
	geminiApply(t, fx, "global", "enable", id)
	saved := readBytes(t, config)
	for _, needle := range []string{"/*before*/", "/*between*/", "/*after*/", `"o\u0074her"`, `"comma,name"`, `"keep":1e3`} {
		if !bytes.Contains(saved, []byte(needle)) {
			t.Fatal(string(saved))
		}
	}
	before := treeManifest(t, fx.base)
	p := geminiChange(t, fx, "global", "enable", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal(p)
	}
	geminiChange(t, fx, "global", "enable", id, p.Confirmation, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE38UnsafeAndAmbiguousRequestsNeverWrite(t *testing.T) {
	for _, kind := range []string{"duplicate", "case-duplicate", "excluded-copy", "project-only", "untrusted", "duplicate-json", "null-entry", "surrogate", "malformed", "config-symlink", "config-hardlink", "role-alias", "content-output", "subsystem", "project-mask", "managed", "managed-duplicate", "linked-managed-reference", "unicode-duplicate", "sanitized-duplicate"} {
		t.Run(kind, func(t *testing.T) {
			fx := geminiFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "target", "target")
			id := codexNamed(t, geminiListResult(t, fx, 0), "target").ID
			config := filepath.Join(fx.home, ".gemini/settings.json")
			scope, choice := "global", "disable"
			switch kind {
			case "linked-managed-reference":
				extension := filepath.Join(fx.home, ".gemini/extensions/linked")
				writeFile(t, filepath.Join(extension, ".gemini-extension-install.json"), []byte(fmt.Sprintf(`{"type":"link","source":%q}`, filepath.Join(fx.base, "allowed/linked-source"))), 0600)
				linked := filepath.Join(fx.base, "allowed/linked-source")
				writeFile(t, filepath.Join(linked, "gemini-extension.json"), []byte(`{"name":"linked","version":"1.0.0"}`), 0600)
				if e := os.MkdirAll(filepath.Join(linked, "skills"), 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(filepath.Dir(target), filepath.Join(linked, "skills/shared")); e != nil {
					t.Fatal(e)
				}
			case "managed", "managed-duplicate":
				extension := filepath.Join(fx.home, ".gemini/extensions/sample")
				writeFile(t, filepath.Join(extension, "gemini-extension.json"), []byte(`{"name":"sample","version":"1.0.0"}`), 0600)
				name := "plugin"
				if kind == "managed-duplicate" {
					name = "TARGET"
				}
				codexSkill(t, filepath.Join(extension, "skills"), "skill", name)
				if kind == "managed" {
					id = codexNamed(t, geminiListResult(t, fx, 0), name).ID
				}
			case "unicode-duplicate":
				writeFile(t, target, []byte("---\nname: ΟΣ\ndescription: Unicode case identity.\n---\nInert.\n"), 0600)
				codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "copy", "ος")
			case "sanitized-duplicate":
				writeFile(t, target, []byte("---\nname: a:b\ndescription: Sanitized identity.\n---\nInert.\n"), 0600)
				codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "copy", "a-b")
			case "duplicate":
				codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "copy", "target")
			case "case-duplicate":
				codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "copy", "TARGET")
			case "excluded-copy":
				codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "copy", "target")
				writeFile(t, config, []byte(`{}`), 0600)
			case "project-only":
				codexSkill(t, filepath.Join(fx.project, ".gemini/skills"), "local", "local")
				id = codexNamed(t, geminiListResult(t, fx, 0), "local").ID
			case "untrusted":
				scope = "project"
				writeFile(t, config, []byte(`{}`), 0600)
			case "duplicate-json":
				writeFile(t, config, []byte(`{"skills":{},"skills":{},"security":{"folderTrust":{"enabled":false}}}`), 0600)
			case "null-entry":
				writeFile(t, config, []byte(`{"skills":{"disabled":[null]},"security":{"folderTrust":{"enabled":false}}}`), 0600)
			case "surrogate":
				writeFile(t, config, []byte(`{"skills":{"disabled":["\ud800"]},"security":{"folderTrust":{"enabled":false}}}`), 0600)
			case "malformed":
				writeFile(t, config, []byte(`{"skills":`), 0600)
			case "config-symlink":
				other := filepath.Join(fx.base, "allowed/settings.json")
				writeFile(t, other, readBytes(t, config), 0600)
				if e := os.Remove(config); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(other, config); e != nil {
					t.Fatal(e)
				}
			case "config-hardlink":
				if e := os.Link(config, filepath.Join(fx.base, "allowed/alias.json")); e != nil {
					t.Fatal(e)
				}
			case "role-alias":
				scope = "project"
				fx.project = fx.home
			case "content-output":
				scope = "project"
				fx.project = filepath.Dir(target)
			case "subsystem":
				choice = "enable"
				writeFile(t, config, []byte(`{"skills":{"enabled":false},"security":{"folderTrust":{"enabled":false}}}`), 0600)
			case "project-mask":
				choice = "enable"
				writeFile(t, config, []byte(`{"skills":{"enabled":false},"security":{"folderTrust":{"enabled":false}}}`), 0600)
				writeFile(t, filepath.Join(fx.project, ".gemini/settings.json"), []byte(`{"skills":{"enabled":true}}`), 0600)
			}
			before := treeManifest(t, fx.base)
			p := geminiChange(t, fx, scope, choice, id, "", 1)
			if p.State != "refused" && p.State != "unsupported" {
				t.Fatal(p)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE38ExternalEditsAndWriteFailure(t *testing.T) {
	for _, kind := range []string{"external", "source", "identity", "trust", "duplicate", "registry", "write", "wrong-token"} {
		t.Run(kind, func(t *testing.T) {
			fx := geminiFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "target", "target")
			id := codexNamed(t, geminiListResult(t, fx, 0), "target").ID
			config := filepath.Join(fx.home, ".gemini/settings.json")
			p := geminiChange(t, fx, "global", "disable", id, "", 0)
			switch kind {
			case "external":
				writeFile(t, config, []byte(`{"security":{"folderTrust":{"enabled":false}},"external":true}`), 0600)
			case "source":
				writeFile(t, target, append(readBytes(t, target), []byte("External\n")...), 0600)
			case "identity":
				old := readBytes(t, config)
				if e := os.Remove(config); e != nil {
					t.Fatal(e)
				}
				writeFile(t, config, old, 0600)
			case "trust":
				writeFile(t, filepath.Join(fx.home, ".gemini/trustedFolders.json"), []byte(`{}`), 0600)
			case "duplicate":
				codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "copy", "TARGET")
			case "registry":
				writeFile(t, filepath.Join(fx.home, ".gemini/extensions/extension-enablement.json"), []byte(`{}`), 0600)
			case "wrong-token":
				p.Confirmation = "incorrect"
			}
			before := treeManifest(t, fx.base)
			if kind == "write" {
				r := changeCommand(t, fx, "(deny file-write*)", "disable", "--agent", "gemini", "--scope", "global", "--project", fx.project, "--json", "--confirm", p.Confirmation, id)
				assertExit(t, r, 1)
				var out changeReceipt
				decodeJSON(t, r.stdout, &out)
				if out.State != "failed" || out.Persistence != "not-written" {
					t.Fatal(out)
				}
			} else {
				out := geminiChange(t, fx, "global", "disable", id, p.Confirmation, 1)
				if out.State != "refused" && out.State != "unsupported" {
					t.Fatal(out)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE38AbsentSettingsCreationAndInheritedNoOp(t *testing.T) {
	fx := geminiFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	id := codexNamed(t, geminiListResult(t, fx, 0), "target").ID
	before := treeManifest(t, fx.base)
	p := geminiChange(t, fx, "project", "inherit", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal(p)
	}
	geminiChange(t, fx, "project", "inherit", id, p.Confirmation, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	config := filepath.Join(fx.home, ".gemini/settings.json")
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Dir(config)); err != nil {
		t.Fatal(err)
	}
	geminiApply(t, fx, "global", "disable", id)
	if info, err := os.Stat(filepath.Dir(config)); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("new settings directory: %v %v", info, err)
	}
	if !bytes.Contains(readBytes(t, config), []byte(`"target"`)) {
		t.Fatal("native disabled entry absent")
	}
	r := fx.run(t, "explain", "--agent", "gemini", "--json", id)
	assertExit(t, r, 0)
	before = treeManifest(t, fx.base)
	r = changeCommand(t, fx, "", "disable", "--agent", "gemini", "--scope", "project", "--json", id)
	assertExit(t, r, 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
