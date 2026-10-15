package acceptance_test

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestReviewRejectUncontainedTmuxPayload(t *testing.T) {
	for _, operation := range []string{"new-session", "new-window", "respawn-pane"} {
		t.Run(operation, func(t *testing.T) {
			fx := newFixture(t)
			canary := filepath.Join(fx.base, "excluded", "system-inputs", "owned-canary")
			writeFile(t, canary, []byte("OWNED_SYSTEM_CANARY"), 0600)
			tmux, err := exec.LookPath("tmux")
			if err != nil {
				t.Fatal(err)
			}
			socket := "review-uncontained"
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			payload := shellQuote(isolationProbeBinary) + " --system " + shellQuote(canary) + "; /bin/sleep 2"
			options := []string{"new-session", "-d", "-x", "220", "-y", "24", "-s", "probe"}
			target := "probe:0.0"
			if operation != "new-session" {
				runTmuxBootstrap(t, tmux, fx, socket, "control", 220, 24)
				if operation == "new-window" {
					options = []string{"new-window", "-d", "-t", "control", "-n", "probe"}
					target = "control:probe.0"
				} else {
					options = []string{"respawn-pane", "-k", "-t", "control:0.0"}
					target = "control:0.0"
				}
			}
			argv := append([]string{"-S", fx.tmuxSocket(socket), "-f", "/dev/null"}, options...)
			command := fx.wrap(exec.Command(tmux, append(argv, payload)...))
			command.Dir, command.Env = fx.project, fx.environment()
			output, err := command.CombinedOutput()
			if err != nil && strings.Contains(err.Error(), "uncontained tmux payload") {
				return
			}
			if err != nil {
				t.Fatalf("unexpected launch failure: %v %s", err, output)
			}
			deadline := time.Now().Add(time.Second)
			var frame string
			for time.Now().Before(deadline) {
				frame = runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-J", "-t", target)
				if strings.Contains(frame, `"read_absent":false`) {
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			writeEvidence(t, "review/uncontained-tmux-"+operation+".txt", []byte(frame))
			t.Fatalf("uncontained tmux payload was launched: %s", frame)
		})
	}
}

func TestReviewRejectUnprofiledLauncher(t *testing.T) {
	fx := newFixture(t)
	canary := filepath.Join(fx.base, "excluded", "system-inputs", "owned-canary")
	writeFile(t, canary, []byte("OWNED_SYSTEM_CANARY"), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, fixtureLaunchBinary, "--directory", filepath.Join(fx.tmuxTemporary, "invocations"), "--artifact", isolationProbeBinary, "--", isolationProbeBinary, "--system", canary)
	command.Dir, command.Env = fx.project, fx.environment()
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 220})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(terminal)
	err = command.Wait()
	terminal.Close()
	writeEvidence(t, "review/unprofiled-launcher.txt", raw)
	if err == nil || command.ProcessState.ExitCode() != 125 || !strings.Contains(string(raw), "missing containment profile") || strings.Contains(string(raw), `"read_absent":false`) {
		t.Fatalf("unprofiled launch must fail before payload: %v %s", err, raw)
	}
}

func TestReviewPiKnownPrecedencePublic(t *testing.T) {
	for _, explicit := range []string{"neither", "winner", "loser"} {
		t.Run(explicit, func(t *testing.T) {
			fx := piFixture(t)
			global := codexSkill(t, filepath.Join(fx.home, ".pi", "agent", "skills"), "global", "collision")
			project := codexSkill(t, filepath.Join(fx.project, ".pi", "skills"), "project", "collision")
			if explicit != "neither" {
				path := project
				if explicit == "loser" {
					path = global
				}
				writeFile(t, path, []byte("---\nname: collision\ndescription: Fixture.\ndisable-model-invocation: true\n---\nInert instructions.\n"), 0600)
			}
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "pi", "--project", fx.project, "--json")
			assertExit(t, result, 0)
			writeEvidence(t, "review/pi-"+explicit+"-list.json", result.stdout)
			var list codexList
			decodeJSON(t, result.stdout, &list)
			if len(list.Sources) != 2 {
				t.Fatalf("sources: %s", result.stdout)
			}
			winner := ""
			for _, source := range list.Sources {
				if source.SourcePath == canonicalPath(t, filepath.Dir(project)) {
					winner = source.ID
				}
			}
			if winner == "" {
				t.Fatalf("project winner absent: %s", result.stdout)
			}
			for _, source := range list.Sources {
				want := "shadowed"
				if source.ID == winner {
					want = "winning"
				}
				check := func(operation string, got codexSource) {
					t.Helper()
					if got.ID != source.ID || got.Availability.Precedence.State != want || got.Availability.Precedence.Winner != winner {
						t.Errorf("%s %s precedence = %+v, want %s winner %s", operation, source.ID, got.Availability.Precedence, want, winner)
					}
				}
				check("list", source)
				for _, operation := range []string{"read", "explain"} {
					result := fx.run(t, operation, "--agent", "pi", "--project", fx.project, "--json", source.ID)
					assertExit(t, result, 0)
					writeEvidence(t, "review/pi-"+explicit+"-"+want+"-"+operation+".json", result.stdout)
					var response struct{ Source codexSource }
					decodeJSON(t, result.stdout, &response)
					check(operation, response.Source)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
