package acceptance_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/tests/launchrecord"
)

func TestALE42RetainedContainedTmuxBootstrap(t *testing.T) {
	for _, route := range []string{"retained-helper", "explicit-shell-control"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			tmux, err := exec.LookPath("tmux")
			if err != nil {
				t.Fatal(err)
			}
			socket := "bootstrap-verification"
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			args := []string{"new-session", "-d", "-x", "80", "-y", "24", "-s", "boundary", "exec /bin/bash -c " + shellQuote(tmuxBootstrapCommand)}
			if route == "retained-helper" {
				runContainedTmux(t, tmux, fx, socket, &containedTmuxPayload{executable: "/bin/bash", command: args[len(args)-1]}, args[:len(args)-1]...)
			} else {
				argv := append([]string{"-S", fx.tmuxSocket(socket), "-f", "/dev/null"}, args...)
				command := fx.recordedCommand(context.Background(), tmux, argv...)
				command.payloadIdentity, err = launchrecord.Identify("/bin/bash")
				if err != nil {
					t.Fatal(err)
				}
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("explicit shell identity: %v %s", err, output)
				}
			}
			got := waitForPaneTitle(t, tmux, fx, socket, "boundary:0.0", "fixture-bootstrap-ready")
			if got != "fixture-bootstrap-ready" {
				t.Fatalf("bootstrap title = %q, want fixture-bootstrap-ready", got)
			}
			writeEvidence(t, "bootstrap-verification/"+route+"-title.txt", []byte(got+"\n"))
		})
	}
}

func TestALE42ContainedTmuxMissingIdentityRefuses(t *testing.T) {
	for _, operation := range []string{"new-session", "new-window", "respawn-pane"} {
		t.Run(operation, func(t *testing.T) {
			fx := newFixture(t)
			tmux, err := exec.LookPath("tmux")
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"-S", fx.tmuxSocket("missing-identity"), "-f", "/dev/null", operation, "exec /usr/bin/printf 'must-not-run'"}
			command := fx.recordedCommand(context.Background(), tmux, args...)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(err.Error(), "tmux payload identity missing") || command.Process != nil {
				t.Fatalf("missing identity reached tmux: %v %s", err, output)
			}
			writeEvidence(t, "bootstrap-verification/missing-identity-"+operation+".txt", []byte(err.Error()+"\n"))
		})
	}
}
