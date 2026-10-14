package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func g2PluginRegistration(t *testing.T, list codexList, file, participation, key string) {
	t.Helper()
	source := gateSourceAt(t, list, filepath.Dir(file))
	found := false
	for _, fact := range source.Availability.References {
		if fact.Provenance.Kind == "plugin" {
			found = true
			if fact.Participation != participation || fact.Provenance.PluginID != key {
				t.Errorf("native registration=%+v; want %s %s", fact, participation, key)
			}
		}
	}
	if !found {
		t.Error("native plugin reference missing")
	}
}

func g2FallbackProblems(t *testing.T, list codexList) {
	t.Helper()
	broken := false
	for _, problem := range list.Problems {
		broken = broken || problem.Code == "broken-reference"
		if problem.Code == "native-input" {
			t.Error("fallback manufactured native-input problem")
		}
	}
	if !broken || list.Scope.Completeness != "incomplete" {
		t.Error("generic broken-reference coverage lost")
	}
}

func TestGeneration2Decision1PolicyIndependentFailures(t *testing.T) {
	for _, mode := range []string{"profile-parse", "profile-shape", "managed-parse", "profile-environment", "managed-environment-redirect"} {
		t.Run(mode, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: ['${env:PRIVATE_PLUGIN}']\n"), 0600)
			managed := filepath.Join(fx.home, "managed")
			writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
			switch mode {
			case "profile-parse":
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins: [\n"), 0600)
			case "profile-shape":
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins: false\n"), 0600)
			case "managed-parse":
				writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins: [\n"), 0600)
			case "profile-environment":
				writeFile(t, filepath.Join(p, ".env"), []byte("HERMES_PLATFORM=${PRIVATE_POLICY}\n"), 0600)
			case "managed-environment-redirect":
				writeFile(t, filepath.Join(managed, ".env"), []byte("HERMES_MANAGED_DIR="+filepath.Join(fx.home, "other-managed")+"\n"), 0600)
			}
			env := []string{"HERMES_MANAGED_DIR=" + managed}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 1, env)
			g2Inspect(t, fx, env, list, file, "unknown")
			found := false
			for _, problem := range list.Problems {
				found = found || problem.Code == "native-input"
			}
			if !found {
				t.Error("independent native input failure erased")
			}
		})
	}
}

func TestGeneration2Decision1FallbackScopes(t *testing.T) {
	for _, origin := range []string{"user", "project"} {
		for _, category := range []bool{false, true} {
			for _, mode := range []string{"yaml", "yml", "both"} {
				t.Run(fmt.Sprintf("%s/category-%t/%s", origin, category, mode), func(t *testing.T) {
					fx, p := hermesFixture(t)
					root := filepath.Join(p, "plugins")
					var env []string
					if origin == "project" {
						root = filepath.Join(fx.project, ".hermes", "plugins")
						env = []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}
					}
					key := "portable"
					if category {
						root = filepath.Join(root, "category")
						key = "category/portable"
					}
					root = filepath.Join(root, "portable")
					file := codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
					writeFile(t, filepath.Join(root, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
					writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: ['"+key+"']\n"), 0600)
					for _, extension := range []string{"yaml", "yml"} {
						if mode == "both" || mode == extension {
							if err := os.Symlink("missing."+extension, filepath.Join(root, "plugin."+extension)); err != nil {
								t.Fatal(err)
							}
						}
					}
					defer g2Seal(t, fx)()
					list := g2List(t, fx, 1, env)
					g2Inspect(t, fx, env, list, file, "unknown")
					g2PluginRegistration(t, list, file, "eligible", key)
					g2FallbackProblems(t, list)
				})
			}
		}
	}
}

func TestGeneration2Decision1PolicyFallbackBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		for _, mode := range []string{"policy", "fallback"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				fx, p := hermesFixture(t)
				file := g2Plugin(t, p)
				state, exit := "explicit-only", 0
				managedConfig := ""
				if mode == "policy" {
					managed := filepath.Join(fx.home, "managed")
					managedConfig = filepath.Join(managed, "config.yaml")
					writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: ['${env:PRIVATE_PLUGIN}']\n"), 0600)
					writeFile(t, filepath.Join(p, ".env"), []byte("HERMES_MANAGED_DIR="+managed+"\n"), 0600)
					writeFile(t, managedConfig, []byte("plugins:\n  enabled: [portable]\n"), 0600)
				} else {
					state, exit = "unknown", 1
					root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
					if err := os.Symlink("missing.yaml", filepath.Join(root, "plugin.yaml")); err != nil {
						t.Fatal(err)
					}
				}
				defer g2Seal(t, fx)()
				list := g2List(t, fx, exit, nil)
				g2Inspect(t, fx, nil, list, file, state)
				g2PluginRegistration(t, list, file, "eligible", "portable")
				if mode == "fallback" {
					g2FallbackProblems(t, list)
				}
				socket := fmt.Sprintf("ale31-policy-fallback-%d-%d-%s", os.Getpid(), width, mode)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := "exec " + shellQuote(skillglassBinary) + " browse --agent hermes --project " + shellQuote(fx.project) + " --availability " + state
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", state+" (")
				prefix := fmt.Sprintf("tmux/ale31-policy-fallback/%d/%s/", width, mode)
				writeEvidence(t, prefix+"00-sources.txt", []byte(frame))
				frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
				}, "Read lines", "Availability: "+state)
				seenPermission, seenEligible, seenRegistration, seenKey, seenOwner := false, false, false, false, managedConfig == ""
				for step := 0; step < 100; step++ {
					assertPaneCells(t, frame, width, 30)
					writeEvidence(t, fmt.Sprintf("%s%03d-detail.txt", prefix, step), []byte(frame))
					seenPermission = seenPermission || paneContains(frame, "Configured permission: allowed")
					seenEligible = seenEligible || paneContains(frame, "eligible (inferred)")
					seenRegistration = seenRegistration || paneContains(frame, "Discovery rule: plugin-registration | declarative portable plugin registration")
					seenKey = seenKey || paneContains(frame, "Plugin: portable")
					seenOwner = seenOwner || paneContains(frame, "Discovery input: "+managedConfig)
					if seenPermission && seenEligible && seenRegistration && seenKey && seenOwner || strings.Contains(frame, "G2_FULL_CONTENT_SENTINEL") {
						break
					}
					var start, end, total int
					for _, line := range strings.Split(frame, "\n") {
						if strings.HasPrefix(line, "Read lines ") {
							if _, err := fmt.Sscanf(line, "Read lines %d-%d of %d", &start, &end, &total); err != nil {
								t.Fatal(err)
							}
						}
					}
					next := min(start+3, total-(end-start))
					if start == 0 || next <= start {
						break
					}
					frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
						runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down", "Down", "Down")
					}, fmt.Sprintf("Read lines %d-", next))
				}
				if !seenPermission || !seenEligible || !seenRegistration || !seenKey || !seenOwner {
					t.Fatalf("browser lost effective policy/native fallback: permission=%t eligible=%t registration=%t key=%t owner=%t\n%s", seenPermission, seenEligible, seenRegistration, seenKey, seenOwner, frame)
				}
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
