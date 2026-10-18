package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestALE41BrowserManifestImpactAndManagerRevalidation(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, managerInput := range []string{"installed-registry", "pi-settings"} {
		t.Run(managerInput, func(t *testing.T) {
			fx, _ := codexFixture(t)
			writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), []byte(`{"defaultProjectTrust":"always"}`), 0600)
			target := filepath.Join(fx.home, ".pi/agent/skills/standalone.md")
			writeFile(t, target, []byte("---\nname: delete-impact\ndescription: Inert fixture.\n---\nBody.\n"), 0600)
			dependent := filepath.Join(fx.home, ".codex/skills/dependent/SKILL.md")
			if err := os.MkdirAll(filepath.Dir(dependent), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, dependent); err != nil {
				t.Fatal(err)
			}
			listed := piList(t, fx, 0)
			id := codexNamed(t, listed, "delete-impact").ID
			socket := fmt.Sprintf("ale41-impact-%d", os.Getpid())
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			argv := fx.launchArgs(t, skillglassBinary, true, "", "browse", "--agent", "pi", "--project", fx.project, "--theme", "none")
			command := exec.Command(argv[0], argv[1:]...)
			command.Dir, command.Env = fx.project, fx.environment()
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", "120", "-y", "28", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", "120", "-y", "28")
			waitForPane(t, tmux, fx, socket, false, "Skillglass", "delete-impact")
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "delete-impact")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter focus")
			send("Enter", "Selected:", "Read lines")
			before := treeManifest(t, fx.home)
			page := send("X", "PERMANENT DELETE", "ready")
			var transcript strings.Builder
			for n := 0; n < 100; n++ {
				assertPaneCells(t, page, 120, 28)
				transcript.WriteString(page)
				if strings.Contains(page, "Type confirmation on the next screen:") {
					break
				}
				old := page
				page = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(frame paneSnapshot) bool {
					return frame.cells != old && paneHas(frame, false, "Delete lines")
				}).cells
			}
			text := transcript.String()
			assertPiDeleteScopeCopy(t, text)
			writeEvidence(t, "tmux/ale41-impact/"+managerInput+"/preview.txt", []byte(text))
			for _, needle := range []string{"Dependent manifest:", "dependent/SKILL.md", "Independent source:", "Dependent provenance:", "Type confirmation on the next screen:"} {
				if !paneContains(text, needle) {
					t.Fatal("browser omitted dependency impact", needle)
				}
			}
			send("Escape", "Cancelled. No further content changed.")
			assertTreeUnchanged(t, before, treeManifest(t, fx.home))
			send("X", "PERMANENT DELETE", "ready")
			send("Enter", "Type: delete src_")
			if managerInput == "pi-settings" {
				writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), []byte(`{"packages":"invalid"}`), 0600)
			} else {
				writeFile(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), []byte("{invalid"), 0600)
			}
			changed := treeManifest(t, fx.home)
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "delete "+id)
			waitForPane(t, tmux, fx, socket, false, "Input: delete src_")
			result := send("Enter", "Permanent delete result: stale", "preview is stale")
			writeEvidence(t, "tmux/ale41-impact/"+managerInput+"/stale.txt", []byte(result))
			assertTreeUnchanged(t, changed, treeManifest(t, fx.home))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			if _, err := waitForTmuxExit(t, tmux, fx, socket, "browser:0.0", 1, 5*time.Second); err != nil {
				t.Fatal(err)
			}
		})
	}
}
