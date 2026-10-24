package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86SelectedApplicationsAcknowledgmentApplyNoopAndRecovery(t *testing.T) {
	fx := newFixture(t)
	fx.xdgConfig = filepath.Join(fx.home, ".config")
	if err := os.MkdirAll(fx.xdgConfig, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)

	paths := map[string]string{
		"codex":    filepath.Join(fx.home, ".codex", "skills", "journey-codex"),
		"pi":       filepath.Join(fx.home, ".pi", "agent", "skills", "journey-pi"),
		"opencode": filepath.Join(fx.xdgConfig, "opencode", "skills", "journey-opencode"),
	}
	for app, path := range paths {
		writeFile(t, filepath.Join(path, "SKILL.md"), []byte(
			"---\nname: journey-"+app+"\ndescription: portable public-fixture skill\n---\nInstructions for the selected-app journey.\n",
		), 0o644)
		writeFile(t, filepath.Join(path, "references", "guide.md"), []byte("Review this supporting resource before recommending use.\n"), 0o640)
		writeFile(t, filepath.Join(path, "assets", "fixture.bin"), []byte{0x00, 0xff, 0x41, 0x00}, 0o600)
		writeFile(t, filepath.Join(path, "scripts", "inert.sh"), []byte("#!/bin/sh\nprintf SHOULD_NOT_RUN\n"), 0o750)
	}
	hermesRoot := filepath.Join(fx.home, ".hermes", "skills")
	hermesPath := filepath.Join(hermesRoot, "excluded-hermes")
	writeFile(t, filepath.Join(hermesPath, "SKILL.md"), []byte("---\nname: excluded-hermes\ndescription: excluded app canary\n---\nHermes stays outside this selected run.\n"), 0o640)
	hermesBefore := treeManifest(t, hermesRoot)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	selectionPath := filepath.Join(fx.xdgConfig, "skillglass", "consolidation.json")
	before := treeManifest(t, fx.base)

	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--app", "opencode", "--library", library, "--json")
	writeEvidence(t, "consolidation/journey-scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	if inventory.SelectedCoverage != collection.Complete || !inventory.LibrarySafe || !reflect.DeepEqual(inventory.Scope.Applications, []string{"codex", "opencode", "pi"}) || inventory.Scope.SelectionFrom != "arguments" || !strings.HasPrefix(inventory.Scope.RememberedSelectionID, "selection1:none:") {
		t.Fatalf("scan does not report the complete explicit selection: %#v", inventory.Scope)
	}
	for _, source := range inventory.Sources {
		if strings.HasPrefix(source.SourcePath, hermesRoot) || source.Name == "excluded-hermes" {
			t.Fatalf("unselected Hermes source entered the consolidation inventory: %#v", source)
		}
	}
	var codeAFRoute bool
	for _, audit := range inventory.DestinationAudits {
		for _, consumer := range audit.PotentialConsumers {
			if consumer.Application == "codeaf" {
				codeAFRoute = true
				if consumer.InstallationState != "unknown" || consumer.RuntimeState != "unknown" {
					t.Fatalf("route audit overstates installation/runtime state: %#v", consumer)
				}
			}
		}
	}
	if !codeAFRoute {
		t.Fatal("fixture did not expose the expected possible CodeAF consumer routes")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	actions := map[collection.SourceID]string{}
	for app, path := range paths {
		source := sourceAtPath(t, inventory, path)
		if !source.Eligible || source.FingerprintCoverage != collection.Complete {
			t.Fatalf("selected %s source is not fully eligible: %#v", app, source)
		}
		actions[source.ID] = "share"
	}
	draft := edit1Draft(inventory, actions)
	noAck := draft
	noAck.Choices = append([]collection.ConsolidationChoice(nil), draft.Choices...)
	for choiceIndex := range noAck.Choices {
		noAck.Choices[choiceIndex].Destinations = append([]collection.ConsolidationDestinationChoice(nil), draft.Choices[choiceIndex].Destinations...)
		for destinationIndex := range noAck.Choices[choiceIndex].Destinations {
			noAck.Choices[choiceIndex].Destinations[destinationIndex].AcknowledgedConsumers = nil
		}
	}
	noAckPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "journey-unacknowledged.json")
	edit1WriteDraft(t, noAckPath, noAck)
	previewBefore := treeManifest(t, fx.base)
	refused := fx.run(t, "consolidate", "preview", "--draft", noAckPath, "--json")
	writeEvidence(t, "consolidation/journey-unacknowledged-preview.json", refused.stdout)
	assertExit(t, refused, 1)
	var refusedPreview collection.ConsolidationPreview
	decodeJSON(t, refused.stdout, &refusedPreview)
	if refusedPreview.State != "refused" || refusedPreview.Failure == nil || refusedPreview.Failure.Code != "unapproved-consumer" {
		t.Fatalf("new indirect exposure without exact approval did not refuse: %#v", refusedPreview)
	}
	assertTreeUnchanged(t, previewBefore, treeManifest(t, fx.base))
	if _, err := os.Lstat(selectionPath); !os.IsNotExist(err) {
		t.Fatalf("scan/refused preview wrote remembered selection: %v", err)
	}
	for _, scenario := range []string{"changed-route-id", "extra-route-id"} {
		raw, err := json.Marshal(draft)
		if err != nil {
			t.Fatal(err)
		}
		var altered collection.ConsolidationDraft
		if err := json.Unmarshal(raw, &altered); err != nil {
			t.Fatal(err)
		}
		changed := false
		for choiceIndex := range altered.Choices {
			for destinationIndex := range altered.Choices[choiceIndex].Destinations {
				for acknowledgmentIndex := range altered.Choices[choiceIndex].Destinations[destinationIndex].AcknowledgedConsumers {
					acknowledgment := &altered.Choices[choiceIndex].Destinations[destinationIndex].AcknowledgedConsumers[acknowledgmentIndex]
					if len(acknowledgment.EvidenceIDs) == 0 {
						continue
					}
					if scenario == "changed-route-id" {
						acknowledgment.EvidenceIDs[0] = "route1:stale-evidence"
					} else {
						acknowledgment.EvidenceIDs = append(acknowledgment.EvidenceIDs, "route1:unlisted-extra")
					}
					changed = true
					break
				}
				if changed {
					break
				}
			}
			if changed {
				break
			}
		}
		if !changed {
			t.Fatalf("fixture has no indirect consumer acknowledgement to mutate: %s", scenario)
		}
		alteredPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "journey-"+scenario+".json")
		edit1WriteDraft(t, alteredPath, altered)
		alteredBefore := treeManifest(t, fx.base)
		alteredResult := fx.run(t, "consolidate", "preview", "--draft", alteredPath, "--json")
		writeEvidence(t, "consolidation/journey-"+scenario+"-preview.json", alteredResult.stdout)
		assertExit(t, alteredResult, 1)
		var alteredPreview collection.ConsolidationPreview
		decodeJSON(t, alteredResult.stdout, &alteredPreview)
		if alteredPreview.State != "refused" || alteredPreview.Failure == nil {
			t.Fatalf("%s consumer-route evidence did not refuse: %#v", scenario, alteredPreview)
		}
		assertTreeUnchanged(t, alteredBefore, treeManifest(t, fx.base))
	}

	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "journey-approved.json")
	edit1WriteDraft(t, draftPath, draft)
	approvedBefore := treeManifest(t, fx.base)
	approved := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "consolidation/journey-approved-preview.json", approved.stdout)
	assertExit(t, approved, 0)
	var approvedPreview collection.ConsolidationPreview
	decodeJSON(t, approved.stdout, &approvedPreview)
	if approvedPreview.State != "ready" || approvedPreview.Confirmation == "" {
		t.Fatalf("exact exposure acknowledgments did not produce a ready plan: %#v", approvedPreview)
	}
	assertTreeUnchanged(t, approvedBefore, treeManifest(t, fx.base))
	humanPreview := fx.run(t, "consolidate", "preview", "--draft", draftPath)
	writeEvidence(t, "consolidation/journey-approved-human-preview.txt", humanPreview.stdout)
	assertExit(t, humanPreview, 0)
	for _, expected := range []string{
		"Selected applications: codex, opencode, pi",
		"Roots: (none)",
		"Canonical library: " + library,
		"Selection: arguments | remembered-selection evidence: " + inventory.Scope.RememberedSelectionID,
		"Complete bundle fingerprint: ",
		"portability: compatible | fixture bundle contains only application-neutral instructions",
		"Possible indirect consumer codeaf | explicitly acknowledged: true | evidence IDs: route1:",
		"does not import or directly link the consumer's skills",
		"Native relocation profile codex-directory-alias-v1 | codex | Codex CLI 0.162.1",
		"Native relocation profile opencode-directory-alias-v1 | opencode | OpenCode 1.17.15",
		"Native relocation profile pi-directory-alias-v1 | pi | Pi 1.4.0",
		"Recovery ID: " + approvedPreview.RecoveryID,
		"Profile limit: observed with Codex CLI 0.162.1 only",
		"filesystem operations across directories are not atomic",
	} {
		if !strings.Contains(string(humanPreview.stdout), expected) {
			t.Fatalf("human preview omitted review evidence %q:\n%s", expected, humanPreview.stdout)
		}
	}
	assertTreeUnchanged(t, approvedBefore, treeManifest(t, fx.base))
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", approvedPreview.Confirmation, "--json")
	writeEvidence(t, "consolidation/journey-apply.json", apply.stdout)
	assertExit(t, apply, 0)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &applied)
	if applied.State != "applied" || !applied.Verified || applied.RecoveryID == "" {
		t.Fatalf("approved plan was not applied with a recovery journal: %#v", applied)
	}
	for app, path := range paths {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Clean(resolved) != filepath.Clean(filepath.Join(library, sourceAtPath(t, inventory, path).BundleFingerprint)) {
			t.Fatalf("%s reference does not resolve to its canonical fingerprint bundle: %s %v", app, resolved, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(fx.home, ".codeaf", "skills", "journey-codex")); !os.IsNotExist(err) {
		t.Fatalf("indirect CodeAF exposure was mistaken for a direct import: %v", err)
	}
	assertTreeUnchanged(t, hermesBefore, treeManifest(t, hermesRoot))
	if _, err := os.Stat(selectionPath); err != nil {
		t.Fatalf("successful apply did not remember the selected scope: %v", err)
	}

	remembered := fx.run(t, "consolidate", "scan", "--json")
	writeEvidence(t, "consolidation/journey-remembered-scan.json", remembered.stdout)
	assertExit(t, remembered, 0)
	var rememberedInventory collection.ConsolidationInventory
	decodeJSON(t, remembered.stdout, &rememberedInventory)
	if rememberedInventory.Scope.SelectionFrom != "remembered" || !reflect.DeepEqual(rememberedInventory.Scope.Applications, []string{"codex", "opencode", "pi"}) || filepath.Clean(rememberedInventory.Scope.Library) != filepath.Clean(library) {
		t.Fatalf("omitted-app scan did not use the saved applications/library: %#v", rememberedInventory.Scope)
	}
	allCanonical := edit1Draft(rememberedInventory, nil)
	noopPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "journey-noop.json")
	edit1WriteDraft(t, noopPath, allCanonical)
	stable := treeManifest(t, fx.base)
	noopPreviewResult := fx.run(t, "consolidate", "preview", "--draft", noopPath, "--json")
	assertExit(t, noopPreviewResult, 0)
	var noopPreview collection.ConsolidationPreview
	decodeJSON(t, noopPreviewResult.stdout, &noopPreview)
	noopApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", noopPath, "--confirm", noopPreview.Confirmation, "--json")
	assertExit(t, noopApply, 0)
	var noop collection.ConsolidationApplyOutcome
	decodeJSON(t, noopApply.stdout, &noop)
	if noop.State != "unchanged" || noop.RecoveryID != "" || !noop.Verified {
		t.Fatalf("same remembered plan did not remain a no-op: %#v", noop)
	}
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))

	recoveryBefore := treeManifest(t, fx.base)
	recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", applied.RecoveryID, "--json")
	writeEvidence(t, "consolidation/journey-recovery-preview.json", recoveryPreviewResult.stdout)
	assertExit(t, recoveryPreviewResult, 0)
	var recoveryPreview collection.ConsolidationPreview
	decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
	if recoveryPreview.State != "ready" || recoveryPreview.Confirmation == "" {
		t.Fatalf("exact recovery did not preview: %#v", recoveryPreview)
	}
	humanRecoveryPreview := fx.run(t, "consolidate", "recover", "--backup", applied.RecoveryID)
	assertExit(t, humanRecoveryPreview, 0)
	if !strings.Contains(string(humanRecoveryPreview.stdout), "Recovery ID: "+applied.RecoveryID) || !strings.Contains(string(humanRecoveryPreview.stdout), "filesystem operations across directories are not atomic") {
		t.Fatalf("human recovery preview omitted its recovery ID or atomicity limit:\n%s", humanRecoveryPreview.stdout)
	}
	assertTreeUnchanged(t, recoveryBefore, treeManifest(t, fx.base))
	recovery := changeCommand(t, fx, "", "consolidate", "recover", "--backup", applied.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
	writeEvidence(t, "consolidation/journey-recovery-apply.json", recovery.stdout)
	assertExit(t, recovery, 0)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, recovery.stdout, &recovered)
	if recovered.State != "recovered" || !recovered.Verified {
		t.Fatalf("recovery did not verify exact restoration: %#v", recovered)
	}
	for app, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("recovery did not restore the original %s directory: mode=%v err=%v", app, info, err)
		}
		if _, err := os.Lstat(filepath.Join(filepath.Dir(path), "journey-codex")); app != "codex" && !os.IsNotExist(err) {
			t.Fatalf("recovery left the created Codex destination link in %s: %v", app, err)
		}
	}
	assertTreeUnchanged(t, hermesBefore, treeManifest(t, hermesRoot))
	if _, err := os.Lstat(selectionPath); !os.IsNotExist(err) {
		t.Fatalf("recovery did not restore the prior absent selection: %v", err)
	}
}

