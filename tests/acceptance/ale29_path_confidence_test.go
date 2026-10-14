package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29SkillsPathConfidence(t *testing.T) {
	cases := []struct {
		name        string
		declaration any
		expected    string
		roots       bool
	}{
		{"unprefixed-string", "extra", "unknown", false},
		{"empty-string", "", "unknown", true},
		{"unprefixed-array-element", []string{"./extra", "extra"}, "unknown", false},
		{"empty-array-element", []string{"./extra", ""}, "unknown", true},
		{"supported-extra", "./extra", "available", false},
		{"supported-missing", "./missing", "available", false},
		{"supported-array-missing", []string{"./extra", "./missing"}, "available", false},
		{"supported-internal-dotdot", "./extra/../extra", "available", false},
		{"supported-dot", ".", "available", true},
		{"supported-dot-slash", "./", "available", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			plugin := filepath.Dir(claudeRootPluginFixture(t, fx, tc.declaration, true))
			def := filepath.Dir(claudeSkill(t, filepath.Join(plugin, "skills"), "default", "default-action", ""))
			extra := filepath.Dir(claudeSkill(t, filepath.Join(plugin, "extra"), "extra", "extra-action", ""))
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			if result.code != 0 && result.code != 1 {
				t.Fatalf("unexpected list exit %d", result.code)
			}
			writeEvidence(t, "decision-paths/"+tc.name+"-list.json", result.stdout)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			filteredResult := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--availability", "unknown", "--json")
			if filteredResult.code != 0 && filteredResult.code != 1 {
				t.Fatalf("unexpected filtered list exit %d", filteredResult.code)
			}
			writeEvidence(t, "decision-paths/"+tc.name+"-unknown-filter.json", filteredResult.stdout)
			var filtered codexList
			decodeJSON(t, filteredResult.stdout, &filtered)
			paths := []string{def}
			if tc.roots {
				paths = append(paths, plugin)
			} else if tc.name != "supported-missing" && tc.name != "supported-dot" && tc.name != "supported-dot-slash" {
				paths = append(paths, extra)
			}
			for _, path := range paths {
				source := claudeAt(t, list, path)
				if source.Ownership.State != "managed" {
					t.Errorf("lost observed managed inventory: %s", source.Ownership.State)
				}
				if source.Availability.State != tc.expected {
					t.Errorf("%s source %s state=%s want literal %s", tc.name, path, source.Availability.State, tc.expected)
				}
				inFilter := false
				for _, candidate := range filtered.Sources {
					inFilter = inFilter || candidate.ID == source.ID
				}
				if inFilter != (tc.expected == "unknown") {
					t.Errorf("unknown filter membership at %s: %t", path, inFilter)
				}
				const diagnostic = "skills paths must be exactly \".\" or start with \"./\""
				if len(source.Availability.References) == 0 {
					t.Fatal("lost managed discovery references")
				}
				for _, ref := range source.Availability.References {
					want := "eligible"
					if tc.expected == "unknown" {
						want = "unknown"
						if ref.Evidence.Rule != "plugin-manifest-unverified" || ref.Evidence.Reason != source.Availability.Evidence.Reason || !strings.Contains(ref.Evidence.Reason, diagnostic) {
							t.Errorf("reference path explanation lost primary parity: %+v", ref)
						}
					}
					if ref.Participation != want || ref.Provenance.PluginID != "demo@market" {
						t.Errorf("participation or provenance at %s: %+v", path, ref)
					}
				}
				if tc.expected == "unknown" && (source.Availability.Evidence.Rule != "plugin-manifest-unverified" || !strings.Contains(source.Availability.Evidence.Reason, diagnostic)) {
					t.Errorf("primary path explanation lost: %+v", source.Availability.Evidence)
				}
				detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
				if detail.code != 0 && detail.code != 1 {
					t.Fatalf("unexpected explain exit %d", detail.code)
				}
				writeEvidence(t, "decision-paths/"+tc.name+"-"+source.ID+"-explain.txt", detail.stdout)
				if tc.expected == "unknown" && !strings.Contains(string(detail.stdout), diagnostic) {
					t.Errorf("explain lacks specific skills path reason: %s", detail.stdout)
				}
				if !strings.Contains(string(detail.stdout), "Availability: "+tc.expected) {
					t.Errorf("explain lacks literal expected state %s", tc.expected)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
