package acceptance_test

import (
	"bytes"
	"context"
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

func TestOrdinaryLaunchContainmentCLIAndPTYAndTmux(t *testing.T) {
	fx := newFixture(t)
	allowed := filepath.Join(fx.base, "allowed", "positive.txt")
	excluded := filepath.Join(fx.base, "excluded", "live-home", "canary.txt")
	system := filepath.Join(fx.base, "excluded", "system-inputs", "config.toml")
	blocked := filepath.Join(fx.base, "allowed", "blocked-write.txt")
	writeFile(t, allowed, []byte("ALLOWED_READ_SENTINEL\n"), 0600)
	writeFile(t, excluded, []byte("LIVE_HOME_READ_CANARY\n"), 0600)
	writeFile(t, system, []byte("MODELED_SYSTEM_POLICY"), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	runPositiveControls(t, allowed, excluded, blocked, listener)
	if data, err := os.ReadFile(system); err != nil || string(data) != "MODELED_SYSTEM_POLICY" {
		t.Fatalf("system positive: %s %v", data, err)
	}
	networkCanary := allowedProbeNetworkCanary(t, fx, listener)
	before := treeManifest(t, fx.base)
	args := []string{"--network-canary", networkCanary, "--system-policy", allowed, excluded, blocked, listener.Addr().String(), system}
	check := func(raw []byte) {
		var result struct {
			Passed bool
			Checks map[string]bool
		}
		decodeJSON(t, raw, &result)
		if !result.Passed || !result.Checks["system_absent"] {
			t.Fatalf("ordinary-launch controls: %s", raw)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := fx.containedCommand(t, ctx, isolationProbeBinary, args...)
	command.Env = fx.environment()
	command.Dir = fx.project
	cliLaunch := recordLaunch(t, command)
	output, err := command.Output()
	recordEvidenceCommand(t, "fixture-controls-cli", commandResult{code: launchExitCode(err), stdout: output, launch: cliLaunch})
	if err != nil {
		t.Fatalf("CLI inert controls: %v\n%s", err, output)
	}
	check(output)
	writeEvidence(t, "fixture-launch/cli.json", output)
	profile, err := os.ReadFile(fx.policy)
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, "fixture-launch/product.sb", profile)
	command = fx.containedCommand(t, ctx, isolationProbeBinary, args...)
	command.Env = fx.environment()
	command.Dir = fx.project
	ptyLaunch := recordLaunch(t, command)
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&raw, terminal); close(done) }()
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	<-done
	_ = terminal.Close()
	recordEvidenceCommand(t, "fixture-controls-pty", commandResult{stdout: raw.Bytes(), launch: ptyLaunch})
	check(bytes.TrimSpace(raw.Bytes()))
	writeEvidence(t, "fixture-launch/pty.json", raw.Bytes())
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("fixture-controls-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	command = fx.containedCommand(t, ctx, isolationProbeBinary, args...)
	runTmuxBootstrap(t, tmux, fx, socket, "control", 220, 24)
	runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "control:0", "remain-on-exit", "on")
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "control:0", "-x", "220", "-y", "24")
	runTmuxProduct(t, tmux, fx, socket, command, "respawn-pane", "-k", "-t", "control:0.0")
	_ = waitForPaneAt(t, tmux, fx, socket, "control:0.0", true, "Pane is dead")
	frame := runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-J", "-S", "-1000", "-t", "control:0.0")
	// The inert probe emits one JSON line; tmux includes the dead-pane marker below it.
	start := strings.Index(frame, `{"passed"`)
	if start < 0 {
		t.Fatalf("missing probe JSON: %s", frame)
	}
	end := strings.Index(frame[start:], "\n")
	if end < 0 {
		t.Fatalf("missing probe JSON: %s", frame)
	}
	check([]byte(frame[start : start+end]))
	writeEvidence(t, "fixture-launch/tmux.txt", []byte(frame))
	assertNoSandboxedConnection(t, listener, networkCanary)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// Inspect the actual product's native system routes under this same launch profile.
	fx, _ = codexFixture(t)
	result := fx.run(t, "list", "--agent", "codex", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	systemConfig, systemSkills := false, false
	for _, layer := range list.Scope.Agent.ConfigLayers {
		if layer.Path == "/etc/codex/config.toml" {
			systemConfig = layer.State == "absent"
		}
	}
	for _, root := range list.Scope.Roots {
		if root.RequestedPath == "/etc/codex/skills" {
			systemSkills = root.State == "missing"
		}
	}
	if !systemConfig || !systemSkills {
		t.Fatalf("native system inputs must be modeled absent: %s", result.stdout)
	}
	encoded, _ := json.MarshalIndent(list.Scope.Agent.ConfigLayers, "", "  ")
	writeEvidence(t, "fixture-launch/codex-system-layers.json", encoded)
}
