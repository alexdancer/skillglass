package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestDecision3ValidSkillsArrays(t *testing.T) {
	fx, _ := codexFixture(t)
	plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
	id := ""
	for _, tc := range []struct {
		name    string
		value   []string
		present bool
	}{{"skills-array-dot", []string{"."}, true}, {"skills-array-empty", []string{}, false}, {"skills-array-dot-repeat", []string{"."}, true}} {
		t.Run(tc.name, func(t *testing.T) {
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "skills": tc.value})
			before := treeManifest(t, fx.base)
			r := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			assertExit(t, r, 0)
			writeEvidence(t, "arrays/"+tc.name+"-list.json", r.stdout)
			var list codexList
			decodeJSON(t, r.stdout, &list)
			var found *codexSource
			for _, s := range list.Sources {
				if s.SourcePath == plugin {
					v := s
					found = &v
				}
			}
			if !tc.present {
				if found != nil {
					t.Fatal("empty explicit skills array unexpectedly discovered root fallback")
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			if found == nil {
				t.Fatal("valid skills array did not discover root source")
			}
			s := *found
			if id == "" {
				id = s.ID
			}
			if s.ID != id {
				t.Error("array presence changed source identity")
			}
			if s.Availability.State != "available" || s.Ownership.State != "managed" || s.Availability.NativeName != "demo:root-action" {
				t.Fatal("valid skills array lost availability/ownership/name")
			}
			for _, reference := range s.Availability.References {
				if reference.Participation != "eligible" {
					t.Error("valid skills array lost eligibility")
				}
			}
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, s.ID)
			assertExit(t, detail, 0)
			writeEvidence(t, "arrays/"+tc.name+"-explain.txt", detail.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