func TestALE86LargeAssetsAreFingerprintableWithoutSemanticInspection(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "large-duplicate")
	codexSource := filepath.Join(fx.home, ".codex", "skills", "large-duplicate")
	manifest := []byte("---\nname: large-duplicate\ndescription: complete bundle fixture\n---\nRead these instructions, then ask when content meaning is unknown.\n")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), manifest, 0o644)
	writeFile(t, filepath.Join(codexSource, "SKILL.md"), manifest, 0o644)
	large := make([]byte, (1<<20)+257)
	for index := range large {
		large[index] = byte((index*31 + 17) % 251)
	}
	writeFile(t, filepath.Join(piSource, "assets", "opaque.bin"), large, 0o600)
	writeFile(t, filepath.Join(codexSource, "assets", "opaque.bin"), large, 0o600)
	writeFile(t, filepath.Join(piSource, "scripts", "inert.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o750)
	writeFile(t, filepath.Join(codexSource, "scripts", "inert.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o750)

	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "consolidation/large-identical-scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	pi := sourceAtPath(t, inventory, piSource)
	codex := sourceAtPath(t, inventory, codexSource)
	if !pi.Eligible || !codex.Eligible || pi.BundleFingerprint == "" || pi.BundleFingerprint != codex.BundleFingerprint || pi.FingerprintCoverage != collection.Complete || codex.FingerprintCoverage != collection.Complete {
		t.Fatalf("complete identical bundles with large assets were not fully fingerprinted: pi=%#v codex=%#v", pi, codex)
	}
	if pi.InspectionComplete || codex.InspectionComplete || len(pi.InspectionLimits) == 0 || len(codex.InspectionLimits) == 0 {
		t.Fatalf("large-resource display limit was not kept separate from full bundle hashing: pi=%#v codex=%#v", pi.InspectionLimits, codex.InspectionLimits)
	}
	draft := edit1Draft(inventory, map[collection.SourceID]string{pi.ID: "share", codex.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "large-duplicate.json")
	edit1WriteDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "consolidation/large-identical-preview.json", previewResult.stdout)
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" {
		t.Fatalf("exact full-bundle duplicate did not preview: %#v", preview)
	}
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 0)
	var result collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &result)
	if result.State != "applied" || !result.Verified {
		t.Fatalf("large identical bundle apply did not verify: %#v", result)
	}
	canonical := filepath.Join(library, pi.BundleFingerprint)
	if got, err := os.ReadFile(filepath.Join(canonical, "assets", "opaque.bin")); err != nil || !reflect.DeepEqual(got, large) {
		t.Fatalf("canonical large opaque asset changed: %v", err)
	}
	info, err := os.Stat(filepath.Join(canonical, "scripts", "inert.sh"))
	if err != nil || info.Mode().Perm() != 0o750 {
		t.Fatalf("canonical executable resource mode changed: mode=%v err=%v", info, err)
	}
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatalf("consolidation executed an inspected script: %v", err)
	}

	// A one-byte change changes the complete fingerprint even when instructions
	// and filenames remain identical; hashes prove byte inequality, not meaning.
	separate := filepath.Join(fx.home, ".codex", "skills", "large-duplicate")
	if err := os.Remove(separate); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(separate, 0o755); err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), large...)
	changed[len(changed)-1] ^= 0x01
	writeFile(t, filepath.Join(separate, "SKILL.md"), manifest, 0o644)
	writeFile(t, filepath.Join(separate, "assets", "opaque.bin"), changed, 0o600)
	refreshed := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	assertExit(t, refreshed, 0)
	var after collection.ConsolidationInventory
	decodeJSON(t, refreshed.stdout, &after)
	edited := sourceAtPath(t, after, separate)
	if edited.BundleFingerprint == pi.BundleFingerprint {
		t.Fatal("one opaque-asset byte change did not alter the complete bundle fingerprint")
	}
}

