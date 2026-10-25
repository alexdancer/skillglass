package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const maxConsolidationJournalBytes = 4 << 20

type ConsolidationApplyOutcome struct {
	SchemaVersion    int                   `json:"schema_version"`
	Operation        string                `json:"operation"`
	Outcome          string                `json:"outcome"`
	ExitCode         int                   `json:"exit_code"`
	State            string                `json:"state"`
	Reason           string                `json:"reason"`
	RecoveryID       string                `json:"recovery_id,omitempty"`
	Effects          []ConsolidationEffect `json:"effects"`
	Verified         bool                  `json:"verified"`
	SelectionUpdated bool                  `json:"selection_updated,omitempty"`
	Failure          *ConsolidationFailure `json:"failure,omitempty"`
}

type consolidationJournal struct {
	SchemaVersion           int                         `json:"schema_version"`
	ID                      string                      `json:"id"`
	State                   string                      `json:"state"`
	Scope                   ConsolidationRequest        `json:"scope"`
	Draft                   ConsolidationDraft          `json:"draft"`
	Confirmation            string                      `json:"confirmation"`
	Entries                 []consolidationJournalEntry `json:"entries"`
	SelectionPath           string                      `json:"selection_path"`
	PriorSelection          []byte                      `json:"prior_selection,omitempty"`
	PriorSelectionExists    bool                        `json:"prior_selection_exists"`
	AppliedSelection        []byte                      `json:"applied_selection,omitempty"`
	PendingSelectionApply   bool                        `json:"pending_selection_apply,omitempty"`
	SelectionApplied        bool                        `json:"selection_applied"`
	SelectionRestored       bool                        `json:"selection_restored"`
	PendingSelectionRestore bool                        `json:"pending_selection_restore,omitempty"`
}

type consolidationJournalEntry struct {
	SourceID             SourceID          `json:"source_id"`
	CanonicalSourceID    SourceID          `json:"canonical_source_id,omitempty"`
	BackupSourceID       SourceID          `json:"backup_source_id,omitempty"`
	Action               string            `json:"action"`
	SourcePath           string            `json:"source_path"`
	Fingerprint          string            `json:"fingerprint"`
	CanonicalPath        string            `json:"canonical_path,omitempty"`
	BackupPath           string            `json:"backup_path,omitempty"`
	MovedToCanonical     bool              `json:"moved_to_canonical"`
	MovedToBackup        bool              `json:"moved_to_backup"`
	PendingMove          string            `json:"pending_move,omitempty"`
	CanonicalWasPresent  bool              `json:"canonical_was_present"`
	CreatedLinks         []string          `json:"created_links"`
	PendingLinks         []string          `json:"pending_links,omitempty"`
	FailedLinks          []string          `json:"failed_links,omitempty"`
	RemovedLinks         []string          `json:"removed_links,omitempty"`
	PendingRecoveryLinks []string          `json:"pending_recovery_links,omitempty"`
	RestoredToSource     bool              `json:"restored_to_source"`
	PendingRecoveryMove  string            `json:"pending_recovery_move,omitempty"`
	ExpectedLinks        []string          `json:"expected_links"`
	BeforeRefs           []string          `json:"before_references"`
	ExpectedAvailability map[string]string `json:"expected_availability,omitempty"`
}

// ApplyConsolidation checks the live evidence and exact token again before any
// mutation. It journals each move and link in a private recovery directory.
func ApplyConsolidation(ctx context.Context, draft ConsolidationDraft, confirmation string) ConsolidationApplyOutcome {
	out := ConsolidationApplyOutcome{SchemaVersion: ConsolidationSchemaVersion, Operation: "consolidate-apply", Outcome: "failed", ExitCode: 1, State: "refused", Reason: "confirmation or current evidence did not authorize the operation", Effects: []ConsolidationEffect{}}
	preview := PreviewConsolidation(ctx, draft)
	if preview.State != "ready" || preview.ExitCode != 0 {
		out.Failure = preview.Failure
		out.Reason = "fresh preview refused: " + preview.Reason
		return out
	}
	if confirmation == "" || confirmation != preview.Confirmation {
		out.Failure = &ConsolidationFailure{Code: "confirmation-mismatch", Message: "exact preview confirmation token does not match the current plan", Action: "Review the fresh preview and repeat apply with its exact confirmation token."}
		out.Reason = out.Failure.Message
		return out
	}
	out.RecoveryID = preview.RecoveryID
	out.Effects = append([]ConsolidationEffect(nil), preview.Effects...)
	if consolidationEffectsAreNoop(out.Effects) {
		for i := range out.Effects {
			if out.Effects[i].Action == "share" {
				out.Effects[i].State = "unchanged"
				out.Effects[i].BackupPath = ""
				out.Effects[i].Reason = "bundle is already canonical and all requested application links already exist"
			}
		}
		currentSelection, selectionErr := CurrentConsolidationSelectionID()
		if selectionErr != nil || currentSelection != draft.Scope.RememberedSelectionID {
			out.RecoveryID = ""
			out.Failure = &ConsolidationFailure{Code: "stale-selection", Message: "last-successful application selection changed after preview", Action: "Repeat scan and prepare a fresh proposal before applying."}
			out.Reason = out.Failure.Message
			return out
		}
		desiredSelection := RememberedSelectionID(draft.Scope.Applications, draft.Scope.Roots, draft.Scope.Library)
		selectionUpdated := false
		if currentSelection != desiredSelection {
			journal, err := newConsolidationJournal(ctx, draft, preview)
			if err != nil {
				return consolidationSelectionRefusal(out, "no bundle or link changed, but the selection-only recovery plan could not be prepared: "+err.Error(), "Resolve the private recovery-store issue and prepare a fresh scan.")
			}
			journal.Draft = ConsolidationDraft{}
			journal.Confirmation = ""
			journal.Entries = []consolidationJournalEntry{}
			if err := commitConsolidationSelection(&journal, writeConsolidationJournal, writePrivateAtomic); err != nil {
				return consolidationSelectionOnlyFailure(out, journal, err)
			}
			selectionUpdated = true
			out.RecoveryID = journal.ID
		}
		if !selectionUpdated {
			out.RecoveryID = ""
		}
		out.SelectionUpdated = selectionUpdated
		out.Outcome, out.ExitCode, out.Verified = "success", 0, true
		if selectionUpdated {
			out.State, out.Reason = "selection-updated", "bundle and link state already matched; the last-successful application selection was updated"
		} else {
			out.State, out.Reason = "unchanged", "all choices and the remembered selection already matched; no filesystem changes were needed"
		}
		return out
	}
	journal, err := newConsolidationJournal(ctx, draft, preview)
	if err != nil {
		out.RecoveryID = ""
		out.Failure = &ConsolidationFailure{Code: "journal-preflight", Message: err.Error(), Action: "Resolve the private recovery-store issue and prepare a fresh preview."}
		out.Reason = err.Error()
		out.Effects = consolidationApplyEffectOutcomes(ctx, preview.Effects, consolidationJournal{})
		return out
	}
	if err := writeConsolidationJournal(journal); err != nil {
		_ = removeEmptyConsolidationPlan(journal.ID)
		out.RecoveryID = ""
		out.Failure = &ConsolidationFailure{Code: "journal-write", Message: err.Error(), Action: "No source should have moved. Inspect the private recovery store before retrying."}
		out.Reason = err.Error()
		out.Effects = consolidationApplyEffectOutcomes(ctx, preview.Effects, journal)
		return out
	}
	setPartial := func(reason string) ConsolidationApplyOutcome {
		journal.State = "partial"
		if err := writeConsolidationJournal(journal); err != nil {
			reason += "; partial recovery record could not be persisted: " + err.Error()
		}
		out.State, out.Outcome, out.ExitCode, out.Reason = "partial", "failed", 1, reason
		out.Failure = &ConsolidationFailure{Code: "partial-apply", Message: reason, Action: "Run consolidate recover --backup " + journal.ID + " to preview a safe restoration."}
		out.Effects = consolidationApplyEffectOutcomes(ctx, preview.Effects, journal)
		return out
	}
	if err := applyConsolidationEntries(ctx, &journal); err != nil {
		return setPartial(err.Error())
	}
	if err := verifyConsolidationJournal(ctx, journal); err != nil {
		return setPartial("post-apply verification failed: " + err.Error())
	}
	if err := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, journal.PriorSelectionExists); err != nil {
		return setPartial("last-successful selection changed during apply; the newer selection was preserved: " + err.Error())
	}
	if err := commitConsolidationSelection(&journal, writeConsolidationJournal, writePrivateAtomic); err != nil {
		return setPartial(err.Error())
	}
	out.State, out.Outcome, out.ExitCode, out.Reason, out.Verified = "applied", "success", 0, "all planned bundle moves and native links were re-inspected", true
	out.Effects = consolidationApplyEffectOutcomes(ctx, preview.Effects, journal)
	return out
}

func commitConsolidationSelection(journal *consolidationJournal, writeJournal func(consolidationJournal) error, writeSelection func(string, []byte) error) error {
	journal.PendingSelectionApply = true
	if err := writeJournal(*journal); err != nil {
		return fmt.Errorf("filesystem changes completed but selection write intent could not be journaled: %w", err)
	}
	if err := writeConsolidationSelectionWithRollback(journal, writeSelection); err != nil {
		return err
	}
	journal.SelectionApplied = true
	journal.PendingSelectionApply = false
	if err := writeJournal(*journal); err != nil {
		return compensateConsolidationSelectionAfterJournalFailure(journal, "selection was written but the selection-applied journal update failed", err, writeSelection)
	}
	journal.State = "applied"
	if err := writeJournal(*journal); err != nil {
		return compensateConsolidationSelectionAfterJournalFailure(journal, "selection was written but the final recovery record could not be persisted", err, writeSelection)
	}
	return nil
}

func compensateConsolidationSelectionAfterJournalFailure(journal *consolidationJournal, message string, writeErr error, writeSelection func(string, []byte) error) error {
	journal.PendingSelectionApply = true
	compensationErr := rollbackConsolidationSelection(journal, writeSelection)
	return selectionCommitFailure(message, writeErr, compensationErr, *journal)
}

func writeConsolidationSelectionWithRollback(journal *consolidationJournal, writeSelection func(string, []byte) error) error {
	if err := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, journal.PriorSelectionExists); err != nil {
		return fmt.Errorf("last-successful selection changed before commit; the newer value was preserved: %w", err)
	}
	if err := writeSelection(journal.SelectionPath, journal.AppliedSelection); err == nil {
		return nil
	} else {
		writeErr := err
		compensationErr := rollbackConsolidationSelection(journal, writeSelection)
		return selectionCommitFailure("last-successful selection write failed", writeErr, compensationErr, *journal)
	}
}

