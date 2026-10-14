package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDecision2HomepagePositiveConfidenceRegression(t *testing.T) {
	for _, tc := range []struct {
		name     string
		homepage any
		present  bool
		want     string
	}{
		{"absent", nil, false, "available"},
		{"valid", "https://example.invalid/docs", true, "available"},
		{"recorded-invalid", "https://", true, "unknown"},
		{"bare-https-scheme", "https:", true, "unknown"},
		{"out-of-range-port", "https://example.invalid:99999/docs", true, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			path := claudeRootPluginFixture(t, fx, nil, false)
			plugin := filepath.Dir(path)
			fields := map[string]any{"name": "demo", "version": "1.0.0"}
			if tc.present {
				fields["homepage"] = tc.homepage
			}
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), fields)
			before := treeManifest(t, fx.base)
			listed := claudeListResult(t, fx, 0)
			source := claudeAt(t, listed, plugin)
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
			assertExit(t, detail, 0)
			writeEvidence(t, "review2/"+tc.name+"-list.json", fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json").stdout)
			writeEvidence(t, "review2/"+tc.name+"-explain.txt", detail.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if source.Ownership.State != "managed" || source.Availability.NativeName != "demo:root-action" {
				t.Fatalf("identity/provenance lost: %+v", source)
			}
			if source.Availability.State != tc.want {
				t.Fatalf("homepage %v: native rejection must prevent positive activation; got %s, want %s", tc.homepage, source.Availability.State, tc.want)
			}
			if tc.want == "unknown" && !strings.Contains(string(detail.stdout), "homepage") {
				t.Fatal("primary explanation lacks homepage limitation")
			}
		})
	}
}
