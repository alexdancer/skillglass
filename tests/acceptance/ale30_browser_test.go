package acceptance_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestALE30BrowserCancelConfirmStaleAndExit(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, _ := codexFixture(t)
			target := claudeSkill(t, filepath.Join(fx.home, ".claude/skills"), strings.Repeat("long-source-path-", 4), "ale30-browser", "")
			config := filepath.Join(fx.home, ".claude/settings.json")
			writeFile(t, config, []byte("{\"preserved\":true}\n"), 0600)
			before := treeManifest(t, fx.base)
			targetBefore := readBytes(t, target)
			socket := fmt.Sprintf("ale30-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "claude-code", "--project", fx.project)
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "24", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "24")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "ale30-browser")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "ale30-browser")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			if width < 96 {
				send("Enter", "Enter open")
			} else {
				send("Enter", "Enter focus")
			}
			if width < 96 {
				send("Enter", "Rendered instructions")
			} else {
				send("Enter", "Selected:", "Rendered instructions")
			}
			send("d", "Selected:", "available")
			send("D", "Select the setting scope explicitly", "g Global")
			preview := send("g", "SKILLGLASS | Claude Code control", "ready", "y Apply global disable", "Enter/Esc Cancel")
			assertPaneCells(t, preview, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale30/%d/01-preview.txt", width), []byte(preview))
			bottom := send("G", "Confirm with --confirm")
			assertPaneCells(t, bottom, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale30/%d/02-preview-bottom.txt", width), []byte(bottom))
			send("Escape", "Read lines", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			send("p", "ready", "y Apply project disable")
			send("Escape", "Read lines")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global disable")
			saved := send("y", "applied", "Persistence: verified", "Effective result: achieved")
			assertPaneCells(t, saved, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale30/%d/03-applied.txt", width), []byte(saved))
			if !strings.Contains(string(readBytes(t, config)), "\"ale30-browser\":\"off\"") {
				t.Fatal("PTY confirmation did not persist")
			}
			back := send("Enter", "Read lines", "disabled", "Result: applied | persistence: verified")
			writeEvidence(t, fmt.Sprintf("tmux/ale30/%d/05-return-after-apply.txt", width), []byte(back))
			if paneContains(back, "Cancelled. No settings changed.") {
				t.Fatal("committed result returned as cancellation")
			}
			send("e", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global enable")
			writeFile(t, config, append(readBytes(t, config), []byte("\n ")...), 0600)
			edited := treeManifest(t, fx.base)
			stale := send("y", "refused", "preview is stale")
			writeEvidence(t, fmt.Sprintf("tmux/ale30/%d/04-stale.txt", width), []byte(stale))
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			send("Escape", "Read lines")
			send("e", "Select the setting scope explicitly")
			send("g", "ready")
			send("Escape", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			deadline := time.Now().Add(5 * time.Second)
			status := ""
			for time.Now().Before(deadline) {
				raw := runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "browser:0.0", "#{pane_dead}:#{pane_dead_status}")
				status = strings.TrimSpace(string(raw))
				if strings.HasPrefix(status, "1:") {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if status != "1:1" {
				t.Fatalf("unsuccessful confirmed mutation hidden by cancel/quit: %q", status)
			}
			writeEvidence(t, fmt.Sprintf("tmux/ale30/%d/exit.txt", width), []byte(status))
			if !bytes.Equal(targetBefore, readBytes(t, target)) {
				t.Fatal("source altered")
			}
		})
	}
}
