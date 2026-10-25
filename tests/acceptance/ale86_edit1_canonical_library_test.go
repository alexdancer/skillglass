package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestEDIT1EditedCanonicalLibrarySourceCanBeDeduplicatedOnFreshRun(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)

	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "edit1-shared")
	codexSource := filepath.Join(fx.home, ".codex", "skills", "edit1-shared")
	original := []byte("---\nname: edit1-shared\ndescription: original complete bundle\n---\nOriginal instructions.\n")
	updated := []byte("---\nname: edit1-shared\ndescription: edited through shared Pi link\n---\nUpdated instructions retained by the user.\n")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), original, 0o644)

	initial := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "edited-library/edit1-initial-scan.json", initial.stdout)
	assertExit(t, initial, 0)
	var initialInventory collection.ConsolidationInventory
	decodeJSON(t, initial.stdout, &initialInventory)
	if initialInventory.SelectedCoverage != collection.Complete || !initialInventory.LibrarySafe {
		t.Fatalf("initial selected scan incomplete or unsafe: coverage=%s library_safe=%t reasons=%v; evidence: edited-library/edit1-initial-scan.json", initialInventory.SelectedCoverage, initialInventory.LibrarySafe, initialInventory.LibraryReasons)
	}
	initialSource := sourceAtPath(t, initialInventory, piSource)
	if !initialSource.Eligible || initialSource.FingerprintCoverage != collection.Complete || initialSource.BundleFingerprint == "" {
		t.Fatalf("initial source is not complete and eligible: %#v", initialSource)
	}
	oldFingerprint := initialSource.BundleFingerprint
	initialDraft := buildConsolidationDraft(initialInventory, map[collection.SourceID]string{initialSource.ID: "share"})
	initialDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "edit1-initial-draft.json")
	writeConsolidationDraft(t, initialDraftPath, initialDraft)
	writeEvidence(t, "edited-library/edit1-initial-draft.json", mustMarshalJSON(t, initialDraft))
	initialPreview := fx.run(t, "consolidate", "preview", "--draft", initialDraftPath, "--json")
	writeEvidence(t, "edited-library/edit1-initial-preview.json", initialPreview.stdout)
	assertExit(t, initialPreview, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, initialPreview.stdout, &preview)
	if preview.State != "ready" || preview.Confirmation == "" {
		t.Fatalf("initial plan not ready: state=%s failure=%#v; evidence: edited-library/edit1-initial-preview.json", preview.State, preview.Failure)
	}
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", initialDraftPath, "--confirm", preview.Confirmation, "--json")
	writeEvidence(t, "edited-library/edit1-initial-apply.json", apply.stdout)
	assertExit(t, apply, 0)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &applied)
	if applied.State != "applied" || !applied.Verified {
		t.Fatalf("initial apply not verified: state=%s failure=%#v", applied.State, applied.Failure)
	}
	initialRecoveryID := applied.RecoveryID
	canonical := filepath.Join(library, oldFingerprint)
	for _, link := range []string{piSource, codexSource} {
		info, err := os.Lstat(link)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("initial source path %s is not the expected retained symlink: mode=%v err=%v", link, info, err)
		}
		resolved, err := filepath.EvalSymlinks(link)
		if err != nil || filepath.Clean(resolved) != filepath.Clean(canonical) {
			t.Fatalf("initial source path %s resolves to %s, want %s (err=%v)", link, resolved, canonical, err)
		}
	}

	// Simulate an ordinary user edit through one application reference, then a
	// newly installed independent copy with the exact current bytes in another.
	if err := os.WriteFile(filepath.Join(piSource, "SKILL.md"), updated, 0o644); err != nil {
		t.Fatalf("edit the shared source through Pi's retained symlink: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(canonical, "SKILL.md")); err != nil || string(got) != string(updated) {
		t.Fatalf("Pi edit did not reach the canonical source: %v", err)
	}
	staleRecovery := fx.run(t, "consolidate", "recover", "--backup", initialRecoveryID, "--json")
	writeEvidence(t, "edited-library/edit1-old-recovery-after-edit.json", staleRecovery.stdout)
	assertExit(t, staleRecovery, 1)
	var staleRecoveryPreview collection.ConsolidationPreview
	decodeJSON(t, staleRecovery.stdout, &staleRecoveryPreview)
	if staleRecoveryPreview.State != "refused" || staleRecoveryPreview.Failure == nil {
		t.Fatalf("the original recovery must refuse to overwrite the later edit: %#v", staleRecoveryPreview)
	}
	if err := os.Remove(codexSource); err != nil {
		t.Fatalf("remove Codex's fixture link without touching its target: %v", err)
	}
	if err := os.Mkdir(codexSource, 0o755); err != nil {
		t.Fatalf("create independent Codex installation: %v", err)
	}
	writeFile(t, filepath.Join(codexSource, "SKILL.md"), updated, 0o644)
	if info, err := os.Lstat(codexSource); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("Codex copy is not an independent real directory: mode=%v err=%v", info, err)
	}

	beforeFreshScan := treeManifest(t, fx.base)
	fresh := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "edited-library/edit1-fresh-scan.json", fresh.stdout)
	assertExit(t, fresh, 0)
	assertTreeUnchanged(t, beforeFreshScan, treeManifest(t, fx.base))
	var freshInventory collection.ConsolidationInventory
	decodeJSON(t, fresh.stdout, &freshInventory)
	if freshInventory.SelectedCoverage != collection.Complete {
		t.Fatalf("fresh selected scan incomplete: coverage=%s; evidence: edited-library/edit1-fresh-scan.json", freshInventory.SelectedCoverage)
	}
	librarySource := sourceAtPath(t, freshInventory, canonical)
	copySource := sourceAtPath(t, freshInventory, codexSource)
	if librarySource.FingerprintCoverage != collection.Complete || copySource.FingerprintCoverage != collection.Complete || librarySource.BundleFingerprint == "" || librarySource.BundleFingerprint != copySource.BundleFingerprint || librarySource.BundleFingerprint == oldFingerprint {
		t.Fatalf("fixture did not produce complete identical edited bundles: old=%s library=%#v copy=%#v", oldFingerprint, librarySource, copySource)
	}

	var failures []string
	if !freshInventory.LibrarySafe {
		failures = append(failures, "fresh scan marked the edited existing canonical library unsafe: "+strings.Join(freshInventory.LibraryReasons, "; "))
	}
	if !librarySource.Eligible {
		failures = append(failures, "edited canonical source should remain recognized for a fresh explicit consolidation plan, got "+librarySource.RelocationScope+": "+strings.Join(librarySource.BlockedReasons, "; "))
	}
	if !copySource.Eligible {
		failures = append(failures, "new independent Codex copy should be eligible, got "+copySource.RelocationScope+": "+strings.Join(copySource.BlockedReasons, "; "))
	}
	if copySource.Eligible {
		shareIDs := map[collection.SourceID]string{copySource.ID: "share"}
		freshDraft := buildConsolidationDraft(freshInventory, shareIDs)
		freshDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "edit1-fresh-draft.json")
		writeConsolidationDraft(t, freshDraftPath, freshDraft)
		freshDraftBytes := mustMarshalJSON(t, freshDraft)
		writeEvidence(t, "edited-library/edit1-fresh-draft.json", freshDraftBytes)
		beforePreview := treeManifest(t, fx.base)
		freshPreviewResult := fx.run(t, "consolidate", "preview", "--draft", freshDraftPath, "--json")
		writeEvidence(t, "edited-library/edit1-fresh-preview.json", freshPreviewResult.stdout)
		var freshPreview collection.ConsolidationPreview
		decodeJSON(t, freshPreviewResult.stdout, &freshPreview)
		assertTreeUnchanged(t, beforePreview, treeManifest(t, fx.base))
		if freshPreview.State != "ready" || freshPreview.ExitCode != 0 || freshPreview.Failure != nil {
			code, message := "none", freshPreview.Reason
			if freshPreview.Failure != nil {
				code, message = freshPreview.Failure.Code, freshPreview.Failure.Message
			}
			failures = append(failures, "fresh identical-bundle dedup preview should be ready without overwriting the edit, got "+freshPreview.State+" "+code+": "+message)
		} else {
			freshApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", freshDraftPath, "--confirm", freshPreview.Confirmation, "--json")
			writeEvidence(t, "edited-library/edit1-fresh-apply.json", freshApply.stdout)
			assertExit(t, freshApply, 0)
			var freshOutcome collection.ConsolidationApplyOutcome
			decodeJSON(t, freshApply.stdout, &freshOutcome)
			if freshOutcome.State != "applied" || !freshOutcome.Verified {
				failures = append(failures, "fresh dedup apply did not verify: state="+freshOutcome.State+" reason="+freshOutcome.Reason)
			} else {
				if freshOutcome.RecoveryID == "" {
					failures = append(failures, "fresh duplicate apply did not return a recovery ID")
				}
				check := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
				writeEvidence(t, "edited-library/edit1-post-apply-scan.json", check.stdout)
				assertExit(t, check, 0)
				var finalInventory collection.ConsolidationInventory
				decodeJSON(t, check.stdout, &finalInventory)
				if final := sourceAtPath(t, finalInventory, canonical); !final.Eligible || final.BundleFingerprint != copySource.BundleFingerprint {
					failures = append(failures, "post-apply canonical source missing or changed")
				}
				for _, link := range []string{piSource, codexSource} {
					resolved, err := filepath.EvalSymlinks(link)
					if err != nil || filepath.Clean(resolved) != filepath.Clean(canonical) {
						failures = append(failures, "post-apply selected reference "+link+" does not resolve to the single updated canonical directory")
					}
				}
				if got, err := os.ReadFile(filepath.Join(canonical, "SKILL.md")); err != nil || string(got) != string(updated) {
					failures = append(failures, "post-apply canonical content did not preserve the user's later edit")
				}
				if _, err := os.Lstat(filepath.Join(library, copySource.BundleFingerprint)); err == nil {
					failures = append(failures, "fresh dedup re-keyed the edited canonical source instead of preserving its stable path")
				} else if !os.IsNotExist(err) {
					failures = append(failures, "cannot verify the new fingerprint path is absent: "+err.Error())
				}
				safeRecovery := fx.run(t, "consolidate", "recover", "--backup", freshOutcome.RecoveryID, "--json")
				writeEvidence(t, "edited-library/edit1-fresh-recovery-preview.json", safeRecovery.stdout)
				assertExit(t, safeRecovery, 0)
				var safeRecoveryPreview collection.ConsolidationPreview
				decodeJSON(t, safeRecovery.stdout, &safeRecoveryPreview)
				if safeRecoveryPreview.State != "ready" || safeRecoveryPreview.Confirmation == "" {
					failures = append(failures, "fresh dedup recovery should preview while the edited canonical remains unchanged")
				} else {
					recoveredCopy := changeCommand(t, fx, "", "consolidate", "recover", "--backup", freshOutcome.RecoveryID, "--confirm", safeRecoveryPreview.Confirmation, "--json")
					writeEvidence(t, "edited-library/edit1-fresh-recovery-apply.json", recoveredCopy.stdout)
					assertExit(t, recoveredCopy, 0)
					var recoveredOutcome collection.ConsolidationApplyOutcome
					decodeJSON(t, recoveredCopy.stdout, &recoveredOutcome)
					if recoveredOutcome.State != "recovered" || !recoveredOutcome.Verified {
						failures = append(failures, "fresh dedup recovery did not restore the independent duplicate")
					} else {
						if got, err := os.ReadFile(filepath.Join(canonical, "SKILL.md")); err != nil || string(got) != string(updated) {
							failures = append(failures, "successful recovery did not preserve the already-edited canonical source")
						}
						if info, err := os.Lstat(codexSource); err != nil || info.Mode()&os.ModeSymlink != 0 {
							failures = append(failures, "successful recovery did not restore the new independent copy")
						}
						if resolved, err := filepath.EvalSymlinks(piSource); err != nil || filepath.Clean(resolved) != filepath.Clean(canonical) {
							failures = append(failures, "successful recovery did not preserve the edited canonical source's pre-existing Pi reference")
						}

						// Create a second fresh journal so recovery can be checked against a
						// later edit without sacrificing the successful restoration above.
						rescan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
						assertExit(t, rescan, 0)
						var restoredInventory collection.ConsolidationInventory
						decodeJSON(t, rescan.stdout, &restoredInventory)
						restoredCopy := sourceAtPath(t, restoredInventory, codexSource)
						secondDraft := buildConsolidationDraft(restoredInventory, map[collection.SourceID]string{restoredCopy.ID: "share"})
						secondChoice := consolidationChoice(t, &secondDraft, restoredCopy.ID)
						for destinationIndex := range secondChoice.Destinations {
							secondChoice.Destinations[destinationIndex].Reason = "fresh re-approval for later-edit recovery verification"
						}
						secondDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "edit1-later-edit-draft.json")
						writeConsolidationDraft(t, secondDraftPath, secondDraft)
						secondPreviewResult := fx.run(t, "consolidate", "preview", "--draft", secondDraftPath, "--json")
						assertExit(t, secondPreviewResult, 0)
						var secondPreview collection.ConsolidationPreview
						decodeJSON(t, secondPreviewResult.stdout, &secondPreview)
						secondApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", secondDraftPath, "--confirm", secondPreview.Confirmation, "--json")
						assertExit(t, secondApply, 0)
						var secondOutcome collection.ConsolidationApplyOutcome
						decodeJSON(t, secondApply.stdout, &secondOutcome)
						if secondOutcome.State != "applied" || !secondOutcome.Verified {
							failures = append(failures, "second fresh dedup apply did not verify")
						} else {
							updatedAfterApply := []byte("---\nname: edit1-shared\ndescription: later edit after repeat consolidation\n---\nA later user edit must survive recovery preview.\n")
							if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), updatedAfterApply, 0o644); err != nil {
								failures = append(failures, "cannot simulate a later edit before recovery: "+err.Error())
							} else {
								laterRecovery := fx.run(t, "consolidate", "recover", "--backup", secondOutcome.RecoveryID, "--json")
								writeEvidence(t, "edited-library/edit1-new-recovery-after-later-edit.json", laterRecovery.stdout)
								assertExit(t, laterRecovery, 1)
								var laterRecoveryPreview collection.ConsolidationPreview
								decodeJSON(t, laterRecovery.stdout, &laterRecoveryPreview)
								if laterRecoveryPreview.State != "refused" || laterRecoveryPreview.Failure == nil {
									failures = append(failures, "recovery must refuse to overwrite a later edit to the stable canonical source")
								}
								if got, err := os.ReadFile(filepath.Join(canonical, "SKILL.md")); err != nil || string(got) != string(updatedAfterApply) {
									failures = append(failures, "recovery refusal did not preserve the later canonical edit")
								}
							}
						}
					}
				}
			}
		}
	}
	if len(failures) != 0 {
		t.Fatalf("EDIT-1 requirement findings: %s; complete evidence is under edited-library/", strings.Join(failures, " | "))
	}
}

