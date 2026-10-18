package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestALE41BrowserNestedManifestRefusal(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, _ := codexFixture(t)
			parent := filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "custom"), "parent", "browser-nested-parent"))
			nested := filepath.Join(parent, "nested", "skill.md")
			writeFile(t, nested, []byte("---\nname: independent-child\ndescription: Inert fixture.\n---\nChild.\n"), 0600)
			if _, err := os.Lstat(filepath.Join(filepath.Dir(nested), "SKILL.md")); os.IsNotExist(err) {
				if err := os.Rename(nested, filepath.Join(filepath.Dir(nested), "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale41-nested-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--root", parent, "--project", fx.project, "--theme", "none")
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "28", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "28")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "browser-nested-parent")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "browser-nested-parent")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			if width < 96 {
				send("Enter", "Enter open")
			} else {
				send("Enter", "Enter focus")
			}
			send("Enter", "Selected:", "Read lines")
			page := send("X", "PERMANENT DELETE", "refused", "another bundle", "Enter back")
			assertPaneCells(t, page, width, 28)
			writeEvidence(t, fmt.Sprintf("tmux/ale41-nested/%d/refused.txt", width), []byte(page))
			back := send("Enter", "browser-nested-parent", "Focus: 1/1")
			assertPaneCells(t, back, width, 28)
			writeEvidence(t, fmt.Sprintf("tmux/ale41-nested/%d/back.txt", width), []byte(back))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			if _, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 0, 5*time.Second); err != nil {
				t.Fatal(err)
			}
		})
	}
}
