package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func piPatternFixture(t *testing.T, origin, pattern string) (fixture, string, string) {
	t.Helper()
	fx := piFixture(t)
	name := "東京"
	if pattern == "ascii" || pattern == "ascii-include" {
		name = "ascii"
	}
	root := filepath.Join(fx.home, ".pi", "agent", "skills")
	if origin != "automatic" {
		root = filepath.Join(fx.base, "allowed", "package", "skills")
	}
	manifest := codexSkill(t, root, name, "pattern-skill")
	bundle := filepath.Dir(manifest)
	patterns, state := []string{}, "available"
	switch pattern {
	case "literal":
		patterns, state = []string{"!東京"}, "disabled"
	case "glob":
		patterns, state = []string{"!東*"}, "disabled"
	case "globstar":
		patterns, state = []string{"!**/東*"}, "disabled"
	case "class":
		patterns, state = []string{"![東]京"}, "disabled"
	case "question":
		patterns, state = []string{"!東?"}, "disabled"
	case "ascii":
		patterns, state = []string{"!ascii"}, "disabled"
	case "ascii-include":
		patterns = []string{"ascii*"}
	case "unmatched":
		patterns = []string{"!別*"}
	case "include":
		patterns = []string{"東*"}
	case "automatic-include-ignored":
		patterns = []string{"別*"}
	case "restore":
		patterns = []string{"!*", "+" + bundle}
	case "exact-deny":
		patterns, state = []string{"!*", "+" + bundle, "-" + bundle}, "disabled"
	}
	settings := map[string]any{"defaultProjectTrust": "always", "skills": patterns}
	if origin == "configured" {
		settings["skills"] = append([]string{bundle}, patterns...)
	}
	if origin == "package" {
		pkg := filepath.Dir(root)
		writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"pi":{"skills":["skills"]}}`), 0600)
		delete(settings, "skills")
		settings["packages"] = []any{map[string]any{"source": pkg, "skills": patterns}}
	}
	piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), settings)
	return fx, "pattern-skill", state
}

func assertPiResourceDetail(t *testing.T, fx fixture, source codexSource, exit int) {
	t.Helper()
	permission, kind := "allowed", "inferred"
	if source.Availability.State == "disabled" {
		permission = "denied"
	}
	if source.Availability.State == "unknown" {
		permission, kind = "unknown", "unknown"
	}
	for _, operation := range []string{"read", "explain"} {
		r := fx.run(t, operation, "--agent", "pi", "--project", fx.project, source.ID)
		assertExit(t, r, exit)
		for _, line := range []string{"Availability: " + source.Availability.State + " (" + kind + ")", "Configured permission: " + permission + " (" + kind + ")"} {
			if !strings.Contains(string(r.stdout), line) {
				t.Errorf("detail lacks %q: %s", line, r.stdout)
			}
		}
		r = fx.run(t, operation, "--agent", "pi", "--project", fx.project, "--json", source.ID)
		assertExit(t, r, exit)
		var detail struct{ Source codexSource }
		decodeJSON(t, r.stdout, &detail)
		if detail.Source.ID != source.ID || detail.Source.Availability.State != source.Availability.State || len(detail.Source.Availability.References) != len(source.Availability.References) {
			t.Errorf("detail diverges from list: %+v", detail.Source)
		}
	}
}

func TestALE33UnicodeResourcePatterns(t *testing.T) {
	for _, origin := range []string{"automatic", "configured", "package"} {
		for _, pattern := range []string{"literal", "glob", "globstar", "class", "question", "ascii", "ascii-include", "unmatched", "include", "restore", "exact-deny", "automatic-include-ignored"} {
			if pattern == "automatic-include-ignored" && origin != "automatic" {
				continue
			}
			t.Run(origin+"/"+pattern, func(t *testing.T) {
				fx, name, state := piPatternFixture(t, origin, pattern)
				before := treeManifest(t, fx.base)
				list := piList(t, fx, 0)
				source := codexNamed(t, list, name)
				if source.Availability.State != state || list.Scope.Completeness != "complete" {
					t.Errorf("want %s with complete coverage: %+v", state, source)
				}
				assertPiResourceDetail(t, fx, source, 0)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

type piArrayCase struct {
	name, raw string
	invalid   bool
	selected  bool
}

var piArrayCases = []piArrayCase{
	{"absent", "", false, false},
	{"empty", `[]`, false, false},
	{"null-array", `null`, true, false},
	{"scalar", `"skills"`, true, false},
	{"null-element", `[null]`, true, false},
	{"mixed-null", `["VALID",null]`, true, true},
	{"mixed-number", `[7,"VALID",false]`, true, true},
	{"number-element", `[7]`, true, false},
	{"string", `["VALID"]`, false, true},
	{"empty-string", `[""]`, false, true},
}

func piArrayFixture(t *testing.T, consumer string, c piArrayCase) (fixture, int, bool) {
	t.Helper()
	fx := piFixture(t)
	agentDir := filepath.Join(fx.home, ".pi", "agent")
	codexSkill(t, filepath.Join(agentDir, "skills"), "automatic", "array-automatic")
	pkg := filepath.Join(fx.base, "allowed", "package")
	selected := filepath.Dir(codexSkill(t, pkg, "selected", "array-selected"))
	codexSkill(t, filepath.Join(pkg, "skills"), "convention", "array-convention")
	base := agentDir
	valid := selected
	if consumer == "registration" || consumer == "manifest" {
		base, valid = pkg, "skills"
	}
	if consumer == "manifest" {
		valid = "selected"
	}
	writeFile(t, filepath.Join(base, "trap.md"), []byte("---\nname: array-trap\ndescription: Base-directory trap.\n---\nBody.\n"), 0600)
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(strings.ReplaceAll(c.raw, `"VALID"`, string(encoded)))
	settings := map[string]any{"defaultProjectTrust": "always"}
	exit, invalid := 0, c.invalid
	if consumer == "legacy" && c.name == "null-array" {
		invalid = false
	}
	if invalid {
		exit = 1
	}
	switch consumer {
	case "settings":
		if c.name != "absent" {
			settings["skills"] = raw
		}
	case "legacy":
		legacy := map[string]any{}
		if c.name != "absent" {
			legacy["customDirectories"] = raw
		}
		settings["skills"] = legacy
	case "registration":
		registration := map[string]any{"source": pkg}
		if c.name != "absent" {
			registration["skills"] = raw
		}
		settings["packages"] = []any{registration}
		writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"pi":{"skills":["skills"]}}`), 0600)
	case "manifest":
		settings["packages"] = []any{map[string]any{"source": pkg}}
		manifest := map[string]any{"pi": map[string]any{}}
		if c.name != "absent" {
			manifest["pi"].(map[string]any)["skills"] = raw
		}
		piSettingsFile(t, filepath.Join(pkg, "package.json"), manifest)
	}
	piSettingsFile(t, filepath.Join(agentDir, "settings.json"), settings)
	return fx, exit, invalid
}

