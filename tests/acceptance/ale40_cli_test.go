package acceptance_test

import (
	"bytes"
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
