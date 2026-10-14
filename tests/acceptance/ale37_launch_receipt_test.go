package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE37FinalizedConfiguredHomeReceipts(t *testing.T) {
	if os.Getenv("SKILLGLASS_EVIDENCE") == "" {
		t.Setenv("SKILLGLASS_EVIDENCE", t.TempDir())
	}
	fx := geminiFixture(t)
	configured := filepath.Join(fx.base, "allowed", "configured-gemini")
	cwd := filepath.Join(fx.base, "allowed", "finalized-gemini-cwd")
	if err := os.MkdirAll(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	codexSkill(t, filepath.Join(configured, ".agents", "skills"), "configured", "configured-gemini-literal")
	codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), "default", "default-gemini-literal")
	before := treeManifest(t, fx.base)
	run := func(env []string, workingDirectory, expected string) *launchReceipt {
		t.Helper()
		command := fx.command(t, context.Background(), "list", "--agent", "gemini", "--json")
		command.Env, command.Dir = env, workingDirectory
		launch := recordLaunch(t, command)
		raw, err := command.Output()
		result := commandResult{stdout: raw, code: launchExitCode(err), launch: launch}
		recordEvidenceCommand(t, "gemini-finalized-home", result)
		assertExit(t, result, 0)
		assertRecordedContext(t, launch, command)
		var list codexList
		decodeJSON(t, raw, &list)
		if len(list.Sources) != 1 || list.Sources[0].Name != expected || list.Sources[0].Availability.State != "ask" {
			t.Fatalf("finalized Gemini home: %s", raw)
		}
		return launch
	}
	first := run(append(fx.environment(), "GEMINI_CLI_HOME="+configured), cwd, "configured-gemini-literal")
	firstPath := filepath.Join(os.Getenv("SKILLGLASS_EVIDENCE"), first.Path)
	original, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second := run(fx.environment(), fx.project, "default-gemini-literal")
	third := run(append(fx.environment(), "GEMINI_CLI_HOME="), fx.project, "default-gemini-literal")
	fourth := run(fx.environment(), fx.project, "default-gemini-literal")
	ids := map[string]bool{}
	for _, receipt := range []*launchReceipt{first, second, third, fourth} {
		if ids[receipt.ID] {
			t.Fatal("Gemini launch receipt identity reused")
		}
		ids[receipt.ID] = true
	}
	preserved, err := os.ReadFile(firstPath)
	if err != nil || !bytes.Equal(original, preserved) {
		t.Fatal("configured Gemini receipt overwritten", err)
	}
	for _, entry := range second.Environment {
		if strings.HasPrefix(entry, "GEMINI_CLI_HOME=") {
			t.Fatal("absent Gemini home override invented")
		}
	}
	emptyFound := false
	for _, entry := range third.Environment {
		emptyFound = emptyFound || entry == "GEMINI_CLI_HOME="
	}
	if !emptyFound {
		t.Fatal("explicit empty Gemini home override lost")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			command := fx.productCommand(t, "browse", "--agent", "gemini")
			command.Env = append(fx.environment(), "GEMINI_CLI_HOME="+configured)
			command.Dir = cwd
			socket := fmt.Sprintf("gemini-finalized-%d", width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			launch := runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", fmt.Sprint(width), "-y", "24", "-s", "browser")
			frame := waitForPane(t, tmux, fx, socket, false, "Skillglass", "configured-gemini-literal", "Focus: 1/1")
			assertPaneCells(t, frame, width, 24)
			if strings.Contains(frame, "default-gemini-literal") {
				t.Fatal("tmux lost finalized Gemini home")
			}
			assertRecordedContext(t, launch, command)
			writeEvidence(t, fmt.Sprintf("receipt-boundaries/gemini-%d-frame.txt", width), []byte(frame))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
