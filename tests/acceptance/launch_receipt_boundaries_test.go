package acceptance_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func assertRecordedContext(t *testing.T, launch *launchReceipt, command *exec.Cmd) {
	t.Helper()
	if os.Getenv("SKILLGLASS_EVIDENCE") == "" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), launch.Path))
	if err != nil {
		t.Fatal(err)
	}
	var recorded launchReceipt
	decodeJSON(t, raw, &recorded)
	cwd := command.Dir
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(recorded.Argv, command.Args) || !reflect.DeepEqual(recorded.Environment, command.Environ()) || recorded.Cwd != canonicalPath(t, cwd) {
		t.Fatalf("finalized receipt context mismatch: %+v", recorded)
	}
	binary, err := os.ReadFile(recorded.Executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binary)
	if recorded.BinarySHA256 != hex.EncodeToString(digest[:]) {
		t.Error("receipt executable hash mismatch")
	}
	if recorded.Profile != "" {
		profile, err := os.ReadFile(command.Args[2])
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(profile)
		if recorded.Profile != string(profile) || recorded.ProfileSHA256 != hex.EncodeToString(digest[:]) {
			t.Error("receipt policy bytes or hash mismatch")
		}
	}
}

func TestFinalizedTmuxLaunchReceipt(t *testing.T) {
	if os.Getenv("SKILLGLASS_EVIDENCE") == "" {
		t.Setenv("SKILLGLASS_EVIDENCE", t.TempDir())
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	fx := newFixture(t)
	configured := filepath.Join(fx.base, "allowed", "configured")
	cwd := filepath.Join(fx.base, "allowed", "finalized-cwd")
	if err := os.MkdirAll(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	claudeSkill(t, filepath.Join(configured, "skills"), "configured", "configured-home-literal", "")
	claudeSkill(t, filepath.Join(fx.home, ".claude", "skills"), "default", "default-home-literal", "")
	command := fx.productCommand(t, "browse", "--agent", "claude-code", "--project", fx.project)
	command.Env = append(fx.environment(), "CLAUDE_CONFIG_DIR="+configured)
	command.Dir = cwd
	before := treeManifest(t, fx.base)
	socket := "finalized-launch"
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	launch := runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser")
	frame := waitForPane(t, tmux, fx, socket, false, "Skillglass", "configured-home-literal", "Focus: 1/1")
	if strings.Contains(frame, "default-home-literal") {
		t.Fatal("tmux payload lost finalized configured home")
	}
	assertRecordedContext(t, launch, command)
	casePaths, err := filepath.Glob(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), "commands", "*", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range casePaths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var record evidenceCase
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		if record.PayloadLaunch != launch.Path {
			continue
		}
		found = true
		utilityBytes, err := os.ReadFile(filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), record.LaunchReceipt))
		if err != nil {
			t.Fatal(err)
		}
		var utility launchReceipt
		decodeJSON(t, utilityBytes, &utility)
		if !reflect.DeepEqual(utility.Environment, fx.environment()) || utility.Cwd != canonicalPath(t, fx.project) || utility.ID == launch.ID || utility.Profile != "" {
			t.Error("tmux utility context was conflated with product payload")
		}
		if record.ExitCode != 0 || !reflect.DeepEqual(record.Argv, utility.Argv) {
			t.Error("tmux launch acknowledgement was not linked to utility receipt")
		}
	}
	if !found {
		t.Error("tmux output record lost finalized payload link")
	}
	writeEvidence(t, "receipt-boundaries/tmux-frame.txt", []byte(frame))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