func rollbackConsolidationSelection(journal *consolidationJournal, writeSelection func(string, []byte) error) error {
	journal.PendingSelectionApply = true
	if err := restoreSelectionWithWriter(*journal, writeSelection); err != nil {
		return fmt.Errorf("conditional rollback failed; %s: %w", consolidationSelectionVisibleState(*journal), err)
	}
	if err := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, journal.PriorSelectionExists); err != nil {
		return fmt.Errorf("conditional rollback did not restore the exact prior selection; %s: %w", consolidationSelectionVisibleState(*journal), err)
	}
	journal.SelectionApplied = false
	journal.PendingSelectionApply = false
	return nil
}

func selectionCommitFailure(message string, writeErr, compensationErr error, journal consolidationJournal) error {
	if compensationErr == nil {
		return fmt.Errorf("%s: %w; the exact prior selection is visible again", message, writeErr)
	}
	return fmt.Errorf("%s: %w; %v; %s", message, writeErr, compensationErr, consolidationSelectionVisibleState(journal))
}

func consolidationSelectionVisibleState(journal consolidationJournal) string {
	if err := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, journal.PriorSelectionExists); err == nil {
		return "the prior selection is currently visible"
	}
	if err := sameBytesOrAbsent(journal.SelectionPath, journal.AppliedSelection, true); err == nil {
		return "the proposed selection is currently visible"
	}
	if _, exists, err := readPrivateSelection(journal.SelectionPath); err != nil {
		return "the current selection could not be read"
	} else if !exists {
		return "the selection file is currently absent"
	}
	return "a different selection is currently visible and was preserved"
}

func consolidationSelectionRefusal(out ConsolidationApplyOutcome, message, action string) ConsolidationApplyOutcome {
	out.RecoveryID = ""
	out.State, out.Outcome, out.ExitCode = "refused", "failed", 1
	out.Reason, out.Verified, out.SelectionUpdated = message, false, false
	out.Failure = &ConsolidationFailure{Code: "selection-write", Message: message, Action: action}
	return out
}

func consolidationSelectionOnlyFailure(out ConsolidationApplyOutcome, journal consolidationJournal, applyErr error) ConsolidationApplyOutcome {
	persisted, err := readConsolidationJournal(journal.ID)
	if err != nil {
		cleanupErr := removeEmptyConsolidationPlan(journal.ID)
		message := "selection-only apply failed before a durable recovery record was available: " + applyErr.Error()
		if cleanupErr != nil && !errors.Is(cleanupErr, unix.ENOENT) {
			message += "; empty recovery plan cleanup failed: " + cleanupErr.Error()
		}
		proposedVisible := sameBytesOrAbsent(journal.SelectionPath, journal.AppliedSelection, true) == nil
		if proposedVisible {
			message += "; the proposed selection remains visible, but no usable recovery record could be read"
		}
		out = consolidationSelectionRefusal(out, message, "Resolve the selection and private recovery-store issue before retrying.")
		if proposedVisible {
			out.State = "partial"
			out.Failure.Action = "Resolve the missing recovery record before restoring the previous selection."
		}
		return out
	}
	if !consolidationSelectionOnlyJournalMatches(journal, persisted) || (!persisted.PendingSelectionApply && !persisted.SelectionApplied) {
		proposedVisible := sameBytesOrAbsent(journal.SelectionPath, journal.AppliedSelection, true) == nil
		out = consolidationSelectionRefusal(out, "selection-only apply failed and its recovery record changed or contains non-selection effects: "+applyErr.Error(), "Preserve the recovery record and inspect it before retrying.")
		if proposedVisible {
			out.State = "partial"
			out.Failure.Action = "Preserve the current state and inspect the mismatched recovery record before restoring the previous selection."
		}
		return out
	}

	priorVisible := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, journal.PriorSelectionExists) == nil
	proposedVisible := sameBytesOrAbsent(journal.SelectionPath, journal.AppliedSelection, true) == nil
	persisted.State = "partial"
	persisted.PendingSelectionApply = true
	if priorVisible {
		persisted.SelectionApplied = false
	}
	journalErr := writeConsolidationJournal(persisted)

	out.RecoveryID = journal.ID
	out.Outcome, out.ExitCode, out.Verified, out.SelectionUpdated = "failed", 1, false, false
	if proposedVisible {
		out.State = "partial"
		out.Reason = "selection-only apply failed and the proposed selection remains visible: " + applyErr.Error()
		out.Failure = &ConsolidationFailure{Code: "selection-write", Message: out.Reason, Action: "Run consolidate recover --backup " + journal.ID + " to restore the previous selection."}
	} else if priorVisible {
		out.State = "refused"
		out.Reason = "selection-only apply failed; the exact prior selection is visible and a recovery record is available: " + applyErr.Error()
		out.Failure = &ConsolidationFailure{Code: "selection-write", Message: out.Reason, Action: "Run consolidate recover --backup " + journal.ID + " to confirm or finish selection restoration."}
	} else {
		out.State = "refused"
		out.Reason = "selection-only apply failed; " + consolidationSelectionVisibleState(journal) + ": " + applyErr.Error()
		out.Failure = &ConsolidationFailure{Code: "selection-write", Message: out.Reason, Action: "Preserve the current selection and resolve it against recovery record " + journal.ID + " before retrying recovery."}
	}
	if journalErr != nil {
		out.Reason += "; pending recovery journal update failed: " + journalErr.Error()
		out.Failure.Message = out.Reason
	}
	return out
}

func consolidationSelectionOnlyJournalMatches(expected, actual consolidationJournal) bool {
	if expected.ID != actual.ID || expected.SelectionPath != actual.SelectionPath || expected.PriorSelectionExists != actual.PriorSelectionExists || !bytes.Equal(expected.PriorSelection, actual.PriorSelection) || !bytes.Equal(expected.AppliedSelection, actual.AppliedSelection) {
		return false
	}
	if RememberedSelectionID(expected.Scope.Applications, expected.Scope.Roots, expected.Scope.Library) != RememberedSelectionID(actual.Scope.Applications, actual.Scope.Roots, actual.Scope.Library) || expected.Scope.SelectionFrom != actual.Scope.SelectionFrom || expected.Scope.RememberedSelectionID != actual.Scope.RememberedSelectionID {
		return false
	}
	if len(actual.Entries) != 0 || actual.Confirmation != "" || actual.Draft.SchemaVersion != 0 || actual.Draft.ScanID != "" || len(actual.Draft.Choices) != 0 || len(actual.Draft.Scope.Applications) != 0 || len(actual.Draft.Scope.Roots) != 0 || actual.Draft.Scope.Library != "" || actual.Draft.Scope.SelectionFrom != "" || actual.Draft.Scope.RememberedSelectionID != "" {
		return false
	}
	return actual.State == "prepared" || actual.State == "applied" || actual.State == "partial"
}

func consolidationEffectsAreNoop(effects []ConsolidationEffect) bool {
	for _, effect := range effects {
		switch effect.Action {
		case "keep":
			continue
		case "share":
			if filepath.Clean(effect.SourcePath) == filepath.Clean(effect.CanonicalPath) && len(effect.Destinations) == 0 {
				continue
			}
		}
		return false
	}
	return true
}

