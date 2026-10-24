package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86ManagedCodexPluginCannotBeSharedOrArchived(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: home-canary\ndescription: personal control\n---\nFixture only.\n",
	), 0o644)
	pluginVersion := filepath.Join(fx.home, ".codex", "plugins", "cache", "fixture-market", "fixture-plugin", "1.0.0")
	writeFile(t, filepath.Join(pluginVersion, ".codex-plugin", "plugin.json"), []byte(
		`{"name":"fixture-plugin","version":"1.0.0","skills":"skills"}`,
	), 0o644)
	managedSource := filepath.Join(pluginVersion, "skills", "managed-canary")
	writeFile(t, filepath.Join(managedSource, "SKILL.md"), []byte(
		"---\nname: managed-canary\ndescription: manager-owned fixture\n---\nDo not move this plugin content.\n",
	), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	before := treeManifest(t, fx.base)
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "consolidation/managed-plugin-scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	var managedExclusion *collection.ConsolidationExclusion
	for index := range inventory.Exclusions {
		if filepath.Clean(inventory.Exclusions[index].Path) == filepath.Clean(managedSource) {
			managedExclusion = &inventory.Exclusions[index]
			break
		}
	}
	if managedExclusion == nil || managedExclusion.SourceID == "" || !strings.Contains(strings.ToLower(managedExclusion.Reason), "managed") {
		t.Fatalf("versioned Codex plugin source should be disclosed as a managed exclusion: %#v", inventory.Exclusions)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	// A forged choice must not promote a manager-owned source into a relocation.
	draft := edit1Draft(inventory, nil)
	draft.Choices = append(draft.Choices, collection.ConsolidationChoice{
		SourceID:   managedExclusion.SourceID,
		SourcePath: managedExclusion.Path,
		Action:     "archive",
	})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "managed-plugin-share.json")
	edit1WriteDraft(t, draftPath, draft)
	beforePreview := treeManifest(t, fx.base)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "consolidation/managed-plugin-preview.json", previewResult.stdout)
	assertExit(t, previewResult, 1)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "refused" || preview.Failure == nil {
		t.Fatalf("forged manager-owned share choice was not refused: %#v", preview)
	}
	assertTreeUnchanged(t, beforePreview, treeManifest(t, fx.base))
}
