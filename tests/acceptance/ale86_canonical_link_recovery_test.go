package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86CanonicalSourceConnectsMissingCodexLinkAndRecovers(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	piSourcePath := filepath.Join(fx.home, ".pi", "agent", "skills", "canonical-link-roundtrip")
	writeFile(t, filepath.Join(piSourcePath, "SKILL.md"), []byte("---\nname: canonical-link-roundtrip\ndescription: connect a missing consumer link\n---\nPreserve this canonical bundle.\n"), 0o644)
	canonicalPath := ""
	backupPath := ""

	initialScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--library", library, "--json")
	writeEvidence(t, "canonical-link/initial-scan.json", initialScan.stdout)
	assertExit(t, initialScan, 0)
	var initialInventory collection.ConsolidationInventory
	decodeJSON(t, initialScan.stdout, &initialInventory)
	initialSource := sourceAtPath(t, initialInventory, piSourcePath)
	initialDraft := buildConsolidationDraft(initialInventory, map[collection.SourceID]string{initialSource.ID: "share"})
	initialDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "canonical-link-initial.json")
	writeConsolidationDraft(t, initialDraftPath, initialDraft)
	initialPreviewResult := fx.run(t, "consolidate", "preview", "--draft", initialDraftPath, "--json")
	writeEvidence(t, "canonical-link/initial-preview.json", initialPreviewResult.stdout)
	assertExit(t, initialPreviewResult, 0)
	var initialPreview collection.ConsolidationPreview
	decodeJSON(t, initialPreviewResult.stdout, &initialPreview)
	if initialPreview.State != "ready" || initialPreview.Confirmation == "" {
		t.Fatalf("initial canonical connection was not ready: %#v", initialPreview)
	}
	initialApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", initialDraftPath, "--confirm", initialPreview.Confirmation, "--json")
	writeEvidence(t, "canonical-link/initial-apply.json", initialApply.stdout)
	assertExit(t, initialApply, 0)
	var initialOutcome collection.ConsolidationApplyOutcome
	decodeJSON(t, initialApply.stdout, &initialOutcome)
	if initialOutcome.State != "applied" || !initialOutcome.Verified || initialOutcome.RecoveryID == "" {
		t.Fatalf("initial Pi source was not moved into the canonical library: %#v", initialOutcome)
	}
	canonicalPath = filepath.Join(library, initialSource.BundleFingerprint)
	if resolved, err := filepath.EvalSymlinks(piSourcePath); err != nil || filepath.Clean(resolved) != filepath.Clean(canonicalPath) {
		t.Fatalf("initial Pi connection does not resolve to the canonical bundle: resolved=%q err=%v", resolved, err)
	}

	connectScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "canonical-link/connect-scan.json", connectScan.stdout)
	assertExit(t, connectScan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, connectScan.stdout, &inventory)
	canonicalSource := sourceAtPath(t, inventory, canonicalPath)
	if !canonicalSource.Eligible || canonicalSource.FingerprintCoverage != collection.Complete || canonicalSource.BundleFingerprint != initialSource.BundleFingerprint {
		t.Fatalf("canonical source is not fully proven on the fresh scan: %#v", canonicalSource)
	}
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{canonicalSource.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "canonical-link-connect.json")
	writeConsolidationDraft(t, draftPath, draft)
	connectPreviewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "canonical-link/connect-preview.json", connectPreviewResult.stdout)
	assertExit(t, connectPreviewResult, 0)
	var connectPreview collection.ConsolidationPreview
	decodeJSON(t, connectPreviewResult.stdout, &connectPreview)
	if connectPreview.State != "ready" || connectPreview.Confirmation == "" {
		t.Fatalf("canonical source with a missing Codex destination was not ready: %#v", connectPreview)
	}
	var codexLinkPlanned bool
	for _, effect := range connectPreview.Effects {
		if effect.SourceID != canonicalSource.ID || effect.Action != "share" {
			continue
		}
		if filepath.Clean(effect.SourcePath) != filepath.Clean(canonicalPath) || filepath.Clean(effect.CanonicalPath) != filepath.Clean(canonicalPath) {
			t.Fatalf("link-only effect did not retain the canonical source path: %#v", effect)
		}
		backupPath = effect.BackupPath
		for _, destination := range effect.Destinations {
			if filepath.Clean(destination) == filepath.Join(fx.home, ".codex", "skills", "canonical-link-roundtrip") {
				codexLinkPlanned = true
			}
		}
	}
	if !codexLinkPlanned {
		t.Fatalf("canonical plan did not include the missing Codex link: %#v", connectPreview.Effects)
	}

	piRoot := filepath.Join(fx.home, ".pi", "agent", "skills")
	codexRoot := filepath.Join(fx.home, ".codex", "skills")
	piBeforeConnect := treeManifest(t, piRoot)
	codexBeforeConnect := treeManifest(t, codexRoot)
	libraryBeforeConnect := treeManifest(t, library)
	codexLink := filepath.Join(codexRoot, "canonical-link-roundtrip")
	connectApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", connectPreview.Confirmation, "--json")
	writeEvidence(t, "canonical-link/connect-apply.json", connectApply.stdout)
	assertExit(t, connectApply, 0)
	var connected collection.ConsolidationApplyOutcome
	decodeJSON(t, connectApply.stdout, &connected)
	if connected.State != "applied" || !connected.Verified || connected.RecoveryID == "" {
		t.Fatalf("canonical link connection did not apply: %#v", connected)
	}
	if resolved, err := filepath.EvalSymlinks(codexLink); err != nil || filepath.Clean(resolved) != filepath.Clean(canonicalPath) {
		t.Fatalf("new Codex link does not resolve to canonical bundle: resolved=%q err=%v", resolved, err)
	}
	if info, err := os.Lstat(canonicalPath); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("canonical bundle moved during a link-only apply: info=%v err=%v", info, err)
	}
	if backupPath != "" {
		if _, err := os.Lstat(backupPath); !os.IsNotExist(err) {
			t.Fatalf("link-only apply created a duplicate recovery bundle: %s err=%v", backupPath, err)
		}
	}

	recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", connected.RecoveryID, "--json")
	writeEvidence(t, "canonical-link/recovery-preview.json", recoveryPreviewResult.stdout)
	assertExit(t, recoveryPreviewResult, 0)
	var recoveryPreview collection.ConsolidationPreview
	decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
	if recoveryPreview.State != "ready" || recoveryPreview.Confirmation == "" {
		t.Fatalf("link-only canonical connection was not recoverable: %#v", recoveryPreview)
	}
	recover := changeCommand(t, fx, "", "consolidate", "recover", "--backup", connected.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
	writeEvidence(t, "canonical-link/recovery-apply.json", recover.stdout)
	assertExit(t, recover, 0)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, recover.stdout, &recovered)
	if recovered.State != "recovered" || !recovered.Verified {
		t.Fatalf("link-only canonical connection did not recover: %#v", recovered)
	}
	if _, err := os.Lstat(codexLink); !os.IsNotExist(err) {
		t.Fatalf("recovery left the newly-created Codex link: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(piSourcePath); err != nil || filepath.Clean(resolved) != filepath.Clean(canonicalPath) {
		t.Fatalf("recovery damaged the prior Pi link: resolved=%q err=%v", resolved, err)
	}
	if info, err := os.Lstat(canonicalPath); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("recovery moved or removed the canonical bundle: info=%v err=%v", info, err)
	}
	if !bytes.Equal(piBeforeConnect, treeManifest(t, piRoot)) || !bytes.Equal(codexBeforeConnect, treeManifest(t, codexRoot)) || !bytes.Equal(libraryBeforeConnect, treeManifest(t, library)) {
		t.Fatal("connect and recovery did not restore the exact Pi, Codex, and canonical-library trees")
	}
}
