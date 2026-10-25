package collection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryRechecksRemovedLinksBeforeRestoringSource(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)

	ctx := context.Background()
	canonical := filepath.Join(data, "skillglass", "skills", "recovery-boundary")
	sourcePath := filepath.Join(home, ".pi", "agent", "skills", "recovery-boundary")
	linkPath := filepath.Join(home, ".pi", "agent", "skills", "other-consumer", "recovery-boundary")
	writeRecoveryBoundaryBundle(t, canonical)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	source, fingerprint, complete, reason := consolidationFingerprintPath(ctx, canonical)
	if !complete {
		t.Fatalf("canonical fixture bundle is not verifiable: %s", reason)
	}
	if err := os.Symlink(canonical, linkPath); err != nil {
		t.Fatal(err)
	}
	journal := consolidationJournal{
		ID:    "c-recovery-window-test",
		State: "recovering",
		Entries: []consolidationJournalEntry{{
			SourceID: source.ID, Action: "share", SourcePath: sourcePath,
			Fingerprint: fingerprint, CanonicalPath: canonical,
			CanonicalSourceID: source.ID, MovedToCanonical: true,
			CreatedLinks: []string{linkPath},
		}},
	}

	if err := removeConsolidationRecoveryLinks(ctx, &journal); err != nil {
		t.Fatalf("first recovery pass failed to remove the recorded link: %v", err)
	}
	if len(journal.Entries[0].RemovedLinks) != 1 || journal.Entries[0].RemovedLinks[0] != linkPath {
		t.Fatalf("first pass did not record the removed link: %#v", journal.Entries[0].RemovedLinks)
	}
	if err := os.Symlink(canonical, linkPath); err != nil {
		t.Fatal(err)
	}

	err = restoreConsolidationRecoverySources(ctx, &journal)
	if err == nil || !strings.Contains(err.Error(), "previously removed recovery link reappeared") {
		t.Fatalf("source restoration did not refuse the later link: %v", err)
	}
	if pathExists(sourcePath) {
		t.Fatal("source was restored after a previously removed link reappeared")
	}
	if err := verifyMoveSource(ctx, canonical, fingerprint, source.ID); err != nil {
		t.Fatalf("canonical source changed on refusal: %v", err)
	}
	if target, err := os.Readlink(linkPath); err != nil || target != canonical {
		t.Fatalf("later link was not preserved on refusal: target=%q err=%v", target, err)
	}
}

func TestRecoveryBoundaryAcceptsRecordedRestoredSourceAtRemovedLinkPath(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)

	ctx := context.Background()
	sourcePath := filepath.Join(home, ".pi", "agent", "skills", "restored-source")
	writeRecoveryBoundaryBundle(t, sourcePath)
	source, fingerprint, complete, reason := consolidationFingerprintPath(ctx, sourcePath)
	if !complete {
		t.Fatalf("restored fixture bundle is not verifiable: %s", reason)
	}
	journal := consolidationJournal{
		ID:    "c-restored-source-test",
		State: "recovering",
		Entries: []consolidationJournalEntry{{
			SourceID: source.ID, Action: "share", SourcePath: sourcePath,
			Fingerprint: fingerprint, RestoredToSource: true,
			RemovedLinks: []string{sourcePath},
		}},
	}

	if err := restoreConsolidationRecoverySources(ctx, &journal); err != nil {
		t.Fatalf("recorded restored-source directory was mistaken for a reappeared link: %v", err)
	}
	if err := verifyMoveSource(ctx, sourcePath, fingerprint, source.ID); err != nil {
		t.Fatalf("recorded restored source changed during retry: %v", err)
	}
}