func newConsolidationJournal(ctx context.Context, draft ConsolidationDraft, preview ConsolidationPreview) (consolidationJournal, error) {
	planID := preview.RecoveryID
	moveDevicesRequired := !consolidationEffectsAreNoop(preview.Effects)
	if err := consolidationMovesPreflightWithDevice(ctx, preview.Effects, moveDevicesRequired, existingParentDevice); err != nil {
		return consolidationJournal{}, err
	}
	selectionPath, oldBytes, oldExists, err := captureConsolidationSelection()
	if err != nil {
		return consolidationJournal{}, fmt.Errorf("inspect remembered selection before mutation: %w", err)
	}
	if draft.Scope.RememberedSelectionID != "" {
		capturedID, selectionErr := consolidationSelectionIDFromBytes(selectionPath, oldBytes, oldExists)
		if selectionErr != nil || capturedID != draft.Scope.RememberedSelectionID {
			return consolidationJournal{}, errors.New("remembered selection captured for recovery does not match the proposal evidence")
		}
	}
	newSelection, err := encodeConsolidationSelection(draft.Scope)
	if err != nil {
		return consolidationJournal{}, err
	}
	root := consolidationBackupRoot()
	planDir := filepath.Join(root, planID)
	if pathExists(planDir) {
		return consolidationJournal{}, fmt.Errorf("recovery plan already exists: %s", planID)
	}
	journal := consolidationJournal{SchemaVersion: ConsolidationSchemaVersion, ID: planID, State: "prepared", Scope: draft.Scope, Draft: draft, Confirmation: preview.Confirmation, Entries: []consolidationJournalEntry{}, SelectionPath: selectionPath, PriorSelection: oldBytes, PriorSelectionExists: oldExists, AppliedSelection: newSelection}
	inventory := ScanConsolidation(ctx, draft.Scope)
	if inventory.Failure != nil || inventory.SelectedCoverage != Complete || !inventory.LibrarySafe || inventory.ScanID != preview.ScanID {
		return consolidationJournal{}, errors.New("complete source and consumer evidence changed or the library became unsafe after preview; no filesystem changes were made")
	}
	if err := consolidationMovesPreflightWithDevice(ctx, preview.Effects, moveDevicesRequired, existingParentDevice); err != nil {
		return consolidationJournal{}, err
	}
	if err := sameBytesOrAbsent(selectionPath, oldBytes, oldExists); err != nil {
		return consolidationJournal{}, fmt.Errorf("remembered selection changed after preview: %w", err)
	}
	if draft.Scope.RememberedSelectionID != "" {
		currentID, selectionErr := CurrentConsolidationSelectionID()
		if selectionErr != nil || currentID != draft.Scope.RememberedSelectionID {
			return consolidationJournal{}, errors.New("last-successful application selection changed after preview")
		}
	} else if strings.EqualFold(draft.Scope.SelectionFrom, "remembered") {
		return consolidationJournal{}, errors.New("remembered selection evidence ID is missing")
	}
	canonicalOwners := map[string]SourceID{}
	for _, effect := range preview.Effects {
		if effect.Action != "share" {
			continue
		}
		path := filepath.Clean(effect.CanonicalPath)
		if _, exists := canonicalOwners[path]; exists {
			continue
		}
		owner := effect.SourceID
		if pathExists(path) {
			inventoryID, found := consolidationInventoryIdentityAtPath(inventory, path, effect.BundleFingerprint)
			if !found {
				return consolidationJournal{}, fmt.Errorf("complete scan did not prove the existing canonical source identity at %s", path)
			}
			canonical, fingerprint, complete, reason := consolidationFingerprintPath(ctx, path)
			if !complete || fingerprint != effect.BundleFingerprint || canonical.ID != inventoryID {
				return consolidationJournal{}, fmt.Errorf("existing canonical source identity changed after scan at %s: %s", path, reason)
			}
			owner = canonical.ID
		}
		canonicalOwners[path] = owner
	}
	for _, effect := range preview.Effects {
		if effect.Action == "keep" {
			continue
		}
		entry := consolidationJournalEntry{SourceID: effect.SourceID, BackupSourceID: effect.SourceID, Action: effect.Action, SourcePath: effect.SourcePath, Fingerprint: effect.BundleFingerprint, BackupPath: effect.BackupPath, CanonicalPath: effect.CanonicalPath, CreatedLinks: []string{}, PendingLinks: []string{}, FailedLinks: []string{}, RemovedLinks: []string{}, PendingRecoveryLinks: []string{}, ExpectedLinks: append([]string(nil), effect.Destinations...), ExpectedAvailability: map[string]string{}}
		if entry.Action == "share" {
			entry.CanonicalSourceID = canonicalOwners[filepath.Clean(entry.CanonicalPath)]
			if entry.CanonicalSourceID == "" {
				return consolidationJournal{}, fmt.Errorf("canonical physical source identity is unavailable at %s", entry.CanonicalPath)
			}
		}
		for _, source := range inventory.Sources {
			if source.ID != effect.SourceID {
				continue
			}
			for _, ref := range source.KnownReferences {
				entry.BeforeRefs = append(entry.BeforeRefs, ref.Reference.DiscoveredPath)
			}
			for app, availability := range source.Availability {
				if hasNativeReference(source, app) {
					entry.ExpectedAvailability[app] = string(availability.State)
				}
			}
			break
		}
		if entry.Action == "share" && filepath.Clean(entry.CanonicalPath) != filepath.Clean(entry.SourcePath) {
			for _, source := range inventory.Sources {
				if source.ID != entry.CanonicalSourceID || source.BundleFingerprint != entry.Fingerprint || source.FingerprintCoverage != Complete || !consolidationPathResolvesTo(source.SourcePath, entry.CanonicalPath) {
					continue
				}
				for _, ref := range source.KnownReferences {
					entry.BeforeRefs = append(entry.BeforeRefs, ref.Reference.DiscoveredPath)
				}
			}
		}
		entry.BeforeRefs = uniqueJournalPaths(entry.BeforeRefs)
		journal.Entries = append(journal.Entries, entry)
	}
	preflightJournal := journal
	if consolidationEffectsAreNoop(preview.Effects) {
		preflightJournal.Draft = ConsolidationDraft{}
		preflightJournal.Confirmation = ""
		preflightJournal.Entries = []consolidationJournalEntry{}
	}
	if err := preflightConsolidationJournalLifecycle(preflightJournal); err != nil {
		return consolidationJournal{}, err
	}
	if err := ensureConsolidationDirectory(filepath.Dir(selectionPath), true); err != nil {
		return consolidationJournal{}, fmt.Errorf("remembered selection directory is not private and safely reachable: %w", err)
	}
	if err := ensureConsolidationDirectory(filepath.Dir(root), true); err != nil {
		return consolidationJournal{}, fmt.Errorf("recovery parent is not private and safely reachable: %w", err)
	}
	if err := ensureConsolidationDirectory(root, true); err != nil {
		return consolidationJournal{}, fmt.Errorf("recovery store is not private and safely reachable: %w", err)
	}
	if err := ensureConsolidationDirectory(draft.Scope.Library, true); err != nil {
		return consolidationJournal{}, fmt.Errorf("canonical library is not private and safely reachable: %w", err)
	}
	if err := createConsolidationPlanDirectory(planDir); err != nil {
		return consolidationJournal{}, err
	}
	return journal, nil
}

func consolidationInventoryIdentityAtPath(inventory ConsolidationInventory, path, fingerprint string) (SourceID, bool) {
	for _, source := range inventory.Sources {
		if source.BundleFingerprint != fingerprint || source.FingerprintCoverage != Complete || source.ID == "" {
			continue
		}
		if consolidationPathResolvesTo(source.SourcePath, path) {
			return source.ID, true
		}
		for _, reference := range source.KnownReferences {
			if consolidationPathResolvesTo(reference.Reference.ResolvedPath, path) {
				return source.ID, true
			}
		}
	}
	return "", false
}

func consolidationPathResolvesTo(path, target string) bool {
	if path == "" {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && filepath.Clean(resolved) == filepath.Clean(target)
}

func consolidationMovesPreflightWithDevice(ctx context.Context, effects []ConsolidationEffect, checkMoveDevices bool, deviceForPath func(string) (int32, error)) error {
	for _, effect := range effects {
		if effect.Action == "keep" {
			continue
		}
		kind := "directory"
		if err := verifyMoveSource(ctx, effect.SourcePath, effect.BundleFingerprint, effect.SourceID); err != nil {
			return fmt.Errorf("source changed or physical identity changed before apply: %w", err)
		}
		target := DeleteTarget{Source: effect.SourceID, Path: effect.SourcePath, Kind: kind, Identity: Identity{Method: "darwin-directory-object", Certainty: "proven"}}
		_, _, ownership, err := inventoryDelete(ctx, target)
		if err != nil || ownership.State != "local-user-controlled" {
			return fmt.Errorf("source move is not ownership-verified: %s: %s", effect.SourcePath, errors.Join(err, errors.New(ownership.Reason)))
		}
		if checkMoveDevices {
			destination := effect.BackupPath
			if effect.Action == "share" {
				destination = effect.CanonicalPath
			}
			device, err := deviceForPath(destination)
			if err != nil {
				return fmt.Errorf("destination filesystem is unavailable: %s: %w", destination, err)
			}
			var source unix.Stat_t
			if err := unix.Lstat(effect.SourcePath, &source); err != nil {
				return fmt.Errorf("source disappeared before apply: %s: %w", effect.SourcePath, err)
			}
			if source.Dev != device {
				return fmt.Errorf("cross-device relocation is unsupported for %s (source device %d, destination device %d)", effect.SourcePath, source.Dev, device)
			}
			if effect.Action == "share" {
				backupDevice, backupErr := deviceForPath(effect.BackupPath)
				if backupErr != nil || backupDevice != source.Dev {
					return fmt.Errorf("duplicate recovery path must be on the source filesystem: %s", effect.BackupPath)
				}
			}
		}
	}
	return nil
}

// preflightConsolidationJournalLifecycle proves that the exact initial record
// and a conservative upper bound for later apply/recovery states fit the same
// encoded limit enforced by the journal reader and writer. All candidate paths
// are known after the scan, so one bounded projection avoids repeatedly
// serializing each growing transaction prefix.
func preflightConsolidationJournalLifecycle(journal consolidationJournal) error {
	if _, err := encodeConsolidationJournal(journal); err != nil {
		return err
	}
	projection := consolidationJournalLifecycleProjection(journal)
	if _, err := encodeConsolidationJournal(projection); err != nil {
		return fmt.Errorf("planned consolidation journal lifecycle exceeds the recovery limit: %w", err)
	}
	return nil
}

func consolidationJournalLifecycleProjection(journal consolidationJournal) consolidationJournal {
	projection := journal
	projection.Entries = append([]consolidationJournalEntry(nil), journal.Entries...)
	projection.State = "recovering"
	projection.PendingSelectionApply = true
	projection.SelectionApplied = true
	projection.SelectionRestored = true
	projection.PendingSelectionRestore = true
	for i := range projection.Entries {
		entry := &projection.Entries[i]
		entry.MovedToCanonical = true
		entry.MovedToBackup = true
		entry.CanonicalWasPresent = true
		entry.RestoredToSource = true
		entry.PendingMove = "duplicate"
		entry.PendingRecoveryMove = "canonical"
		if entry.Action != "share" {
			continue
		}
		paths := make([]string, 0, len(entry.ExpectedLinks)+1)
		if filepath.Clean(entry.SourcePath) != filepath.Clean(entry.CanonicalPath) {
			paths = append(paths, entry.SourcePath)
		}
		paths = append(paths, entry.ExpectedLinks...)
		paths = uniqueJournalPaths(paths)
		entry.CreatedLinks = append([]string(nil), paths...)
		// Apply and recovery serialize one current intent or failure at a time;
		// the full Created/Removed sets bound the completed path prefixes.
		entry.PendingLinks = longestEncodedJournalPath(paths)
		entry.FailedLinks = longestEncodedJournalPath(paths)
		entry.RemovedLinks = append([]string(nil), paths...)
		entry.PendingRecoveryLinks = longestEncodedJournalPath(paths)
	}
	return projection
}

func longestEncodedJournalPath(paths []string) []string {
	longest := ""
	longestEncodedSize := -1
	for _, path := range paths {
		encoded, err := json.Marshal(path)
		if err == nil && len(encoded) > longestEncodedSize {
			longest, longestEncodedSize = path, len(encoded)
		}
	}
	if longestEncodedSize < 0 {
		return []string{}
	}
	return []string{longest}
}

func applyConsolidationEntries(ctx context.Context, journal *consolidationJournal) error {
	// Move every original bundle out of native destinations before creating
	// links. Identical selected copies can occupy each other's planned link
	// paths; linking one entry first would make a safe whole-bundle dedup look
	// like an unexpected collision.
	for i := range journal.Entries {
		entry := &journal.Entries[i]
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Action == "archive" {
			if err := ensureConsolidationDirectory(filepath.Dir(entry.BackupPath), true); err != nil {
				return err
			}
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return err
			}
			entry.PendingMove = "archive"
			if err := writeConsolidationJournal(*journal); err != nil {
				return fmt.Errorf("cannot journal archive intent: %w", err)
			}
			if err := movePathExclusive(entry.SourcePath, entry.BackupPath, entry.BackupSourceID); err != nil {
				return fmt.Errorf("move source %s to recovery: %w", entry.SourcePath, err)
			}
			if err := verifyMoveSource(ctx, entry.BackupPath, entry.Fingerprint, entry.BackupSourceID); err != nil {
				return fmt.Errorf("archived source identity changed during move: %w", err)
			}
			entry.MovedToBackup = true
			entry.PendingMove = ""
			if err := writeConsolidationJournal(*journal); err != nil {
				return fmt.Errorf("source moved but recovery journal update failed: %w", err)
			}
			continue
		}
		if entry.Action != "share" {
			return fmt.Errorf("unsupported journal action %q", entry.Action)
		}
		if err := ensureConsolidationDirectory(filepath.Dir(entry.CanonicalPath), true); err != nil {
			return err
		}
		if pathExists(entry.CanonicalPath) {
			if err := verifyMoveSource(ctx, entry.CanonicalPath, entry.Fingerprint, entry.CanonicalSourceID); err != nil {
				return fmt.Errorf("canonical collision changed during apply at %s: %w", entry.CanonicalPath, err)
			}
			entry.CanonicalWasPresent = true
		} else if filepath.Clean(entry.SourcePath) != filepath.Clean(entry.CanonicalPath) {
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return err
			}
			entry.PendingMove = "canonical"
			if err := writeConsolidationJournal(*journal); err != nil {
				return fmt.Errorf("cannot journal canonical move intent: %w", err)
			}
			if err := movePathExclusive(entry.SourcePath, entry.CanonicalPath, entry.CanonicalSourceID); err != nil {
				return fmt.Errorf("move source %s into canonical library: %w", entry.SourcePath, err)
			}
			if err := verifyMoveSource(ctx, entry.CanonicalPath, entry.Fingerprint, entry.CanonicalSourceID); err != nil {
				return fmt.Errorf("canonical source identity changed during move: %w", err)
			}
			entry.MovedToCanonical = true
			entry.PendingMove = ""
			if err := writeConsolidationJournal(*journal); err != nil {
				return fmt.Errorf("canonical move succeeded but recovery journal update failed: %w", err)
			}
		}
		if entry.MovedToCanonical == false && filepath.Clean(entry.SourcePath) != filepath.Clean(entry.CanonicalPath) && pathExists(entry.SourcePath) {
			if err := ensureConsolidationDirectory(filepath.Dir(entry.BackupPath), true); err != nil {
				return err
			}
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return err
			}
			entry.PendingMove = "duplicate"
			if err := writeConsolidationJournal(*journal); err != nil {
				return fmt.Errorf("cannot journal duplicate move intent: %w", err)
			}
			if err := movePathExclusive(entry.SourcePath, entry.BackupPath, entry.BackupSourceID); err != nil {
				return fmt.Errorf("move identical duplicate %s into recovery: %w", entry.SourcePath, err)
			}
			if err := verifyMoveSource(ctx, entry.BackupPath, entry.Fingerprint, entry.BackupSourceID); err != nil {
				return fmt.Errorf("duplicate backup identity changed during move: %w", err)
			}
			entry.MovedToBackup = true
			entry.PendingMove = ""
			if err := writeConsolidationJournal(*journal); err != nil {
				return fmt.Errorf("duplicate moved but recovery journal update failed: %w", err)
			}
		}
	}

	for i := range journal.Entries {
		entry := &journal.Entries[i]
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Action != "share" {
			continue
		}
		if filepath.Clean(entry.SourcePath) != filepath.Clean(entry.CanonicalPath) {
			if err := ensureLink(journal, entry, entry.SourcePath); err != nil {
				return err
			}
		}
		for _, destination := range entry.ExpectedLinks {
			if err := ensureLink(journal, entry, destination); err != nil {
				return err
			}
		}
		if err := verifyMoveSource(ctx, entry.CanonicalPath, entry.Fingerprint, entry.CanonicalSourceID); err != nil {
			return fmt.Errorf("canonical bundle failed verification after relocation: %s: %w", entry.CanonicalPath, err)
		}
	}
	return nil
}

