package acceptance_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserBracketedPasteIsSearchData(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux is required for bracketed-paste regression")
	}
	fx := newFixture(t)
	before := treeManifest(t, fx.base)
	socket := fmt.Sprintf("skillglass-paste-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{"exec", shellQuote(skillglassBinary), "browse", "--root", shellQuote(fx.cleanRoot), "--project", shellQuote(fx.project)}, " ")
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "60", "-y", "30", "-s", "paste", commandLine)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "paste:0", "remain-on-exit", "on")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "paste:0", "-x", "60", "-y", "30")
	rawPath := filepath.Join(t.TempDir(), "paste.ansi")
	runTmux(t, tmux, fx, socket, "pipe-pane", "-O", "-t", "paste:0.0", "cat > "+shellQuote(rawPath))
	defer func() {
		if raw, err := os.ReadFile(rawPath); err == nil {
			writeEvidence(t, "tmux/paste/raw.ansi", raw)
		}
	}()
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Focus: 1/2", "/ search")
	writeEvidence(t, "tmux/paste/command.txt", []byte(commandLine+"\n"))
	writeEvidence(t, "tmux/paste/dimensions.txt", []byte("60x30\n"))
	var keys strings.Builder
	defer func() { writeEvidence(t, "tmux/paste/keys.txt", []byte(keys.String())) }()
	paste := func(content string) {
		payload := "\x1b[200~" + content + "\x1b[201~"
		keys.WriteString(payload)
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "paste:0.0", "-l", payload)
	}
	key := func(name string) {
		fmt.Fprintf(&keys, "\n<%s>\n", name)
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "paste:0.0", name)
	}
	paste("q\r\ns")
	outside := waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Focus: 1/2", "/ search")
	assertSearchViewport(t, outside, 60, 30, "", false, false)
	key("/")
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Search: █", "Enter keep")
	paste("normal-skill")
	filtered := waitForPaneCaptureAt(t, tmux, fx, socket, "paste:0.0", false, "Search: normal-skill█", "Focus: 1/1")
	writeEvidence(t, "tmux/paste/01-filtered.txt", []byte(filtered))
	assertSearchViewport(t, filtered, 60, 30, "normal-skill", true, false)
	if !strings.Contains(filtered, "Focus: 1/1") {
		t.Fatalf("paste did not filter:\n%s", filtered)
	}
	key("Enter")
	kept := waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Search: normal-skill", "Enter read")
	assertSearchViewport(t, kept, 60, 30, "normal-skill", false, false)
	key("/")
	paste("q\r\n\x03\x15\x1b[2J\a")
	controls := waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, `normal-skillq\r\n\x03\x15\a`, "No matching sources.", "Ctrl+U clear")
	writeEvidence(t, "tmux/paste/02-controls.txt", []byte(controls))
	assertSearchViewport(t, controls, 60, 30, `q\r\n\x03\x15\a`, true, false)
	assertSearchChrome(t, controls)
	key("Escape")
	cancelled := waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Search: normal-skill", "Focus: 1/1", "Enter read")
	writeEvidence(t, "tmux/paste/03-cancelled.txt", []byte(cancelled))
	assertSearchViewport(t, cancelled, 60, 30, "normal-skill", false, false)
	key("/")
	key("C-u")
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Search: █", "Focus: 1/2")
	paste("q")
	q := waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Search: q█", "No matching sources.", "Enter keep")
	assertSearchViewport(t, q, 60, 30, "q", true, false)
	key("C-u")
	cleared := waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Search: █", "Focus: 1/2")
	writeEvidence(t, "tmux/paste/04-cleared.txt", []byte(cleared))
	assertSearchViewport(t, cleared, 60, 30, "", true, false)
	key("Enter")
	key("Enter")
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Manifest: parsed", "Esc back")
	paste("q\rs")
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Manifest: parsed", "Esc back")
	key("s")
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "SKILLGLASS | Scope", "Root 1 [searched]")
	paste("q\rs")
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "SKILLGLASS | Scope", "Root 1 [searched]")
	key("q")
	_ = waitForPaneAt(t, tmux, fx, socket, "paste:0.0", false, "Pane is dead")
	status := strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "paste:0.0", "#{pane_dead}:#{pane_dead_status}"))
	writeEvidence(t, "tmux/paste/exit.txt", []byte(status+"\n"))
	if status != "1:0" {
		t.Fatalf("browser exit = %q, want dead with status 0", status)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("paste journey mutated fixture")
	}
}
