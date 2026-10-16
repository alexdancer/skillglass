package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func piChange(t *testing.T, fx fixture, scope, choice, id, token string, want int) changeReceipt {
	t.Helper()
	args := []string{choice, "--agent", "pi", "--scope", scope, "--project", fx.project, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	args = append(args, id)
	result := changeCommand(t, fx, "", args...)
	assertExit(t, result, want)
	var receipt changeReceipt
	decodeJSON(t, result.stdout, &receipt)
	return receipt
}
func piApply(t *testing.T, fx fixture, scope, choice, id string) changeReceipt {
	t.Helper()
	before := treeManifest(t, fx.base)
	preview := piChange(t, fx, scope, choice, id, "", 0)
	if preview.State != "ready" || preview.Confirmation == "" {
		t.Fatal(preview)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	out := piChange(t, fx, scope, choice, id, preview.Confirmation, 0)
	assertSavedChange(t, out)
	return out
}
func piSettingsValues(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(readBytes(t, path), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
func piSourceAt(t *testing.T, fx fixture, path string) codexSource {
	t.Helper()
	for _, source := range piList(t, fx, 0).Sources {
		if source.SourcePath == filepath.Dir(path) || source.SourcePath == path {
			return source
		}
	}
	t.Fatal("source absent", path)
	return codexSource{}
}

func TestALE34GlobalProjectInheritAndPatterns(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
	sibling := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "sibling", "sibling")
	config := filepath.Join(fx.home, ".pi/agent/settings.json")
	project := filepath.Join(fx.project, ".pi/settings.json")
	raw := []byte("{\n \"defaultProjectTrust\":\"always\",\n \"untouched\": {\"number\":1e3}, \"skills\":[\"!targ*\",\"!unrelated\"]\n}\n")
	writeFile(t, config, raw, 0600)
	otherSettings := []string{filepath.Join(fx.home, ".codex/config.toml"), filepath.Join(fx.home, ".claude/settings.json"), filepath.Join(fx.home, ".hermes/config.yaml")}
	for _, path := range otherSettings {
		writeFile(t, path, []byte("other application sentinel\n"), 0600)
	}
	second := fx
	second.project = filepath.Join(fx.base, "allowed/second-project")
	writeFile(t, filepath.Join(second.project, ".git/marker"), []byte("fixture"), 0600)
	contents := readBytes(t, target)
	siblingBefore := readBytes(t, sibling)
	id := piSourceAt(t, fx, target).ID
	piApply(t, fx, "global", "enable", id)
	saved := readBytes(t, config)
	if !bytes.Contains(saved, []byte(`"untouched": {"number":1e3}`)) || !bytes.Contains(saved, []byte(`"!targ*","!unrelated"`)) {
		t.Fatal("unrelated bytes lost", string(saved))
	}
	piApply(t, fx, "global", "disable", id)
	globalDisabled := readBytes(t, config)
	piApply(t, fx, "project", "enable", id)
	if piSourceAt(t, second, target).Availability.State != "disabled" {
		t.Fatal("other project lost inherited global disable")
	}
	if !bytes.Equal(globalDisabled, readBytes(t, config)) {
		t.Fatal("project wrote global")
	}
	if !strings.Contains(string(readBytes(t, project)), target) {
		t.Fatal("project exact registration absent")
	}
	piApply(t, fx, "project", "inherit", id)
	if piSourceAt(t, fx, target).Availability.State != "disabled" {
		t.Fatal("inherit did not restore global denial")
	}
	if !bytes.Equal(globalDisabled, readBytes(t, config)) {
		t.Fatal("inherit changed global")
	}
	piApply(t, fx, "global", "enable", id)
	globalEnabled := readBytes(t, config)
	piApply(t, fx, "project", "disable", id)
	if piSourceAt(t, fx, target).Availability.State != "disabled" {
		t.Fatal("project disable ineffective")
	}
	// A global choice cannot bypass an existing project denial.
	refused := piChange(t, fx, "global", "enable", id, "", 1)
	if refused.State != "refused" || !strings.Contains(refused.Reason, "higher-priority") {
		t.Fatal(refused)
	}
	piApply(t, fx, "project", "inherit", id)
	if piSourceAt(t, fx, target).Availability.State != "available" {
		t.Fatal("inherit did not restore global enable")
	}
	if !bytes.Equal(globalEnabled, readBytes(t, config)) || !bytes.Equal(contents, readBytes(t, target)) || !bytes.Equal(siblingBefore, readBytes(t, sibling)) {
		t.Fatal("unrelated state changed")
	}
	for _, path := range otherSettings {
		if string(readBytes(t, path)) != "other application sentinel\n" {
			t.Fatal("other application settings changed")
		}
	}
	// Repeated inheritance verifies without writing.
	preview := piChange(t, fx, "project", "inherit", id, "", 0)
	if !preview.NoWriteNeeded {
		t.Fatal(preview)
	}
	piApply(t, fx, "project", "inherit", id)
}

func TestALE34AliasesDuplicateNamesAndAmbiguousRegistration(t *testing.T) {
	fx := piFixture(t)
	root := filepath.Join(fx.home, ".pi/agent/skills")
	target := codexSkill(t, root, "target", "collision")
	other := codexSkill(t, root, "other", "collision")
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Dir(target), alias); err != nil {
		t.Fatal(err)
	}
	source := piSourceAt(t, fx, target)
	otherSource := piSourceAt(t, fx, other)
	// Disable whichever independent source currently wins, preserving the other copy.
	winner := source
	if otherSource.Availability.State == "available" {
		winner = otherSource
	}
	otherBytes := readBytes(t, other)
	targetBytes := readBytes(t, target)
	piApply(t, fx, "global", "disable", winner.ID)
	if !bytes.Equal(otherBytes, readBytes(t, other)) || !bytes.Equal(targetBytes, readBytes(t, target)) {
		t.Fatal("content changed")
	}
	// Enabling a lower-priority copy must not silently report its competitor's result.
	if piSourceAt(t, fx, target).Availability.State == "shadowed" {
		out := piChange(t, fx, "global", "enable", source.ID, "", 1)
		if out.State != "refused" {
			t.Fatal(out)
		}
	}
	// A user-authored resource is not assumed to be our removable override.
	project := filepath.Join(fx.project, ".pi/settings.json")
	encoded, _ := json.Marshal(map[string]any{"skills": []string{target}, "keep": true})
	writeFile(t, project, encoded, 0600)
	before := treeManifest(t, fx.base)
	out := piChange(t, fx, "project", "disable", source.ID, "", 1)
	if out.State != "refused" || !strings.Contains(out.Reason, "ambiguous") {
		t.Fatal(out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE34StaleTrustIdentityAndFailures(t *testing.T) {
	for _, kind := range []string{"settings", "source", "trust", "alias", "write"} {
		t.Run(kind, func(t *testing.T) {
			fx := piFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
			config := filepath.Join(fx.home, ".pi/agent/settings.json")
			if kind == "alias" {
				if err := os.Symlink(filepath.Dir(target), filepath.Join(filepath.Dir(filepath.Dir(target)), "alias")); err != nil {
					t.Fatal(err)
				}
			}
			id := piSourceAt(t, fx, target).ID
			preview := piChange(t, fx, "global", "disable", id, "", 0)
			switch kind {
			case "settings":
				writeFile(t, config, []byte(`{"defaultProjectTrust":"always","external":true}`), 0600)
			case "source":
				writeFile(t, target, append(readBytes(t, target), []byte("external\n")...), 0600)
			case "trust":
				writeFile(t, filepath.Join(fx.home, ".pi/agent/trust.json"), []byte(`{}`), 0600)
			case "alias":
				path := filepath.Join(filepath.Dir(filepath.Dir(target)), "alias")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				other := codexSkill(t, filepath.Join(fx.base, "allowed"), "other", "other")
				if err := os.Symlink(filepath.Dir(other), path); err != nil {
					t.Fatal(err)
				}
			case "write":
				if err := os.Chmod(filepath.Dir(config), 0500); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(filepath.Dir(config), 0700)
			}
			before := treeManifest(t, fx.base)
			out := piChange(t, fx, "global", "disable", id, preview.Confirmation, 1)
			if out.State == "applied" {
				t.Fatal(out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE34NoFallbackUnsafeAndUnknownScopes(t *testing.T) {
	for _, kind := range []string{"trust", "package", "legacy", "duplicate-json", "config-link", "no-project"} {
		t.Run(kind, func(t *testing.T) {
			fx := piFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
			global := filepath.Join(fx.home, ".pi/agent/settings.json")
			scope := "global"
			switch kind {
			case "trust":
				scope = "project"
				writeFile(t, global, []byte(`{"defaultProjectTrust":"never"}`), 0600)
				writeFile(t, filepath.Join(fx.project, ".pi/settings.json"), []byte(`{}`), 0600)
			case "legacy":
				writeFile(t, global, []byte(`{"skills":{"customDirectories":[]}}`), 0600)
			case "duplicate-json":
				writeFile(t, global, []byte(`{"skills":[],"skills":[]}`), 0600)
			case "package":
				encoded, _ := json.Marshal(map[string]any{"packages": []string{filepath.Dir(filepath.Dir(filepath.Dir(target)))}})
				writeFile(t, global, encoded, 0600)
			case "config-link":
				data := readBytes(t, global)
				if err := os.Remove(global); err != nil {
					t.Fatal(err)
				}
				other := filepath.Join(fx.base, "allowed/settings.json")
				writeFile(t, other, data, 0600)
				if err := os.Symlink(other, global); err != nil {
					t.Fatal(err)
				}
			case "no-project":
				scope = "global"
			}
			list := piList(t, fx, 0)
			var id string
			for _, s := range list.Sources {
				if s.Name == "target" {
					id = s.ID
				}
			}
			if id == "" {
				t.Fatal("target absent")
			}
			before := treeManifest(t, fx.base)
			choice := "disable"
			if kind == "no-project" {
				choice = "inherit"
			}
			out := piChange(t, fx, scope, choice, id, "", 1)
			if out.State != "refused" && out.State != "unsupported" {
				t.Fatal(out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE34SharedAliasesProjectScopeAndNativeOverridePairs(t *testing.T) {
	fx := piFixture(t)
	shared := codexSkill(t, filepath.Join(fx.base, "allowed/shared"), "bundle", "alias-target")
	globalRoot := filepath.Join(fx.home, ".pi/agent/skills")
	projectRoot := filepath.Join(fx.project, ".pi/skills")
	if err := os.MkdirAll(globalRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectRoot, 0700); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{filepath.Join(globalRoot, "global-alias"), filepath.Join(projectRoot, "project-alias")} {
		if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
			t.Fatal(err)
		}
	}
	id := codexNamed(t, piList(t, fx, 0), "alias-target").ID
	globalConfig := filepath.Join(fx.home, ".pi/agent/settings.json")
	projectConfig := filepath.Join(fx.project, ".pi/settings.json")
	globalBytes := readBytes(t, globalConfig)
	piApply(t, fx, "project", "disable", id)
	saved := readBytes(t, projectConfig)
	for _, alias := range []string{filepath.Join(globalRoot, "global-alias/SKILL.md"), filepath.Join(projectRoot, "project-alias/SKILL.md")} {
		if !strings.Contains(string(saved), "-"+alias) {
			t.Fatal("lexical exclusion missing", string(saved))
		}
	}
	if !bytes.Equal(globalBytes, readBytes(t, globalConfig)) {
		t.Fatal("project touched global")
	}
	piApply(t, fx, "project", "inherit", id)
	if codexNamed(t, piList(t, fx, 0), "alias-target").Availability.State != "available" {
		t.Fatal("alias inheritance failed")
	}
	// An explicit native pair is removable by user choice, regardless of author.
	lexical := filepath.Join(globalRoot, "global-alias/SKILL.md")
	data, _ := json.Marshal(map[string]any{"skills": []string{lexical, "-" + lexical, "!keep"}, "keep": 123})
	writeFile(t, projectConfig, data, 0600)
	piApply(t, fx, "project", "inherit", id)
	doc := piSettingsValues(t, projectConfig)
	if string(doc["skills"]) != `["!keep"]` || string(doc["keep"]) != "123" {
		t.Fatal(string(readBytes(t, projectConfig)))
	}
	// A lone user registration survives inherit unchanged.
	data, _ = json.Marshal(map[string]any{"skills": []string{lexical, "!keep"}})
	writeFile(t, projectConfig, data, 0600)
	before := readBytes(t, projectConfig)
	piApply(t, fx, "project", "inherit", id)
	if !bytes.Equal(before, readBytes(t, projectConfig)) {
		t.Fatal("lone registration removed")
	}
}

func TestALE34ExactDenialsConfiguredFilesAndIndependentWinner(t *testing.T) {
	fx := piFixture(t)
	root := filepath.Join(fx.base, "allowed/configured")
	target := codexSkill(t, root, "target", "duplicate")
	projectCopy := codexSkill(t, filepath.Join(fx.project, ".pi/skills"), "copy", "duplicate")
	config := filepath.Join(fx.home, ".pi/agent/settings.json")
	data, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "skills": []string{root, "!targ*", "+" + target, "-" + filepath.Dir(target)}, "packages": []any{}, "other": map[string]int{"number": 42}})
	writeFile(t, config, data, 0600)
	id := piSourceAt(t, fx, target).ID
	before := treeManifest(t, fx.base)
	out := piChange(t, fx, "global", "enable", id, "", 1)
	if out.State != "refused" {
		t.Fatal(out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// Disabling the independent project winner does not change global settings.
	pid := piSourceAt(t, fx, projectCopy).ID
	piApply(t, fx, "project", "disable", pid)
	piApply(t, fx, "global", "enable", id)
	if piSourceAt(t, fx, target).Availability.State != "available" {
		t.Fatal("exact minus was not removed")
	}
	saved := readBytes(t, config)
	if !strings.Contains(string(saved), root) || !strings.Contains(string(saved), "!targ*") || strings.Contains(string(saved), "-"+filepath.Dir(target)+`"`) {
		t.Fatal(string(saved))
	}
	// Standalone instruction files get exact selectors, never parent roots.
	standalone := filepath.Join(fx.home, ".pi/agent/skills/standalone.md")
	writeFile(t, standalone, []byte("---\nname: standalone\ndescription: Inert file.\n---\nNo execution.\n"), 0600)
	sid := piSourceAt(t, fx, standalone).ID
	body := readBytes(t, standalone)
	piApply(t, fx, "global", "disable", sid)
	piApply(t, fx, "global", "enable", sid)
	if !bytes.Equal(body, readBytes(t, standalone)) {
		t.Fatal("standalone changed")
	}
}

func TestALE34WriteDenialAndMissingOverrideNoop(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
	id := piSourceAt(t, fx, target).ID
	preview := piChange(t, fx, "project", "inherit", id, "", 0)
	if !preview.NoWriteNeeded || preview.CreateDirectory == "" {
		t.Fatal(preview)
	}
	before := treeManifest(t, fx.base)
	assertSavedChange(t, piChange(t, fx, "project", "inherit", id, preview.Confirmation, 0))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	preview = piChange(t, fx, "global", "disable", id, "", 0)
	args := []string{"disable", "--agent", "pi", "--scope", "global", "--project", fx.project, "--json", "--confirm", preview.Confirmation, id}
	denied := changeCommand(t, fx, "(deny file-write*)", args...)
	assertExit(t, denied, 1)
	var out changeReceipt
	decodeJSON(t, denied.stdout, &out)
	if out.State != "failed" || out.Persistence != "not-written" {
		t.Fatal(out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE34NewProjectSettingsCannotBypassTrust(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
	global := filepath.Join(fx.home, ".pi/agent/settings.json")
	writeFile(t, global, []byte(`{}`), 0600)
	// No project resources currently require trust. Creating the override would.
	id := piSourceAt(t, fx, target).ID
	before := treeManifest(t, fx.base)
	out := piChange(t, fx, "project", "disable", id, "", 1)
	if out.State != "refused" {
		t.Fatal(out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE34RepeatedOverrideRegistrationIsAmbiguous(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
	id := piSourceAt(t, fx, target).ID
	project := filepath.Join(fx.project, ".pi/settings.json")
	data, _ := json.Marshal(map[string]any{"skills": []string{target, target, "-" + target}})
	writeFile(t, project, data, 0600)
	before := treeManifest(t, fx.base)
	out := piChange(t, fx, "project", "inherit", id, "", 1)
	if out.State != "refused" || !strings.Contains(out.Reason, "ambiguous") {
		t.Fatal(out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE34RelativeExactSelectorsPreserveResourceEntries(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
	config := filepath.Join(fx.home, ".pi/agent/settings.json")
	writeFile(t, config, []byte(`{"defaultProjectTrust":"always","skills":["skills","!targ*","-skills/target","+skills/target/SKILL.md"],"keep":"\u0061"}`), 0600)
	id := piSourceAt(t, fx, target).ID
	piApply(t, fx, "global", "enable", id)
	saved := readBytes(t, config)
	for _, entry := range []string{`"skills","!targ*"`, `"keep":"\u0061"`} {
		if !bytes.Contains(saved, []byte(entry)) {
			t.Fatal("unrelated string representation changed", string(saved))
		}
	}
	if bytes.Contains(saved, []byte(`-skills/target`)) {
		t.Fatal("relative exact denial survived")
	}
	piApply(t, fx, "global", "disable", id)
}
