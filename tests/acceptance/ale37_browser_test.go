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

func TestALE37BrowserExplanationAvailabilityAndRefresh(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx := geminiFixture(t)
			codexSkill(t, filepath.Join(fx.home, ".gemini", "skills"), strings.Repeat("long-gemini-path-", 5), "browser-gemini")
			config := filepath.Join(fx.home, ".gemini", "settings.json")
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale37-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec " + shellQuote(skillglassBinary) + " browse --agent gemini --project " + shellQuote(fx.project) + " --availability ask"
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "24", "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "24")
			pane := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "browser-gemini", "ask (inferred)", "Agent: gemini")
			assertPaneCells(t, pane, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale37/%d/01-sources.txt", width), []byte(pane))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			pane = send("Enter", "Read lines", "Availability: ask", "Session: unknown")
			assertPaneCells(t, pane, width, 24)
			collect := func(prefix string, needles ...string) {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "g")
				frame := waitForPane(t, tmux, fx, socket, false, "Read lines 1-")
				rows := make(map[int]string)
				for page := 0; page < 40; page++ {
					assertPaneCells(t, frame, width, 24)
					first, last, total, body := sourceDetailBody(t, frame)
					for index, row := range body {
						rows[first+index] = row
					}
					ordered := make([]string, total)
					for index := range ordered {
						ordered[index] = rows[index+1]
					}
					content := strings.Join(ordered, "\n")
					writeEvidence(t, fmt.Sprintf("tmux/ale37/%d/%s-page-%02d.txt", width, prefix, page), []byte(frame))
					found := true
					for _, needle := range needles {
						found = found && paneContains(content, needle)
					}
					if found {
						return
					}
					if last == total {
						t.Fatalf("missing browser explanation: %v\n%s", needles, content)
					}
					previous := first
					frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool { first, _, _ := sourceDetailRange(next.cells); return first > previous }).cells
				}
				t.Fatal("browser pager did not reach evidence")
			}
			collect("02-detail", "Native name: browser-gemini", "Permission: allowed", "Invocation: ask", "Capability: enable", "Session: unknown")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeFile(t, config, []byte(`{"security":{"folderTrust":{"enabled":false}},"skills":{"disabled":["BROWSER-GEMINI"]}}`), 0600)
			before = treeManifest(t, fx.base)
			send("r", "Refresh: selected directory object retained")
			collect("03-refreshed", "Availability: disabled", "Permission: denied", "disabled-name-union", "Session: unknown")
			send("Escape", "SKILLGLASS | Sources", "hidden by filters", "0 matches")
			// The shared cycle is all, available, disabled, explicit-only, ask, ...
			send("a", "Availability filter: shadowed")
			send("a", "Availability filter: unavailable")
			send("a", "Availability filter: unknown")
			send("a", "Availability filter: all")
			send("a", "Availability filter: available")
			pane = send("a", "Availability filter: disabled", "browser-gemini", "disabled (inferred)")
			assertPaneCells(t, pane, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale37/%d/04-filter.txt", width), []byte(pane))
			pane = send("s", "SKILLGLASS | Scope and filters", "Agent: gemini", "Gemini CLI 0.63.0")
			assertPaneCells(t, pane, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale37/%d/05-scope.txt", width), []byte(pane))
			send("Escape", "SKILLGLASS | Sources")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
