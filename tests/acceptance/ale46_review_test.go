package acceptance_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/alexdancer/skillglass/internal/companion"
)

func ale46Draft(apps []string, scope collection.ControlScope, project string) collection.ReviewDraft {
	return collection.ReviewDraft{SchemaVersion: 1, Context: collection.ReviewContext{Applications: apps, Scope: scope, Project: project, Goal: "lean", KeepEnabled: []collection.SourceID{}}, Sources: []collection.ReviewAssessment{}, Changes: []collection.ReviewRecommendation{}, Unchanged: []collection.ReviewUnchanged{}, Unresolved: []string{}}
}
func ale46Assess(id string, paths ...string) collection.ReviewAssessment {
	return collection.ReviewAssessment{Source: collection.SourceID(id), Assessment: "Instructions and selective supporting evidence fit the user's goal.", SupportingPaths: append([]string{}, paths...), Limitations: []string{}}
}
func ale46Change(id, source, app string, action collection.ControlChoice) collection.ReviewRecommendation {
	return collection.ReviewRecommendation{ID: id, Source: collection.SourceID(source), Application: app, Action: action, Reason: "Explicit user goal and inspected instructions support this availability choice."}
}
func ale46File(t *testing.T, fx fixture, name string, value any) string {
	t.Helper()
	path := filepath.Join(fx.base, "allowed", "review-artifacts", name)
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, raw, 0600)
	return path
}
func ale46Prepare(t *testing.T, fx fixture, path string, want int) collection.ReviewProposal {
	t.Helper()
	r := fx.run(t, "review", "prepare", "--draft", path, "--json")
	assertExit(t, r, want)
	if len(r.stderr) != 0 {
		t.Fatalf("machine stderr: %s", r.stderr)
	}
	var p collection.ReviewProposal
	decodeJSON(t, r.stdout, &p)
	if p.ProposalID == "" || collection.ValidateReviewProposal(p) != nil {
		t.Fatalf("invalid proposal: %s", r.stdout)
	}
	return p
}
func ale46Select(t *testing.T, fx fixture, path string, chosen []string, approval string, want int) commandResult {
	t.Helper()
	args := []string{"review", "select", "--proposal", path, "--json"}
	for _, id := range chosen {
		args = append(args, "--choose", id)
	}
	if approval != "" {
		args = append(args, "--approve", approval)
	}
	result := fx.run(t, args...)
	assertExit(t, result, want)
	if len(result.stderr) != 0 {
		t.Fatalf("machine stderr: %s", result.stderr)
	}
	return result
}