func ensureLink(journal *consolidationJournal, entry *consolidationJournalEntry, path string) error {
	if target, ok, err := exactLink(path, entry.CanonicalPath); err != nil {
		entry.FailedLinks = uniqueJournalPaths(append(entry.FailedLinks, path))
		_ = writeConsolidationJournal(*journal)
		return err
	} else if ok {
		_ = target
		return nil
	}
	if pathExists(path) {
		entry.FailedLinks = uniqueJournalPaths(append(entry.FailedLinks, path))
		_ = writeConsolidationJournal(*journal)
		return fmt.Errorf("link path became occupied during apply: %s", path)
	}
	entry.PendingLinks = append(entry.PendingLinks, path)
	if err := writeConsolidationJournal(*journal); err != nil {
		entry.PendingLinks = removeJournalPath(entry.PendingLinks, path)
		return fmt.Errorf("cannot journal link intent for %s: %w", path, err)
	}
	if err := ensureConsolidationDirectory(filepath.Dir(path), false); err != nil {
		entry.PendingLinks = removeJournalPath(entry.PendingLinks, path)
		entry.FailedLinks = uniqueJournalPaths(append(entry.FailedLinks, path))
		_ = writeConsolidationJournal(*journal)
		return fmt.Errorf("destination directory is not safely reachable: %s: %w", filepath.Dir(path), err)
	}
	if err := symlinkExclusive(entry.CanonicalPath, path); err != nil {
		entry.PendingLinks = removeJournalPath(entry.PendingLinks, path)
		entry.FailedLinks = uniqueJournalPaths(append(entry.FailedLinks, path))
		_ = writeConsolidationJournal(*journal)
		return fmt.Errorf("create link %s: %w", path, err)
	}
	entry.CreatedLinks = append(entry.CreatedLinks, path)
	entry.PendingLinks = removeJournalPath(entry.PendingLinks, path)
	entry.FailedLinks = removeJournalPath(entry.FailedLinks, path)
	if err := writeConsolidationJournal(*journal); err != nil {
		return fmt.Errorf("link created but recovery journal update failed: %w", err)
	}
	return nil
}

func removeJournalPath(values []string, path string) []string {
	out := values[:0]
	for _, value := range values {
		if value != path {
			out = append(out, value)
		}
	}
	return out
}

func verifyMoveSource(ctx context.Context, path, expected string, expectedID SourceID) error {
	source, fingerprint, complete, reason := consolidationFingerprintPath(ctx, path)
	if expectedID == "" {
		return fmt.Errorf("recovery journal lacks the required physical source identity for %s", path)
	}
	if !complete || fingerprint != expected {
		return fmt.Errorf("source freshness check failed for %s: %s", path, reason)
	}
	if source.ID != expectedID {
		return fmt.Errorf("physical source identity changed at %s: expected %s, found %s", path, expectedID, source.ID)
	}
	return nil
}

func consolidationApplyEffectOutcomes(ctx context.Context, effects []ConsolidationEffect, journal consolidationJournal) []ConsolidationEffect {
	out := append([]ConsolidationEffect(nil), effects...)
	entries := make(map[SourceID]consolidationJournalEntry, len(journal.Entries))
	for _, entry := range journal.Entries {
		entries[entry.SourceID] = entry
	}
	for index := range out {
		effect := &out[index]
		effect.Operations = nil
		if effect.Action == "keep" {
			effect.State = "unchanged"
			continue
		}
		entry, exists := entries[effect.SourceID]
		if !exists {
			effect.State = "pending"
			effect.Operations = append(effect.Operations, ConsolidationOperationOutcome{Action: "apply", Path: effect.SourcePath, State: "pending", Reason: "journaled execution has not started"})
			continue
		}
		if effect.Action == "archive" {
			effect.Operations = append(effect.Operations, consolidationMoveOutcome(ctx, "move-to-recovery", effect.SourcePath, effect.BackupPath, effect.BundleFingerprint, entry.SourceID, entry.BackupSourceID, entry.MovedToBackup, entry.PendingMove == "archive"))
		} else if effect.Action == "share" {
			if filepath.Clean(effect.SourcePath) != filepath.Clean(effect.CanonicalPath) {
				if entry.CanonicalWasPresent {
					effect.Operations = append(effect.Operations, consolidationMoveOutcome(ctx, "move-duplicate-to-recovery", effect.SourcePath, effect.BackupPath, effect.BundleFingerprint, entry.SourceID, entry.BackupSourceID, entry.MovedToBackup, entry.PendingMove == "duplicate"))
				} else {
					effect.Operations = append(effect.Operations, consolidationMoveOutcome(ctx, "move-to-canonical", effect.SourcePath, effect.CanonicalPath, effect.BundleFingerprint, entry.SourceID, entry.CanonicalSourceID, entry.MovedToCanonical, entry.PendingMove == "canonical"))
				}
				effect.Operations = append(effect.Operations, consolidationLinkOutcome(effect.SourcePath, effect.CanonicalPath, entry))
			}
			for _, destination := range effect.Destinations {
				effect.Operations = append(effect.Operations, consolidationLinkOutcome(destination, effect.CanonicalPath, entry))
			}
		}
		effect.State = consolidationEffectOutcomeState(effect.Operations)
	}
	return out
}

func consolidationMoveOutcome(ctx context.Context, action, from, to, fingerprint string, sourceID, targetID SourceID, recorded, pending bool) ConsolidationOperationOutcome {
	operation := ConsolidationOperationOutcome{Action: action, From: from, Path: to, State: "pending", Reason: "move was not reached before apply stopped"}
	if verifyMoveSource(ctx, to, fingerprint, targetID) == nil {
		operation.State, operation.Reason = "completed", ""
		return operation
	}
	if recorded || pending {
		operation.State, operation.Reason = "failed", "expected moved bundle is not present with its journaled identity"
	}
	if sourceID == targetID && filepath.Clean(from) == filepath.Clean(to) {
		operation.State, operation.Reason = "not-needed", "source already occupies the canonical path"
	}
	return operation
}

func consolidationLinkOutcome(path, canonical string, entry consolidationJournalEntry) ConsolidationOperationOutcome {
	operation := ConsolidationOperationOutcome{Action: "create-link", Path: path, State: "pending", Reason: "link was not reached before apply stopped"}
	if _, exact, err := exactLink(path, canonical); err == nil && exact {
		operation.State, operation.Reason = "completed", ""
	} else if containsString(entry.FailedLinks, path) {
		operation.State, operation.Reason = "failed", "link creation did not produce the requested canonical reference"
	}
	return operation
}

