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
			rows := map[int]string{}
			for page := 0; page < 40; page++ {
				first, last, total, body := sourceDetailBody(t, frame)
				for i, row := range body {
					rows[first+i] = row
				}
				writeEvidence(t, fmt.Sprintf("tmux/ale39/%d/detail-%02d.txt", width, page), []byte(frame))
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
			for _, needle := range []string{"example@local", "persistent CodeAF-only per-skill control is unsupported", "conversation detachment", "another agent's setting", "Capability: disable", "source v0.7.1"} {
				if !paneContains(body.String(), needle) {
					t.Fatalf("missing %q in terminal detail:\n%s", needle, body.String())
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