func TestALE46BundledCompanionAndReadOnlySelection(t *testing.T) {
	fx, id, target := ale40Fixture(t, false)
	second := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "second", "ale40-target")
	secondID := changeSourceID(t, fx, second)
	writeFile(t, filepath.Join(filepath.Dir(target), "references/guide.md"), []byte("SHARED_EVIDENCE\n"), 0600)
	writeFile(t, filepath.Join(filepath.Dir(target), "run.sh"), []byte("#!/bin/sh\ntouch "+shellQuote(fx.marker)+"\n"), 0700)
	draft := ale46Draft([]string{"codex", "claude-code"}, collection.ControlGlobal, "")
	draft.Context.KeepEnabled = []collection.SourceID{collection.SourceID(secondID)}
	draft.Sources = []collection.ReviewAssessment{ale46Assess(id, "references/guide.md", "run.sh"), ale46Assess(secondID)}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("codex-choice", id, "codex", collection.ControlDisable), ale46Change("claude-choice", id, "claude-code", collection.ControlDisable)}
	draft.Unchanged = []collection.ReviewUnchanged{{Source: collection.SourceID(secondID), Application: "codex", Reason: "Independent same-name copy is explicitly protected."}}
	draft.Unresolved = []string{"No actual usage data was supplied."}
	path := ale46File(t, fx, "draft.json", draft)
	before := treeManifest(t, fx.base)
	bundleResult := fx.run(t, "companion", "--json")
	assertExit(t, bundleResult, 0)
	var bundle companion.Bundle
	decodeJSON(t, bundleResult.stdout, &bundle)
	if bundle.Name != companion.Name || bundle.Version != "1.0.0" || bundle.ContractVersion != 1 || len(bundle.Files) != 3 {
		t.Fatalf("bundle: %s", bundleResult.stdout)
	}
	for _, entry := range bundle.Files {
		sum := fmt.Sprintf("%x", sha256.Sum256([]byte(entry.Content)))
		if sum != entry.SHA256 {
			t.Fatal("bundle byte identity mismatch")
		}
		emitted := fx.run(t, "companion", "--file", entry.Path)
		assertExit(t, emitted, 0)
		if string(emitted.stdout) != entry.Content {
			t.Fatal("companion file differs from bundle")
		}
	}
	help := fx.run(t, "help", "--json")
	if !bytes.Contains(help.stdout, []byte("review prepare")) || !bytes.Contains(help.stdout, []byte("proposal order")) {
		t.Fatalf("help: %s", help.stdout)
	}
	p := ale46Prepare(t, fx, path, 0)
	if p.State != "prepared" || len(p.Sources) != 2 || len(p.Discovery) != 2 || len(p.Changes) != 2 || len(p.Effects) != 2 || p.Coverage.SelectedEvidence != collection.Complete {
		t.Fatalf("proposal: %+v", p)
	}
	if len(p.Sources[0].Applications) != 2 || p.Sources[0].Source.ID == p.Sources[1].Source.ID {
		t.Fatal("shared/independent identity lost")
	}
	if p.Sources[0].Evidence[1].SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte("SHARED_EVIDENCE\n"))) {
		t.Fatal("evidence is not exact bytes")
	}
	for _, change := range p.Changes {
		if change.Baseline.Confirmation != "" || change.ObservationID == "" || len(change.Consumers) != 7 {
			t.Fatal("native authority leaked or consumers lost")
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	proposalPath := ale46File(t, fx, "proposal.json", p)
	before = treeManifest(t, fx.base)
	result := ale46Select(t, fx, proposalPath, []string{"claude-choice"}, "", 0)
	var selection collection.ReviewSelection
	decodeJSON(t, result.stdout, &selection)
	if selection.State != "preview" || len(selection.Chosen) != 1 || selection.Chosen[0] != "claude-choice" || len(selection.Omitted) != 1 || selection.Omitted[0] != "codex-choice" || len(selection.Effects) != 1 {
		t.Fatalf("subset: %s", result.stdout)
	}
	if collection.ValidateReviewSelection(selection, true) == nil {
		t.Fatal("preview treated as approved")
	}
	ale46Select(t, fx, proposalPath, []string{"claude-choice"}, p.ProposalID, 1)
	approved := ale46Select(t, fx, proposalPath, []string{"claude-choice"}, selection.SelectionID, 0)
	decodeJSON(t, approved.stdout, &selection)
	if err := collection.ValidateReviewSelection(selection, true); err != nil {
		t.Fatal(err)
	}
	reversed := ale46Select(t, fx, proposalPath, []string{"claude-choice", "codex-choice"}, "", 0)
	decodeJSON(t, reversed.stdout, &selection)
	if strings.Join(selection.Chosen, ",") != "codex-choice,claude-choice" {
		t.Fatal("subset reordered proposal")
	}
	empty := ale46Select(t, fx, proposalPath, nil, "", 0)
	decodeJSON(t, empty.stdout, &selection)
	if len(selection.Chosen) != 0 || len(selection.Effects) != 0 || len(selection.Omitted) != 2 {
		t.Fatal("empty selection grants implicit approval")
	}
	ale46Select(t, fx, proposalPath, nil, selection.SelectionID, 0)
	human := fx.run(t, "review", "prepare", "--draft", path)
	assertExit(t, human, 0)
	if !bytes.Contains(human.stdout, []byte("Unchanged")) || !bytes.Contains(human.stdout, []byte("Unresolved")) || !bytes.Contains(human.stdout, []byte("Coverage")) {
		t.Fatal("human summary incomplete")
	}
	writeEvidence(t, "ale46/human-proposal.txt", human.stdout)
	humanSelection := fx.run(t, "review", "select", "--proposal", proposalPath, "--choose", "claude-choice")
	assertExit(t, humanSelection, 0)
	writeEvidence(t, "ale46/human-selection.txt", humanSelection.stdout)
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatal("inspected content executed")
	}
	after := treeManifest(t, fx.base)
	assertTreeUnchanged(t, before, after)
	writeEvidence(t, "ale46/tree-before.json", before)
	writeEvidence(t, "ale46/tree-after.json", after)
}

