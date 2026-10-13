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
	"strconv"
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
	raw := canonicalPath(t, isolationProbeBinary)
	wrapper, err := fixtureLauncher(raw, fx.temporary)
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(fx.base, "allowed", "fixture.sb")
	policy := fixtureProfile(canonicalPath(t, fx.base), raw)
	writeFile(t, profile, []byte(policy), 0600)
	writeEvidence(t, "fixture-launch/fixture.sb", []byte(policy))
	launcher, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "fixture-launch/launcher.sh", launcher)
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
	skillglassBinary = wrapper
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
	command := exec.Command(wrapper, args...)
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
	line := shellQuote(wrapper)
	for _, arg := range args {
		line += " " + shellQuote(arg)
	}
	line += "; /bin/sleep 30"
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(160), "-y", "24", "-s", "browser", line)
	var pane string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pane = capturePane(t, tmux, fx, socket, true)
		if strings.Contains(pane, `"passed":true`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	verify([]byte(pane))
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
