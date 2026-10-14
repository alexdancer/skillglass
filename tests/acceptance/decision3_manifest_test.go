package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecision3ManifestShapeConfidence(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot, "tools", "claude_manifest_shape_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Field, Want string
		SourcePresent     bool `json:"source_present"`
		Present           bool
		Value             any
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	fx, _ := codexFixture(t)
	path := claudeRootPluginFixture(t, fx, nil, false)
	plugin := filepath.Dir(path)
	firstID := ""
	firstRef := ""
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			metadata := map[string]any{"name": "demo", "version": "1.0.0"}
			if tc.Present {
				metadata[tc.Field] = tc.Value
			}
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), metadata)
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
			assertExit(t, result, 0)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			writeEvidence(t, "shapes/"+tc.Name+"-list.json", result.stdout)
			if !tc.SourcePresent {
				for _, candidate := range list.Sources {
					if candidate.SourcePath == plugin {
						t.Error("nonpositive control unexpectedly discovered root source")
					}
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			source := claudeAt(t, list, plugin)
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
			assertExit(t, detail, 0)
			writeEvidence(t, "shapes/"+tc.Name+"-list.json", result.stdout)
			writeEvidence(t, "shapes/"+tc.Name+"-explain.txt", detail.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if firstID == "" {
				firstID = source.ID
			}
			if source.ID != firstID {
				t.Error("metadata shape changed source identity")
			}
			if source.Ownership.State != "managed" {
				t.Error("managed ownership lost")
			}
			if tc.Name != "version-number" && source.Availability.NativeName != "demo:root-action" {
				t.Error("namespace lost")
			}
			if source.Availability.State != tc.Want {
				t.Errorf("%s=%#v: got state %s; want literal %s", tc.Field, tc.Value, source.Availability.State, tc.Want)
			}
			if tc.Want == "unknown" && (source.Availability.Evidence.Rule != "plugin-manifest-unverified" || !strings.Contains(source.Availability.Evidence.Reason, tc.Field) || !strings.Contains(string(detail.stdout), tc.Field+" ")) {
				t.Errorf("field-specific primary diagnostic lost: %s", detail.stdout)
			}
			for _, r := range source.Availability.References {
				if firstRef == "" {
					firstRef = r.ReferenceID
				}
				if r.ReferenceID != firstRef || r.Provenance.PluginID != "demo@market" {
					t.Error("reference identity/provenance changed")
				}
				if tc.Want == "unknown" && (r.Evidence.Rule != "plugin-manifest-unverified" || !strings.Contains(r.Evidence.Reason, tc.Field)) {
					t.Error("field-specific reference diagnostic lost")
				}
				want := "unknown"
				if tc.Want == "available" {
					want = "eligible"
				}
				if r.Participation != want {
					t.Errorf("reference got %s want %s", r.Participation, want)
				}
			}
		})
	}
}
