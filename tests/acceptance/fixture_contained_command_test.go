package acceptance_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func (fx fixture) containedSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	fx.policy = filepath.Join(fx.tmuxTemporary, "bootstrap.sb")
	sleep := canonicalPath(t, "/bin/sleep")
	writeFile(t, fx.policy, []byte(runtimeProfile(sleep, filepath.Join(fx.base, "allowed"))), 0600)
	return fx.containedCommand(t, context.Background(), sleep, "60")
}

func (fx fixture) containedCommand(t *testing.T, ctx context.Context, executable string, args ...string) *exec.Cmd {
	t.Helper()
	argv := fx.invocation(t, executable, args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir, command.Env = fx.project, fx.environment()
	return command
}
