package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func piAstralFixture(t *testing.T, origin, pattern string, mixed bool) fixture {
	t.Helper()
	fx := piFixture(t)
	agentDir := filepath.Join(fx.home, ".pi", "agent")
	root := filepath.Join(agentDir, "skills")
	if origin != "automatic" {
		root = filepath.Join(fx.base, "allowed", "P", "skills")
	}
	if origin == "ancestor" {
		root = filepath.Join(fx.base, "allowed", "🔍", "skills")
	}
	folder := "🔍"
	if origin == "ancestor" {
		folder = "ascii"
	}
	manifest := codexSkill(t, root, folder, "astral")
	bundle := filepath.Dir(manifest)
	patterns := []string{pattern}
	switch pattern {
	case "restore":
		patterns = []string{"!*", "+" + bundle}
	case "deny":
		patterns = []string{"!*", "+" + bundle, "-" + bundle}
	case "unsupported-restore":
		patterns = []string{"!?", "+" + bundle}
	}
	if mixed {
		codexSkill(t, root, "東京健康", "healthy-BMP")
		codexSkill(t, root, "healthy-ASCII", "healthy-ASCII")
	}
	settings := map[string]any{"defaultProjectTrust": "always", "skills": patterns}
	if origin == "configured" || origin == "ancestor" {
		settings["skills"] = append([]string{root}, patterns...)
	}
	if origin == "package" {
		pkg := filepath.Dir(root)
		writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"pi":{"skills":["skills"]}}`), 0600)
		delete(settings, "skills")
		settings["packages"] = []any{map[string]any{"source": pkg, "skills": patterns}}
	}
	piSettingsFile(t, filepath.Join(agentDir, "settings.json"), settings)
	return fx
}

func TestALE33AstralResourcePatterns(t *testing.T) {
	for _, origin := range []string{"automatic", "configured", "package"} {
		for _, tc := range []struct{ label, pattern, state string }{
			{"one-unit", "!?", "unknown"},
			{"two-unit", "!??", "unknown"},
			{"astral-class", "![🔍]", "unknown"},
			{"negated-class", "![!x]", "unknown"},
			{"literal", "!🔍", "disabled"},
			{"star", "!*", "disabled"},
			{"literal-star", "!🔍*", "disabled"},
			{"exact-restore", "restore", "available"},
			{"exact-deny", "deny", "disabled"},
		} {
			t.Run(origin+"/"+tc.label, func(t *testing.T) {
				fx := piAstralFixture(t, origin, tc.pattern, false)
				before := treeManifest(t, fx.base)
				exit, coverage := 0, "complete"
				if tc.state == "unknown" {
					exit, coverage = 1, "incomplete"
				}
				list := piList(t, fx, exit)
				source := codexNamed(t, list, "astral")
				if source.Availability.State != tc.state || list.Scope.Completeness != coverage {
					t.Errorf("pattern=%q state=%s want=%s coverage=%s", tc.pattern, source.Availability.State, tc.state, list.Scope.Completeness)
				}
				assertPiResourceDetail(t, fx, source, exit)
				filtered := piList(t, fx, exit, "--availability", tc.state)
				if len(filtered.Sources) != 1 || filtered.Sources[0].ID != source.ID {
					t.Errorf("availability filter disagrees: %+v", filtered.Sources)
				}
				if tc.state == "unknown" {
					problemView := piList(t, fx, exit, "--problem", "native-input")
					if len(problemView.Sources) != 1 || problemView.Sources[0].ID != source.ID {
						t.Error("unknown pattern lacks owning diagnostic")
					}
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE33AstralPatternReferenceScope(t *testing.T) {
	for _, origin := range []string{"automatic", "configured", "package"} {
		for _, pattern := range []string{"!?", "!??", "![!x]"} {
			t.Run(origin+"/"+pattern, func(t *testing.T) {
				fx := piAstralFixture(t, origin, pattern, true)
				before := treeManifest(t, fx.base)
				list := piRelatedInventory(t, fx, 1)
				var affected collection.Source
				for _, source := range list.Sources {
					if source.Name == "astral" {
						affected = source
						if source.Availability.State != collection.AvailabilityUnknown || source.Availability.Evidence.Kind != collection.EvidenceUnknown || len(source.Problems) != 1 {
							t.Errorf("astral uncertainty not recorded: %+v", source)
						}
					} else if source.Availability.State != collection.AvailabilityAvailable || len(source.Problems) != 0 {
						t.Errorf("healthy sibling contaminated: %s", source.Name)
					}
				}
				if len(list.Problems) != 1 || len(list.Problems[0].ReferenceIDs) != 1 || len(list.Problems[0].SourceIDs) != 1 || list.Problems[0].SourceIDs[0] != affected.ID {
					t.Errorf("path-specific ownership expanded: %+v", list.Problems)
				}
				for _, ref := range list.References {
					if ref.Source != affected.ID && len(ref.Problems) != 0 {
						t.Error("unaffected reference inherited uncertainty")
					}
				}
				filtered := piRelatedInventory(t, fx, 1, "--problem", "native-input")
				if len(filtered.Sources) != 1 || filtered.Sources[0].ID != affected.ID {
					t.Error("problem filter disagrees with affected occurrence")
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
	for _, scenario := range []string{"ancestor", "unsupported-restore"} {
		t.Run(scenario, func(t *testing.T) {
			origin, pattern := "ancestor", "!?"
			if scenario == "unsupported-restore" {
				origin, pattern = "configured", scenario
			}
			fx := piAstralFixture(t, origin, pattern, false)
			before := treeManifest(t, fx.base)
			source := codexNamed(t, piList(t, fx, 1), "astral")
			if source.Availability.State != "unknown" {
				t.Errorf("conservative policy changed: %+v", source)
			}
			assertPiResourceDetail(t, fx, source, 1)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func piIgnoreFixture(t *testing.T, origin, control string) (fixture, string, string, int) {
	t.Helper()
	fx := piFixture(t)
	agent := filepath.Join(fx.home, ".pi", "agent")
	global := filepath.Join(agent, "skills")
	root := filepath.Join(fx.base, "allowed", "P", "skills")
	if origin == "automatic" {
		root = global
	}
	if origin == "cache" {
		root = filepath.Join(agent, "git", "P", "skills")
	}
	folder := "A"
	if control == "alias" {
		folder = "blocked/A"
	}
	a := filepath.Dir(codexSkill(t, root, folder, "ignore-A"))
	codexSkill(t, root, "B", "healthy-B")
	codexSkill(t, global, "C", "healthy-C")
	ignore, exit := "*\n", 1
	if control == "comment-only" {
		ignore, exit = "# comment only\n\n", 0
	}
	ignoreDir := a
	if control == "alias" {
		ignoreDir = filepath.Dir(a)
		if err := os.Symlink(a, filepath.Join(global, "shared-alias")); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(ignoreDir, ".ignore"), []byte(ignore), 0600)
	if control == "multiple" {
		d := filepath.Dir(codexSkill(t, root, "D", "ignore-D"))
		writeFile(t, filepath.Join(d, ".ignore"), []byte(ignore), 0600)
	}
	settings := map[string]any{"defaultProjectTrust": "always"}
	if origin == "configured" {
		settings["skills"] = []string{root}
	}
	if origin == "package" {
		pkg := filepath.Dir(root)
		writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"pi":{"skills":["skills"]}}`), 0600)
		settings["packages"] = []string{pkg}
	}
	if control == "context" {
		settings["skills"] = 7
	}
	piSettingsFile(t, filepath.Join(agent, "settings.json"), settings)
	return fx, root, a, exit
}

