package collection

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsolidationSelectionWriteCompensatesVisibleFailure(t *testing.T) {
	journal, prior, applied := selectionCommitFixture(t, true)
	writes := 0
	write := func(path string, data []byte) error {
		writes++
		if writes == 1 {
			if err := writePrivateAtomic(path, data); err != nil {
				return err
			}
			return errors.New("simulated directory sync failure after rename")
		}
		return writePrivateAtomic(path, data)
	}

	err := writeConsolidationSelectionWithRollback(&journal, write)
	if err == nil || !strings.Contains(err.Error(), "exact prior selection is visible again") {
		t.Fatalf("visible selection-write error was not reported after compensation: %v", err)
	}
	got, found, readErr := readPrivateSelection(journal.SelectionPath)
	if readErr != nil || !found || !bytes.Equal(got, prior) {
		t.Fatalf("post-visibility error did not restore exact prior bytes: found=%t err=%v got=%q want=%q", found, readErr, got, prior)
	}
	if !bytes.Equal(journal.AppliedSelection, applied) || journal.SelectionApplied || journal.PendingSelectionApply {
		t.Fatalf("successful compensation left misleading selection flags: %#v", journal)
	}
}

func TestConsolidationSelectionWriteCompensatesAbsentPrior(t *testing.T) {
	journal, _, applied := selectionCommitFixture(t, false)
	write := func(path string, data []byte) error {
		if !bytes.Equal(data, applied) {
			return errors.New("unexpected rollback write for absent prior state")
		}
		if err := writePrivateAtomic(path, data); err != nil {
			return err
		}
		return errors.New("simulated directory sync failure after rename")
	}

	err := writeConsolidationSelectionWithRollback(&journal, write)
	if err == nil || !strings.Contains(err.Error(), "exact prior selection is visible again") {
		t.Fatalf("visible selection-write error was not reported after unlink compensation: %v", err)
	}
	if _, found, readErr := readPrivateSelection(journal.SelectionPath); readErr != nil || found {
		t.Fatalf("absent prior selection was not restored: found=%t err=%v", found, readErr)
	}
	if journal.SelectionApplied || journal.PendingSelectionApply {
		t.Fatalf("successful absent-state compensation left pending selection flags: %#v", journal)
	}
}

func TestConsolidationSelectionRollbackPreservesThirdPartyValue(t *testing.T) {
	journal, _, applied := selectionCommitFixture(t, true)
	thirdParty := []byte("{\"schema_version\":1,\"applications\":[\"claude-code\"],\"library\":\"/other/library\"}\n")
	writes := 0
	write := func(path string, data []byte) error {
		writes++
		if writes == 1 {
			if !bytes.Equal(data, applied) {
				return errors.New("expected proposed selection write")
			}
			if err := writePrivateAtomic(path, data); err != nil {
				return err
			}
			if err := writePrivateAtomic(path, thirdParty); err != nil {
				return err
			}
			return errors.New("simulated post-visibility write error")
		}
		return writePrivateAtomic(path, data)
	}

	err := writeConsolidationSelectionWithRollback(&journal, write)
	if err == nil || !strings.Contains(err.Error(), "different selection is currently visible and was preserved") {
		t.Fatalf("third-party selection was not reported as preserved: %v", err)
	}
	got, found, readErr := readPrivateSelection(journal.SelectionPath)
	if readErr != nil || !found || !bytes.Equal(got, thirdParty) {
		t.Fatalf("conditional rollback overwrote the later selection: found=%t err=%v got=%q want=%q", found, readErr, got, thirdParty)
	}
	if !journal.PendingSelectionApply {
		t.Fatalf("failed rollback did not retain pending recovery: %#v", journal)
	}
}

