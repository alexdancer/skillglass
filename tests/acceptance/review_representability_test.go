package acceptance_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataRepresentabilityPreservesPartialInventory(t *testing.T) {
	for _, tc := range []struct {
		name        string
		frontmatter string
		diagnostic  string
	}{
		{"binary-key-collision", "name: unsupported\nextra: {a: one, !!binary YQ==: two}\n", "mapping key"},
		{"alias-key-collision", "name: unsupported\nextra: {a: &same a, *same: two}\n", "mapping key"},
		{"top-alias-key-collision", "name: &same name\n*same: overwritten\n", "mapping key"},
		{"aliased-merge-tag-key-collision", "name: unsupported\nkey: &merge !!merge <<\nextra: {*merge: one, '<<': two}\n", "mapping key"},
		{"merged-key-collision", "name: unsupported\nextra: {<<: {a: one, !!binary YQ==: two}}\n", "mapping key"},
		{"sequence-key-collision", "name: unsupported\nextra: [{a: &same a, *same: two}]\n", "mapping key"},
		{"binary-key-utf8", "name: unsupported\nextra: {!!binary /w==: value}\n", "UTF-8"},
		{"binary-value-utf8", "name: unsupported\nextra: !!binary /w==\n", "UTF-8"},
		{"nested-binary-value-utf8", "name: unsupported\nextra: [{nested: [!!binary /w==]}]\n", "UTF-8"},
		{"aliased-binary-value-utf8", "name: unsupported\nextra: &value !!binary /w==\ncopy: [*value]\n", "UTF-8"},
		{"merged-overridden-binary-value-utf8", "name: unsupported\nextra: {<<: {a: !!binary /w==}, a: retained}\n", "UTF-8"},
		{"name-utf8", "name: !!binary /w==\n", "UTF-8"},
		{"description-utf8", "name: unsupported\ndescription: !!binary /w==\n", "UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			bundle := filepath.Join(fx.cleanRoot, "unsupported")
			instructions := "REPRESENTABILITY_INSTRUCTIONS\n"
			raw := "---\n" + tc.frontmatter + "---\n" + instructions
			writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte(raw), 0o644)
			before := treeManifest(t, fx.base)
			list := fx.run(t, "list", "--root", fx.cleanRoot, "--project", fx.project, "--json")
			var inventory publicList
			decodeJSON(t, list.stdout, &inventory)
			var source publicSource
			for _, candidate := range inventory.Sources {
				if candidate.SourcePath == canonicalPath(t, bundle) {
					source = candidate
				}
			}
			if source.ID == "" {
				t.Fatalf("unsupported source missing: %s", list.stdout)
			}
			read := fx.run(t, "read", "--root", fx.cleanRoot, "--json", source.ID)
			var detail publicRead
			decodeJSON(t, read.stdout, &detail)
			t.Logf("list exit=%d read exit=%d manifest=%s metadata=%#v", list.code, read.code, source.Manifest.Status, source.Manifest.Metadata)
			assertExit(t, list, 1)
			assertExit(t, read, 1)
			if len(inventory.Sources) != 3 || inventory.Scope.Completeness != "incomplete" || inventory.Scope.Roots[0].State != "incomplete" {
				t.Fatalf("partial inventory = %s", list.stdout)
			}
			_ = sourceNamed(t, inventory.Sources, "normal-skill")
			_ = sourceNamed(t, inventory.Sources, "second-skill")
			for _, retained := range []*publicSource{&source, detail.Source} {
				if retained == nil || retained.Manifest.Status != "malformed" || retained.Name != "" || retained.Description != "" ||
					retained.Manifest.RawText != raw || retained.Manifest.RawFrontmatter != tc.frontmatter ||
					retained.Manifest.Instructions != instructions || len(retained.Manifest.Metadata) != 0 {
					t.Fatalf("unsupported metadata misrepresented: %#v", retained)
				}
				manifestPath := filepath.Join(canonicalPath(t, bundle), "SKILL.md")
				assertProblem(t, retained.Problems, "malformed-manifest", manifestPath, true)
				if len(retained.Problems) != 1 || !strings.Contains(retained.Problems[0].Detail, tc.diagnostic) {
					t.Fatalf("unsupported metadata diagnostic = %#v", retained.Problems)
				}
			}
			human := fx.run(t, "read", "--root", fx.cleanRoot, source.ID)
			assertExit(t, human, 1)
			for _, line := range strings.Split(strings.TrimSuffix(tc.frontmatter, "\n"), "\n") {
				if !bytes.Contains(human.stdout, []byte(line)) {
					t.Fatalf("human read lost %q: %s", line, human.stdout)
				}
			}
			if !bytes.Equal(before, treeManifest(t, fx.base)) {
				t.Fatal("representability inspection mutated fixture")
			}
		})
	}
}

