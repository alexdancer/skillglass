package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestALE31HermesBrowserExplanationFilterRefreshAndNarrowScope(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, profile := hermesFixture(t)
			manifest := codexSkill(t, filepath.Join(profile, "skills"), "very-long-source-path-for-native-hermes-profile-browser-artifact", "browser-hermes")
			writeFile(t, manifest, []byte("---\nname: browser-hermes\ndescription: Inert.\nplatforms: false\nenvironments: false\n---\nBrowser content.\n"), 0600)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale31-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--agent", "hermes", "--project", fx.project, "--availability", "available")
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
			frame := waitForPane(t, tmux, fx, socket, false, "Skillglass", "browser-hermes", "Agent: hermes", "available (inferred)")
			assertPaneCells(t, frame, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale31/%d/01-sources.txt", width), []byte(frame))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("Enter", "Rendered instructions")
			detail := send("d", "Read lines", "Availability: available", "Configured permission: allowed", "Session: unknown")
			assertPaneCells(t, detail, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale31/%d/02-detail.txt", width), []byte(detail))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  platform_disabled:\n    telegram: [browser-hermes]\n"), 0600)
			writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=telegram\n"), 0600)
			before = treeManifest(t, fx.base)
			detail = send("r", "Refresh: selected directory object retained", "Availability: disabled", "Configured permission: denied")
			assertPaneCells(t, detail, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale31/%d/03-refresh.txt", width), []byte(detail))
			send("Escape", "Search /", "0 matches")
			filtered := send("a", "Availability filter: disabled", "browser-hermes", "disabled (inferred)")
			assertPaneCells(t, filtered, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale31/%d/04-filter.txt", width), []byte(filtered))
			scope := send("s", "Scope and filters", "Agent: hermes", "Validated rules: Hermes 0.20.0")
			assertPaneCells(t, scope, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale31/%d/05-scope.txt", width), []byte(scope))
			send("Escape", "Search /")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
