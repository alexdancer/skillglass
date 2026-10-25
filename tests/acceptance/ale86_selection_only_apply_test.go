package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86SelectionOnlyApplyReportsPreferenceWriteAndExactRerunIsNoop(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)

	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "selection-noop")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), []byte("---\nname: selection-noop\ndescription: portable fixture\n---\nKeep these instructions.\n"), 0o644)

	firstScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	assertExit(t, firstScan, 0)
	var firstInventory collection.ConsolidationInventory
	decodeJSON(t, firstScan.stdout, &firstInventory)
	firstSource := sourceAtPath(t, firstInventory, piSource)
	firstDraft := buildConsolidationDraft(firstInventory, map[collection.SourceID]string{firstSource.ID: "share"})
	firstDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "selection-first-draft.json")
	writeConsolidationDraft(t, firstDraftPath, firstDraft)
	firstPreview := fx.run(t, "consolidate", "preview", "--draft", firstDraftPath, "--json")
	assertExit(t, firstPreview, 0)
	var firstPlan collection.ConsolidationPreview
	decodeJSON(t, firstPreview.stdout, &firstPlan)
	firstApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", firstDraftPath, "--confirm", firstPlan.Confirmation, "--json")
	assertExit(t, firstApply, 0)
	var firstOutcome collection.ConsolidationApplyOutcome
	decodeJSON(t, firstApply.stdout, &firstOutcome)
	if firstOutcome.State != "applied" || !firstOutcome.Verified {
		t.Fatalf("initial Pi+Codex sharing did not apply: %#v", firstOutcome)
	}

	selectionPath := filepath.Join(fx.xdgConfig, "skillglass", "consolidation.json")
	selectionBefore, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	piOnly := selectionOnlyDraft(t, fx, library)
	selectionPreview := fx.run(t, "consolidate", "preview", "--draft", piOnly.path, "--json")
	assertExit(t, selectionPreview, 0)
	var selectionPlan collection.ConsolidationPreview
	decodeJSON(t, selectionPreview.stdout, &selectionPlan)
	if selectionPlan.State != "ready" || len(selectionPlan.Effects) == 0 {
		t.Fatalf("selection-only plan not ready: %#v", selectionPlan)
	}
	for _, effect := range selectionPlan.Effects {
		if effect.Action == "share" && (filepath.Clean(effect.SourcePath) != filepath.Clean(effect.CanonicalPath) || len(effect.Destinations) != 0) {
			t.Fatalf("fixture should exercise an already-shared no-op, got %#v", effect)
		}
	}
	selectionApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", piOnly.path, "--confirm", selectionPlan.Confirmation, "--json")
	assertExit(t, selectionApply, 0)
	var selectionOutcome struct {
		State            string                           `json:"state"`
		SelectionUpdated bool                             `json:"selection_updated"`
		Verified         bool                             `json:"verified"`
		Reason           string                           `json:"reason"`
		Effects          []collection.ConsolidationEffect `json:"effects"`
	}
	decodeJSON(t, selectionApply.stdout, &selectionOutcome)
	if selectionOutcome.State != "selection-updated" || !selectionOutcome.SelectionUpdated || !selectionOutcome.Verified {
		t.Fatalf("selection-only apply must report the persisted preference change truthfully: %#v", selectionOutcome)
	}
	for _, effect := range selectionOutcome.Effects {
		if effect.Action == "share" && effect.State != "unchanged" {
			t.Fatalf("selection-only apply reported a filesystem change: %#v", effect)
		}
	}
	selectionAfter, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	selectionStatAfter, err := os.Stat(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	var remembered struct {
		Applications []string `json:"applications"`
	}
	if err := json.Unmarshal(selectionAfter, &remembered); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(remembered.Applications, []string{"pi"}) || reflect.DeepEqual(selectionBefore, selectionAfter) {
		t.Fatalf("selection-only apply did not persist the Pi-only selection: before=%s after=%s", selectionBefore, selectionAfter)
	}

	piOnly = selectionOnlyDraft(t, fx, library)
	noOpPreview := fx.run(t, "consolidate", "preview", "--draft", piOnly.path, "--json")
	assertExit(t, noOpPreview, 0)
	var noOpPlan collection.ConsolidationPreview
	decodeJSON(t, noOpPreview.stdout, &noOpPlan)
	noOpApply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", piOnly.path, "--confirm", noOpPlan.Confirmation, "--json")
	assertExit(t, noOpApply, 0)
	var noOpOutcome struct {
		State            string                           `json:"state"`
		SelectionUpdated bool                             `json:"selection_updated"`
		Verified         bool                             `json:"verified"`
		Effects          []collection.ConsolidationEffect `json:"effects"`
	}
	decodeJSON(t, noOpApply.stdout, &noOpOutcome)
	if noOpOutcome.State != "unchanged" || noOpOutcome.SelectionUpdated || !noOpOutcome.Verified {
		t.Fatalf("exact rerun must report an unchanged, write-free selection: %#v", noOpOutcome)
	}
	for _, effect := range noOpOutcome.Effects {
		if effect.Action == "share" && effect.State != "unchanged" {
			t.Fatalf("unchanged rerun reported a filesystem change: %#v", effect)
		}
	}
	selectionFinal, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	selectionStatFinal, err := os.Stat(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !selectionStatAfter.ModTime().Equal(selectionStatFinal.ModTime()) || !reflect.DeepEqual(selectionAfter, selectionFinal) {
		t.Fatal("unchanged rerun changed remembered selection bytes or timestamp")
	}
}

type selectionDraftFile struct {
	path string
}

func selectionOnlyDraft(t *testing.T, fx fixture, library string) selectionDraftFile {
	t.Helper()
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	var source collection.ConsolidationSource
	for _, candidate := range inventory.Sources {
		if candidate.Name == "selection-noop" {
			source = candidate
			break
		}
	}
	if source.ID == "" {
		t.Fatal("shared fixture source missing from Pi-only scan")
	}
	canonical := filepath.Join(library, source.BundleFingerprint)
	source = sourceAtPath(t, inventory, canonical)
	if !source.Eligible {
		t.Fatalf("shared canonical source became ineligible in Pi-only selection: %#v", source)
	}
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
	path := filepath.Join(fx.base, "allowed", "ownership-artifacts", "selection-pi-only-draft.json")
	writeConsolidationDraft(t, path, draft)
	return selectionDraftFile{path: path}
}
