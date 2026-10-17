package acceptance_test

import (
	"bytes"
	"encoding/json"
	"github.com/alexdancer/skillglass/internal/collection"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ale40Fixture(t *testing.T, missing bool) (fixture, string, string) {
	t.Helper()
	fx, _ := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "ale40-target", "ale40-target")
	for _, root := range []string{filepath.Join(fx.home, ".claude/skills"), filepath.Join(fx.home, ".hermes/skills"), filepath.Join(fx.home, ".pi/agent/skills"), filepath.Join(fx.xdgConfig, "opencode/skills"), filepath.Join(fx.home, ".gemini/skills")} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Dir(target), filepath.Join(root, "ale40-target")); err != nil {
			t.Fatal(err)
		}
	}
	for path, raw := range map[string]string{filepath.Join(fx.home, ".hermes/config.yaml"): "keep: true\nskills:\n  disabled: []\n", filepath.Join(fx.home, ".pi/agent/settings.json"): "{\"keep\":true}", filepath.Join(fx.xdgConfig, "opencode/opencode.json"): "{\"keep\":true}", filepath.Join(fx.home, ".gemini/settings.json"): "{\"keep\":true,\"security\":{\"folderTrust\":{\"enabled\":false}}}"} {
		writeFile(t, path, []byte(raw), 0600)
	}
	if !missing {
		writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte("# keep\n"), 0600)
		writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte("{\"keep\":true}"), 0600)
	}
	id := changeSourceID(t, fx, target)
	return fx, id, target
}
func ale40Command(t *testing.T, fx fixture, apps []string, choice, scope, id, token, policy string, want int) commandResult {
	t.Helper()
	args := []string{choice, "--scope", scope, "--project", fx.project, "--json"}
	for _, app := range apps {
		args = append(args, "--agents", app)
	}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	args = append(args, id)
	r := changeCommand(t, fx, policy, args...)
	assertExit(t, r, want)
	return r
}
func ale40Preview(t *testing.T, fx fixture, apps []string, scope, id string, want int) collection.GroupView {
	t.Helper()
	r := ale40Command(t, fx, apps, "disable", scope, id, "", "", want)
	var p collection.GroupView
	decodeJSON(t, r.stdout, &p)
	return p
}
func ale40Apply(t *testing.T, fx fixture, apps []string, id, token, policy string, want int) collection.GroupOutcome {
	t.Helper()
	r := ale40Command(t, fx, apps, "disable", "global", id, token, policy, want)
	var out collection.GroupOutcome
	decodeJSON(t, r.stdout, &out)
	return out
}
func TestALE40PublicSupportedGroups(t *testing.T) {
	for _, apps := range [][]string{{"codex", "claude-code"}, {"claude-code", "codex"}, {"codex", "claude-code", "hermes", "pi", "opencode", "gemini"}, {"gemini", "opencode", "pi", "hermes", "claude-code", "codex"}} {
		for _, missing := range []bool{false, true} {
			t.Run(strings.Join(apps, "-")+map[bool]string{true: "-missing", false: "-existing"}[missing], func(t *testing.T) {
				fx, id, target := ale40Fixture(t, missing)
				body := readBytes(t, target)
				before := treeManifest(t, fx.base)
				p := ale40Preview(t, fx, apps, "global", id, 0)
				if p.State != "ready" || p.Confirmation == "" {
					t.Fatal(p.State, p.Reason, p.Members)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if len(p.Consumers) != 7 {
					t.Fatal("catalog coverage missing")
				}
				for _, m := range p.Members {
					if m.Preview.Confirmation != "" {
						t.Fatal("child execution authority exposed")
					}
				}
				out := ale40Apply(t, fx, apps, id, p.Confirmation, "", 0)
				if out.State != "applied" || len(out.Results) != len(apps) {
					t.Fatal(out.State, out.Reason)
				}
				for i, r := range out.Results {
					if r.Application != apps[i] || r.State != "succeeded" || r.Outcome == nil || r.Outcome.ExitCode() != 0 || r.Outcome.Reload.State == "" {
						t.Fatal(r)
					}
				}
				if !bytes.Equal(body, readBytes(t, target)) {
					t.Fatal("source changed")
				}
				if !missing {
					for _, path := range []string{filepath.Join(fx.home, ".codex/config.toml"), filepath.Join(fx.home, ".claude/settings.json")} {
						if !bytes.Contains(readBytes(t, path), []byte("keep")) {
							t.Fatal("unrelated setting lost")
						}
					}
				}
				// A new already-satisfied group is a verified no-op, not a rewrite.
				p = ale40Preview(t, fx, apps, "global", id, 0)
				before = treeManifest(t, fx.base)
				ale40Apply(t, fx, apps, id, p.Confirmation, "", 0)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}
func TestALE40PublicRefusalAndFreshSubset(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	before := treeManifest(t, fx.base)
	for _, apps := range [][]string{{"codex", "claude-code", "codeaf"}, {"codex", "unknown"}, {"codex", "codex"}} {
		p := ale40Preview(t, fx, apps, "global", id, 1)
		if p.State == "ready" || p.Confirmation != "" {
			t.Fatal(p.State, p.Reason)
		}
		out := ale40Apply(t, fx, apps, id, "g1:forged", "", 1)
		for _, r := range out.Results {
			if r.State != "not-attempted" {
				t.Fatal(r)
			}
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	}
	p := ale40Preview(t, fx, []string{"codex", "claude-code"}, "project", id, 1)
	if p.Confirmation != "" {
		t.Fatal("unsupported Codex project authorized")
	}
	p = ale40Preview(t, fx, []string{"codex", "claude-code"}, "global", id, 0)
	ale40Apply(t, fx, []string{"claude-code", "codex"}, id, p.Confirmation, "", 1)
	ale40Apply(t, fx, []string{"codex"}, id, p.Confirmation, "", 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	ale40Apply(t, fx, []string{"codex", "claude-code"}, id, p.Confirmation, "", 0)
	ale40Apply(t, fx, []string{"codex", "claude-code"}, id, p.Confirmation, "", 1)
}
func TestALE40PublicStaleInputs(t *testing.T) {
	for _, scenario := range []string{"source", "config", "project", "coverage", "consumer"} {
		t.Run(scenario, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			apps := []string{"codex", "claude-code"}
			p := ale40Preview(t, fx, apps, "global", id, 0)
			switch scenario {
			case "source":
				writeFile(t, target, append(readBytes(t, target), []byte("changed\n")...), 0600)
			case "config":
				writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte("{\"external\":true}"), 0600)
			case "project":
				fx.project = filepath.Dir(fx.project)
			case "coverage":
				writeFile(t, filepath.Join(fx.home, ".hermes/config.yaml"), []byte("[invalid"), 0600)
			case "consumer":
				if err := os.Remove(filepath.Join(fx.home, ".claude/skills/ale40-target")); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			ale40Apply(t, fx, apps, id, p.Confirmation, "", 1)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE40PublicIndependentCopiesAndIncompleteCoverage(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	alias := filepath.Join(fx.home, ".claude/skills/ale40-target")
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	copyManifest := codexSkill(t, filepath.Dir(alias), "ale40-target", "ale40-target")
	copyBefore := readBytes(t, copyManifest)
	before := treeManifest(t, fx.base)
	p := ale40Preview(t, fx, []string{"codex", "claude-code"}, "global", id, 1)
	if p.Confirmation != "" {
		t.Fatal("independent copy substituted")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if !bytes.Equal(copyBefore, readBytes(t, copyManifest)) {
		t.Fatal("copy changed")
	}
	// Incomplete unrequested discovery stays visible without claiming all-machine coverage.
	writeFile(t, filepath.Join(fx.home, ".hermes/config.yaml"), []byte("[invalid"), 0600)
	p = ale40Preview(t, fx, []string{"codex"}, "global", id, 0)
	incomplete := false
	for _, c := range p.Consumers {
		if c.Application == "hermes" && c.Scope.Completeness == collection.Incomplete {
			incomplete = true
		}
	}
	if !incomplete {
		t.Fatal("coverage hidden")
	}
	ale40Apply(t, fx, []string{"codex"}, id, p.Confirmation, "", 0)
}
func TestALE40PublicRuntimeFailureRetainsEarlierSuccess(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	apps := []string{"codex", "claude-code", "hermes"}
	p := ale40Preview(t, fx, apps, "global", id, 0)
	third := filepath.Join(fx.home, ".hermes/config.yaml")
	before := readBytes(t, third)
	policy := "(deny file-write* (subpath " + sandboxString(filepath.Join(fx.home, ".claude")) + "))"
	out := ale40Apply(t, fx, apps, id, p.Confirmation, policy, 1)
	if len(out.Results) != 3 || out.Results[0].State != "succeeded" || out.Results[1].State != "failed" || out.Results[2].State != "not-attempted" {
		t.Fatal(out.Results, out.Reason)
	}
	if !strings.Contains(string(readBytes(t, filepath.Join(fx.home, ".codex/config.toml"))), "enabled = false") {
		t.Fatal("earlier change rolled back")
	}
	if !bytes.Equal(before, readBytes(t, third)) {
		t.Fatal("third app touched")
	}
	if strings.Contains(string(readBytes(t, filepath.Join(fx.home, ".claude/settings.json"))), "off") {
		t.Fatal("failed target unexpectedly changed")
	}
}

func TestALE40PublicAliasedDestinations(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	agent := filepath.Join(fx.home, ".pi/agent")
	if err := os.Rename(agent, agent+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fx.home, ".claude"), agent); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	for _, apps := range [][]string{{"claude-code", "pi"}, {"pi", "claude-code"}} {
		p := ale40Preview(t, fx, apps, "global", id, 1)
		if p.Confirmation != "" || !strings.Contains(p.Reason, "alias") {
			t.Fatal(p.State, p.Reason)
		}
		for _, m := range p.Members {
			if m.Preview.State != "ready" {
				t.Fatal("fixture must reach aggregate alias check", m.Preview.Reason)
			}
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	}
}
func TestALE40PublicSafelyMissingParents(t *testing.T) {
	for _, apps := range [][]string{{"codex", "claude-code"}, {"claude-code", "codex"}} {
		t.Run(strings.Join(apps, "-"), func(t *testing.T) {
			fx, id, target := ale40Fixture(t, true)
			// Move Claude discovery to the selected project so both user destinations
			// can safely create their absent immediate configuration directory.
			dir := filepath.Join(fx.home, ".claude")
			if err := os.Rename(dir, dir+"-saved"); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(fx.project, ".claude/skills")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Dir(target), filepath.Join(root, "ale40-target")); err != nil {
				t.Fatal(err)
			}
			codexHome := filepath.Join(fx.home, ".codex")
			if err := os.Rename(codexHome, codexHome+"-saved"); err != nil {
				t.Fatal(err)
			}
			p := ale40Preview(t, fx, apps, "global", id, 0)
			for _, m := range p.Members {
				if m.Preview.CreateDirectory == "" {
					t.Fatal("missing directory not exposed", m)
				}
			}
			ale40Apply(t, fx, apps, id, p.Confirmation, "", 0)
		})
	}
}
func TestALE40PublicProjectAndInheritance(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	apps := []string{"claude-code", "opencode"}
	writeFile(t, filepath.Join(fx.project, ".claude/settings.json"), []byte("{\"keep\":true}"), 0600)
	global := ale40Preview(t, fx, apps, "global", id, 0)
	ale40Apply(t, fx, apps, id, global.Confirmation, "", 0)
	run := func(choice string) collection.GroupOutcome {
		r := ale40Command(t, fx, apps, choice, "project", id, "", "", 0)
		var p collection.GroupView
		decodeJSON(t, r.stdout, &p)
		r = ale40Command(t, fx, apps, choice, "project", id, p.Confirmation, "", 0)
		var out collection.GroupOutcome
		decodeJSON(t, r.stdout, &out)
		return out
	}
	for _, choice := range []string{"enable", "inherit"} {
		out := run(choice)
		for _, r := range out.Results {
			if r.State != "succeeded" || r.Outcome == nil {
				t.Fatal(r)
			}
			expected := collection.AvailabilityAvailable
			if choice == "inherit" {
				expected = collection.AvailabilityDisabled
			}
			if r.Outcome.Effective.State != expected {
				t.Fatal(r.Outcome.Effective.State, expected)
			}
		}
	}
}

func TestALE40PublicCreationReadInteraction(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	before := treeManifest(t, fx.base)
	p := ale40Preview(t, fx, []string{"claude-code", "opencode"}, "project", id, 1)
	if p.Confirmation != "" || !strings.Contains(p.Reason, "preview input") {
		t.Fatal(p.State, p.Reason)
	}
	for _, m := range p.Members {
		if m.Preview.State != "ready" {
			t.Fatal("fixture must reach aggregate read-interaction check", m.Preview.Reason)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE40PublicOpenCodeDiscoveryContainment(t *testing.T) {
	for _, inspection := range []string{"selected-and-global", "selected-only", "global-only", "no-project"} {
		for _, route := range []string{"direct", "symlink", "child-symlink"} {
			for _, writerFirst := range []bool{true, false} {
				t.Run(inspection+"/"+route+"/"+map[bool]string{true: "claude-first", false: "opencode-first"}[writerFirst], func(t *testing.T) {
					ale40NativeDiscoveryContainment(t, "opencode", inspection, route, writerFirst)
				})
			}
		}
	}
}
func TestALE40PublicPiChildBundleDiscoveryContainment(t *testing.T) {
	for _, inspection := range []string{"selected-and-global", "no-project"} {
		for _, writerFirst := range []bool{true, false} {
			t.Run(inspection+"/"+map[bool]string{true: "claude-first", false: "pi-first"}[writerFirst], func(t *testing.T) {
				ale40NativeDiscoveryContainment(t, "pi", inspection, "child-symlink", writerFirst)
			})
		}
	}
}
func TestALE40PublicInspectionBundleDiscoveryContainment(t *testing.T) {
	for _, reader := range []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini"} {
		t.Run(reader+"/writer-first", func(t *testing.T) {
			ale40NativeDiscoveryContainment(t, reader, "inspection-only", "child-symlink", true)
		})
		for _, writerFirst := range []bool{true, false} {
			t.Run(reader+"/no-op-writer/"+map[bool]string{true: "writer-first", false: "reader-first"}[writerFirst], func(t *testing.T) {
				ale40NativeDiscoveryContainment(t, reader, "inspection-only-noop", "child-symlink", writerFirst)
			})
		}
	}
}
func TestALE40PublicNativeChildBundleDiscoveryContainment(t *testing.T) {
	for _, reader := range []string{"codex", "claude-code", "hermes", "gemini"} {
		for _, writerFirst := range []bool{true, false} {
			t.Run(reader+"/"+map[bool]string{true: "writer-first", false: "reader-first"}[writerFirst], func(t *testing.T) {
				ale40NativeDiscoveryContainment(t, reader, "native-alias", "child-symlink", writerFirst)
			})
		}
	}
}
func TestALE40PublicPiDiscoveryDoesNotReadSiblingOrExternalTarget(t *testing.T) {
	for _, route := range []string{"standalone", "external-resource"} {
		for _, writerFirst := range []bool{true, false} {
			t.Run(route+"/"+map[bool]string{true: "claude-first", false: "pi-first"}[writerFirst], func(t *testing.T) {
				ale40NativeDiscoveryContainment(t, "pi", "selected-and-global", route, writerFirst)
			})
		}
	}
}
func TestALE40PublicBundleContainsCreatedConfigurationParent(t *testing.T) {
	ale40NativeDiscoveryContainment(t, "pi", "parent-creation", "child-symlink", true)
}
func ale40NativeDiscoveryContainment(t *testing.T, reader, inspection, route string, writerFirst bool) {
	t.Helper()
	fx, id, target := ale40Fixture(t, false)
	body := readBytes(t, target)
	writer := "claude-code"
	settingsDirectory := filepath.Join(fx.home, ".claude")
	if reader == "claude-code" {
		writer = "pi"
		settingsDirectory = filepath.Join(fx.home, ".pi/agent")
	}
	if inspection == "parent-creation" {
		writer = "codex"
		settingsDirectory = fx.home
		if err := os.Rename(filepath.Join(fx.home, ".codex"), filepath.Join(fx.home, "codex-saved")); err != nil {
			t.Fatal(err)
		}
	}
	writerNoOp := inspection == "inspection-only-noop"
	if writerNoOp {
		prior := ale40Preview(t, fx, []string{writer}, "global", id, 0)
		ale40Apply(t, fx, []string{writer}, id, prior.Confirmation, "", 0)
	}
	manifest := []byte("---\nname: configuration-container\ndescription: Inert additional inspected bundle.\n---\nNo execution.\n")
	root := settingsDirectory
	inspected := settingsDirectory
	if route == "standalone" {
		root = filepath.Join(settingsDirectory, "companion.md")
		writeFile(t, root, manifest, 0600)
		inspected = root
	} else if route == "external-resource" {
		root = filepath.Join(fx.home, "independent-bundle")
		writeFile(t, filepath.Join(root, "SKILL.md"), manifest, 0600)
		if err := os.Symlink(filepath.Join(settingsDirectory, "settings.json"), filepath.Join(root, "external-settings.json")); err != nil {
			t.Fatal(err)
		}
		inspected = root
	} else {
		writeFile(t, filepath.Join(root, "SKILL.md"), manifest, 0600)
	}
	if route == "symlink" {
		alias := filepath.Join(fx.home, "configuration-alias")
		if err := os.Symlink(root, alias); err != nil {
			t.Fatal(err)
		}
		root = alias
	} else if route == "child-symlink" {
		container := filepath.Join(fx.home, "child-bundles")
		name := "settings-child"
		if inspection == "native-alias" {
			container = filepath.Join(fx.home, map[string]string{"codex": ".codex/skills", "claude-code": ".claude/skills", "hermes": ".hermes/skills", "gemini": ".gemini/skills"}[reader])
			name = "configuration-container"
		}
		if err := os.MkdirAll(container, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(root, filepath.Join(container, name)); err != nil {
			t.Fatal(err)
		}
		root = container
	}
	extra := []string{}
	if inspection == "inspection-only" || writerNoOp {
		extra = []string{"--root", root}
	} else if inspection == "native-alias" {
		// The reader alone observes its ordinary native child directory alias.
	} else if reader == "pi" {
		settings, err := json.Marshal(map[string]any{"keep": true, "skills": []string{root}})
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), settings, 0600)
	} else {
		settings, err := json.Marshal(map[string]any{"keep": true, "skills": map[string]any{"paths": []string{root}}})
		if err != nil {
			t.Fatal(err)
		}
		config := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
		if inspection == "selected-only" {
			config = filepath.Join(fx.project, "opencode.json")
		}
		writeFile(t, config, settings, 0600)
		if inspection == "global-only" {
			writeFile(t, filepath.Join(fx.project, "opencode.json"), []byte(`{"skills":{"paths":[]}}`), 0600)
		}
	}
	// Confirm the independently inspected bundle and its resource inventory through
	// the public reader context. Global-only OpenCode deliberately hides it in the
	// selected context, so inspect the global context in that case.
	listArgs := []string{"list", "--agent", reader, "--json"}
	if inspection != "no-project" && inspection != "global-only" {
		listArgs = append(listArgs, "--project", fx.project)
	}
	listArgs = append(listArgs, extra...)
	listed := fx.run(t, listArgs...)
	assertExit(t, listed, 0)
	var inventory struct{ Sources []collection.Source }
	decodeJSON(t, listed.stdout, &inventory)
	found := false
	for _, source := range inventory.Sources {
		if source.Directory != canonicalPath(t, inspected) {
			continue
		}
		if route == "standalone" {
			found = source.Identity.Method == "darwin-file-object" && source.Directory == source.Manifest.ResolvedPath && len(source.Resources) == 0
		} else if route == "external-resource" {
			for _, resource := range source.Resources {
				if resource.Kind == "symlink" && resource.LinkScope == "external" {
					found = true
				}
			}
		} else {
			found = len(source.Resources) > 0
		}
	}
	if !found {
		t.Fatal("reader must observe the independent aliased configuration bundle and resources")
	}
	apps := []string{reader, writer}
	if writerFirst {
		apps = []string{writer, reader}
	}
	evidence := "discovery-containment/" + reader + "/" + inspection + "/" + route + "/" + strings.Join(apps, "-")
	writeEvidence(t, evidence+"/inventory.json", listed.stdout)
	run := func(token string) commandResult {
		args := []string{"disable", "--scope", "global", "--json", "--agents", apps[0], "--agents", apps[1]}
		if inspection != "no-project" {
			args = append(args, "--project", fx.project)
		}
		args = append(args, extra...)
		if token != "" {
			args = append(args, "--confirm", token)
		}
		return changeCommand(t, fx, "", append(args, id)...)
	}
	before := treeManifest(t, fx.base)
	result := run("")
	writeEvidence(t, evidence+"/preview.json", result.stdout)
	writeEvidence(t, evidence+"/tree-before.json", before)
	var preview collection.GroupView
	decodeJSON(t, result.stdout, &preview)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if len(preview.Members) != 2 {
		t.Fatal("both requested native previews are required", preview.Members)
	}
	for _, member := range preview.Members {
		if inspection == "parent-creation" && member.Application == writer && member.Preview.CreateDirectory == "" {
			t.Fatal("expected missing configuration parent creation", member.Preview)
		}
		if member.Preview.State != "ready" || member.Preview.NoWriteNeeded != (writerNoOp && member.Application == writer) {
			t.Fatal("must reach discovery containment with native write previews", member.Preview)
		}
	}
	if writerFirst && !writerNoOp && route != "standalone" && route != "external-resource" {
		if preview.State != "refused" || preview.Confirmation != "" || !strings.Contains(preview.Reason, "intersects later native discovery") {
			t.Fatalf("earlier replacement beneath later inspected bundle must refuse: state=%q confirmation=%q reason=%q", preview.State, preview.Confirmation, preview.Reason)
		}
		assertExit(t, result, 1)
		attempted := run("g1:forged")
		assertExit(t, attempted, 1)
		var out collection.GroupOutcome
		decodeJSON(t, attempted.stdout, &out)
		if out.State != "refused" || len(out.Results) != len(apps) {
			t.Fatal(out.State, out.Results)
		}
		for _, result := range out.Results {
			if result.State != "not-attempted" {
				t.Fatal(result)
			}
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	} else {
		assertExit(t, result, 0)
		if preview.State != "ready" || preview.Confirmation == "" {
			t.Fatal(preview.State, preview.Reason)
		}
		applied := run(preview.Confirmation)
		assertExit(t, applied, 0)
		var out collection.GroupOutcome
		decodeJSON(t, applied.stdout, &out)
		if out.State != "applied" || len(out.Results) != len(apps) {
			t.Fatal(out.State, out.Results)
		}
		for _, result := range out.Results {
			if result.State != "succeeded" || result.Outcome == nil || result.Outcome.ExitCode() != 0 {
				t.Fatal(result)
			}
		}
		// Earlier no-ops introduce no resource replacement. Even writer-first
		// now safely verifies the already achieved settings without rewriting.
		apps = []string{writer, reader}
		noOp := run("")
		assertExit(t, noOp, 0)
		decodeJSON(t, noOp.stdout, &preview)
		for _, member := range preview.Members {
			if !member.Preview.NoWriteNeeded {
				t.Fatal("expected verified native no-op", member.Preview)
			}
		}
		unchanged := treeManifest(t, fx.base)
		assertExit(t, run(preview.Confirmation), 0)
		assertTreeUnchanged(t, unchanged, treeManifest(t, fx.base))
	}
	writeEvidence(t, evidence+"/tree-after.json", treeManifest(t, fx.base))
	if !bytes.Equal(body, readBytes(t, target)) {
		t.Fatal("shared source changed")
	}
}

func TestALE40PublicHumanAndUsage(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	apps := []string{"codex", "claude-code"}
	p := ale40Preview(t, fx, apps, "global", id, 0)
	r := changeCommand(t, fx, "", "disable", "--agents", "codex", "--agents", "claude-code", "--scope", "global", "--project", fx.project, "--confirm", p.Confirmation, id)
	assertExit(t, r, 0)
	for _, text := range []string{"Group result: applied", "1. Codex: succeeded", "2. Claude Code: succeeded", "Persistence: verified", "Reload:", "Bounded native discovery"} {
		if !strings.Contains(string(r.stdout), text) {
			t.Fatal("missing human result", text)
		}
	}
	for _, args := range [][]string{{"disable", "--agent", "codex", "--agents", "claude-code", "--scope", "global", id}, {"disable", "--agents", "codex", id}} {
		r := changeCommand(t, fx, "", args...)
		assertExit(t, r, 2)
	}
}
