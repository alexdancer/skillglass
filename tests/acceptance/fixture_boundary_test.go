package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/creack/pty"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFixtureLaunchBoundary(t *testing.T) {
	fx := newFixture(t)
	excluded := filepath.Join(fx.base, "excluded", "system-model")
	codexSkill(t, excluded, "sentinel", "EXCLUDED_SYSTEM_SENTINEL")
	before := treeManifest(t, fx.base)
	positive := fx.run(t, "list", "--root", fx.cleanRoot, "--json")
	assertExit(t, positive, 0)
	negative := fx.run(t, "list", "--root", excluded, "--json")
	assertExit(t, negative, 1)
	var list publicList
	decodeJSON(t, negative.stdout, &list)
	if len(list.Sources) != 0 || len(list.Problems) != 1 || list.Problems[0].Code != "root-missing" {
		t.Fatalf("excluded fixture model was not absent: %s", negative.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeEvidence(t, "fixture-boundary/positive.json", positive.stdout)
	writeEvidence(t, "fixture-boundary/negative.json", negative.stdout)
}

func fixtureRuntimeProfile(executable, fixtureRoot string) string {
	var ancestors strings.Builder
	for path := filepath.Dir(fixtureRoot); ; path = filepath.Dir(path) {
		fmt.Fprintf(&ancestors, " (literal %s)\n", sandboxString(path))
		if path == "/" {
			break
		}
	}
	return fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-write*)
(deny process-exec (require-not (literal %s)))
(deny file-read-data (require-not (require-any
 (literal %s)
%s (subpath %s)
 (subpath "/System")
 (subpath "/usr/lib")
 (subpath "/dev"))))
(deny file-read* (with errno 2) (require-any
 (subpath "/etc/codex")
 (subpath "/private/etc/codex")
 (subpath %s)))
`, sandboxString(executable), sandboxString(executable), ancestors.String(), sandboxString(fixtureRoot), sandboxString(filepath.Join(fixtureRoot, "excluded")))
}

func TestFixtureBrowserLaunchBoundaries(t *testing.T) {
	fx := newFixture(t)
	excluded := filepath.Join(fx.base, "excluded", "system-model")
	codexSkill(t, excluded, "sentinel", "EXCLUDED_SYSTEM_SENTINEL")
	before := treeManifest(t, fx.base)
	args := []string{"browse", "--root", fx.cleanRoot, "--root", excluded, "--project", fx.project}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	command := fx.command(t, ctx, args...)
	launch := recordLaunch(t, command)
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	var raw terminalTranscript
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&raw, terminal); close(done) }()
	raw.wait(t, 0, "normal-skill", "Coverage: incomplete", "Enter read")
	_, _ = io.WriteString(terminal, "q")
	err = command.Wait()
	<-done
	_ = terminal.Close()
	if command.ProcessState.ExitCode() != 1 || ctx.Err() != nil || bytes.Contains(raw.Bytes(), []byte("EXCLUDED_SYSTEM_SENTINEL")) {
		t.Fatalf("direct PTY boundary: %v, %s", err, raw.Bytes())
	}
	writeEvidence(t, "fixture-boundary/direct.ansi", raw.Bytes())
	recordEvidenceCommand(t, "fixture-boundary-direct-pty", commandResult{code: 1, stdout: raw.Bytes(), launch: launch})
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("fixture-boundary-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runTmuxProduct(t, tmux, fx, socket, fx.productCommand(t, args...), "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser")
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	frame := waitForPane(t, tmux, fx, socket, false, "normal-skill", "Coverage: incomplete", "Enter read")
	if strings.Contains(frame, "EXCLUDED_SYSTEM_SENTINEL") {
		t.Fatalf("tmux boundary: %s", frame)
	}
	writeEvidence(t, "fixture-boundary/tmux.txt", []byte(frame))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestFixturePolicyInertControls(t *testing.T) {
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "positive-control", "allowed.txt")
	excluded := filepath.Join(fx.base, "excluded", "system-model", "config.toml")
	blockedWrite := filepath.Join(fx.base, "allowed", "positive-control", "blocked.txt")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, excluded, []byte("LIVE_HOME_READ_CANARY\n"), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	positive := runPositiveControls(t, allowed, excluded, blockedWrite, listener)
	for name, passed := range positive {
		if !passed {
			t.Fatalf("positive control %s failed", name)
		}
	}
	executable := canonicalPath(t, isolationProbeBinary)
	profile := filepath.Join(fx.temporary, "probe.sb")
	writeFile(t, profile, []byte(fixtureRuntimeProfile(executable, canonicalPath(t, fx.base))), 0600)
	before := treeManifest(t, fx.base)
	args := []string{allowed, excluded, blockedWrite, listener.Addr().String(), filepath.Dir(excluded)}
	command := exec.Command("/usr/bin/sandbox-exec", append([]string{"-f", profile, executable}, args...)...)
	command.Dir = fx.project
	command.Env = fx.environment()
	launch := recordLaunch(t, command)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture inert controls: %v, %s", err, output)
	}
	var controls struct {
		Passed bool
		Checks map[string]bool
	}
	decodeJSON(t, output, &controls)
	if !controls.Passed {
		t.Fatalf("fixture controls: %s", output)
	}
	assertNoSandboxedConnection(t, listener)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeEvidence(t, "fixture-boundary/inert-controls.json", output)
	recordEvidenceCommand(t, "fixture-boundary-inert-probe", commandResult{stdout: output, launch: launch})
}
