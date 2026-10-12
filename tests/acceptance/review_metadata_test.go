package acceptance_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedMetadataPreservesPartialInventory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata string
	}{
		{"integer-key", "extra: {1: one}\n"},
		{"boolean-key", "extra: {true: yes}\n"},
		{"sequence-map", "extra: [{nested: {false: value}}]\n"},
		{"mixed-keys", "extra: {1: one, '1': string-one}\n"},
		{"general-map-nan", "extra: {1: .nan, value: .inf}\n"},
		{"general-map-sequence", "extra: {false: [.nan, -.inf, {value: .inf}]}\n"},
		{"aliased-map", "extra: &values {true: .nan}\ncopy: *values\n"},
		{"merged-integer-key", "<<: {1: one}\n"},
		{"nested-merge-sequence", "extra: [{<<: [{false: .nan}, {value: .inf}]}]\n"},
		{"top-integer-key", "1: one\n"},
		{"top-boolean-key", "true: yes\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			bundle := filepath.Join(fx.cleanRoot, "unsupported")
			frontmatter := "name: unsupported\ndescription: retained raw metadata\n" + tc.metadata
			instructions := "UNSUPPORTED_METADATA_INSTRUCTIONS\n"
			raw := "---\n" + frontmatter + "---\n" + instructions
			writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte(raw), 0o644)
			before := treeManifest(t, fx.base)
			list := fx.run(t, "list", "--root", fx.cleanRoot, "--project", fx.project, "--json")
			assertExit(t, list, 1)
			var inventory publicList
			decodeJSON(t, list.stdout, &inventory)
			if len(inventory.Sources) != 3 || inventory.Scope.Completeness != "incomplete" {
				t.Fatalf("partial inventory = %s", list.stdout)
			}
			_ = sourceNamed(t, inventory.Sources, "normal-skill")
			_ = sourceNamed(t, inventory.Sources, "second-skill")
			var source publicSource
			for _, candidate := range inventory.Sources {
				if candidate.SourcePath == canonicalPath(t, bundle) {
					source = candidate
				}
			}
			if source.ID == "" || source.Manifest.Status != "malformed" ||
				source.Manifest.RawText != raw || source.Manifest.RawFrontmatter != frontmatter ||
				source.Manifest.Instructions != instructions || len(source.Manifest.Metadata) != 0 {
				t.Fatalf("unsupported metadata was misrepresented: %#v", source)
			}
			manifestPath := filepath.Join(canonicalPath(t, bundle), "SKILL.md")
			assertProblem(t, source.Problems, "malformed-manifest", manifestPath, true)
			assertProblem(t, inventory.Problems, "malformed-manifest", manifestPath, true)
			if len(source.Problems) != 1 || !strings.Contains(source.Problems[0].Detail, "mapping key") {
				t.Fatalf("unsupported key diagnostic = %#v", source.Problems)
			}
			read := fx.run(t, "read", "--root", fx.cleanRoot, "--json", source.ID)
			assertExit(t, read, 1)
			var detail publicRead
			decodeJSON(t, read.stdout, &detail)
			if detail.Source == nil || detail.Source.Manifest.RawText != raw {
				t.Fatalf("raw read = %s", read.stdout)
			}
			human := fx.run(t, "list", "--root", fx.cleanRoot)
			assertExit(t, human, 1)
			if bytes.Contains(human.stdout, []byte("{unavailable}")) ||
				!bytes.Contains(human.stdout, []byte("Coverage: incomplete")) ||
				!bytes.Contains(human.stdout, []byte("malformed-manifest")) {
				t.Fatalf("human partial inventory = %s", human.stdout)
			}
			humanRead := fx.run(t, "read", "--root", fx.cleanRoot, source.ID)
			assertExit(t, humanRead, 1)
			for _, line := range strings.Split(strings.TrimSuffix(tc.metadata, "\n"), "\n") {
				if !bytes.Contains(humanRead.stdout, []byte(line)) {
					t.Fatalf("human raw read lost %q: %s", line, humanRead.stdout)
				}
			}
			if !bytes.Contains(humanRead.stdout, []byte(instructions)) {
				t.Fatalf("human raw read = %s", humanRead.stdout)
			}
			if !bytes.Equal(before, treeManifest(t, fx.base)) {
				t.Fatal("metadata inspection mutated fixture")
			}
		})
	}
}

