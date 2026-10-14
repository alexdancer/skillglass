package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestRuntimeContentIsolation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec acceptance is Darwin-specific")
	}
	const sandboxExec = "/usr/bin/sandbox-exec"
	if _, err := os.Stat(sandboxExec); err != nil {
		t.Skipf("sandbox-exec is unavailable: %v", err)
	}

	fx := newFixture(t)
	allowedSentinel := filepath.Join(fx.base, "allowed", "positive-control", "allowed.txt")
	liveHomeSentinel := filepath.Join(fx.base, "excluded", "live-home", ".skillglass-canary")
	blockedWrite := filepath.Join(fx.base, "allowed", "positive-control", "blocked-write.txt")
	writeFile(t, allowedSentinel, []byte("ALLOWED_READ_SENTINEL\n"), 0o644)
	writeFile(t, liveHomeSentinel, []byte("LIVE_HOME_READ_CANARY\n"), 0o644)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	positive := runPositiveControls(t, allowedSentinel, liveHomeSentinel, blockedWrite, listener)

	canonicalAllowed := canonicalPath(t, filepath.Join(fx.base, "allowed"))
	canonicalProduct := canonicalPath(t, skillglassExecutable)
	canonicalProbe := canonicalPath(t, isolationProbeBinary)
	profiles := filepath.Join(fx.base, "allowed", "sandbox-profiles")
	productProfile := filepath.Join(profiles, "product.sb")
	probeProfile := filepath.Join(profiles, "probe.sb")
	writeFile(t, productProfile, []byte(runtimeProfile(canonicalProduct, canonicalAllowed)), 0o644)
	writeFile(t, probeProfile, []byte(runtimeProfile(canonicalProbe, canonicalAllowed)), 0o644)
	writeEvidence(t, "isolation/product.sb", []byte(runtimeProfile(canonicalProduct, canonicalAllowed)))
	writeEvidence(t, "isolation/probe.sb", []byte(runtimeProfile(canonicalProbe, canonicalAllowed)))
	positiveJSON, err := json.MarshalIndent(positive, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "isolation/positive-controls.json", append(positiveJSON, '\n'))

	before := treeManifest(t, fx.base)
	probe := runSandboxCommand(t, fx, probeProfile, canonicalProbe, allowedSentinel, liveHomeSentinel, blockedWrite, listener.Addr().String())
	assertExit(t, probe, 0)
	var controls struct {
		Passed bool            `json:"passed"`
		Checks map[string]bool `json:"checks"`
	}
	decodeJSON(t, probe.stdout, &controls)
	if !controls.Passed {
		t.Fatalf("sandbox controls failed: %s", probe.stdout)
	}
	for _, name := range []string{"allowed_read", "excluded_read_denied", "file_write_denied", "network_denied", "child_exec_denied"} {
		if !controls.Checks[name] {
			t.Fatalf("sandbox control %q did not pass", name)
		}
	}
	writeEvidence(t, "isolation/controls.json", probe.stdout)
	assertNoSandboxedConnection(t, listener)
	list := runSandboxCommand(
		t,
		fx,
		productProfile,
		canonicalProduct,
		"list",
		"--root",
		fx.cleanRoot,
		"--project",
		fx.project,
		"--json",
	)
	assertExit(t, list, 0)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	normal := sourceNamed(t, inventory.Sources, "normal-skill")
	read := runSandboxCommand(
		t,
		fx,
		productProfile,
		canonicalProduct,
		"read",
		"--root",
		fx.cleanRoot,
		"--project",
		fx.project,
		"--json",
		normal.ID,
	)
	assertExit(t, read, 0)
	for _, canary := range []string{"LIVE_HOME_READ_CANARY", "UNSELECTED_ROOT_CANARY", "HOME_FALLBACK_CANARY"} {
		if bytes.Contains(list.stdout, []byte(canary)) || bytes.Contains(read.stdout, []byte(canary)) {
			t.Fatalf("sandboxed product exposed excluded content %q", canary)
		}
	}
	writeEvidence(t, "isolation/list.json", list.stdout)
	writeEvidence(t, "isolation/read.json", read.stdout)

	browseRaw := runSandboxBrowser(t, fx, productProfile, canonicalProduct)
	if !bytes.Contains(browseRaw, []byte("normal-skill")) ||
		!bytes.Contains(browseRaw, []byte("coverage complete")) {
		t.Fatalf("sandboxed browser output is incomplete: %q", browseRaw)
	}
	writeEvidence(t, "isolation/browse.ansi", browseRaw)

	after := treeManifest(t, fx.base)
	writeEvidence(t, "isolation/tree-before.json", before)
	writeEvidence(t, "isolation/tree-after.json", after)
	if !bytes.Equal(before, after) {
		t.Fatal("sandboxed commands mutated their fixture")
	}
	if _, err := os.Stat(blockedWrite); !os.IsNotExist(err) {
		t.Fatalf("sandboxed helper wrote a file: %v", err)
	}
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatalf("sandboxed product executed fixture script: %v", err)
	}
}

