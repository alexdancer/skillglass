package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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

func ale47Approved(t *testing.T, fx fixture, draft collection.ReviewDraft, chosen []string) (collection.ReviewSelection, string) {
	t.Helper()
	p := ale46Prepare(t, fx, ale46File(t, fx, "draft.json", draft), 0)
	proposal := ale46File(t, fx, "proposal.json", p)
	var selection collection.ReviewSelection
	decodeJSON(t, ale46Select(t, fx, proposal, chosen, "", 0).stdout, &selection)
	decodeJSON(t, ale46Select(t, fx, proposal, chosen, selection.SelectionID, 0).stdout, &selection)
	return selection, ale46File(t, fx, "approved.json", selection)
}
func ale47Apply(t *testing.T, fx fixture, path, policy string, want int) collection.ReviewOutcome {
	t.Helper()
	r := changeCommand(t, fx, policy, "review", "apply", "--selection", path, "--json")
	assertExit(t, r, want)
	if len(r.stderr) != 0 {
		t.Fatalf("machine stderr: %s", r.stderr)
	}
	var out collection.ReviewOutcome
	decodeJSON(t, r.stdout, &out)
	if out.Kind != "skillglass-review-outcome" || out.SchemaVersion != 1 || out.ExitCode() != want {
		t.Fatalf("outcome: %s", r.stdout)
	}
	return out
}
func ale47Draft(t *testing.T, fx fixture, first string, apps []string) (collection.ReviewDraft, string) {
	t.Helper()
	second := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "ale47-second", "ale47-second")
	for _, root := range []string{filepath.Join(fx.home, ".claude/skills"), filepath.Join(fx.home, ".hermes/skills"), filepath.Join(fx.home, ".pi/agent/skills"), filepath.Join(fx.xdgConfig, "opencode/skills"), filepath.Join(fx.home, ".gemini/skills")} {
		if err := os.Symlink(filepath.Dir(second), filepath.Join(root, "ale47-second")); err != nil {
			t.Fatal(err)
		}
	}
	id := changeSourceID(t, fx, second)
	draft := ale46Draft(apps, collection.ControlGlobal, "")
	draft.Sources = []collection.ReviewAssessment{ale46Assess(first), ale46Assess(id)}
	for _, app := range apps {
		draft.Changes = append(draft.Changes, ale46Change(app+"-first", first, app, collection.ControlDisable), ale46Change(app+"-second", id, app, collection.ControlDisable))
	}
	return draft, second
}
func TestALE47ApprovedSubsetAndNoOp(t *testing.T) {
	fx, id, target := ale40Fixture(t, false)
	draft, second := ale47Draft(t, fx, id, []string{"codex", "claude-code"})
	// Keep an independent same-name source enabled and preserve all content.
	copyManifest := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "independent", "ale40-target")
	copyID := changeSourceID(t, fx, copyManifest)
	draft.Context.KeepEnabled = []collection.SourceID{collection.SourceID(copyID)}
	draft.Sources = append(draft.Sources, ale46Assess(copyID))
	bodies := [][]byte{readBytes(t, target), readBytes(t, second), readBytes(t, copyManifest)}
	selection, path := ale47Approved(t, fx, draft, []string{"claude-code-second", "codex-second"})
	before := treeManifest(t, fx.base)
	out := ale47Apply(t, fx, path, "", 0)
	if out.State != "applied" || len(out.Results) != 2 || len(out.Omitted) != 2 || out.SelectionID != selection.SelectionID || len(out.Reinspection) != 2 {
		t.Fatalf("%+v", out)
	}
	for _, r := range out.Results {
		if r.State != "succeeded" || r.Outcome == nil || r.Outcome.Persistence != "verified" || r.Outcome.Achievement != "achieved" || r.Final == nil || r.Final.State != collection.AvailabilityDisabled || r.FinalObservation != "observed" || r.Outcome.Reload.State == "" {
			t.Fatalf("%+v", r)
		}
	}
	for i, p := range []string{target, second, copyManifest} {
		if !bytes.Equal(bodies[i], readBytes(t, p)) {
			t.Fatal("source content changed")
		}
	}
	if !bytes.Contains(readBytes(t, filepath.Join(fx.home, ".codex/config.toml")), []byte("# keep")) || !bytes.Contains(readBytes(t, filepath.Join(fx.home, ".claude/settings.json")), []byte(`"keep":true`)) {
		t.Fatal("unrelated settings lost")
	}
	for _, app := range []string{"codex", "claude-code"} {
		r := fx.run(t, "list", "--agent", app, "--json")
		var view collection.View
		decodeJSON(t, r.stdout, &view)
		for _, source := range view.Sources {
			if string(source.ID) == id || string(source.ID) == copyID {
				if source.Availability.State != collection.AvailabilityAvailable {
					t.Fatal("omitted or independent source changed", source.ID, source.Availability.State)
				}
			}
		}
	}
	writeEvidence(t, "ale47/subset-before.json", before)
	writeEvidence(t, "ale47/subset-after.json", treeManifest(t, fx.base))
	// Original approval is stale after successful writes, even with desired state saved.
	stable := treeManifest(t, fx.base)
	ale47Apply(t, fx, path, "", 1)
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
	_, path = ale47Approved(t, fx, draft, []string{"codex-second"})
	stable = treeManifest(t, fx.base)
	out = ale47Apply(t, fx, path, "", 0)
	if out.Results[0].State != "no-op" || out.Results[0].Outcome.WriteState != "none" {
		t.Fatalf("%+v", out.Results)
	}
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
	_, path = ale47Approved(t, fx, draft, nil)
	stable = treeManifest(t, fx.base)
	out = ale47Apply(t, fx, path, "", 0)
	if len(out.Results) != 0 || len(out.Omitted) != 4 {
		t.Fatal("empty selection implied actions")
	}
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
}
func TestALE47SharedConfigurationProgression(t *testing.T) {
	for _, app := range []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini"} {
		t.Run(app, func(t *testing.T) {
			fx, id, _ := ale40Fixture(t, false)
			draft, _ := ale47Draft(t, fx, id, []string{app})
			selection, path := ale47Approved(t, fx, draft, []string{app + "-second", app + "-first"})
			if selection.Effects[0].AfterSHA256 != selection.Effects[1].BeforeSHA256 {
				t.Fatal("missing ordered effects")
			}
			out := ale47Apply(t, fx, path, "", 0)
			for _, r := range out.Results {
				if r.State != "succeeded" || r.Final == nil || r.Final.State != collection.AvailabilityDisabled {
					t.Fatalf("%+v", out)
				}
			}
			actual := fmt.Sprintf("%x", sha256.Sum256(readBytes(t, selection.Effects[1].ConfigPath)))
			if actual != selection.Effects[1].AfterSHA256 {
				t.Fatal("final bytes differ from approved effects")
			}
		})
	}
}
func TestALE47WholeSelectionRefusalsWriteNothing(t *testing.T) {
	for _, scenario := range []string{"unapproved", "altered", "configuration", "same-byte-replacement", "evidence", "identity", "new-consumer", "unsupported"} {
		t.Run(scenario, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			writeFile(t, filepath.Join(filepath.Dir(target), "guide.md"), []byte("approved evidence"), 0600)
			draft, _ := ale47Draft(t, fx, id, []string{"codex", "claude-code"})
			draft.Sources[0].SupportingPaths = []string{"guide.md"}
			selection, path := ale47Approved(t, fx, draft, []string{"codex-first", "claude-code-second"})
			switch scenario {
			case "unapproved":
				selection.State = "preview"
				path = ale46File(t, fx, "unapproved.json", selection)
			case "altered":
				selection.Effects[1].AfterSHA256 = "changed"
				path = ale46File(t, fx, "altered.json", selection)
			case "configuration":
				writeFile(t, selection.Effects[1].ConfigPath, []byte(`{"keep":true,"external":true}`), 0600)
			case "same-byte-replacement":
				p := selection.Effects[1].ConfigPath
				raw := readBytes(t, p)
				if err := os.Rename(p, p+"-saved"); err != nil {
					t.Fatal(err)
				}
				writeFile(t, p, raw, 0600)
			case "evidence":
				writeFile(t, filepath.Join(filepath.Dir(target), "guide.md"), []byte("external evidence"), 0600)
			case "identity":
				if err := os.Rename(filepath.Dir(target), filepath.Dir(target)+"-saved"); err != nil {
					t.Fatal(err)
				}
				writeSkill(t, filepath.Dir(target), "ale40-target", "replacement", "Replacement")
			case "new-consumer":
				if err := os.Symlink(filepath.Dir(target), filepath.Join(fx.home, ".claude/skills/new-reference")); err != nil {
					t.Fatal(err)
				}
			case "unsupported":
				writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte(`{"permissions":{"deny":["Skill(*)"]}}`), 0600)
			}
			before := treeManifest(t, fx.base)
			out := ale47Apply(t, fx, path, "", 1)
			if out.State != "refused" {
				t.Fatalf("%+v", out)
			}
			for _, r := range out.Results {
				if r.State != "not-attempted" || r.Outcome != nil {
					t.Fatal("partial subset was applied")
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
	fx, id, _ := ale40Fixture(t, false)
	draft := ale46Draft([]string{"codex"}, collection.ControlGlobal, "")
	draft.Sources = []collection.ReviewAssessment{ale46Assess(id)}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("disable", id, "codex", collection.ControlDisable), ale46Change("enable", id, "codex", collection.ControlEnable)}
	path := ale46File(t, fx, "contradictory.json", draft)
	before := treeManifest(t, fx.base)
	r := fx.run(t, "review", "prepare", "--draft", path, "--json")
	assertExit(t, r, 2)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	draft.Context.Scope, draft.Context.Project = collection.ControlProject, fx.project
	draft.Context.Applications = []string{"codex", "claude-code"}
	draft.Changes = append(draft.Changes[:1], ale46Change("supported", id, "claude-code", collection.ControlDisable))
	unsupportedPath := ale46File(t, fx, "unsupported-draft.json", draft)
	before = treeManifest(t, fx.base)
	p := ale46Prepare(t, fx, unsupportedPath, 1)
	if p.State != "blocked" || len(p.Effects) != 0 || p.Changes[0].Baseline.State != "unsupported" {
		t.Fatal("unsupported scope broadened")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	proposalPath := ale46File(t, fx, "unsupported-proposal.json", p)
	before = treeManifest(t, fx.base)
	ale46Select(t, fx, proposalPath, []string{"supported"}, "", 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE47PartialFailureAndFinalReinspection(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	draft, _ := ale47Draft(t, fx, id, []string{"codex", "claude-code", "hermes"})
	selection, path := ale47Approved(t, fx, draft, []string{"codex-first", "codex-second", "claude-code-first", "hermes-first"})
	laterBefore := readBytes(t, filepath.Join(fx.home, ".hermes/config.yaml"))
	policy := "(deny file-write* (subpath " + sandboxString(filepath.Join(fx.home, ".claude")) + "))"
	out := ale47Apply(t, fx, path, policy, 1)
	if out.State != "failed" || len(out.Results) != 4 || out.Results[0].State != "succeeded" || out.Results[1].State != "succeeded" || out.Results[2].State != "failed" || out.Results[3].State != "not-attempted" {
		t.Fatalf("%+v", out)
	}
	if out.Results[2].Outcome == nil || out.Results[2].Outcome.WriteState != "none" || out.Results[3].Outcome != nil {
		t.Fatal("untruthful failure receipt")
	}
	for _, r := range out.Results {
		if r.Final == nil || r.FinalObservation != "observed" {
			t.Fatal("final reinspection omitted", r)
		}
	}
	if !bytes.Equal(laterBefore, readBytes(t, filepath.Join(fx.home, ".hermes/config.yaml"))) {
		t.Fatal("later file touched")
	}
	if fmt.Sprintf("%x", sha256.Sum256(readBytes(t, selection.Effects[1].ConfigPath))) != selection.Effects[1].AfterSHA256 {
		t.Fatal("earlier writes rolled back")
	}
	writeEvidence(t, "ale47/partial-outcome.json", mustJSON(t, out))
}
func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestALE47ProjectAndHumanReceipts(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	draft := ale46Draft([]string{"claude-code"}, collection.ControlProject, fx.project)
	draft.Sources = []collection.ReviewAssessment{ale46Assess(id)}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("project-\x1b[2J", id, "claude-code", collection.ControlDisable)}
	_, path := ale47Approved(t, fx, draft, []string{"project-\x1b[2J"})
	globalBefore := readBytes(t, filepath.Join(fx.home, ".claude/settings.json"))
	result := changeCommand(t, fx, "", "review", "apply", "--selection", path)
	assertExit(t, result, 0)
	if bytes.Contains(result.stdout, []byte{27}) || !bytes.Contains(result.stdout, []byte("Persistence: verified")) || !bytes.Contains(result.stdout, []byte("Final reinspection: observed")) || !strings.Contains(string(result.stdout), fx.project) {
		t.Fatalf("%s", result.stdout)
	}
	if !bytes.Equal(globalBefore, readBytes(t, filepath.Join(fx.home, ".claude/settings.json"))) {
		t.Fatal("project changed global settings")
	}
	writeEvidence(t, "ale47/human-outcome.txt", result.stdout)
}

// The controller holds the second native configuration directory lock. After
// the first real commit, it makes a genuine edit and releases the writer. There
// is no production fault switch or synthetic freshness bypass.
func TestALE47ExternalEditsDuringBatch(t *testing.T) {
	for _, scenario := range []string{"saved-config", "same-byte-config", "evidence", "source-identity", "new-reference"} {
		t.Run(scenario, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			guide := filepath.Join(filepath.Dir(target), "guide.md")
			writeFile(t, guide, []byte("initial reviewed evidence"), 0600)
			draft, _ := ale47Draft(t, fx, id, []string{"codex", "claude-code", "hermes"})
			draft.Sources[0].SupportingPaths = []string{"guide.md"}
			selection, path := ale47Approved(t, fx, draft, []string{"codex-first", "claude-code-first", "hermes-first"})
			lock, err := os.Open(filepath.Join(fx.home, ".claude"))
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
			command.Env = fx.environment()
			command.Dir = fx.project
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			launch := recordLaunch(t, command)
			started := time.Now()
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()
			first := selection.Effects[0].ConfigPath
			deadline := time.Now().Add(7 * time.Second)
			committed := false
			for time.Now().Before(deadline) {
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
			switch scenario {
			case "saved-config":
				writeFile(t, first, append(readBytes(t, first), []byte("\n# unrelated external edit\n")...), 0600)
			case "same-byte-config":
				raw := readBytes(t, first)
				if err := os.Rename(first, first+"-external"); err != nil {
					t.Fatal(err)
				}
				writeFile(t, first, raw, 0600)
			case "evidence":
				writeFile(t, guide, []byte("changed reviewed evidence"), 0600)
			case "source-identity":
				if err := os.Rename(filepath.Dir(target), filepath.Dir(target)+"-external"); err != nil {
					t.Fatal(err)
				}
				writeSkill(t, filepath.Dir(target), "ale40-target", "replacement", "Replacement")
			case "new-reference":
				if err := os.Symlink(filepath.Dir(target), filepath.Join(fx.home, ".claude/skills/new-reference")); err != nil {
					t.Fatal(err)
				}
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
			recordEvidenceCommand(t, "offline-public-mutation-with-external-edit", result)
			assertRecordedContext(t, launch, command)
			var out collection.ReviewOutcome
			decodeJSON(t, stdout.Bytes(), &out)
			if out.State != "failed" || out.Results[0].Outcome == nil || out.Results[0].Outcome.WriteState != "committed" || out.Results[2].State != "not-attempted" {
				t.Fatalf("%+v", out)
			}
			// Depending on the controller/writer interleaving, either the first
			// item's post-inspection or the next immediate guard detects the edit.
			// In both cases no further native availability write may happen.
			if out.Results[1].Outcome != nil && out.Results[1].Outcome.WriteState != "none" {
				t.Fatalf("external change accepted: %+v", out.Results)
			}
			assertTreeUnchanged(t, afterExternal, treeManifest(t, fx.base))
			writeEvidence(t, "ale47/external-"+scenario+".json", stdout.Bytes())
		})
	}
}
func TestALE47MissingDirectoryAndIncompleteEvidence(t *testing.T) {
	fx, id, target := ale40Fixture(t, false)
	// Preserve the baseline private skills while making the native home absent.
	if err := os.Rename(filepath.Join(fx.home, ".codex"), filepath.Join(fx.home, ".codex-saved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("absent", filepath.Join(filepath.Dir(target), "missing.md")); err != nil {
		t.Fatal(err)
	}
	draft, _ := ale47Draft(t, fx, id, []string{"codex"})
	draft.Sources[0].SupportingPaths = []string{"missing.md", "../outside.md"}
	selection, path := ale47Approved(t, fx, draft, []string{"codex-first", "codex-second"})
	if selection.Proposal.Coverage.SelectedEvidence != collection.Incomplete || selection.Effects[0].CreateDirectory == "" {
		t.Fatal("fixture did not expose missing directory and incomplete evidence")
	}
	out := ale47Apply(t, fx, path, "", 0)
	if len(out.Results) != 2 || out.Results[1].State != "succeeded" {
		t.Fatalf("%+v", out)
	}
}
