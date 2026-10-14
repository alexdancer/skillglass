package acceptance_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexdancer/skillglass/tests/launchrecord"
)

func TestALE33FindingLaunchReceipts(t *testing.T) {
	fx := piFixture(t)
	for _, result := range []struct {
		program string
		code    int
		output  string
	}{{"/usr/bin/printf", 0, "receipt-output"}, {"/usr/bin/printf", 0, "new-session"}, {"/usr/bin/false", 1, ""}} {
		command := fx.recordedCommand(context.Background(), result.program)
		if result.output != "" {
			command.Args = append(command.Args, result.output)
		}
		command.Dir = fx.project
		command.Env = fx.environment()
		output, err := command.CombinedOutput()
		if result.code == 0 && err != nil {
			t.Fatal(err)
		}
		if result.code != 0 && err == nil {
			t.Fatal("failed child accepted")
		}
		assertLaunchReceipt(t, command, output, result.code)
	}
	t.Run("prelaunch-ordering", func(t *testing.T) {
		t.Setenv("SKILLGLASS_EVIDENCE", fx.tmuxTemporary)
		command := fx.recordedCommand(context.Background(), "/bin/sh", "-c", `set -- "$FIXTURE_RECEIPTS"/invocations/*.invocation.json; test -f "$1" || exit 7; printf prelaunch`)
		command.Dir = fx.project
		command.Env = append(fx.environment(), "FIXTURE_RECEIPTS="+fx.tmuxTemporary)
		output, err := command.CombinedOutput()
		if err != nil || string(output) != "prelaunch" {
			t.Fatalf("invocation must precede execution: %v %q", err, output)
		}
		assertLaunchReceipt(t, command, output, 0)
	})
	t.Run("finalized-fields", func(t *testing.T) {
		command := fx.recordedCommand(context.Background(), "/usr/bin/printf", "initial")
		command.Dir = fx.project
		command.Env = append(fx.environment(), "FIXTURE_VALUE=final")
		command.Args[len(command.Args)-1] = "final"
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if string(output) != "final" {
			t.Fatalf("argv not finalized: %q", output)
		}
		assertLaunchReceipt(t, command, output, 0)
		if command.record.Identity.Environment[len(command.Env)-1] != "FIXTURE_VALUE=final" {
			t.Fatal("environment sampled before launch")
		}
	})
	t.Run("file-profile", func(t *testing.T) {
		profile := filepath.Join(fx.temporary, "record.sb")
		writeFile(t, profile, []byte("(version 1)\n(allow default)\n(deny network*)\n"), 0600)
		command := fx.wrap(exec.Command("/usr/bin/sandbox-exec", "-f", profile, "/usr/bin/printf", "file-profile"))
		command.Dir = fx.project
		command.Env = fx.environment()
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		assertLaunchReceipt(t, command, output, 0)
		raw, err := os.ReadFile(profile)
		if err != nil {
			t.Fatal(err)
		}
		if command.record.Identity.Profile.SHA256 != launchrecord.Digest(raw) {
			t.Fatal("file profile not sampled")
		}
	})
	t.Run("pty", func(t *testing.T) {
		command := fx.recordedCommand(context.Background(), "/usr/bin/printf", "pty-receipt")
		command.Dir = fx.project
		command.Env = fx.environment()
		terminal, err := command.startPTY(nil)
		if err != nil {
			t.Fatal(err)
		}
		output, _ := io.ReadAll(terminal)
		err = command.Wait()
		terminal.Close()
		if receiptErr := command.completePTY(err, output); receiptErr != nil {
			t.Fatal(receiptErr)
		}
		assertLaunchReceipt(t, command, output, 0)
	})
	t.Run("failed-start", func(t *testing.T) {
		invalid := filepath.Join(fx.temporary, "not-executable")
		writeFile(t, invalid, []byte("not executable\n"), 0600)
		command := fx.recordedCommand(context.Background(), invalid)
		command.Dir = fx.project
		command.Env = fx.environment()
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatal("failed launch accepted")
		}
		if command.record == nil {
			t.Fatal("failed launch identity missing")
		}
		raw, readErr := os.ReadFile(filepath.Join(command.record.Directory, command.record.Identity.ID+".completion.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var completion struct {
			Code int `json:"exit_code"`
		}
		decodeJSON(t, raw, &completion)
		if completion.Code == 0 {
			t.Fatalf("failed launch recorded success: %q %q", raw, output)
		}
	})
	t.Run("unavailable-evidence", func(t *testing.T) {
		destination := filepath.Join(fx.temporary, "evidence-file")
		writeFile(t, destination, nil, 0600)
		t.Setenv("SKILLGLASS_EVIDENCE", destination)
		marker := filepath.Join(fx.temporary, "must-not-run")
		command := fx.recordedCommand(context.Background(), "/bin/sh", "-c", "printf forbidden > "+shellQuote(marker))
		command.Dir = fx.project
		command.Env = fx.environment()
		if err := command.Run(); err == nil {
			t.Fatal("unrecorded command launched")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("payload ran without evidence")
		}
	})
}

