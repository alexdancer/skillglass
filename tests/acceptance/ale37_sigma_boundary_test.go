package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGeminiSigmaTrust(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("native trust paths are case-sensitive on this platform")
	}
	if runtime.GOOS == "darwin" {
		// Keep room for padding even when the host's default temporary path is long.
		// Each case still pins sigma to the same absolute byte offset.
		temporary, err := os.MkdirTemp("/private/tmp", "ale-37-sigma-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(temporary); err != nil {
				t.Error(err)
			}
		})
		t.Setenv("TMPDIR", temporary)
		t.Setenv("GOTMPDIR", temporary)
	}
	for _, offset := range []int{125, 126, 127, 128, 129} {
		for _, level := range []string{"TRUST_PARENT", "DO_NOT_TRUST"} {
			label := "allow"
			if level == "DO_NOT_TRUST" {
				label = "deny"
			}
			t.Run(fmt.Sprintf("sigma/%s-%d", label, offset), func(t *testing.T) {
				geminiSigmaTrustCase(t, offset, "ΟΣ", "ος", level)
			})
		}
	}
	for _, control := range []struct{ label, component, key string }{
		{"lower", "ος", "ος"}, {"ascii", "OS", "os"},
	} {
		for _, level := range []string{"TRUST_PARENT", "DO_NOT_TRUST"} {
			label := "allow"
			if level == "DO_NOT_TRUST" {
				label = "deny"
			}
			t.Run(control.label+"/"+label, func(t *testing.T) {
				geminiSigmaTrustCase(t, 128, control.component, control.key, level)
			})
		}
	}
}

func geminiSigmaTrustCase(t *testing.T, offset int, component, key, level string) {
	t.Helper()
	fx := newFixture(t)
	prefix := filepath.Join(fx.base, "allowed", "p")
	first := string([]rune(component)[0])
	padding := offset - len(prefix) - 2 - len(first)
	if padding < 1 {
		t.Fatalf("fixture prefix is too long to place sigma at byte %d: %q", offset, prefix)
	}
	parent := filepath.Join(prefix, strings.Repeat("x", padding))
	fx.project = filepath.Join(parent, component, "Work")
	// Pin the absolute UTF-8 byte position independently of the randomized fixture prefix.
	if actual := len(filepath.Join(parent, first)); actual != offset {
		t.Fatalf("second rune starts at byte %d, want %d", actual, offset)
	}
	if err := os.MkdirAll(fx.project, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":true}}}`), 0600)
	global := filepath.Dir(codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "global", "sigma-native"))
	projectGemini := filepath.Dir(codexSkill(t, filepath.Join(fx.project, ".gemini/skills"), "project", "sigma-native"))
	projectAgents := filepath.Dir(codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "project", "sigma-native"))
	nativeKey := filepath.Join(strings.ToLower(parent), key, "work")
	rule := nativeKey
	if level == "TRUST_PARENT" {
		rule = filepath.Join(rule, "child")
	}
	rules := map[string]string{rule: level}
	if level == "DO_NOT_TRUST" {
		rules[strings.ToLower(parent)] = "TRUST_FOLDER"
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".gemini/trustedFolders.json"), raw, 0600)
	before := treeManifest(t, fx.base)
	writeEvidence(t, "sigma-boundary/"+t.Name()+"/fixture-before.json", before)
	for repeat := 0; repeat < 2; repeat++ {
		list := geminiListResult(t, fx, 0)
		if len(list.Sources) != 3 || len(list.Problems) != 3 {
			t.Fatalf("expected three sources and three duplicate-name diagnostics, got %d sources and %+v", len(list.Sources), list.Problems)
		}
		for _, problem := range list.Problems {
			if problem.Code != "duplicate-name" {
				t.Fatalf("unexpected diagnostic: %+v", problem)
			}
		}
		byPath := make(map[string]codexSource)
		for _, source := range list.Sources {
			if _, duplicate := byPath[source.SourcePath]; duplicate {
				t.Fatalf("duplicate source path %q", source.SourcePath)
			}
			byPath[source.SourcePath] = source
		}
		winner := byPath[projectAgents].ID
		if level == "DO_NOT_TRUST" {
			winner = byPath[global].ID
		}
		for _, path := range []string{global, projectGemini, projectAgents} {
			source, present := byPath[path]
			if !present || source.ID == "" {
				t.Fatalf("expected source %q missing from %+v", path, list.Sources)
			}
			state, participation := "shadowed", "eligible"
			if path == projectAgents {
				state = "ask"
			}
			if level == "DO_NOT_TRUST" {
				state = "ask"
				if path != global {
					state, participation = "unavailable", "excluded"
				}
			}
			if source.Availability.State != state {
				t.Errorf("repeat %d, sigma byte %d: %s state %s, want %s", repeat, offset, path, source.Availability.State, state)
			}
			if len(source.Availability.References) != 1 || source.Availability.References[0].Participation != participation {
				t.Errorf("%s participation: %+v, want exactly one %s reference", path, source.Availability.References, participation)
			}
			if participation == "eligible" && source.Availability.Precedence.Winner != winner {
				t.Errorf("%s winner %q, want %q", path, source.Availability.Precedence.Winner, winner)
			}
		}
	}
	after := treeManifest(t, fx.base)
	writeEvidence(t, "sigma-boundary/"+t.Name()+"/fixture-after.json", after)
	assertTreeUnchanged(t, before, after)
}

