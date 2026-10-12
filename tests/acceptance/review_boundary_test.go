package acceptance_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootManifestPresencePermissionBoundary(t *testing.T) {
	for _, mode := range []os.FileMode{0o444, 0, 0o755} {
		t.Run(mode.String(), func(t *testing.T) {
			fx := newFixture(t)
			parent := filepath.Join(fx.base, "allowed", "roots", "presence")
			bundle := filepath.Join(parent, "bundle")
			manifest := filepath.Join(bundle, "SKILL.md")
			writeFile(t, manifest, []byte("---\nname: presence\n---\nPRESENCE_INSTRUCTIONS\n"), 0o644)
			if err := os.Chmod(bundle, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(bundle, 0o755); err != nil {
					t.Errorf("restore bundle permissions: %v", err)
				}
				if _, err := os.ReadFile(manifest); err != nil {
					t.Errorf("restored manifest read: %v", err)
				}
			})
			if mode == 0o444 {
				entries, err := os.ReadDir(bundle)
				if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" || !entries[0].Type().IsRegular() {
					t.Fatalf("listing positive control: %v %#v", err, entries)
				}
				if _, err := os.Lstat(manifest); !errors.Is(err, fs.ErrPermission) {
					t.Fatalf("Lstat denial control: %v", err)
				}
				if _, err := os.ReadFile(manifest); !errors.Is(err, fs.ErrPermission) {
					t.Fatalf("read denial control: %v", err)
				}
			}
			for _, root := range []string{bundle, parent} {
				result := fx.run(t, "list", "--root", root, "--project", fx.project, "--json")
				var inventory publicList
				decodeJSON(t, result.stdout, &inventory)
				if mode == 0o755 {
					assertExit(t, result, 0)
					if len(inventory.Sources) != 1 || len(inventory.Problems) != 0 || inventory.Scope.Completeness != "complete" || inventory.Scope.Roots[0].State != "searched" {
						t.Fatalf("readable control: %s", result.stdout)
					}
					continue
				}
				assertExit(t, result, 1)
				state, code, problemPath := "incomplete", "unreadable-manifest", filepath.Join(canonicalPath(t, bundle), "SKILL.md")
				if mode == 0 && root == bundle {
					state, code, problemPath = "unreadable", "root-unreadable", bundle
				}
				if len(inventory.Sources) != 0 || inventory.Scope.Completeness != "incomplete" || len(inventory.Problems) != 1 ||
					inventory.Scope.Roots[0].State != state || inventory.Scope.Roots[0].SourceCount != 0 || inventory.Scope.Roots[0].ProblemCount != 1 {
					t.Fatalf("permission boundary: %s", result.stdout)
				}
				assertProblem(t, inventory.Problems, code, problemPath, true)
				if code == "unreadable-manifest" && inventory.Problems[0].Summary != "manifest presence could not be determined" {
					t.Fatalf("presence diagnostic: %#v", inventory.Problems)
				}
				if !strings.Contains(inventory.Problems[0].Detail, "permission denied") {
					t.Fatalf("permission detail: %#v", inventory.Problems)
				}
			}
		})
	}
}

