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

func TestALE40BrowserGroupJourney(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 120} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/partial-%t", width, partial), func(t *testing.T) {
				fx, _, target := ale40Fixture(t, false)
				long := filepath.Join(fx.base, "allowed", strings.Repeat("long-source-directory-", 7), "ale40-target")
				if err := os.MkdirAll(filepath.Dir(long), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Dir(target), long); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(long, filepath.Dir(target)); err != nil {
					t.Fatal(err)
				}
				sourceBefore := readBytes(t, filepath.Join(long, "SKILL.md"))
				third := filepath.Join(fx.home, ".hermes/config.yaml")
				thirdBefore := readBytes(t, third)
				policy := ""
				if partial {
					policy = "(deny file-write* (subpath " + sandboxString(filepath.Join(fx.home, ".claude")) + "))"
				}
				socket := fmt.Sprintf("ale40-%d-%d-%t", os.Getpid(), width, partial)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				argv := fx.launchArgs(t, skillglassBinary, true, policy, "browse", "--agent", "codex", "--project", fx.project)
				command := exec.Command(argv[0], argv[1:]...)
				command.Dir, command.Env = fx.project, fx.environment()
				runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "28", "-s", "browser")
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "28")
				waitForPane(t, tmux, fx, socket, false, "Skillglass", "ale40-target")
				send := func(key string, needles ...string) string {
					return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
				}
				capture := func(name, text string) {
					assertPaneCells(t, text, width, 28)
					writeEvidence(t, fmt.Sprintf("tmux/ale40/%d/partial-%t/%s.txt", width, partial, name), []byte(text))
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
				capture("01-checklist", send("A", "Choose applications explicitly", "[ ] Codex", "[ ] Claude Code"))
				send("Space", "[1] Codex")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
				send("Space", "[2] Claude Code")
				if partial {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
					send("Space", "[3] Hermes")
				}
				send("Enter", "Select the group setting scope explicitly")
				capture("02-preview", send("g", "Group global disable | ready", "y Apply group"))
				send("d", "Source ID:")
				capture("03-preview-bottom", send("G", "Preview only. No settings changed.", "Session adoption is unknown"))
				send("Escape", "Cancelled. No settings changed.")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				send("A", "All start unchecked", "[ ] Codex")
				send("Space", "[1] Codex")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
				send("Space", "[2] Claude Code")
				if partial {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
					send("Space", "[3] Hermes")
					send("Enter", "Select the group setting scope explicitly")
					send("g", "ready")
					capture("04-partial-result", send("y", "Group result: failed", "1. Codex: succeeded", "2. Claude Code: failed", "3. Hermes: not-attempted"))
					if !strings.Contains(string(readBytes(t, filepath.Join(fx.home, ".codex/config.toml"))), "enabled = false") {
						t.Fatal("prior success lost")
					}
					if !bytes.Equal(thirdBefore, readBytes(t, third)) {
						t.Fatal("unattempted third changed")
					}
				} else {
					// Add CodeAF explicitly. The request refuses without applying its ready members.
					for i := 0; i < 5; i++ {
						runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
					}
					send("Space", "[3] CodeAF")
					send("Enter", "Select the group setting scope explicitly")
					capture("04-mixed-refusal", send("g", "Whole request unapplied", "unsupported"))
					assertTreeUnchanged(t, before, treeManifest(t, fx.base))
					send("s", "Edit selection", "[3] CodeAF")
					send("Space", "[ ] CodeAF")
					send("Enter", "Select the group setting scope explicitly")
					capture("05-fresh-subset", send("g", "ready", "y Apply group"))
					capture("06-success", send("y", "Group result: applied", "1. Codex: succeeded", "2. Claude Code: succeeded"))
					send("Enter", "Read lines", "Group result: applied")
					send("E", "All start unchecked")
					send("Space", "[1] Codex")
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down")
					send("Space", "[2] Claude Code")
					send("Enter", "Select the group setting scope explicitly")
					send("g", "ready")
					path := filepath.Join(fx.home, ".claude/settings.json")
					writeFile(t, path, append(readBytes(t, path), []byte("\n ")...), 0600)
					edited := treeManifest(t, fx.base)
					capture("07-stale", send("y", "Group result: refused", "group preview is stale"))
					assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
				}
				if !bytes.Equal(sourceBefore, readBytes(t, filepath.Join(long, "SKILL.md"))) {
					t.Fatal("source changed")
				}
				send("Enter", "Read lines", "Group result:")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
				status, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 1, 5*time.Second)
				writeEvidence(t, fmt.Sprintf("tmux/ale40/%d/partial-%t/exit.txt", width, partial), []byte(status))
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