func TestGeminiSigmaDisabled(t *testing.T) {
	type scenario struct{ label, name, selector, state string }
	var scenarios []scenario
	for _, offset := range []int{125, 126, 127, 128, 129} {
		upper, lower := strings.Repeat("A", offset-2)+"ΟΣ", strings.Repeat("a", offset-2)+"ος"
		scenarios = append(scenarios,
			scenario{fmt.Sprintf("native-%d", offset), upper, lower, "disabled"},
			scenario{fmt.Sprintf("selector-%d", offset), lower, upper, "disabled"},
			scenario{fmt.Sprintf("nonfinal-%d", offset), upper + "Α", strings.Repeat("a", offset-2) + "οσα", "disabled"},
			scenario{fmt.Sprintf("distinct-%d", offset), upper, strings.Repeat("a", offset-2) + "οσ", "ask"},
		)
	}
	scenarios = append(scenarios,
		scenario{"expansion-sigma", strings.Repeat("İ", 64) + "ΟΣ", strings.Repeat("i\u0307", 64) + "ος", "disabled"},
		scenario{"expansion-selector", strings.Repeat("i\u0307", 128), strings.Repeat("İ", 128), "disabled"},
		scenario{"expansion-distinct", strings.Repeat("i", 128), strings.Repeat("İ", 128), "ask"},
	)
	for _, tc := range scenarios {
		t.Run(tc.label, func(t *testing.T) {
			fx := geminiFixture(t)
			path := filepath.Dir(codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "source", tc.name))
			raw, err := json.Marshal(map[string]any{"security": map[string]any{"folderTrust": map[string]bool{"enabled": false}}, "skills": map[string]any{"disabled": []string{tc.selector}}})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), raw, 0600)
			before := treeManifest(t, fx.base)
			writeEvidence(t, "sigma-boundary/"+t.Name()+"/fixture-before.json", before)
			for repeat := 0; repeat < 2; repeat++ {
				list := geminiListResult(t, fx, 0)
				if len(list.Sources) != 1 || len(list.Problems) != 0 {
					t.Fatalf("expected exactly one intact source without problems, got %+v", list)
				}
				source := list.Sources[0]
				if source.SourcePath != path || source.Availability.NativeName != tc.name || source.Availability.State != tc.state {
					t.Errorf("repeat %d: path %q, native name %q, state %s; want path %q, name %q, state %s", repeat, source.SourcePath, source.Availability.NativeName, source.Availability.State, path, tc.name, tc.state)
				}
			}
			after := treeManifest(t, fx.base)
			writeEvidence(t, "sigma-boundary/"+t.Name()+"/fixture-after.json", after)
			assertTreeUnchanged(t, before, after)
		})
	}
}
