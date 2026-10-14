package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestALE39CodeAFBrowserExplanationAndPluginRefresh(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx := codeafFixture(t)
			_, settings := codeafPluginFixture(t, fx)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale39-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec " + shellQuote(skillglassBinary) + " browse --agent codeaf --project " + shellQuote(fx.project) + " --availability available"
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
			frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "Agent: codeaf", "plugin-skill")
			assertPaneCells(t, frame, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale39/%d/01-sources.txt", width), []byte(frame))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "plugin-skill")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter read")
			frame = send("Enter", "Read lines", "Availability: available", "Session: unknown")
			assertPaneCells(t, frame, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale39/%d/02-detail.txt", width), []byte(frame))
			// Read the actual paged terminal artifact, including all control reasons.
			frame, body := captureCodeAFDetailPages(t, tmux, fx, socket, frame, width, fmt.Sprintf("tmux/ale39/%d/detail", width))
			for _, needle := range []string{"example@local", "persistent CodeAF-only per-skill control is unsupported", "conversation detachment", "another agent's setting", "Capability: disable", "source v0.7.1"} {
				if !paneContains(body, needle) {
					t.Fatalf("missing %q in terminal detail:\n%s", needle, body)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeFile(t, settings, []byte(`{"enabledPlugins":{"example@local":false}}`), 0600)
			before = treeManifest(t, fx.base)
			send("r", "Refresh: selected directory object retained")
			frame = send("g", "Read lines 1-", "Availability: unavailable")
			writeEvidence(t, fmt.Sprintf("tmux/ale39/%d/03-refreshed.txt", width), []byte(frame))
			assertPaneCells(t, frame, width, 30)
			frame = send("Escape", "SKILLGLASS | Sources", "0 matches", "hidden by filters")
			assertPaneCells(t, frame, width, 30)
			send("s", "SKILLGLASS | Scope", "Agent: codeaf")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("Escape", "SKILLGLASS | Sources")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
		})
	}
}

func TestALE39BrokenPluginBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx := codeafFixture(t)
			manifest, _ := codeafPluginFixture(t, fx)
			container := filepath.Dir(filepath.Dir(manifest))
			writeFile(t, filepath.Join(fx.base, "plugin/.claude-plugin/plugin.json"), []byte(`{"skills":"skills"}`), 0600)
			if err := os.Symlink("missing-target.md", filepath.Join(container, "SKILL.md")); err != nil {
				t.Fatal(err)
			}
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale39-broken-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec " + shellQuote(skillglassBinary) + " browse --agent codeaf --project " + shellQuote(fx.project)
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
			frame := waitForPane(t, tmux, fx, socket, false, "plugin-skill", "Coverage: incomplete")
			assertPaneCells(t, frame, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale39-broken/%d/01-sources.txt", width), []byte(frame))
			actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "/") }, "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "plugin-skill")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Enter read")
			frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Availability: available", "Session: unknown", "Read lines")
			assertPaneCells(t, frame, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale39-broken/%d/02-detail.txt", width), []byte(frame))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
		})
	}
}