func runPositiveControls(
	t *testing.T,
	allowedSentinel string,
	excludedSentinel string,
	writePath string,
	listener net.Listener,
) map[string]bool {
	t.Helper()
	allowed, err := os.ReadFile(allowedSentinel)
	if err != nil {
		t.Fatal(err)
	}
	excluded, err := os.ReadFile(excludedSentinel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(writePath, []byte("positive-control\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(writePath); err != nil {
		t.Fatal(err)
	}
	mutationPath := filepath.Join(filepath.Dir(writePath), "tree-negative-control.txt")
	writeFile(t, mutationPath, []byte("before\n"), 0o600)
	treeBefore := treeManifest(t, filepath.Dir(filepath.Dir(writePath)))
	writeFile(t, mutationPath, []byte("after\n"), 0o600)
	treeAfter := treeManifest(t, filepath.Dir(filepath.Dir(writePath)))
	if err := os.Remove(mutationPath); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(treeBefore, treeAfter) {
		t.Fatal("tree-manifest negative control missed a content mutation")
	}
	if err := exec.Command("/usr/bin/true").Run(); err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	setListenerDeadline(t, listener, time.Now().Add(time.Second))
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	_ = accepted.Close()
	setListenerDeadline(t, listener, time.Time{})
	return map[string]bool{
		"allowed_content_readable_without_sandbox":  string(allowed) == "ALLOWED_READ_SENTINEL\n",
		"excluded_content_readable_without_sandbox": string(excluded) == "LIVE_HOME_READ_CANARY\n",
		"write_succeeds_without_sandbox":            true,
		"loopback_succeeds_without_sandbox":         true,
		"child_exec_succeeds_without_sandbox":       true,
		"tree_manifest_detects_mutation":            true,
	}
}

func runSandboxCommand(
	t *testing.T,
	fx fixture,
	profile string,
	executable string,
	args ...string,
) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	commandArgs := append([]string{"-f", profile, executable}, args...)
	command := fx.wrap(exec.CommandContext(ctx, "/usr/bin/sandbox-exec", commandArgs...))
	command.Dir = canonicalPath(t, fx.project)
	command.Env = fx.environment()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	launch := recordLaunch(t, command.Cmd)
	err := command.Run()
	result := commandResult{
		code:     launchExitCode(err),
		launch:   launch,
		stdout:   stdout.Bytes(),
		stderr:   stderr.Bytes(),
		duration: time.Since(started),
	}
	recordEvidenceCommand(t, "sandboxed-command", result)
	if ctx.Err() != nil {
		t.Fatalf("sandboxed command timed out: %s %s", executable, strings.Join(args, " "))
	}
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run sandboxed command: %v", err)
		}
	}
	assertRecordedContext(t, launch, command.Cmd)
	return result
}

func runSandboxBrowser(t *testing.T, fx fixture, profile, executable string, browserArgs ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if len(browserArgs) == 0 {
		browserArgs = []string{"browse", "--root", fx.cleanRoot, "--project", fx.project}
	}
	if browserArgs[0] != "browse" {
		browserArgs = append([]string{"browse"}, browserArgs...)
	}
	commandArgs := append([]string{"-f", profile, executable}, browserArgs...)
	command := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", commandArgs...)
	command.Dir = canonicalPath(t, fx.project)
	command.Env = fx.environment()
	started := time.Now()
	launch := recordLaunch(t, command)
	recorded := fx.wrap(command)
	terminal, err := recorded.startPTY(&pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	var raw terminalTranscript
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&raw, terminal)
		close(copyDone)
	}()
	raw.wait(t, 0, "Focused: normal-skill", "? help")
	if _, err := io.WriteString(terminal, "q"); err != nil {
		t.Fatal(err)
	}
	err = command.Wait()
	<-copyDone
	_ = terminal.Close()
	if receiptErr := recorded.completePTY(err, raw.Bytes()); receiptErr != nil {
		t.Fatal(receiptErr)
	}
	recordEvidenceCommand(t, "sandboxed-browser-pty", commandResult{code: launchExitCode(err), stdout: raw.Bytes(), duration: time.Since(started), launch: launch})
	if ctx.Err() != nil {
		t.Fatal("sandboxed browser timed out")
	}
	if err != nil {
		t.Fatalf("sandboxed browser exit: %v\n%q", err, raw.Bytes())
	}
	assertRecordedContext(t, launch, command)
	return raw.Bytes()
}

