package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29HomepageConfidenceCensus(t *testing.T) {
	var cases []struct {
		Name       string `json:"name"`
		Homepage   string `json:"homepage"`
		Present    bool   `json:"present"`
		State      string `json:"public_state"`
		Diagnostic string `json:"diagnostic"`
	}
	raw, err := os.ReadFile(filepath.Join(repositoryRoot, "tools", "claude_homepage_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			assertClaudeHomepageConfidence(t, tc.Name, tc.Homepage, tc.Present, tc.State, tc.Diagnostic)
		})
	}
}

func TestALE29HomepageIDNACoverageAndNonDomainControls(t *testing.T) {
	for _, tc := range []struct{ name, homepage, state, diagnostic string }{
		{"unicode-path", "https://example.invalid/é", "available", ""},
		{"opaque-unicode", "fixture:é", "available", ""},
		{"opaque-host-unicode", "fixture://éxample.invalid/docs", "available", ""},
		{"ipv6", "https://[::1]:443/docs", "available", ""},
		{"max-port", "https://example.invalid:65535/docs", "available", ""},
		{"overflow-port", "https://example.invalid:65536/docs", "unknown", "WHATWG URL parsing"},
		{"unicode-percent-domain", "https://%C3%A9xample.invalid/docs", "unknown", "IDNA validity"},
		{"alabel", "https://xn--xample-9ua.invalid/docs", "unknown", "IDNA validity"},
		{"alabel-false-accept", "https://xn--ASCII-/docs", "unknown", "IDNA validity"},
		{"alabel-percent-encoded", "https://%78n--ASCII-/docs", "unknown", "IDNA validity"},
		{"alabel-nested", "https://a.XN--unicode-.org/docs", "unknown", "IDNA validity"},
		{"alabel-false-accept-nested", "https://xn--xn--a--gua.pt/docs", "unknown", "IDNA validity"},
		{"alabel-false-accept-prefix", "https://xn--xn---epa/docs", "unknown", "IDNA validity"},
		{"alabel-encoded-dot", "https://a%2EXN--unicode-.org/docs", "unknown", "IDNA validity"},
		{"alabel-tab-normalized", "https://x\tn--ASCII-/docs", "unknown", "IDNA validity"},
		{"userinfo-alabel", "https://xn--user@example.invalid/xn--path", "available", ""},
		{"malformed-percent-domain", "https://exa%zzmple.invalid/docs", "unknown", "IDNA validity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertClaudeHomepageConfidence(t, tc.name, tc.homepage, true, tc.state, tc.diagnostic)
		})
	}
}

func assertClaudeHomepageConfidence(t *testing.T, name, homepage string, present bool, state, diagnostic string) {
	t.Helper()
	fx, _ := codexFixture(t)
	path := claudeRootPluginFixture(t, fx, nil, false)
	plugin := filepath.Dir(path)
	original := claudeAt(t, claudeListResult(t, fx, 0), plugin)
	fields := map[string]any{"name": "demo", "version": "1.0.0"}
	if present {
		fields["homepage"] = homepage
	}
	claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), fields)
	before := treeManifest(t, fx.base)
	result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	var listed codexList
	decodeJSON(t, result.stdout, &listed)
	source := claudeAt(t, listed, plugin)
	detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
	assertExit(t, detail, 0)
	writeEvidence(t, "census/"+name+"-list.json", result.stdout)
	writeEvidence(t, "census/"+name+"-explain.txt", detail.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if source.ID != original.ID || source.SourcePath != original.SourcePath || source.Ownership.State != "managed" || source.Availability.NativeName != "demo:root-action" {
		t.Fatalf("source identity or provenance lost: %+v", source)
	}
	if source.Availability.State != state {
		t.Fatalf("homepage %q: got %s want %s", homepage, source.Availability.State, state)
	}
	if len(source.Availability.References) != len(original.Availability.References) {
		t.Fatal("reference count changed")
	}
	for i, ref := range source.Availability.References {
		if ref.ReferenceID != original.Availability.References[i].ReferenceID || ref.Provenance.PluginID != "demo@market" {
			t.Fatal("reference identity/provenance changed")
		}
		want := "eligible"
		if state == "unknown" {
			want = "unknown"
		}
		if ref.Participation != want {
			t.Fatalf("reference participation: %+v", ref)
		}
		if state == "unknown" && !strings.Contains(ref.Evidence.Reason, diagnostic) {
			t.Fatalf("reference explanation lacks %q: %+v", diagnostic, ref)
		}
	}
	if state == "unknown" && (source.Availability.Evidence.Rule != "plugin-manifest-unverified" || !strings.Contains(source.Availability.Evidence.Reason, diagnostic) || !strings.Contains(string(detail.stdout), diagnostic)) {
		t.Fatalf("primary explanation lacks %q: %s", diagnostic, detail.stdout)
	}
	if diagnostic == "IDNA validity" && strings.Contains(string(detail.stdout), "rejects") {
		t.Fatal("unsupported IDNA misrepresented as native rejection")
	}
}
