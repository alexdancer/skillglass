package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func geminiFixture(t *testing.T) fixture {
	t.Helper()
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".gemini", "settings.json"), []byte(`{"security":{"folderTrust":{"enabled":false}}}`), 0600)
	return fx
}
func geminiListResult(t *testing.T, fx fixture, exit int, extra ...string) codexList {
	t.Helper()
	args := append([]string{"list", "--agent", "gemini", "--project", fx.project, "--json"}, extra...)
	result := fx.run(t, args...)
	assertExit(t, result, exit)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	return list
}
func TestALE37NativeScopePrecedenceSharingAndOrdinaryCommands(t *testing.T) {
	fx := geminiFixture(t)
	user := filepath.Join(fx.home, ".gemini", "skills")
	alias := filepath.Join(fx.home, ".agents", "skills")
	project := filepath.Join(fx.project, ".gemini", "skills")
	projectAlias := filepath.Join(fx.project, ".agents", "skills")
	for index, root := range []string{user, alias, project, projectAlias} {
		codexSkill(t, root, "collision", "same-native")
		writeFile(t, filepath.Join(root, "collision", "body.txt"), []byte(fmt.Sprint(index)), 0600)
	}
	codexSkill(t, user, "unique", "global-native")
	codexSkill(t, user, "nested/too-deep", "too-deep")
	codexSkill(t, user, ".hidden", "hidden-native")
	codexSkill(t, user, "node_modules/skip", "node-module-native")
	shared := filepath.Dir(codexSkill(t, user, "shared", "shared-native"))
	if err := os.Symlink(shared, filepath.Join(projectAlias, "shared-alias")); err != nil {
		t.Fatal(err)
	}
	// Neither Git ancestors nor arbitrary viewer roots extend native Gemini workspace scope.
	codexSkill(t, filepath.Join(filepath.Dir(fx.project), ".gemini", "skills"), "parent", "parent-native")
	custom := filepath.Join(fx.base, "allowed", "inspection")
	codexSkill(t, custom, "same-native", "same-native")
	before := treeManifest(t, fx.base)
	list := geminiListResult(t, fx, 0, "--root", custom)
	if list.Scope.Agent.Application != "gemini" || list.Scope.Agent.ValidatedVersion != "Gemini CLI 0.63.0" {
		t.Fatalf("scope: %+v", list.Scope)
	}
	counts := map[string]int{}
	var winner codexSource
	for _, source := range list.Sources {
		counts[source.Name]++
		if source.Name == "same-native" {
			expected := "shadowed"
			if strings.HasPrefix(source.SourcePath, projectAlias) {
				expected = "ask"
				winner = source
			}
			if strings.HasPrefix(source.SourcePath, custom) {
				expected = "unknown"
			}
			if source.Availability.State != expected {
				t.Fatalf("copy %s: %s, want %s", source.SourcePath, source.Availability.State, expected)
			}
		}
	}
	if counts["same-native"] != 5 || counts["shared-native"] != 1 {
		t.Fatalf("independent/shared identity: %+v", counts)
	}
	for _, name := range []string{"parent-native", "too-deep", "hidden-native", "node-module-native"} {
		if counts[name] != 0 {
			t.Fatalf("native traversal loaded %s", name)
		}
	}
	sharedSource := codexNamed(t, list, "shared-native")
	if len(sharedSource.Availability.References) != 2 || sharedSource.Availability.State != "ask" {
		t.Fatalf("shared refs: %+v", sharedSource)
	}
	for _, source := range list.Sources {
		if source.Name == "same-native" && source.Availability.State != "unknown" && source.Availability.Precedence.Winner != winner.ID {
			t.Fatal("winning source not linked")
		}
	}
	for _, operation := range []string{"read", "explain"} {
		result := fx.run(t, operation, "--agent", "gemini", "--project", fx.project, "--json", winner.ID)
		assertExit(t, result, 0)
		if !strings.Contains(string(result.stdout), "native-tier-precedence") {
			t.Fatal("shared explanation missing")
		}
	}
	for _, state := range []string{"ask", "shadowed", "unknown"} {
		filtered := geminiListResult(t, fx, 0, "--root", custom, "--availability", state)
		if len(filtered.Sources) == 0 {
			t.Fatalf("empty %s filter", state)
		}
		for _, source := range filtered.Sources {
			if source.Availability.State != state {
				t.Fatal("availability filter mismatch")
			}
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE37SettingsUnionScalarPrecedenceTrustAndRefresh(t *testing.T) {
	fx := geminiFixture(t)
	user := filepath.Join(fx.home, ".gemini", "settings.json")
	project := filepath.Join(fx.project, ".gemini", "settings.json")
	codexSkill(t, filepath.Join(fx.home, ".gemini", "skills"), "user", "union-name")
	codexSkill(t, filepath.Join(fx.project, ".agents", "skills"), "project", "union-name")
	writeFile(t, user, []byte(`{/* no leak */"security":{"folderTrust":{"enabled":false}},"skills":{"disabled":["UNION-NAME"]},"private":"UNRELATED_GEMINI_SECRET"}`), 0600)
	writeFile(t, project, []byte(`{"skills":{"disabled":[]}}`), 0600)
	list := geminiListResult(t, fx, 0)
	winner := codexNamed(t, list, "union-name")
	for _, source := range list.Sources {
		if source.Availability.Precedence.State == "winning" {
			winner = source
		}
	}
	if winner.Availability.State != "disabled" || winner.Availability.References[0].Permission.Evidence.Rule != "disabled-name-union" {
		t.Fatalf("union lost: %+v", winner)
	}
	result := fx.run(t, "explain", "--agent", "gemini", "--project", fx.project, winner.ID)
	assertExit(t, result, 0)
	if strings.Contains(string(result.stdout), "UNRELATED_GEMINI_SECRET") {
		t.Fatal("unrelated settings leaked")
	}
	writeFile(t, user, []byte(`{"skills":{"enabled":false},"security":{"folderTrust":{"enabled":false}}}`), 0600)
	writeFile(t, project, []byte(`{"skills":{"enabled":true}}`), 0600)
	list = geminiListResult(t, fx, 0)
	for _, source := range list.Sources {
		if source.Availability.Precedence.State == "winning" && source.Availability.State != "ask" {
			t.Fatal("workspace scalar precedence lost")
		}
	}
	writeFile(t, user, []byte(`{}`), 0600)
	// Untrusted workspace skill is inspected but excluded, and its settings ignored.
	list = geminiListResult(t, fx, 0)
	for _, source := range list.Sources {
		if strings.HasPrefix(source.SourcePath, fx.project) && source.Availability.State != "unavailable" {
			t.Fatal("untrusted workspace enabled")
		}
	}
	trust := filepath.Join(fx.home, ".gemini", "trustedFolders.json")
	writeFile(t, trust, []byte(fmt.Sprintf("{%q:\"TRUST_FOLDER\"}", fx.project)), 0600)
	list = geminiListResult(t, fx, 0)
	for _, source := range list.Sources {
		if strings.HasPrefix(source.SourcePath, fx.project) && source.Availability.State != "ask" {
			t.Fatal("persisted trust not applied")
		}
	}
	writeFile(t, trust, []byte(fmt.Sprintf("{%q:\"DO_NOT_TRUST\"}", fx.project)), 0600)
	list = geminiListResult(t, fx, 0)
	for _, source := range list.Sources {
		if strings.HasPrefix(source.SourcePath, fx.project) && source.Availability.State != "unavailable" {
			t.Fatal("persisted distrust not applied")
		}
	}
}
func TestALE37ExtensionsNativeNamesAndUnknownOrdering(t *testing.T) {
	fx := geminiFixture(t)
	registry := filepath.Join(fx.home, ".gemini", "extensions")
	extension := filepath.Join(registry, "sample")
	writeFile(t, filepath.Join(extension, "gemini-extension.json"), []byte(`{"name":"sample","version":"1.0.0"}`), 0600)
	codexSkill(t, filepath.Join(extension, "skills"), "skill", "extension-native")
	codexSkill(t, filepath.Join(registry, "stale", "skills"), "skill", "stale-native")
	codexSkill(t, filepath.Join(fx.home, ".gemini", "skills"), "name", "a:b")
	codexSkill(t, filepath.Join(fx.home, ".gemini", "skills"), "one", "ambiguous")
	codexSkill(t, filepath.Join(fx.home, ".gemini", "skills"), "two", "ambiguous")
	list := geminiListResult(t, fx, 0)
	plugin := codexNamed(t, list, "extension-native")
	if plugin.Availability.State != "ask" || plugin.Ownership.State != "managed" || plugin.Availability.References[0].Provenance.PluginID != "sample" {
		t.Fatalf("plugin: %+v", plugin)
	}
	if codexNamed(t, list, "stale-native").Availability.State != "unavailable" {
		t.Fatal("folder treated as registration")
	}
	if codexNamed(t, list, "a:b").Availability.NativeName != "a-b" {
		t.Fatal("native name not sanitized independently")
	}
	for _, source := range list.Sources {
		if source.Name == "ambiguous" && source.Availability.State != "unknown" {
			t.Fatal("guessed within-root winner")
		}
	}
	enablement := filepath.Join(registry, "extension-enablement.json")
	writeFile(t, enablement, []byte(`{"sample":{"overrides":["!/*"]}}`), 0600)
	if codexNamed(t, geminiListResult(t, fx, 0), "extension-native").Availability.State != "unavailable" {
		t.Fatal("inactive extension effective")
	}
	writeFile(t, enablement, []byte(fmt.Sprintf("{\"sample\":{\"overrides\":[\"!/*\",%q]}}", fx.project+"/")), 0600)
	if codexNamed(t, geminiListResult(t, fx, 0), "extension-native").Availability.State != "ask" {
		t.Fatal("last extension override lost")
	}
	for _, capability := range plugin.Availability.Capabilities {
		if capability.State != "unsupported" {
			t.Fatal("unverified writer exposed")
		}
	}
}
func TestALE37MissingMalformedAndBoundedNativeInputs(t *testing.T) {
	for _, kind := range []string{"invalid", "fifo", "oversized", "broken-link", "typed-invalid"} {
		t.Run(kind, func(t *testing.T) {
			fx := geminiFixture(t)
			path := filepath.Join(fx.home, ".gemini", "settings.json")
			codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "safe", "safe-gemini")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "invalid":
				writeFile(t, path, []byte(`{"skills":`), 0600)
			case "typed-invalid":
				writeFile(t, path, []byte(`{"skills":{"enabled":"false"}}`), 0600)
			case "fifo":
				if err := syscall.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				writeFile(t, path, []byte(strings.Repeat(" ", (4<<20)+1)), 0600)
			case "broken-link":
				if err := os.Symlink(filepath.Join(fx.base, "missing-settings"), path); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			list := geminiListResult(t, fx, 1)
			if codexNamed(t, list, "safe-gemini").Availability.State != "unknown" {
				t.Fatal("bad native settings promoted availability")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
	fx := geminiFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "safe", "missing-project")
	if codexNamed(t, geminiListResult(t, fx, 1, "--project", filepath.Join(fx.base, "no-project")), "missing-project").Availability.State != "unknown" {
		t.Fatal("missing selected project coverage lost")
	}
}

func TestALE37OfflineReadOnlyExecutableContainment(t *testing.T) {
	fx := geminiFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "native", "contained-gemini")
	executable := canonicalPath(t, skillglassBinary)
	profile := filepath.Join(fx.base, "allowed", "gemini-offline.sb")
	writeFile(t, profile, []byte(runtimeProfile(executable, canonicalPath(t, filepath.Join(fx.base, "allowed")))), 0600)
	before := treeManifest(t, fx.base)
	result := runSandboxCommand(t, fx, profile, executable, "list", "--agent", "gemini", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	source := codexNamed(t, list, "contained-gemini")
	if source.Availability.State != "ask" {
		t.Fatal("contained native source not usable")
	}
	for _, operation := range []string{"read", "explain"} {
		result = runSandboxCommand(t, fx, profile, executable, operation, "--agent", "gemini", "--project", fx.project, "--json", source.ID)
		assertExit(t, result, 0)
		writeEvidence(t, "ale37/isolation/"+operation+".json", result.stdout)
	}
	raw := runSandboxBrowser(t, fx, profile, executable, "browse", "--agent", "gemini", "--project", fx.project)
	if !bytes.Contains(raw, []byte("contained-gemini")) {
		t.Fatal("contained native browser lacks source")
	}
	writeEvidence(t, "ale37/isolation/browse.ansi", raw)
	after := treeManifest(t, fx.base)
	assertTreeUnchanged(t, before, after)
	writeEvidence(t, "ale37/isolation/product.sb", []byte(runtimeProfile(executable, canonicalPath(t, filepath.Join(fx.base, "allowed")))))
	writeEvidence(t, "ale37/isolation/tree-before.json", before)
	writeEvidence(t, "ale37/isolation/tree-after.json", after)
}

func TestALE37ConfiguredHomeAndManagedAliases(t *testing.T) {
	fx := geminiFixture(t)
	alternate := filepath.Join(fx.base, "allowed", "alternate-home")
	codexSkill(t, filepath.Join(alternate, ".agents", "skills"), "alternate", "configured-home")
	command := fx.command(t, context.Background(), "list", "--agent", "gemini", "--json")
	command.Dir = fx.project
	command.Env = append(fx.environment(), "GEMINI_CLI_HOME="+alternate)
	launch := recordLaunch(t, command)
	raw, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var list codexList
	decodeJSON(t, raw, &list)
	if len(list.Sources) != 1 || codexNamed(t, list, "configured-home").Availability.State != "ask" {
		t.Fatalf("configured home: %s", raw)
	}
	recordEvidenceCommand(t, "public-cli-contained", commandResult{stdout: raw, launch: launch})
	extension := filepath.Join(fx.home, ".gemini", "extensions", "sample")
	writeFile(t, filepath.Join(extension, "gemini-extension.json"), []byte(`{"name":"sample","version":"1.0.0"}`), 0600)
	managed := filepath.Dir(codexSkill(t, filepath.Join(extension, "skills"), "managed", "managed-alias"))
	user := filepath.Join(fx.home, ".agents", "skills")
	if err := os.MkdirAll(user, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(managed, filepath.Join(user, "shared-managed")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Dir(codexSkill(t, user, "outside", "external-content"))
	if err := os.Symlink(outside, filepath.Join(extension, "skills", "external")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".gemini", "extensions", "extension-enablement.json"), []byte(`{"sample":{"overrides":["!/*"]}}`), 0600)
	list = geminiListResult(t, fx, 0)
	if source := codexNamed(t, list, "managed-alias"); source.Ownership.State != "managed" || source.Availability.State != "ask" {
		t.Fatalf("independent user alias participation lost: %+v", source)
	}
	if source := codexNamed(t, list, "external-content"); source.Ownership.State == "managed" || source.Availability.State != "ask" {
		t.Fatalf("external target ownership invented: %+v", source)
	}
	writeFile(t, filepath.Join(fx.home, ".gemini", "extensions", "extension-enablement.json"), []byte(`{"sample":{}}`), 0600)
	list = geminiListResult(t, fx, 1)
	if codexNamed(t, list, "managed-alias").Availability.State != "unknown" {
		t.Fatal("wrongly typed enablement promoted availability")
	}
}

func TestALE37ExtensionDirectoryAliasDoesNotGrantTargetOwnership(t *testing.T) {
	fx := geminiFixture(t)
	external := filepath.Join(fx.base, "allowed", "external-extension")
	writeFile(t, filepath.Join(external, "gemini-extension.json"), []byte(`{"name":"external","version":"1.0.0"}`), 0600)
	codexSkill(t, filepath.Join(external, "skills"), "source", "external-registration-target")
	registry := filepath.Join(fx.home, ".gemini", "extensions")
	if err := os.MkdirAll(registry, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(registry, "external")); err != nil {
		t.Fatal(err)
	}
	source := codexNamed(t, geminiListResult(t, fx, 0), "external-registration-target")
	if source.Availability.State != "ask" {
		t.Fatalf("registered native reference lost: %+v", source)
	}
	if source.Ownership.State == "managed" {
		t.Fatalf("registration alias granted ownership to external content: %+v", source)
	}
}
