package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestALE42RetainedProbeDispatch(t *testing.T) {
	const want = `{"directory_absent":true,"passed":true,"read_absent":true,"stat_absent":true}`
	for _, route := range []string{"ordinary", "tmux"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			model := filepath.Join(fx.base, "excluded", "system-inputs", "settings.json")
			writeFile(t, model, []byte("inert model"), 0600)
			if _, err := os.ReadFile(model); err != nil {
				t.Fatal(err)
			}
			tmux, socket := "", "probe-dispatch"
			if route == "tmux" {
				var err error
				tmux, err = exec.LookPath("tmux")
				if err != nil {
					t.Fatal(err)
				}
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				runTmuxBootstrap(t, tmux, fx, socket, "probe", 120, 30)
				runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "probe:0", "remain-on-exit", "on")
			}
			original := skillglassBinary
			skillglassBinary = isolationProbeBinary
			defer func() { skillglassBinary = original }()
			args := []string{"--system", model}
			if route == "ordinary" {
				result := fx.run(t, args...)
				assertExit(t, result, 0)
				if got := strings.TrimSpace(string(result.stdout)); got != want {
					t.Fatalf("probe output = %q, want %q", got, want)
				}
				return
			}
			runTmuxProduct(t, tmux, fx, socket, fx.productCommand(t, args...), "respawn-pane", "-k", "-t", "probe:0.0")
			deadline := time.Now().Add(5 * time.Second)
			frame := ""
			for time.Now().Before(deadline) {
				frame = runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-J", "-S", "-", "-t", "probe:0.0")
				if strings.Split(strings.TrimSpace(frame), "\n")[0] == want {
					return
				}
				time.Sleep(25 * time.Millisecond)
			}
			t.Fatalf("tmux probe output = %q, want %q", strings.TrimSpace(frame), want)
		})
	}
}
