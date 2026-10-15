package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestFixtureLaunchContainment(t *testing.T) {
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "positive")
	excluded := filepath.Join(fx.base, "excluded", "modeled-system", "config")
	blocked := filepath.Join(fx.base, "allowed", "forbidden-write")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, excluded, []byte("MODELED_SYSTEM_CANARY\n"), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	positive := runPositiveControls(t, allowed, excluded, blocked, listener)
	if !positive["write_succeeds_without_sandbox"] || !positive["loopback_succeeds_without_sandbox"] {
		t.Fatal(positive)
	}
	profile := fx.policy
	policy := mustReadFixturePolicy(t, fx)
	writeEvidence(t, "fixture-launch/fixture.sb", policy)
	args := []string{"--absence", allowed, excluded, blocked, listener.Addr().String()}
	verify := func(data []byte) {
		t.Helper()
		var result struct {
			Passed bool
			Checks map[string]bool
		}
		if err := json.Unmarshal(bytes.TrimSpace(data), &result); err != nil {
			t.Fatalf("%v: %s", err, data)
		}
		if !result.Passed {
			t.Fatalf("fixture containment checks: %s", data)
		}
	}
	original := skillglassBinary
	skillglassBinary = isolationProbeBinary
	defer func() { skillglassBinary = original }()
	result := fx.run(t, args...)
	assertExit(t, result, 0)
	verify(result.stdout)
	writeEvidence(t, "fixture-launch/cli.json", result.stdout)
	alias := filepath.Join(fx.base, "allowed", "excluded-alias")
	if err := os.Symlink(excluded, alias); err != nil {
		t.Fatal(err)
	}
	aliasArgs := append([]string(nil), args...)
	aliasArgs[2] = alias
	aliasResult := fx.run(t, aliasArgs...)
	assertExit(t, aliasResult, 0)
	verify(aliasResult.stdout)
	writeEvidence(t, "fixture-launch/symlink.json", aliasResult.stdout)
	command := fx.productCommand(t, args...)
	command.Dir = fx.project
	command.Env = fx.environment()
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(terminal)
	_ = terminal.Close()
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	verify(data)
	writeEvidence(t, "fixture-launch/direct-pty.json", data)
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux containment control requires tmux")
	}
	socket := fmt.Sprintf("fixture-control-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runTmuxBootstrap(t, tmux, fx, socket, "browser", 160, 80)
	runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "browser:0", "remain-on-exit", "on")
	runTmuxProduct(t, tmux, fx, socket, fx.productCommand(t, args...), "respawn-pane", "-k", "-t", "browser:0.0")
	var pane string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pane = runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-J", "-S", "-", "-t", "browser:0.0")
		if strings.Contains(pane, `"passed":true`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	start, end := strings.Index(pane, "{\"passed\":"), strings.LastIndex(pane, "}")
	if start < 0 || end < start {
		t.Fatalf("probe output missing: %s", pane)
	}
	verify([]byte(pane[start : end+1]))
	writeEvidence(t, "fixture-launch/tmux.json", []byte(pane))
	assertNoSandboxedConnection(t, listener)
	if _, err := os.Stat(blocked); !os.IsNotExist(err) {
		t.Fatal("sandbox wrote fixture")
	}
	// A malformed profile must fail before the executable can read the allowed sentinel.
	writeFile(t, profile, []byte("(invalid policy)"), 0600)
	failed := fx.run(t, args...)
	if failed.code == 0 || len(failed.stdout) > 0 {
		t.Fatalf("policy failure did not stop child: %#v", failed)
	}
	writeEvidence(t, "fixture-launch/fail-closed.stderr", failed.stderr)
}