func consolidationEffectOutcomeState(operations []ConsolidationOperationOutcome) string {
	if len(operations) == 0 {
		return "unchanged"
	}
	completed, unfinished, failed := 0, 0, false
	for _, operation := range operations {
		switch operation.State {
		case "completed", "not-needed":
			completed++
		case "failed":
			unfinished++
			failed = true
		default:
			unfinished++
		}
	}
	if unfinished == 0 {
		return "completed"
	}
	if completed > 0 {
		return "partial"
	}
	if failed {
		return "failed"
	}
	return "pending"
}

func verifyConsolidationJournal(ctx context.Context, journal consolidationJournal) error {
	for _, entry := range journal.Entries {
		if entry.Action == "archive" {
			if !entry.MovedToBackup {
				return fmt.Errorf("archive move was not recorded for %s", entry.SourcePath)
			}
			if err := verifyMoveSource(ctx, entry.BackupPath, entry.Fingerprint, entry.BackupSourceID); err != nil {
				return err
			}
			continue
		}
		if err := verifyMoveSource(ctx, entry.CanonicalPath, entry.Fingerprint, entry.CanonicalSourceID); err != nil {
			return fmt.Errorf("canonical bundle is not verified at %s: %w", entry.CanonicalPath, err)
		}
		for _, link := range append(append([]string(nil), entry.CreatedLinks...), entry.ExpectedLinks...) {
			_, ok, err := exactLink(link, entry.CanonicalPath)
			if err != nil {
				return fmt.Errorf("expected link does not resolve exactly: %s: %w", link, err)
			}
			if !ok {
				return fmt.Errorf("expected link does not resolve exactly: %s", link)
			}
		}
	}
	inventory := ScanConsolidation(ctx, journal.Scope)
	if inventory.SelectedCoverage != Complete {
		return errors.New("post-apply selected source reinspection is incomplete")
	}
	for _, entry := range journal.Entries {
		if entry.Action != "share" || len(entry.ExpectedAvailability) == 0 {
			continue
		}
		var current *ConsolidationSource
		for i := range inventory.Sources {
			if inventory.Sources[i].BundleFingerprint == entry.Fingerprint && filepath.Clean(inventory.Sources[i].SourcePath) == filepath.Clean(entry.CanonicalPath) {
				current = &inventory.Sources[i]
				break
			}
		}
		if current == nil {
			return fmt.Errorf("post-apply scan no longer sees canonical source %s", entry.CanonicalPath)
		}
		for app, expected := range entry.ExpectedAvailability {
			actual, ok := current.Availability[app]
			if !ok || string(actual.State) != expected {
				return fmt.Errorf("post-apply effective availability changed for %s: expected %s, observed %s", app, expected, actual.State)
			}
		}
	}
	return nil
}

func consolidationJournalPath(id string) string {
	return filepath.Join(consolidationBackupRoot(), id, "record.json")
}

func encodeConsolidationJournal(journal consolidationJournal) ([]byte, error) {
	encoded, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxConsolidationJournalBytes {
		return nil, fmt.Errorf("recovery journal exceeds %d MiB encoded limit", maxConsolidationJournalBytes/(1<<20))
	}
	return encoded, nil
}

func writeConsolidationJournal(journal consolidationJournal) error {
	path := consolidationJournalPath(journal.ID)
	if !filepath.IsAbs(path) {
		return errors.New("recovery journal path is not absolute")
	}
	encoded, err := encodeConsolidationJournal(journal)
	if err != nil {
		return err
	}
	return writePrivateAtomic(path, encoded)
}

func readConsolidationJournal(id string) (consolidationJournal, error) {
	if !consolidationName.MatchString(strings.TrimPrefix(id, "c-")) || !strings.HasPrefix(id, "c-") {
		return consolidationJournal{}, errors.New("backup ID has an invalid format")
	}
	path := consolidationJournalPath(id)
	parent, err := openAnchored(filepath.Dir(path), true)
	if err != nil {
		return consolidationJournal{}, err
	}
	defer parent.Close()
	var parentStat unix.Stat_t
	if err := unix.Fstat(int(parent.Fd()), &parentStat); err != nil {
		return consolidationJournal{}, err
	}
	if parentStat.Uid != uint32(os.Geteuid()) || parentStat.Mode&0o077 != 0 {
		return consolidationJournal{}, errors.New("recovery plan directory is not private to the effective user")
	}
	var before unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), filepath.Base(path), &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return consolidationJournal{}, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Uid != uint32(os.Geteuid()) || before.Mode&0o077 != 0 || before.Size > maxConsolidationJournalBytes {
		return consolidationJournal{}, errors.New("recovery journal is not a bounded regular file")
	}
	fd, err := unix.Openat(int(parent.Fd()), filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return consolidationJournal{}, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return consolidationJournal{}, err
	}
	if opened.Dev != before.Dev || opened.Ino != before.Ino || opened.Uid != uint32(os.Geteuid()) || opened.Mode&0o077 != 0 || opened.Size > maxConsolidationJournalBytes {
		return consolidationJournal{}, errors.New("recovery journal changed during inspection")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxConsolidationJournalBytes+1))
	if err != nil {
		return consolidationJournal{}, err
	}
	if len(raw) > maxConsolidationJournalBytes {
		return consolidationJournal{}, errors.New("recovery journal exceeds 4 MiB")
	}
	var journal consolidationJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return consolidationJournal{}, err
	}
	if journal.SchemaVersion != ConsolidationSchemaVersion || journal.ID != id || journal.State == "" {
		return consolidationJournal{}, errors.New("recovery journal schema or identity is invalid")
	}
	return journal, nil
}

func ensureConsolidationDirectory(path string, privateLeaf bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) == "/" {
		return errors.New("directory must be a non-root absolute path")
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), string(filepath.Separator))
	current, err := os.Open("/")
	if err != nil {
		return err
	}
	defer func() { _ = current.Close() }()
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errors.New("directory path contains an unsafe component")
		}
		fd := int(current.Fd())
		child, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if err := unix.Mkdirat(fd, part, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
				return fmt.Errorf("create directory component %s: %w", part, err)
			}
			child, openErr = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
		if openErr != nil {
			return fmt.Errorf("open directory component %s without following aliases: %w", part, openErr)
		}
		var stat unix.Stat_t
		if err := unix.Fstat(child, &stat); err != nil {
			unix.Close(child)
			return err
		}
		last := i == len(parts)-1
		if last && (stat.Uid != uint32(os.Geteuid()) || privateLeaf && stat.Mode&0o077 != 0) {
			unix.Close(child)
			return fmt.Errorf("directory %s must be owned by the effective user%s", path, map[bool]string{true: " with mode 0700", false: ""}[privateLeaf])
		}
		_ = current.Close()
		current = os.NewFile(uintptr(child), part)
	}
	return nil
}

func createConsolidationPlanDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) == "/" {
		return errors.New("consolidation plan directory must be a non-root absolute path")
	}
	parent, err := openAnchored(filepath.Dir(path), true)
	if err != nil {
		return fmt.Errorf("open private recovery parent: %w", err)
	}
	defer parent.Close()
	if err := unix.Mkdirat(int(parent.Fd()), filepath.Base(path), 0o700); err != nil {
		return fmt.Errorf("create exclusive recovery plan directory: %w", err)
	}
	if err := unix.Fsync(int(parent.Fd())); err != nil {
		_ = unix.Unlinkat(int(parent.Fd()), filepath.Base(path), unix.AT_REMOVEDIR)
		return err
	}
	fd, err := unix.Openat(int(parent.Fd()), filepath.Base(path), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Unlinkat(int(parent.Fd()), filepath.Base(path), unix.AT_REMOVEDIR)
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		return errors.New("recovery plan directory must be owned by the effective user with mode 0700")
	}
	return nil
}

func removeEmptyConsolidationPlan(id string) error {
	if !consolidationName.MatchString(strings.TrimPrefix(id, "c-")) || !strings.HasPrefix(id, "c-") {
		return errors.New("consolidation plan ID has an invalid format")
	}
	path := filepath.Join(consolidationBackupRoot(), id)
	parent, err := openAnchored(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := unix.Unlinkat(int(parent.Fd()), filepath.Base(path), unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(int(parent.Fd()))
}

func existingParentDevice(path string) (int32, error) {
	parent := filepath.Dir(path)
	for {
		f, err := openAnchored(parent, true)
		if err == nil {
			var stat unix.Stat_t
			err = unix.Fstat(int(f.Fd()), &stat)
			f.Close()
			return stat.Dev, err
		}
		if !errors.Is(err, unix.ENOENT) || parent == "/" {
			return 0, err
		}
		parent = filepath.Dir(parent)
	}
}

func movePathExclusive(source, destination string, expectedID SourceID) error {
	if !filepath.IsAbs(source) || !filepath.IsAbs(destination) || filepath.Clean(source) == filepath.Clean(destination) {
		return errors.New("move paths must be distinct absolute paths")
	}
	if expectedID == "" {
		return errors.New("move lacks a recorded physical source identity")
	}
	sourceParent, err := openAnchored(filepath.Dir(source), true)
	if err != nil {
		return fmt.Errorf("open source parent: %w", err)
	}
	defer sourceParent.Close()
	destinationParent, err := openAnchored(filepath.Dir(destination), true)
	if err != nil {
		return fmt.Errorf("open destination parent: %w", err)
	}
	defer destinationParent.Close()
	var sourceStat, destinationStat unix.Stat_t
	if err := unix.Fstatat(int(sourceParent.Fd()), filepath.Base(source), &sourceStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if sourceStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("source is no longer a directory bundle")
	}
	token, err := directoryObjectToken(source)
	if err != nil {
		return fmt.Errorf("inspect move source identity at %s: %w", source, err)
	}
	actualID := sourceID(token)
	if actualID != expectedID {
		return fmt.Errorf("move source physical identity changed at %s: expected %s, found %s", source, expectedID, actualID)
	}
	if err := unix.Fstat(int(destinationParent.Fd()), &destinationStat); err != nil {
		return err
	}
	if sourceStat.Dev != destinationStat.Dev {
		return errors.New("cross-device move is unsupported")
	}
	if err := unix.RenameatxNp(int(sourceParent.Fd()), filepath.Base(source), int(destinationParent.Fd()), filepath.Base(destination), unix.RENAME_EXCL); err != nil {
		return err
	}
	if err := unix.Fsync(int(sourceParent.Fd())); err != nil {
		return err
	}
	return unix.Fsync(int(destinationParent.Fd()))
}

func symlinkExclusive(target, path string) error {
	parent, err := openAnchored(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := unix.Symlinkat(target, int(parent.Fd()), filepath.Base(path)); err != nil {
		return err
	}
	return unix.Fsync(int(parent.Fd()))
}

func exactLink(path, target string) (string, bool, error) {
	parent, err := openAnchored(filepath.Dir(path), true)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return "", false, nil
		}
		return "", false, err
	}
	defer parent.Close()
	var stat unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), filepath.Base(path), &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return "", false, nil
		}
		return "", false, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFLNK {
		return "", false, nil
	}
	buffer := make([]byte, 4096)
	length, err := unix.Readlinkat(int(parent.Fd()), filepath.Base(path), buffer)
	if err != nil {
		return "", false, err
	}
	got := string(buffer[:length])
	return got, got == target, nil
}

