package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestALE33DecisionExcludedProjectAliasKeepsNativeDedup(t *testing.T) {
	fx := piFixture(t)
	shared := codexSkill(t, filepath.Join(fx.home, ".pi", "agent", "skills"), "shared", "shared")
	alias := filepath.Join(fx.project, ".pi", "skills", "alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(shared), alias); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"skills": []string{"-" + alias}})
	writeFile(t, filepath.Join(fx.project, ".pi", "settings.json"), raw, 0600)
	before := treeManifest(t, fx.base)
	list := piList(t, fx, 0)
	if len(list.Sources) != 1 {
		t.Fatalf("shared source merged incorrectly: %d", len(list.Sources))
	}
	if list.Sources[0].Availability.State != "disabled" {
		t.Fatalf("native retains excluded project occurrence: got %q want disabled", list.Sources[0].Availability.State)
	}
	if len(list.Sources[0].Availability.References) != 2 {
		t.Fatal("lost reference provenance")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE33DecisionExplicitManifestNameThroughAlias(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.base, "allowed"), "canonical-target", "explicit-name")
	alias := filepath.Join(fx.home, ".pi", "agent", "skills", "native-alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(target), alias); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	list := piList(t, fx, 0)
	if len(list.Sources) != 1 {
		t.Fatalf("source count: %d", len(list.Sources))
	}
	if list.Sources[0].Availability.NativeName != "explicit-name" {
		t.Fatalf("explicit name: got %q want explicit-name", list.Sources[0].Availability.NativeName)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE33DecisionAliasFallbackDoesNotShadowIndependentName(t *testing.T) {
	fx := piFixture(t)
	target := filepath.Join(fx.base, "allowed", "canonical-target")
	writeFile(t, filepath.Join(target, "SKILL.md"), []byte("---\ndescription: Inert native fixture.\n---\nNo workload.\n"), 0600)
	root := filepath.Join(fx.home, ".pi", "agent", "skills")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "native-alias")); err != nil {
		t.Fatal(err)
	}
	codexSkill(t, root, "independent", "canonical-target")
	before := treeManifest(t, fx.base)
	list := piList(t, fx, 0)
	if len(list.Sources) != 2 {
		t.Fatalf("independent copies merged: %d", len(list.Sources))
	}
	for _, source := range list.Sources {
		if source.Availability.State != "available" {
			t.Fatalf("native names differ and must not shadow: %s got %q want available", source.SourcePath, source.Availability.State)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
