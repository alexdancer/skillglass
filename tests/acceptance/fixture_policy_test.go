package acceptance_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func absentSystemPolicy() string {
	var policy strings.Builder
	for _, path := range []string{"/Library/Application Support/ClaudeCode", "/Library/Managed Preferences", "/Library/Preferences/com.anthropic.claudecode.plist", "/etc/codex"} {
		fmt.Fprintf(&policy, "(deny file-read* (subpath %s) (with errno ENOENT))\n", sandboxString(path))
	}
	return policy.String()
}

func ancestorReadLiterals(root string) string {
	var filters strings.Builder
	for parent := filepath.Dir(root); parent != "/"; parent = filepath.Dir(parent) {
		fmt.Fprintf(&filters, " (literal %s)\n", sandboxString(parent))
	}
	return filters.String()
}

func (fx fixture) seal(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Fatal("fixture containment requires Darwin sandbox-exec")
	}
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Fatal(err)
	}
	allowed := canonicalPath(t, filepath.Join(fx.base, "allowed"))
	product, probe := canonicalPath(t, skillglassBinary), canonicalPath(t, isolationProbeBinary)
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-write*)
(deny file-read-data (require-not (require-any
 (literal %s) (literal %s) (literal "/") %s
 (subpath %s) (subpath %s)
 (subpath "/Library/Application Support/ClaudeCode") (subpath "/Library/Managed Preferences")
 (literal "/Library/Preferences/com.anthropic.claudecode.plist") (subpath "/etc/codex")
 (subpath "/System") (subpath "/usr/lib") (subpath "/dev"))))
