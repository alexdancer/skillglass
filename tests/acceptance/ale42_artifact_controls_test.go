package acceptance_test

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/tests/launchrecord"
)

func TestALE42ProductTamperingFailsClosed(t *testing.T) {
	for _, route := range []string{"ordinary", "tmux", "finalized-tmux-payload"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			original := readBytes(t, stagedProductPath)
			// Change the real staged product bytes, not the expected digest or dispatch.
			if err := os.WriteFile(stagedProductPath, append(append([]byte(nil), original...), []byte("TAMPER_CANARY")...), 0700); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(stagedProductPath, original, 0700); err != nil {
					t.Fatal(err)
				}
			})
			command := fx.wrap(fx.productCommand(t, "--help"))
			want := "product artifact changed"
			if route == "tmux" {
				tmux, err := exec.LookPath("tmux")
				if err != nil {
					t.Fatal(err)
				}
				command = fx.wrap(exec.Command(tmux, "-S", fx.tmuxSocket("tamper-control"), "-f", "/dev/null", "new-session", "-d", "exec "+shellQuote(stagedProductPath)+" --help"))
				command.payloadProfile = []byte(fx.fixturePolicy(t, false))
				command.payloadIdentity = launchrecord.File{Path: stagedProductPath, SHA256: productDigest}
				command.Dir, command.Env = fx.project, fx.environment()
			}
			if route == "finalized-tmux-payload" {
				// Exercise the same inner launcher check used after tmux dispatch. A
				// mismatch must exit before allocating a PTY or starting its shell payload.
				command = fx.wrap(exec.Command(fixtureLaunchBinary, "--directory", filepath.Join(fx.tmuxTemporary, "invocations"), "--profile", base64.StdEncoding.EncodeToString([]byte(fx.fixturePolicy(t, false))), "--deferred-profile", "--artifact", stagedProductPath, "--expected-artifact-sha256", productDigest, "--", "/bin/sh", "-c", "exec "+shellQuote(stagedProductPath)+" --help"))
				command.Dir, command.Env = fx.project, fx.environment()
				want = "finalized payload artifact mismatch"
			}
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(err.Error()+string(output), want) {
				t.Fatalf("tampering reached payload or wrong refusal: %v %s", err, output)
			}
			if route != "finalized-tmux-payload" && command.Process != nil {
				t.Fatal("tampered product controller was started")
			}
			if route == "finalized-tmux-payload" {
				if command.ProcessState == nil || command.ProcessState.ExitCode() != 125 {
					t.Fatalf("inner launcher exit: %v", err)
				}
			}
			writeEvidence(t, "ale42/tamper/"+route+".txt", []byte(err.Error()+"\n"+string(output)))
		})
	}
}