func removeExactLink(path, target string) error {
	parent, err := openAnchored(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer parent.Close()
	buffer := make([]byte, 4096)
	length, err := unix.Readlinkat(int(parent.Fd()), filepath.Base(path), buffer)
	if err != nil {
		return err
	}
	if string(buffer[:length]) != target {
		return fmt.Errorf("link target changed at %s", path)
	}
	if err := unix.Unlinkat(int(parent.Fd()), filepath.Base(path), 0); err != nil {
		return err
	}
	return unix.Fsync(int(parent.Fd()))
}

func captureConsolidationSelection() (string, []byte, bool, error) {
	path, err := consolidationSelectionPath()
	if err != nil {
		return "", nil, false, err
	}
	raw, found, err := readPrivateSelection(path)
	return path, raw, found, err
}

func encodeConsolidationSelection(request ConsolidationRequest) ([]byte, error) {
	value := consolidationSelection{SchemaVersion: ConsolidationSchemaVersion, Applications: append([]string(nil), request.Applications...), Roots: append([]string(nil), request.Roots...), Library: request.Library}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxConsolidationSelectionBytes {
		return nil, fmt.Errorf("remembered consolidation selection exceeds %d KiB encoded limit", maxConsolidationSelectionBytes/1024)
	}
	return encoded, nil
}

func consolidateRecoverySeal(journal consolidationJournal, inventory ConsolidationInventory, effects []ConsolidationEffect) string {
	encoded, _ := json.Marshal(struct {
		Journal consolidationJournal
		ScanID  string
		Effects []ConsolidationEffect
	}{journal, inventory.ScanID, effects})
	digest := sha256.Sum256(encoded)
	return "r1:" + hex.EncodeToString(digest[:])
}

func sameBytesOrAbsent(path string, expected []byte, expectedExists bool) error {
	got, found, err := readPrivateSelection(path)
	if err != nil {
		return err
	}
	if !found {
		if expectedExists {
			return errors.New("expected prior selection file is missing")
		}
		return nil
	}
	if !expectedExists {
		return errors.New("selection state changed after consolidation")
	}
	if !bytes.Equal(got, expected) {
		return errors.New("selection state changed after consolidation")
	}
	return nil
}

func restoreSelection(journal consolidationJournal) error {
	return restoreSelectionWithWriter(journal, writePrivateAtomic)
}

func restoreSelectionWithWriter(journal consolidationJournal, writeSelection func(string, []byte) error) error {
	if !journal.SelectionApplied && !journal.PendingSelectionApply && !journal.PendingSelectionRestore {
		return nil
	}
	if journal.PriorSelectionExists {
		if err := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, true); err == nil {
			return nil
		}
	} else {
		if err := sameBytesOrAbsent(journal.SelectionPath, nil, false); err == nil {
			return nil
		}
	}
	if err := sameBytesOrAbsent(journal.SelectionPath, journal.AppliedSelection, true); err != nil {
		return err
	}
	if journal.PriorSelectionExists {
		return writeSelection(journal.SelectionPath, journal.PriorSelection)
	}
	parent, err := openAnchored(filepath.Dir(journal.SelectionPath), true)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := unix.Unlinkat(int(parent.Fd()), filepath.Base(journal.SelectionPath), 0); err != nil {
		return err
	}
	return unix.Fsync(int(parent.Fd()))
}

func uniqueJournalPaths(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	out := result[:0]
	for _, value := range result {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

// PreviewConsolidationRecovery checks the exact recorded state and current
// references before exposing a reversible recovery plan.
func PreviewConsolidationRecovery(ctx context.Context, id string) ConsolidationPreview {
	preview := ConsolidationPreview{SchemaVersion: ConsolidationSchemaVersion, Operation: "consolidate-recover-preview", Outcome: "failed", ExitCode: 1, State: "refused", Reason: "recovery state is not safe to restore", Choices: []ConsolidationChoice{}, Effects: []ConsolidationEffect{}, Conflicts: []ConsolidationPreviewConflict{}, Exclusions: []ConsolidationExclusion{}, SourceAvailability: []ConsolidationPreviewSource{}, Warnings: []string{}, RecoveryID: id}
	journal, err := readConsolidationJournal(id)
	if err != nil {
		return consolidationPreviewFailure(preview, "recovery-record", err.Error(), "Use the exact recovery ID printed by apply and inspect the private recovery store.")
	}
	preview.Scope = journal.Scope
	if journal.State == "recovered" {
		preview.State, preview.Reason, preview.Outcome, preview.ExitCode = "ready", "recovery was already completed; repeating it is a no-op", "success", 0
		preview.Confirmation = consolidationRecoverySeal(journal, ConsolidationInventory{ScanID: "already-recovered"}, nil)
		return preview
	}
	if journal.State != "applied" && journal.State != "partial" && journal.State != "prepared" && journal.State != "recovering" {
		return consolidationPreviewFailure(preview, "recovery-state", "recovery journal state is unsupported: "+journal.State, "Inspect the journal and preserve it for manual recovery.")
	}
	inventory := ScanConsolidation(ctx, journal.Scope)
	preview.ScanID = inventory.ScanID
	if !consolidationRecoveryCoverageAllowed(inventory, journal) {
		return consolidationPreviewFailure(preview, "incomplete-recovery-scan", "current consumer/reference inventory is incomplete", "Resolve the listed inventory evidence gaps and retry recovery preview.")
	}
	if !inventory.LibrarySafe {
		return consolidationPreviewFailure(preview, "unsafe-library", strings.Join(inventory.LibraryReasons, "; "), "Choose a dedicated path outside every discovered consumer root and custom source root before recovery.")
	}
	var effects []ConsolidationEffect
	for i := len(journal.Entries) - 1; i >= 0; i-- {
		entry := journal.Entries[i]
		if err := validateRecoveryEntry(ctx, entry, journal.Entries); err != nil {
			return consolidationPreviewFailure(preview, "recovery-freshness", err.Error(), "Preserve current files and inspect the changed bundle or link before retrying.")
		}
		if entry.Action == "archive" {
			if entry.MovedToBackup || entry.PendingMove == "archive" {
				if pathExists(entry.BackupPath) {
					effects = append(effects, ConsolidationEffect{Action: "restore-archive", SourceID: entry.SourceID, SourcePath: entry.SourcePath, BackupPath: entry.BackupPath, BundleFingerprint: entry.Fingerprint, State: "restore-original-source"})
				}
			}
			continue
		}
		links := ownedJournalLinks(entry)
		if len(links) > 0 || entry.MovedToCanonical || entry.MovedToBackup || entry.PendingMove != "" || entry.RestoredToSource {
			effects = append(effects, ConsolidationEffect{Action: "restore-share", SourceID: entry.SourceID, SourcePath: entry.SourcePath, CanonicalPath: entry.CanonicalPath, BackupPath: entry.BackupPath, BundleFingerprint: entry.Fingerprint, Destinations: links, State: "remove-owned-links-and-restore-source"})
		}
	}
	if journal.SelectionApplied || journal.PendingSelectionApply {
		if err := validateSelectionForRecovery(journal); err != nil {
			return consolidationPreviewFailure(preview, "selection-freshness", err.Error(), "Do not overwrite a later user selection; resolve it manually or restore the selection state first.")
		}
		effects = append(effects, ConsolidationEffect{Action: "restore-selection", SourcePath: journal.SelectionPath, State: "restore-previous-selection"})
	}
	if err := validateNoUnexpectedConsolidationReferences(inventory, journal); err != nil {
		return consolidationPreviewFailure(preview, "unexpected-reference", err.Error(), "Keep the canonical bundle until the new reference is reviewed and resolved.")
	}
	preview.Effects = effects
	preview.State, preview.Reason, preview.Outcome, preview.ExitCode = "ready", "recorded moves, created links, consumer references and selection state are fresh and can be restored", "success", 0
	preview.Confirmation = consolidationRecoverySeal(journal, inventory, effects)
	return preview
}

func validateRemovedRecoveryLink(ctx context.Context, path string, journalEntries []consolidationJournalEntry) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect previously removed recovery link %s: %w", path, err)
	}
	for _, restored := range journalEntries {
		if filepath.Clean(path) != filepath.Clean(restored.SourcePath) || !restored.RestoredToSource && restored.PendingRecoveryMove == "" {
			continue
		}
		if verifyMoveSource(ctx, path, restored.Fingerprint, restored.SourceID) == nil {
			return nil
		}
	}
	return fmt.Errorf("previously removed recovery link reappeared: %s", path)
}