func TestALE39Review001BrowserUncertainPrecedenceRefresh(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, topology := range []string{"project-plugin-home", "home-plugin-system", "lower-shared-aliases"} {
		for _, width := range []int{48, 80} {
			t.Run(topology+"/"+strconv.Itoa(width), func(t *testing.T) {
				fx := codeafFixture(t)
				pluginManifest, _ := codeafPluginFixture(t, fx)
				scope := "project"
				root := filepath.Join(fx.home, ".codeaf/skills")
				if topology == "home-plugin-system" {
					scope = "user"
					root = filepath.Join(fx.home, ".codex/skills/.system")
				} else if topology == "lower-shared-aliases" {
					root = filepath.Join(fx.base, "shared")
				}
				lower := filepath.Dir(codexSkill(t, root, "lower-bundle", "plugin-skill"))
				if topology == "lower-shared-aliases" {
					for _, folder := range []string{".codeaf/skills", ".agents/skills"} {
						alias := filepath.Join(fx.home, folder, "alias")
						if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(lower, alias); err != nil {
							t.Fatal(err)
						}
					}
				}
				plugin := filepath.Join(fx.base, "plugin")
				registration := map[string]any{"plugins": map[string]any{"example@local": []any{map[string]any{"scope": scope, "projectPath": fx.project, "installPath": plugin, "version": "1.2"}}}}
				raw, err := json.Marshal(registration)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), raw, 0600)
				initial := codeafListResult(t, fx, 0)
				pluginID := codeafPath(t, initial, filepath.Dir(pluginManifest)).ID
				before := treeManifest(t, fx.base)
				socket := fmt.Sprintf("ale39-review001-%s-%d-%d", topology, os.Getpid(), width)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := "exec " + shellQuote(skillglassBinary) + " browse --agent codeaf --project " + shellQuote(fx.project) + " --availability shadowed"
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				frame := waitForPane(t, tmux, fx, socket, false, "Focus: 1/1", "plugin-skill")
				assertPaneCells(t, frame, width, 30)
				send := func(key string, needles ...string) string {
					return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
				}
				send("/", "Enter keep")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "lower-bundle")
				waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
				send("Enter", "Enter read")
				frame = send("Enter", "Read lines", "Availability: shadowed")
				prefix := fmt.Sprintf("tmux/ale39-review001/%s/%d", topology, width)
				writeEvidence(t, prefix+"/01-shadowed.txt", []byte(frame))
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				local := filepath.Join(fx.project, ".claude/settings.local.json")
				writeFile(t, local, []byte(`{"enabledPlugins":`), 0600)
				before = treeManifest(t, fx.base)
				frame = send("r", "Refresh: selected directory object retained", "Read lines 1-", "Availability: unknown", "Configured permission: allowed")
				writeEvidence(t, prefix+"/02-unknown.txt", []byte(frame))
				frame, body := captureCodeAFDetailPages(t, tmux, fx, socket, frame, width, prefix+"/unknown")
				for _, needle := range []string{"Precedence: unknown", "Related independent source: " + pluginID, "uncertain enablement", "Capability: disable", "persistent CodeAF-only per-skill control is unsupported"} {
					if !paneContains(body, needle) {
						t.Fatalf("missing %q in uncertain detail:\n%s", needle, body)
					}
				}
				if paneContains(body, "Winning source:") || paneContains(body, ": eligible (") {
					t.Fatalf("uncertain detail asserts a winner:\n%s", body)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				send("Escape", "0 matches", "hidden by filters")
				send("a", "Availability filter: unavailable")
				frame = send("a", "Availability filter: unknown", "Focus: 1/1")
				writeEvidence(t, prefix+"/03-unknown-filter.txt", []byte(frame))
				send("Enter", "Read lines", "Availability: unknown")
				writeFile(t, local, []byte(`{"enabledPlugins":{"example@local":false}}`), 0600)
				before = treeManifest(t, fx.base)
				frame = send("r", "Refresh: selected directory object retained", "Read lines 1-", "Availability: available")
				assertPaneCells(t, frame, width, 30)
				writeEvidence(t, prefix+"/04-available.txt", []byte(frame))
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}

func captureCodeAFDetailPages(t *testing.T, tmux string, fx fixture, socket, frame string, width int, evidencePrefix string) (string, string) {
	t.Helper()
	rows := map[int]string{}
	for page := 0; page < 40; page++ {
		first, last, total, body := sourceDetailBody(t, frame)
		for i, row := range body {
			rows[first+i] = row
		}
		writeEvidence(t, fmt.Sprintf("%s-%02d.txt", evidencePrefix, page), []byte(frame))
		assertPaneCells(t, frame, width, 30)
		if last == total {
			break
		}
		frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool {
			nextFirst, _, _ := sourceDetailRange(next.cells)
			return nextFirst > first
		}).cells
	}
	var body strings.Builder
	for i := 1; i <= len(rows); i++ {
		body.WriteString(rows[i])
		body.WriteByte('\n')
	}
	return frame, body.String()
}
