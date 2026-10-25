package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86RecoveryJournalEncodedLimit(t *testing.T) {
	t.Run("oversized-plan-refuses-before-effects", func(t *testing.T) {
		fx, sourcePath, library, draftPath, preview := journalLimitFixture(t, strings.Repeat("R", 5<<20), "journal-size/large")
		if preview.State != "ready" {
			t.Fatalf("large reason did not reach the apply boundary: state=%s failure=%+v", preview.State, preview.Failure)
		}
		before := treeManifest(t, fx.base)
		apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
		writeEvidence(t, "journal-size/large-apply.json", apply.stdout)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &outcome)
		if apply.code != 1 || outcome.State != "refused" || outcome.Failure == nil || outcome.Failure.Code != "journal-preflight" || !strings.Contains(outcome.Failure.Message, "4 MiB encoded limit") || outcome.RecoveryID != "" {
			t.Fatalf("oversized encoded journal did not refuse before apply: exit=%d state=%s failure=%+v recovery_id=%q", apply.code, outcome.State, outcome.Failure, outcome.RecoveryID)
		}
		if !bytes.Equal(before, treeManifest(t, fx.base)) {
			t.Fatal("oversized recovery journal changed fixture files before refusal")
		}
		planDir := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", preview.RecoveryID)
		if _, err := os.Stat(planDir); !os.IsNotExist(err) {
			t.Fatalf("oversized recovery plan directory remained after refusal: %v", err)
		}
		if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
			t.Fatalf("source did not remain at its original path: %v", err)
		}
	})

	t.Run("small-plan-applies-and-recovers", func(t *testing.T) {
		fx, _, library, draftPath, preview := journalLimitFixture(t, "A bounded fixture reason for Codex.", "journal-size/small")
		piRoot := filepath.Join(fx.home, ".pi", "agent", "skills")
		codexRoot := filepath.Join(fx.home, ".codex", "skills")
		beforePi := treeManifest(t, piRoot)
		beforeCodex := treeManifest(t, codexRoot)
		beforeLibrary := treeManifest(t, library)
		apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
		writeEvidence(t, "journal-size/small-apply.json", apply.stdout)
		assertExit(t, apply, 0)
		var applied collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &applied)
		if applied.State != "applied" || !applied.Verified || applied.RecoveryID == "" {
			t.Fatalf("small recovery journal did not apply: state=%s verified=%t recovery_id=%q", applied.State, applied.Verified, applied.RecoveryID)
		}
		journalPath := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", applied.RecoveryID, "record.json")
		record, err := os.ReadFile(journalPath)
		if err != nil || len(record) > 4<<20 {
			t.Fatalf("applied journal is not within its reader bound: bytes=%d err=%v", len(record), err)
		}
		var journalState struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(record, &journalState); err != nil || journalState.State != "applied" {
			t.Fatalf("persisted journal state is unreadable: state=%q err=%v", journalState.State, err)
		}
		writeEvidence(t, "journal-size/small-record.json", record)

		recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", applied.RecoveryID, "--json")
		writeEvidence(t, "journal-size/small-recovery-preview.json", recoveryPreviewResult.stdout)
		assertExit(t, recoveryPreviewResult, 0)
		var recoveryPreview collection.ConsolidationPreview
		decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
		if recoveryPreview.State != "ready" || recoveryPreview.Confirmation == "" {
			t.Fatalf("bounded applied journal was not readable for recovery: state=%s failure=%+v", recoveryPreview.State, recoveryPreview.Failure)
		}
		recover := changeCommand(t, fx, "", "consolidate", "recover", "--backup", applied.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
		writeEvidence(t, "journal-size/small-recovery-apply.json", recover.stdout)
		assertExit(t, recover, 0)
		var recovered collection.ConsolidationApplyOutcome
		decodeJSON(t, recover.stdout, &recovered)
		if recovered.State != "recovered" || !recovered.Verified {
			t.Fatalf("public recovery did not finish: state=%s verified=%t failure=%+v", recovered.State, recovered.Verified, recovered.Failure)
		}
		if !bytes.Equal(beforePi, treeManifest(t, piRoot)) || !bytes.Equal(beforeCodex, treeManifest(t, codexRoot)) || !bytes.Equal(beforeLibrary, treeManifest(t, library)) {
			t.Fatal("public recovery did not restore the original Pi, Codex, and library trees")
		}
	})
}

