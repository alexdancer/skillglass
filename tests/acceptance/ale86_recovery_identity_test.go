package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

type recoveryJournalView struct {
	Entries []struct {
		SourceID         string   `json:"source_id"`
		Action           string   `json:"action"`
		SourcePath       string   `json:"source_path"`
		Fingerprint      string   `json:"fingerprint"`
		CanonicalPath    string   `json:"canonical_path"`
		MovedToCanonical bool     `json:"moved_to_canonical"`
		CreatedLinks     []string `json:"created_links"`
	} `json:"entries"`
}

func recoveryFixture(t *testing.T) (fixture, string, string) {
	t.Helper()
	fx := piFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".codex", "skills"), "home-canary", "home-native")
	sourceDir := filepath.Join(fx.home, ".pi", "agent", "skills", "portable-probe")
	writeFile(t, filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: portable-probe\ndescription: Inert recovery fixture.\n---\nRead references/guide.txt only.\n"), 0o600)
	writeFile(t, filepath.Join(sourceDir, "references", "guide.txt"), []byte("RECOVERY_REFERENCE_SENTINEL\n"), 0o640)
	writeFile(t, filepath.Join(sourceDir, "tools", "probe.sh"), []byte("#!/bin/sh\necho INERT_FIXTURE\n"), 0o750)
	library := filepath.Join(fx.xdgData, "recovery-library")
	return fx, sourceDir, library
}

func recoveryScan(t *testing.T, fx fixture, library, evidence string) collection.ConsolidationInventory {
	t.Helper()
	result := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	assertExit(t, result, 0)
	writeEvidence(t, evidence, result.stdout)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, result.stdout, &inventory)
	if inventory.Failure != nil || inventory.SelectedCoverage != collection.Complete {
		t.Fatalf("consolidation scan incomplete: failure=%+v selected=%s", inventory.Failure, inventory.SelectedCoverage)
	}
	return inventory
}

func recoveryDraft(t *testing.T, inventory collection.ConsolidationInventory, sourcePath string) collection.ConsolidationDraft {
	t.Helper()
	draft := collection.ConsolidationDraft{SchemaVersion: collection.ConsolidationSchemaVersion, Scope: inventory.Scope, ScanID: inventory.ScanID, Choices: []collection.ConsolidationChoice{}}
	selectedSource := false
	appDestinations := map[string]string{}
	for _, app := range inventory.Applications {
		appDestinations[app.Application] = app.DestinationRoot
	}
	for _, source := range inventory.Sources {
		if !source.Eligible {
			continue
		}
		choice := collection.ConsolidationChoice{SourceID: source.ID, SourcePath: source.SourcePath, BundleFingerprint: source.BundleFingerprint, Action: "keep"}
		if filepath.Clean(source.SourcePath) == filepath.Clean(sourcePath) {
			selectedSource = true
			choice.Action = "share"
			choice.CanonicalName = "portable-probe"
			choice.Destinations = []collection.ConsolidationDestinationChoice{}
			for _, app := range inventory.Scope.Applications {
				assessment := collection.ConsolidationDestinationChoice{Application: app, Portability: "unknown", Reason: "disposable acceptance fixture explicitly approves this unknown portability for the recovery probe", UserApproved: true}
				if app == "codex" {
					for _, audit := range inventory.DestinationAudits {
						if audit.Application != app || filepath.Clean(audit.DestinationRoot) != filepath.Clean(appDestinations[app]) {
							continue
						}
						for _, route := range audit.PotentialConsumers {
							if route.Application == "pi" || route.Application == "codex" {
								continue
							}
							assessment.AcknowledgedConsumers = append(assessment.AcknowledgedConsumers, collection.ConsolidationConsumerAcknowledgment{Application: route.Application, EvidenceIDs: []string{route.EvidenceID}, UserApproved: true})
						}
					}
				}
				choice.Destinations = append(choice.Destinations, assessment)
			}
		}
		draft.Choices = append(draft.Choices, choice)
	}
	if !selectedSource {
		t.Fatalf("eligible source %q is absent from current consolidation inventory", sourcePath)
	}
	return draft
}