func TestALE86ArbitraryLibraryAliasDoesNotGainOwnershipFromItsPath(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	foreignPath := filepath.Join(library, strings.Repeat("a", 64))
	writeFile(t, filepath.Join(foreignPath, "SKILL.md"), []byte("---\nname: foreign-source\ndescription: not previously consolidated\n---\nKeep this source in place.\n"), 0o644)
	piAlias := filepath.Join(fx.home, ".pi", "agent", "skills", "foreign-source")
	if err := os.MkdirAll(filepath.Dir(piAlias), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignPath, piAlias); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--library", library, "--json")
	writeEvidence(t, "edited-library/arbitrary-library-alias-scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	if inventory.SelectedCoverage != collection.Complete {
		t.Fatalf("arbitrary library source fixture was not completely scanned: %s", inventory.SelectedCoverage)
	}
	source := sourceAtPath(t, inventory, foreignPath)
	if source.Eligible || source.RelocationScope != "outside-selected-personal-roots" {
		t.Fatalf("arbitrary library alias acquired relocation authority without a matching current digest or successful journal: %#v", source)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func sourceAtPath(t *testing.T, inventory collection.ConsolidationInventory, path string) collection.ConsolidationSource {
	t.Helper()
	for _, source := range inventory.Sources {
		if filepath.Clean(source.SourcePath) == filepath.Clean(path) {
			return source
		}
	}
	t.Fatalf("source path %s is missing from scan", path)
	return collection.ConsolidationSource{}
}

func buildConsolidationDraft(inventory collection.ConsolidationInventory, actions map[collection.SourceID]string) collection.ConsolidationDraft {
	selected := make(map[string]bool, len(inventory.Scope.Applications))
	for _, app := range inventory.Scope.Applications {
		selected[app] = true
	}
	draft := collection.ConsolidationDraft{SchemaVersion: 1, Scope: inventory.Scope, ScanID: inventory.ScanID, Choices: []collection.ConsolidationChoice{}}
	for _, source := range inventory.Sources {
		if !source.Eligible {
			continue
		}
		action := actions[source.ID]
		if action == "" {
			action = "keep"
		}
		choice := collection.ConsolidationChoice{SourceID: source.ID, SourcePath: source.SourcePath, BundleFingerprint: source.BundleFingerprint, Action: action}
		if action == "share" {
			choice.CanonicalName = source.Name
			choice.Destinations = []collection.ConsolidationDestinationChoice{}
			for _, app := range inventory.Scope.Applications {
				assessment := collection.ConsolidationDestinationChoice{Application: app, Portability: "compatible", Reason: "fixture bundle contains only application-neutral instructions"}
				for _, audit := range inventory.DestinationAudits {
					if audit.Application != app {
						continue
					}
					for _, consumer := range audit.PotentialConsumers {
						if !selected[consumer.Application] {
							assessment.AcknowledgedConsumers = append(assessment.AcknowledgedConsumers, collection.ConsolidationConsumerAcknowledgment{Application: consumer.Application, EvidenceIDs: []string{consumer.EvidenceID}, UserApproved: true})
						}
					}
				}
				choice.Destinations = append(choice.Destinations, assessment)
			}
		}
		draft.Choices = append(draft.Choices, choice)
	}
	return draft
}

func writeConsolidationDraft(t *testing.T, path string, draft collection.ConsolidationDraft) {
	t.Helper()
	writeFile(t, path, mustMarshalJSON(t, draft), 0o600)
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}