func journalLimitFixture(t *testing.T, reason, evidence string) (fixture, string, string, string, collection.ConsolidationPreview) {
	t.Helper()
	fx, sourcePath, library := recoveryFixture(t)
	if err := os.MkdirAll(library, 0o700); err != nil {
		t.Fatal(err)
	}
	inventory := recoveryScan(t, fx, library, evidence+"-scan.json")
	draft := recoveryDraft(t, inventory, sourcePath)
	found := false
	for i := range draft.Choices {
		for j := range draft.Choices[i].Destinations {
			if draft.Choices[i].Destinations[j].Application == "codex" && draft.Choices[i].Action == "share" {
				draft.Choices[i].Destinations[j].Reason = reason
				found = true
			}
		}
	}
	if !found {
		t.Fatal("fixture draft has no Codex destination reason")
	}
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "journal-size-draft.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, evidence+"-preview.json", previewResult.stdout)
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" || preview.Confirmation == "" || preview.RecoveryID == "" {
		t.Fatalf("fixture preview was not ready: state=%s failure=%+v", preview.State, preview.Failure)
	}
	return fx, sourcePath, library, draftPath, preview
}

func TestALE86ConsolidationHelpTokensRespectOptionValues(t *testing.T) {
	libraryFixture := newFixture(t)
	writeFile(t, filepath.Join(libraryFixture.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: help-value scan fixture\n---\nFixture only.\n"), 0o644)
	libraryNames := []string{"help", "--help", "-h", "ordinary-library"}
	for _, name := range libraryNames {
		if err := os.MkdirAll(filepath.Join(libraryFixture.project, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	libraryBefore := treeManifest(t, libraryFixture.base)
	libraryCases := []struct {
		args []string
		name string
	}{
		{[]string{"--library", "help"}, "help"},
		{[]string{"--library", "--help"}, "--help"},
		{[]string{"--library=--help"}, "--help"},
		{[]string{"--library", "-h"}, "-h"},
		{[]string{"--library=-h"}, "-h"},
		{[]string{"--library", "ordinary-library"}, "ordinary-library"},
	}
	for _, testCase := range libraryCases {
		args := append([]string{"consolidate", "scan", "--app", "codex"}, testCase.args...)
		args = append(args, "--json")
		result := libraryFixture.run(t, args...)
		writeEvidence(t, "help-flags/library-"+strings.TrimPrefix(strings.ReplaceAll(testCase.name, "-", "dash"), "")+".json", result.stdout)
		assertExit(t, result, 0)
		var inventory collection.ConsolidationInventory
		decodeJSON(t, result.stdout, &inventory)
		if inventory.Operation != "consolidate-scan" || inventory.Outcome != "success" || filepath.Clean(inventory.Scope.Library) != filepath.Join(libraryFixture.project, testCase.name) {
			t.Fatalf("library token %q was not parsed as a value: operation=%s outcome=%s path=%q", testCase.name, inventory.Operation, inventory.Outcome, inventory.Scope.Library)
		}
	}
	if !bytes.Equal(libraryBefore, treeManifest(t, libraryFixture.base)) {
		t.Fatal("library scans changed the sealed fixture")
	}

	draftFixture, _, draftLibrary := recoveryFixture(t)
	if err := os.MkdirAll(draftLibrary, 0o700); err != nil {
		t.Fatal(err)
	}
	inventory := recoveryScan(t, draftFixture, draftLibrary, "help-flags/draft-scan.json")
	draft := recoveryDraft(t, inventory, filepath.Join(draftFixture.home, ".pi", "agent", "skills", "portable-probe"))
	draftNames := []string{"help", "--help", "-h", "ordinary-draft.json"}
	for _, name := range draftNames {
		writeConsolidationDraft(t, filepath.Join(draftFixture.project, name), draft)
	}
	draftBefore := treeManifest(t, draftFixture.base)
	for _, testCase := range []struct {
		args []string
		name string
	}{
		{[]string{"--draft", "help"}, "help"},
		{[]string{"--draft", "--help"}, "--help"},
		{[]string{"--draft=--help"}, "--help"},
		{[]string{"--draft", "-h"}, "-h"},
		{[]string{"--draft=-h"}, "-h"},
		{[]string{"--draft", "ordinary-draft.json"}, "ordinary-draft.json"},
	} {
		args := append([]string{"consolidate", "preview"}, testCase.args...)
		args = append(args, "--json")
		result := draftFixture.run(t, args...)
		writeEvidence(t, "help-flags/preview-"+testCase.name+".json", result.stdout)
		assertExit(t, result, 0)
		var preview collection.ConsolidationPreview
		decodeJSON(t, result.stdout, &preview)
		if preview.Operation != "consolidate-preview" || preview.State != "ready" || preview.Confirmation == "" {
			t.Fatalf("draft token %q did not reach preview: operation=%s state=%s failure=%+v", testCase.name, preview.Operation, preview.State, preview.Failure)
		}
	}
	for _, testCase := range []struct {
		args []string
		name string
	}{
		{[]string{"--draft", "--help"}, "--help"},
		{[]string{"--draft=--help"}, "--help"},
		{[]string{"--draft", "-h"}, "-h"},
		{[]string{"--draft=-h"}, "-h"},
	} {
		args := append([]string{"consolidate", "apply"}, testCase.args...)
		args = append(args, "--confirm", "not-the-preview-token", "--json")
		result := draftFixture.run(t, args...)
		writeEvidence(t, "help-flags/apply-"+strings.ReplaceAll(testCase.name, "-", "dash")+".json", result.stdout)
		if result.code != 1 {
			t.Fatalf("apply draft value %q did not reach validation: exit=%d", testCase.name, result.code)
		}
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, result.stdout, &outcome)
		if outcome.Operation != "consolidate-apply" || outcome.Failure == nil || outcome.Failure.Code != "confirmation-mismatch" {
			t.Fatalf("apply draft value %q was not parsed: operation=%s failure=%+v", testCase.name, outcome.Operation, outcome.Failure)
		}
	}
	for _, testCase := range []struct {
		args []string
		name string
	}{
		{[]string{"--backup", "--help"}, "--help"},
		{[]string{"--backup=--help"}, "--help"},
		{[]string{"--backup", "-h"}, "-h"},
		{[]string{"--backup=-h"}, "-h"},
	} {
		args := append([]string{"consolidate", "recover"}, testCase.args...)
		args = append(args, "--json")
		result := draftFixture.run(t, args...)
		writeEvidence(t, "help-flags/recover-"+strings.ReplaceAll(testCase.name, "-", "dash")+".json", result.stdout)
		if result.code != 1 {
			t.Fatalf("recover backup value %q did not reach validation: exit=%d", testCase.name, result.code)
		}
		var preview collection.ConsolidationPreview
		decodeJSON(t, result.stdout, &preview)
		if preview.State != "refused" || preview.Failure == nil {
			t.Fatalf("recover backup value %q was not parsed: operation=%s state=%s failure=%+v", testCase.name, preview.Operation, preview.State, preview.Failure)
		}
	}
	if !bytes.Equal(draftBefore, treeManifest(t, draftFixture.base)) {
		t.Fatal("preview and refused mutation commands changed the sealed fixture")
	}

	for _, args := range [][]string{
		{"consolidate", "--help"},
		{"consolidate", "help"},
		{"consolidate", "scan", "--help"},
		{"consolidate", "scan", "-h"},
		{"consolidate", "preview", "--help"},
		{"consolidate", "preview", "-h"},
		{"consolidate", "apply", "--help"},
		{"consolidate", "apply", "-h"},
		{"consolidate", "recover", "--help"},
		{"consolidate", "recover", "-h"},
	} {
		result := draftFixture.run(t, args...)
		if result.code != 0 || !strings.Contains(string(result.stdout), "Usage:") {
			t.Fatalf("true help invocation %v changed behavior: exit=%d stdout=%q", args, result.code, result.stdout)
		}
	}
	machineHelp := draftFixture.run(t, "consolidate", "scan", "--help", "--json")
	if machineHelp.code != 0 {
		t.Fatalf("machine help failed: exit=%d", machineHelp.code)
	}
	var help struct {
		Operation string `json:"operation"`
		Help      string `json:"help"`
	}
	decodeJSON(t, machineHelp.stdout, &help)
	if help.Operation != "consolidate-help" || !strings.Contains(help.Help, "Usage:") {
		t.Fatalf("machine help output changed: operation=%s", help.Operation)
	}

	for _, args := range [][]string{
		{"consolidate", "scan", "--app"},
		{"consolidate", "preview", "--draft"},
		{"consolidate", "apply", "--confirm"},
		{"consolidate", "recover", "--backup"},
	} {
		result := draftFixture.run(t, args...)
		if result.code != 2 || !strings.Contains(string(result.stderr), "argument") {
			t.Fatalf("missing option value %v was not rejected normally: exit=%d stderr=%q", args, result.code, result.stderr)
		}
	}
	for _, operation := range []string{"scan", "preview", "apply", "recover"} {
		result := draftFixture.run(t, "consolidate", operation, "--unknown")
		if result.code != 2 || !strings.Contains(string(result.stderr), "flag provided but not defined") {
			t.Fatalf("unknown flag for %s was not rejected normally: exit=%d stderr=%q", operation, result.code, result.stderr)
		}
	}
}
