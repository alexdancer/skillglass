package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// Fixture launches treat machine-wide policy as absent. Production paths stay native.
func (fx fixture) launchProfile(executable string) string {
	return filepath.Join(fx.temporary, filepath.Base(executable)+".sb")
}
func (fx fixture) prepareLaunchProfiles(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Fatal("ordinary acceptance requires macOS OS containment")
	}
	if _, err := exec.LookPath("/usr/bin/sandbox-exec"); err != nil {
		t.Fatal(err)
	}
	for _, executable := range []string{skillglassBinary, isolationProbeBinary} {
		policy := runtimeProfile(canonicalPath(t, executable), canonicalPath(t, fx.base), filepath.Join(canonicalPath(t, fx.base), "excluded", "system-inputs"))
		// Each test owns the complete fixture, including comparison roots beside allowed/.
		// Keep modeled live-home input denied and modeled machine policy absent.
		policy += fmt.Sprintf("(deny file-read* (require-all (subpath %s) (require-not (subpath %s))))\n", sandboxString(filepath.Join(canonicalPath(t, fx.base), "excluded")), sandboxString(filepath.Join(canonicalPath(t, fx.base), "excluded", "system-inputs")))
		policy += fmt.Sprintf("(deny file-read* (require-all (subpath \"/Users\") (require-not (literal %s))))\n", sandboxString(canonicalPath(t, executable)))
		// Ancestor enumeration supports project boundary discovery without reading sibling data.
		for path := filepath.Dir(canonicalPath(t, fx.base)); path != "/"; path = filepath.Dir(path) {
			policy += fmt.Sprintf("(allow file-read-data (literal %s))\n", sandboxString(path))
		}
		writeFile(t, fx.launchProfile(executable), []byte(policy), 0600)
		raw, err := os.ReadFile(fx.launchProfile(executable))
		if err != nil {
			t.Fatal(err)
		}
		writeEvidence(t, fmt.Sprintf("launch-profiles/%x.sb", sha256.Sum256(raw)), raw)
	}
}
func (fx fixture) fixtureCommand(ctx context.Context, executable string, args ...string) *exec.Cmd {
	argv := append([]string{"-f", fx.launchProfile(executable), executable}, args...)
	return exec.CommandContext(ctx, "/usr/bin/sandbox-exec", argv...)
}
func (fx fixture) containTmuxLaunch(args []string) []string {
	if len(args) == 0 || args[0] != "new-session" {
		return args
	}
	args = append([]string(nil), args...)
	command := args[len(args)-1]
	for _, executable := range []string{skillglassBinary, isolationProbeBinary} {
		quoted := shellQuote(executable)
		prefix := "/usr/bin/sandbox-exec -f " + shellQuote(fx.launchProfile(executable)) + " " + quoted
		command = strings.ReplaceAll(command, quoted, prefix)
	}
	args[len(args)-1] = command
	return args
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
				command := fx.fixtureCommand(ctx, isolationProbeBinary, "--system", path)
				command.Env = fx.environment()
				command.Dir = fx.project
				var err error
				raw, err = command.CombinedOutput()
				if err != nil {
					t.Fatalf("CLI: %v %s", err, raw)
				}
			case "pty":
				command := fx.fixtureCommand(ctx, isolationProbeBinary, "--system", path)
				command.Env = fx.environment()
				command.Dir = fx.project
				terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 80})
				if err != nil {
					t.Fatal(err)
				}
				raw, _ = io.ReadAll(terminal)
				err = command.Wait()
				terminal.Close()
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
				command := shellQuote(isolationProbeBinary) + " --system " + shellQuote(path) + "; /bin/sleep 10"
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-s", "browser", command)
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					raw = []byte(capturePane(t, tmux, fx, socket, false))
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
	command := fx.fixtureCommand(ctx, isolationProbeBinary, allowed, excluded, blocked, listener.Addr().String())
	command.Env = fx.environment()
	command.Dir = fx.project
	raw, err := command.CombinedOutput()
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
