package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86ArchiveRequiresPlannedNativeOwnerReplacement(t *testing.T) {
	t.Run("already-connected winner has no replacement destination", func(t *testing.T) {
		fx := newFixture(t)
		writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
		winnerPath := filepath.Join(fx.home, ".pi", "agent", "skills", "selected-winner")
		writeFile(t, filepath.Join(winnerPath, "SKILL.md"), []byte("---\nname: selected-winner\ndescription: archive replacement fixture\n---\nWinner bytes.\n"), 0o644)
		library := filepath.Join(fx.xdgData, "skillglass", "skills")
		initialScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
		assertExit(t, initialScan, 0)
		var initialInventory collection.ConsolidationInventory
		decodeJSON(t, initialScan.stdout, &initialInventory)
		winner := sourceAtPath(t, initialInventory, winnerPath)
		initialDraft := buildConsolidationDraft(initialInventory, map[collection.SourceID]string{winner.ID: "share"})
		initialDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "archive-initial.json")
		writeConsolidationDraft(t, initialDraftPath, initialDraft)
		initialPreviewResult := fx.run(t, "consolidate", "preview", "--draft", initialDraftPath, "--json")
		assertExit(t, initialPreviewResult, 0)
		var initialPreview collection.ConsolidationPreview
		decodeJSON(t, initialPreviewResult.stdout, &initialPreview)
		initialApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", initialDraftPath, "--confirm", initialPreview.Confirmation, "--json")
		assertExit(t, initialApply, 0)

		archivedPath := filepath.Join(fx.home, ".pi", "agent", "skills", "archived-owner")
		archivedBytes := []byte("---\nname: archived-owner\ndescription: divergent owner bundle\n---\nRetain these exact bytes.\n")
		writeFile(t, filepath.Join(archivedPath, "SKILL.md"), archivedBytes, 0o640)
		freshScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
		assertExit(t, freshScan, 0)
		var inventory collection.ConsolidationInventory
		decodeJSON(t, freshScan.stdout, &inventory)
		canonical := sourceAtPath(t, inventory, filepath.Join(library, winner.BundleFingerprint))
		archived := sourceAtPath(t, inventory, archivedPath)
		draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{canonical.ID: "share", archived.ID: "archive"})
		consolidationChoice(t, &draft, canonical.ID).CanonicalName = "archived-owner"
		draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "archive-no-destination.json")
		writeConsolidationDraft(t, draftPath, draft)
		before := treeManifest(t, fx.base)
		result := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
		assertExit(t, result, 1)
		var preview collection.ConsolidationPreview
		decodeJSON(t, result.stdout, &preview)
		if preview.State != "refused" || preview.Failure == nil || preview.Failure.Code != "unsupported-native-relocation" {
			t.Fatalf("archive without a planned exact owner replacement was accepted: %#v", preview)
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		if got := readBytes(t, filepath.Join(archivedPath, "SKILL.md")); !bytes.Equal(got, archivedBytes) {
			t.Fatalf("refused archive changed the owner's bytes: %q", got)
		}
	})

	t.Run("planned exact leaf with a different native skill name is refused", func(t *testing.T) {
		fx := newFixture(t)
		writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
		winnerPath := filepath.Join(fx.home, ".codex", "skills", "selected-winner")
		archivedPath := filepath.Join(fx.home, ".pi", "agent", "skills", "archived-owner")
		writeFile(t, filepath.Join(winnerPath, "SKILL.md"), []byte("---\nname: selected-winner\ndescription: intentionally different native identity\n---\nWinner bytes.\n"), 0o644)
		archivedBytes := []byte("---\nname: archived-owner\ndescription: divergent owner bundle\n---\nRetain these exact bytes.\n")
		writeFile(t, filepath.Join(archivedPath, "SKILL.md"), archivedBytes, 0o640)
		library := filepath.Join(fx.xdgData, "skillglass", "skills")
		scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--library", library, "--json")
		assertExit(t, scan, 0)
		var inventory collection.ConsolidationInventory
		decodeJSON(t, scan.stdout, &inventory)
		winner := sourceAtPath(t, inventory, winnerPath)
		archived := sourceAtPath(t, inventory, archivedPath)
		draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{winner.ID: "share", archived.ID: "archive"})
		consolidationChoice(t, &draft, winner.ID).CanonicalName = "archived-owner"
		draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "archive-wrong-native-name.json")
		writeConsolidationDraft(t, draftPath, draft)
		before := treeManifest(t, fx.base)
		result := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
		assertExit(t, result, 1)
		var preview collection.ConsolidationPreview
		decodeJSON(t, result.stdout, &preview)
		if preview.State != "refused" || preview.Failure == nil || preview.Failure.Code != "unsupported-native-relocation" {
			t.Fatalf("leaf-only replacement with a different native skill identity was accepted: %#v", preview)
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		if got := readBytes(t, filepath.Join(archivedPath, "SKILL.md")); !bytes.Equal(got, archivedBytes) {
			t.Fatalf("refused archive changed the owner's bytes: %q", got)
		}
	})
}

