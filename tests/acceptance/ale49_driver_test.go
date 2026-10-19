package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// These are controller-boundary probes, not product CLI invocations. They use
// the ordinary fixture sandbox, synthetic environment and recorded child bytes.
func TestALE49DriverLateContextPreservesExit(t *testing.T) {
	for _, code := range []int{0, 7} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			fx := newFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := ale49DriverCommand(t, fx, ctx, fmt.Sprintf("printf 'DRIVER_STDOUT'; printf 'DRIVER_STDERR' >&2; exit %d", code))
			result, err := executeFinalizedCommand(t, cmd)
			cancel()
			if ctx.Err() == nil {
				t.Fatal("late context did not expire")
			}
			recordEvidenceCommand(t, "driver-boundary-control", result)
			if result.code != code || result.executionCanceled || result.recordingErr != nil {
				t.Fatalf("late context changed completed execution: %+v %v", result, err)
			}
			if (err == nil) != (code == 0) {
				t.Fatalf("exit error changed: %v", err)
			}
			receipt := ale49DriverReceipt(t, result)
			if receipt.ExitCode != code || receipt.ExecutionCanceled || !bytes.Equal(result.stdout, []byte("DRIVER_STDOUT")) || !bytes.Equal(result.stderr, []byte("DRIVER_STDERR")) {
				t.Fatal("completed streams or exit lost", receipt)
			}
		})
	}
}

func TestALE49DriverCanceledMutationRetainsReceipt(t *testing.T) {
	fx := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	marker := filepath.Join(fx.base, "allowed", "driver-started")
	started := make(chan bool, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				started <- false
				return
			case <-ticker.C:
				if _, err := os.Stat(marker); err == nil {
					cancel()
					started <- true
					return
				}
			}
		}
	}()
	script := "printf 'BEFORE_CANCEL'; printf 'CANCEL_STDERR' >&2; : > " + shellQuote(marker) + "; exec /bin/sleep 60"
	cmd := ale49DriverCommand(t, fx, ctx, script)
	result, err := executeEvidenceCommand(t, cmd, "driver-boundary-control")
	if !<-started {
		t.Fatal("controlled child did not start before cancellation")
	}
	if err == nil || !result.executionCanceled || result.code != -1 || result.recordingErr != nil {
		t.Fatalf("actual cancellation not retained: %+v %v", result, err)
	}
	receipt := ale49DriverReceipt(t, result)
	if !receipt.ExecutionCanceled || receipt.ExecutionError == "" || receipt.ExitCode != -1 || receipt.Kind != "driver-boundary-control" {
		t.Fatal("mutation cancellation receipt incomplete", receipt)
	}
	if !bytes.Equal(result.stdout, []byte("BEFORE_CANCEL")) || !bytes.Equal(result.stderr, []byte("CANCEL_STDERR")) {
		t.Fatal("partial child streams lost")
	}
}

func TestALE49DriverPrelaunchDeadlineRetainsReceipt(t *testing.T) {
	fx := newFixture(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	cmd := ale49DriverCommand(t, fx, ctx, "printf 'MUST_NOT_RUN'")
	result, err := executeEvidenceCommand(t, cmd, "driver-boundary-control")
	var exit *exec.ExitError
	if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &exit) || !result.executionCanceled || result.code != -1 {
		t.Fatalf("non-ExitError cancellation changed: %+v %v", result, err)
	}
	receipt := ale49DriverReceipt(t, result)
	if receipt.ID != result.launch.ID || receipt.ExecutionError == "" || !receipt.ExecutionCanceled || len(result.stdout) != 0 || len(result.stderr) != 0 {
		t.Fatal("prelaunch failure evidence lost", receipt)
	}
}

func ale49DriverReceipt(t *testing.T, result commandResult) evidenceCase {
	t.Helper()
	root := os.Getenv("SKILLGLASS_EVIDENCE")
	if root == "" {
		t.Fatal("driver contract requires retained evidence")
	}
	var receipt evidenceCase
	path := filepath.Join(root, "commands", result.launch.ID, "case.json")
	if err := json.Unmarshal(readBytes(t, path), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ID != result.launch.ID || receipt.LaunchReceipt != result.launch.Path {
		t.Fatal("launch/result identity lost", receipt)
	}
	if !bytes.Equal(readBytes(t, filepath.Join(root, receipt.Stdout)), result.stdout) || !bytes.Equal(readBytes(t, filepath.Join(root, receipt.Stderr)), result.stderr) {
		t.Fatal("saved child streams differ")
	}
	if _, err := os.Stat(filepath.Join(root, receipt.LaunchReceipt)); err != nil {
		t.Fatal("prelaunch receipt missing", err)
	}
	return receipt
}

func ale49DriverCommand(t *testing.T, fx fixture, ctx context.Context, script string) *exec.Cmd {
	t.Helper()
	command := fx.recordedCommand(ctx, "/bin/sh", "-c", script).Cmd
	// The ordinary probe policy remains byte-identical; file-backed launch context
	// is the shared mutation helper's existing receipt contract.
	profile := filepath.Join(fx.base, "driver-policy.sb")
	writeFile(t, profile, []byte(command.Args[2]), 0600)
	command.Args[1], command.Args[2] = "-f", profile
	return command
}
