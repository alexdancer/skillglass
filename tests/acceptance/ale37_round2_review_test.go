package acceptance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func round2Extension(t *testing.T, fx fixture) string {
	t.Helper()
	root := filepath.Join(fx.home, ".gemini/extensions/sample")
	writeFile(t, filepath.Join(root, "gemini-extension.json"), []byte(`{"name":"sample","version":"1.0.0"}`), 0600)
	writeFile(t, filepath.Join(root, ".gemini-extension-install.json"), []byte(`{"type":"git","source":"inert-source"}`), 0600)
	codexSkill(t, filepath.Join(root, "skills"), "skill", "extension-round2")
	return root
}

func round2List(t *testing.T, fx fixture, wantExit int, environment ...string) codexList {
	t.Helper()
	command := fx.productCommand(t, context.Background(), "list", "--agent", "gemini", "--project", fx.project, "--json")
	command.Dir = fx.project
	command.Env = append(fx.environment(), environment...)
	raw, err := command.Output()
	code := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	result := commandResult{code: code, stdout: raw}
	recordEvidenceCommand(t, "public-cli", command.Path, command.Args[1:], command.Dir, result)
	assertExit(t, result, wantExit)
	var list codexList
	decodeJSON(t, raw, &list)
	return list
}

func TestRound2R6UnicodeExtensionOverrides(t *testing.T) {
	for _, pathCase := range []struct{ name, suffix string }{{"café", "/"}, {"café", "/*"}, {"資料😀.[x]", "/"}, {"資料😀.[x]", "/*"}, {"ascii", "/"}} {
		for _, disable := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-wildcard-%t-disable-%t", pathCase.name, pathCase.suffix == "/*", disable), func(t *testing.T) {
				fx := geminiFixture(t)
				fx.project = filepath.Join(fx.base, "allowed", "projects", pathCase.name)
				if err := os.MkdirAll(fx.project, 0755); err != nil {
					t.Fatal(err)
				}
				round2Extension(t, fx)
				overrides := []string{"!/*", fx.project + pathCase.suffix}
				want := "ask"
				if disable {
					overrides = []string{"/*", "!" + fx.project + pathCase.suffix}
					want = "unavailable"
				}
				data, _ := json.Marshal(map[string]any{"sample": map[string]any{"overrides": overrides}})
				writeFile(t, filepath.Join(fx.home, ".gemini/extensions/extension-enablement.json"), data, 0600)
				before := treeManifest(t, fx.base)
				list := round2List(t, fx, 0)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if got := codexNamed(t, list, "extension-round2").Availability.State; got != want {
					t.Fatalf("state %s, want %s", got, want)
				}
			})
		}
	}
}

func TestRound2R7NativeDisabledEnvironmentForms(t *testing.T) {
	for _, tc := range []struct {
		selector string
		expands  bool
	}{
		{"$DISABLED_NAME", true}, {"${DISABLED_NAME}", true}, {"${MISSING_NAME:-expanded-native}", true},
		{"prefix$DISABLED_NAME", true}, {"$1", true}, {"${NAME-WITH-DASH}", true}, {"${NAME:-}", true},
		{"cost$", false}, {"$-", false}, {"${}", false}, {"${unfinished", false},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			fx := geminiFixture(t)
			codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "source", "expanded-native")
			codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "literal", "cost$")
			data, _ := json.Marshal(map[string]any{"security": map[string]any{"folderTrust": map[string]any{"enabled": false}}, "skills": map[string]any{"disabled": []string{tc.selector}}})
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), data, 0600)
			want, exit := "ask", 0
			if tc.expands {
				want, exit = "unknown", 1
			}
			before := treeManifest(t, fx.base)
			list := round2List(t, fx, exit, "DISABLED_NAME=expanded-native")
			if got := codexNamed(t, list, "expanded-native").Availability.State; got != want {
				t.Errorf("selector %s: state %s, want %s", tc.selector, got, want)
			}
			literalWant := want
			if tc.selector == "cost$" {
				literalWant = "disabled"
			}
			if got := codexNamed(t, list, "cost$").Availability.State; got != literalWant {
				t.Errorf("literal dollar name: state %s, want %s", got, literalWant)
			}
			if tc.expands {
				found := false
				for _, p := range list.Problems {
					found = found || p.Code == "native-input"
				}
				if !found {
					t.Error("unverified expansion missing native-input diagnostic")
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestRound2R8InspectionFailuresRemainUnknown(t *testing.T) {
	for _, kind := range []string{"oversized-skill", "unreadable-skill", "encoding-skill", "oversized-registration", "unreadable-registration", "encoding-registration", "broken-registration", "known-invalid-skill", "known-invalid-registration", "missing-registration"} {
		t.Run(kind, func(t *testing.T) {
			fx := geminiFixture(t)
			bundle := ""
			if strings.Contains(kind, "registration") {
				ext := round2Extension(t, fx)
				bundle = filepath.Join(ext, "skills/skill")
				manifest := filepath.Join(ext, "gemini-extension.json")
				if kind == "oversized-registration" {
					writeFile(t, manifest, []byte(`{"name":"sample","version":"1.0.0","padding":"`+strings.Repeat("x", (4<<20)+1)+`"}`), 0600)
				}
				if kind == "unreadable-registration" {
					if err := os.Chmod(manifest, 0); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Chmod(manifest, 0600) })
				}
				if kind == "known-invalid-registration" {
					writeFile(t, manifest, []byte(`{"name":`), 0600)
				}
				if kind == "encoding-registration" {
					writeFile(t, manifest, []byte("{\"name\":\"sample\",\"version\":\"1.0.0\",\"padding\":\"\xff\"}"), 0600)
				}
				if kind == "missing-registration" || kind == "broken-registration" {
					if err := os.Remove(manifest); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "broken-registration" {
					if err := os.Symlink(filepath.Join(ext, "absent.json"), manifest); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				manifest := codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "fixture", "readable-native")
				bundle = filepath.Dir(manifest)
				if kind == "oversized-skill" {
					writeFile(t, manifest, []byte("---\nname: readable-native\ndescription: Valid native fixture.\n---\n"+strings.Repeat("x", (4<<20)+1)), 0600)
				}
				if kind == "unreadable-skill" {
					if err := os.Chmod(manifest, 0); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Chmod(manifest, 0600) })
				}
				if kind == "encoding-skill" {
					writeFile(t, manifest, []byte("---\nname: readable-native\ndescription: Valid native fixture.\n---\n\xff\n"), 0600)
				}
				if kind == "known-invalid-skill" {
					writeFile(t, manifest, []byte("Not a native frontmatter manifest.\n"), 0600)
				}
			}
			before := treeManifest(t, fx.base)
			exit := 1
			if kind == "known-invalid-registration" || kind == "missing-registration" {
				exit = 0
			}
			list := round2List(t, fx, exit)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			want := "unknown"
			if strings.HasPrefix(kind, "known-invalid") || kind == "missing-registration" {
				want = "unavailable"
			}
			if want == "unknown" && len(list.Problems) == 0 {
				t.Error("inspection failure lacks diagnostic")
			}
			found := false
			for _, s := range list.Sources {
				if s.SourcePath == bundle {
					found = true
					if s.Availability.State != want {
						t.Errorf("%s state %s, want %s", kind, s.Availability.State, want)
					}
				}
			}
			if !found {
				t.Fatal("source not retained")
			}
		})
	}
}

func TestRound2R9MalformedControlsRemainUnknown(t *testing.T) {
	for _, scenario := range []struct{ name, settings, overrides, want string }{
		{"block-array", `{"security":{"folderTrust":{"enabled":false},"blockGitExtensions":[]}}`, "", "unknown"},
		{"block-null", `{"security":{"folderTrust":{"enabled":false},"blockGitExtensions":null}}`, "", "unknown"},
		{"block-string", `{"security":{"folderTrust":{"enabled":false},"blockGitExtensions":"false"}}`, "", "unknown"},
		{"allowlist-false", `{"security":{"folderTrust":{"enabled":false},"allowedExtensions":false}}`, "", "unknown"},
		{"allowlist-null-element", `{"security":{"folderTrust":{"enabled":false},"allowedExtensions":["match",null]}}`, "", "unknown"},
		{"allowlist-number", `{"security":{"folderTrust":{"enabled":false},"allowedExtensions":[1]}}`, "", "unknown"},
		{"allowlist-string", `{"security":{"folderTrust":{"enabled":false},"allowedExtensions":"match"}}`, "", "unknown"},
		{"override-null", `{"security":{"folderTrust":{"enabled":false}}}`, `{"sample":{"overrides":["!/*",null]}}`, "unknown"},
		{"override-number", `{"security":{"folderTrust":{"enabled":false}}}`, `{"sample":{"overrides":["!/*",4]}}`, "unknown"},
		{"override-missing", `{"security":{"folderTrust":{"enabled":false}}}`, `{"sample":{}}`, "unknown"},
		{"override-array-null", `{"security":{"folderTrust":{"enabled":false}}}`, `{"sample":{"overrides":null}}`, "unknown"},
		{"valid-empty-override", `{"security":{"folderTrust":{"enabled":false}}}`, `{"sample":{"overrides":[]}}`, "ask"},
		{"valid-clear", `{"security":{"folderTrust":{"enabled":false},"blockGitExtensions":false,"allowedExtensions":[]}}`, "", "ask"},
		{"valid-disable", `{"security":{"folderTrust":{"enabled":false}}}`, `{"sample":{"overrides":["!/*"]}}`, "unavailable"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fx := geminiFixture(t)
			round2Extension(t, fx)
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(scenario.settings), 0600)
			if scenario.overrides != "" {
				writeFile(t, filepath.Join(fx.home, ".gemini/extensions/extension-enablement.json"), []byte(scenario.overrides), 0600)
			}
			exit := 0
			if scenario.want == "unknown" {
				exit = 1
			}
			before := treeManifest(t, fx.base)
			list := round2List(t, fx, exit)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if got := codexNamed(t, list, "extension-round2").Availability.State; got != scenario.want {
				t.Errorf("state %s, want %s", got, scenario.want)
			}
			if scenario.want == "unknown" {
				found := false
				for _, p := range list.Problems {
					found = found || p.Code == "native-input"
				}
				if !found {
					t.Error("malformed control lacks native-input diagnostic")
				}
			}
		})
	}
}

func TestRound2R10OrdinaryControllingDiscoverySettings(t *testing.T) {
	for _, control := range []string{"GEMINI_CLI_TRUST_WORKSPACE=true", "GEMINI_CLI_TRUST_WORKSPACE=false", "GEMINI_RESTRICTED_MODE=true"} {
		t.Run(control, func(t *testing.T) {
			fx := newFixture(t)
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{}`), 0600)
			for _, alias := range []string{".gemini", ".agents"} {
				codexSkill(t, filepath.Join(fx.project, alias, "skills"), "skill", "control"+strings.ReplaceAll(alias, ".", ""))
			}
			list := round2List(t, fx, 0, control)
			if len(list.Sources) != 2 {
				t.Fatal("expected both project aliases")
			}
			for _, source := range list.Sources {
				command := fx.productCommand(t, context.Background(), "explain", "--agent", "gemini", "--project", fx.project, source.ID)
				command.Dir = fx.project
				command.Env = append(fx.environment(), control)
				raw, err := command.Output()
				if err != nil {
					t.Fatal(err)
				}
				recordEvidenceCommand(t, "public-cli", command.Path, command.Args[1:], command.Dir, commandResult{stdout: raw})
				if !strings.Contains(string(raw), control) {
					t.Errorf("ordinary explain omits %s:\n%s", control, raw)
				}
			}
		})
	}
}

func TestRound2R10BrowserControllingDiscoverySetting(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, control := range []string{"GEMINI_CLI_TRUST_WORKSPACE=true", "GEMINI_CLI_TRUST_WORKSPACE=false", "GEMINI_RESTRICTED_MODE=true"} {
		for _, alias := range []string{".gemini", ".agents"} {
			for _, width := range []int{48, 80} {
				t.Run(fmt.Sprintf("%s/%s/%d", control, alias, width), func(t *testing.T) {
					fx := newFixture(t)
					writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{}`), 0600)
					codexSkill(t, filepath.Join(fx.project, alias, "skills"), "source", "browser-control")
					before := treeManifest(t, fx.base)
					socket := fmt.Sprintf("ale37-r10-%d-%d", os.Getpid(), width)
					defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
					command := control + " exec " + fx.productShell(t) + " browse --agent gemini --project " + shellQuote(fx.project)
					runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", fmt.Sprint(width), "-y", "24", "-s", "browser", command)
					runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
					runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", fmt.Sprint(width), "-y", "24")
					frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "browser-control")
					frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Read lines")
					rows := map[int]string{}
					content := ""
					prefix := fmt.Sprintf("round2/r10-browser/%s/%s/%d", control, alias, width)
					for page := 0; page < 80; page++ {
						assertPaneCells(t, frame, width, 24)
						first, last, total, body := sourceDetailBody(t, frame)
						for i, row := range body {
							rows[first+i] = row
						}
						ordered := make([]string, total)
						for i := range ordered {
							ordered[i] = rows[i+1]
						}
						content = strings.Join(ordered, "\n")
						writeEvidence(t, fmt.Sprintf("%s/page-%02d.txt", prefix, page), []byte(frame))
						if paneContains(content, "Discovery setting: "+control) {
							break
						}
						if last == total {
							break
						}
						previous := first
						frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool { first, _, _ := sourceDetailRange(next.cells); return first > previous }).cells
					}
					writeEvidence(t, prefix+"/collected.txt", []byte(content))
					if !paneContains(content, "Discovery setting: "+control) {
						t.Error("browser omits controlling discovery setting")
					}
					assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				})
			}
		}
	}
}
