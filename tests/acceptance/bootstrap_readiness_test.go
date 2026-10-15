package acceptance_test

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestContainedBootstrapImmediateShutdown(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 100; iteration++ {
		t.Run(fmt.Sprint(iteration), func(t *testing.T) {
			fx := newFixture(t)
			socket := "bootstrap-readiness"
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			runTmuxBootstrap(t, tmux, fx, socket, "control", 80, 24)
			runTmux(t, tmux, fx, socket, "kill-session", "-t", "control")
		})
	}
}