func TestALE33IgnoreDiagnosticReferenceScope(t *testing.T) {
	for _, origin := range []string{"configured", "automatic", "package", "cache"} {
		for _, control := range []string{"active", "comment-only", "context", "multiple", "alias"} {
			if (control == "alias" && origin != "configured") || (control == "context" && origin == "cache") {
				continue
			}
			t.Run(origin+"/"+control, func(t *testing.T) {
				fx, root, a, exit := piIgnoreFixture(t, origin, control)
				before := treeManifest(t, fx.base)
				list := piRelatedInventory(t, fx, exit)
				for _, source := range list.Sources {
					affected := source.Name == "ignore-A" || source.Name == "ignore-D"
					state := collection.AvailabilityAvailable
					if control == "context" || affected && control != "comment-only" {
						state = collection.AvailabilityUnknown
					}
					if origin == "cache" && source.Name != "healthy-C" {
						state = collection.AvailabilityUnavailable
					}
					if source.Availability.State != state {
						t.Errorf("%s state=%s want=%s", source.Name, source.Availability.State, state)
					}
					if control != "context" && !affected && len(source.Problems) != 0 {
						t.Errorf("healthy source %s inherited ignore diagnostic", source.Name)
					}
					if affected && exit == 1 && len(source.Problems) == 0 {
						t.Error("affected source lost diagnostic")
					}
					for _, op := range []string{"read", "explain"} {
						r := fx.run(t, op, "--agent", "pi", "--project", fx.project, "--json", string(source.ID))
						assertExit(t, r, exit)
						var detail struct{ Source collection.Source }
						decodeJSON(t, r.stdout, &detail)
						if detail.Source.Availability.State != source.Availability.State || len(detail.Source.Problems) != len(source.Problems) {
							t.Error("detail diverged from list")
						}
					}
				}
				filtered := piRelatedInventory(t, fx, exit, "--problem", "native-input")
				if control != "context" {
					want := 1
					if control == "comment-only" {
						want = 0
					}
					if control == "multiple" {
						want = 2
					}
					if len(filtered.Sources) != want || len(list.Problems) > 1 {
						t.Errorf("occurrence ownership/merge: sources=%d problems=%+v", len(filtered.Sources), list.Problems)
					}
					if want > 0 && (len(list.Problems) != 1 || len(list.Problems[0].ReferenceIDs) != want || len(list.Problems[0].SourceIDs) != want || len(list.Problems[0].RootIDs) != 1) {
						t.Errorf("merged ignore links: %+v", list.Problems)
					}
					for _, ref := range list.References {
						affected := ref.DiscoveredPath == a || filepath.Base(ref.DiscoveredPath) == "D"
						if !affected && len(ref.Problems) != 0 {
							t.Errorf("unaffected reference inherited ignore diagnostic: %s", ref.DiscoveredPath)
						}
					}
					for _, report := range list.Scope.Roots {
						owningRoot := root
						if origin == "cache" {
							owningRoot = filepath.Join(fx.home, ".pi", "agent", "git")
						}
						if origin != "automatic" && report.Requested != owningRoot && (report.ProblemCount != 0 || report.State == collection.RootIncomplete) {
							t.Errorf("unrelated root became incomplete: %+v", report)
						}
					}
				}
				if control == "alias" {
					view := piRelatedInventory(t, fx, exit, "--problem", "native-input", "--source-root", filepath.Join(fx.home, ".pi", "agent", "skills"))
					if len(view.Sources) != 0 || len(view.Problems) != 0 {
						t.Error("unaffected alias filter inherited ignore problem")
					}
				}
				coverage := collection.Incomplete
				if exit == 0 {
					coverage = collection.Complete
				}
				if list.Scope.Completeness != coverage {
					t.Error("ignore coverage disagrees")
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE33AstralIgnoreBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"astral", "ignore"} {
		t.Run(scenario, func(t *testing.T) {
			var fx fixture
			name := "astral"
			if scenario == "astral" {
				fx = piAstralFixture(t, "configured", "!?", true)
			} else {
				fx, _, _, _ = piIgnoreFixture(t, "configured", "active")
				name = "ignore-A"
			}
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale33-uncertainty-%d-%s", os.Getpid(), scenario)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse --agent pi --project "+shellQuote(fx.project)+" --availability unknown")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", name)
			frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Read lines", "Availability: unknown")
			for _, line := range []string{"Availability: unknown (unknown)", "Configured permission: unknown (unknown)", "Coverage: incomplete"} {
				if !strings.Contains(frame, line) {
					t.Errorf("uncertainty missing %s: %s", line, frame)
				}
			}
			assertPaneCells(t, frame, 80, 30)
			writeEvidence(t, "uncertainty/"+scenario+"/browser.txt", []byte(frame))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
