package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// Fixture probes use the same sealed ordinary policy as product launches.
func (fx fixture) fixtureCommand(t *testing.T, ctx context.Context, executable string, args ...string) *exec.Cmd {
	t.Helper()
	argv := fx.invocation(t, executable, args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir, command.Env = fx.project, fx.environment()
	return command
}

func TestOrdinaryLaunchSystemFixtureIsolation(t *testing.T) {
	for _, mode := range []string{"cli", "pty", "tmux"} {
		t.Run(mode, func(t *testing.T) {
			fx := newFixture(t)
			path := filepath.Join(fx.base, "excluded", "system-inputs", "config.toml")
			writeFile(t, path, []byte("OWNED_SYSTEM_CANARY"), 0600)
			if raw, err := os.ReadFile(path); err != nil || string(raw) != "OWNED_SYSTEM_CANARY" {
				t.Fatal("positive input control", err)
			}
			// The original ordinary launcher reads this owned stand-in for an absolute machine input.
			uncontained := exec.Command(isolationProbeBinary, "--system", path)
			uncontained.Env = fx.environment()
			uncontained.Dir = fx.project
			positive, _ := uncontained.CombinedOutput()
			if !bytes.Contains(positive, []byte(`"read_absent":false`)) {
				t.Fatalf("uncontained control: %s", positive)
			}
			writeEvidence(t, "ordinary-isolation/"+mode+"-before.json", positive)
			before := treeManifest(t, fx.base)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var raw []byte
			switch mode {
			case "cli":
				command := fx.fixtureCommand(t, ctx, isolationProbeBinary, "--system", path)
				command.Env = fx.environment()
				command.Dir = fx.project
				launch := recordLaunch(t, command)
				started := time.Now()
				var err error
				raw, err = command.CombinedOutput()
				recordEvidenceCommand(t, "ordinary-system-cli", commandResult{code: launchExitCode(err), stdout: raw, duration: time.Since(started), launch: launch})
				assertRecordedContext(t, launch, command)
				if err != nil {
					t.Fatalf("CLI: %v %s", err, raw)
				}
			case "pty":
				command := fx.fixtureCommand(t, ctx, isolationProbeBinary, "--system", path)
				command.Env = fx.environment()
				command.Dir = fx.project
				launch := recordLaunch(t, command)
				started := time.Now()
				terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 80})
				if err != nil {
					t.Fatal(err)
				}
				raw, _ = io.ReadAll(terminal)
				err = command.Wait()
				terminal.Close()
				recordEvidenceCommand(t, "ordinary-system-pty", commandResult{code: launchExitCode(err), stdout: raw, duration: time.Since(started), launch: launch})
				assertRecordedContext(t, launch, command)
				if err != nil {
					t.Fatalf("PTY: %v %s", err, raw)
				}
			case "tmux":
				tmux, err := exec.LookPath("tmux")
				if err != nil {
					t.Fatal(err)
				}
				socket := "ale31-isolation-" + strconv.Itoa(os.Getpid())
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				runTmuxBootstrap(t, tmux, fx, socket, "browser", 80, 24)
				runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "browser:0", "remain-on-exit", "on")
				command := fx.fixtureCommand(t, ctx, isolationProbeBinary, "--system", path)
				launch := runTmuxProduct(t, tmux, fx, socket, command, "respawn-pane", "-k", "-t", "browser:0.0")
				assertRecordedContext(t, launch, command)
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					raw = []byte(runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-J", "-S", "-", "-t", "browser:0.0"))
					if bytes.Contains(raw, []byte(`"passed":true`)) {
						break
					}
					time.Sleep(25 * time.Millisecond)
				}
			}
			var result struct{ Passed bool }
			line := strings.Split(strings.TrimSpace(string(raw)), "\n")[0]
			if err := json.Unmarshal([]byte(line), &result); err != nil || !result.Passed {
				t.Fatalf("%s isolation: %v %s", mode, err, raw)
			}
			writeEvidence(t, "ordinary-isolation/"+mode+"-after.json", raw)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestOrdinaryLaunchReadWriteNetworkChildControls(t *testing.T) {
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "positive.txt")
	excluded := filepath.Join(fx.base, "excluded", "home-canary.txt")
	blocked := filepath.Join(fx.base, "allowed", "forbidden.txt")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, excluded, []byte("LIVE_HOME_READ_CANARY\n"), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	positive := runPositiveControls(t, allowed, excluded, blocked, listener)
	for key, passed := range positive {
		if !passed {
			t.Fatalf("positive %s failed", key)
		}
	}
	before := treeManifest(t, fx.base)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := fx.fixtureCommand(t, ctx, isolationProbeBinary, allowed, excluded, blocked, listener.Addr().String())
	command.Env = fx.environment()
	command.Dir = fx.project
	launch := recordLaunch(t, command)
	started := time.Now()
	raw, err := command.CombinedOutput()
	recordEvidenceCommand(t, "ordinary-full-controls", commandResult{code: launchExitCode(err), stdout: raw, duration: time.Since(started), launch: launch})
	assertRecordedContext(t, launch, command)
	if err != nil {
		t.Fatalf("ordinary contained probe: %v %s", err, raw)
	}
	var result struct{ Passed bool }
	if err = json.Unmarshal(raw, &result); err != nil || !result.Passed {
		t.Fatalf("controls: %v %s", err, raw)
	}
	writeEvidence(t, "ordinary-isolation/full-controls.json", raw)
	assertNoSandboxedConnection(t, listener)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
