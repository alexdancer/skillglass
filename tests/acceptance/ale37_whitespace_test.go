package acceptance_test

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestALE37FallbackECMAScriptWhitespace(t *testing.T) {
	for _, tc := range []struct{ name, fields, native, state string }{
		{"nbsp-continuation", "name: actual\ndescription: A workflow: details\n  \u00a0\n  name: replacement", "replacement", "disabled"},
		{"bom-continuation", "name: actual\ndescription: A workflow: details\n  \ufeff\n  name: replacement", "replacement", "disabled"},
		{"nel-continuation", "name: actual\ndescription: A workflow: details\n  \u0085\n  name: replacement", "actual", "ask"},
		{"bom-name-trim", "name: \ufeffreplacement\ufeff\ndescription: A workflow: details", "replacement", "disabled"},
		{"nel-name-trim", "name: \u0085replacement\u0085\ndescription: A workflow: details", "\u0085replacement\u0085", "ask"},
		{"bom-field-prefix", "name: actual\n\ufeffname: replacement\ndescription: A workflow: details", "replacement", "disabled"},
		{"nel-field-prefix", "name: actual\n\u0085name: replacement\ndescription: A workflow: details", "actual", "ask"},
		{"bom-description-prefix", "name: replacement\n\ufeffdescription: A workflow: details", "replacement", "disabled"},
		{"nel-description-prefix", "name: actual\ndescription: A workflow: details\n\u0085description: ignored\n  name: replacement", "replacement", "disabled"},
		{"line-separator-prefix", "name: actual\n\u2028name: replacement\ndescription: A workflow: details", "replacement", "disabled"},
		{"paragraph-separator-prefix", "name: actual\n\u2029name: replacement\ndescription: A workflow: details", "replacement", "disabled"},
		{"line-separator-in-name", "name: actual\nname: replacement\u2028suffix\ndescription: A workflow: details", "actual", "ask"},
		{"paragraph-separator-in-name", "name: actual\nname: replacement\u2029suffix\ndescription: A workflow: details", "actual", "ask"},
		{"carriage-return-in-name", "name: actual\nname: replacement\rsuffix\ndescription: A workflow: details", "actual", "ask"},
		{"line-separator-in-description", "name: actual\ndescription: A workflow: details\ndescription: ignored\u2028suffix\n  name: replacement", "replacement", "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := geminiFixture(t)
			raw := "---\n" + tc.fields + "\n---\nInert content.\n"
			writeFile(t, filepath.Join(fx.home, ".gemini/skills/source/SKILL.md"), []byte(raw), 0600)
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":false}},"skills":{"disabled":["replacement"]}}`), 0600)
			before := treeManifest(t, fx.base)
			listed := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--json")
			assertExit(t, listed, 1)
			writeEvidence(t, "whitespace/"+tc.name+"-list.json", listed.stdout)
			var list codexList
			decodeJSON(t, listed.stdout, &list)
			if len(list.Sources) != 1 {
				t.Fatalf("source count: %s", listed.stdout)
			}
			source := list.Sources[0]
			if source.Availability.NativeName != tc.native || source.Availability.State != tc.state {
				t.Errorf("list name/state = %q/%q, want %q/%q", source.Availability.NativeName, source.Availability.State, tc.native, tc.state)
			}
			var generic publicList
			decodeJSON(t, listed.stdout, &generic)
			if generic.Sources[0].Manifest.RawText != raw || generic.Sources[0].Manifest.Status != "malformed" || !bytes.Contains(listed.stdout, []byte("malformed-manifest")) {
				t.Error("generic raw bytes or malformed diagnostic changed")
			}
			explained := fx.run(t, "explain", "--agent", "gemini", "--project", fx.project, "--json", source.ID)
			assertExit(t, explained, 1)
			writeEvidence(t, "whitespace/"+tc.name+"-explain.json", explained.stdout)
			var detail struct{ Source codexSource }
			decodeJSON(t, explained.stdout, &detail)
			if detail.Source.Availability.NativeName != tc.native || detail.Source.Availability.State != tc.state {
				t.Errorf("explain name/state = %q/%q, want %q/%q", detail.Source.Availability.NativeName, detail.Source.Availability.State, tc.native, tc.state)
			}
			filtered := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--availability", "disabled", "--json")
			assertExit(t, filtered, 1)
			writeEvidence(t, "whitespace/"+tc.name+"-disabled.json", filtered.stdout)
			var disabled codexList
			decodeJSON(t, filtered.stdout, &disabled)
			wantCount := 0
			if tc.state == "disabled" {
				wantCount = 1
			}
			if len(disabled.Sources) != wantCount {
				t.Errorf("disabled source count = %d, want %d", len(disabled.Sources), wantCount)
			}
			after := treeManifest(t, fx.base)
			writeEvidence(t, "whitespace/"+tc.name+"-before.json", before)
			writeEvidence(t, "whitespace/"+tc.name+"-after.json", after)
			assertTreeUnchanged(t, before, after)
		})
	}
}

func TestALE37FallbackWhitespacePreservesNativePrecedence(t *testing.T) {
	fx := geminiFixture(t)
	global := filepath.Dir(codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "global", "replacement"))
	project := filepath.Join(fx.project, ".agents/skills/project")
	writeFile(t, filepath.Join(project, "SKILL.md"), []byte("---\nname: actual\ndescription: A workflow: details\n  \u00a0\n  name: replacement\n---\nInert content.\n"), 0600)
	writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":false}},"skills":{"disabled":["replacement"]}}`), 0600)
	before := treeManifest(t, fx.base)
	result := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--json")
	assertExit(t, result, 1)
	writeEvidence(t, "whitespace/precedence-list.json", result.stdout)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	if len(list.Sources) != 2 {
		t.Fatalf("source count: %s", result.stdout)
	}
	for _, source := range list.Sources {
		state, precedence := "disabled", "winning"
		if source.SourcePath == global {
			state, precedence = "shadowed", "shadowed"
		}
		if source.Availability.NativeName != "replacement" || source.Availability.State != state || source.Availability.Precedence.State != precedence {
			t.Errorf("%s: name/state/precedence = %q/%q/%q, want replacement/%s/%s", source.SourcePath, source.Availability.NativeName, source.Availability.State, source.Availability.Precedence.State, state, precedence)
		}
	}
	after := treeManifest(t, fx.base)
	writeEvidence(t, "whitespace/precedence-before.json", before)
	writeEvidence(t, "whitespace/precedence-after.json", after)
	assertTreeUnchanged(t, before, after)
}
