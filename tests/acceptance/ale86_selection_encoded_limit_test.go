package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86OversizedSelectionRefusesBeforeEffects(t *testing.T) {
	fx := piFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	if err := os.MkdirAll(library, 0o700); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(fx.base, "allowed", "roots", strings.Repeat("p", 200))
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	const rootCount = 150
	roots := make([]string, 0, rootCount)
	for index := 0; index < rootCount; index++ {
		root := filepath.Join(parent, fmt.Sprintf("%03d-%s", index, strings.Repeat("r", 235)))
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
	}
	sourcePath := filepath.Join(roots[0], "selection-limit-canary")
	writeFile(t, filepath.Join(sourcePath, "SKILL.md"), []byte("---\nname: selection-limit-canary\ndescription: public fixture\n---\nKeep in place.\n"), 0o600)
	scanArgs := []string{"consolidate", "scan", "--app", "pi", "--library", library, "--json"}
	for _, root := range roots {
		scanArgs = append(scanArgs, "--root", root)
	}
	scan := fx.run(t, scanArgs...)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	if inventory.SelectedCoverage != collection.Complete || !inventory.LibrarySafe || len(inventory.Scope.Roots) != rootCount {
		t.Fatalf("large explicit root scope is not complete and safe: coverage=%s library_safe=%t roots=%d", inventory.SelectedCoverage, inventory.LibrarySafe, len(inventory.Scope.Roots))
	}
	source := sourceAtPath(t, inventory, sourcePath)
	if !source.Eligible {
		t.Fatalf("fixture source is not eligible: %v", source.BlockedReasons)
	}
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "keep"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "oversized-selection.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" || preview.Confirmation == "" {
		t.Fatalf("oversized-selection fixture did not reach the apply boundary: state=%s failure=%+v", preview.State, preview.Failure)
	}
	expected, err := json.MarshalIndent(struct {
		SchemaVersion int      `json:"schema_version"`
		Applications  []string `json:"applications"`
		Roots         []string `json:"roots,omitempty"`
		Library       string   `json:"library"`
	}{collection.ConsolidationSchemaVersion, inventory.Scope.Applications, inventory.Scope.Roots, inventory.Scope.Library}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	expected = append(expected, '\n')
	if len(expected) <= 64<<10 {
		t.Fatalf("fixture did not exceed the encoded selection limit: %d bytes", len(expected))
	}
	before := treeManifest(t, fx.base)
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &outcome)
	failureCode := ""
	if outcome.Failure != nil {
		failureCode = outcome.Failure.Code
	}
	t.Logf("public oversized selection: exit=%d state=%s selection_updated=%t recovery_id_present=%t encoded_bytes=%d failure_code=%s", apply.code, outcome.State, outcome.SelectionUpdated, outcome.RecoveryID != "", len(expected), failureCode)
	if apply.code == 0 || outcome.State != "refused" || outcome.SelectionUpdated || outcome.RecoveryID != "" || outcome.Failure == nil || !strings.Contains(outcome.Failure.Message, "64 KiB encoded limit") {
		t.Fatalf("oversized selection was not refused with no recovery claim: exit=%d state=%s updated=%t recovery=%q failure=%+v", apply.code, outcome.State, outcome.SelectionUpdated, outcome.RecoveryID, outcome.Failure)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("oversized selection refusal changed the fixture tree")
	}
	selectionPath := filepath.Join(fx.xdgConfig, "skillglass", "consolidation.json")
	if _, err := os.Stat(selectionPath); !os.IsNotExist(err) {
		t.Fatalf("oversized selection refusal wrote remembered selection state: %v", err)
	}
	planDir := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", preview.RecoveryID)
	if _, err := os.Stat(planDir); !os.IsNotExist(err) {
		t.Fatalf("oversized selection refusal left a recovery plan directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
		t.Fatalf("oversized selection refusal changed the selected source: %v", err)
	}
}