func runtimeProfile(executable, allowedRoot string) string {
	controlledSystem := filepath.Join(filepath.Dir(allowedRoot), "excluded", "controlled-system")
	return fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-read* (subpath "/private/etc/codex") (with errno 2))
(deny file-read* (subpath "/etc/codex") (with errno 2))
(deny file-read* (subpath "/Library/Application Support/opencode") (with errno 2))
(deny file-read* (subpath "/Library/Managed Preferences") (with errno 2))
(deny file-write*)
(deny file-read-data (require-not (require-any
 (literal %s) (literal "/") %s
 (subpath %s) (subpath %s)
 (subpath "/Library/Application Support/ClaudeCode") (subpath "/Library/Managed Preferences")
 (literal "/Library/Preferences/com.anthropic.claudecode.plist")
 (subpath "/etc/codex") (subpath "/private/etc/codex") (subpath "/etc/hermes") (subpath "/private/etc/hermes")
 (subpath "/System") (subpath "/usr/lib") (subpath "/dev"))))
(deny process-exec (require-not (literal %s)))
(deny file-read* (subpath %s) (with errno ENOENT))
`, sandboxString(executable), ancestorReadLiterals(allowedRoot), sandboxString(allowedRoot), sandboxString(controlledSystem), sandboxString(executable), sandboxString(controlledSystem)) + absentSystemPolicy()
}

func sandboxString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func assertNoSandboxedConnection(t *testing.T, listener net.Listener) {
	t.Helper()
	setListenerDeadline(t, listener, time.Now().Add(200*time.Millisecond))
	connection, err := listener.Accept()
	if err == nil {
		_ = connection.Close()
		t.Fatal("sandboxed network probe reached the loopback listener")
	}
	var networkError net.Error
	if !errors.As(err, &networkError) || !networkError.Timeout() {
		t.Fatalf("accept after sandboxed probe: %v", err)
	}
}

func setListenerDeadline(t *testing.T, listener net.Listener, deadline time.Time) {
	t.Helper()
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		t.Fatalf("listener type = %T", listener)
	}
	if err := tcpListener.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
}

func TestOrdinaryCLIRejectsOutsideFixtureContent(t *testing.T) {
	fx := newFixture(t)
	outside := filepath.Join(fx.base, "excluded", "owned-system", "skills")
	writeFile(t, filepath.Join(outside, "canary", "SKILL.md"), []byte("---\nname: outside-canary\ndescription: Inert isolation control.\n---\nOWNED_OUTSIDE_CONTENT_CANARY\n"), 0600)
	result := fx.run(t, "list", "--root", outside, "--json")
	if bytes.Contains(result.stdout, []byte("OWNED_OUTSIDE_CONTENT_CANARY")) {
		t.Fatalf("ordinary CLI read outside fixture content: %s", result.stdout)
	}
	assertExit(t, result, 1)
}

func (fx fixture) launchArgs(t *testing.T, executable string, writer bool, extra string, args ...string) []string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("ordinary runtime containment requires Darwin sandbox-exec")
	}
	executable = canonicalPath(t, executable)
	profile := fx.fixturePolicy(t, writer)
	profile += extra
	sum := sha256.Sum256([]byte(profile))
	path := filepath.Join(builtBinaryDir, fmt.Sprintf("fixture-%x.sb", sum))
	if err := os.WriteFile(path, []byte(profile), 0600); err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, fmt.Sprintf("isolation/profiles/%x.sb", sum), []byte(profile))
	return append([]string{"/usr/bin/sandbox-exec", "-f", path, executable}, args...)
}

func TestFixtureLauncherContainmentControls(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, writer := range []bool{false, true} {
		for _, route := range []string{"cli", "pty", "tmux"} {
			t.Run(fmt.Sprintf("%s/writer-%t", route, writer), func(t *testing.T) {
				fx := newFixture(t)
				allowed := filepath.Join(fx.base, "allowed", "control.txt")
				excluded := filepath.Join(fx.base, "excluded", "canary.txt")
				write := filepath.Join(fx.base, "allowed", "control-write.txt")
				outsideWrite := filepath.Join(fx.base, "excluded", "outside-write.txt")
				absent := filepath.Join(fx.base, "excluded", "controlled-system")
				config := filepath.Join(absent, "config.toml")
				skills := filepath.Join(absent, "skills")
				writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
				writeFile(t, excluded, []byte("OWNED_EXCLUDED_READ_CANARY\n"), 0600)
				writeFile(t, config, []byte("OWNED_SYSTEM_CONFIG_CANARY\n"), 0600)
				writeFile(t, filepath.Join(skills, "control", "SKILL.md"), []byte("OWNED_SYSTEM_SKILL_CANARY\n"), 0600)
				if string(readBytes(t, config)) != "OWNED_SYSTEM_CONFIG_CANARY\n" {
					t.Fatal("system config positive control")
				}
				if entries, err := os.ReadDir(skills); err != nil || len(entries) != 1 {
					t.Fatal("system root positive control", err)
				}
				writeFile(t, outsideWrite, []byte("positive write"), 0600)
				if err := os.Remove(outsideWrite); err != nil {
					t.Fatal(err)
				}
				before := treeManifest(t, fx.base)
				argv := fx.launchArgs(t, isolationProbeBinary, writer, "", "boundary", fmt.Sprint(writer), allowed, excluded, write, outsideWrite, config, skills)
				var raw []byte
				switch route {
				case "cli":
					cmd := exec.Command(argv[0], argv[1:]...)
					cmd.Env, cmd.Dir = fx.environment(), fx.project
					launch := recordLaunch(t, cmd)
					raw, err = cmd.CombinedOutput()
					recordEvidenceCommand(t, "fixture-mode-control-cli", commandResult{code: launchExitCode(err), stdout: raw, launch: launch})
					assertRecordedContext(t, launch, cmd)
					if err != nil {
						t.Fatalf("control failed: %v\n%s", err, raw)
					}
				case "pty":
					cmd := exec.Command(argv[0], argv[1:]...)
					cmd.Env, cmd.Dir = fx.environment(), fx.project
					launch := recordLaunch(t, cmd)
					terminal, startErr := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
					if startErr != nil {
						t.Fatal(startErr)
					}
					raw, _ = io.ReadAll(terminal)
					err = cmd.Wait()
					_ = terminal.Close()
					recordEvidenceCommand(t, "fixture-mode-control-pty", commandResult{code: launchExitCode(err), stdout: raw, launch: launch})
					assertRecordedContext(t, launch, cmd)
					if err != nil {
						t.Fatalf("PTY control failed: %v\n%s", err, raw)
					}
				case "tmux":
					socket := fmt.Sprintf("ale27-controls-%d-%t", os.Getpid(), writer)
					defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
					runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "24", "-s", "browser", "exec /bin/cat")
					runTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
					cmd := exec.Command(argv[0], argv[1:]...)
					cmd.Env, cmd.Dir = fx.environment(), fx.project
					runTmuxProduct(t, tmux, fx, socket, cmd, "respawn-pane", "-k", "-t", "browser:0.0")
					deadline := time.Now().Add(3 * time.Second)
					var pane string
					for time.Now().Before(deadline) {
						status := strings.TrimSpace(runTmux(t, tmux, fx, socket, "display-message", "-p", "-t", "browser:0.0", "#{pane_dead}:#{pane_dead_status}"))
						if strings.HasPrefix(status, "1:") {
							pane = runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-J", "-S", "-", "-t", "browser:0.0")
							if status != "1:0" {
								t.Fatalf("tmux control exited %s: %s", status, pane)
							}
							break
						}
						time.Sleep(20 * time.Millisecond)
					}
					if pane == "" {
						t.Fatal("tmux control timed out")
					}
					raw = []byte(strings.Split(strings.TrimSpace(pane), "\n")[0])
				}
				var controls struct {
					Passed bool
					Checks map[string]bool
				}
				decodeJSON(t, bytes.TrimSpace(raw), &controls)
				if !controls.Passed {
					t.Fatalf("controls did not pass: %s", raw)
				}
				for name, passed := range controls.Checks {
					if !passed {
						t.Error("control failed", name)
					}
				}
				writeEvidence(t, fmt.Sprintf("isolation/ordinary/%s-writer-%t.json", route, writer), raw)
				writeEvidence(t, fmt.Sprintf("isolation/ordinary/%s-writer-%t.sb", route, writer), readBytes(t, argv[2]))
				if _, err := os.Stat(outsideWrite); !os.IsNotExist(err) {
					t.Fatal("outside write escaped", err)
				}
				if writer {
					if string(readBytes(t, write)) != "fixture-write\n" {
						t.Fatal("fixture writer did not write")
					}
					if err := os.Remove(write); err != nil {
						t.Fatal(err)
					}
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}
