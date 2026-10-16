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

func TestALE32BrowserCancelConfirmStaleAndExit(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, profile := hermesFixture(t)
			target := codexSkill(t, filepath.Join(fx.base, "allowed"), strings.Repeat("long-source-path-", 4), "ale32-browser")
			skills := filepath.Join(profile, "skills")
			if err := os.MkdirAll(skills, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Dir(target), filepath.Join(skills, "ale32-browser")); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(profile, "config.yaml")
			writeFile(t, config, []byte("preserved: true\nskills:\n  external_dirs: [shared]\n"), 0600)
			before := treeManifest(t, fx.base)
			targetBefore := readBytes(t, target)
			socket := fmt.Sprintf("ale32-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "hermes", "--project", fx.project)
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "24", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "24")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "ale32-browser")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "ale32-browser")
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
			send("d", "Selected:", "available", "D Disable", "e Enable")
			send("D", "Select the setting scope explicitly", "g Global")
			preview := send("g", "SKILLGLASS | Hermes control", "ready", "y Apply global disable", "Enter/Esc Cancel")
			assertPaneCells(t, preview, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale32/%d/01-preview.txt", width), []byte(preview))
			bottom := send("G", "Confirm with --confirm")
			assertPaneCells(t, bottom, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale32/%d/02-preview-bottom.txt", width), []byte(bottom))
			send("Escape", "Read lines", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			unsupported := send("p", "unsupported", "No global fallback")
			writeEvidence(t, fmt.Sprintf("tmux/ale32/%d/06-project-unsupported.txt", width), []byte(unsupported))
			send("Escape", "Read lines")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global disable")
			saved := send("y", "applied", "Persistence: verified", "Effective result: achieved")
			assertPaneCells(t, saved, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale32/%d/03-applied.txt", width), []byte(saved))
			if !strings.Contains(string(readBytes(t, config)), "disabled: [\"ale32-browser\"]") {
				t.Fatal("PTY confirmation did not persist")
			}
			back := send("Enter", "Read lines", "disabled", "Result: applied | persistence: verified")
			writeEvidence(t, fmt.Sprintf("tmux/ale32/%d/05-return-after-apply.txt", width), []byte(back))
			if paneContains(back, "Cancelled. No settings changed.") {
				t.Fatal("committed result returned as cancellation")
			}
			send("e", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global enable")
			writeFile(t, config, append(readBytes(t, config), []byte("\n ")...), 0600)
			edited := treeManifest(t, fx.base)
			stale := send("y", "refused", "preview is stale")
			writeEvidence(t, fmt.Sprintf("tmux/ale32/%d/04-stale.txt", width), []byte(stale))
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			send("Escape", "Read lines")
			send("e", "Select the setting scope explicitly")
			send("g", "ready")
			send("Escape", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			status, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 1, 5*time.Second)
			writeEvidence(t, fmt.Sprintf("tmux/ale32/%d/exit.txt", width), []byte(status))
			if err != nil {
				t.Fatalf("unsuccessful confirmed mutation hidden by cancel/quit: %v", err)
			}
			if !bytes.Equal(targetBefore, readBytes(t, target)) {
				t.Fatal("source altered")
			}
		})
	}
}
