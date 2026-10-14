package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29ExclusionEvidenceOrder(t *testing.T) {
	cases := []struct {
		name, form, alias, state, pluginRule, aliasRule string
		enabled, safe                                   bool
	}{
		{"disabled-malformed", "malformed", "", "unavailable", "plugin-disabled", "", false, false},
		{"enabled-malformed-control", "malformed", "", "unknown", "manifest-eligibility", "", true, false},
		{"disabled-noncanonical", "noncanonical", "", "unavailable", "plugin-disabled", "", false, false},
		{"enabled-noncanonical-control", "noncanonical", "", "unknown", "unverified-native-manifest-form", "", true, false},
		{"disabled-valid-control", "valid", "", "unavailable", "plugin-disabled", "", false, false},
		{"enabled-valid-control", "valid", "", "available", "local-marketplace-plugin", "", true, false},
		{"reserved-managed-alias", "valid", "synced", "unavailable", "plugin-disabled", "reserved-skill-name", false, false},
		{"safe-managed-alias", "valid", "work", "unavailable", "customization-policy", "customization-policy", false, true},
		{"enabled-reserved-managed-alias-control", "valid", "synced", "available", "local-marketplace-plugin", "reserved-skill-name", true, false},
		{"excluded-plus-unresolved-control", "valid", "work", "unknown", "plugin-disabled", "managed-alias-unverified", false, false},
		{"reserved-plus-unresolved-control", "valid", "synced+work", "unknown", "plugin-disabled", "reserved-skill-name", false, false},
		{"safe-malformed-managed", "malformed", "work", "unavailable", "customization-policy", "customization-policy", false, true},
		{"disabled-skills-shape", "unsupported-skills", "", "unavailable", "plugin-disabled", "", false, false},
		{"enabled-skills-shape-control", "unsupported-skills", "", "unknown", "plugin-manifest-unverified", "", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			fx.claudeSafeMode = tc.safe
			skill := claudeRootPluginFixture(t, fx, nil, false)
			plugin := filepath.Dir(skill)
			if tc.form == "malformed" {
				writeFile(t, skill, []byte("---\nname: [\n---\nInert instructions.\n"), 0600)
			}
			if tc.form == "noncanonical" {
				writeFile(t, skill, []byte("---\nname: Bad_Name\ndescription: Inert.\n---\nInert instructions.\n"), 0600)
			}
			if tc.form == "unsupported-skills" {
				claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "skills": map[string]string{"path": "./skills"}})
				skill = claudeSkill(t, filepath.Join(plugin, "skills"), "child", "action", "")
			}
			bundle := filepath.Dir(skill)
			settings := map[string]any{"enabledPlugins": map[string]bool{"demo@market": tc.enabled}}
			claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), settings)
			aliases := []string{}
			if tc.alias != "" {
				for _, name := range strings.Split(tc.alias, "+") {
					alias := filepath.Join(fx.home, ".claude/skills", name)
					if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(bundle, alias); err != nil {
						t.Fatal(err)
					}
					aliases = append(aliases, alias)
				}
			}
			before := treeManifest(t, fx.base)
			r := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			writeEvidence(t, "exclusions/"+tc.name+"-list.json", r.stdout)
			writeEvidence(t, "exclusions/"+tc.name+"-stderr.txt", r.stderr)
			wantExit := 0
			if tc.form == "malformed" || tc.form == "unsupported-skills" {
				wantExit = 1
			}
			assertExit(t, r, wantExit)
			var list codexList
			decodeJSON(t, r.stdout, &list)
			s := claudeAt(t, list, bundle)
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, s.ID)
			writeEvidence(t, "exclusions/"+tc.name+"-explain.txt", detail.stdout)
			assertExit(t, detail, wantExit)
			for _, state := range []string{"unknown", "unavailable", "available"} {
				f := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json", "--availability", state)
				writeEvidence(t, "exclusions/"+tc.name+"-"+state+"-filter.json", f.stdout)
				assertExit(t, f, wantExit)
				var filtered codexList
				decodeJSON(t, f.stdout, &filtered)
				found := false
				for _, v := range filtered.Sources {
					found = found || v.ID == s.ID
				}
				if found != (tc.state == state) {
					t.Errorf("%s filter source membership=%t want %t", state, found, tc.state == state)
				}
			}
			if s.Availability.State != tc.state {
				t.Errorf("state=%s want %s", s.Availability.State, tc.state)
			}
			if !strings.Contains(string(detail.stdout), "Availability: "+tc.state) {
				t.Errorf("explain lacks expected Availability: %s", tc.state)
			}
			if len(s.Availability.References) != 1+len(aliases) {
				t.Errorf("shared reference count=%d want %d", len(s.Availability.References), 1+len(aliases))
			}
			if s.Ownership.State != "managed" {
				t.Errorf("managed provenance lost: %s", s.Ownership.State)
			}
			var public publicList
			decodeJSON(t, r.stdout, &public)
			refPaths := map[string]string{}
			for _, ref := range public.References {
				if ref.SourceID == s.ID {
					refPaths[ref.ID] = ref.DiscoveredPath
				}
			}
			if len(refPaths) != 1+len(aliases) {
				t.Errorf("public reference identities lost: %v", refPaths)
			}
			for _, fact := range s.Availability.References {
				path, found := refPaths[fact.ReferenceID]
				if !found {
					t.Fatalf("availability reference absent from public inventory: %s", fact.ReferenceID)
				}
				rule := tc.pluginRule
				participation := "excluded"
				if path != bundle {
					rule = tc.aliasRule
					if filepath.Base(path) == "work" && tc.alias == "synced+work" {
						rule = "managed-alias-unverified"
					}
				}
				if rule == "local-marketplace-plugin" {
					participation = "eligible"
				}
				if rule == "manifest-eligibility" || rule == "unverified-native-manifest-form" || rule == "managed-alias-unverified" || rule == "plugin-manifest-unverified" {
					participation = "unknown"
				}
				if fact.Participation != participation || fact.Evidence.Rule != rule {
					t.Errorf("route %s=%s/%s want %s/%s", path, fact.Participation, fact.Evidence.Rule, participation, rule)
				}
				if !strings.Contains(string(detail.stdout), "Reference "+fact.ReferenceID+": "+participation) || !strings.Contains(string(detail.stdout), "Discovery rule: "+rule+" |") {
					t.Errorf("explain contradicts reference %s participation/rule", fact.ReferenceID)
				}
				if fact.Provenance.PluginID != "demo@market" || fact.Provenance.Kind != "plugin" {
					t.Errorf("plugin identity lost: %+v", fact)
				}
			}
			t.Logf("source=%s state=%s primary=%s exits=%d/%d refs=%+v", s.ID, s.Availability.State, s.Availability.Evidence.Rule, r.code, detail.code, s.Availability.References)
			if wantExit == 1 && public.Scope.Completeness != "incomplete" {
				t.Errorf("invalid input lost incomplete coverage: %s", public.Scope.Completeness)
			}
			if tc.form == "malformed" {
				found := false
				for _, v := range public.Sources {
					if v.ID == s.ID {
						found = v.Manifest.Status == "malformed" && len(v.Problems) > 0
					}
				}
				if !found || !strings.Contains(string(detail.stdout), "malformed-manifest") {
					t.Error("malformed diagnostic disappeared")
				}
			}
			if tc.form == "unsupported-skills" {
				assertProblem(t, public.Problems, "native-input", filepath.Join(plugin, ".claude-plugin/plugin.json"), true)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
