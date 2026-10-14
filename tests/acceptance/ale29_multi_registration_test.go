package acceptance_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type multiCase struct {
	name         string
	count        int
	outer, inner bool
	topology     string
	alias        string
	repeats      int
}

func TestDecisionSharedPluginRegistrations(t *testing.T) {
	cases := []multiCase{
		{"single-enabled", 1, true, false, "local", "", 1},
		{"single-disabled", 1, false, false, "local", "", 1},
		{"equal-disabled", 2, false, false, "local", "", 4},
		{"equal-enabled", 2, true, true, "local", "", 4},
		{"opposite", 2, false, true, "local", "", 24},
		{"opposite-reversed", 2, true, false, "local", "", 24},
		{"local-independent-cache", 1, true, false, "independent-cache", "", 1},
		{"cache-path-collision", 1, true, false, "cache-collision", "", 4},
		{"physical-path-alias", 2, false, true, "physical-alias", "", 4},
		{"ordinary-alias-all-disabled", 2, false, false, "local", "native", 1},
		{"viewer-alias-all-disabled", 2, false, false, "local", "viewer", 1},
		{"shared-root-overlap", 2, false, true, "shared-root", "", 1},
		{"installed-id-overlay", 1, true, false, "installed-overlay", "", 1},
		{"nonapplicable-overlay", 1, true, false, "nonapplicable-overlay", "", 1},
		{"inactive-overlay", 1, true, false, "inactive-overlay", "", 1},
		{"duplicate-source-declarations", 1, true, false, "duplicate-declarations", "", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			if err := os.MkdirAll(filepath.Join(fx.project, ".git", "objects"), 0700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.project, ".git", "HEAD"), []byte("ref: refs/heads/fixture\n"), 0600)
			outer := filepath.Join(fx.base, "allowed", "market-outer")
			inner := filepath.Join(outer, "nested")
			plugin := filepath.Join(inner, "demo")
			if tc.topology == "cache-collision" {
				plugin = filepath.Join(fx.home, ".claude", "plugins", "cache", "cache-market", "demo", "1")
			}
			bundle := filepath.Dir(claudeSkill(t, filepath.Join(plugin, "skills"), "work", "work", ""))
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"), map[string]any{"name": "demo", "version": "1"})
			sourceForOuter := "./nested/demo"
			if tc.topology == "shared-root" {
				sourceForOuter = "./nested"
				claudeSettings(t, filepath.Join(inner, ".claude-plugin", "plugin.json"), map[string]any{"name": "demo", "version": "1", "skills": "./demo/skills"})
			}
			if tc.topology == "cache-collision" {
				outer = filepath.Join(fx.home, ".claude", "plugins", "cache", "cache-market")
				sourceForOuter = "./demo/1"
			}
			markets := map[string]any{}
			register := func(name, market, source string) {
				claudeSettings(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), map[string]any{"name": name, "plugins": []map[string]any{{"name": "demo", "source": source}}})
				markets[name] = map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-01-01T00:00:00Z"}
			}
			register("outer", outer, sourceForOuter)
			duplicateBundle := ""
			if tc.topology == "duplicate-declarations" {
				copyPlugin := filepath.Join(inner, "independent")
				duplicateBundle = filepath.Dir(claudeSkill(t, filepath.Join(copyPlugin, "skills"), "work", "work", ""))
				claudeSettings(t, filepath.Join(copyPlugin, ".claude-plugin", "plugin.json"), map[string]any{"name": "demo", "version": "1"})
				claudeSettings(t, filepath.Join(outer, ".claude-plugin", "marketplace.json"), map[string]any{"name": "outer", "plugins": []map[string]any{{"name": "demo", "source": sourceForOuter}, {"name": "demo", "source": "./nested/independent"}}})
			}
			innerPlugin := plugin
			if tc.count == 2 {
				innerSource := "./demo"
				if tc.topology == "physical-alias" {
					innerPlugin = filepath.Join(inner, "alias-demo")
					if err := os.Symlink(plugin, innerPlugin); err != nil {
						t.Fatal(err)
					}
					innerSource = "./alias-demo"
				}
				register("inner", inner, innerSource)
			}
			claudeSettings(t, filepath.Join(fx.home, ".claude", "plugins", "known_marketplaces.json"), markets)
			cacheCopy := ""
			installOuter := plugin
			if tc.topology == "shared-root" {
				installOuter = inner
			}
			if tc.topology == "independent-cache" {
				cachePlugin := filepath.Join(fx.home, ".claude", "plugins", "cache", "outer", "demo", "1")
				cacheCopy = filepath.Dir(claudeSkill(t, filepath.Join(cachePlugin, "skills"), "work", "work", ""))
				claudeSettings(t, filepath.Join(cachePlugin, ".claude-plugin", "plugin.json"), map[string]any{"name": "demo", "version": "1"})
				installOuter = cachePlugin
			}
			installed := map[string]any{"demo@outer": []map[string]any{{"scope": "user", "installPath": installOuter, "version": "1"}}}
			if tc.count == 2 {
				installed["demo@inner"] = []map[string]any{{"scope": "user", "installPath": innerPlugin, "version": "1"}}
			}
			if strings.HasSuffix(tc.topology, "overlay") {
				entry := map[string]any{"scope": "user", "installPath": plugin, "version": "1"}
				if tc.topology == "nonapplicable-overlay" {
					entry["scope"] = "project"
					entry["projectPath"] = filepath.Join(fx.base, "allowed", "other-project")
				}
				if tc.topology == "inactive-overlay" {
					entry["version"] = "other"
				}
				installed["demo@overlay"] = []map[string]any{entry}
			}
			claudeSettings(t, filepath.Join(fx.home, ".claude", "plugins", "installed_plugins.json"), map[string]any{"version": 2, "plugins": installed})
			enabled := map[string]bool{"demo@outer": tc.outer}
			if tc.count == 2 {
				enabled["demo@inner"] = tc.inner
			}
			if tc.topology == "cache-collision" {
				enabled["demo@cache-market"] = false
			}
			if strings.HasSuffix(tc.topology, "overlay") {
				enabled["demo@overlay"] = tc.topology != "installed-overlay"
			}
			claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": enabled})
			extra := []string{}
			if tc.alias != "" {
				aliasRoot := filepath.Join(fx.home, ".claude", "skills")
				if tc.alias == "viewer" {
					aliasRoot = filepath.Join(fx.base, "allowed", "inspection")
					extra = []string{"--root", aliasRoot}
				}
				if err := os.MkdirAll(aliasRoot, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(bundle, filepath.Join(aliasRoot, "alias")); err != nil {
					t.Fatal(err)
				}
			}
			profile, err := os.ReadFile(fx.policy)
			if err != nil {
				t.Fatal(err)
			}
			writeEvidence(t, "multireg/"+tc.name+"-policy.sb", profile)
			before := treeManifest(t, fx.base)
			multiEvidence(t, "multireg/"+tc.name+"-fixture.json", map[string]any{"root": fx.base, "bundle": bundle, "cache_copy": cacheCopy, "case": tc, "policy": fx.policy})
			outcomes := map[string]int{}
			var first codexSource
			missing := false
			for i := 0; i < tc.repeats; i++ {
				list := claudeListResult(t, fx, 0, extra...)
				source := claudeAt(t, list, bundle)
				if i == 0 {
					first = source
				} else if !reflect.DeepEqual(first, source) {
					t.Errorf("unchanged inputs changed source associations or confidence")
				}
				wantState := "available"
				if !tc.outer {
					wantState = "unavailable"
				}
				if tc.count == 2 && (tc.outer || tc.inner) || tc.topology == "installed-overlay" || tc.topology == "duplicate-declarations" || tc.alias == "native" {
					wantState = "unknown"
				}
				if source.Availability.State != wantState {
					t.Errorf("state=%s want %s", source.Availability.State, wantState)
				}
				if wantState == "unknown" && (source.Availability.Precedence.State != "unknown" || source.Availability.Precedence.Winner != "") {
					t.Error("unverified multi-registration winner became definite")
				}
				if duplicateBundle != "" {
					other := claudeAt(t, list, duplicateBundle)
					if other.ID == source.ID || other.Availability.State != "unknown" {
						t.Error("duplicate declarations lost independent source or confidence")
					}
				}
				ids := map[string]bool{}
				for _, fact := range source.Availability.References {
					id := fact.Provenance.PluginID
					if id != "" {
						ids[id] = true
						flag, known := enabled[id]
						if known && fact.Provenance.Setting.Setting != fmt.Sprintf("enabledPlugins.%s=%t", id, flag) {
							t.Errorf("route copied another registration setting: %+v", fact)
						}
						if fact.Provenance.Evidence.InputPath == "" || len(fact.Provenance.Records) == 0 {
							t.Error("route lost manifest/producer provenance")
						}
						marketRecords, installRecords := 0, 0
						for _, record := range fact.Provenance.Records {
							if record.Kind == "marketplace" {
								marketRecords++
								if record.RegistrationInput == "" || record.DeclaredSource == "" || string(record.LastUpdated) != `"2026-01-01T00:00:00Z"` {
									t.Error("marketplace declaration lost original inputs")
								}
							}
							if record.Kind == "installation" {
								installRecords++
								if record.Scope == "" || record.Applicability == "" || len(record.Version) == 0 {
									t.Error("installation record lost shape or applicability")
								}
							}
						}
						if id == "demo@outer" && (marketRecords == 0 || installRecords == 0) || tc.topology == "duplicate-declarations" && id == "demo@outer" && marketRecords != 2 {
							t.Error("registration lost producer observations")
						}
						if tc.count == 2 && (tc.outer || tc.inner) && flag && fact.Provenance.Activation.Kind != "unknown" {
							t.Error("shared-source registration retained positive activation confidence")
						}
						if fact.NativeName != "" && fact.NativeName != "demo:work" && tc.alias == "" {
							t.Error("registration changed raw plugin namespace")
						}
						if known && !flag && fact.NativeName == "demo:work" && (fact.Participation != "excluded" || fact.Evidence.Rule != "plugin-disabled" || fact.Evidence.Setting != "enabledPlugins."+id+"=false") {
							t.Error("registration-specific disable evidence lost")
						}
					}
				}
				if tc.topology == "cache-collision" && !ids["demo@cache-market"] || strings.HasSuffix(tc.topology, "overlay") && !ids["demo@overlay"] {
					t.Error("installed/cache producer provenance lost")
				}
				names := []string{}
				for id := range ids {
					names = append(names, id)
				}
				sort.Strings(names)
				key := source.Availability.State + "|" + strings.Join(names, ",")
				outcomes[key]++
				if !ids["demo@outer"] || (tc.count == 2 && !ids["demo@inner"]) {
					missing = true
				}
				if tc.count == 1 && tc.topology == "local" && tc.alias == "" {
					want := "available"
					if !tc.outer {
						want = "unavailable"
					}
					if source.Availability.State != want {
						t.Errorf("single registration state got %s want %s", source.Availability.State, want)
					}
				}
				if cacheCopy != "" {
					copy := claudeAt(t, list, cacheCopy)
					if copy.ID == source.ID || copy.Availability.State != "unavailable" {
						t.Errorf("independent cache control: %+v", copy)
					}
				}
			}
			explainArgs := append([]string{"explain", "--agent", "claude-code", "--project", fx.project}, extra...)
			human := fx.run(t, append(explainArgs, first.ID)...)
			assertExit(t, human, 0)
			detail := fx.run(t, append(explainArgs, "--json", first.ID)...)
			assertExit(t, detail, 0)
			var explained struct{ Source codexSource }
			decodeJSON(t, detail.stdout, &explained)
			if !reflect.DeepEqual(first.Availability, explained.Source.Availability) {
				t.Error("list/explain route parity lost")
			}
			for id := range enabled {
				if !strings.Contains(string(human.stdout), "Plugin: "+id) || !strings.Contains(string(human.stdout), fmt.Sprintf("enabledPlugins.%s=%t", id, enabled[id])) {
					t.Errorf("human explanation lost %s or its own setting", id)
				}
			}
			for _, state := range []string{"available", "unavailable", "unknown"} {
				filtered := claudeListResult(t, fx, 0, append(extra, "--availability", state)...)
				found := false
				for _, candidate := range filtered.Sources {
					found = found || candidate.ID == first.ID
				}
				if found != (first.Availability.State == state) {
					t.Errorf("%s filter membership lost", state)
				}
			}
			after := treeManifest(t, fx.base)
			writeEvidence(t, "multireg/"+tc.name+"-tree-before.json", before)
			writeEvidence(t, "multireg/"+tc.name+"-tree-after.json", after)
			multiEvidence(t, "multireg/"+tc.name+"-outcomes.json", outcomes)
			assertTreeUnchanged(t, before, after)
			if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
				t.Fatal("fixture instructions executed")
			}
			t.Logf("%s outcomes: %v", tc.name, outcomes)
			if tc.name == "ordinary-alias-all-disabled" && first.Availability.State != "unknown" {
				t.Errorf("ordinary alias uncertainty lost: %+v", first.Availability)
			}
			if tc.name == "viewer-alias-all-disabled" && first.Availability.State != "unavailable" {
				t.Errorf("inspection alias defeats exclusion: %+v", first.Availability)
			}
			if missing {
				t.Errorf("registration provenance lost: require demo@outer%s in every public source", func() string {
					if tc.count == 2 {
						return " and demo@inner"
					}
					return ""
				}())
			}
			if tc.count == 2 && tc.outer != tc.inner && len(outcomes) > 1 {
				t.Errorf("unchanged public inputs yield different plugin IDs/states: %v", outcomes)
			}
		})
	}
}

