package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

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
