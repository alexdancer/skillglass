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

func TestVerifyPolicyFollowupsSiblingControls(t *testing.T) {
	for _, mode := range []string{"enable-expansion-denied", "enable-type-denied", "category-key-denied", "category-name-denied", "qualified-platform-denied", "raw-type-denied", "denied-collision", "genuine-profile-parse", "genuine-environment", "yaml-unreadable-target"} {
		t.Run(mode, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			config := "plugins:\n  enabled: [off]\n  disabled: [portable]\n"
			state, exit := "unavailable", 0
			env := []string{"HERMES_PLATFORM=telegram"}
			switch mode {
			case "enable-expansion-denied":
				config, exit = "plugins:\n  enabled: ['${env:PRIVATE_PLUGIN}']\n  disabled: [portable]\n", 1
			case "enable-type-denied":
				config, exit = "plugins:\n  enabled: false\n  disabled: [portable]\n", 1
			case "category-key-denied", "category-name-denied":
				root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
				category := filepath.Join(p, "plugins", "category")
				if err := os.MkdirAll(category, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(root, filepath.Join(category, "portable")); err != nil {
					t.Fatal(err)
				}
				file = filepath.Join(category, "portable", "skills", "explicit", "SKILL.md")
				if mode == "category-key-denied" {
					config = "plugins:\n  enabled: [off]\n  disabled: ['category/portable']\n"
				}
			case "qualified-platform-denied":
				config = "plugins:\n  enabled: [portable]\nskills:\n  disabled: [off]\n  platform_disabled:\n    telegram: [agent-plugin-portable-01e78282:explicit]\n"
				state = "disabled"
			case "raw-type-denied", "denied-collision":
				file = codexSkill(t, filepath.Join(p, "skills"), "guarded", "guarded")
				config = "plugins:\n  enabled: [portable]\nskills:\n  disabled: false\n  platform_disabled:\n    telegram: [guarded]\n"
				state = "disabled"
				if mode == "denied-collision" {
					config = "plugins:\n  enabled: [portable]\nskills:\n  external_dirs: [shared]\n  disabled: [guarded]\n  platform_disabled:\n    telegram: [off]\n"
					codexSkill(t, filepath.Join(p, "shared"), "other", "guarded")
				}
			case "genuine-profile-parse":
				config, state, exit = "plugins: [\n", "unknown", 1
				managed := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins:\n  disabled: [portable]\n"), 0600)
				env = append(env, "HERMES_MANAGED_DIR="+managed)
			case "genuine-environment":
				state, exit = "unknown", 1
				writeFile(t, filepath.Join(p, ".env"), []byte("HERMES_PLATFORM=${PRIVATE_POLICY}\n"), 0600)
			case "yaml-unreadable-target":
				config, state, exit = "plugins:\n  enabled: [portable]\n", "unknown", 1
				target := filepath.Join(fx.base, "excluded", "plugin.yaml")
				writeFile(t, target, []byte("name: portable\nversion: '1'\n"), 0600)
				root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
				if err := os.Symlink(target, filepath.Join(root, "plugin.yaml")); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(config), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, exit, env)
			g2Inspect(t, fx, env, list, file, state)
			if mode == "denied-collision" {
				for _, source := range list.Sources {
					if source.Name != "guarded" {
						continue
					}
					if source.Availability.State != "disabled" || source.Availability.Precedence.Winner != "" {
						t.Errorf("denied source became an index candidate: %+v", source)
					}
				}
				inv := g2ExplicitInvocation(t, fx, gateSourceAt(t, list, filepath.Dir(file)))
				if inv.State != "ambiguous-bare-name" {
					t.Error("denial changed independent explicit collision facts")
				}
			}
			if exit == 1 {
				found := false
				for _, problem := range list.Problems {
					found = found || problem.Code == "native-input"
				}
				if !found || list.Scope.Completeness != "incomplete" {
					t.Error("genuine or leaf interpretation diagnostic lost")
				}
			}
		})
	}
}

func TestVerifyPolicyFollowupsBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		for _, mode := range []string{"owner", "denial", "manifest"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				fx, p := hermesFixture(t)
				file := g2Plugin(t, p)
				state, exit, participation := "explicit-only", 0, "eligible"
				profile := filepath.Join(p, "config.yaml")
				managedConfig := ""
				if mode == "owner" {
					managed := filepath.Join(fx.home, "managed")
					managedConfig = filepath.Join(managed, "config.yaml")
					writeFile(t, filepath.Join(p, ".env"), []byte("HERMES_MANAGED_DIR="+managed+"\n"), 0600)
					writeFile(t, managedConfig, []byte("plugins:\n  disabled: []\n"), 0600)
				} else if mode == "denial" {
					state, participation = "unavailable", "excluded"
					writeFile(t, profile, []byte("plugins:\n  enabled: [off]\n  disabled: [portable]\n"), 0600)
				} else {
					state, exit = "unknown", 1
					root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
					if err := os.Symlink("plugin.yaml", filepath.Join(root, "plugin.yaml")); err != nil {
						t.Fatal(err)
					}
				}
				defer g2Seal(t, fx)()
				list := g2List(t, fx, exit, nil)
				g2Inspect(t, fx, nil, list, file, state)
				g2PluginRegistration(t, list, file, participation, "portable")
				source := gateSourceAt(t, list, filepath.Dir(file))
				explanation := hermesRun(t, fx, nil, "explain", "--agent", "hermes", "--project", fx.project, source.ID)
				assertExit(t, explanation, exit)
				if mode == "owner" {
					for _, expected := range []string{"Discovery input: " + profile + "\n", "Discovery setting: plugins.enabled\n", "Config layer: " + profile + " [plugins.enabled, read]", "Config layer: " + managedConfig + " [plugins.disabled, read]"} {
						if !strings.Contains(string(explanation.stdout), expected) {
							t.Errorf("effective leaf ownership lost: %s", expected)
						}
					}
				}
				socket := fmt.Sprintf("ale31-followups-%d-%d-%s", os.Getpid(), width, mode)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := fx.productCommand(t, "browse", "--agent", "hermes", "--project", fx.project, "--availability", state)
				runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				frame := waitForPane(t, tmux, fx, socket, false, "Skillglass", state+" (")
				prefix := fmt.Sprintf("tmux/ale31-followups/%d/%s/", width, mode)
				writeEvidence(t, prefix+"00-sources.txt", []byte(frame))
				frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d")
				}, "Read lines", "Availability: "+state)
				setting := "plugins.enabled"
				if mode == "denial" {
					setting = "plugins.disabled"
				}
				seenFact, seenInput, seenSetting, seenEnabled, seenDisabled := false, false, false, mode != "owner", mode != "owner"
				for step := 0; step < 120; step++ {
					assertPaneCells(t, frame, width, 30)
					writeEvidence(t, fmt.Sprintf("%s%03d-detail.txt", prefix, step), []byte(frame))
					seenFact = seenFact || paneContains(frame, participation+" (inferred)")
					seenInput = seenInput || paneContains(frame, "Discovery input: "+profile)
					seenSetting = seenSetting || paneContains(frame, "Discovery setting: "+setting)
					seenEnabled = seenEnabled || paneContains(frame, "Config layer: "+profile+" [plugins.enabled, read]")
					seenDisabled = seenDisabled || paneContains(frame, "Config layer: "+managedConfig+" [plugins.disabled, read]")
					if seenFact && seenInput && seenSetting && seenEnabled && seenDisabled {
						break
					}
					var advanced bool
					frame, advanced = advanceSourceDetailFrame(t, tmux, fx, socket, frame)
					if !advanced {
						break
					}
				}
				if !seenFact || !seenInput || !seenSetting || !seenEnabled || !seenDisabled {
					t.Fatalf("browser lost policy evidence: fact=%t input=%t setting=%t enabled=%t disabled=%t\n%s", seenFact, seenInput, seenSetting, seenEnabled, seenDisabled, frame)
				}
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