func prepareRecoveryFixture(t *testing.T, fx fixture, sourcePath, library, evidencePrefix string) (collection.ConsolidationApplyOutcome, collection.ConsolidationPreview, collection.ConsolidationDraft, string, recoveryJournalView) {
	t.Helper()
	inventory := recoveryScan(t, fx, library, evidencePrefix+"/initial-scan.json")
	draft := recoveryDraft(t, inventory, sourcePath)
	draftBytes, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(fx.base, "allowed", "recovery-draft.json")
	writeFile(t, draftPath, append(draftBytes, '\n'), 0o600)
	writeEvidence(t, evidencePrefix+"/initial-draft.json", append(draftBytes, '\n'))
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, previewResult, 0)
	writeEvidence(t, evidencePrefix+"/initial-preview.json", previewResult.stdout)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" {
		t.Fatalf("initial consolidation preview refused: %+v", preview)
	}
	applyResult := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, applyResult, 0)
	writeEvidence(t, evidencePrefix+"/initial-apply.json", applyResult.stdout)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, applyResult.stdout, &applied)
	if applied.State != "applied" || applied.RecoveryID == "" {
		t.Fatalf("initial consolidation did not apply: %+v", applied)
	}
	journalPath := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", applied.RecoveryID, "record.json")
	journalBytes := readBytes(t, journalPath)
	writeEvidence(t, evidencePrefix+"/journal.json", journalBytes)
	var journal recoveryJournalView
	decodeJSON(t, journalBytes, &journal)
	return applied, preview, draft, draftPath, journal
}

func recoveryJournalEntry(t *testing.T, journal recoveryJournalView, action, sourcePath string) struct {
	SourceID         string   `json:"source_id"`
	Action           string   `json:"action"`
	SourcePath       string   `json:"source_path"`
	Fingerprint      string   `json:"fingerprint"`
	CanonicalPath    string   `json:"canonical_path"`
	MovedToCanonical bool     `json:"moved_to_canonical"`
	CreatedLinks     []string `json:"created_links"`
} {
	t.Helper()
	for _, entry := range journal.Entries {
		if entry.Action == action && filepath.Clean(entry.SourcePath) == filepath.Clean(sourcePath) {
			return entry
		}
	}
	t.Fatalf("journal lacks %s entry for %s: %+v", action, sourcePath, journal.Entries)
	return struct {
		SourceID         string   `json:"source_id"`
		Action           string   `json:"action"`
		SourcePath       string   `json:"source_path"`
		Fingerprint      string   `json:"fingerprint"`
		CanonicalPath    string   `json:"canonical_path"`
		MovedToCanonical bool     `json:"moved_to_canonical"`
		CreatedLinks     []string `json:"created_links"`
	}{}
}