func TestConsolidationSelectionJournalFailuresCompensate(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		failAt         int
		priorExists    bool
		thirdPartyEdit bool
	}{
		{name: "selection-applied marker", failAt: 2, priorExists: true},
		{name: "final applied state", failAt: 3, priorExists: true},
		{name: "final state with absent prior", failAt: 3, priorExists: false},
		{name: "third-party edit during marker failure", failAt: 2, priorExists: true, thirdPartyEdit: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			journal, prior, applied := selectionCommitFixture(t, testCase.priorExists)
			thirdParty := []byte("{\"schema_version\":1,\"applications\":[\"claude-code\"],\"library\":\"/other/library\"}\n")
			journalWrites := 0
			writeJournal := func(value consolidationJournal) error {
				journalWrites++
				if err := writeConsolidationJournal(value); err != nil {
					return err
				}
				if journalWrites != testCase.failAt {
					return nil
				}
				if testCase.thirdPartyEdit {
					if err := writePrivateAtomic(journal.SelectionPath, thirdParty); err != nil {
						return err
					}
				}
				return errors.New("simulated journal sync failure after rename")
			}

			err := commitConsolidationSelection(&journal, writeJournal, writePrivateAtomic)
			if err == nil {
				t.Fatal("injected post-selection journal error was ignored")
			}
			if journalWrites != testCase.failAt {
				t.Fatalf("journal writes=%d, want failure at %d", journalWrites, testCase.failAt)
			}
			if testCase.thirdPartyEdit {
				got, found, readErr := readPrivateSelection(journal.SelectionPath)
				if readErr != nil || !found || !bytes.Equal(got, thirdParty) {
					t.Fatalf("later selection was overwritten: found=%t err=%v got=%q", found, readErr, got)
				}
				if !journal.PendingSelectionApply || !journal.SelectionApplied {
					t.Fatalf("failed compensation did not retain its recovery intent: %#v", journal)
				}
				if err := validateSelectionForRecovery(journal); err == nil {
					t.Fatal("recovery accepted a third-party selection value")
				}
				return
			}
			if journal.SelectionApplied || journal.PendingSelectionApply {
				t.Fatalf("successful compensation left pending selection flags: %#v", journal)
			}
			if testCase.priorExists {
				got, found, readErr := readPrivateSelection(journal.SelectionPath)
				if readErr != nil || !found || !bytes.Equal(got, prior) {
					t.Fatalf("journal failure did not restore exact prior selection: found=%t err=%v got=%q want=%q", found, readErr, got, prior)
				}
			} else if _, found, readErr := readPrivateSelection(journal.SelectionPath); readErr != nil || found {
				t.Fatalf("journal failure did not restore absent prior selection: found=%t err=%v", found, readErr)
			}
			persisted, readErr := readConsolidationJournal(journal.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err := validateSelectionForRecovery(persisted); err != nil {
				t.Fatalf("visible journal after fsync error cannot safely recover the restored prior selection: %v", err)
			}
			if !bytes.Equal(persisted.AppliedSelection, applied) {
				t.Fatalf("journal lost the exact proposed selection: %#v", persisted)
			}
		})
	}
}

func selectionCommitFixture(t *testing.T, priorExists bool) (consolidationJournal, []byte, []byte) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "xdg-config")
	data := filepath.Join(home, "xdg-data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("XDG_DATA_HOME", data)
	prior, err := encodeConsolidationSelection(ConsolidationRequest{Applications: []string{"codex", "pi"}, Library: filepath.Join(data, "skillglass", "skills")})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := encodeConsolidationSelection(ConsolidationRequest{Applications: []string{"pi"}, Library: filepath.Join(data, "skillglass", "skills")})
	if err != nil {
		t.Fatal(err)
	}
	selectionPath, err := consolidationSelectionPath()
	if err != nil {
		t.Fatal(err)
	}
	if priorExists {
		if err := writePrivateAtomic(selectionPath, prior); err != nil {
			t.Fatal(err)
		}
	}
	journal := consolidationJournal{
		SchemaVersion:        ConsolidationSchemaVersion,
		ID:                   "c-0123456789abcdef01234567",
		State:                "prepared",
		Scope:                ConsolidationRequest{Applications: []string{"pi"}, Library: filepath.Join(data, "skillglass", "skills")},
		Entries:              []consolidationJournalEntry{},
		SelectionPath:        selectionPath,
		PriorSelection:       prior,
		PriorSelectionExists: priorExists,
		AppliedSelection:     applied,
	}
	if err := writeConsolidationJournal(journal); err != nil {
		t.Fatal(err)
	}
	return journal, prior, applied
}
