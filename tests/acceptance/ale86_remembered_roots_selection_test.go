package acceptance_test

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86ExplicitAppsPreserveRememberedCustomRoots(t *testing.T) {
	t.Run("change-app-without-root-retains-roots-and-uses-default-library", func(t *testing.T) {
		fx, rootR, _, defaultLibrary := rememberedRootFixture(t)
		rememberedLibrary := filepath.Join(fx.base, "allowed", "remembered-library")
		initial := rememberedRootScan(t, fx, "initial", "--app", "pi", "--root", rootR, "--library", rememberedLibrary)
		assertRememberedRoots(t, "initial scan", initial, rootR)
		if filepath.Clean(initial.Scope.Library) != filepath.Clean(rememberedLibrary) {
			t.Fatalf("initial scan ignored explicit library: got %s want %s", initial.Scope.Library, rememberedLibrary)
		}
		applyRememberedRootSelection(t, fx, initial, "initial")

		explicit := rememberedRootScan(t, fx, "explicit-app", "--app", "codex")
		assertRememberedRoots(t, "explicit app scan", explicit, rootR)
		if filepath.Clean(explicit.Scope.Library) != filepath.Clean(defaultLibrary) {
			t.Fatalf("explicit app scan adopted remembered library: got %s want default %s", explicit.Scope.Library, defaultLibrary)
		}
		applyRememberedRootSelection(t, fx, explicit, "explicit-app")
		persisted := rememberedRootScan(t, fx, "after-explicit-app-apply")
		if persisted.Scope.SelectionFrom != "remembered" {
			t.Errorf("post-apply scan did not use remembered selection: got %s", persisted.Scope.SelectionFrom)
		}
		if !reflect.DeepEqual(persisted.Scope.Applications, []string{"codex"}) {
			t.Errorf("applied explicit app was not remembered: got %v want [codex]", persisted.Scope.Applications)
		}
		assertRememberedRoots(t, "selection after explicit app apply", persisted, rootR)
		if filepath.Clean(persisted.Scope.Library) != filepath.Clean(defaultLibrary) {
			t.Errorf("applied explicit app remembered the wrong library: got %s want %s", persisted.Scope.Library, defaultLibrary)
		}
	})

	t.Run("explicit-root-unions-and-explicit-library-wins", func(t *testing.T) {
		fx, rootR, rootS, defaultLibrary := rememberedRootFixture(t)
		rememberedLibrary := filepath.Join(fx.base, "allowed", "remembered-library")
		initial := rememberedRootScan(t, fx, "union-initial", "--app", "pi", "--root", rootR, "--library", rememberedLibrary)
		applyRememberedRootSelection(t, fx, initial, "union-initial")

		explicit := rememberedRootScan(t, fx, "union-explicit", "--app", "codex", "--root", rootS, "--library", defaultLibrary)
		assertRememberedRoots(t, "explicit root union scan", explicit, rootR, rootS)
		if filepath.Clean(explicit.Scope.Library) != filepath.Clean(defaultLibrary) {
			t.Fatalf("explicit library did not override remembered library: got %s want %s", explicit.Scope.Library, defaultLibrary)
		}
		applyRememberedRootSelection(t, fx, explicit, "union-explicit")
		persisted := rememberedRootScan(t, fx, "union-after-apply")
		assertRememberedRoots(t, "selection after root union apply", persisted, rootR, rootS)
		if filepath.Clean(persisted.Scope.Library) != filepath.Clean(defaultLibrary) {
			t.Errorf("explicit library was not remembered: got %s want %s", persisted.Scope.Library, defaultLibrary)
		}
	})

	t.Run("clear-roots-keeps-explicit-root", func(t *testing.T) {
		fx, rootR, rootS, library := rememberedRootFixture(t)
		initial := rememberedRootScan(t, fx, "clear-initial", "--app", "pi", "--root", rootR, "--library", library)
		applyRememberedRootSelection(t, fx, initial, "clear-initial")

		cleared := rememberedRootScan(t, fx, "clear-explicit", "--app", "codex", "--root", rootS, "--clear-roots", "--library", library)
		assertRememberedRoots(t, "clear roots scan", cleared, rootS)
		applyRememberedRootSelection(t, fx, cleared, "clear-explicit")
		persisted := rememberedRootScan(t, fx, "clear-after-apply")
		if persisted.Scope.SelectionFrom != "remembered" {
			t.Errorf("post-clear apply scan did not use remembered selection: got %s", persisted.Scope.SelectionFrom)
		}
		if !reflect.DeepEqual(persisted.Scope.Applications, []string{"codex"}) {
			t.Errorf("clear-roots changed selected applications: got %v want [codex]", persisted.Scope.Applications)
		}
		assertRememberedRoots(t, "selection after clear-roots apply", persisted, rootS)
	})
}

func rememberedRootFixture(t *testing.T) (fixture, string, string, string) {
	t.Helper()
	fx := newFixture(t)
	rootR := filepath.Join(fx.base, "allowed", "roots", "remembered-R")
	rootS := filepath.Join(fx.base, "allowed", "roots", "explicit-S")
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	writeFile(t, filepath.Join(rootR, "remembered-r", "SKILL.md"), []byte("---\nname: remembered-r\ndescription: remembered root fixture\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(rootS, "explicit-s", "SKILL.md"), []byte("---\nname: explicit-s\ndescription: explicit root fixture\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: home-canary\ndescription: selection fixture\n---\nFixture only.\n"), 0o644)
	return fx, rootR, rootS, library
}

func rememberedRootScan(t *testing.T, fx fixture, evidence string, args ...string) collection.ConsolidationInventory {
	t.Helper()
	command := []string{"consolidate", "scan"}
	command = append(command, args...)
	command = append(command, "--json")
	result := fx.run(t, command...)
	if evidence != "" {
		writeEvidence(t, "remembered-roots/"+evidence+".json", result.stdout)
	}
	assertExit(t, result, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, result.stdout, &inventory)
	if inventory.Failure != nil || inventory.Scope.Library == "" {
		t.Fatalf("scan did not return a usable scope: %#v", inventory)
	}
	return inventory
}

func applyRememberedRootSelection(t *testing.T, fx fixture, inventory collection.ConsolidationInventory, label string) {
	t.Helper()
	draft := buildConsolidationDraft(inventory, nil)
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "remembered-roots-"+label+".json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "remembered-roots/"+label+"-preview.json", previewResult.stdout)
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" || preview.Confirmation == "" {
		t.Fatalf("%s selection preview was not ready: %#v", label, preview)
	}
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	writeEvidence(t, "remembered-roots/"+label+"-apply.json", apply.stdout)
	assertExit(t, apply, 0)
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &outcome)
	if outcome.State != "selection-updated" || !outcome.SelectionUpdated || !outcome.Verified {
		t.Fatalf("%s selection did not persist through public apply: %#v", label, outcome)
	}
}

func assertRememberedRoots(t *testing.T, label string, inventory collection.ConsolidationInventory, want ...string) {
	t.Helper()
	got := append([]string(nil), inventory.Scope.Roots...)
	expected := append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("%s roots mismatch: got %v want %v (selection_from=%s)", label, got, expected, inventory.Scope.SelectionFrom)
	}
}
