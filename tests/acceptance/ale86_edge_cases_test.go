package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86SubsetPortabilityAndUnknownApproval(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "subset-skill")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: subset-skill\ndescription: requires a capability unavailable in Codex\n---\nUse the Pi-only capability only in Pi.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	source := sourceAtPath(t, inventory, piSource)
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
	choice := consolidationChoice(t, &draft, source.ID)
	for index := range choice.Destinations {
		if choice.Destinations[index].Application == "codex" {
			choice.Destinations[index].Portability = "unknown"
			choice.Destinations[index].Reason = "the exact required capability could not be verified for Codex"
			choice.Destinations[index].UserApproved = false
		}
	}
	unknownPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "subset-unknown.json")
	writeConsolidationDraft(t, unknownPath, draft)
	before := treeManifest(t, fx.base)
	unknown := fx.run(t, "consolidate", "preview", "--draft", unknownPath, "--json")
	assertExit(t, unknown, 1)
	var unknownPreview collection.ConsolidationPreview
	decodeJSON(t, unknown.stdout, &unknownPreview)
	if unknownPreview.State != "refused" || unknownPreview.Failure == nil || unknownPreview.Failure.Code != "invalid-portability" {
		t.Fatalf("unresolved unknown portability did not block sharing: %#v", unknownPreview)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	choice = consolidationChoice(t, &draft, source.ID)
	for index := range choice.Destinations {
		switch choice.Destinations[index].Application {
		case "codex":
			choice.Destinations[index].Portability = "incompatible"
			choice.Destinations[index].Reason = "fixture capability is unavailable in Codex"
		case "pi":
			choice.Destinations[index].Portability = "compatible"
			choice.Destinations[index].Reason = "fixture capability is available in Pi"
		}
	}
	subsetPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "subset-compatible-only.json")
	writeConsolidationDraft(t, subsetPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", subsetPath, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" || len(preview.Effects) != 2 {
		t.Fatalf("subset-compatible plan should retain one share and the canary keep: %#v", preview)
	}
	var destinationCount int
	for _, effect := range preview.Effects {
		if effect.SourceID == source.ID {
			destinationCount = len(effect.Destinations)
			if len(effect.Destinations) != 0 || !containsPath(effect.PreservedReferences, piSource) {
				t.Fatalf("incompatible Codex received a link or existing Pi reference was not preserved: %#v", effect)
			}
		}
	}
	if destinationCount != 0 {
		t.Fatalf("unexpected new link for an already-connected Pi source: %d", destinationCount)
	}
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", subsetPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 0)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &applied)
	if applied.State != "applied" || !applied.Verified {
		t.Fatalf("subset share did not apply: %#v", applied)
	}
	if _, err := os.Lstat(filepath.Join(fx.home, ".codex", "skills", "subset-skill")); !os.IsNotExist(err) {
		t.Fatalf("incompatible Codex unexpectedly received a reference: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(piSource)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(filepath.Join(library, source.BundleFingerprint)) {
		t.Fatalf("compatible Pi reference missing: %s %v", resolved, err)
	}

	// The same path may be explicitly approved as unknown only through a fresh
	// exact draft; merely changing an old token is not accepted.
	fresh := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	assertExit(t, fresh, 0)
	var freshInventory collection.ConsolidationInventory
	decodeJSON(t, fresh.stdout, &freshInventory)
	canonical := sourceAtPath(t, freshInventory, filepath.Join(library, source.BundleFingerprint))
	approvedUnknownDraft := buildConsolidationDraft(freshInventory, map[collection.SourceID]string{canonical.ID: "share"})
	approvedChoice := consolidationChoice(t, &approvedUnknownDraft, canonical.ID)
	for index := range approvedChoice.Destinations {
		if approvedChoice.Destinations[index].Application == "codex" {
			approvedChoice.Destinations[index].Portability = "unknown"
			approvedChoice.Destinations[index].Reason = "user reviewed and explicitly accepts the unresolved Codex capability"
			approvedChoice.Destinations[index].UserApproved = true
		}
	}
	approvedUnknownPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "subset-approved-unknown.json")
	writeConsolidationDraft(t, approvedUnknownPath, approvedUnknownDraft)
	approvedUnknownResult := fx.run(t, "consolidate", "preview", "--draft", approvedUnknownPath, "--json")
	assertExit(t, approvedUnknownResult, 0)
	var approvedUnknown collection.ConsolidationPreview
	decodeJSON(t, approvedUnknownResult.stdout, &approvedUnknown)
	if approvedUnknown.State != "ready" {
		t.Fatalf("explicit unknown-portability approval was not bound into a ready proposal: %#v", approvedUnknown)
	}
}

