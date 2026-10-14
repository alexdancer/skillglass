package acceptance_test

import (
	"context"
	"os/exec"
	"testing"
)

func (fx fixture) containedCommand(t *testing.T, ctx context.Context, executable string, args ...string) *exec.Cmd {
	t.Helper()
	argv := fx.invocation(t, executable, args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir, command.Env = fx.project, fx.environment()
	return command
}
