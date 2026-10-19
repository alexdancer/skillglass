package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alexdancer/skillglass/internal/collection"
)

func ale52AliasReview(t *testing.T, scenario string) (fixture, collection.ReviewDraft, []string, string) {
	t.Helper()
	fx := piFixture(t)
	project := strings.HasPrefix(scenario, "project-")
	from := filepath.Join(fx.home, ".pi/agent")
	config := filepath.Join(from, "settings.json")
	root := filepath.Join(from, "skills")
	scope, selectedProject := collection.ControlGlobal, ""
	if project {
		from = filepath.Join(fx.project, ".pi")
		config = filepath.Join(from, "settings.json")
		root = filepath.Join(from, "skills")
		writeFile(t, config, []byte(`{"keep":true}`), 0600)
		scope, selectedProject = collection.ControlProject, fx.project
	}
	if strings.HasSuffix(scenario, "-create") {
		if err := os.Remove(config); err != nil {
			t.Fatal(err)
		}
	}
	if scenario == "global-create-directory" {
		if err := os.RemoveAll(from); err != nil {
			t.Fatal(err)
		}
		from = fx.home
		root = filepath.Join(fx.home, ".agents/skills")
	}
	if scenario != "control" {
		to := filepath.Join(fx.home, "pi-real")
		if scenario == "global-create-directory" {
			to = filepath.Join(fx.base, "allowed", "pi-real")
		}
		if err := os.Rename(from, to); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(to, from); err != nil {
			t.Fatal(err)
		}
	}
	manifests := []string{codexSkill(t, root, "first", "first"), codexSkill(t, root, "second", "second")}
	draft := ale46Draft([]string{"pi"}, scope, selectedProject)
	for i, path := range manifests {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		id := ale50PiSource(t, fx, resolved, 0).ID
		draft.Sources = append(draft.Sources, ale46Assess(id))
		draft.Changes = append(draft.Changes, ale46Change(fmt.Sprintf("disable-%d", i), id, "pi", collection.ControlDisable))
	}
	return fx, draft, manifests, from
}

func TestALE52PiOrderedDirectoryAlias(t *testing.T) {
	for _, scenario := range []string{"control", "global-existing", "global-create", "project-existing", "project-create", "global-create-directory"} {
		t.Run(scenario, func(t *testing.T) {
			fx, draft, manifests, _ := ale52AliasReview(t, scenario)
			before := [][]byte{readBytes(t, manifests[0]), readBytes(t, manifests[1])}
			_, path := ale47Approved(t, fx, draft, []string{"disable-0", "disable-1"})
			out := ale47Apply(t, fx, path, "", 0)
			if out.State != "applied" || len(out.Results) != 2 {
				t.Fatalf("ordered approved own writes must finish: %+v", out)
			}
			for i, result := range out.Results {
				if result.State != "succeeded" || result.Outcome == nil || result.Outcome.WriteState != "committed" || result.Outcome.Persistence != "verified" || result.Final == nil || result.Final.State != collection.AvailabilityDisabled {
					t.Fatalf("unexpected receipt: %+v", result)
				}
				if !bytes.Equal(before[i], readBytes(t, manifests[i])) {
					t.Fatal("source modified")
				}
			}
			stable := treeManifest(t, fx.base)
			ale47Apply(t, fx, path, "", 1)
			assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
			_, path = ale47Approved(t, fx, draft, []string{"disable-1"})
			stable = treeManifest(t, fx.base)
			out = ale47Apply(t, fx, path, "", 0)
			if len(out.Results) != 1 {
				t.Fatalf("unexpected no-op results: %+v", out)
			}
			for _, result := range out.Results {
				if result.State != "no-op" || result.Outcome == nil || result.Outcome.WriteState != "none" || result.Outcome.Persistence != "verified" {
					t.Fatalf("unexpected no-op receipt: %+v", result)
				}
			}
			assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
		})
	}
}

