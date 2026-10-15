package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestALE28BrowserProjectAndInheritanceRefusal(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, root := codexFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "project-control", "ale28-browser")
			writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte(selector(target, false)+trustProject(root)), 0600)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale28-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "codex", "--project", fx.project)
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "24", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "24")
			waitForPane(t, tmux, fx, socket, false, "Skillglass")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "ale28-browser")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter open")
			send("Enter", "Rendered instructions")
			for _, action := range []string{"D", "e"} {
				for _, scope := range []string{"p", "i"} {
					menu := send(action, "Select the setting scope explicitly", "i Remove project override")
					assertPaneCells(t, menu, width, 24)
					refusal := send(scope, "unsupported", "Codex 0.155.1", "skills/list")
					assertPaneCells(t, refusal, width, 24)
					writeEvidence(t, fmt.Sprintf("tmux/ale28/%d/%s-%s.txt", width, action, scope), []byte(refusal))
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "y")
					waitForPane(t, tmux, fx, socket, false, "unsupported")
					send("Escape", "Cancelled. No settings changed.")
					assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				}
			}
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
		})
	}
}
