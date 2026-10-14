package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentALE29HomepagePublicControls(t *testing.T) {
	for _, c := range []struct{ name, homepage string }{{"absent", ""}, {"valid", "https://example.invalid/docs"}, {"invalid", "not-an-absolute-url"}} {
		t.Run(c.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			path := claudeRootPluginFixture(t, fx, nil, false)
			plugin := filepath.Dir(path)
			m := map[string]any{"name": "demo", "version": "1.0.0"}
			if c.homepage != "" {
				m["homepage"] = c.homepage
			}
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), m)
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			assertExit(t, result, 0)
			writeEvidence(t, "independent/"+c.name+"-list.json", result.stdout)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			s := claudeAt(t, list, plugin)
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, s.ID)
			assertExit(t, detail, 0)
			writeEvidence(t, "independent/"+c.name+"-explain.txt", detail.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			t.Logf("state=%s participation=%s explain_local_activation=%v", s.Availability.State, s.Availability.References[0].Participation, strings.Contains(string(detail.stdout), "local-marketplace-plugin"))
			if c.name == "invalid" {
				if s.Availability.State == "available" {
					t.Error("literal required result: loader-invalid manifest must not establish available activation")
				}
			} else if s.Availability.State != "available" {
				t.Errorf("activation positive control state=%s", s.Availability.State)
			}
		})
	}
}
