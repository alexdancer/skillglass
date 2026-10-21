package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise identity on the actual decision pages, including the pickers that
// intentionally show neither full paths nor internal source IDs.
func TestALE53MutationIdentityAndProjectContext(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, projectSelected := range []bool{false, true} {
		t.Run(fmt.Sprintf("project-%t", projectSelected), func(t *testing.T) {
			fx, _ := codexFixture(t)
			name := strings.Repeat("long-same-name-", 5)
			for _, directory := range []string{"first", "second"} {
				codexSkill(t, filepath.Join(fx.home, ".agents/skills"), directory, name)
			}
			before := treeManifest(t, fx.base)
			args := []string{"browse", "--agent", "codex", "--theme", "none"}
			context := "Inspection project: none selected"
			if projectSelected {
				args = append(args, "--project", fx.project)
				context = "Inspection project: " + fx.project
			}
			socket := fmt.Sprintf("ale53-context-%d-%t", os.Getpid(), projectSelected)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, args...)
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", "48", "-y", "30", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", "48", "-y", "30")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "Focused:")
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "long-same-name")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/2")
			frame := send("Enter", "Enter open", "Focus: 1/2", "Focused:")
			hints := []string{}
			for index := 0; index < 2; index++ {
				hint := identityHint(frameLineWithPrefix(t, frame, "Focused: "))
				if hint == "" || index > 0 && hint == hints[0] {
					t.Fatalf("independent same-name sources lack distinct hints: %s", frame)
				}
				hints = append(hints, hint)
				send("Enter", "Read lines", "Selected:")
				check := func(label, pane string) {
					t.Helper()
					assertPaneCells(t, pane, 48, 30)
					if got := identityHint(frameLineWithPrefix(t, pane, "Selected: ")); got != hint {
						t.Fatalf("%s lost selected identity: want %s, got %s\n%s", label, hint, got, pane)
					}
					writeEvidence(t, fmt.Sprintf("tmux/ale53/project-%t/source-%d/%s.txt", projectSelected, index, label), []byte(pane))
				}
				check("scope", send("D", "Select the setting scope explicitly", context))
				check("global-preview", send("g", "global disable | ready", context))
				send("Escape", "Read lines")
				check("applications", send("A", "All start unchecked"))
				send("Space", "[1] Codex")
				check("group-scope", send("Enter", "Select the group setting scope explicitly"))
				check("group-preview", send("g", "Group global disable | ready", context))
				send("Escape", "Read lines")
				check("delete-summary", send("X", "PERMANENT DELETE", context, "No trash or undo"))
				check("delete-confirmation", send("Enter", "Type: confirm delete", context))
				send("Escape", "Read lines")
				send("Escape", "Focus:")
				if index == 0 {
					frame = send("Down", "Focus: 2/2", "Focused:")
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
		})
	}
}
