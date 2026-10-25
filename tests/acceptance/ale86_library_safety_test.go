package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86UnsafeLibraryRouteRefusesApplyAndRecovery(t *testing.T) {
	t.Run("route-added-after-preview", func(t *testing.T) {
		fx := newFixture(t)
		writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
		piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "library-safety-probe")
		writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: library-safety-probe\ndescription: isolated library safety fixture\n---\nPreserve this source unless the exact reviewed plan remains safe.\n"), 0o644)
		library := filepath.Join(fx.xdgData, "skillglass", "skills")
		if err := os.MkdirAll(library, 0o700); err != nil {
			t.Fatal(err)
		}
		scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
		assertExit(t, scan, 0)
		var inventory collection.ConsolidationInventory
		decodeJSON(t, scan.stdout, &inventory)
		if !inventory.LibrarySafe {
			t.Fatalf("isolated library was unsafe before approval: %v", inventory.LibraryReasons)
		}
		source := sourceAtPath(t, inventory, piSource)
		draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
		draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "library-safety-apply.json")
		writeConsolidationDraft(t, draftPath, draft)
		writeEvidence(t, "library-safety/apply-scan.json", scan.stdout)
		draftBytes, err := json.MarshalIndent(draft, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeEvidence(t, "library-safety/apply-draft.json", append(draftBytes, '\n'))

		previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
		assertExit(t, previewResult, 0)
		var preview collection.ConsolidationPreview
		decodeJSON(t, previewResult.stdout, &preview)
		if preview.State != "ready" || preview.Confirmation == "" {
			t.Fatalf("safe initial plan did not produce an approval token: %#v", preview)
		}
		writeEvidence(t, "library-safety/apply-preview.json", previewResult.stdout)

		hiddenAlias := filepath.Join(fx.home, ".codex", "skills", ".shared")
		if err := os.Symlink(library, hiddenAlias); err != nil {
			t.Fatal(err)
		}
		if target, err := filepath.EvalSymlinks(hiddenAlias); err != nil || filepath.Clean(target) != filepath.Clean(library) {
			t.Fatalf("hidden route control does not resolve to the library: target=%s err=%v", target, err)
		}
		beforeApply := treeManifest(t, fx.base)
		apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
		writeEvidence(t, "library-safety/apply-refused.json", apply.stdout)
		assertExit(t, apply, 1)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &outcome)
		if outcome.State != "refused" || outcome.Failure == nil || outcome.Failure.Code != "unsafe-library" {
			t.Fatalf("apply did not reject a newly exposed canonical library: %#v", outcome)
		}
		assertTreeUnchanged(t, beforeApply, treeManifest(t, fx.base))
		if _, err := os.Lstat(filepath.Join(fx.xdgConfig, "skillglass", "consolidation.json")); !os.IsNotExist(err) {
			t.Fatalf("refused apply recorded the selected applications: %v", err)
		}
	})

	t.Run("unsafe-library-blocks-recovery-preview-and-apply", func(t *testing.T) {
		fx, sourcePath, library := recoveryFixture(t)
		applied, _, _, _, _ := prepareRecoveryFixture(t, fx, sourcePath, library, "library-safety/recovery")
		hiddenAlias := filepath.Join(fx.home, ".codex", "skills", ".shared")
		if err := os.Symlink(library, hiddenAlias); err != nil {
			t.Fatal(err)
		}
		if target, err := filepath.EvalSymlinks(hiddenAlias); err != nil || filepath.Clean(target) != filepath.Clean(library) {
			t.Fatalf("hidden route control does not resolve to the library: target=%s err=%v", target, err)
		}
		journalPath := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", applied.RecoveryID, "record.json")
		journalBefore := readBytes(t, journalPath)
		beforeRecovery := treeManifest(t, fx.base)

		preview := fx.run(t, "consolidate", "recover", "--backup", applied.RecoveryID, "--json")
		writeEvidence(t, "library-safety/recovery-refused-preview.json", preview.stdout)
		assertExit(t, preview, 1)
		var recoveryPreview collection.ConsolidationPreview
		decodeJSON(t, preview.stdout, &recoveryPreview)
		if recoveryPreview.State != "refused" || recoveryPreview.Failure == nil || recoveryPreview.Failure.Code != "unsafe-library" {
			t.Fatalf("recovery preview did not disclose the unsafe-library refusal: %#v", recoveryPreview)
		}

		apply := changeCommand(t, fx, "", "consolidate", "recover", "--backup", applied.RecoveryID, "--confirm", "stale-test-token", "--json")
		writeEvidence(t, "library-safety/recovery-refused-apply.json", apply.stdout)
		assertExit(t, apply, 1)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &outcome)
		if outcome.State != "refused" || outcome.Failure == nil || outcome.Failure.Code != "unsafe-library" {
			t.Fatalf("recovery apply did not revalidate the unsafe library: %#v", outcome)
		}
		assertTreeUnchanged(t, beforeRecovery, treeManifest(t, fx.base))
		if after := readBytes(t, journalPath); string(after) != string(journalBefore) {
			t.Fatal("unsafe-library recovery refusal modified its journal")
		}
	})
}