func TestStringKeyedMetadataKeepsNestedValues(t *testing.T) {
	fx := newFixture(t)
	bundle := filepath.Join(fx.cleanRoot, "structured")
	raw := "---\nname: structured\n" +
		"extra: &values {'1': one, 'true': [.nan, .inf, -.inf, {value: 7}], empty: null, flag: true}\n" +
		"copy: *values\nmerged: {<<: *values, additional: retained}\ncreated: 2026-10-12\n---\nSTRUCTURED_INSTRUCTIONS\n"
	writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte(raw), 0o644)
	list := fx.run(t, "list", "--root", fx.cleanRoot, "--json")
	assertExit(t, list, 0)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	source := sourceNamed(t, inventory.Sources, "structured")
	if source.Manifest.RawText != raw || source.Manifest.Metadata["created"] != "2026-10-12T00:00:00Z" {
		t.Fatalf("structured manifest = %#v", source.Manifest)
	}
	for _, field := range []string{"extra", "copy", "merged"} {
		mapping, ok := source.Manifest.Metadata[field].(map[string]any)
		count := 4
		if field == "merged" {
			count++
		}
		if field == "merged" && mapping["additional"] != "retained" {
			t.Fatalf("merged value = %#v", mapping["additional"])
		}
		if !ok || mapping["1"] != "one" || mapping["flag"] != true || mapping["empty"] != nil || len(mapping) != count {
			t.Fatalf("structured mapping %s = %#v", field, mapping)
		}
		sequence, ok := mapping["true"].([]any)
		if !ok || len(sequence) != 4 {
			t.Fatalf("structured sequence = %#v", mapping["true"])
		}
		for index, label := range []string{"nan", "positive-infinity", "negative-infinity"} {
			assertNonFiniteTag(t, sequence[index], label)
		}
		if sequence[3].(map[string]any)["value"] != float64(7) {
			t.Fatalf("nested finite value = %#v", sequence[3])
		}
	}
	read := fx.run(t, "read", "--root", fx.cleanRoot, "--json", source.ID)
	assertExit(t, read, 0)
	var detail publicRead
	decodeJSON(t, read.stdout, &detail)
	if detail.Source == nil || detail.Source.Manifest.RawText != raw {
		t.Fatalf("structured read = %s", read.stdout)
	}
}

func TestOverlappingRootsUseCachedSourceProblems(t *testing.T) {
	for _, kind := range []string{"malformed", "unreadable-resources", "informational"} {
		for _, bundleFirst := range []bool{false, true} {
			order := "collection-first"
			if bundleFirst {
				order = "bundle-first"
			}
			t.Run(kind+"/"+order, func(t *testing.T) {
				fx := newFixture(t)
				root := filepath.Join(fx.base, "allowed", "roots", "overlapping")
				bundle := filepath.Join(root, "bundle")
				raw := "---\nname: overlap\n---\nOVERLAP_INSTRUCTIONS\n"
				code := "malformed-manifest"
				state := "incomplete"
				exit := 1
				switch kind {
				case "malformed":
					raw = "---\nname: [invalid\n---\nOVERLAP_INSTRUCTIONS\n"
				case "informational":
					raw = "---\nname: overlap\nextra: [{value: .nan}, {value: .inf}, {value: -.inf}]\n---\nOVERLAP_INSTRUCTIONS\n"
					code, state, exit = "metadata-non-finite-float", "searched", 0
				case "unreadable-resources":
					code = "unreadable-resource"
				}
				writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte(raw), 0o644)
				if err := os.Symlink(bundle, filepath.Join(root, "bundle-alias")); err != nil {
					t.Fatal(err)
				}
				if kind == "unreadable-resources" {
					if err := os.Chmod(bundle, 0o111); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Chmod(bundle, 0o755) })
					if _, err := os.ReadDir(bundle); !errors.Is(err, fs.ErrPermission) {
						t.Fatalf("permission denial control = %v", err)
					}
				}
				roots := []string{root, bundle}
				if bundleFirst {
					roots[0], roots[1] = roots[1], roots[0]
				}
				args := []string{"list", "--root", roots[0], "--root", roots[1], "--project", fx.project, "--json"}
				result := fx.run(t, args...)
				assertExit(t, result, exit)
				var inventory publicList
				decodeJSON(t, result.stdout, &inventory)
				if len(inventory.Sources) != 1 || len(inventory.Problems) != 1 {
					t.Fatalf("deduplicated inventory = %s", result.stdout)
				}
				if len(inventory.Scope.Roots) != 2 {
					t.Fatalf("roots = %#v", inventory.Scope.Roots)
				}
				for index, report := range inventory.Scope.Roots {
					count := 1
					if roots[index] == root {
						count = 2
					}
					if report.Requested != roots[index] || report.State != state ||
						report.SourceCount != count || report.ProblemCount != 1 {
						t.Fatalf("root %d lost cached diagnostics: %#v", index, report)
					}
				}
				problemPath := filepath.Join(canonicalPath(t, bundle), "SKILL.md")
				if kind == "unreadable-resources" {
					problemPath = canonicalPath(t, bundle)
				}
				assertProblem(t, inventory.Problems, code, problemPath, exit == 1)
				if kind == "informational" {
					values := inventory.Sources[0].Manifest.Metadata["extra"].([]any)
					for index, label := range []string{"nan", "positive-infinity", "negative-infinity"} {
						assertNonFiniteTag(t, values[index].(map[string]any)["value"], label)
					}
				}
				human := fx.run(t, args[:len(args)-1]...)
				assertExit(t, human, exit)
				if strings.Count(string(human.stdout), "["+state+"]") != 2 {
					t.Fatalf("human overlapping scope = %s", human.stdout)
				}
			})
		}
	}
}