func TestALE86CustomRootIsExplicitAndLibraryOverlapRefuses(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	custom := filepath.Join(fx.base, "allowed", "custom-personal-skills")
	customSkill := filepath.Join(custom, "custom-source")
	writeFile(t, filepath.Join(customSkill, "SKILL.md"), []byte("---\nname: custom-source\ndescription: explicit custom root fixture\n---\nCustom-root source.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	before := treeManifest(t, fx.base)
	result := fx.run(t, "consolidate", "scan", "--app", "pi", "--root", custom, "--library", library, "--json")
	assertExit(t, result, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, result.stdout, &inventory)
	if len(inventory.Scope.Roots) != 1 || filepath.Clean(inventory.Scope.Roots[0]) != filepath.Clean(custom) {
		t.Fatalf("custom root was not explicit in scan scope: %#v", inventory.Scope)
	}
	source := sourceAtPath(t, inventory, customSkill)
	if !source.Eligible {
		t.Fatalf("custom personal source should be eligible with an external safe library: %#v", source)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	unsafe := fx.run(t, "consolidate", "scan", "--app", "pi", "--root", custom, "--library", custom, "--json")
	writeEvidence(t, "consolidation/custom-root-overlap-scan.json", unsafe.stdout)
	assertExit(t, unsafe, 0)
	var unsafeInventory collection.ConsolidationInventory
	decodeJSON(t, unsafe.stdout, &unsafeInventory)
	if unsafeInventory.LibrarySafe {
		t.Fatal("library overlapping an explicit source root was reported safe")
	}
	unsafeDraft := edit1Draft(unsafeInventory, map[collection.SourceID]string{sourceAtPath(t, unsafeInventory, customSkill).ID: "share"})
	unsafePath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "custom-overlap.json")
	edit1WriteDraft(t, unsafePath, unsafeDraft)
	stable := treeManifest(t, fx.base)
	refused := fx.run(t, "consolidate", "preview", "--draft", unsafePath, "--json")
	assertExit(t, refused, 1)
	var preview collection.ConsolidationPreview
	decodeJSON(t, refused.stdout, &preview)
	if preview.State != "refused" {
		t.Fatalf("unsafe overlapping canonical library did not refuse: %#v", preview)
	}
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))

	// A successful keep-only apply remembers the custom scope without moving
	// the source; a later omitted-app scan reuses that scope unless cleared.
	rememberDraft := edit1Draft(inventory, nil)
	rememberPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "remember-custom-root.json")
	edit1WriteDraft(t, rememberPath, rememberDraft)
	rememberPreviewResult := fx.run(t, "consolidate", "preview", "--draft", rememberPath, "--json")
	assertExit(t, rememberPreviewResult, 0)
	var rememberPreview collection.ConsolidationPreview
	decodeJSON(t, rememberPreviewResult.stdout, &rememberPreview)
	rememberApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", rememberPath, "--confirm", rememberPreview.Confirmation, "--json")
	assertExit(t, rememberApply, 0)
	var rememberedOutcome collection.ConsolidationApplyOutcome
	decodeJSON(t, rememberApply.stdout, &rememberedOutcome)
	if rememberedOutcome.State != "selection-updated" || !rememberedOutcome.SelectionUpdated {
		t.Fatalf("keep-only apply did not truthfully record the requested scope: %#v", rememberedOutcome)
	}
	omitted := fx.run(t, "consolidate", "scan", "--json")
	assertExit(t, omitted, 0)
	var omittedInventory collection.ConsolidationInventory
	decodeJSON(t, omitted.stdout, &omittedInventory)
	if omittedInventory.Scope.SelectionFrom != "remembered" || !reflect.DeepEqual(omittedInventory.Scope.Roots, []string{custom}) {
		t.Fatalf("omitted scan did not restore the successfully applied custom root: %#v", omittedInventory.Scope)
	}
	cleared := fx.run(t, "consolidate", "scan", "--clear-roots", "--json")
	assertExit(t, cleared, 0)
	var clearedInventory collection.ConsolidationInventory
	decodeJSON(t, cleared.stdout, &clearedInventory)
	if clearedInventory.Scope.SelectionFrom != "remembered" || len(clearedInventory.Scope.Roots) != 0 {
		t.Fatalf("--clear-roots did not remove remembered roots for this scan: %#v", clearedInventory.Scope)
	}
}
