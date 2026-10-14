package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestR13NativeNameIndependentOfUnrelatedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		exit         int
	}{
		{"quoted-numeric-key", "name: \"actual\"\ndescription: ordinary\nmetadata: {2026: stable}", 1},
		{"nested-numeric-key", "name: \"actual\"\ndescription: ordinary\nmetadata: [{nested: {2026: stable}}]", 1},
		{"boolean-key", "name: \"actual\"\ndescription: ordinary\nmetadata: {true: stable}", 1},
		{"quoted-string-key-control", "name: \"actual\"\ndescription: ordinary\nmetadata: {year: stable}", 0},
		{"unquoted-numeric-key-control", "name: actual\ndescription: ordinary\nmetadata: {2026: stable}", 1},
		{"quoted-no-metadata-control", "name: \"actual\"\ndescription: ordinary", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := geminiFixture(t)
			writeFile(t, filepath.Join(fx.home, ".gemini/skills/source/SKILL.md"), []byte("---\n"+tc.fields+"\n---\nInert content.\n"), 0600)
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":false}},"skills":{"disabled":["actual"]}}`), 0600)
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--json")
			assertExit(t, result, tc.exit)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			if len(list.Sources) != 1 {
				t.Fatalf("source count: %s", result.stdout)
			}
			source := list.Sources[0]
			var generic publicList
			decodeJSON(t, result.stdout, &generic)
			if generic.Sources[0].Manifest.RawText != "---\n"+tc.fields+"\n---\nInert content.\n" {
				t.Error("native identity extraction changed manifest bytes")
			}
			if tc.exit == 1 {
				if generic.Sources[0].Manifest.Status != "malformed" {
					t.Error("generic metadata diagnostic was removed")
				}
				found := false
				for _, problem := range generic.Problems {
					found = found || problem.Code == "malformed-manifest"
				}
				if !found {
					t.Error("generic malformed-manifest problem missing")
				}
			}
			if source.Availability.NativeName != "actual" || source.Availability.State != "disabled" {
				t.Errorf("public native name/state = %q/%q, want actual/disabled", source.Availability.NativeName, source.Availability.State)
			}
			explain := fx.run(t, "explain", "--agent", "gemini", "--project", fx.project, "--json", source.ID)
			assertExit(t, explain, tc.exit)
			var detail struct{ Source codexSource }
			decodeJSON(t, explain.stdout, &detail)
			if detail.Source.ID != source.ID || detail.Source.Availability.NativeName != "actual" || detail.Source.Availability.State != "disabled" {
				t.Errorf("explain native name/state = %q/%q, want actual/disabled", detail.Source.Availability.NativeName, detail.Source.Availability.State)
			}
			filtered := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--availability", "disabled", "--json")
			assertExit(t, filtered, tc.exit)
			var disabled codexList
			decodeJSON(t, filtered.stdout, &disabled)
			if len(disabled.Sources) != 1 {
				t.Errorf("disabled filter returns %d sources, want 1", len(disabled.Sources))
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestR13NumericMetadataPreservesNativeConflict(t *testing.T) {
	fx := geminiFixture(t)
	global := filepath.Dir(codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "global", "actual"))
	project := filepath.Join(fx.project, ".agents/skills/project")
	writeFile(t, filepath.Join(project, "SKILL.md"), []byte("---\nname: \"actual\"\ndescription: ordinary\nmetadata: {2026: stable}\n---\nInert content.\n"), 0600)
	before := treeManifest(t, fx.base)
	result := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--json")
	assertExit(t, result, 1)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	if len(list.Sources) != 2 {
		t.Fatalf("source count: %s", result.stdout)
	}
	for _, source := range list.Sources {
		wantState, wantPrecedence := "ask", "winning"
		if source.SourcePath == global {
			wantState, wantPrecedence = "shadowed", "shadowed"
		}
		if source.Availability.NativeName != "actual" || source.Availability.State != wantState || source.Availability.Precedence.State != wantPrecedence {
			t.Errorf("%s: native/state/precedence = %q/%q/%q, want actual/%s/%s", source.SourcePath, source.Availability.NativeName, source.Availability.State, source.Availability.Precedence.State, wantState, wantPrecedence)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
