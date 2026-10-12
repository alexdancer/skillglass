package acceptance_test

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type paneSnapshot struct {
	cells  string
	joined string
	width  int
	height int
}

func snapshotPane(t *testing.T, tmux string, fx fixture, socket, target string) paneSnapshot {
	t.Helper()
	var frame paneSnapshot
	dimensions := runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", target, "#{pane_width}x#{pane_height}")
	if _, err := fmt.Sscanf(strings.TrimSpace(dimensions), "%dx%d", &frame.width, &frame.height); err != nil {
		t.Fatal(err)
	}
	frame.cells = capturePaneAt(t, tmux, fx, socket, target, false)
	frame.joined = capturePaneAt(t, tmux, fx, socket, target, true)
	return frame
}

func paneHas(frame paneSnapshot, joined bool, needles ...string) bool {
	capture := frame.cells
	if joined {
		capture = frame.joined
	}
	for _, needle := range needles {
		if !paneContains(capture, needle) {
			return false
		}
	}
	return true
}

func actOnPane(t *testing.T, tmux string, fx fixture, socket, target string, action func(), ready func(paneSnapshot) bool) paneSnapshot {
	t.Helper()
	before := snapshotPane(t, tmux, fx, socket, target)
	if ready(before) {
		t.Fatalf("action postcondition already holds before action (%dx%d):\n%s", before.width, before.height, before.cells)
	}
	action()
	deadline := time.Now().Add(5 * time.Second)
	var after paneSnapshot
	for time.Now().Before(deadline) {
		after = snapshotPane(t, tmux, fx, socket, target)
		rows := strings.Split(strings.TrimSuffix(after.cells, "\n"), "\n")
		currentGrid := len(rows) == after.height && strings.TrimSpace(rows[len(rows)-1]) != ""
		if after.cells != before.cells && ready(after) && (currentGrid || strings.Contains(after.cells, "Pane is dead")) {
			return after
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("action postcondition timed out (%dx%d):\n%s", after.width, after.height, after.cells)
	return paneSnapshot{}
}

func actForPaneAt(t *testing.T, tmux string, fx fixture, socket, target string, joined bool, action func(), needles ...string) string {
	t.Helper()
	frame := actOnPane(t, tmux, fx, socket, target, action, func(frame paneSnapshot) bool {
		return paneHas(frame, joined, needles...)
	})
	if joined {
		return frame.joined
	}
	return frame.cells
}

func resizePaneAt(t *testing.T, tmux string, fx fixture, socket, target string, width, height int, needles ...string) string {
	t.Helper()
	frame := actOnPane(t, tmux, fx, socket, target, func() {
		runTmux(t, tmux, fx, socket, "resize-window", "-t", target, "-x", fmt.Sprint(width), "-y", fmt.Sprint(height))
	}, func(frame paneSnapshot) bool {
		return frame.width == width && frame.height == height && paneHas(frame, false, needles...)
	})
	assertPaneCells(t, frame.cells, width, height)
	return frame.cells
}

type terminalTranscript struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (transcript *terminalTranscript) Write(value []byte) (int, error) {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	return transcript.data.Write(value)
}

func (transcript *terminalTranscript) Bytes() []byte {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	return bytes.Clone(transcript.data.Bytes())
}

func (transcript *terminalTranscript) wait(t *testing.T, checkpoint int, needles ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output := string(transcript.Bytes()[checkpoint:])
		found := true
		for _, needle := range needles {
			if !strings.Contains(output, needle) {
				found = false
				break
			}
		}
		if found && output != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("new PTY output did not show %q after checkpoint %d:\n%q", needles, checkpoint, transcript.Bytes()[checkpoint:])
}
