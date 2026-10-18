package acceptance_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Ordinary CLI, direct PTY and tmux share this process boundary. Codex has
// absolute system inputs in addition to HOME; controlled absence keeps fixtures
// deterministic without changing production discovery or inspecting live inputs.
func (fx fixture) recordedCommand(ctx context.Context, executable string, args ...string) *fixtureCommand {
	if runtime.GOOS != "darwin" {
		return fx.wrap(exec.CommandContext(ctx, executable, args...))
	}
	account, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("fixture home boundary unavailable: %v", err))
	}
	profile := fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(allow network* (local unix-socket (subpath %s)) (local unix-socket (subpath %s)))
(deny file-write*)
(allow file-write* (subpath %s) (subpath %s) (subpath %s) (subpath "/dev"))
(deny file-read* (subpath %s) (subpath %s))
(allow file-read* (literal %s) (literal %s) (literal %s))
(deny file-read* (with errno ENOENT) (subpath "/etc/codex") (subpath "/private/etc/codex") (subpath %s))
`, sandboxString(fx.tmuxTemporary), sandboxString(tmuxSocketDirectory), sandboxString(fx.base), sandboxString(fx.tmuxTemporary), sandboxString(tmuxSocketDirectory), sandboxString(account.HomeDir), sandboxString(filepath.Join(fx.base, "excluded", "live-home")), sandboxString(skillglassBinary), sandboxString(isolationProbeBinary), sandboxString(fixtureLaunchBinary), sandboxString(filepath.Join(fx.base, "excluded", "system-codex")))
	argv := append([]string{"-p", profile, executable}, args...)
	command := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", argv...)
	command.Dir, command.Env = fx.project, fx.environment()
	return fx.wrap(command)
}

func TestOrdinaryFixtureBoundary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("ordinary macOS fixture boundary is Darwin-specific")
	}
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "read-canary")
	excluded := filepath.Join(fx.base, "excluded", "system-codex", "config.toml")
	writeFile(t, allowed, []byte("OWNED_ALLOWED_CANARY\n"), 0600)
	writeFile(t, excluded, []byte("OWNED_SYSTEM_CANARY\n"), 0600)
	raw, err := os.ReadFile(excluded)
	if err != nil || !strings.Contains(string(raw), "OWNED_SYSTEM_CANARY") {
		t.Fatal("negative control is unreadable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := fx.recordedCommand(ctx, "/bin/cat", allowed)
	command.Env = fx.environment()
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "OWNED_ALLOWED_CANARY") {
		t.Fatalf("positive read: %v %s", err, output)
	}
	command = fx.recordedCommand(ctx, "/bin/cat", excluded)
	command.Env = fx.environment()
	output, err = command.CombinedOutput()
	writeEvidence(t, "fixture-boundary/cli.txt", output)
	if err == nil || !strings.Contains(string(output), "No such file or directory") {
		t.Errorf("CLI must see controlled absence: %v %s", err, output)
	}
	command = fx.recordedCommand(ctx, "/bin/cat", allowed, excluded)
	command.Env = fx.environment()
	terminal, err := command.startPTY(nil)
	if err != nil {
		t.Fatal(err)
	}
	output, _ = io.ReadAll(terminal)
	err = command.Wait()
	_ = terminal.Close()
	if receiptErr := command.completePTY(err, output); receiptErr != nil {
		t.Fatal(receiptErr)
	}
	writeEvidence(t, "fixture-boundary/direct-pty.txt", output)
	if err == nil || !strings.Contains(string(output), "OWNED_ALLOWED_CANARY") || !strings.Contains(string(output), "No such file or directory") {
		t.Errorf("direct PTY must see controlled absence: %v %s", err, output)
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	socket := "fixture-boundary-" + filepath.Base(fx.base)
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runContainedTmux(t, tmux, fx, socket, &containedTmuxPayload{executable: "/bin/bash", command: "exec /bin/bash -c " + shellQuote(tmuxBootstrapCommand)}, "new-session", "-d", "-x", "80", "-y", "24", "-s", "boundary")
	waitForPaneTitle(t, tmux, fx, socket, "boundary:0.0", tmuxBootstrapTitle)
	runContainedTmux(t, tmux, fx, socket, nil, "set-option", "-g", "remain-on-exit", "on")
	runContainedTmux(t, tmux, fx, socket, &containedTmuxPayload{executable: "/bin/sh", command: "exec /bin/sh -c " + shellQuote("/bin/cat "+shellQuote(allowed)+" "+shellQuote(excluded)+"; printf 'BOUNDARY_FINISHED\\n'; /bin/sleep 1")}, "new-window", "-t", "boundary", "-n", "probe")
	_ = waitForPaneAt(t, tmux, fx, socket, "boundary:probe.0", false, "BOUNDARY_FINISHED")
	frame := runContainedTmux(t, tmux, fx, socket, nil, "capture-pane", "-p", "-S", "-100", "-J", "-t", "boundary:probe.0")
	writeEvidence(t, "fixture-boundary/tmux.txt", []byte(frame))
	if !strings.Contains(frame, "OWNED_ALLOWED_CANARY") || !strings.Contains(frame, "No such file or directory") || strings.Contains(frame, "OWNED_SYSTEM_CANARY") {
		t.Errorf("tmux child must inherit controlled absence: %s", frame)
	}
}