func TestALE46OrderedEffectsMatchNativeControls(t *testing.T) {
	for _, app := range []string{"codex", "claude-code", "hermes", "pi", "opencode", "gemini"} {
		t.Run(app, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			second := codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "second", "second")
			for _, root := range []string{filepath.Join(fx.home, ".claude/skills"), filepath.Join(fx.home, ".hermes/skills"), filepath.Join(fx.home, ".pi/agent/skills"), filepath.Join(fx.xdgConfig, "opencode/skills"), filepath.Join(fx.home, ".gemini/skills")} {
				if err := os.Symlink(filepath.Dir(second), filepath.Join(root, "second")); err != nil {
					t.Fatal(err)
				}
			}
			secondID := changeSourceID(t, fx, second)
			draft := ale46Draft([]string{app}, collection.ControlGlobal, "")
			draft.Sources = []collection.ReviewAssessment{ale46Assess(id), ale46Assess(secondID)}
			draft.Changes = []collection.ReviewRecommendation{ale46Change("one", id, app, collection.ControlDisable), ale46Change("two", secondID, app, collection.ControlDisable)}
			path := ale46File(t, fx, "draft.json", draft)
			before := treeManifest(t, fx.base)
			proposal := ale46Prepare(t, fx, path, 0)
			if len(proposal.Effects) != 2 || proposal.Effects[0].AfterSHA256 != proposal.Effects[1].BeforeSHA256 {
				t.Fatal("ordered shared-config progression absent")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			proposalPath := ale46File(t, fx, "proposal.json", proposal)
			subset := ale46Select(t, fx, proposalPath, []string{"two"}, "", 0)
			var selection collection.ReviewSelection
			decodeJSON(t, subset.stdout, &selection)
			if len(selection.Effects) != 1 || selection.Effects[0].BeforeSHA256 != proposal.Effects[0].BeforeSHA256 || selection.Effects[0].AfterSHA256 == proposal.Effects[1].AfterSHA256 {
				t.Fatal("subset inherited omitted effect")
			}
			for _, effect := range proposal.Effects {
				previewResult := fx.run(t, "disable", "--agent", app, "--scope", "global", "--json", string(effect.Intent.Source))
				assertExit(t, previewResult, 0)
				var preview collection.ChangeView
				decodeJSON(t, previewResult.stdout, &preview)
				applied := changeCommand(t, fx, "", "disable", "--agent", app, "--scope", "global", "--confirm", preview.Confirmation, "--json", string(effect.Intent.Source))
				assertExit(t, applied, 0)
				var outcome collection.ChangeOutcome
				decodeJSON(t, applied.stdout, &outcome)
				if outcome.ExitCode() != 0 {
					t.Fatalf("native outcome: %s", applied.stdout)
				}
				actual := fmt.Sprintf("%x", sha256.Sum256(readBytes(t, effect.ConfigPath)))
				if actual != effect.AfterSHA256 {
					t.Fatalf("%s pure transform differs from actual writer: %s != %s", app, actual, effect.AfterSHA256)
				}
			}
			if !bytes.Contains(readBytes(t, target), []byte("ale40-target")) {
				t.Fatal("source content changed")
			}
		})
	}
}