func TestDecisionSharedRegistrationContainment(t *testing.T) {
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "positive")
	outside := filepath.Join(fx.base, "excluded", "outside")
	system := filepath.Join(fx.base, "excluded", "system-inputs", "managed-settings.json")
	write := filepath.Join(fx.base, "excluded", "write")
	link := filepath.Join(fx.base, "allowed", "escape")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, outside, []byte("outside inert control"), 0600)
	writeFile(t, system, []byte("system inert control"), 0600)
	for _, p := range []string{allowed, outside, system} {
		if _, e := os.ReadFile(p); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(write, []byte("positive write"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(write); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Dir(write), link); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	connection, e := net.Dial("tcp", listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	connection.Close()
	original := skillglassBinary
	skillglassBinary = isolationProbeBinary
	defer func() { skillglassBinary = original }()
	before := treeManifest(t, fx.base)
	result := fx.run(t, "fixture-policy", allowed, outside, system, write, filepath.Join(link, "write"), listener.Addr().String())
	assertExit(t, result, 0)
	var controls struct {
		Passed bool
		Checks map[string]bool
	}
	if e := json.Unmarshal(result.stdout, &controls); e != nil {
		t.Fatal(e)
	}
	if !controls.Passed || len(controls.Checks) != 16 {
		t.Fatalf("controls failed: %s", result.stdout)
	}
	writeEvidence(t, "multireg/containment.json", result.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	originalPolicy, e := os.ReadFile(fx.policy)
	if e != nil {
		t.Fatal(e)
	}
	writeEvidence(t, "multireg/control-policy.sb", originalPolicy)
	writeFile(t, fx.policy, []byte("invalid sandbox policy"), 0600)
	invalid := fx.run(t, "fixture-policy", allowed, outside, system, write, filepath.Join(link, "write"), listener.Addr().String())
	if invalid.code == 0 || len(invalid.stdout) > 0 {
		t.Fatal("invalid policy did not fail closed")
	}
	writeEvidence(t, "multireg/invalid-policy-stderr.txt", invalid.stderr)
}

func multiEvidence(t *testing.T, path string, value any) {
	t.Helper()
	raw, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	writeEvidence(t, path, append(raw, '\n'))
}
