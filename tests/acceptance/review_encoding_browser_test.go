package acceptance_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserInvalidManifestEncodingHasRecoveryHint(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux is required for encoding recovery regression")
	}
	fx := newFixture(t)
	bundle := filepath.Join(fx.base, "allowed", "roots", "invalid-encoding")
	rawManifest := []byte("---\nname: invalid-encoding\n---\nDO_NOT_RENDER_\xff_SENTINEL\n")
	writeFile(t, filepath.Join(bundle, "SKILL.md"), rawManifest, 0o644)
	writeFile(t, filepath.Join(bundle, "resource.txt"), []byte("RESOURCE\n"), 0o644)
	before := treeManifest(t, fx.base)
	socket := fmt.Sprintf("skillglass-encoding-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{"exec", shellQuote(skillglassBinary), "browse", "--root", shellQuote(bundle), "--project", shellQuote(fx.project)}, " ")
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "120", "-y", "35", "-s", "encoding", commandLine)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "encoding:0", "remain-on-exit", "on")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "encoding:0", "-x", "120", "-y", "35")
	rawPath := filepath.Join(t.TempDir(), "encoding.ansi")
	runTmux(t, tmux, fx, socket, "pipe-pane", "-O", "-t", "encoding:0.0", "cat > "+shellQuote(rawPath))
	defer func() {
		if raw, err := os.ReadFile(rawPath); err == nil {
			writeEvidence(t, "tmux/encoding/raw.ansi", raw)
		}
	}()
	_ = waitForPaneAt(t, tmux, fx, socket, "encoding:0.0", false, "Selected: none", "Focus: 1/1")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "encoding:0.0", "Enter")
	joined := waitForPaneCaptureAt(t, tmux, fx, socket, "encoding:0.0", true, "manifest-invalid-encoding", "manifest.raw_bytes_base64", "Manifest: malformed")
	cells := capturePaneAt(t, tmux, fx, socket, "encoding:0.0", false)
	writeEvidence(t, "tmux/encoding/detail.txt", []byte(cells))
	writeEvidence(t, "tmux/encoding/detail-joined.txt", []byte(joined))
	writeEvidence(t, "tmux/encoding/command.txt", []byte(commandLine+"\n"))
	writeEvidence(t, "tmux/encoding/keys.txt", []byte("Enter\nq\n"))
	writeEvidence(t, "tmux/encoding/dimensions.txt", []byte("120x35\n"))
	assertPaneCells(t, cells, 120, 35)
	for _, expected := range []string{"Coverage: incomplete", "Manifest: malformed", "manifest-invalid-encoding", "list/read --json", "manifest.raw_bytes_base64", "resource.txt", "Availability: unknown"} {
		if !paneContains(joined, expected) {
			t.Fatalf("recovery detail missing %q:\n%s", expected, joined)
		}
	}
	for _, forbidden := range []string{"DO_NOT_RENDER", "SENTINEL", "�", "Raw manifest:", "Instructions:", base64.StdEncoding.EncodeToString(rawManifest)} {
		if paneContains(joined, forbidden) {
			t.Fatalf("invalid body displayed %q:\n%s", forbidden, joined)
		}
	}
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "encoding:0.0", "q")
	_ = waitForPaneAt(t, tmux, fx, socket, "encoding:0.0", false, "Pane is dead")
	status := strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "encoding:0.0", "#{pane_dead}:#{pane_dead_status}"))
	writeEvidence(t, "tmux/encoding/exit.txt", []byte(status+"\n"))
	if status != "1:1" {
		t.Fatalf("incomplete browser exit = %q, want 1:1", status)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("encoding browser mutated fixture")
	}
}
