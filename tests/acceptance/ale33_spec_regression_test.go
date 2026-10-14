package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestALE33SpecManagedPackageConfiguredOverlap(t *testing.T) {
	fx := piFixture(t)
	pkg := filepath.Join(fx.base, "allowed", "package")
	codexSkill(t, filepath.Join(pkg, "skills"), "managed", "managed")
	writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"name":"fixture","version":"1.2.3","pi":{"skills":["skills"]}}`), 0600)
	raw, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "skills": []string{filepath.Join(pkg, "skills")}, "packages": []any{map[string]any{"source": pkg, "skills": []string{}}}})
	writeFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), raw, 0600)
	before := treeManifest(t, fx.base)
	source := codexNamed(t, piList(t, fx, 0), "managed")
	if source.Availability.State != "disabled" {
		t.Fatalf("recorded Pi 1.1.0 excludes this overlapping package resource: got %q want disabled", source.Availability.State)
	}
	if source.Ownership.State != "managed" || len(source.Availability.References) != 2 || source.Availability.References[0].Provenance.Kind != "plugin" {
		t.Fatalf("lost managed package provenance: %+v", source)
	}
	explanation := fx.run(t, "explain", "--agent", "pi", "--project", fx.project, "--json", source.ID)
	assertExit(t, explanation, 0)
	var explained struct{ Source codexSource }
	decodeJSON(t, explanation.stdout, &explained)
	if explained.Source.Availability.State != "disabled" {
		t.Fatalf("explanation resurrected package: %+v", explained)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE33SpecNativeNameThroughAlias(t *testing.T) {
	fx := piFixture(t)
	target := filepath.Join(fx.base, "allowed", "canonical-target")
	writeFile(t, filepath.Join(target, "SKILL.md"), []byte("---\ndescription: Isolated native fixture.\n---\nNo workload.\n"), 0600)
	alias := filepath.Join(fx.home, ".pi", "agent", "skills", "native-alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	list := piList(t, fx, 0)
	if len(list.Sources) != 1 {
		t.Fatalf("expected one source, got %d", len(list.Sources))
	}
	if list.Sources[0].Availability.NativeName != "native-alias" {
		t.Fatalf("recorded Pi 1.1.0 fallback is discovered alias: got native name %q want native-alias", list.Sources[0].Availability.NativeName)
	}
	if list.Sources[0].Availability.NativeNameEvidence.InputPath != filepath.Join(alias, "SKILL.md") {
		t.Fatalf("native name evidence is not lexical: %+v", list.Sources[0].Availability.NativeNameEvidence)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