(deny file-read* (subpath %s))
(allow file-read* (literal %s) (literal %s))
(deny process-exec (require-not (require-any (literal %s) (literal %s))))
`, sandboxString(product), sandboxString(probe), ancestorReadLiterals(allowed), sandboxString(allowed), sandboxString(filepath.Join(fx.base, "excluded", "system-inputs")), sandboxString(account.HomeDir), sandboxString(product), sandboxString(probe), sandboxString(product), sandboxString(probe))
	policy += absentSystemPolicy()
	policy += fmt.Sprintf("(deny file-read* (subpath %s) (with errno ENOENT))\n", sandboxString(filepath.Join(fx.base, "excluded", "system-inputs")))
	writeFile(t, fx.policy, []byte(policy), 0600)
}

func (fx fixture) invocation(t *testing.T, executable string, args ...string) []string {
	t.Helper()
	return append([]string{"/usr/bin/sandbox-exec", "-f", fx.policy, canonicalPath(t, executable)}, args...)
}

func (fx fixture) command(t *testing.T, ctx context.Context, args ...string) *exec.Cmd {
	t.Helper()
	argv := fx.invocation(t, skillglassBinary, args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir, command.Env = fx.project, fx.environment()
	return command
}

func (fx fixture) productCommand(t *testing.T, args ...string) *exec.Cmd {
	return fx.command(t, context.Background(), args...)
}

func (fx fixture) tmuxSocket(socket string) string {
	key := sha256.Sum256([]byte(fx.base + "\x00" + socket))
	return filepath.Join(tmuxSocketDirectory, hex.EncodeToString(key[:12]))
}

func TestALE29OrdinaryPolicyControlsAndTmuxInheritance(t *testing.T) {
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "policy-positive")
	excluded := filepath.Join(fx.base, "excluded", "outside-settings")
	system := filepath.Join(fx.base, "excluded", "system-inputs", "managed-settings.json")
	outsideWrite := filepath.Join(fx.base, "excluded", "escape")
	link := filepath.Join(fx.base, "allowed", "escape-link")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, excluded, []byte("inert outside settings"), 0600)
	writeFile(t, system, []byte("inert system settings"), 0600)
	if err := os.Symlink(filepath.Dir(outsideWrite), link); err != nil {
		t.Fatal(err)
	}
	// Controller-owned positive controls establish that denial is policy-caused.
	for _, path := range []string{allowed, excluded, system} {
		if _, err := os.ReadFile(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(outsideWrite, []byte("positive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(outsideWrite); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	original := skillglassBinary
	skillglassBinary = isolationProbeBinary
	defer func() { skillglassBinary = original }()
	args := []string{"fixture-policy", allowed, excluded, system, outsideWrite, filepath.Join(link, "symlink-escape"), listener.Addr().String()}
	before := treeManifest(t, fx.base)
	result := fx.run(t, args...)
	assertExit(t, result, 0)
	var controls struct {
		Passed bool            `json:"passed"`
		Checks map[string]bool `json:"checks"`
	}
	decodeJSON(t, result.stdout, &controls)
	if !controls.Passed || len(controls.Checks) != 16 {
		t.Fatalf("ordinary controls: %s", result.stdout)
	}
	writeEvidence(t, "fixture-policy/ordinary-controls.json", result.stdout)
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	socket := "fixture-policy-control"
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser", "exec /bin/sleep 30")
	runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "browser:0", "remain-on-exit", "on")
	runTmuxProduct(t, tmux, fx, socket, fx.productCommand(t, args...), "respawn-pane", "-k", "-t", "browser:0.0")
	frame := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame = runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-J", "-S", "-", "-t", "browser:0.0")
		if strings.Contains(frame, "\"passed\":true") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	writeEvidence(t, "fixture-policy/tmux-controls-frame.txt", []byte(frame))
	frame = strings.Split(strings.TrimSpace(frame), "\n")[0]
	decodeJSON(t, []byte(frame), &controls)
	if !controls.Passed || len(controls.Checks) != 16 {
		t.Fatalf("tmux payload controls: %s", frame)
	}
	writeEvidence(t, "fixture-policy/tmux-controls.json", []byte(strings.TrimSpace(frame)+"\n"))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE29OrdinaryPolicyFailsClosed(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, fx.policy, []byte("invalid sandbox policy"), 0600)
	result := fx.run(t, "list", "--root", fx.cleanRoot, "--json")
	if result.code == 0 || len(result.stdout) > 0 {
		t.Fatalf("invalid policy reached product: %+v", result)
	}
}

func TestALE29KnownClaudeAndCodexSystemInputsAreControlledAbsent(t *testing.T) {
	for _, tc := range []struct{ agent, settings, skills string }{
		{"claude-code", "/Library/Application Support/ClaudeCode/managed-settings.json", "/Library/Application Support/ClaudeCode/.claude/skills"},
		{"codex", "/etc/codex/config.toml", "/etc/codex/skills"},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			fx, _ := codexFixture(t)
			result := fx.run(t, "list", "--agent", tc.agent, "--project", fx.project, "--json")
			assertExit(t, result, 0)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			layerFound := false
			for _, layer := range list.Scope.Agent.ConfigLayers {
				if layer.Path == tc.settings {
					layerFound = layer.State == "absent"
				}
			}
			var roots struct {
				Scope struct {
					Roots []struct {
						Requested string `json:"requested_path"`
						State     string
					}
				}
			}
			decodeJSON(t, result.stdout, &roots)
			rootFound := false
			for _, root := range roots.Scope.Roots {
				if root.Requested == tc.skills {
					rootFound = root.State == "missing"
				}
			}
			if !layerFound || !rootFound {
				t.Fatalf("system inputs not controlled absent: %s", result.stdout)
			}
			model := filepath.Join(fx.base, "excluded", "system-inputs", tc.agent, filepath.Base(tc.settings))
			writeFile(t, model, []byte("inert system model"), 0600)
			if _, err := os.ReadFile(model); err != nil {
				t.Fatal(err)
			}
			allowed := filepath.Join(fx.base, "allowed", "positive")
			writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
			excluded := filepath.Join(fx.base, "excluded", "outside")
			writeFile(t, excluded, []byte("outside"), 0600)
			before := treeManifest(t, fx.base)
			original := skillglassBinary
			skillglassBinary = isolationProbeBinary
			defer func() { skillglassBinary = original }()
			probe := fx.run(t, "fixture-policy", allowed, excluded, model, filepath.Join(fx.temporary, "blocked-write"), filepath.Join(fx.temporary, "blocked-link-write"), "127.0.0.1:9")
			assertExit(t, probe, 0)
			writeEvidence(t, "fixture-policy/"+tc.agent+"-system-model.json", probe.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