func TestALE46IncompleteEvidenceAndProjectScope(t *testing.T) {
	fx, id, target := ale40Fixture(t, false)
	writeFile(t, filepath.Join(filepath.Dir(target), "binary.dat"), []byte{0, 255}, 0600)
	if err := os.Symlink("missing", filepath.Join(filepath.Dir(target), "missing.md")); err != nil {
		t.Fatal(err)
	}
	draft := ale46Draft([]string{"claude-code"}, collection.ControlProject, fx.project)
	draft.Context.Goal = "broad"
	draft.Sources = []collection.ReviewAssessment{ale46Assess(id, "binary.dat", "missing.md")}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("project-choice", id, "claude-code", collection.ControlDisable)}
	draft.Unresolved = []string{"Supporting guide is inaccessible; binary meaning has not been inspected."}
	path := ale46File(t, fx, "project.json", draft)
	before := treeManifest(t, fx.base)
	p := ale46Prepare(t, fx, path, 0)
	if p.Coverage.SelectedEvidence != collection.Incomplete || len(p.Sources[0].Limitations) == 0 || len(p.Sources[0].Evidence[2].Diagnostics) == 0 || p.Effects[0].Intent.Scope != collection.ControlProject || !strings.HasPrefix(p.Effects[0].ConfigPath, fx.project+"/") {
		t.Fatalf("project evidence: %+v", p)
	}
	if p.Draft.Context.Goal != "broad" || !p.Discovery[0].View.Scope.Project.Selected {
		t.Fatal("goal or project lost")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	draft.Context.Applications = []string{"codex"}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("unsupported-project", id, "codex", collection.ControlDisable)}
	path = ale46File(t, fx, "unsupported.json", draft)
	blocked := ale46Prepare(t, fx, path, 1)
	if blocked.State != "blocked" || len(blocked.Effects) != 0 || blocked.Changes[0].Baseline.State != "unsupported" {
		t.Fatal("unsupported project broadened")
	}
	proposalPath := ale46File(t, fx, "blocked.json", blocked)
	before = treeManifest(t, fx.base)
	ale46Select(t, fx, proposalPath, nil, "", 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE46DiscoveryAndUnreviewedCoverageRemainSeparate(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	root := filepath.Join(fx.home, ".agents/skills")
	if err := os.Symlink("missing", filepath.Join(root, "broken-review-reference")); err != nil {
		t.Fatal(err)
	}
	draft := ale46Draft([]string{"codex"}, collection.ControlGlobal, "")
	draft.Sources = []collection.ReviewAssessment{ale46Assess(id)}
	draft.Unchanged = []collection.ReviewUnchanged{{Source: collection.SourceID(id), Application: "codex", Reason: "Retained for occasional research."}}
	draft.Unresolved = []string{"Broken discovery reference remains unresolved."}
	path := ale46File(t, fx, "partial.json", draft)
	before := treeManifest(t, fx.base)
	p := ale46Prepare(t, fx, path, 0)
	if p.Coverage.Discovery != collection.Incomplete || p.Coverage.SelectedEvidence != collection.Complete || len(p.Coverage.Unreviewed) == 0 || len(p.Effects) != 0 {
		t.Fatalf("coverage was overstated: %+v", p.Coverage)
	}
	if len(p.Discovery) != 1 || p.Discovery[0].View.Scope.Completeness != collection.Incomplete {
		t.Fatal("selected application discovery gaps lost")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE46MachineErrorsAndTerminalSafety(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	draft := ale46Draft([]string{"codex"}, collection.ControlGlobal, "")
	draft.Sources = []collection.ReviewAssessment{ale46Assess(id)}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("unsafe-\x1b[2J", id, "codex", collection.ControlDisable)}
	path := ale46File(t, fx, "draft.json", draft)
	p := ale46Prepare(t, fx, path, 0)
	proposalPath := ale46File(t, fx, "proposal.json", p)
	fifo := filepath.Join(fx.base, "allowed", "review.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	for _, args := range [][]string{
		{"review", "--json"}, {"review", "unknown", "--json"}, {"review", "prepare", "--json"},
		{"review", "prepare", "--draft", path, "--json", "--unknown"},
		{"review", "prepare", "--draft", fifo, "--json"},
		{"review", "select", "--proposal", proposalPath, "--json", "--approve", ""},
		{"companion", "--json", "--file", "../SKILL.md"},
	} {
		result := fx.run(t, args...)
		if result.code == 0 || len(result.stderr) != 0 {
			t.Fatalf("machine refusal %v: %d %s %s", args, result.code, result.stdout, result.stderr)
		}
		var failure map[string]any
		decodeJSON(t, result.stdout, &failure)
		if failure["failure"] == nil {
			t.Fatalf("missing failure: %s", result.stdout)
		}
	}
	human := fx.run(t, "review", "select", "--proposal", proposalPath, "--choose", "unsafe-\x1b[2J")
	assertExit(t, human, 0)
	if bytes.Contains(human.stdout, []byte{27}) {
		t.Fatal("terminal escape leaked from change ID")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE46InvalidAlteredAndStaleSelections(t *testing.T) {
	fx, id, target := ale40Fixture(t, false)
	draft := ale46Draft([]string{"codex"}, collection.ControlGlobal, "")
	draft.Sources = []collection.ReviewAssessment{ale46Assess(id)}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("one", id, "codex", collection.ControlDisable)}
	path := ale46File(t, fx, "valid.json", draft)
	p := ale46Prepare(t, fx, path, 0)
	proposalPath := ale46File(t, fx, "proposal.json", p)
	before := treeManifest(t, fx.base)
	for _, chosen := range [][]string{{"unknown"}, {"one", "one"}} {
		ale46Select(t, fx, proposalPath, chosen, "", 1)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	for _, scenario := range []string{"goal", "applications", "scope", "protected", "duplicate", "contradiction", "missing-assessment", "action", "version"} {
		t.Run(scenario, func(t *testing.T) {
			raw, _ := json.Marshal(draft)
			var bad collection.ReviewDraft
			decodeJSON(t, raw, &bad)
			switch scenario {
			case "goal":
				bad.Context.Goal = ""
			case "applications":
				bad.Context.Applications = nil
			case "scope":
				bad.Context.Scope = ""
			case "protected":
				bad.Context.KeepEnabled = []collection.SourceID{collection.SourceID(id)}
			case "duplicate":
				bad.Changes = append(bad.Changes, bad.Changes[0])
			case "contradiction":
				bad.Unchanged = append(bad.Unchanged, collection.ReviewUnchanged{Source: collection.SourceID(id), Application: "codex", Reason: "Conflict"})
			case "missing-assessment":
				bad.Sources = []collection.ReviewAssessment{}
			case "action":
				bad.Changes[0].Action = "delete"
			case "version":
				bad.SchemaVersion = 2
			}
			badPath := ale46File(t, fx, "bad-"+scenario+".json", bad)
			baseline := treeManifest(t, fx.base)
			r := fx.run(t, "review", "prepare", "--draft", badPath, "--json")
			assertExit(t, r, 2)
			var failure map[string]any
			decodeJSON(t, r.stdout, &failure)
			if failure["state"] != "refused" || failure["failure"] == nil {
				t.Fatalf("failure: %s", r.stdout)
			}
			assertTreeUnchanged(t, baseline, treeManifest(t, fx.base))
		})
	}
	for _, raw := range []string{`{"schema_version":1,"schema_version":1}`, `{"schema_version":1,"unknown":true}`, `{} {}`, `null`} {
		path := filepath.Join(fx.base, "allowed", "invalid-raw.json")
		writeFile(t, path, []byte(raw), 0600)
		baseline := treeManifest(t, fx.base)
		r := fx.run(t, "review", "prepare", "--draft", path, "--json")
		assertExit(t, r, 2)
		assertTreeUnchanged(t, baseline, treeManifest(t, fx.base))
	}
	altered := p
	altered.Draft.Context.Goal = "broad"
	alteredPath := ale46File(t, fx, "altered.json", altered)
	before = treeManifest(t, fx.base)
	ale46Select(t, fx, alteredPath, []string{"one"}, "", 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	for _, scenario := range []string{"config", "evidence", "source"} {
		t.Run(scenario, func(t *testing.T) {
			current := ale46Prepare(t, fx, pathForDraft(t, fx, draft), 0)
			currentPath := ale46File(t, fx, "current-proposal.json", current)
			switch scenario {
			case "config":
				writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte("# external edit\n"), 0600)
			case "evidence":
				writeFile(t, target, append(readBytes(t, target), []byte("\nExternal edit")...), 0600)
			case "source":
				if err := os.Rename(filepath.Dir(target), filepath.Dir(target)+"-old"); err != nil {
					t.Fatal(err)
				}
				writeSkill(t, filepath.Dir(target), "ale40-target", "replacement", "Replacement")
			}
			baseline := treeManifest(t, fx.base)
			ale46Select(t, fx, currentPath, []string{"one"}, "", 1)
			assertTreeUnchanged(t, baseline, treeManifest(t, fx.base))
		})
	}
}
func pathForDraft(t *testing.T, fx fixture, draft collection.ReviewDraft) string {
	return ale46File(t, fx, "fresh-draft.json", draft)
}
