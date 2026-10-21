package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestALE34BrowserCancelConfirmStaleAndExit(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx := piFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), strings.Repeat("long-source-path-", 4), "ale34-browser")
			config := filepath.Join(fx.home, ".pi/agent/settings.json")
			unrelated := make([]string, 80)
			for i := range unrelated {
				unrelated[i] = fmt.Sprintf("!ALE53_UNRELATED_%03d/**", i)
			}
			initial, err := json.Marshal(map[string]any{"defaultProjectTrust": "always", "keep": 123, "skills": unrelated})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, config, initial, 0600)
			before := treeManifest(t, fx.base)
			targetBefore := readBytes(t, target)
			socket := fmt.Sprintf("ale34-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "pi", "--project", fx.project)
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "24", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "24")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "ale34-browser")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "ale34-browser")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter open")
			send("Enter", "Rendered instructions")
			send("d", "Selected:", "Configured permission: allowed")
			send("D", "Select the setting scope explicitly", "g Global")
			preview := send("g", "SKILLGLASS | Pi control", "ready", "y Apply global disable", "Enter/Esc Cancel")
			assertPaneCells(t, preview, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale34/%d/01-preview.txt", width), []byte(preview))
			// Walk the complete summary so large unrelated configuration arrays
			// cannot be hidden below the first viewport.
			page := preview
			var summary strings.Builder
			for n := 0; n < 30; n++ {
				summary.WriteString(page)
				if paneContains(page, "Preview only. No settings changed.") {
					break
				}
				if n == 29 {
					t.Fatal("summary did not terminate")
				}
				old := page
				page = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(frame paneSnapshot) bool { return frame.cells != old && paneHas(frame, false, "Control lines") }).cells
			}
			if strings.Contains(summary.String(), "ALE53_UNRELATED") {
				t.Fatal("summary exposed unrelated settings array")
			}
			writeEvidence(t, fmt.Sprintf("tmux/ale34/%d/01-full-summary.txt", width), []byte(summary.String()))
			send("d", "Source ID:", "ALE53_UNRELATED_000")
			bottom := send("G", "Preview only. No settings changed.")
			assertPaneCells(t, bottom, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale34/%d/02-preview-bottom.txt", width), []byte(bottom))
			send("Escape", "Read lines", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			send("p", "ready", "y Apply project disable")
			send("Escape", "Read lines")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			send("D", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global disable")
			saved := send("y", "applied", "Persistence: verified", "Effective result: achieved", "Reload: required")
			assertPaneCells(t, saved, width, 24)
			writeEvidence(t, fmt.Sprintf("tmux/ale34/%d/03-applied.txt", width), []byte(saved))
			persisted := piSettingsValues(t, config)
			var retained []string
			if err := json.Unmarshal(persisted["skills"], &retained); err != nil {
				t.Fatal(err)
			}
			if len(retained) != len(unrelated)+1 {
				t.Fatalf("unrelated settings lost: %d entries", len(retained))
			}
			for i, want := range unrelated {
				if retained[i] != want {
					t.Fatalf("unrelated setting %d changed", i)
				}
			}
			if !strings.Contains(string(readBytes(t, config)), "-"+target) {
				t.Fatal("PTY confirmation did not persist")
			}
			back := send("Enter", "Read lines", "Configured permission: denied", "Result: applied | persistence: verified")
			writeEvidence(t, fmt.Sprintf("tmux/ale34/%d/05-return-after-apply.txt", width), []byte(back))
			if paneContains(back, "Cancelled. No settings changed.") {
				t.Fatal("committed result returned as cancellation")
			}
			send("e", "Select the setting scope explicitly")
			send("g", "ready", "y Apply global enable")
			doc := piSettingsValues(t, config)
			doc["external"] = json.RawMessage(`true`)
			external, _ := json.Marshal(doc)
			writeFile(t, config, external, 0600)
			edited := treeManifest(t, fx.base)
			stale := send("y", "refused", "preview is stale")
			writeEvidence(t, fmt.Sprintf("tmux/ale34/%d/04-stale.txt", width), []byte(stale))
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			send("Escape", "Read lines")
			send("e", "Select the setting scope explicitly")
			send("g", "ready")
			send("Escape", "Cancelled. No settings changed.")
			assertTreeUnchanged(t, edited, treeManifest(t, fx.base))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			status, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 1, 5*time.Second)
			writeEvidence(t, fmt.Sprintf("tmux/ale34/%d/exit.txt", width), []byte(status))
			if err != nil {
				t.Fatalf("unsuccessful confirmed mutation hidden by cancel/quit: %v", err)
			}
			if !bytes.Equal(targetBefore, readBytes(t, target)) {
				t.Fatal("source altered")
			}
		})
	}
}