func TestALE33ResourceArrayElements(t *testing.T) {
	for _, consumer := range []string{"settings", "legacy", "registration", "manifest"} {
		for _, c := range piArrayCases {
			t.Run(consumer+"/"+c.name, func(t *testing.T) {
				fx, exit, invalid := piArrayFixture(t, consumer, c)
				before := treeManifest(t, fx.base)
				list := piList(t, fx, exit)
				coverage := "complete"
				if invalid {
					coverage = "incomplete"
				}
				if list.Scope.Completeness != coverage {
					t.Errorf("coverage = %s, want %s", list.Scope.Completeness, coverage)
				}
				states := map[string]string{}
				for _, source := range list.Sources {
					states[source.Name] = source.Availability.State
					if source.Name == "array-trap" && c.name != "empty-string" {
						t.Error("non-string entry fabricated a base-directory resource")
					}
				}
				wantAutomatic := "available"
				if invalid && consumer != "manifest" {
					wantAutomatic = "unknown"
				}
				if states["array-automatic"] != wantAutomatic {
					t.Errorf("unrelated automatic state: %+v", states)
				}
				wantSelected, wantConvention := "", ""
				switch consumer {
				case "settings", "legacy":
					if c.selected && c.name != "empty-string" {
						wantSelected = "available"
					}
				case "registration":
					wantConvention = "disabled"
					if c.name == "absent" {
						wantConvention = "available"
					}
				case "manifest":
					if c.name == "absent" || c.name == "empty-string" {
						wantConvention = "available"
					}
					if c.selected {
						wantSelected = "available"
					}
				}
				if invalid {
					if wantSelected != "" {
						wantSelected = "unknown"
					}
					if wantConvention != "" {
						wantConvention = "unknown"
					}
				}
				if states["array-selected"] != wantSelected || states["array-convention"] != wantConvention {
					t.Errorf("valid resources lost or fabricated: %+v; selected=%s convention=%s", states, wantSelected, wantConvention)
				}
				for _, root := range list.Scope.Roots {
					if c.name != "empty-string" && (root.Origin == "pi-configured-global" || root.Origin == "pi-package") && (root.RequestedPath == filepath.Join(fx.home, ".pi", "agent") || root.RequestedPath == filepath.Join(fx.base, "allowed", "package")) {
						t.Errorf("fabricated base root: %+v", root)
					}
				}
				for _, source := range list.Sources {
					assertPiResourceDetail(t, fx, source, exit)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE33ResourcePatternsBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"unicode", "null", "manifest-mixed"} {
		t.Run(scenario, func(t *testing.T) {
			var fx fixture
			name, state, exit := "pattern-skill", "disabled", 0
			if scenario == "unicode" {
				fx, _, _ = piPatternFixture(t, "automatic", "literal")
			} else {
				consumer, c := "settings", piArrayCases[4]
				name, state, exit = "array-automatic", "unknown", 1
				if scenario == "manifest-mixed" {
					consumer, c, name = "manifest", piArrayCases[5], "array-selected"
				}
				fx, _, _ = piArrayFixture(t, consumer, c)
			}
			before := treeManifest(t, fx.base)
			source := codexNamed(t, piList(t, fx, exit), name)
			if source.Availability.State != state {
				t.Errorf("browser fixture state = %s, want %s", source.Availability.State, state)
			}
			socket := fmt.Sprintf("ale33-pattern-%d-%s", os.Getpid(), scenario)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			runTmuxProduct(t, tmux, fx, socket, fx.productCommand(t, "browse", "--agent", "pi", "--project", fx.project, "--availability", state), "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", name)
			frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Read lines", "Configured permission:")
			permission, kind := "denied", "inferred"
			if state == "unknown" {
				permission, kind = "unknown", "unknown"
			}
			for _, line := range []string{"Availability: " + state + " (" + kind + ")", "Configured permission: " + permission + " (" + kind + ")"} {
				if !strings.Contains(frame, line) {
					t.Errorf("browser lacks %q: %s", line, frame)
				}
			}
			assertPaneCells(t, frame, 80, 30)
			writeEvidence(t, "patterns/"+scenario+"/browser.txt", []byte(frame))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