func TestALE33FindingTmuxReceipts(t *testing.T) {
	fx := piFixture(t)
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	socket := "receipts-" + filepath.Base(fx.base)
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runContainedTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "48", "-y", "24", "-s", "receipt", "exec /bin/sleep 30")
	runContainedTmux(t, tmux, fx, socket, "set-option", "-g", "remain-on-exit", "on")
	runContainedTmux(t, tmux, fx, socket, "new-window", "-t", "receipt", "-n", "probe", "exec /usr/bin/printf 'tmux-receipt\\n'")
	directory := filepath.Join(fx.tmuxTemporary, "invocations")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		files, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".completion.json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(directory, file.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var completion struct {
				ID     string `json:"launch_id"`
				Code   int    `json:"exit_code"`
				Stdout string `json:"stdout_sha256"`
			}
			decodeJSON(t, raw, &completion)
			if completion.Code != 0 {
				t.Fatalf("tmux payload failed: %s", raw)
			}
			raw, err = os.ReadFile(filepath.Join(directory, completion.ID+".invocation.json"))
			if err != nil {
				t.Fatal(err)
			}
			var identity launchrecord.Identity
			decodeJSON(t, raw, &identity)
			if len(identity.Executables) != 1 || len(identity.Artifacts) != 2 || identity.Argv[0] != "/bin/sh" || identity.Profile == nil || identity.Profile.InheritedFrom != identity.Profile.SHA256 {
				t.Fatalf("missing inherited payload identity: %s", raw)
			}
			output, err := os.ReadFile(filepath.Join(directory, completion.ID+".stdout"))
			if err != nil {
				t.Fatal(err)
			}
			if completion.Stdout != launchrecord.Digest(output) || !strings.Contains(string(output), "tmux-receipt") {
				t.Fatal("tmux output identity mismatch")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("tmux payload completion missing")
}

func assertLaunchReceipt(t *testing.T, command *fixtureCommand, output []byte, code int) {
	t.Helper()
	record := command.record
	if record == nil {
		t.Fatal("launch identity missing")
	}
	raw, err := os.ReadFile(filepath.Join(record.Directory, record.Identity.ID+".invocation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var identity launchrecord.Identity
	decodeJSON(t, raw, &identity)
	if identity.ID == "" || identity.Cwd != command.Dir || len(identity.Executables) == 0 || len(identity.Environment) != len(command.Env) {
		t.Fatalf("incomplete identity: %+v", identity)
	}
	actual, err := launchrecord.Identify(command.Path)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Executables[0] != actual {
		t.Fatal("executable identity mismatch")
	}
	if filepath.Base(command.Path) == "sandbox-exec" && (identity.Profile == nil || launchrecord.Digest(identity.Profile.Bytes) != identity.Profile.SHA256) {
		t.Fatal("missing applied policy bytes")
	}
	raw, err = os.ReadFile(filepath.Join(record.Directory, identity.ID+".completion.json"))
	if err != nil {
		t.Fatal(err)
	}
	var completion struct {
		ID     string `json:"launch_id"`
		Code   int    `json:"exit_code"`
		Stdout string `json:"stdout_sha256"`
	}
	if err := json.Unmarshal(raw, &completion); err != nil {
		t.Fatal(err)
	}
	if completion.ID != identity.ID || completion.Code != code || completion.Stdout != launchrecord.Digest(output) {
		t.Fatalf("completion mismatch: %s", raw)
	}
}
