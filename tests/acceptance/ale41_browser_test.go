package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestALE41BrowserTypedDelete(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 60, 80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, _, target := ale40Fixture(t, false)
			long := filepath.Join(fx.base, "allowed", strings.Repeat("long-delete-directory-", 7), "ale40-target")
			if err := os.MkdirAll(filepath.Dir(long), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Dir(target), long); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(long, filepath.Dir(target)); err != nil {
				t.Fatal(err)
			}
			id := changeSourceID(t, fx, filepath.Join(long, "SKILL.md"))
			socket := fmt.Sprintf("ale41-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "codex", "--project", fx.project, "--theme", "none")
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "ale40-target")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			capture := func(name, text string) {
				assertPaneCells(t, text, width, 30)
				writeEvidence(t, fmt.Sprintf("tmux/ale41/%d/%s.txt", width, name), []byte(text))
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "ale40-target")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			if width < 96 {
				send("Enter", "Enter open")
			} else {
				send("Enter", "Enter focus")
			}
			send("Enter", "Selected:", "Read lines")
			before := treeManifest(t, fx.base)
			capture("01-preview", send("X", "PERMANENT DELETE", "ready", "Exact target:"))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "y")
			waitForPane(t, tmux, fx, socket, false, "PERMANENT DELETE", "ready")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("d", "Source ID:")
			capture("02-bottom", send("G", "Preview only. No content or settings changed."))
			capture("02-summary", send("i", "Exact target:", "Known references:"))
			send("Enter", "Type: confirm delete")
			capture("03-empty-refused", send("Enter", "Confirmation does not match"))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "delete "+id)
			waitForPane(t, tmux, fx, socket, false, "Input: delete "+id)
			capture("03-old-phrase-refused", send("Enter", "Confirmation does not match"))
			actOnPane(t, tmux, fx, socket, "browser:0.0", func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "C-u")
			}, func(frame paneSnapshot) bool {
				for _, line := range strings.Split(frame.cells, "\n") {
					if strings.TrimSpace(line) == "Input:" {
						return true
					}
				}
				return false
			})
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "digrqy")
			capture("03-literal-input", waitForPane(t, tmux, fx, socket, false, "Input: digrqy"))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("Escape", "Cancelled. No further content changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("X", "PERMANENT DELETE", "ready")
			send("Enter", "Type: confirm delete")
			manifest := filepath.Join(long, "SKILL.md")
			writeFile(t, manifest, append(readBytes(t, manifest), []byte("external edit\n")...), 0600)
			changed := treeManifest(t, fx.base)
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "confirm delete")
			waitForPane(t, tmux, fx, socket, false, "Input: confirm delete")
			capture("04-stale", send("Enter", "Permanent delete result: stale", "preview is stale"))
			assertTreeUnchanged(t, changed, treeManifest(t, fx.base))
			send("Escape", "Delete result: stale")
			send("Enter", "Read lines", "Selected:")
			send("X", "PERMANENT DELETE", "ready")
			send("Enter", "Type: confirm delete")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "confirm delete")
			waitForPane(t, tmux, fx, socket, false, "Input: confirm delete")
			capture("04-consent", waitForPane(t, tmux, fx, socket, false, "Input: confirm delete"))
			capture("05-result", send("Enter", "Permanent delete result: removed", "Verified removal: true"))
			if _, err := os.Lstat(long); !os.IsNotExist(err) {
				t.Fatal("selected source survives", err)
			}
			if info, err := os.Lstat(filepath.Dir(target)); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("alias removed", err)
			}
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			status, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 1, 5*time.Second)
			writeEvidence(t, fmt.Sprintf("tmux/ale41/%d/exit.txt", width), []byte(status))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
