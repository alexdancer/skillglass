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
	act := func(action func(), needles ...string) string {
		return actForPaneAt(t, tmux, fx, socket, "paste:0.0", false, action, needles...)
	}
	outside := act(func() { paste("q\r\ns"); key("/") }, "Search: █", "Focus: 1/2", "Enter keep")
	assertSearchViewport(t, outside, 60, 30, "", true, false)
	writeEvidence(t, "tmux/paste/00-inventory-barrier.txt", []byte(outside))
	filtered := act(func() { paste("normal-skill") }, "Search: normal-skill█", "Focus: 1/1")
	writeEvidence(t, "tmux/paste/01-filtered.txt", []byte(filtered))
	assertSearchViewport(t, filtered, 60, 30, "normal-skill", true, false)
	kept := act(func() { key("Enter") }, "Search: normal-skill", "Enter read")
	assertSearchViewport(t, kept, 60, 30, "normal-skill", false, false)
	_ = act(func() { key("/") }, "Search: normal-skill█", "Enter keep")
	controls := act(func() { paste("q\r\n\x03\x15\x1b[2J\a") }, `normal-skillq\r\n\x03\x15\a`, "No matching sources.", "Ctrl+U clear")
	writeEvidence(t, "tmux/paste/02-controls.txt", []byte(controls))
	assertSearchViewport(t, controls, 60, 30, `q\r\n\x03\x15\a`, true, false)
	assertSearchChrome(t, controls)
	cancelled := act(func() { key("Escape") }, "Search: normal-skill", "Focus: 1/1", "Enter read")
	writeEvidence(t, "tmux/paste/03-cancelled.txt", []byte(cancelled))
	assertSearchViewport(t, cancelled, 60, 30, "normal-skill", false, false)
	_ = act(func() { key("/"); key("C-u") }, "Search: █", "Focus: 1/2")
	q := act(func() { paste("q") }, "Search: q█", "No matching sources.", "Enter keep")
	assertSearchViewport(t, q, 60, 30, "q", true, false)
	cleared := act(func() { key("C-u") }, "Search: █", "Focus: 1/2")
	writeEvidence(t, "tmux/paste/04-cleared.txt", []byte(cleared))
	assertSearchViewport(t, cleared, 60, 30, "", true, false)
	_ = act(func() { key("Enter") }, "Focus: 1/2", "Enter read")
	_ = act(func() { key("Enter") }, "SKILLGLASS | Source", "Manifest: parsed", "Read lines")
	detailBarrier := act(func() { paste("q\rs"); key("s") }, "SKILLGLASS | Scope", "Root 1 [searched]")
	writeEvidence(t, "tmux/paste/05-detail-barrier.txt", []byte(detailBarrier))
	scopeBarrier := act(func() { paste("q\rs"); key("Escape") }, "SKILLGLASS | Source", "Manifest: parsed", "Read lines")
	writeEvidence(t, "tmux/paste/06-scope-barrier.txt", []byte(scopeBarrier))
	_ = act(func() { key("q") }, "Pane is dead")
	status := strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "paste:0.0", "#{pane_dead}:#{pane_dead_status}"))
	writeEvidence(t, "tmux/paste/exit.txt", []byte(status+"\n"))
	if status != "1:0" {
		t.Fatalf("browser exit = %q, want dead with status 0", status)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("paste journey mutated fixture")
	}
}
