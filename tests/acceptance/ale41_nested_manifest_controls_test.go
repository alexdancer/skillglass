package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func nestedManifestSource(t *testing.T, fx fixture, parent string) string {
	t.Helper()
	r := fx.run(t, "list", "--root", parent, "--json")
	assertExit(t, r, 0)
	var view collection.View
	decodeJSON(t, r.stdout, &view)
	for _, source := range view.Sources {
		if source.Directory == parent {
			return string(source.ID)
		}
	}
	t.Fatal("parent source missing", view.Sources)
	return ""
}

func TestALE41RootManifestLookup(t *testing.T) {
	for _, name := range []string{"SKILL.md", "skill.md", "sKiLl.Md", "external-symlink"} {
		t.Run(name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			manifest := codexSkill(t, filepath.Join(fx.base, "allowed", "custom"), "parent", "root-manifest")
			parent := filepath.Dir(manifest)
			external := filepath.Join(fx.base, "allowed", "external.md")
			input := readBytes(t, manifest)
			writeFile(t, external, input, 0600)
			if name == "external-symlink" {
				if err := os.Remove(manifest); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, manifest); err != nil {
					t.Fatal(err)
				}
			} else if name != "SKILL.md" {
				if err := os.Rename(manifest, filepath.Join(parent, name)); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(manifest); os.IsNotExist(err) {
					t.Skip("filesystem does not resolve this spelling as SKILL.md")
				} else if err != nil {
					t.Fatal(err)
				}
			}
			id := nestedManifestSource(t, fx, parent)
			args := []string{"delete", "--root", parent, "--project", fx.project, "--json"}
			r := changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var preview collection.DeleteView
			decodeJSON(t, r.stdout, &preview)
			if preview.State != "ready" {
				t.Fatal(preview.State, preview.Reason)
			}
			r = changeCommand(t, fx, "", append(append(args, "--confirm", preview.Confirmation), id)...)
			assertExit(t, r, 0)
			var outcome collection.DeleteOutcome
			decodeJSON(t, r.stdout, &outcome)
			if outcome.State != "removed" || !outcome.Verified {
				t.Fatal(outcome.State, outcome.Reason)
			}
			if _, err := os.Lstat(parent); !os.IsNotExist(err) {
				t.Fatal("parent survives", err)
			}
			if !bytes.Equal(readBytes(t, external), input) {
				t.Fatal("external manifest target changed")
			}
			writeEvidence(t, "root-manifest/"+name+".json", r.stdout)
		})
	}
}

func TestALE41NestedManifestEntryKinds(t *testing.T) {
	for _, kind := range []string{"external-symlink", "dangling-symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			parent := filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "custom"), "parent", "nested-entry"))
			nested := filepath.Join(parent, "nested", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(nested), 0700); err != nil {
				t.Fatal(err)
			}
			external := filepath.Join(fx.base, "allowed", "external.md")
			input := []byte("external target remains unchanged\n")
			writeFile(t, external, input, 0600)
			switch kind {
			case "external-symlink":
				if err := os.Symlink(external, nested); err != nil {
					t.Fatal(err)
				}
			case "dangling-symlink":
				if err := os.Symlink(external+"-absent", nested); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(nested, 0700); err != nil {
					t.Fatal(err)
				}
			}
			id := nestedManifestSource(t, fx, parent)
			before := treeManifest(t, fx.base)
			r := changeCommand(t, fx, "", "delete", "--root", parent, "--project", fx.project, "--json", id)
			assertExit(t, r, 1)
			var preview collection.DeleteView
			decodeJSON(t, r.stdout, &preview)
			if preview.State != "refused" || preview.Confirmation != "" {
				t.Fatal(preview.State, preview.Reason)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if !bytes.Equal(readBytes(t, external), input) {
				t.Fatal("external manifest target changed")
			}
			writeEvidence(t, "nested-entry/"+kind+".json", r.stdout)
		})
	}
}

func TestALE41NestedManifestStalesConsent(t *testing.T) {
	for _, mutation := range []string{"added-manifest", "renamed-manifest"} {
		t.Run(mutation, func(t *testing.T) {
			fx, _ := codexFixture(t)
			parent := filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "custom"), "parent", "nested-stale"))
			data := filepath.Join(parent, "nested", "data.txt")
			writeFile(t, data, []byte("resource before preview\n"), 0600)
			id := nestedManifestSource(t, fx, parent)
			args := []string{"delete", "--root", parent, "--project", fx.project, "--json"}
			r := changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var preview collection.DeleteView
			decodeJSON(t, r.stdout, &preview)
			if preview.State != "ready" {
				t.Fatal(preview.State, preview.Reason)
			}
			manifest := filepath.Join(filepath.Dir(data), "skill.md")
			if mutation == "added-manifest" {
				writeFile(t, manifest, []byte("nested manifest after preview\n"), 0600)
			} else if err := os.Rename(data, manifest); err != nil {
				t.Fatal(err)
			}
			before := treeManifest(t, fx.base)
			r = changeCommand(t, fx, "", append(append(args, "--confirm", preview.Confirmation), id)...)
			assertExit(t, r, 1)
			var outcome collection.DeleteOutcome
			decodeJSON(t, r.stdout, &outcome)
			if outcome.State != "stale" || len(outcome.Removed) != 0 {
				t.Fatal(outcome.State, outcome.Reason)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeEvidence(t, "nested-stale/"+mutation+".json", r.stdout)
		})
	}
}
