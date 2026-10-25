package acceptance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86PendingRecoveryLinkMustBeAbsentOrRecordedSource(t *testing.T) {
	t.Run("occupied pending link is refused and preserved", func(t *testing.T) {
		fx := newFixture(t)
		recovery := prepareALE86Recovery(t, fx, false)
		piLink := filepath.Join(fx.home, ".pi", "agent", "skills", "retry-source")
		preparePendingRecoveryLink(t, fx, recovery.recoveryID, piLink)
		if err := os.Symlink(recovery.canonical, piLink); err != nil {
			t.Fatal(err)
		}
		before := treeManifest(t, fx.base)

		result := fx.run(t, "consolidate", "recover", "--backup", recovery.recoveryID, "--json")
		assertExit(t, result, 1)
		var preview collection.ConsolidationPreview
		decodeJSON(t, result.stdout, &preview)
		if preview.State != "refused" || preview.Failure == nil || preview.Failure.Code != "recovery-freshness" {
			t.Fatalf("occupied pending recovery path was not refused as stale: %#v", preview)
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		if target, err := os.Readlink(piLink); err != nil || filepath.Clean(target) != filepath.Clean(recovery.canonical) {
			t.Fatalf("recovery refusal did not preserve the later symlink: target=%q err=%v", target, err)
		}
		if _, err := os.Stat(recovery.canonical); err != nil {
			t.Fatalf("canonical bundle changed on refusal: %v", err)
		}
	})

	t.Run("absent pending link completes recovery", func(t *testing.T) {
		fx := newFixture(t)
		recovery := prepareALE86Recovery(t, fx, false)
		piLink := filepath.Join(fx.home, ".pi", "agent", "skills", "retry-source")
		journalPath := preparePendingRecoveryLink(t, fx, recovery.recoveryID, piLink)

		previewResult := fx.run(t, "consolidate", "recover", "--backup", recovery.recoveryID, "--json")
		assertExit(t, previewResult, 0)
		var preview collection.ConsolidationPreview
		decodeJSON(t, previewResult.stdout, &preview)
		if preview.State != "ready" || preview.Confirmation == "" {
			t.Fatalf("absent pending path blocked recovery: %#v", preview)
		}
		apply := changeCommand(t, fx, "", "consolidate", "recover", "--backup", recovery.recoveryID, "--confirm", preview.Confirmation, "--json")
		assertExit(t, apply, 0)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &outcome)
		if outcome.State != "recovered" || !outcome.Verified {
			t.Fatalf("absent pending path did not complete recovery: %#v", outcome)
		}
		if got := readBytes(t, filepath.Join(recovery.firstSourcePath, "SKILL.md")); !bytes.Equal(got, recovery.firstManifest) {
			t.Fatalf("recovery changed original source bytes: %q", got)
		}
		if _, err := os.Lstat(piLink); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Pi link remains after recovery: %v", err)
		}
		if _, err := os.Lstat(recovery.canonical); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("canonical bundle remains after recovery: %v", err)
		}
		var journal struct {
			State   string `json:"state"`
			Entries []struct {
				PendingRecoveryLinks []string `json:"pending_recovery_links"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(readBytes(t, journalPath), &journal); err != nil {
			t.Fatal(err)
		}
		if journal.State != "recovered" {
			t.Fatalf("journal state=%q, want recovered", journal.State)
		}
		for _, entry := range journal.Entries {
			if len(entry.PendingRecoveryLinks) != 0 {
				t.Fatalf("completed recovery retained pending removals: %v", entry.PendingRecoveryLinks)
			}
		}
	})
}

func preparePendingRecoveryLink(t *testing.T, fx fixture, recoveryID, link string) string {
	t.Helper()
	journalPath := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", recoveryID, "record.json")
	raw := readBytes(t, journalPath)
	var journal map[string]json.RawMessage
	if err := json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(journal["entries"], &entries); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		var created []string
		if err := json.Unmarshal(entry["created_links"], &created); err != nil {
			t.Fatal(err)
		}
		for _, path := range created {
			if filepath.Clean(path) != filepath.Clean(link) {
				continue
			}
			pending, err := json.Marshal([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			removed, err := json.Marshal([]string{})
			if err != nil {
				t.Fatal(err)
			}
			entry["pending_recovery_links"] = pending
			entry["removed_links"] = removed
			found = true
			break
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("applied recovery journal did not own expected link %s", link)
	}
	journal["state"], _ = json.Marshal("recovering")
	journal["entries"], _ = json.Marshal(entries)
	updated, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(journalPath, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	return journalPath
}