func TestManifestRawEncodingPreservesExactBytes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   []byte
		valid bool
	}{
		{"invalid-instructions", []byte("---\nname: encoding\n---\nALTERED_BODY_\xff_SENTINEL\n"), false},
		{"invalid-frontmatter", []byte("---\nname: encoding\nextra: \xff\n---\nALTERED_BODY_SENTINEL\n"), false},
		{"invalid-raw-only", []byte("UNSPLIT_RAW_\xff_SENTINEL\n"), false},
		{"valid-utf8", []byte("---\nname: encoding\ndescription: café\n---\nVALID_BODY_😀_<tags>&\n"), true},
		{"literal-replacement", []byte("---\nname: encoding\n---\nLITERAL_�_BODY\n"), true},
		{"valid-empty-malformed", []byte{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			bundle := filepath.Join(fx.cleanRoot, "encoding")
			writeFile(t, filepath.Join(bundle, "SKILL.md"), tc.raw, 0o644)
			writeFile(t, filepath.Join(bundle, "resource.txt"), []byte("ENCODING_RESOURCE\n"), 0o644)
			before := treeManifest(t, fx.base)
			list := fx.run(t, "list", "--root", fx.cleanRoot, "--project", fx.project, "--json")
			var inventory publicList
			decodeJSON(t, list.stdout, &inventory)
			_ = sourceNamed(t, inventory.Sources, "normal-skill")
			_ = sourceNamed(t, inventory.Sources, "second-skill")
			var source publicSource
			for _, candidate := range inventory.Sources {
				if candidate.SourcePath == canonicalPath(t, bundle) {
					source = candidate
				}
			}
			if source.ID == "" || len(inventory.Sources) != 3 || len(source.Resources) != 1 || source.Identity.Certainty != "proven" {
				t.Fatalf("retained inventory: %s", list.stdout)
			}
			read := fx.run(t, "read", "--root", fx.cleanRoot, "--json", source.ID)
			var listData, readData map[string]any
			decodeJSON(t, list.stdout, &listData)
			decodeJSON(t, read.stdout, &readData)
			var listSource map[string]any
			for _, item := range listData["sources"].([]any) {
				candidate := item.(map[string]any)
				if candidate["id"] == source.ID {
					listSource = candidate
				}
			}
			readSource := readData["source"].(map[string]any)
			if len(readData["references"].([]any)) != 1 {
				t.Fatalf("read references missing: %s", read.stdout)
			}
			exit := 1
			if tc.valid && len(tc.raw) > 0 {
				exit = 0
			}
			for _, data := range []map[string]any{listSource, readSource} {
				manifest := data["manifest"].(map[string]any)
				if manifest["path"] != filepath.Join(canonicalPath(t, bundle), "SKILL.md") || manifest["resolved_path"] != manifest["path"] || len(data["reference_ids"].([]any)) != 1 {
					t.Fatalf("encoding provenance changed: %#v", data)
				}
				if tc.valid {
					if text, ok := manifest["raw_text"].(string); !ok || !bytes.Equal([]byte(text), tc.raw) {
						t.Fatalf("valid raw text altered: %#v", manifest)
					}
					if _, present := manifest["raw_bytes_base64"]; present {
						t.Fatalf("base64 changed valid UTF-8 schema: %#v", manifest)
					}
					continue
				}
				encoded, ok := manifest["raw_bytes_base64"].(string)
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if !ok || err != nil || !bytes.Equal(decoded, tc.raw) || encoded != base64.StdEncoding.EncodeToString(tc.raw) {
					t.Fatalf("exact byte representation: %#v", manifest)
				}
				if manifest["status"] != "malformed" {
					t.Fatalf("invalid encoding status: %#v", manifest)
				}
				for _, field := range []string{"raw_text", "raw_frontmatter", "instructions", "name", "description", "metadata"} {
					if _, present := manifest[field]; present {
						t.Fatalf("derived field %s present: %#v", field, manifest)
					}
				}
				for _, field := range []string{"name", "description"} {
					if _, present := data[field]; present {
						t.Fatalf("derived source field %s present", field)
					}
				}
			}
			assertExit(t, list, exit)
			assertExit(t, read, exit)
			if tc.name == "valid-utf8" && (!bytes.Contains(list.stdout, []byte("<tags>&")) || !bytes.Contains(read.stdout, []byte("<tags>&"))) {
				t.Fatal("valid UTF-8 JSON escaping changed")
			}
			if !tc.valid {
				if inventory.Scope.Completeness != "incomplete" || inventory.Scope.Roots[0].State != "incomplete" || inventory.Scope.Roots[0].ProblemCount != 1 {
					t.Fatalf("encoding scope: %#v", inventory.Scope)
				}
				assertProblem(t, inventory.Problems, "manifest-invalid-encoding", filepath.Join(canonicalPath(t, bundle), "SKILL.md"), true)
				if len(source.Problems) != 1 || source.Problems[0].Summary != "manifest is not valid UTF-8" || !strings.Contains(source.Problems[0].Detail, "list/read --json") || !strings.Contains(source.Problems[0].Detail, "manifest.raw_bytes_base64") {
					t.Fatalf("recovery diagnostic: %#v", source.Problems)
				}
				human := fx.run(t, "read", "--root", fx.cleanRoot, source.ID)
				assertExit(t, human, 1)
				if !bytes.Contains(human.stdout, []byte("manifest.raw_bytes_base64")) || bytes.Contains(human.stdout, []byte("SENTINEL")) || bytes.Contains(human.stdout, []byte("�")) || bytes.Contains(human.stdout, []byte("Raw manifest:")) || bytes.Contains(human.stdout, []byte(base64.StdEncoding.EncodeToString(tc.raw))) {
					t.Fatalf("altered human body or missing recovery: %s", human.stdout)
				}
			}
			if !bytes.Equal(before, treeManifest(t, fx.base)) {
				t.Fatal("encoding inspection mutated fixture")
			}
		})
	}
}
