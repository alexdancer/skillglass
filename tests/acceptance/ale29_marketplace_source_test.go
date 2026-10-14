package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29MarketplaceSourceShape(t *testing.T) {
	cases := []struct {
		name             string
		source           any
		present          bool
		child            bool
		timestampMissing bool
		disabled         bool
		nonApplicable    bool
		unregistered     bool
		cacheOnly        bool
		rejected         bool
	}{
		{name: "null", present: true, rejected: true},
		{name: "absent", rejected: true},
		{name: "number", source: 17, present: true, rejected: true},
		{name: "boolean", source: false, present: true, rejected: true},
		{name: "array", source: []string{"."}, present: true, rejected: true},
		{name: "remote-object", source: map[string]string{"source": "github", "repo": "fixture/demo"}, present: true, rejected: true},
		{name: "dot-root", source: ".", present: true},
		{name: "slash-root", source: "./", present: true},
		{name: "child", source: "./demo", present: true, child: true},
		{name: "absolute", source: "ABSOLUTE_FIXTURE", present: true, rejected: true},
		{name: "escape", source: "../outside", present: true, rejected: true},
		{name: "timestamp-missing", source: ".", present: true, timestampMissing: true},
		{name: "null-timestamp-missing", present: true, timestampMissing: true, rejected: true},
		{name: "disabled", source: ".", present: true, disabled: true},
		{name: "null-disabled", present: true, disabled: true, rejected: true},
		{name: "nonapplicable", source: ".", present: true, nonApplicable: true},
		{name: "null-nonapplicable", present: true, nonApplicable: true, rejected: true},
		{name: "unregistered", source: ".", present: true, unregistered: true},
		{name: "null-cache-only", present: true, cacheOnly: true, rejected: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			ordinary := filepath.Dir(claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "ordinary", "ordinary", ""))
			market := filepath.Join(fx.base, "allowed", "market")
			local := market
			if tc.child {
				local = filepath.Join(market, "demo")
			}
			cache := filepath.Join(fx.home, ".claude", "plugins", "cache", "market", "demo", "1")
			for _, bundle := range []string{local, cache} {
				claudeSkill(t, filepath.Dir(bundle), filepath.Base(bundle), "action", "")
				claudeSettings(t, filepath.Join(bundle, ".claude-plugin", "plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "skills": "."})
			}
			writeFile(t, filepath.Join(local, "SKILL.md"), []byte("---\nname: action\ndescription: LOCAL_SOURCE\n---\nInert. Never execute.\n"), 0600)
			writeFile(t, filepath.Join(cache, "SKILL.md"), []byte("---\nname: action\ndescription: INDEPENDENT_CACHE\n---\nInert. Never execute.\n"), 0600)
			source := tc.source
			if tc.name == "absolute" {
				source = local
			}
			plugin := map[string]any{"name": "demo"}
			if tc.present {
				plugin["source"] = source
			}
			claudeSettings(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), map[string]any{"name": "market", "owner": map[string]string{"name": "Fixture"}, "plugins": []any{plugin}})
			plugins := filepath.Join(fx.home, ".claude", "plugins")
			record := map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market}
			if !tc.timestampMissing {
				record["lastUpdated"] = "2026-10-13T00:00:00Z"
			}
			claudeSettings(t, filepath.Join(plugins, "known_marketplaces.json"), map[string]any{"market": record})
			entries := []map[string]any{}
			if !tc.unregistered {
				paths := []string{local, cache}
				if tc.cacheOnly {
					paths = []string{cache}
				}
				for _, path := range paths {
					entry := map[string]any{"scope": "user", "installPath": path, "version": "1.0.0"}
					if tc.nonApplicable {
						entry["scope"] = "project"
						entry["projectPath"] = filepath.Join(fx.base, "allowed", "other-project")
					}
					entries = append(entries, entry)
				}
			}
			registry := map[string]any{}
			if !tc.unregistered {
				registry["demo@market"] = entries
			}
			claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": registry})
			claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": !tc.disabled}, "permissions": map[string]any{"ask": []string{"Skill(ordinary)"}}})
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			wantExit := 0
			if tc.timestampMissing {
				wantExit = 1
			}
			assertExit(t, result, wantExit)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			writeEvidence(t, "source-shape/"+tc.name+"-list.json", result.stdout)
			hasNativeInput := false
			for _, problem := range list.Problems {
				hasNativeInput = hasNativeInput || problem.Code == "native-input"
			}
			if hasNativeInput != tc.timestampMissing {
				t.Errorf("native-input diagnostic=%t want %t", hasNativeInput, tc.timestampMissing)
			}
			completeness := "complete"
			if tc.timestampMissing {
				completeness = "incomplete"
			}
			if list.Scope.Completeness != completeness {
				t.Errorf("coverage=%s want %s", list.Scope.Completeness, completeness)
			}
			expectedGap := tc.rejected
			hasGap := false
			for _, gap := range list.Scope.Agent.Gaps {
				hasGap = hasGap || strings.Contains(gap, "unverified local plugin source form: demo@market") || strings.Contains(gap, "local plugin source escapes marketplace: demo@market")
			}
			if hasGap != expectedGap {
				t.Errorf("source-form gap=%t want %t", hasGap, expectedGap)
			}
			ord := claudeAt(t, list, ordinary)
			if ord.Availability.State != "ask" {
				t.Errorf("ordinary permission changed: %s", ord.Availability.State)
			}
			cached := claudeAt(t, list, cache)
			for _, filteredState := range []string{"unknown", "available", "unavailable"} {
				filteredResult := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--availability", filteredState, "--json")
				assertExit(t, filteredResult, wantExit)
				writeEvidence(t, "source-shape/"+tc.name+"-"+filteredState+".json", filteredResult.stdout)
				var filtered codexList
				decodeJSON(t, filteredResult.stdout, &filtered)
				expectedCache := "unknown"
				if tc.disabled || tc.nonApplicable || (!tc.rejected && !tc.timestampMissing && !tc.unregistered) {
					expectedCache = "unavailable"
				}
				found := false
				for _, s := range filtered.Sources {
					found = found || s.ID == cached.ID
				}
				if found != (filteredState == expectedCache) {
					t.Errorf("cache %s filter membership=%t want %t", filteredState, found, filteredState == expectedCache)
				}
				expectedLocal := "available"
				if tc.rejected || tc.timestampMissing || tc.unregistered {
					expectedLocal = "unknown"
				}
				if tc.disabled || tc.nonApplicable {
					expectedLocal = "unavailable"
				}
				foundLocal := false
				for _, candidate := range filtered.Sources {
					foundLocal = foundLocal || candidate.SourcePath == local
				}
				wantLocal := !tc.cacheOnly && filteredState == expectedLocal
				if foundLocal != wantLocal {
					t.Errorf("local %s filter membership=%t want %t", filteredState, foundLocal, wantLocal)
				}
			}
			localFound := false
			for _, candidate := range list.Sources {
				if candidate.SourcePath == local {
					localFound = true
				}
			}
			if tc.cacheOnly {
				if localFound {
					t.Errorf("null source fabricated a local managed root with cache-only registration")
				}
			} else {
				active := claudeAt(t, list, local)
				if active.ID == cached.ID {
					t.Fatal("independent source/cache merged")
				}
				for _, bundle := range []struct {
					source  codexSource
					isCache bool
				}{{active, false}, {cached, true}} {
					want := "available"
					participation := "eligible"
					rule := "local-marketplace-plugin"
					if bundle.isCache {
						want = "unavailable"
						participation = "excluded"
						rule = "local-plugin-cache-copy"
					}
					if tc.rejected || tc.unregistered {
						want = "unknown"
						participation = "unknown"
						rule = "plugin-activation-unverified"
					}
					if tc.timestampMissing && !tc.rejected {
						want = "unknown"
						participation = "unknown"
						rule = "local-marketplace-unverified"
					}
					if tc.disabled {
						want = "unavailable"
						participation = "excluded"
						rule = "plugin-disabled"
					}
					if tc.nonApplicable {
						want = "unavailable"
						participation = "excluded"
						rule = "inactive-plugin-installation"
					}
					if bundle.source.Availability.State != want {
						t.Errorf("%s state=%s want literal %s", bundle.source.SourcePath, bundle.source.Availability.State, want)
					}
					if bundle.source.Ownership.State != "managed" || bundle.source.Availability.NativeName != "demo:action" {
						t.Errorf("lost independent registered provenance or namespace: %+v", bundle.source)
					}
					if len(bundle.source.Availability.References) != 1 {
						t.Fatalf("references=%d", len(bundle.source.Availability.References))
					}
					ref := bundle.source.Availability.References[0]
					if ref.Participation != participation || ref.Evidence.Rule != rule {
						t.Errorf("reference=%s/%s want %s/%s", ref.Participation, ref.Evidence.Rule, participation, rule)
					}
					if ref.Provenance.PluginID != "demo@market" || ref.Permission.State != "allowed" {
						t.Errorf("provenance/permission changed: %+v", ref)
					}
					detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, bundle.source.ID)
					assertExit(t, detail, wantExit)
					writeEvidence(t, "source-shape/"+tc.name+"-"+bundle.source.ID+"-explain.txt", detail.stdout)
					if !strings.Contains(string(detail.stdout), "Availability: "+want) || !strings.Contains(string(detail.stdout), rule) {
						t.Errorf("human explain lacks literal state/rule %s/%s", want, rule)
					}
					detailJSON := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, "--json", bundle.source.ID)
					assertExit(t, detailJSON, wantExit)
					writeEvidence(t, "source-shape/"+tc.name+"-"+bundle.source.ID+"-explain.json", detailJSON.stdout)
					var decoded struct{ Source codexSource }
					decodeJSON(t, detailJSON.stdout, &decoded)
					left, _ := json.Marshal(bundle.source.Availability)
					right, _ := json.Marshal(decoded.Source.Availability)
					if string(left) != string(right) {
						t.Errorf("list/explain availability parity lost")
					}
				}
			}
			if tc.cacheOnly {
				if cached.Availability.State != "unknown" || len(cached.Availability.References) != 1 || cached.Ownership.State != "managed" || cached.Availability.NativeName != "demo:action" {
					t.Fatalf("cache-only null source lost unknown managed inventory: %+v", cached)
				}
				ref := cached.Availability.References[0]
				if ref.Participation != "unknown" || ref.Evidence.Rule != "plugin-activation-unverified" || ref.Provenance.PluginID != "demo@market" || ref.Permission.State != "allowed" {
					t.Errorf("cache-only null source manufactured activation: %+v", ref)
				}
				for _, format := range []string{"human", "json"} {
					args := []string{"explain", "--agent", "claude-code", "--project", fx.project}
					if format == "json" {
						args = append(args, "--json")
					}
					detail := fx.run(t, append(args, cached.ID)...)
					assertExit(t, detail, 0)
					writeEvidence(t, "source-shape/"+tc.name+"-cache-explain-"+format, detail.stdout)
					if format == "json" {
						var decoded struct{ Source codexSource }
						decodeJSON(t, detail.stdout, &decoded)
						left, _ := json.Marshal(cached.Availability)
						right, _ := json.Marshal(decoded.Source.Availability)
						if string(left) != string(right) {
							t.Error("cache-only list/explain availability parity lost")
						}
					} else if !strings.Contains(string(detail.stdout), "Availability: unknown") || !strings.Contains(string(detail.stdout), "plugin-activation-unverified") {
						t.Error("cache-only human explanation gained activation confidence")
					}
				}
			}
			after := treeManifest(t, fx.base)
			writeEvidence(t, "source-shape/"+tc.name+"-tree-before.json", before)
			writeEvidence(t, "source-shape/"+tc.name+"-tree-after.json", after)
			assertTreeUnchanged(t, before, after)
			if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
				t.Fatal("fixture instructions executed")
			}
		})
	}
}
