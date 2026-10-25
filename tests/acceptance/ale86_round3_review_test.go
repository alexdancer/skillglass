package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/alexdancer/skillglass/internal/companion"
	"github.com/alexdancer/skillglass/internal/setup"
)

func TestALE86HermesExplicitProfileRequiresHomeProvenance(t *testing.T) {
	fx, profile := hermesFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	profileSource := filepath.Join(profile, "skills", "profile-canary")
	manifest := []byte("---\nname: profile-canary\ndescription: direct home profile fixture\n---\nKeep this profile content.\n")
	writeFile(t, filepath.Join(profileSource, "SKILL.md"), manifest, 0o644)
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "pi-canary")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: pi-canary\ndescription: Hermes destination fixture\n---\nKeep this Pi source.\n"), 0o644)

	direct := hermesRun(t, fx, []string{"HERMES_HOME=" + profile}, "consolidate", "scan", "--app", "hermes", "--app", "pi", "--library", library, "--json")
	assertExit(t, direct, 0)
	var directInventory collection.ConsolidationInventory
	decodeJSON(t, direct.stdout, &directInventory)
	directSource := sourceAtPath(t, directInventory, profileSource)
	if !directSource.Eligible || !hasApplicationRelocationProfile(directInventory, "hermes", "hermes-explicit-profile-index-alias-v1") {
		t.Fatalf("direct canonical HOME profile did not retain its recorded relocation proof: %#v", directInventory.Applications)
	}

	outsideProfile := filepath.Join(fx.base, "allowed", "other-parent", ".hermes", "profiles", "research")
	outsideSource := filepath.Join(outsideProfile, "skills", "profile-canary")
	writeFile(t, filepath.Join(outsideSource, "SKILL.md"), manifest, 0o644)
	outside := hermesRun(t, fx, []string{"HERMES_HOME=" + outsideProfile}, "consolidate", "scan", "--app", "hermes", "--app", "pi", "--library", library, "--json")
	assertExit(t, outside, 0)
	var outsideInventory collection.ConsolidationInventory
	decodeJSON(t, outside.stdout, &outsideInventory)
	outsideResult := sourceAtPath(t, outsideInventory, outsideSource)
	if hasApplicationRelocationProfile(outsideInventory, "hermes", "hermes-explicit-profile-index-alias-v1") {
		t.Fatalf("unobserved outside-parent profile received a relocation proof: %#v", outsideInventory.Applications)
	}
	if outsideResult.Eligible {
		sourceDraft := buildConsolidationDraft(outsideInventory, map[collection.SourceID]string{outsideResult.ID: "share"})
		sourceDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-outside-source.json")
		writeConsolidationDraft(t, sourceDraftPath, sourceDraft)
		sourcePreview := hermesRun(t, fx, []string{"HERMES_HOME=" + outsideProfile}, "consolidate", "preview", "--draft", sourceDraftPath, "--json")
		assertExit(t, sourcePreview, 1)
		var refusedSource collection.ConsolidationPreview
		decodeJSON(t, sourcePreview.stdout, &refusedSource)
		if refusedSource.Failure == nil || refusedSource.Failure.Code != "unsupported-native-relocation" {
			t.Fatalf("unobserved Hermes source profile was migratable: %#v", refusedSource)
		}
	} else if len(outsideResult.BlockedReasons) == 0 {
		t.Fatalf("unproven outside-parent source was not left inspectable and keepable: %#v", outsideResult)
	}
	piCandidate := sourceAtPath(t, outsideInventory, piSource)
	draft := buildConsolidationDraft(outsideInventory, map[collection.SourceID]string{piCandidate.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-outside-destination.json")
	writeConsolidationDraft(t, draftPath, draft)
	destinationPreview := hermesRun(t, fx, []string{"HERMES_HOME=" + outsideProfile}, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, destinationPreview, 1)
	var refusedDestination collection.ConsolidationPreview
	decodeJSON(t, destinationPreview.stdout, &refusedDestination)
	if refusedDestination.State != "refused" || refusedDestination.Failure == nil || refusedDestination.Failure.Code != "unsupported-native-relocation" {
		t.Fatalf("unobserved Hermes destination accepted a new link: %#v", refusedDestination)
	}

	activeProfile := hermesRun(t, fx, nil, "consolidate", "scan", "--app", "hermes", "--app", "pi", "--library", library, "--json")
	assertExit(t, activeProfile, 0)
	var activeInventory collection.ConsolidationInventory
	decodeJSON(t, activeProfile.stdout, &activeInventory)
	activeSource := sourceAtPath(t, activeInventory, profileSource)
	if hasApplicationRelocationProfile(activeInventory, "hermes", "hermes-default-root-index-alias-v1") {
		t.Fatalf("active_profile route received an unobserved relocation proof: %#v", activeInventory.Applications)
	}
	if activeSource.Eligible {
		activeDraft := buildConsolidationDraft(activeInventory, map[collection.SourceID]string{activeSource.ID: "share"})
		activeDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-active-profile.json")
		writeConsolidationDraft(t, activeDraftPath, activeDraft)
		activePreview := hermesRun(t, fx, nil, "consolidate", "preview", "--draft", activeDraftPath, "--json")
		assertExit(t, activePreview, 1)
		var refusedActive collection.ConsolidationPreview
		decodeJSON(t, activePreview.stdout, &refusedActive)
		if refusedActive.Failure == nil || refusedActive.Failure.Code != "unsupported-native-relocation" {
			t.Fatalf("active_profile source was migratable without recorded evidence: %#v", refusedActive)
		}
	} else if len(activeSource.BlockedReasons) == 0 {
		t.Fatalf("active_profile source was not left inspectable and keepable: %#v", activeSource)
	}
	if err := os.Remove(filepath.Join(fx.home, ".hermes", "active_profile")); err != nil {
		t.Fatal(err)
	}
	defaultSourcePath := filepath.Join(fx.home, ".hermes", "skills", "default-canary")
	writeFile(t, filepath.Join(defaultSourcePath, "SKILL.md"), []byte("---\nname: default-canary\ndescription: observed default Hermes profile\n---\nKeep this default source.\n"), 0o644)
	defaultScan := hermesRun(t, fx, nil, "consolidate", "scan", "--app", "hermes", "--app", "pi", "--library", library, "--json")
	assertExit(t, defaultScan, 0)
	var defaultInventory collection.ConsolidationInventory
	decodeJSON(t, defaultScan.stdout, &defaultInventory)
	defaultResult := sourceAtPath(t, defaultInventory, defaultSourcePath)
	if !defaultResult.Eligible || !hasApplicationRelocationProfile(defaultInventory, "hermes", "hermes-default-root-index-alias-v1") {
		t.Fatalf("observed HOME default profile lost its relocation proof: %#v", defaultInventory.Applications)
	}
}

func hasApplicationRelocationProfile(inventory collection.ConsolidationInventory, application, id string) bool {
	for _, app := range inventory.Applications {
		if app.Application == application && app.NativeRelocationProfile != nil && app.NativeRelocationProfile.ID == id {
			return true
		}
	}
	return false
}

func TestALE86NamedExecutableCheckAcceptsExactPhysicalCopy(t *testing.T) {
	fx := ale48Fixture(t)
	artifact, err := os.ReadFile(skillglassBinary)
	if err != nil {
		t.Fatal(err)
	}
	physicalBin := filepath.Join(fx.home, "physical-bin")
	copyPath := filepath.Join(physicalBin, "skillglass")
	writeFile(t, copyPath, artifact, 0o700)
	fx.extraExecutables = append(fx.extraExecutables, copyPath)
	originalInfo, err := os.Stat(skillglassBinary)
	if err != nil {
		t.Fatal(err)
	}
	copyInfo, err := os.Stat(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(originalInfo, copyInfo) || !bytes.Equal(readBytes(t, copyPath), artifact) {
		t.Fatal("fixture is not a separate physical copy with identical executable bytes")
	}

	before := treeManifest(t, fx.base)
	for _, args := range [][]string{
		{"setup", "--check", "--companion", companion.ConsolidationName, "--json"},
		{"setup", "--check", "--json"},
	} {
		result := ale48Run(t, fx, "", physicalBin, args...)
		assertExit(t, result, 0)
		var report setup.SetupReport
		decodeJSON(t, result.stdout, &report)
		if report.State != "compatible" || report.Executable.State != "compatible" {
			t.Fatalf("exact physical executable copy was not compatible for %v: %#v", args, report)
		}
	}
	preview := ale48Run(t, fx, "", physicalBin, "setup", "--companion", companion.ConsolidationName, "--app", "codex", "--allow-shared-app", "codeaf", "--json")
	assertExit(t, preview, 0)
	var setupPreview setup.SetupReport
	decodeJSON(t, preview.stdout, &setupPreview)
	if setupPreview.State != "preview" || setupPreview.Executable.State != "compatible" {
		t.Fatalf("named setup did not accept its exact PATH copy: %#v", setupPreview)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	install := ale48Run(t, fx, "", physicalBin, "setup", "--companion", companion.ConsolidationName, "--app", "codex", "--allow-shared-app", "codeaf", "--install", "--json")
	assertExit(t, install, 0)
	var installed setup.SetupReport
	decodeJSON(t, install.stdout, &installed)
	if installed.State != "complete" || installed.Executable.State != "compatible" || len(installed.Results) != 1 || installed.Results[0].State != "installed" {
		t.Fatalf("named install did not accept its exact PATH copy: %#v", installed)
	}
	named, err := companion.BundledNamed(companion.ConsolidationName)
	if err != nil {
		t.Fatal(err)
	}
	ale86AssertInstalledBundle(t, installed.Results[0].Destination, named)
}

func TestALE86RepeatedScansCanonicalizeUnresolvedExclusions(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	piRoot := filepath.Join(fx.home, ".pi", "agent", "skills")
	if err := os.MkdirAll(piRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{
		"broken-one": "missing-one",
		"broken-two": "missing-two",
		"cycle-one":  "cycle-two",
		"cycle-two":  "cycle-one",
	} {
		if err := os.Symlink(target, filepath.Join(piRoot, name)); err != nil {
			t.Fatal(err)
		}
	}
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	args := []string{"consolidate", "scan", "--app", "codex", "--library", library, "--json"}
	first := fx.run(t, args...)
	assertExit(t, first, 1)
	second := fx.run(t, args...)
	assertExit(t, second, 1)
	var firstInventory, secondInventory collection.ConsolidationInventory
	decodeJSON(t, first.stdout, &firstInventory)
	decodeJSON(t, second.stdout, &secondInventory)
	if firstInventory.ScanID != secondInventory.ScanID || !reflect.DeepEqual(firstInventory.Exclusions, secondInventory.Exclusions) {
		t.Fatalf("unchanged unresolved references changed scan identity or exclusion order: first=%s %#v second=%s %#v", firstInventory.ScanID, firstInventory.Exclusions, secondInventory.ScanID, secondInventory.Exclusions)
	}
	if len(firstInventory.Exclusions) < 4 {
		t.Fatalf("fixture did not retain the expected unresolved references: %#v", firstInventory.Exclusions)
	}
}

func TestALE86NoopSelectionWriteFailureReportsUnchangedEffects(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory permissions do not constrain a privileged test process")
	}
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "selection-noop")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: selection-noop\ndescription: portable fixture\n---\nKeep these instructions.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	source := sourceAtPath(t, inventory, piSource)
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "selection-noop-initial.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 0)

	noOpDraft := selectionOnlyDraft(t, fx, library)
	noOpPreview := fx.run(t, "consolidate", "preview", "--draft", noOpDraft.path, "--json")
	assertExit(t, noOpPreview, 0)
	var noOpPlan collection.ConsolidationPreview
	decodeJSON(t, noOpPreview.stdout, &noOpPlan)
	selectionPath := filepath.Join(fx.xdgConfig, "skillglass", "consolidation.json")
	selectionBefore := readBytes(t, selectionPath)
	selectionParent := filepath.Dir(selectionPath)
	parentInfo, err := os.Stat(selectionParent)
	if err != nil {
		t.Fatal(err)
	}
	originalMode := parentInfo.Mode().Perm()
	selectionStatBefore, err := os.Stat(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	backupRoot := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups")
	backupTreeBefore := treeManifest(t, backupRoot)
	piTreeBefore := treeManifest(t, filepath.Join(fx.home, ".pi", "agent", "skills"))
	codexTreeBefore := treeManifest(t, filepath.Join(fx.home, ".codex", "skills"))
	libraryTreeBefore := treeManifest(t, library)
	if err := os.Chmod(selectionParent, 0o500); err != nil {
		t.Fatal(err)
	}
	restoredPermissions := false
	t.Cleanup(func() {
		if !restoredPermissions {
			if err := os.Chmod(selectionParent, originalMode); err != nil {
				t.Error(err)
			}
		}
	})
	failedApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", noOpDraft.path, "--confirm", noOpPlan.Confirmation, "--json")
	assertExit(t, failedApply, 1)
	if err := os.Chmod(selectionParent, originalMode); err != nil {
		t.Fatal(err)
	}
	restoredPermissions = true
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, failedApply.stdout, &outcome)
	if outcome.State != "refused" || outcome.Failure == nil || outcome.Failure.Code != "selection-write" || outcome.RecoveryID == "" || outcome.RecoveryID != noOpPlan.RecoveryID {
		t.Fatalf("no-op preference failure was not reported distinctly: %#v", outcome)
	}
	var sharedEffect *collection.ConsolidationEffect
	for i := range outcome.Effects {
		if outcome.Effects[i].Action == "share" {
			sharedEffect = &outcome.Effects[i]
		}
	}
	if sharedEffect == nil || sharedEffect.State != "unchanged" || sharedEffect.BackupPath != "" {
		t.Fatalf("failed no-op retained planned filesystem effects or recovery path: %#v", outcome.Effects)
	}
	if !bytes.Equal(selectionBefore, readBytes(t, selectionPath)) {
		t.Fatal("failed no-op changed remembered selection bytes")
	}
	if !reflect.DeepEqual(piTreeBefore, treeManifest(t, filepath.Join(fx.home, ".pi", "agent", "skills"))) || !reflect.DeepEqual(codexTreeBefore, treeManifest(t, filepath.Join(fx.home, ".codex", "skills"))) || !reflect.DeepEqual(libraryTreeBefore, treeManifest(t, library)) {
		t.Fatal("failed selection-only apply changed a source, link, or canonical library tree")
	}
	backupTreeAfter := treeManifest(t, backupRoot)
	if reflect.DeepEqual(backupTreeBefore, backupTreeAfter) {
		t.Fatal("failed selection-only scope change did not retain its durable recovery journal")
	}
	recordPath := filepath.Join(backupRoot, outcome.RecoveryID, "record.json")
	recordRaw := readBytes(t, recordPath)
	var record struct {
		State                 string            `json:"state"`
		Entries               []json.RawMessage `json:"entries"`
		PriorSelection        []byte            `json:"prior_selection"`
		AppliedSelection      []byte            `json:"applied_selection"`
		PendingSelectionApply bool              `json:"pending_selection_apply"`
		SelectionApplied      bool              `json:"selection_applied"`
		Draft                 struct {
			Choices []json.RawMessage `json:"choices"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(recordRaw, &record); err != nil {
		t.Fatal(err)
	}
	if record.State != "partial" || len(record.Entries) != 0 || len(record.Draft.Choices) != 0 || !bytes.Equal(record.PriorSelection, selectionBefore) || !record.PendingSelectionApply || record.SelectionApplied {
		t.Fatalf("failed preference-only apply did not retain metadata-only pending recovery: %#v", record)
	}
	recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", outcome.RecoveryID, "--json")
	assertExit(t, recoveryPreviewResult, 0)
	var recoveryPreview collection.ConsolidationPreview
	decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
	if recoveryPreview.State != "ready" || len(recoveryPreview.Effects) != 1 || recoveryPreview.Effects[0].Action != "restore-selection" {
		t.Fatalf("failed preference-only apply did not expose selection-only recovery: %#v", recoveryPreview)
	}
	recoveryApply := changeCommand(t, fx, "", "consolidate", "recover", "--backup", outcome.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
	assertExit(t, recoveryApply, 0)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, recoveryApply.stdout, &recovered)
	if recovered.State != "recovered" || !recovered.Verified {
		t.Fatalf("selection-only recovery did not complete: %#v", recovered)
	}
	selectionStatAfterRecovery, err := os.Stat(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(selectionBefore, readBytes(t, selectionPath)) || !selectionStatBefore.ModTime().Equal(selectionStatAfterRecovery.ModTime()) {
		t.Fatal("recovery changed the already-restored prior selection")
	}
}

type ale86RecoveryFixture struct {
	library          string
	canonical        string
	recoveryID       string
	firstSourcePath  string
	secondSourcePath string
	firstManifest    []byte
	secondManifest   []byte
}

func prepareALE86Recovery(t *testing.T, fx fixture, archive bool) ale86RecoveryFixture {
	t.Helper()
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	if err := os.MkdirAll(filepath.Join(fx.home, ".pi", "agent", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	firstPath := filepath.Join(fx.home, ".codex", "skills", "retry-source")
	firstManifest := []byte("---\nname: retry-source\ndescription: recovery retry fixture\n---\nRestore this exact source.\n")
	var secondPath string
	var secondManifest []byte
	firstApp, secondApp := "codex", "pi"
	if archive {
		// CodeAF can consume Codex's personal skills while unselected, so keep
		// this archive/recovery fixture on independently selected Pi and Hermes
		// roots. The fixture uses Pi's proven agent skills root and Hermes's
		// proven default root with no active_profile marker.
		firstApp, secondApp = "pi", "hermes"
		firstPath = filepath.Join(fx.home, ".pi", "agent", "skills", "recovery-archive")
		secondPath = filepath.Join(fx.home, ".hermes", "skills", "recovery-archive")
		firstManifest = []byte("---\nname: recovery-archive\ndescription: divergent recovery fixture\n---\nWinner source bytes.\n")
		secondManifest = []byte("---\nname: recovery-archive\ndescription: divergent recovery fixture\n---\nArchived source bytes.\n")
		writeFile(t, filepath.Join(firstPath, "SKILL.md"), firstManifest, 0o644)
		writeFile(t, filepath.Join(secondPath, "SKILL.md"), secondManifest, 0o644)
	} else {
		writeFile(t, filepath.Join(firstPath, "SKILL.md"), firstManifest, 0o644)
	}
	scan := fx.run(t, "consolidate", "scan", "--app", firstApp, "--app", secondApp, "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	first := sourceAtPath(t, inventory, firstPath)
	actions := map[collection.SourceID]string{first.ID: "share"}
	if archive {
		second := sourceAtPath(t, inventory, secondPath)
		winner, archived := first, second
		if winner.ID > archived.ID {
			winner, archived = archived, winner
		}
		actions = map[collection.SourceID]string{winner.ID: "share", archived.ID: "archive"}
		if winner.SourcePath != firstPath {
			firstPath, secondPath = secondPath, firstPath
			firstManifest, secondManifest = secondManifest, firstManifest
		}
	}
	draft := buildConsolidationDraft(inventory, actions)
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "recovery-initial.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	var canonical string
	for _, effect := range preview.Effects {
		if effect.Action == "share" {
			canonical = effect.CanonicalPath
			break
		}
	}
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 0)
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &outcome)
	if outcome.State != "applied" || outcome.RecoveryID == "" {
		t.Fatalf("initial share/archive did not apply: %#v", outcome)
	}
	return ale86RecoveryFixture{library: library, canonical: canonical, recoveryID: outcome.RecoveryID, firstSourcePath: firstPath, secondSourcePath: secondPath, firstManifest: firstManifest, secondManifest: secondManifest}
}

func forceALE86PartialRecovery(t *testing.T, fx fixture, fixture ale86RecoveryFixture) {
	t.Helper()
	info, err := os.Stat(fixture.library)
	if err != nil {
		t.Fatal(err)
	}
	originalMode := info.Mode().Perm()
	if err := os.Chmod(fixture.library, 0o555); err != nil {
		t.Fatal(err)
	}
	restoredPermissions := false
	t.Cleanup(func() {
		if !restoredPermissions {
			if err := os.Chmod(fixture.library, originalMode); err != nil {
				t.Error(err)
			}
		}
	})
	previewResult := fx.run(t, "consolidate", "recover", "--backup", fixture.recoveryID, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" {
		t.Fatalf("initial recovery preview was not ready: %#v", preview)
	}
	apply := changeCommand(t, fx, "", "consolidate", "recover", "--backup", fixture.recoveryID, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 1)
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &outcome)
	if outcome.State != "partial" || outcome.Failure == nil || outcome.Failure.Code != "partial-recovery" {
		t.Fatalf("controlled recovery failure did not leave a partial journal: %#v", outcome)
	}
	if err := os.Chmod(fixture.library, originalMode); err != nil {
		t.Fatal(err)
	}
	restoredPermissions = true
}

func TestALE86RecoveryRetryRefusesRecreatedRemovedLink(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory permissions do not constrain a privileged test process")
	}
	t.Run("unmodified retry completes", func(t *testing.T) {
		fx := newFixture(t)
		fixture := prepareALE86Recovery(t, fx, false)
		forceALE86PartialRecovery(t, fx, fixture)
		previewResult := fx.run(t, "consolidate", "recover", "--backup", fixture.recoveryID, "--json")
		assertExit(t, previewResult, 0)
		var preview collection.ConsolidationPreview
		decodeJSON(t, previewResult.stdout, &preview)
		if preview.State != "ready" {
			t.Fatalf("untouched recovery retry was refused: %#v", preview)
		}
		apply := changeCommand(t, fx, "", "consolidate", "recover", "--backup", fixture.recoveryID, "--confirm", preview.Confirmation, "--json")
		assertExit(t, apply, 0)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &outcome)
		if outcome.State != "recovered" || !outcome.Verified {
			t.Fatalf("untouched retry did not complete recovery: %#v", outcome)
		}
		if got := readBytes(t, filepath.Join(fixture.firstSourcePath, "SKILL.md")); !bytes.Equal(got, fixture.firstManifest) {
			t.Fatalf("recovered source bytes changed: %q", got)
		}
		if _, err := os.Lstat(fixture.canonical); !os.IsNotExist(err) {
			t.Fatalf("canonical bundle remained after successful recovery: %v", err)
		}
	})

	t.Run("reappeared link is preserved and refused", func(t *testing.T) {
		fx := newFixture(t)
		fixture := prepareALE86Recovery(t, fx, false)
		forceALE86PartialRecovery(t, fx, fixture)
		piLink := filepath.Join(fx.home, ".pi", "agent", "skills", "retry-source")
		if err := os.Symlink(fixture.canonical, piLink); err != nil {
			t.Fatal(err)
		}
		canonicalBefore := treeManifest(t, fixture.canonical)
		before := treeManifest(t, fx.base)
		preview := fx.run(t, "consolidate", "recover", "--backup", fixture.recoveryID, "--json")
		assertExit(t, preview, 1)
		var refused collection.ConsolidationPreview
		decodeJSON(t, preview.stdout, &refused)
		if refused.State != "refused" || refused.Failure == nil || refused.Failure.Code != "recovery-freshness" {
			t.Fatalf("recreated removed link did not refuse recovery: %#v", refused)
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		if got, err := os.Readlink(piLink); err != nil || filepath.Clean(got) != filepath.Clean(fixture.canonical) {
			t.Fatalf("later Pi link was not preserved: %s %v", got, err)
		}
		if !reflect.DeepEqual(canonicalBefore, treeManifest(t, fixture.canonical)) {
			t.Fatal("recovery refusal changed canonical bundle content")
		}
		if _, err := os.Stat(piLink); err != nil {
			t.Fatalf("preserved Pi link became dangling: %v", err)
		}
	})
}

func TestALE86RecoveryRetryAllowsRecordedArchiveRestore(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory permissions do not constrain a privileged test process")
	}
	fx := newFixture(t)
	fixture := prepareALE86Recovery(t, fx, true)
	t.Logf("archive recovery order: winner=%s archive=%s", fixture.firstSourcePath, fixture.secondSourcePath)
	forceALE86PartialRecovery(t, fx, fixture)
	if got := readBytes(t, filepath.Join(fixture.secondSourcePath, "SKILL.md")); !bytes.Equal(got, fixture.secondManifest) {
		t.Fatalf("recorded archive restore did not preserve its original source path: %q", got)
	}
	previewResult := fx.run(t, "consolidate", "recover", "--backup", fixture.recoveryID, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" {
		t.Fatalf("recorded archived source at a removed-link path was not accepted: %#v", preview)
	}
	apply := changeCommand(t, fx, "", "consolidate", "recover", "--backup", fixture.recoveryID, "--confirm", preview.Confirmation, "--json")
	assertExit(t, apply, 0)
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &outcome)
	if outcome.State != "recovered" || !outcome.Verified {
		t.Fatalf("winner/archive recovery retry did not finish: %#v", outcome)
	}
	for _, source := range []struct {
		path     string
		manifest []byte
	}{{fixture.firstSourcePath, fixture.firstManifest}, {fixture.secondSourcePath, fixture.secondManifest}} {
		if got := readBytes(t, filepath.Join(source.path, "SKILL.md")); !bytes.Equal(got, source.manifest) {
			t.Fatalf("recovery changed original source at %s: %q", source.path, got)
		}
	}
	if _, err := os.Lstat(fixture.canonical); !os.IsNotExist(err) {
		t.Fatalf("canonical winner remained after recovery: %v", err)
	}
}
