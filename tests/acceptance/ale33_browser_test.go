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

func TestALE33PiBrowserReadExplanationFiltersAndScope(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx := piFixture(t)
			manifest := codexSkill(t, filepath.Join(fx.home, ".pi", "agent", "skills"), strings.Repeat("long-pi-source-", 5), "browser-pi")
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale33-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse --agent pi --project "+shellQuote(fx.project))
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			pane := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "browser-pi", "Agent: pi")
			assertPaneCells(t, pane, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale33/%d/01-sources.txt", width), []byte(pane))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			detail := send("Enter", "Read lines", "Availability: available", "Session: unknown")
			assertPaneCells(t, detail, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale33/%d/02-read.txt", width), []byte(detail))
			frame := detail
			var explanation strings.Builder
			for page := 0; page < 20; page++ {
				first, last, total, body := sourceDetailBody(t, frame)
				explanation.WriteString(strings.Join(body, "\n"))
				writeEvidence(t, fmt.Sprintf("tmux/ale33/%d/02-explanation-%02d.txt", width, page), []byte(frame))
				if last == total {
					break
				}
				frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool {
					nextFirst, _, _ := sourceDetailRange(next.cells)
					return nextFirst > first
				}).cells
				assertPaneCells(t, frame, width, 30)
			}
			for _, rule := range []string{"resource-filters", "native first name wins", "no running Pi session"} {
				if !paneContains(explanation.String(), rule) {
					t.Fatalf("Pi browser explanation lacks %s: %s", rule, explanation.String())
				}
			}

			writeFile(t, manifest, []byte("---\nname: browser-pi\ndescription: Native Pi browser fixture.\ndisable-model-invocation: true\n---\nBody\n"), 0600)
			before = treeManifest(t, fx.base)
			send("r", "Refresh:")
			send("Escape", "SKILLGLASS | Sources")
			frame = send("a", "Availability filter: available", "0 matches")
			assertPaneCells(t, frame, width, 30)
			send("a", "Availability filter: disabled")
			frame = send("a", "Availability filter: explicit-only", "browser-pi")
			assertPaneCells(t, frame, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale33/%d/03-filter.txt", width), []byte(frame))
			scope := send("s", "SKILLGLASS | Scope and filters", "Agent: pi | Validated: pi 1.1.0")
			assertPaneCells(t, scope, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale33/%d/04-scope.txt", width), []byte(scope))
			send("Escape", "SKILLGLASS | Sources")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE33PiBrowserStandaloneFileRefresh(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	fx := piFixture(t)
	file := filepath.Join(fx.home, ".pi", "agent", "skills", "file.md")
	writeFile(t, file, []byte("---\nname: file-skill\ndescription: Standalone file fixture.\n---\nBody.\n"), 0600)
	socket := fmt.Sprintf("ale33-file-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse --agent pi --project "+shellQuote(fx.project))
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	waitForPane(t, tmux, fx, socket, false, "file-skill", "Agent: pi")
	actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Read lines", "Availability: available")
	before := treeManifest(t, fx.base)
	frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "r") }, "Refresh: selected file object retained")
	assertPaneCells(t, frame, 80, 30)
	writeEvidence(t, "tmux/ale33/file-refresh.txt", []byte(frame))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
