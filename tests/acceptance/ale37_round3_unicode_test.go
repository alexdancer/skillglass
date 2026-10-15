package acceptance_test

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestR12JavaScriptUnicodeDisabledKeys(t *testing.T) {
	for _, tc := range []struct{ label, name, selector, state string }{
		{"dot-false-positive", "i", "İ", "ask"},
		{"dot-false-negative", "i\u0307", "İ", "disabled"},
		{"reverse-dot-false-positive", "İ", "i", "ask"},
		{"reverse-dot-match", "İ", "i\u0307", "disabled"},
		{"final-sigma", "ΟΣ", "ος", "disabled"},
		{"final-sigma-reverse", "ος", "ΟΣ", "disabled"},
		{"nonfinal-sigma", "ΟΣΑ", "οσα", "disabled"},
		{"sigma-not-folded", "οσ", "ος", "ask"},
		{"ascii-control", "actual", "ACTUAL", "disabled"},
		{"accent-control", "CAFÉ", "café", "disabled"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			fx := geminiFixture(t)
			codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "source", tc.name)
			raw, err := json.Marshal(map[string]any{"security": map[string]any{"folderTrust": map[string]bool{"enabled": false}}, "skills": map[string]any{"disabled": []string{tc.selector}}})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), raw, 0600)
			before := treeManifest(t, fx.base)
			list := geminiListResult(t, fx, 0)
			source := codexNamed(t, list, tc.name)
			if source.Availability.State != tc.state || source.Availability.NativeName != tc.name {
				t.Errorf("name %q selector %q: native name %q, state %s, want %s", tc.name, tc.selector, source.Availability.NativeName, source.Availability.State, tc.state)
			}
			filtered := geminiListResult(t, fx, 0, "--availability", "disabled")
			want := 0
			if tc.state == "disabled" {
				want = 1
			}
			if len(filtered.Sources) != want {
				t.Errorf("disabled filter returned %d sources, want %d", len(filtered.Sources), want)
			}
			explained := fx.run(t, "explain", "--agent", "gemini", "--project", fx.project, "--json", source.ID)
			assertExit(t, explained, 0)
			var detail struct{ Source codexSource }
			decodeJSON(t, explained.stdout, &detail)
			if detail.Source.ID != source.ID || detail.Source.Name != tc.name || detail.Source.Availability.State != tc.state {
				t.Errorf("explanation changed source identity or state: %s", explained.stdout)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestR12NativePersistedUnicodeTrust(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("pinned macOS trust comparison")
	}
	for _, tc := range []struct{ label, component, key, level string }{
		{"dot-folder", "İ", "i\u0307", "TRUST_FOLDER"},
		{"dot-parent", "İ", "i\u0307", "TRUST_PARENT"},
		{"dot-denial", "İ", "i\u0307", "DO_NOT_TRUST"},
		{"sigma-folder", "ΟΣ", "ος", "TRUST_FOLDER"},
		{"sigma-parent", "ΟΣ", "ος", "TRUST_PARENT"},
		{"sigma-denial", "ΟΣ", "ος", "DO_NOT_TRUST"},
		{"ascii-control", "Work", "work", "TRUST_FOLDER"},
		{"accent-control", "CAFÉ", "café", "TRUST_FOLDER"},
	} {
		for _, alias := range []string{".gemini", ".agents"} {
			t.Run(tc.label+"/"+alias, func(t *testing.T) {
				fx := geminiFixture(t)
				fx.project = filepath.Join(filepath.Dir(fx.project), tc.component, "Work")
				projectSource := filepath.Dir(codexSkill(t, filepath.Join(fx.project, alias, "skills"), "project", "collision"))
				globalSource := filepath.Dir(codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "global", "collision"))
				writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{}`), 0600)
				projectKey := filepath.Join(strings.ToLower(filepath.Dir(filepath.Dir(fx.project))), tc.key, "work")
				rule := projectKey
				if tc.level == "TRUST_PARENT" {
					rule = filepath.Join(rule, "child")
				}
				rules := map[string]string{rule: tc.level}
				if tc.level == "DO_NOT_TRUST" {
					rules[strings.ToLower(filepath.Dir(filepath.Dir(fx.project)))] = "TRUST_FOLDER"
				}
				raw, err := json.Marshal(rules)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(fx.home, ".gemini/trustedFolders.json"), raw, 0600)
				before := treeManifest(t, fx.base)
				list := geminiListResult(t, fx, 0)
				if len(list.Sources) != 2 {
					t.Fatalf("source count: %d", len(list.Sources))
				}
				for _, source := range list.Sources {
					want := "ask"
					if source.SourcePath == globalSource {
						want = "shadowed"
					}
					if tc.level == "DO_NOT_TRUST" {
						want = "ask"
						if source.SourcePath == projectSource {
							want = "unavailable"
						}
					}
					if source.Availability.State != want {
						t.Errorf("%s state %s, want %s", source.SourcePath, source.Availability.State, want)
					}
					if source.SourcePath != globalSource && source.SourcePath != projectSource {
						t.Errorf("display path changed: %s", source.SourcePath)
					}
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}
