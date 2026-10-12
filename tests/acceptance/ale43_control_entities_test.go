package acceptance_test

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// Exercise the Markdown decoder through the public browser, not the renderer
// helper: both current cells and the emitted terminal stream must stay inert.
func TestBrowserEncodedControlsStayInertInRenderedAndRawViews(t *testing.T) {
	for _, theme := range []string{"dark", "light", "none", "no-color"} {
		t.Run(theme, func(t *testing.T) {
			fx := newFixture(t)
			root := filepath.Join(fx.base, "allowed", "roots", "control-entities")
			instructions := "# Control boundary\n\n" +
				"Decimal &#27;]52;c;ENTITY_CLIP&#7;\n" +
				"Hex &#x1b;]0;ENTITY_TITLE&#x7;\n" +
				"Carriage &#13;AFTER_ENTITY_CR\n" +
				"C1 &#155;2J &#157;52;c;C1_CLIP&#7;\n" +
				"Literal \x1b]52;c;LITERAL_CLIP\a\n" +
				"Title \x1b]0;LITERAL_TITLE\a\n" +
				"LiteralCR \rAFTER_LITERAL_CR\n" +
				"LiteralC1 \u009b2J \u009d52;c;LITERAL_C1\a\n" +
				"CONTROL_BOUNDARY_END\n"
			writeSkill(t, filepath.Join(root, "probe"), "control-probe", "literal and encoded terminal controls", instructions)
			before := treeManifest(t, fx.base)
			selectedTheme := theme
			if theme == "no-color" {
				selectedTheme = "dark"
			}
			tmux, socket, target := startALE43ReviewBrowser(t, fx, 80, 30, "--root", root, "--theme", selectedTheme)
			if theme == "no-color" {
				commandLine := "NO_COLOR=1 exec " + shellQuote(skillglassBinary) + " browse --root " + shellQuote(root) + " --theme dark"
				runTmux(t, tmux, fx, socket, "respawn-pane", "-k", "-t", target, commandLine)
				// The plain cells are identical across themes. Wait for the new
				// unstyled frame, not stale dark cells left by the old process.
				deadline := time.Now().Add(5 * time.Second)
				for {
					styled := captureStyledPane(t, tmux, fx, socket, target)
					if !strings.ContainsRune(styled, '\x1b') && strings.Contains(styled, "Focused: control-probe") {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("NO_COLOR browser did not render a fresh unstyled frame")
					}
					time.Sleep(25 * time.Millisecond)
				}
				_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Focused: control-probe", "? help")
			}
			rendered := actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
			}, "Rendered instructions", "CONTROL_BOUNDARY_END", `\x1b]52;c;ENTITY_CLIP\a`, `\x1b]0;ENTITY_TITLE\a`, `\rAFTER_ENTITY_CR`, `\x9b2J`, `\x9d52;c;C1_CLIP\a`)
			assertPaneCells(t, rendered, 80, 30)
			writeEvidence(t, "control-entities/"+theme+"/rendered.txt", []byte(rendered))
			writeEvidence(t, "control-entities/"+theme+"/rendered.ansi", []byte(captureStyledPane(t, tmux, fx, socket, target)))
			raw := actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "v")
			}, "Raw instructions", "CONTROL_BOUNDARY_END", "&#27;]52;c;ENTITY_CLIP&#7;", "&#x1b;]0;ENTITY_TITLE&#x7;", "&#13;AFTER_ENTITY_CR", "&#155;2J", `\x1b]52;c;LITERAL_CLIP\a`, `\rAFTER_LITERAL_CR`, `\x9d52;c;LITERAL_C1\a`)
			assertPaneCells(t, raw, 80, 30)
			writeEvidence(t, "control-entities/"+theme+"/raw.txt", []byte(raw))
			writeEvidence(t, "control-entities/"+theme+"/raw.ansi", []byte(captureStyledPane(t, tmux, fx, socket, target)))
			title := runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", target, "#{pane_title}")
			if strings.Contains(title, "ENTITY_TITLE") || strings.Contains(title, "LITERAL_TITLE") {
				t.Fatalf("hostile instructions changed terminal title: %q", title)
			}
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "q")

			// A fresh sized and drained PTY exposes the bytes the product sends,
			// including controls which a cell capture alone might have consumed.
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, skillglassBinary, "browse", "--root", root, "--theme", selectedTheme)
			command.Dir = fx.project
			command.Env = fx.environment()
			if theme == "no-color" {
				command.Env = append(command.Env, "NO_COLOR=1")
			}
			terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 80})
			if err != nil {
				t.Fatal(err)
			}
			defer terminal.Close()
			var transcript terminalTranscript
			done := make(chan struct{})
			go func() {
				_, _ = io.Copy(&transcript, terminal)
				close(done)
			}()
			transcript.wait(t, 0, "Focused: control-probe", "? help")
			checkpoint := len(transcript.Bytes())
			if _, err := io.WriteString(terminal, "\r"); err != nil {
				t.Fatal(err)
			}
			transcript.wait(t, checkpoint, "Rendered instructions", "CONTROL_BOUNDARY_END", `\x1b]52;c;ENTITY_CLIP\a`)
			checkpoint = len(transcript.Bytes())
			if _, err := io.WriteString(terminal, "v"); err != nil {
				t.Fatal(err)
			}
			transcript.wait(t, checkpoint, "&#27;]52;c;ENTITY_CLIP&#7;", "&#x1b;]0;ENTITY_TITLE&#x7;")
			if _, err := io.WriteString(terminal, "q"); err != nil {
				t.Fatal(err)
			}
			err = command.Wait()
			<-done
			if ctx.Err() != nil || err != nil {
				t.Fatalf("browser exit: %v, context: %v", err, ctx.Err())
			}
			output := transcript.Bytes()
			writeEvidence(t, "control-entities/"+theme+"/transcript.ansi", output)
			for _, forbidden := range []string{"\x1b]", "\a", "\u009b", "\u009d", "\rAFTER_ENTITY_CR", "\rAFTER_LITERAL_CR"} {
				if bytes.Contains(output, []byte(forbidden)) {
					t.Fatalf("product emitted active hostile control %q", forbidden)
				}
			}
			if (theme == "none" || theme == "no-color") && regexp.MustCompile(`\x1b\[[0-9;:]*m`).Match(output) {
				t.Fatal("no-color terminal stream contains SGR styling")
			}
			if !bytes.Equal(before, treeManifest(t, fx.base)) {
				t.Fatal("control probe mutated fixture content or settings")
			}
		})
	}
}
