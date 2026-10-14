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

func TestALE25BrowserCompareCurrentBytesAndStaleSelection(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 60, 80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, root, a, b := compareFixture(t)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale25-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--root", root, "--project", fx.project)
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
			initial := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "Focus: 1/2")
			assertPaneCells(t, initial, width, 30)
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			marked := send("m", "Compare left:", "Left path:")
			assertPaneCells(t, marked, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale25/%d/01-marked.txt", width), []byte(marked))
			send("j", "Focus: 2/2")
			// Change after browser discovery. Comparison must read current instructions.
			writeFile(t, filepath.Join(a, "SKILL.md"), []byte("---\nname: same-name\n---\nLEFT_CURRENT\n"), 0600)
			writeFile(t, filepath.Join(b, "SKILL.md"), []byte("---\nname: same-name\n---\nRIGHT_CURRENT\n"), 0600)
			for i := 0; i < 12; i++ {
				writeFile(t, filepath.Join(a, fmt.Sprintf("z-%02d.txt", i)), []byte("left resource\n"), 0600)
				writeFile(t, filepath.Join(b, fmt.Sprintf("z-%02d.txt", i)), []byte("right resource\n"), 0600)
			}
			before = treeManifest(t, fx.base)
			pane := send("v", "SKILLGLASS | Comparison", "different")
			assertPaneCells(t, pane, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale25/%d/02-comparison-top.txt", width), []byte(pane))
			end := send("G", "z-11.txt", "left resource", "right resource")
			assertPaneCells(t, end, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale25/%d/03-diff-end.txt", width), []byte(end))
			send("Escape", "SKILLGLASS | Sources")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			// The marked directory is still selected through discovery identity, then replaced.
			// Neither independent replacement is an acceptable substitute for its old ID.
			for _, p := range []string{a, b} {
				if err := os.Rename(p, filepath.Join(fx.base, "allowed", filepath.Base(p)+"-old")); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(p, "SKILL.md"), []byte("---\nname: replacement\n---\nREPLACEMENT_NOT_SELECTED\n"), 0600)
			}
			stale := send("v", "SKILLGLASS | Comparison", "indeterminate")
			assertPaneCells(t, stale, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale25/%d/04-stale.txt", width), []byte(stale))
			send("Escape", "SKILLGLASS | Sources")
			refreshed := send("r", "Refresh:")
			if strings.Contains(refreshed, "Compare left:") {
				t.Fatal("refresh retained removed marked source")
			}
			writeEvidence(t, fmt.Sprintf("tmux/ale25/%d/05-refresh.txt", width), []byte(refreshed))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
		})
	}
}