func TestALE52PiAliasExternalEditsDuringReview(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		for _, mutation := range []string{"content", "metadata", "same-byte-replacement", "retarget", "new-input"} {
			t.Run(scope+"/"+mutation, func(t *testing.T) {
				fx, draft, manifests, alias := ale52AliasReview(t, scope+"-existing")
				claudeDir := filepath.Join(fx.home, ".claude")
				if scope == "project" {
					claudeDir = filepath.Join(fx.project, ".claude")
					writeFile(t, filepath.Join(claudeDir, "settings.json"), []byte(`{}`), 0600)
				}
				shared := filepath.Join(claudeDir, "skills/first")
				if err := os.MkdirAll(filepath.Dir(shared), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(manifests[0]), shared); err != nil {
					t.Fatal(err)
				}
				draft.Context.Applications = []string{"pi", "claude-code"}
				draft.Changes = append(draft.Changes, ale46Change("pause-claude", string(draft.Sources[0].Source), "claude-code", collection.ControlDisable))
				selection, path := ale47Approved(t, fx, draft, []string{"disable-0", "pause-claude", "disable-1"})
				lock, err := os.Open(claudeDir)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
				if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
					t.Fatal(err)
				}
				defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				argv := fx.launchArgs(t, skillglassBinary, true, "", "review", "apply", "--selection", path, "--json")
				command := exec.CommandContext(ctx, argv[0], argv[1:]...)
				command.Env, command.Dir = fx.environment(), fx.project
				var stdout, stderr bytes.Buffer
				command.Stdout, command.Stderr = &stdout, &stderr
				launch := recordLaunch(t, command)
				started := time.Now()
				if err := command.Start(); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- command.Wait() }()
				first := selection.Effects[0].ConfigPath
				committed := false
				for deadline := time.Now().Add(7 * time.Second); time.Now().Before(deadline); {
					raw, err := os.ReadFile(first)
					if err == nil && fmt.Sprintf("%x", sha256.Sum256(raw)) == selection.Effects[0].AfterSHA256 {
						committed = true
						break
					}
					select {
					case err := <-done:
						t.Fatalf("apply ended before first commit: %v %s %s", err, stdout.Bytes(), stderr.Bytes())
					default:
					}
					time.Sleep(10 * time.Millisecond)
				}
				if !committed {
					cancel()
					<-done
					t.Fatal("first native write did not commit")
				}
				switch mutation {
				case "content":
					writeFile(t, first, append(readBytes(t, first), '\n'), 0600)
				case "metadata":
					if err := os.Chmod(first, 0640); err != nil {
						t.Fatal(err)
					}
				case "same-byte-replacement":
					raw := readBytes(t, first)
					if err := os.Rename(first, first+"-external"); err != nil {
						t.Fatal(err)
					}
					writeFile(t, first, raw, 0600)
				case "retarget":
					target, err := os.Readlink(alias)
					if err != nil {
						t.Fatal(err)
					}
					other := filepath.Join(fx.home, "pi-other")
					if err := os.Symlink(target, other); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(alias); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(other, alias); err != nil {
						t.Fatal(err)
					}
				case "new-input":
					writeFile(t, filepath.Join(filepath.Dir(manifests[1]), ".piignore"), []byte("unrelated/\n"), 0600)
				}
				afterExternal := treeManifest(t, fx.base)
				if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
					t.Fatal(err)
				}
				waitErr := <-done
				exit, ok := waitErr.(*exec.ExitError)
				if !ok || exit.ExitCode() != 1 || ctx.Err() != nil || stderr.Len() != 0 {
					t.Fatalf("unexpected apply exit: %v %s %s", waitErr, stdout.Bytes(), stderr.Bytes())
				}
				result := commandResult{code: 1, stdout: stdout.Bytes(), stderr: stderr.Bytes(), duration: time.Since(started), launch: launch}
				recordEvidenceCommand(t, "offline-public-alias-review-with-external-edit", result)
				assertRecordedContext(t, launch, command)
				var out collection.ReviewOutcome
				decodeJSON(t, stdout.Bytes(), &out)
				if out.State != "failed" || len(out.Results) != 3 || out.Results[0].Outcome == nil || out.Results[0].Outcome.WriteState != "committed" || out.Results[2].State != "not-attempted" {
					t.Fatalf("unexpected stopped review: %+v", out)
				}
				if out.Results[1].Outcome != nil && out.Results[1].Outcome.WriteState != "none" {
					t.Fatalf("external change accepted: %+v", out.Results)
				}
				assertTreeUnchanged(t, afterExternal, treeManifest(t, fx.base))
			})
		}
	}
}