func validateRecoveryEntry(ctx context.Context, entry consolidationJournalEntry, journalEntries []consolidationJournalEntry) error {
	if entry.Fingerprint == "" || entry.SourcePath == "" {
		return errors.New("recovery journal entry is missing source identity or fingerprint")
	}
	if entry.Action == "archive" {
		if entry.BackupSourceID == "" {
			return errors.New("recovery journal lacks the archived bundle's physical identity; preserve it for manual recovery")
		}
		backupExists := pathExists(entry.BackupPath)
		sourceExists := pathExists(entry.SourcePath)
		if backupExists {
			if sourceExists && !journalOwnsExactShareLink(entry.SourcePath, journalEntries) {
				return fmt.Errorf("both archived backup and original path exist: %s", entry.SourcePath)
			}
			if err := verifyMoveSource(ctx, entry.BackupPath, entry.Fingerprint, entry.BackupSourceID); err != nil {
				return err
			}
		} else if sourceExists {
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return err
			}
		} else if entry.MovedToBackup || entry.PendingMove == "archive" {
			return fmt.Errorf("both archived backup and source are missing: %s", entry.SourcePath)
		}
		if entry.PendingRecoveryMove == "archive" && !backupExists && sourceExists {
			return verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID)
		}
		return nil
	}
	if entry.Action != "share" {
		return fmt.Errorf("unknown recovery action %q", entry.Action)
	}
	if entry.CanonicalSourceID == "" || entry.BackupSourceID == "" {
		return errors.New("recovery journal lacks the canonical or duplicate physical identity; preserve it for manual recovery")
	}
	if entry.RestoredToSource {
		if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
			return fmt.Errorf("recorded restored source changed: %w", err)
		}
	}
	canonicalExists := pathExists(entry.CanonicalPath)
	if canonicalExists {
		if err := verifyMoveSource(ctx, entry.CanonicalPath, entry.Fingerprint, entry.CanonicalSourceID); err != nil {
			return err
		}
	}
	backupExists := entry.BackupPath != "" && pathExists(entry.BackupPath)
	if backupExists {
		if err := verifyMoveSource(ctx, entry.BackupPath, entry.Fingerprint, entry.BackupSourceID); err != nil {
			return err
		}
	}
	for _, path := range entry.RemovedLinks {
		if err := validateRemovedRecoveryLink(ctx, path, journalEntries); err != nil {
			return err
		}
	}
	for _, path := range ownedJournalLinks(entry) {
		if containsString(entry.RemovedLinks, path) || entry.RestoredToSource && filepath.Clean(path) == filepath.Clean(entry.SourcePath) {
			continue
		}
		if !pathExists(path) {
			continue
		}
		if filepath.Clean(path) == filepath.Clean(entry.SourcePath) && !entry.MovedToCanonical && !entry.MovedToBackup && entry.PendingMove == "" {
			if err := verifyMoveSource(ctx, path, entry.Fingerprint, entry.SourceID); err == nil {
				continue
			}
		}
		if _, exact, err := exactLink(path, entry.CanonicalPath); err != nil || !exact {
			return fmt.Errorf("recorded or destination link changed after apply: %s: %w", path, err)
		}
	}
	if entry.MovedToCanonical || entry.PendingMove == "canonical" {
		if !canonicalExists && !entry.RestoredToSource {
			if verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID) != nil {
				return fmt.Errorf("canonical bundle moved by this plan is missing: %s", entry.CanonicalPath)
			}
		}
	}
	if entry.MovedToBackup || entry.PendingMove == "duplicate" {
		if !backupExists && !entry.RestoredToSource {
			if !pathExists(entry.SourcePath) || verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID) != nil {
				return fmt.Errorf("duplicate backup and original source are missing: %s", entry.SourcePath)
			}
		}
	}
	return nil
}

func journalOwnsExactShareLink(path string, entries []consolidationJournalEntry) bool {
	for _, entry := range entries {
		if entry.Action != "share" || !containsString(ownedJournalLinks(entry), path) {
			continue
		}
		if _, exact, err := exactLink(path, entry.CanonicalPath); err == nil && exact {
			return true
		}
	}
	return false
}

func ownedJournalLinks(entry consolidationJournalEntry) []string {
	return uniqueJournalPaths(append(append([]string(nil), entry.CreatedLinks...), entry.PendingLinks...))
}

func validateNoUnexpectedConsolidationReferences(inventory ConsolidationInventory, journal consolidationJournal) error {
	for _, entry := range journal.Entries {
		if entry.Action != "share" || (!entry.MovedToCanonical && entry.PendingMove != "canonical" && !pathExists(entry.CanonicalPath)) {
			continue
		}
		if entry.CanonicalSourceID == "" {
			return errors.New("recovery journal lacks the canonical source identity; preserve it for manual recovery")
		}
		allowed := map[string]bool{}
		for _, path := range append(append(append([]string(nil), entry.BeforeRefs...), entry.CreatedLinks...), entry.ExpectedLinks...) {
			allowed[consolidationReferencePathKey(path)] = true
		}
		for _, path := range entry.PendingLinks {
			allowed[consolidationReferencePathKey(path)] = true
		}
		for _, source := range inventory.Sources {
			if source.ID != entry.CanonicalSourceID || !consolidationPathResolvesTo(source.SourcePath, entry.CanonicalPath) {
				continue
			}
			for _, reference := range source.KnownReferences {
				if reference.Reference.Source != entry.CanonicalSourceID || !consolidationPathResolvesTo(reference.Reference.ResolvedPath, entry.CanonicalPath) {
					continue
				}
				path := consolidationReferencePathKey(reference.Reference.DiscoveredPath)
				if !allowed[path] {
					return fmt.Errorf("new or unrecorded %s reference %s now reaches canonical bundle %s", reference.Application, reference.Reference.DiscoveredPath, entry.CanonicalPath)
				}
			}
		}
	}
	return nil
}