func TestALE86DivergentResourceArchiveIsRecoverableAndPreservesWinner(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "divergent-choice")
	codexSource := filepath.Join(fx.home, ".codex", "skills", "divergent-choice")
	sharedManifest := []byte("---\nname: divergent-choice\ndescription: same instructions, different supporting resource\n---\nRead the guide before acting.\n")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), sharedManifest, 0o644)
	writeFile(t, filepath.Join(piSource, "references", "guide.md"), []byte("Pi variant: preserve this text.\n"), 0o640)
	writeFile(t, filepath.Join(codexSource, "SKILL.md"), sharedManifest, 0o644)
	writeFile(t, filepath.Join(codexSource, "references", "guide.md"), []byte("Codex variant: these are different instructions.\n"), 0o640)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	piBefore := treeManifest(t, piSource)
	codexBefore := treeManifest(t, codexSource)
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "consolidation/divergent-resource-scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	pi := sourceAtPath(t, inventory, piSource)
	codex := sourceAtPath(t, inventory, codexSource)
	if !pi.Eligible || !codex.Eligible || pi.BundleFingerprint == codex.BundleFingerprint {
		t.Fatalf("supporting resource difference was not reflected in complete identity: pi=%#v codex=%#v", pi, codex)
	}
	var divergentConflict bool
	for _, conflict := range inventory.Conflicts {
		if conflict.Kind == "divergent-name" && conflict.Name == "divergent-choice" {
			divergentConflict = true
		}
	}
	if !divergentConflict {
		t.Fatalf("same native name with divergent full bundle was not reported: %#v", inventory.Conflicts)
	}

	keepDraft := buildConsolidationDraft(inventory, map[collection.SourceID]string{codex.ID: "share"})
	keepPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "divergent-keep-refusal.json")
	writeConsolidationDraft(t, keepPath, keepDraft)
	before := treeManifest(t, fx.base)
	keepResult := fx.run(t, "consolidate", "preview", "--draft", keepPath, "--json")
	assertExit(t, keepResult, 1)
	var keepPreview collection.ConsolidationPreview
	decodeJSON(t, keepResult.stdout, &keepPreview)
	if keepPreview.State != "refused" || keepPreview.Failure == nil {
		t.Fatalf("keeping a divergent active same-name version was not refused: %#v", keepPreview)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	// Retain the Pi variant under the private recovery store while sharing the
	// Codex winner. Consolidation preserves bytes; it does not merge content.
	winnerDraft := buildConsolidationDraft(inventory, map[collection.SourceID]string{codex.ID: "share", pi.ID: "archive"})
	winnerPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "divergent-winner-archive.json")
	writeConsolidationDraft(t, winnerPath, winnerDraft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", winnerPath, "--json")
	writeEvidence(t, "consolidation/divergent-winner-preview.json", previewResult.stdout)
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" {
		t.Fatalf("explicit winner and archive plan refused: %#v", preview)
	}
	var archivePath string
	for _, effect := range preview.Effects {
		if effect.Action == "archive" && effect.SourceID == pi.ID {
			archivePath = effect.BackupPath
		}
	}
	if archivePath == "" {
		t.Fatalf("preview did not disclose the divergent backup path: %#v", preview.Effects)
	}
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", winnerPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 0)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &applied)
	if applied.State != "applied" || !applied.Verified || applied.RecoveryID == "" {
		t.Fatalf("winner/archive apply did not verify or produce recovery: %#v", applied)
	}
	if info, err := os.Lstat(piSource); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Pi should now expose the reviewed winner through a symlink: mode=%v err=%v", info, err)
	}
	if got := readBytes(t, filepath.Join(archivePath, "references", "guide.md")); !bytes.Equal(got, []byte("Pi variant: preserve this text.\n")) {
		t.Fatalf("archive did not preserve the exact divergent resource: %q", got)
	}
	canonical := filepath.Join(library, codex.BundleFingerprint)
	if got := readBytes(t, filepath.Join(canonical, "references", "guide.md")); !bytes.Equal(got, []byte("Codex variant: these are different instructions.\n")) {
		t.Fatalf("selected winner resource changed: %q", got)
	}
	piAlias := filepath.Join(fx.home, ".pi", "agent", "skills", "divergent-choice")
	resolved, err := filepath.EvalSymlinks(piAlias)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(canonical) {
		t.Fatalf("Pi destination did not resolve to the reviewed winner: %s %v", resolved, err)
	}
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatalf("consolidation executed bundle resources: %v", err)
	}

	recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", applied.RecoveryID, "--json")
	assertExit(t, recoveryPreviewResult, 0)
	var recoveryPreview collection.ConsolidationPreview
	decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
	if recoveryPreview.State != "ready" {
		t.Fatalf("fresh archive recovery was not available: %#v", recoveryPreview)
	}
	recovery := changeCommand(t, fx, "", "consolidate", "recover", "--backup", applied.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
	assertExit(t, recovery, 0)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, recovery.stdout, &recovered)
	if recovered.State != "recovered" || !recovered.Verified {
		t.Fatalf("archive recovery did not restore both versions: %#v", recovered)
	}
	if !bytes.Equal(piBefore, treeManifest(t, piSource)) || !bytes.Equal(codexBefore, treeManifest(t, codexSource)) {
		t.Fatal("archive recovery did not restore exact divergent source trees")
	}
}

