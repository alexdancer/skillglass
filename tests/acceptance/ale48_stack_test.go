package acceptance_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/alexdancer/skillglass/internal/companion"
)

// This is a public-executable contract journey. Actual installed-agent judgment
// and skill interpretation remain separate from deterministic CLI evidence.
func TestALE48InstalledCompanionApprovedSubsetApply(t *testing.T) {
	fx := ale48Fixture(t)
	root := filepath.Join(fx.home, ".codex/skills")
	codexSkill(t, root, "home-canary", "home-native")
	installed := ale48Setup(t, fx, true, 0, "codex", "opencode", "codeaf")
	for _, result := range installed.Results {
		if result.State != "installed" {
			t.Fatal(result)
		}
		ale48AssertBundle(t, result.Destination)
	}
	check := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--check", "--json")
	assertExit(t, check, 0)
	if !bytes.Contains(check.stdout, []byte(`"compatible"`)) {
		t.Fatalf("startup contract failed %s", check.stdout)
	}
	emitted := fx.run(t, "companion", "--json")
	assertExit(t, emitted, 0)
	var bundle companion.Bundle
	decodeJSON(t, emitted.stdout, &bundle)
	if !bytes.Contains([]byte(bundle.Files[0].Content), []byte("review apply")) || !bytes.Contains([]byte(bundle.Files[0].Content), []byte("setup --check")) {
		t.Fatal("combined companion lost setup or apply instructions")
	}
	first := codexSkill(t, root, "selected-source", "selected-source")
	second := codexSkill(t, root, "omitted-source", "omitted-source")
	writeFile(t, filepath.Join(filepath.Dir(first), "references/guide.md"), []byte("Supporting evidence for the selected source.\n"), 0600)
	inventory := ale48GlobalInventory(t, fx)
	ids := map[string]string{}
	for _, source := range inventory.Sources {
		ids[source.Directory] = string(source.ID)
	}
	firstID, secondID := ids[filepath.Dir(first)], ids[filepath.Dir(second)]
	if firstID == "" || secondID == "" {
		t.Fatal("global source identity missing", ids)
	}
	draft := ale46Draft([]string{"codex"}, collection.ControlGlobal, "")
	draft.Sources = []collection.ReviewAssessment{ale46Assess(firstID, "references/guide.md"), ale46Assess(secondID)}
	draft.Changes = []collection.ReviewRecommendation{ale46Change("selected", firstID, "codex", collection.ControlDisable), ale46Change("omitted", secondID, "codex", collection.ControlDisable)}
	selection, path := ale47Approved(t, fx, draft, []string{"selected"})
	before := treeManifest(t, root)
	out := ale47Apply(t, fx, path, "", 0)
	if out.State != "applied" || out.SelectionID != selection.SelectionID || len(out.Results) != 1 || out.Results[0].State != "succeeded" || len(out.Omitted) != 1 || out.Omitted[0] != "omitted" {
		t.Fatalf("unexpected subset outcome %+v", out)
	}
	if out.Results[0].Final == nil || out.Results[0].Final.State != collection.AvailabilityDisabled {
		t.Fatal("selected source not disabled", out)
	}
	view := ale48GlobalInventory(t, fx)
	observed := map[string]collection.AvailabilityState{}
	for _, source := range view.Sources {
		observed[string(source.ID)] = source.Availability.State
	}
	if observed[firstID] != collection.AvailabilityDisabled || observed[secondID] != collection.AvailabilityAvailable {
		t.Fatal("selected or omitted result incorrect", observed)
	}
	assertTreeUnchanged(t, before, treeManifest(t, root))
	for _, result := range installed.Results {
		ale48AssertBundle(t, result.Destination)
	}
	if !bytes.Contains(readBytes(t, filepath.Join(fx.home, ".codex/config.toml")), []byte("# unrelated setting")) {
		t.Fatal("unrelated native settings lost")
	}
	stable := treeManifest(t, fx.base)
	again := ale48Setup(t, fx, true, 0, "codex", "opencode", "codeaf")
	for _, result := range again.Results {
		if result.State != "already-present" {
			t.Fatal(result)
		}
	}
	assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
	writeEvidence(t, "ale48-stack/skills-before.json", before)
	writeEvidence(t, "ale48-stack/skills-after.json", treeManifest(t, root))
}

func ale48GlobalInventory(t *testing.T, fx fixture) collection.View {
	t.Helper()
	result := fx.run(t, "list", "--agent", "codex", "--json")
	assertExit(t, result, 0)
	var view collection.View
	decodeJSON(t, result.stdout, &view)
	return view
}