func consolidationReferencePathKey(path string) string {
	canonical, err := canonicalizePathParent(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(canonical)
}

func consolidationRecoveryCoverageAllowed(inventory ConsolidationInventory, journal consolidationJournal) bool {
	var archived []string
	for _, entry := range journal.Entries {
		if entry.Action == "archive" && pathExists(entry.BackupPath) {
			archived = append(archived, filepath.Clean(entry.SourcePath))
		}
	}
	for _, app := range inventory.Applications {
		if app.Failure != "" {
			if app.Selected {
				return false
			}
		}
		if !app.Selected {
			continue
		}
		for _, problem := range app.Problems {
			if !problem.LimitsCoverage {
				continue
			}
			matched := false
			for _, sourcePath := range archived {
				problemPath := filepath.Clean(problem.Path)
				if pathWithin(sourcePath, problemPath) || pathWithin(problemPath, sourcePath) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	for _, entry := range journal.Entries {
		if entry.Action != "share" {
			continue
		}
		for _, link := range entry.ExpectedLinks {
			found := false
			for _, audit := range inventory.DestinationAudits {
				if filepath.Clean(audit.DestinationRoot) != filepath.Clean(filepath.Dir(link)) {
					continue
				}
				found = true
				if len(audit.Refusals) != 0 {
					return false
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func validateSelectionForRecovery(journal consolidationJournal) error {
	if journal.PendingSelectionRestore || journal.SelectionApplied {
		if journal.PriorSelectionExists {
			if err := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, true); err == nil {
				return nil
			}
		} else if err := sameBytesOrAbsent(journal.SelectionPath, nil, false); err == nil {
			return nil
		}
	}
	if err := sameBytesOrAbsent(journal.SelectionPath, journal.AppliedSelection, true); err == nil {
		return nil
	}
	if journal.PendingSelectionApply && journal.PriorSelectionExists {
		if err := sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, true); err == nil {
			return nil
		}
	}
	if journal.PendingSelectionApply && !journal.PriorSelectionExists {
		if err := sameBytesOrAbsent(journal.SelectionPath, nil, false); err == nil {
			return nil
		}
	}
	return errors.New("remembered selection no longer matches this plan's applied or prior state")
}

// ApplyConsolidationRecovery restores only objects journaled by this plan and
// only after a second complete preview produces the exact supplied token.
func ApplyConsolidationRecovery(ctx context.Context, id, confirmation string) ConsolidationApplyOutcome {
	out := ConsolidationApplyOutcome{SchemaVersion: ConsolidationSchemaVersion, Operation: "consolidate-recover", Outcome: "failed", ExitCode: 1, State: "refused", Reason: "recovery confirmation does not match current evidence", RecoveryID: id, Effects: []ConsolidationEffect{}}
	preview := PreviewConsolidationRecovery(ctx, id)
	if preview.State != "ready" || preview.ExitCode != 0 {
		out.Failure = preview.Failure
		out.Reason = preview.Reason
		return out
	}
	if confirmation == "" || confirmation != preview.Confirmation {
		out.Failure = &ConsolidationFailure{Code: "recovery-confirmation-mismatch", Message: "exact recovery preview confirmation token does not match current evidence", Action: "Review the recovery preview and repeat recover with its exact token."}
		out.Reason = out.Failure.Message
		return out
	}
	latest := PreviewConsolidationRecovery(ctx, id)
	if latest.State != "ready" || latest.ExitCode != 0 || latest.Confirmation != confirmation {
		out.Failure = &ConsolidationFailure{Code: "stale-recovery", Message: "recovery evidence changed after confirmation", Action: "Review a fresh recovery preview and repeat with its exact token."}
		out.Reason = out.Failure.Message
		return out
	}
	preview = latest
	journal, err := readConsolidationJournal(id)
	if err != nil {
		out.Failure = &ConsolidationFailure{Code: "recovery-record", Message: err.Error(), Action: "Inspect the private recovery store."}
		out.Reason = err.Error()
		return out
	}
	out.Effects = consolidationRecoveryEffectOutcomes(ctx, preview.Effects, journal)
	if journal.State == "recovered" {
		out.State, out.Outcome, out.ExitCode, out.Reason, out.Verified = "recovered", "success", 0, "recovery was already completed; no files changed", true
		return out
	}
	for _, entry := range journal.Entries {
		if err := validateRecoveryEntry(ctx, entry, journal.Entries); err != nil {
			out.Failure = &ConsolidationFailure{Code: "stale-recovery", Message: err.Error(), Action: "Review the current paths and run a new recovery preview before any recovery write."}
			out.Reason = out.Failure.Message
			return out
		}
	}
	journal.State = "recovering"
	if err := writeConsolidationJournal(journal); err != nil {
		out.Failure = &ConsolidationFailure{Code: "recovery-journal", Message: err.Error(), Action: "Resolve the private journal write failure before recovery."}
		out.Reason = err.Error()
		return out
	}
	partial := func(err error) ConsolidationApplyOutcome {
		journal.State = "recovering"
		_ = writeConsolidationJournal(journal)
		out.State, out.Outcome, out.ExitCode, out.Reason = "partial", "failed", 1, err.Error()
		out.Failure = &ConsolidationFailure{Code: "partial-recovery", Message: err.Error(), Action: "Review the recovery journal and rerun recover preview; completed exact steps are retained."}
		out.Effects = consolidationRecoveryEffectOutcomes(ctx, preview.Effects, journal)
		return out
	}
	// Remove every owned link before restoring any source directory. A selected
	// duplicate can occupy another entry's original destination path, so
	// restoring it while a sibling entry still owns that link would fail or
	// overwrite an independent source.
	if err := removeConsolidationRecoveryLinks(ctx, &journal); err != nil {
		return partial(err)
	}
	if err := restoreConsolidationRecoverySources(ctx, &journal); err != nil {
		return partial(err)
	}
	if journal.SelectionApplied || journal.PendingSelectionApply {
		journal.PendingSelectionRestore = true
		if err := writeConsolidationJournal(journal); err != nil {
			return partial(err)
		}
		if err := restoreSelection(journal); err != nil {
			return partial(err)
		}
		journal.SelectionRestored = true
		journal.PendingSelectionRestore = false
		if err := writeConsolidationJournal(journal); err != nil {
			return partial(err)
		}
	}
	for _, entry := range journal.Entries {
		if entry.Action == "archive" || entry.MovedToBackup || entry.MovedToCanonical {
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return partial(err)
			}
		}
	}
	journal.State = "recovered"
	if err := writeConsolidationJournal(journal); err != nil {
		return partial(err)
	}
	out.State, out.Outcome, out.ExitCode, out.Reason, out.Verified = "recovered", "success", 0, "all journaled moves and links were restored and verified", true
	out.Effects = consolidationRecoveryEffectOutcomes(ctx, preview.Effects, journal)
	return out
}

func removeConsolidationRecoveryLinks(ctx context.Context, journal *consolidationJournal) error {
	for i := len(journal.Entries) - 1; i >= 0; i-- {
		entry := &journal.Entries[i]
		for _, link := range entry.RemovedLinks {
			if err := validateRemovedRecoveryLink(ctx, link, journal.Entries); err != nil {
				return err
			}
		}
		if entry.RestoredToSource {
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return fmt.Errorf("recorded restored source changed: %w", err)
			}
			continue
		}
		for _, link := range ownedJournalLinks(*entry) {
			if containsString(entry.RemovedLinks, link) {
				if err := validateRemovedRecoveryLink(ctx, link, journal.Entries); err != nil {
					return err
				}
				continue
			}
			if !pathExists(link) {
				continue
			}
			if pathExists(entry.CanonicalPath) {
				if err := verifyMoveSource(ctx, entry.CanonicalPath, entry.Fingerprint, entry.CanonicalSourceID); err != nil {
					return err
				}
			} else if entry.RestoredToSource || filepath.Clean(entry.SourcePath) == filepath.Clean(entry.CanonicalPath) {
				if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("canonical source identity is missing before removing link %s", link)
			}
			if filepath.Clean(link) == filepath.Clean(entry.SourcePath) && verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID) == nil {
				continue
			}
			entry.PendingRecoveryLinks = append(entry.PendingRecoveryLinks, link)
			if err := writeConsolidationJournal(*journal); err != nil {
				return err
			}
			if err := removeExactLink(link, entry.CanonicalPath); err != nil {
				return err
			}
			entry.PendingRecoveryLinks = removeJournalPath(entry.PendingRecoveryLinks, link)
			entry.RemovedLinks = append(entry.RemovedLinks, link)
			if err := writeConsolidationJournal(*journal); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoreConsolidationRecoverySources(ctx context.Context, journal *consolidationJournal) error {
	for i := len(journal.Entries) - 1; i >= 0; i-- {
		entry := &journal.Entries[i]
		for _, link := range entry.RemovedLinks {
			if err := validateRemovedRecoveryLink(ctx, link, journal.Entries); err != nil {
				return err
			}
		}
		if entry.RestoredToSource {
			continue
		}
		if entry.PendingRecoveryMove != "" && verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID) == nil {
			entry.RestoredToSource = true
			entry.PendingRecoveryMove = ""
			if err := writeConsolidationJournal(*journal); err != nil {
				return err
			}
			continue
		}
		if entry.Action == "archive" {
			if pathExists(entry.BackupPath) {
				if pathExists(entry.SourcePath) {
					return fmt.Errorf("original source path became occupied: %s", entry.SourcePath)
				}
				if err := verifyMoveSource(ctx, entry.BackupPath, entry.Fingerprint, entry.BackupSourceID); err != nil {
					return err
				}
				entry.PendingRecoveryMove = "archive"
				if err := writeConsolidationJournal(*journal); err != nil {
					return err
				}
				if err := movePathExclusive(entry.BackupPath, entry.SourcePath, entry.BackupSourceID); err != nil {
					return err
				}
				if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
					return err
				}
			}
			entry.RestoredToSource = true
			entry.PendingRecoveryMove = ""
			if err := writeConsolidationJournal(*journal); err != nil {
				return err
			}
			continue
		}
		if (entry.MovedToBackup || entry.PendingMove == "duplicate") && pathExists(entry.BackupPath) {
			if pathExists(entry.SourcePath) {
				return fmt.Errorf("original source path became occupied: %s", entry.SourcePath)
			}
			if err := verifyMoveSource(ctx, entry.BackupPath, entry.Fingerprint, entry.BackupSourceID); err != nil {
				return err
			}
			entry.PendingRecoveryMove = "duplicate"
			if err := writeConsolidationJournal(*journal); err != nil {
				return err
			}
			if err := movePathExclusive(entry.BackupPath, entry.SourcePath, entry.BackupSourceID); err != nil {
				return err
			}
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return err
			}
		}
		if (entry.MovedToCanonical || entry.PendingMove == "canonical") && pathExists(entry.CanonicalPath) {
			if pathExists(entry.SourcePath) {
				return fmt.Errorf("original source path became occupied: %s", entry.SourcePath)
			}
			if err := verifyMoveSource(ctx, entry.CanonicalPath, entry.Fingerprint, entry.CanonicalSourceID); err != nil {
				return err
			}
			entry.PendingRecoveryMove = "canonical"
			if err := writeConsolidationJournal(*journal); err != nil {
				return err
			}
			if err := movePathExclusive(entry.CanonicalPath, entry.SourcePath, entry.CanonicalSourceID); err != nil {
				return err
			}
			if err := verifyMoveSource(ctx, entry.SourcePath, entry.Fingerprint, entry.SourceID); err != nil {
				return err
			}
		}
		entry.RestoredToSource = true
		entry.PendingRecoveryMove = ""
		if err := writeConsolidationJournal(*journal); err != nil {
			return err
		}
	}
	return nil
}

func consolidationRecoveryEffectOutcomes(ctx context.Context, effects []ConsolidationEffect, journal consolidationJournal) []ConsolidationEffect {
	out := append([]ConsolidationEffect(nil), effects...)
	entries := make(map[SourceID]consolidationJournalEntry, len(journal.Entries))
	for _, entry := range journal.Entries {
		entries[entry.SourceID] = entry
	}
	for index := range out {
		effect := &out[index]
		effect.Operations = nil
		switch effect.Action {
		case "restore-selection":
			operation := ConsolidationOperationOutcome{Action: "restore-selection", Path: effect.SourcePath, State: "pending", Reason: "selection restoration was not reached"}
			if journal.SelectionRestored || consolidationPriorSelectionIsPresent(journal) {
				operation.State, operation.Reason = "completed", ""
			} else if journal.PendingSelectionRestore {
				operation.State, operation.Reason = "failed", "previous selection is not present after the recorded restore attempt"
			}
			effect.Operations = append(effect.Operations, operation)
		case "restore-archive":
			entry := entries[effect.SourceID]
			effect.Operations = append(effect.Operations, consolidationRecoveryMoveOutcome(ctx, effect.BackupPath, effect.SourcePath, effect.BundleFingerprint, entry, "archive"))
		case "restore-share":
			entry := entries[effect.SourceID]
			for _, path := range effect.Destinations {
				effect.Operations = append(effect.Operations, consolidationRemoveLinkOutcome(ctx, path, effect.CanonicalPath, entry, journal.Entries))
			}
			if entry.MovedToBackup || entry.PendingMove == "duplicate" {
				effect.Operations = append(effect.Operations, consolidationRecoveryMoveOutcome(ctx, effect.BackupPath, effect.SourcePath, effect.BundleFingerprint, entry, "duplicate"))
			} else if entry.MovedToCanonical || entry.PendingMove == "canonical" {
				effect.Operations = append(effect.Operations, consolidationRecoveryMoveOutcome(ctx, effect.CanonicalPath, effect.SourcePath, effect.BundleFingerprint, entry, "canonical"))
			} else {
				effect.Operations = append(effect.Operations, ConsolidationOperationOutcome{Action: "restore-source", Path: effect.SourcePath, State: "not-needed", Reason: "the original source was not moved"})
			}
		}
		effect.State = consolidationEffectOutcomeState(effect.Operations)
	}
	return out
}

func consolidationPriorSelectionIsPresent(journal consolidationJournal) bool {
	if journal.PriorSelectionExists {
		return sameBytesOrAbsent(journal.SelectionPath, journal.PriorSelection, true) == nil
	}
	return sameBytesOrAbsent(journal.SelectionPath, nil, false) == nil
}

func consolidationRecoveryMoveOutcome(ctx context.Context, from, to, fingerprint string, entry consolidationJournalEntry, action string) ConsolidationOperationOutcome {
	operation := ConsolidationOperationOutcome{Action: "restore-source", From: from, Path: to, State: "pending", Reason: "source restoration was not reached"}
	if verifyMoveSource(ctx, to, fingerprint, entry.SourceID) == nil {
		operation.State, operation.Reason = "completed", ""
	} else if entry.PendingRecoveryMove == action {
		operation.State, operation.Reason = "failed", "original source is not present with its journaled identity after the restore attempt"
	}
	return operation
}

func consolidationRemoveLinkOutcome(ctx context.Context, path, canonical string, entry consolidationJournalEntry, entries []consolidationJournalEntry) ConsolidationOperationOutcome {
	operation := ConsolidationOperationOutcome{Action: "remove-link", Path: path, State: "pending", Reason: "link removal was not reached"}
	for _, restored := range entries {
		if filepath.Clean(path) == filepath.Clean(restored.SourcePath) && verifyMoveSource(ctx, path, restored.Fingerprint, restored.SourceID) == nil {
			operation.State, operation.Reason = "completed", ""
			return operation
		}
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		operation.State, operation.Reason = "completed", ""
		return operation
	} else if err != nil {
		operation.State, operation.Reason = "failed", "link path cannot be inspected after recovery"
		return operation
	}
	if _, exact, err := exactLink(path, canonical); err == nil && exact {
		if containsString(entry.PendingRecoveryLinks, path) {
			operation.State, operation.Reason = "failed", "canonical link remains after the recorded removal attempt"
		}
		return operation
	}
	operation.State, operation.Reason = "failed", "link path contains an object other than the original source or exact canonical link"
	return operation
}

func consolidationRecoverySeal(journal consolidationJournal, inventory ConsolidationInventory, effects []ConsolidationEffect) string {
	encoded, _ := json.Marshal(struct {
		Journal consolidationJournal
		ScanID  string
		Effects []ConsolidationEffect
	}{journal, inventory.ScanID, effects})
	digest := sha256.Sum256(encoded)
	return "r1:" + hex.EncodeToString(digest[:])
}

var _ = bytes.Equal
var _ = uniqueJournalPaths