func TestPendingRecoveryLinkIsRefusedAtValidationAndMutationBoundaries(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)

	ctx := context.Background()
	canonical := filepath.Join(data, "skillglass", "skills", "pending-boundary")
	sourcePath := filepath.Join(home, ".pi", "agent", "skills", "pending-boundary")
	linkPath := filepath.Join(home, ".pi", "agent", "skills", "consumer", "pending-boundary")
	writeRecoveryBoundaryBundle(t, canonical)
	source, fingerprint, complete, reason := consolidationFingerprintPath(ctx, canonical)
	if !complete {
		t.Fatalf("canonical fixture bundle is not verifiable: %s", reason)
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonical, linkPath); err != nil {
		t.Fatal(err)
	}
	entry := consolidationJournalEntry{
		SourceID: source.ID, BackupSourceID: source.ID, Action: "share",
		SourcePath: sourcePath, Fingerprint: fingerprint, CanonicalPath: canonical,
		CanonicalSourceID: source.ID, MovedToCanonical: true,
		CreatedLinks: []string{linkPath}, PendingRecoveryLinks: []string{linkPath},
	}
	journal := consolidationJournal{State: "recovering", Entries: []consolidationJournalEntry{entry}}

	if err := validateRecoveryEntry(ctx, entry, journal.Entries); err == nil || !strings.Contains(err.Error(), "pending recovery link path is occupied") {
		t.Fatalf("recovery validation accepted an occupied pending link: %v", err)
	}
	if err := removeConsolidationRecoveryLinks(ctx, &journal); err == nil || !strings.Contains(err.Error(), "pending recovery link path is occupied") {
		t.Fatalf("link-removal boundary accepted an occupied pending link: %v", err)
	}
	if err := restoreConsolidationRecoverySources(ctx, &journal); err == nil || !strings.Contains(err.Error(), "pending recovery link path is occupied") {
		t.Fatalf("source-restoration boundary accepted an occupied pending link: %v", err)
	}
	if pathExists(sourcePath) {
		t.Fatal("source was restored despite an unresolved pending link")
	}
	if target, err := os.Readlink(linkPath); err != nil || filepath.Clean(target) != filepath.Clean(canonical) {
		t.Fatalf("later symlink was not preserved: target=%q err=%v", target, err)
	}
	if err := verifyMoveSource(ctx, canonical, fingerprint, source.ID); err != nil {
		t.Fatalf("canonical source changed on refusal: %v", err)
	}
}

func TestPendingRecoveryLinkRecognizesOnlyRecordedRestoredSourceDirectories(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)

	ctx := context.Background()
	sourcePath := filepath.Join(home, ".pi", "agent", "skills", "recorded-source")
	writeRecoveryBoundaryBundle(t, sourcePath)
	source, fingerprint, complete, reason := consolidationFingerprintPath(ctx, sourcePath)
	if !complete {
		t.Fatalf("restored fixture bundle is not verifiable: %s", reason)
	}
	for _, restoration := range []struct {
		name              string
		restoredToSource  bool
		pendingRecoveryMove string
	}{
		{name: "recorded source", restoredToSource: true},
		{name: "pending source restore", pendingRecoveryMove: "canonical"},
	} {
		t.Run(restoration.name, func(t *testing.T) {
			entries := []consolidationJournalEntry{{
				Action: "share", SourcePath: sourcePath, SourceID: source.ID, Fingerprint: fingerprint,
				RestoredToSource: restoration.restoredToSource, PendingRecoveryMove: restoration.pendingRecoveryMove,
			}}
			restored, err := validatePendingRecoveryLink(ctx, sourcePath, entries)
			if err != nil || !restored {
				t.Fatalf("recorded source directory was not accepted: restored=%t err=%v", restored, err)
			}
		})
	}

	linkPath := filepath.Join(home, ".pi", "agent", "skills", "symlinked-source")
	if err := os.Symlink(sourcePath, linkPath); err != nil {
		t.Fatal(err)
	}
	entries := []consolidationJournalEntry{{
		Action: "share", SourcePath: linkPath, SourceID: source.ID, Fingerprint: fingerprint,
		RestoredToSource: true,
	}}
	if restored, err := validatePendingRecoveryLink(ctx, linkPath, entries); err == nil || restored {
		t.Fatalf("same-target symlink was mistaken for the recorded source directory: restored=%t err=%v", restored, err)
	}
}

func writeRecoveryBoundaryBundle(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte("---\nname: recovery-boundary\ndescription: recovery boundary fixture\n---\nKeep this source.\n")
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
}
