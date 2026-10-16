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
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "codex", "--project", fx.project)
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "24", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "24")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "ale27-browser")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "ale27-browser")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter open")
			send("Enter", "Rendered instructions")
			send("d", "Selected:", "Configured permission: allowed")
			send("D", "Select the setting scope explicitly", "g Global")
			preview := send("g", "SKILLGLASS | Codex control", "ready", "y Apply global disable", "Enter/Esc Cancel")
			assertPaneCells(t, preview, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/01-preview.txt", width), []byte(preview))
			bottom := send("G", "Confirm with --confirm")
			assertPaneCells(t, bottom, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/02-preview-bottom.txt", width), []byte(bottom))
			send("Escape", "Read lines", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			send("p", "unsupported", "project controls do not fall back")
			send("Escape", "Read lines")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global disable")
			saved := send("y", "applied", "Persistence: verified", "Effective result: achieved")
			assertPaneCells(t, saved, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/03-applied.txt", width), []byte(saved))
			if !strings.Contains(string(readBytes(t, config)), "enabled = false") {
				t.Fatal("PTY confirmation did not persist")
			}
			back := send("Enter", "Read lines", "Configured permission: denied", "Result: applied | persistence: verified")
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
			send("Escape", "Read lines")
			send("e", "Select the setting scope explicitly")
			send("g", "ready")
			send("Escape", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			status, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 1, 5*time.Second)
			writeEvidence(t, fmt.Sprintf("tmux/ale27/%d/exit.txt", width), []byte(status))
			if err != nil {
				t.Fatalf("unsuccessful confirmed mutation hidden by cancel/quit: %v", err)
			}
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
	argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "codex", "--project", fx.project)
	command := exec.Command(argv[0], argv[1:]...)
	command.Dir, command.Env = fx.project, fx.environment()
	runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", "48", "-y", "24", "-s", "browser")
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", "48", "-y", "24")
	waitForPane(t, tmux, fx, socket, false, "Skillglass")
	send := func(key string, needles ...string) string {
		return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
	}
	send("/", "Enter keep")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "blocked-browser")
	waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
	send("Enter", "Enter open")
	send("Enter", "Rendered instructions")
	send("d", "Configured permission: denied")
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
	back := send("Escape", "Read lines", "Result: applied | persistence: verified")
	writeEvidence(t, "tmux/ale27/unmet/02-return-after-unmet.txt", []byte(back))
	if paneContains(back, "Cancelled. No settings changed.") {
		t.Fatal("unmet committed result returned as cancellation")
	}
	send("e", "Select the setting scope explicitly")
	send("g", "ready")
	send("Escape", "Cancelled. No settings changed.")
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
	status, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 1, 5*time.Second)
	writeEvidence(t, "tmux/ale27/unmet/exit.txt", []byte(status))
	if err != nil {
		t.Fatal("unmet saved choice reported success after cancel", err)
	}
}
