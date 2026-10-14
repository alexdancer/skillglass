package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestALE35RouteIdentityTerminal(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		for _, disabled := range []bool{false, true} {
			label := "enabled"
			if disabled {
				label = "disabled"
			}
			t.Run(fmt.Sprintf("%s-%dx%d", label, size.width, size.height), func(t *testing.T) {
				fx := ale35OverlapFixture(t)
				var env []string
				if disabled {
					env = []string{"OPENCODE_DISABLE_EXTERNAL_SKILLS=1"}
				}
				before := treeManifest(t, fx.base)
				ale35PreserveInputs(t, fx, "overlap", env, before)
				socket := fmt.Sprintf("ale35-routes-%d-%s-%d", os.Getpid(), label, size.width)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := fx.productCommand(t, "browse", "--agent", "opencode", "--project", fx.project, "--context", "global", "--source-root", filepath.Join(fx.home, ".agents", "skills"))
				command.Env = append(fx.environment(), env...)
				runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser")
				runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "browser:0", "remain-on-exit", "on")
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
				state := "ask"
				if disabled {
					state = "unavailable"
				}
				frame := waitForPane(t, tmux, fx, socket, false, "context global", "agents-route", "Focus: 1/1", state+" (inferred)")
				prefix := "tmux/route-identity/" + t.Name() + "/"
				assertPaneCells(t, frame, size.width, size.height)
				writeEvidence(t, prefix+"sources.txt", []byte(frame))
				frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Configured permission: ask", "Session: unknown", "Read lines 1-")
				rows := map[int]string{}
				var joined string
				for page := 0; page < 40; page++ {
					first, last, total, body := sourceDetailBody(t, frame)
					for index, row := range body {
						rows[first+index] = row
					}
					assertPaneCells(t, frame, size.width, size.height)
					writeEvidence(t, fmt.Sprintf("%sdetail-%02d.txt", prefix, page), []byte(frame))
					if last == total {
						ordered := make([]string, total)
						for index := range ordered {
							ordered[index] = rows[index+1]
						}
						joined = strings.Join(ordered, "\n")
						break
					}
					frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool { start, _, _ := sourceDetailRange(next.cells); return start > first }).cells
				}
				writeEvidence(t, prefix+"detail-body.txt", []byte(joined))
				if joined == "" || strings.Count(joined, "Context: global") != 1 || strings.Count(joined, "Context: project") != 1 || strings.Count(joined, "Provenance: global") != 1 || strings.Count(joined, "Provenance: project") != 1 {
					t.Fatalf("terminal provenance lost or repeated: %s", joined)
				}
				if !paneContains(joined, "Permission: ask") || !paneContains(joined, "unsupported") {
					t.Fatalf("terminal permission/capability contract changed: %s", joined)
				}
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
				var exit string
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					exit = strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "browser:0.0", "#{pane_dead}:#{pane_dead_status}"))
					if exit == "1:0" {
						break
					}
					time.Sleep(25 * time.Millisecond)
				}
				if exit != "1:0" {
					t.Fatalf("browser did not quit normally: %s", exit)
				}
				writeEvidence(t, prefix+"exit.txt", []byte(exit))
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}