func TestALE86StaleSourceAndRememberedSelectionRefuseWithoutExtraWrites(t *testing.T) {
	t.Run("source-bytes", func(t *testing.T) {
		fx := newFixture(t)
		writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
			"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
		), 0o644)
		piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "stale-source")
		writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: stale-source\ndescription: fresh source approval fixture\n---\nOriginal source text.\n"), 0o644)
		library := filepath.Join(fx.xdgData, "skillglass", "skills")
		scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
		assertExit(t, scan, 0)
		var inventory collection.ConsolidationInventory
		decodeJSON(t, scan.stdout, &inventory)
		source := sourceAtPath(t, inventory, piSource)
		draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
		draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "stale-source.json")
		writeConsolidationDraft(t, draftPath, draft)
		previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
		assertExit(t, previewResult, 0)
		var preview collection.ConsolidationPreview
		decodeJSON(t, previewResult.stdout, &preview)
		if preview.State != "ready" {
			t.Fatalf("fresh source plan did not preview: %#v", preview)
		}
		writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: stale-source\ndescription: externally changed source\n---\nA user edit invalidates the previous plan.\n"), 0o644)
		beforeApply := treeManifest(t, fx.base)
		stale := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
		assertExit(t, stale, 1)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, stale.stdout, &outcome)
		if outcome.State != "refused" || outcome.Failure == nil || outcome.Failure.Code != "stale-scan" {
			t.Fatalf("changed source did not invalidate exact approval: %#v", outcome)
		}
		assertTreeUnchanged(t, beforeApply, treeManifest(t, fx.base))
		if _, err := os.Lstat(filepath.Join(fx.xdgConfig, "skillglass", "consolidation.json")); !os.IsNotExist(err) {
			t.Fatalf("stale apply recorded a successful selection: %v", err)
		}
	})

	t.Run("selection-evidence", func(t *testing.T) {
		fx := newFixture(t)
		writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
			"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
		), 0o644)
		piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "selection-stale")
		writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: selection-stale\ndescription: selected-app freshness fixture\n---\nKeep source bytes unchanged.\n"), 0o644)
		library := filepath.Join(fx.xdgData, "skillglass", "skills")
		seedScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--library", library, "--json")
		assertExit(t, seedScan, 0)
		var seedInventory collection.ConsolidationInventory
		decodeJSON(t, seedScan.stdout, &seedInventory)
		seedDraft := buildConsolidationDraft(seedInventory, nil)
		seedPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "selection-seed.json")
		writeConsolidationDraft(t, seedPath, seedDraft)
		seedPreviewResult := fx.run(t, "consolidate", "preview", "--draft", seedPath, "--json")
		assertExit(t, seedPreviewResult, 0)
		var seedPreview collection.ConsolidationPreview
		decodeJSON(t, seedPreviewResult.stdout, &seedPreview)
		seedApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", seedPath, "--confirm", seedPreview.Confirmation, "--json")
		assertExit(t, seedApply, 0)
		if !bytes.Contains(seedApply.stdout, []byte(`"state": "selection-updated"`)) {
			t.Fatalf("selection-only seed did not record the explicit Pi selection: %s", seedApply.stdout)
		}

		staleScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--library", library, "--json")
		assertExit(t, staleScan, 0)
		var staleInventory collection.ConsolidationInventory
		decodeJSON(t, staleScan.stdout, &staleInventory)
		staleSource := sourceAtPath(t, staleInventory, piSource)
		staleDraft := buildConsolidationDraft(staleInventory, map[collection.SourceID]string{staleSource.ID: "keep"})
		stalePath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "selection-stale.json")
		writeConsolidationDraft(t, stalePath, staleDraft)
		stalePreviewResult := fx.run(t, "consolidate", "preview", "--draft", stalePath, "--json")
		assertExit(t, stalePreviewResult, 0)
		var stalePreview collection.ConsolidationPreview
		decodeJSON(t, stalePreviewResult.stdout, &stalePreview)
		if stalePreview.State != "ready" {
			t.Fatalf("explicit selection preview was not ready: %#v", stalePreview)
		}

		otherScan := fx.run(t, "consolidate", "scan", "--app", "codex", "--library", library, "--json")
		assertExit(t, otherScan, 0)
		var otherInventory collection.ConsolidationInventory
		decodeJSON(t, otherScan.stdout, &otherInventory)
		otherDraft := buildConsolidationDraft(otherInventory, nil)
		otherPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "selection-other.json")
		writeConsolidationDraft(t, otherPath, otherDraft)
		otherPreviewResult := fx.run(t, "consolidate", "preview", "--draft", otherPath, "--json")
		assertExit(t, otherPreviewResult, 0)
		var otherPreview collection.ConsolidationPreview
		decodeJSON(t, otherPreviewResult.stdout, &otherPreview)
		otherApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", otherPath, "--confirm", otherPreview.Confirmation, "--json")
		assertExit(t, otherApply, 0)
		if !bytes.Contains(otherApply.stdout, []byte(`"state": "selection-updated"`)) {
			t.Fatalf("Codex selection-only run did not persist the new app scope: %s", otherApply.stdout)
		}
		selectionPath := filepath.Join(fx.xdgConfig, "skillglass", "consolidation.json")
		selectionBeforeRefusal := readBytes(t, selectionPath)
		beforeRefusal := treeManifest(t, fx.base)
		staleApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", stalePath, "--confirm", stalePreview.Confirmation, "--json")
		assertExit(t, staleApply, 1)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, staleApply.stdout, &outcome)
		if outcome.State != "refused" || outcome.Failure == nil || outcome.Failure.Code != "stale-selection" {
			t.Fatalf("explicit plan failed to bind remembered-selection evidence: %#v", outcome)
		}
		assertTreeUnchanged(t, beforeRefusal, treeManifest(t, fx.base))
		if !bytes.Equal(selectionBeforeRefusal, readBytes(t, selectionPath)) {
			t.Fatal("stale explicit plan overwrote the newer successful selection")
		}
	})
}

