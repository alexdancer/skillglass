package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestRenewedALE29InvalidPluginManifestCannotEstablishAvailability(t *testing.T) {
	fx, _ := codexFixture(t)
	path := claudeRootPluginFixture(t, fx, nil, false)
	plugin := filepath.Dir(path)
	control := claudeAt(t, claudeListResult(t, fx, 0), plugin)
	if control.Availability.State != "available" {
		t.Fatalf("valid activation control: %+v", control.Availability)
	}
	claudeSettings(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"), map[string]any{
		"name": "demo", "version": "1.0.0", "homepage": "not-an-absolute-url",
	})
	before := treeManifest(t, fx.base)
	result := fx.run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
	writeEvidence(t, "renewed-spec/invalid-homepage-list.json", result.stdout)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	source := claudeAt(t, list, plugin)
	detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
	writeEvidence(t, "renewed-spec/invalid-homepage-explain.txt", detail.stdout)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if source.Availability.State == "available" {
		t.Fatalf("loader-rejected plugin manifest cannot establish available: %+v", source.Availability)
	}
}
