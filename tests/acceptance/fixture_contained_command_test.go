package acceptance_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

const tmuxBootstrapTitle = "fixture-bootstrap-ready"
const tmuxBootstrapCommand = "printf '\\033]0;" + tmuxBootstrapTitle + "\\007'; read -r bootstrap"

func runTmuxBootstrap(t *testing.T, tmux string, fx fixture, socket, session string, width, height int) {
	t.Helper()
	fx.policy = filepath.Join(fx.tmuxTemporary, "bootstrap.sb")
	shell := canonicalPath(t, "/bin/bash")
	writeFile(t, fx.policy, []byte(runtimeProfile(shell, filepath.Join(fx.base, "allowed"))), 0600)
	command := fx.containedCommand(t, context.Background(), shell, "-c", tmuxBootstrapCommand)
	runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", strconv.Itoa(height), "-s", session)
	waitForPaneTitle(t, tmux, fx, socket, session+":0.0", tmuxBootstrapTitle)
}

func (fx fixture) containedCommand(t *testing.T, ctx context.Context, executable string, args ...string) *exec.Cmd {
	t.Helper()
	argv := fx.invocation(t, executable, args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir, command.Env = fx.project, fx.environment()
	return command
}
