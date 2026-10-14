package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRound5RelativeTrustGapBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			fx := geminiFixture(t)
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":true}}}`), 0600)
			writeFile(t, filepath.Join(fx.home, ".gemini/trustedFolders.json"), []byte(`{"relative":"TRUST_FOLDER"}`), 0600)
			codexSkill(t, filepath.Join(fx.project, ".agents/skills"), "source", "relative-trust")
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale37-r5-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec " + fx.productShell(t) + " browse --agent gemini --project " + shellQuote(fx.project)
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", fmt.Sprint(width), "-y", "24", "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", fmt.Sprint(width), "-y", "24")
			waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "relative-trust", "unknown")
			for _, phase := range []string{"initial", "refreshed"} {
				actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "s") }, "SKILLGLASS | Scope and filters")
				reason := "Unverified: relative trusted-folder keys require an uninspected native working directory"
				frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "G") }, reason)
				assertPaneCells(t, frame, width, 24)
				compact := strings.Join(strings.Fields(frame), "")
				if count := strings.Count(compact, strings.Join(strings.Fields(reason), "")); count != 1 {
					t.Errorf("browser scope contains relative-key gap %d times, want once:\n%s", count, frame)
				}
				writeEvidence(t, fmt.Sprintf("round5/browser/%d/%s.txt", width, phase), []byte(frame))
				actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Escape") }, "SKILLGLASS | Sources", "relative-trust", "unknown")
				if phase == "initial" {
					actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "r") }, "Refresh: collection updated", "unknown")
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