func TestALE86ReadyPreviewCarriesScanDisclosures(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	pluginVersion := filepath.Join(fx.home, ".codex", "plugins", "cache", "fixture-market", "fixture-plugin", "1.0.0")
	writeFile(t, filepath.Join(pluginVersion, ".codex-plugin", "plugin.json"), []byte(`{"name":"fixture-plugin","version":"1.0.0","skills":"skills"}`), 0o644)
	writeFile(t, filepath.Join(pluginVersion, "skills", "managed-canary", "SKILL.md"), []byte("---\nname: managed-canary\ndescription: managed fixture\n---\nKeep managed.\n"), 0o644)
	piPath := filepath.Join(fx.home, ".pi", "agent", "skills", "same-name")
	codexPath := filepath.Join(fx.home, ".codex", "skills", "same-name")
	writeFile(t, filepath.Join(piPath, "SKILL.md"), []byte("---\nname: same-name\ndescription: Pi variant\n---\nPi text.\n"), 0o644)
	writeFile(t, filepath.Join(codexPath, "SKILL.md"), []byte("---\nname: same-name\ndescription: Codex variant\n---\nCodex text.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	var pi, codex collection.ConsolidationSource
	for _, source := range inventory.Sources {
		if filepath.Clean(source.SourcePath) == filepath.Clean(piPath) {
			pi = source
		}
		if filepath.Clean(source.SourcePath) == filepath.Clean(codexPath) {
			codex = source
		}
	}
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{codex.ID: "share", pi.ID: "archive"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "preview-disclosures.json")
	writeConsolidationDraft(t, draftPath, draft)
	before := treeManifest(t, fx.base)
	result := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, result, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, result.stdout, &preview)
	if preview.State != "ready" || len(preview.Conflicts) == 0 || len(preview.Exclusions) == 0 || len(preview.SourceAvailability) == 0 {
		t.Fatalf("ready preview omitted scan conflicts, exclusions, or source availability: %#v", preview)
	}
	var conflictResolved, pluginExcluded bool
	for _, conflict := range preview.Conflicts {
		if conflict.ID == inventory.Conflicts[0].ID && conflict.State == "resolved" {
			conflictResolved = true
		}
	}
	for _, exclusion := range preview.Exclusions {
		if filepath.Clean(exclusion.Path) == filepath.Clean(filepath.Join(pluginVersion, "skills", "managed-canary")) {
			pluginExcluded = true
		}
	}
	if !conflictResolved || !pluginExcluded {
		t.Fatalf("preview did not report the resolved conflict and exact managed exclusion: conflicts=%#v exclusions=%#v", preview.Conflicts, preview.Exclusions)
	}
	for _, source := range preview.SourceAvailability {
		if source.SourceID == codex.ID && source.Resolution != "resolved" {
			t.Fatalf("ready source was inaccurately reported unresolved: %#v", source)
		}
	}
	if strings.Contains(string(result.stdout), "SECRET") || strings.Contains(string(result.stdout), "config.toml") {
		t.Fatalf("preview disclosure leaked native settings: %s", result.stdout)
	}
	human := fx.run(t, "consolidate", "preview", "--draft", draftPath)
	assertExit(t, human, 0)
	for _, expected := range []string{"Conflict", "resolved", "Excluded " + filepath.Join(pluginVersion, "skills", "managed-canary"), "Source availability"} {
		if !strings.Contains(string(human.stdout), expected) {
			t.Fatalf("human preview omitted disclosure %q:\n%s", expected, human.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE86ApplyReportsPerPathPartialProgressAndRecovery(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory permissions do not constrain a privileged test process")
	}
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	codexPath := filepath.Join(fx.home, ".codex", "skills", "partial-canary")
	writeFile(t, filepath.Join(codexPath, "SKILL.md"), []byte("---\nname: partial-canary\ndescription: controlled partial apply\n---\nKeep these bytes.\n"), 0o640)
	piRoot := filepath.Join(fx.home, ".pi", "agent", "skills")
	if err := os.MkdirAll(piRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	source := sourceAtPath(t, inventory, codexPath)
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "partial-apply.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if err := os.Chmod(piRoot, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(piRoot, 0o755)
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 1)
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &outcome)
	if outcome.State != "partial" || outcome.RecoveryID == "" || len(outcome.Effects) == 0 {
		t.Fatalf("controlled destination failure did not preserve a partial outcome and recovery ID: %#v", outcome)
	}
	var completedOriginal, failedPiLink bool
	for _, effect := range outcome.Effects {
		if effect.SourceID != source.ID {
			continue
		}
		if effect.State != "partial" {
			t.Fatalf("share effect retained planned state instead of partial progress: %#v", effect)
		}
		for _, operation := range effect.Operations {
			if operation.Action == "create-link" && filepath.Clean(operation.Path) == filepath.Clean(codexPath) && operation.State == "completed" {
				completedOriginal = true
			}
			if operation.Action == "create-link" && filepath.Clean(operation.Path) == filepath.Join(piRoot, "partial-canary") && operation.State == "failed" {
				failedPiLink = true
			}
		}
	}
	if !completedOriginal || !failedPiLink {
		t.Fatalf("partial outcome omitted completed Codex link or failed Pi link: %#v", outcome.Effects)
	}
	canonical := filepath.Join(library, source.BundleFingerprint)
	if resolved, err := filepath.EvalSymlinks(codexPath); err != nil || filepath.Clean(resolved) != filepath.Clean(canonical) {
		t.Fatalf("completed original-path link does not resolve to canonical content: %s %v", resolved, err)
	}
	if _, err := os.Lstat(filepath.Join(piRoot, "partial-canary")); !os.IsNotExist(err) {
		t.Fatalf("failed destination unexpectedly exists: %v", err)
	}
	if err := os.Chmod(piRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", outcome.RecoveryID, "--json")
	assertExit(t, recoveryPreviewResult, 0)
	var recoveryPreview collection.ConsolidationPreview
	decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
	recovery := changeCommand(t, fx, "", "consolidate", "recover", "--backup", outcome.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
	assertExit(t, recovery, 0)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, recovery.stdout, &recovered)
	if recovered.State != "recovered" || !recovered.Verified {
		t.Fatalf("partial apply recovery did not complete: %#v", recovered)
	}
	for _, effect := range recovered.Effects {
		if effect.State != "completed" {
			t.Fatalf("recovery effect did not report completed: %#v", effect)
		}
		for _, operation := range effect.Operations {
			if operation.State != "completed" && operation.State != "not-needed" {
				t.Fatalf("recovery operation did not report completion: %#v", operation)
			}
		}
	}
	if info, err := os.Lstat(codexPath); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("recovery did not restore the original Codex directory: mode=%v err=%v", info, err)
	}
}

func TestALE86HumanApplyPrintsPerPathOutcomes(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	codexPath := filepath.Join(fx.home, ".codex", "skills", "human-outcomes")
	writeFile(t, filepath.Join(codexPath, "SKILL.md"), []byte("---\nname: human-outcomes\ndescription: human outcome fixture\n---\nKeep these bytes.\n"), 0o640)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	source := sourceAtPath(t, inventory, codexPath)
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "human-outcomes.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation)
	assertExit(t, apply, 0)
	canonical := filepath.Join(library, source.BundleFingerprint)
	for _, expected := range []string{
		"share " + string(source.ID) + " | completed",
		"Operation move-to-canonical | " + codexPath + " -> " + canonical + " | completed",
		"Operation create-link | " + codexPath + " | completed",
		"Operation create-link | " + filepath.Join(fx.home, ".pi", "agent", "skills", "human-outcomes") + " | completed",
	} {
		if !strings.Contains(string(apply.stdout), expected) {
			t.Fatalf("human apply output omitted per-path outcome %q:\n%s", expected, apply.stdout)
		}
	}
}

func TestALE86SuccessfulAndNoopEffectsReportObservedStates(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	piPath := filepath.Join(fx.home, ".pi", "agent", "skills", "observed-state")
	writeFile(t, filepath.Join(piPath, "SKILL.md"), []byte("---\nname: observed-state\ndescription: observed result fixture\n---\nKeep these bytes.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	source := sourceAtPath(t, inventory, piPath)
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "observed-state.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 0)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &applied)
	var completed bool
	for _, effect := range applied.Effects {
		if effect.SourceID == source.ID {
			if effect.State != "completed" {
				t.Fatalf("successful share retained preview state: %#v", effect)
			}
			for _, operation := range effect.Operations {
				if operation.State != "completed" && operation.State != "not-needed" {
					t.Fatalf("successful apply reported an unfinished operation: %#v", operation)
				}
			}
			completed = true
		}
	}
	if !completed {
		t.Fatalf("successful apply omitted source effect: %#v", applied.Effects)
	}
	freshScan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--library", library, "--json")
	assertExit(t, freshScan, 0)
	var freshInventory collection.ConsolidationInventory
	decodeJSON(t, freshScan.stdout, &freshInventory)
	canonical := sourceAtPath(t, freshInventory, filepath.Join(library, source.BundleFingerprint))
	noopDraft := buildConsolidationDraft(freshInventory, nil)
	noopPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "observed-noop.json")
	writeConsolidationDraft(t, noopPath, noopDraft)
	noopPreviewResult := fx.run(t, "consolidate", "preview", "--draft", noopPath, "--json")
	assertExit(t, noopPreviewResult, 0)
	var noopPreview collection.ConsolidationPreview
	decodeJSON(t, noopPreviewResult.stdout, &noopPreview)
	noopApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", noopPath, "--confirm", noopPreview.Confirmation, "--json")
	assertExit(t, noopApply, 0)
	var noop collection.ConsolidationApplyOutcome
	decodeJSON(t, noopApply.stdout, &noop)
	if noop.State != "unchanged" {
		t.Fatalf("exact unchanged rerun changed state: %#v", noop)
	}
	for _, effect := range noop.Effects {
		if effect.SourceID == canonical.ID && effect.State != "unchanged" {
			t.Fatalf("no-op effect was not reported as unchanged: %#v", effect)
		}
	}
}