func TestALE86SelectedAliasCannotMoveExcludedHermesOwnedSource(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	hermesRoot := filepath.Join(fx.home, ".hermes", "skills")
	hermesSource := filepath.Join(hermesRoot, "hermes-owned")
	writeFile(t, filepath.Join(hermesSource, "SKILL.md"), []byte("---\nname: hermes-owned\ndescription: source physically owned by excluded Hermes\n---\nThis bundle remains under Hermes ownership.\n"), 0o640)
	piAlias := filepath.Join(fx.home, ".pi", "agent", "skills", "hermes-owned")
	if err := os.MkdirAll(filepath.Dir(piAlias), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hermesSource, piAlias); err != nil {
		t.Fatal(err)
	}
	hermesBefore := treeManifest(t, hermesRoot)
	aliasTargetBefore, err := os.Readlink(piAlias)
	if err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--library", library, "--json")
	writeEvidence(t, "consolidation/excluded-hermes-ownership-scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	var found *collection.ConsolidationSource
	for index := range inventory.Sources {
		if filepath.Clean(inventory.Sources[index].SourcePath) == filepath.Clean(hermesSource) {
			found = &inventory.Sources[index]
			break
		}
	}
	if found == nil || found.Eligible || !strings.Contains(strings.ToLower(strings.Join(found.BlockedReasons, " ")), "unselected") {
		t.Fatalf("alias into excluded Hermes's physical root did not remain outside relocation scope: %#v", found)
	}
	if len(inventory.Scope.Applications) != 1 || inventory.Scope.Applications[0] != "pi" {
		t.Fatalf("Hermes entered direct consolidation selection: %#v", inventory.Scope.Applications)
	}
	shareDraft := buildConsolidationDraft(inventory, nil)
	shareDraft.Choices = append(shareDraft.Choices, collection.ConsolidationChoice{SourceID: found.ID, SourcePath: found.SourcePath, BundleFingerprint: found.BundleFingerprint, Action: "share", CanonicalName: found.Name, Destinations: []collection.ConsolidationDestinationChoice{{Application: "pi", Portability: "compatible", Reason: "fixture-only invalid request to verify excluded physical ownership"}}})
	sharePath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-share-refusal.json")
	writeConsolidationDraft(t, sharePath, shareDraft)
	sharePreviewResult := fx.run(t, "consolidate", "preview", "--draft", sharePath, "--json")
	assertExit(t, sharePreviewResult, 1)
	var sharePreview collection.ConsolidationPreview
	decodeJSON(t, sharePreviewResult.stdout, &sharePreview)
	if sharePreview.State != "refused" || sharePreview.Failure == nil || sharePreview.Failure.Code != "ineligible-source" {
		t.Fatalf("share request for excluded Hermes-owned source did not refuse: %#v", sharePreview)
	}
	archiveDraft := shareDraft
	archiveDraft.Choices = append([]collection.ConsolidationChoice(nil), shareDraft.Choices...)
	for index := range archiveDraft.Choices {
		if archiveDraft.Choices[index].SourceID == found.ID {
			archiveDraft.Choices[index] = collection.ConsolidationChoice{SourceID: found.ID, SourcePath: found.SourcePath, BundleFingerprint: found.BundleFingerprint, Action: "archive"}
		}
	}
	archivePath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-archive-refusal.json")
	writeConsolidationDraft(t, archivePath, archiveDraft)
	before := treeManifest(t, fx.base)
	archiveResult := fx.run(t, "consolidate", "preview", "--draft", archivePath, "--json")
	assertExit(t, archiveResult, 1)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if !bytes.Equal(hermesBefore, treeManifest(t, hermesRoot)) {
		t.Fatal("refusing an excluded Hermes source changed its physical owner directory")
	}
	aliasTargetAfter, err := os.Readlink(piAlias)
	if err != nil || aliasTargetAfter != aliasTargetBefore {
		t.Fatalf("pre-existing selected alias changed: before=%q after=%q err=%v", aliasTargetBefore, aliasTargetAfter, err)
	}
}

func TestALE86NestedSourcesCannotMoveAcrossEachOther(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	personalRoot := filepath.Join(fx.home, ".pi", "agent", "skills")
	root := filepath.Join(fx.base, "allowed", "custom-nested-skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(personalRoot, "nested-parent")
	child := filepath.Join(parent, "child-skill")
	writeFile(t, filepath.Join(parent, "SKILL.md"), []byte("---\nname: nested-parent\ndescription: parent bundle containing a nested skill\n---\nParent bundle bytes include the child directory.\n"), 0o644)
	writeFile(t, filepath.Join(child, "SKILL.md"), []byte("---\nname: child-skill\ndescription: child bundle nested in the parent\n---\nChild bundle has its own native manifest.\n"), 0o644)
	if err := os.Symlink(child, filepath.Join(root, "child-alias")); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--root", root, "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	parentSource := sourceAtPath(t, inventory, parent)
	childSource := sourceAtPath(t, inventory, child)
	if !parentSource.Eligible || !childSource.Eligible {
		t.Fatalf("nested fixture sources did not produce complete evidence: parent=%#v child=%#v", parentSource, childSource)
	}

	for _, moving := range []collection.SourceID{parentSource.ID, childSource.ID} {
		actions := map[collection.SourceID]string{moving: "share"}
		draft := buildConsolidationDraft(inventory, actions)
		path := filepath.Join(fx.base, "allowed", "ownership-artifacts", "nested-"+string(moving)+".json")
		writeConsolidationDraft(t, path, draft)
		before := treeManifest(t, fx.base)
		result := fx.run(t, "consolidate", "preview", "--draft", path, "--json")
		assertExit(t, result, 1)
		var preview collection.ConsolidationPreview
		decodeJSON(t, result.stdout, &preview)
		if preview.State != "refused" || preview.Failure == nil || preview.Failure.Code != "overlapping-source-paths" {
			t.Fatalf("moving one nested source did not refuse for physical overlap: %#v", preview)
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	}

	keepDraft := buildConsolidationDraft(inventory, nil)
	keepPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "nested-both-kept.json")
	writeConsolidationDraft(t, keepPath, keepDraft)
	keepResult := fx.run(t, "consolidate", "preview", "--draft", keepPath, "--json")
	assertExit(t, keepResult, 0)
	var keepPreview collection.ConsolidationPreview
	decodeJSON(t, keepResult.stdout, &keepPreview)
	if keepPreview.State != "ready" {
		t.Fatalf("keeping physically nested sources should remain a valid no-write plan: %#v", keepPreview)
	}
}

func consolidationChoice(t *testing.T, draft *collection.ConsolidationDraft, id collection.SourceID) *collection.ConsolidationChoice {
	t.Helper()
	for index := range draft.Choices {
		if draft.Choices[index].SourceID == id {
			return &draft.Choices[index]
		}
	}
	t.Fatalf("draft has no choice for source %s", id)
	return nil
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if filepath.Clean(path) == filepath.Clean(want) {
			return true
		}
	}
	return false
}
