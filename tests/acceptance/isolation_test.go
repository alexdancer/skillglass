package acceptance_test

import (
	"bytes"
	"context"
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
	canonicalProduct := canonicalPath(t, skillglassBinary)
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
		!bytes.Contains(browseRaw, []byte("Coverage: complete")) {
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
	command := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", commandArgs...)
	command.Dir = canonicalPath(t, fx.project)
	command.Env = fx.environment()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	launch := recordLaunch(t, command)
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
	assertRecordedContext(t, launch, command)
	return result
}

func runSandboxBrowser(t *testing.T, fx fixture, profile, executable string, browseArgs ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if len(browseArgs) == 0 {
		browseArgs = []string{"--root", fx.cleanRoot, "--project", fx.project}
	}
	commandArgs := append([]string{"-f", profile, executable, "browse"}, browseArgs...)
	command := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", commandArgs...)
	command.Dir = canonicalPath(t, fx.project)
	command.Env = fx.environment()
	started := time.Now()
	launch := recordLaunch(t, command)
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	var raw terminalTranscript
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&raw, terminal)
		close(copyDone)
	}()
	raw.wait(t, 0, "Selected: none", "Enter read")
	if _, err := io.WriteString(terminal, "q"); err != nil {
		t.Fatal(err)
	}
	err = command.Wait()
	<-copyDone
	_ = terminal.Close()
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
	return fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-write*)
(deny file-read-data (require-not (require-any
 (literal %s)
 (literal "/") %s
 (subpath %s)
 (subpath "/Library/Application Support/ClaudeCode") (subpath "/Library/Managed Preferences")
 (literal "/Library/Preferences/com.anthropic.claudecode.plist") (subpath "/etc/codex") (subpath "/etc/hermes")
 (subpath "/System")
 (subpath "/usr/lib")
 (subpath "/dev"))))
(deny process-exec (require-not (literal %s)))
`, sandboxString(executable), ancestorReadLiterals(allowedRoot), sandboxString(allowedRoot), sandboxString(executable)) + absentSystemPolicy()
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
