package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func waitForTmuxExit(t *testing.T, tmux string, fx fixture, socket, target string, expected int, timeout time.Duration) (string, error) {
	t.Helper()
	return observePaneExit(func() string {
		return runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", target, "#{pane_dead}:#{pane_dead_status}:#{pane_dead_signal}")
	}, expected, timeout)
}

func observePaneExit(observe func() string, expected int, timeout time.Duration) (string, error) {
	want := fmt.Sprintf("1:%d:", expected)
	deadline := time.Now().Add(timeout)
	var status string
	for time.Now().Before(deadline) {
		status = strings.TrimSuffix(observe(), "\n")
		fields := strings.Split(status, ":")
		if len(fields) != 3 || (fields[0] != "0" && fields[0] != "1") {
			return status, fmt.Errorf("malformed pane exit observation %q", status)
		}
		if fields[1] != "" || fields[2] != "" {
			if status != want {
				return status, fmt.Errorf("pane exit observation %q, want %q (no signal)", status, want)
			}
			return status, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return status, fmt.Errorf("pane exit observation timed out at %q, want %q (no signal)", status, want)
}

func TestPaneExitObservationControls(t *testing.T) {
	for _, tc := range []struct {
		name     string
		readings []string
		expected int
		want     string
		wantErr  bool
	}{
		{"incomplete-then-exit-one", []string{"0::", "1::", "1:1:\n"}, 1, "1:1:", false},
		{"incomplete-then-exit-zero", []string{"1::", "1:0:"}, 0, "1:0:", false},
		{"wrong-zero-stops", []string{"1:0:", "1:1:"}, 1, "1:0:", true},
		{"wrong-255-stops", []string{"1:255:", "1:1:"}, 1, "1:255:", true},
		{"wrong-one-stops", []string{"1:1:", "1:0:"}, 0, "1:1:", true},
		{"signal-stops", []string{"1::15", "1:1:"}, 1, "1::15", true},
		{"expected-status-with-signal", []string{"1:1:15", "1:1:"}, 1, "1:1:15", true},
		{"zero-with-signal", []string{"1:0:9", "1:0:"}, 0, "1:0:9", true},
		{"missing-field", []string{"1:1", "1:1:"}, 1, "1:1", true},
		{"extra-field", []string{"1:1::", "1:1:"}, 1, "1:1::", true},
		{"invalid-dead", []string{"2:1:", "1:1:"}, 1, "2:1:", true},
		{"live-with-status", []string{"0:1:", "1:1:"}, 1, "0:1:", true},
		{"live-with-signal", []string{"0::15", "1:1:"}, 1, "0::15", true},
		{"invalid-status", []string{"1:bad:", "1:1:"}, 1, "1:bad:", true},
		{"whitespace", []string{" 1:1:", "1:1:"}, 1, " 1:1:", true},
		{"extra-newline", []string{"1:1:\n\n", "1:1:"}, 1, "1:1:\n", true},
		{"empty-observation", []string{"", "1:1:"}, 1, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got, err := observePaneExit(func() string {
				if calls >= len(tc.readings) {
					t.Fatal("observer read past completed result")
				}
				value := tc.readings[calls]
				calls++
				return value
			}, tc.expected, time.Second)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("observation = %q, %v; want %q, error=%t", got, err, tc.want, tc.wantErr)
			}
			wantCalls := len(tc.readings)
			if tc.wantErr {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("read %d observations, want %d", calls, wantCalls)
			}
		})
	}
	for _, pending := range []string{"0::", "1::"} {
		t.Run("timeout-"+pending, func(t *testing.T) {
			calls := 0
			got, err := observePaneExit(func() string {
				calls++
				return pending
			}, 1, 60*time.Millisecond)
			if got != pending || err == nil || !strings.Contains(err.Error(), "timed out") || calls == 0 {
				t.Fatalf("pending observation = %q, %v after %d reads", got, err, calls)
			}
		})
	}
}

func TestPaneExitObservationRealSignal(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	fx := newFixture(t)
	socket := fmt.Sprintf("exit-signal-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runTmuxBootstrap(t, tmux, fx, socket, "control", 80, 24)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
	pid, err := strconv.Atoi(strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "control:0.0", "#{pane_pid}")))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid owned pane pid %d: %v", pid, err)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	status, err := waitForTmuxExit(t, tmux, fx, socket, "control:0.0", 1, 3*time.Second)
	writeEvidence(t, "tmux/exit-observation/signal.txt", []byte(status+"\n"))
	if err == nil || status != "1:255:" {
		t.Fatalf("signaled payload was not rejected: %q, %v", status, err)
	}
}
