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

func TestALE27BrowserCancelConfirmStaleAndExit(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, _ := codexFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), strings.Repeat("long-source-path-", 4), "ale27-browser")
			config := filepath.Join(fx.home, ".codex/config.toml")
			writeFile(t, config, []byte("# preserved\n"), 0600)
			before := treeManifest(t, fx.base)
			targetBefore := readBytes(t, target)
			socket := fmt.Sprintf("ale27-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec /usr/bin/sandbox-exec -p '(version 1)(allow default)(deny network*)' " + shellQuote(skillglassBinary) + " browse --agent codex --project " + shellQuote(fx.project)
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "24", "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "24")
			waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "ale27-browser")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "ale27-browser")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter read")
			send("Enter", "SKILLGLASS | Source", "Configured permission: allowed")
			send("d", "Select the setting scope explicitly", "g Global")
			preview := send("g", "SKILLGLASS | Codex control", "ready", "y Apply global disable", "Enter/Esc Cancel")
			assertPaneCells(t, preview, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/01-preview.txt", width), []byte(preview))
			bottom := send("G", "Confirm with --confirm")
			assertPaneCells(t, bottom, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/02-preview-bottom.txt", width), []byte(bottom))
			send("Escape", "SKILLGLASS | Source", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("d", "Select the setting scope explicitly")
			send("p", "unsupported", "project controls do not fall back")
			send("Escape", "SKILLGLASS | Source")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("d", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global disable")
			saved := send("y", "applied", "Persistence: verified", "Effective result: achieved")
			assertPaneCells(t, saved, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/03-applied.txt", width), []byte(saved))
			if !strings.Contains(string(readBytes(t, config)), "enabled = false") {
				t.Fatal("PTY confirmation did not persist")
			}
			back := send("Enter", "SKILLGLASS | Source", "Configured permission: denied", "Result: applied | persistence: verified")
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/05-return-after-apply.txt", width), []byte(back))
			if paneContains(back, "Cancelled. No settings changed.") {
				t.Fatal("committed result returned as cancellation")
			}
			send("e", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global enable")
			writeFile(t, config, append(readBytes(t, config), []byte("\n# external while preview held\n")...), 0600)
			edited := treeManifest(t, fx.base)
			stale := send("y", "refused", "preview is stale")
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/04-stale.txt", width), []byte(stale))
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			send("Escape", "SKILLGLASS | Source")
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
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/exit.txt", width), []byte(status))
			if !bytes.Equal(targetBefore, readBytes(t, target)) {
				t.Fatal("source altered")
			}
		})
	}
}

func TestALE27BrowserAppliedUnmetExitsNonzeroAfterCancellation(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	fx, root := codexFixture(t)
	target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), strings.Repeat("long-context-source-", 3), "blocked-browser")
	config := filepath.Join(fx.home, ".codex/config.toml")
	projectConfig := filepath.Join(root, ".codex/config.toml")
	writeFile(t, config, []byte(selector(target, false)+trustProject(root)), 0600)
	writeFile(t, projectConfig, []byte(selector(target, false)), 0600)
	projectBefore := readBytes(t, projectConfig)
	socket := fmt.Sprintf("ale27-unmet-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	command := "exec /usr/bin/sandbox-exec -p '(version 1)(allow default)(deny network*)' " + shellQuote(skillglassBinary) + " browse --agent codex --project " + shellQuote(fx.project)
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "48", "-y", "24", "-s", "browser", command)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", "48", "-y", "24")
	waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources")
	send := func(key string, needles ...string) string {
		return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
	}
	send("/", "Enter keep")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "blocked-browser")
	waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
	send("Enter", "Enter read")
	send("Enter", "Configured permission: denied")
	send("e", "Select the setting scope explicitly")
	send("g", "ready", "y Apply global enable")
	pane := send("y", "applied", "Persistence: verified", "Effective result: unmet", "Persisted permission: allowed", "Availability: disabled")
	assertPaneCells(t, pane, 48, 24)
	writeEvidence(t, "tmux/ale27/unmet/01-applied-unmet.txt", []byte(pane))
	if !bytes.Equal(projectBefore, readBytes(t, projectConfig)) {
		t.Fatal("project layer changed")
	}
	if !strings.Contains(string(readBytes(t, config)), "enabled = true") {
		t.Fatal("applied persistence missing")
	}
	before := treeManifest(t, fx.base)
	back := send("Escape", "SKILLGLASS | Source", "Result: applied | persistence: verified")
	writeEvidence(t, "tmux/ale27/unmet/02-return-after-unmet.txt", []byte(back))
	if paneContains(back, "Cancelled. No settings changed.") {
		t.Fatal("unmet committed result returned as cancellation")
	}
	send("e", "Select the setting scope explicitly")
	send("g", "ready")
	send("Escape", "Cancelled. No settings changed.")
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
	status := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status = strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "browser:0.0", "#{pane_dead}:#{pane_dead_status}"))
		if strings.HasPrefix(status, "1:") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status != "1:1" {
		t.Fatal("unmet saved choice reported success after cancel", status)
	}
	writeEvidence(t, "tmux/ale27/unmet/exit.txt", []byte(status))
}