func TestMetadataRepresentabilityKeepsValidStringsAndMergeOverrides(t *testing.T) {
	fx := newFixture(t)
	bundle := filepath.Join(fx.cleanRoot, "representable")
	frontmatter := "name: representable\ndescription: valid metadata\n" +
		"key: &key alias-key\naliased: {*key: retained}\n" +
		"merge-key: &merge !!merge <<\nliteral: {*merge: retained}\n" +
		"binary: {!!binary YQ==: !!binary 8J+YgA==}\n" +
		"replacement: '\ufffd'\n" +
		"defaults: &defaults {a: one, b: first}\n" +
		"other: &other {a: two, c: second}\n" +
		"merged: {<<: *defaults, a: explicit}\n" +
		"sequence: [{<<: [*defaults, *other], b: override}]\n" +
		"inline: {<<: {a: one}, !!binary YQ==: override}\n"
	raw := "---\n" + frontmatter + "---\nVALID_REPRESENTABILITY_INSTRUCTIONS\n"
	writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte(raw), 0o644)
	before := treeManifest(t, fx.base)
	list := fx.run(t, "list", "--root", fx.cleanRoot, "--json")
	assertExit(t, list, 0)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	source := sourceNamed(t, inventory.Sources, "representable")
	read := fx.run(t, "read", "--root", fx.cleanRoot, "--json", source.ID)
	assertExit(t, read, 0)
	var detail publicRead
	decodeJSON(t, read.stdout, &detail)
	for _, retained := range []*publicSource{&source, detail.Source} {
		if retained == nil || retained.Manifest.Status != "parsed" || retained.Manifest.RawText != raw || len(retained.Problems) != 0 {
			t.Fatalf("valid metadata misrepresented: %#v", retained)
		}
		metadata := retained.Manifest.Metadata
		for field, expected := range map[string]map[string]string{
			"aliased": {"alias-key": "retained"},
			"literal": {"<<": "retained"},
			"binary":  {"a": "😀"},
			"merged":  {"a": "explicit", "b": "first"},
			"inline":  {"a": "override"},
		} {
			mapping, ok := metadata[field].(map[string]any)
			if !ok || len(mapping) != len(expected) {
				t.Fatalf("%s = %#v", field, metadata[field])
			}
			for key, value := range expected {
				if mapping[key] != value {
					t.Fatalf("%s.%s = %#v, want %q", field, key, mapping[key], value)
				}
			}
		}
		sequence, ok := metadata["sequence"].([]any)
		if !ok || len(sequence) != 1 {
			t.Fatalf("merged sequence = %#v", metadata["sequence"])
		}
		mapping, ok := sequence[0].(map[string]any)
		if !ok || len(mapping) != 3 || mapping["a"] != "one" || mapping["b"] != "override" || mapping["c"] != "second" {
			t.Fatalf("merged sequence values = %#v", sequence[0])
		}
		if metadata["replacement"] != "\ufffd" {
			t.Fatalf("literal Unicode replacement character lost: %#v", metadata["replacement"])
		}
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("valid metadata inspection mutated fixture")
	}
}