func copyRecoveryBundle(t *testing.T, source, destination string) {
	t.Helper()
	info, err := os.Lstat(source)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("source bundle is not a real directory: %s: %v", source, err)
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := destination
		if relative != "." {
			target = filepath.Join(destination, relative)
		}
		item, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if item.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture copy does not accept symlinks: %s", path)
		}
		if item.IsDir() {
			if err := os.MkdirAll(target, item.Mode().Perm()); err != nil {
				return err
			}
			return os.Chmod(target, item.Mode().Perm())
		}
		if !item.Mode().IsRegular() {
			return fmt.Errorf("fixture copy encountered a special file: %s", path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, raw, item.Mode().Perm()); err != nil {
			return err
		}
		return os.Chmod(target, item.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func recoverySourceAt(t *testing.T, inventory collection.ConsolidationInventory, path string) collection.ConsolidationSource {
	t.Helper()
	for _, source := range inventory.Sources {
		if filepath.Clean(source.SourcePath) == filepath.Clean(path) {
			return source
		}
	}
	t.Fatalf("consolidation scan does not contain source path %s", path)
	return collection.ConsolidationSource{}
}

func evidenceJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func TestRECIDRecoveryDoesNotMoveAnUnrecordedReplacementObject(t *testing.T) {
	fx, sourceDir, library := recoveryFixture(t)
	evidence := "recovery-id"
	apply, _, _, _, journal := prepareRecoveryFixture(t, fx, sourceDir, library, evidence)
	entry := recoveryJournalEntry(t, journal, "share", sourceDir)
	if !entry.MovedToCanonical || entry.SourceID == "" {
		t.Fatalf("initial journal did not record the physical source move: %+v", entry)
	}
	aside := entry.CanonicalPath + "-recorded-object-aside"
	if err := os.Rename(entry.CanonicalPath, aside); err != nil {
		t.Fatal(err)
	}
	copyRecoveryBundle(t, aside, entry.CanonicalPath)
	if got := readBytes(t, filepath.Join(aside, "references", "guide.txt")); string(got) != "RECOVERY_REFERENCE_SENTINEL\n" {
		t.Fatal("asided original object lost the fixture reference")
	}
	if got := readBytes(t, filepath.Join(entry.CanonicalPath, "references", "guide.txt")); string(got) != "RECOVERY_REFERENCE_SENTINEL\n" {
		t.Fatal("replacement object did not preserve the fixture reference")
	}
	writeEvidence(t, evidence+"/replacement-layout.json", evidenceJSON(t, map[string]any{"recovery_id": apply.RecoveryID, "journal_source_id": entry.SourceID, "journal_source_path": entry.SourcePath, "canonical_path": entry.CanonicalPath, "original_object_aside": aside, "replacement_path": entry.CanonicalPath}))
	current := recoveryScan(t, fx, library, evidence+"/replacement-scan.json")
	replacement := recoverySourceAt(t, current, entry.CanonicalPath)
	if string(replacement.ID) == entry.SourceID || replacement.BundleFingerprint != entry.Fingerprint {
		t.Fatalf("replacement identity/fingerprint control failed: journal=%s/%s current=%s/%s", entry.SourceID, entry.Fingerprint, replacement.ID, replacement.BundleFingerprint)
	}
	previewResult := fx.run(t, "consolidate", "recover", "--backup", apply.RecoveryID, "--json")
	if previewResult.code != 0 && previewResult.code != 1 {
		t.Fatalf("recovery preview unexpected exit %d: %s", previewResult.code, previewResult.stderr)
	}
	writeEvidence(t, evidence+"/recovery-preview.json", previewResult.stdout)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State == "ready" {
		applyResult := changeCommand(t, fx, "", "consolidate", "recover", "--backup", apply.RecoveryID, "--confirm", preview.Confirmation, "--json")
		writeEvidence(t, evidence+"/recovery-apply.json", applyResult.stdout)
		var recovered collection.ConsolidationApplyOutcome
		decodeJSON(t, applyResult.stdout, &recovered)
		post := recoveryScan(t, fx, library, evidence+"/post-recovery-scan.json")
		var movedSource collection.ConsolidationSource
		for _, source := range post.Sources {
			if filepath.Clean(source.SourcePath) == filepath.Clean(entry.SourcePath) {
				movedSource = source
				break
			}
		}
		identityResult := map[string]any{"preview_state": preview.State, "preview_failure": preview.Failure, "apply_state": recovered.State, "apply_exit": applyResult.code, "journal_source_id": entry.SourceID, "replacement_source_id_before_recovery": replacement.ID, "source_id_after_recovery": movedSource.ID, "source_path_after_recovery": movedSource.SourcePath, "canonical_exists_after_recovery": pathExistsForAcceptance(entry.CanonicalPath), "asided_original_exists_after_recovery": pathExistsForAcceptance(aside), "replacement_arrived_at_recorded_source": movedSource.ID == replacement.ID}
		writeEvidence(t, evidence+"/identity-result.json", evidenceJSON(t, identityResult))
		if applyResult.code == 0 && recovered.State == "recovered" && movedSource.ID == replacement.ID {
			t.Fatalf("REC-ID-1: recovery restored a byte/mode-identical replacement object with SourceID %s, although the journal approved original SourceID %s; the separately preserved original remains at %s", replacement.ID, entry.SourceID, aside)
		}
		t.Fatalf("REC-ID-1: preview accepted replacement SourceID %s for journal SourceID %s; recovery result=%+v; identity evidence=%s", replacement.ID, entry.SourceID, recovered, evidence+"/identity-result.json")
	}
	if preview.State != "refused" || preview.Failure == nil {
		t.Fatalf("REC-ID-1: unexpected recovery preview result %+v", preview)
	}
	if !pathExistsForAcceptance(entry.CanonicalPath) || !pathExistsForAcceptance(aside) {
		t.Fatalf("refused preview changed the replacement or original paths: %+v", preview)
	}
	writeEvidence(t, evidence+"/identity-result.json", evidenceJSON(t, map[string]any{"disposition": "disconfirmed", "preview_state": preview.State, "failure": preview.Failure, "journal_source_id": entry.SourceID, "replacement_source_id": replacement.ID, "fingerprint": replacement.BundleFingerprint, "canonical_exists": true, "asided_original_exists": true}))
}

func TestRECREFRecoveryIgnoresAnIndependentEqualBundle(t *testing.T) {
	fx, sourceDir, library := recoveryFixture(t)
	evidence := "recovery-ref"
	apply, _, _, _, journal := prepareRecoveryFixture(t, fx, sourceDir, library, evidence)
	entry := recoveryJournalEntry(t, journal, "share", sourceDir)
	independent := filepath.Join(fx.home, ".codex", "skills", "independent-copy")
	copyRecoveryBundle(t, entry.CanonicalPath, independent)
	independentBefore := treeManifest(t, independent)
	writeEvidence(t, evidence+"/independent-copy-before.json", independentBefore)
	current := recoveryScan(t, fx, library, evidence+"/scan-with-independent-copy.json")
	var independentSource *collection.ConsolidationSource
	for i := range current.Sources {
		if filepath.Clean(current.Sources[i].SourcePath) == filepath.Clean(independent) {
			independentSource = &current.Sources[i]
			break
		}
	}
	if independentSource == nil {
		writeEvidence(t, evidence+"/claim-result.json", evidenceJSON(t, map[string]any{"disposition": "inconclusive", "reason": "native inventory did not expose the new independent copy as its own source", "recovery_entry": entry, "independent_path": independent, "sources": current.Sources}))
		return
	}
	if string(independentSource.ID) == entry.SourceID || independentSource.BundleFingerprint != entry.Fingerprint {
		writeEvidence(t, evidence+"/claim-result.json", evidenceJSON(t, map[string]any{"disposition": "inconclusive", "reason": "independent-source control did not have a distinct physical SourceID and equal complete fingerprint", "journal_source_id": entry.SourceID, "journal_fingerprint": entry.Fingerprint, "independent_source": independentSource}))
		return
	}
	previewResult := fx.run(t, "consolidate", "recover", "--backup", apply.RecoveryID, "--json")
	if previewResult.code != 0 && previewResult.code != 1 {
		t.Fatalf("recovery preview unexpected exit %d: %s", previewResult.code, previewResult.stderr)
	}
	writeEvidence(t, evidence+"/recovery-preview.json", previewResult.stdout)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State == "refused" && preview.Failure != nil && preview.Failure.Code == "unexpected-reference" && strings.Contains(preview.Failure.Message, independent) {
		writeEvidence(t, evidence+"/claim-result.json", evidenceJSON(t, map[string]any{"disposition": "established", "reason": "recovery treated an independent equal-fingerprint source path as a new reference to the canonical bundle", "failure": preview.Failure, "journal_source_id": entry.SourceID, "independent_source_id": independentSource.ID, "fingerprint": entry.Fingerprint, "independent_path": independent}))
		t.Fatalf("REC-REF-2: equal contents at independent source %s were treated as an unrecorded reference to canonical %s: %s", independent, entry.CanonicalPath, preview.Failure.Message)
	}
	if preview.State != "ready" {
		writeEvidence(t, evidence+"/claim-result.json", evidenceJSON(t, map[string]any{"disposition": "inconclusive", "reason": "recovery refused for a reason other than the claimed equal-content reference", "failure": preview.Failure, "journal_source_id": entry.SourceID, "independent_source_id": independentSource.ID, "fingerprint": entry.Fingerprint, "independent_path": independent}))
		return
	}
	applyResult := changeCommand(t, fx, "", "consolidate", "recover", "--backup", apply.RecoveryID, "--confirm", preview.Confirmation, "--json")
	writeEvidence(t, evidence+"/recovery-apply.json", applyResult.stdout)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, applyResult.stdout, &recovered)
	independentAfter := treeManifest(t, independent)
	unchanged := strings.EqualFold(fmt.Sprintf("%x", independentBefore), fmt.Sprintf("%x", independentAfter)) && string(independentBefore) == string(independentAfter)
	result := map[string]any{"disposition": "disconfirmed", "preview_state": preview.State, "apply_state": recovered.State, "apply_exit": applyResult.code, "independent_source_id": independentSource.ID, "journal_source_id": entry.SourceID, "independent_path": independent, "independent_copy_unchanged": unchanged, "independent_before": string(independentBefore), "independent_after": string(independentAfter)}
	writeEvidence(t, evidence+"/claim-result.json", evidenceJSON(t, result))
	if applyResult.code != 0 || recovered.State != "recovered" || !unchanged {
		t.Fatalf("REC-REF-2: recovery did not restore only its recorded objects while preserving the independent copy: %+v", result)
	}
}

func pathExistsForAcceptance(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
