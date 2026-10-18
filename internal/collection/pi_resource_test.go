package collection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiPackageResourceOwnershipRelations(t *testing.T) {
	for _, declaration := range []string{"skills", "skill*"} {
		for _, topology := range []string{"inside", "equal", "alias", "case-alias", "external", "missing-inside", "missing-outside", "cycle", "inaccessible", "not-directory"} {
			t.Run(declaration+"/"+topology, func(t *testing.T) {
				root := t.TempDir()
				pkg := filepath.Join(root, "PackageRoot")
				resources := filepath.Join(pkg, "resources")
				outside := filepath.Join(root, "outside")
				for _, dir := range []string{resources, outside} {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				path := filepath.Join(pkg, "skills")
				want := ManagerBoundary
				resourceDeclaration := declaration
				switch topology {
				case "inside":
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				case "equal":
					if err := os.Symlink(pkg, path); err != nil {
						t.Fatal(err)
					}
				case "alias":
					if err := os.Symlink(resources, path); err != nil {
						t.Fatal(err)
					}
				case "case-alias":
					alias := filepath.Join(root, "pACKAGErOOT")
					original, err := os.Stat(pkg)
					if err != nil {
						t.Fatal(err)
					}
					alternate, err := os.Stat(alias)
					if err == nil && os.SameFile(original, alternate) {
						if err := os.Symlink(filepath.Join(alias, "resources"), path); err != nil {
							t.Fatal(err)
						}
					} else {
						// On a case-sensitive filesystem this is a distinct object,
						// so matching its spelling must never transfer ownership.
						if err := os.MkdirAll(filepath.Join(alias, "resources"), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(filepath.Join(alias, "resources"), path); err != nil {
							t.Fatal(err)
						}
						want = ""
					}
				case "external":
					if err := os.Symlink(outside, path); err != nil {
						t.Fatal(err)
					}
					want = ""
				case "missing-inside":
					if declaration == "skill*" {
						want = ""
					}
				case "missing-outside":
					path = filepath.Join(outside, "skills")
					resourceDeclaration = path
					if declaration == "skill*" {
						resourceDeclaration += "*"
					}
					want = ""
				case "cycle":
					if err := os.Symlink(path, path); err != nil {
						t.Fatal(err)
					}
					want = ManagerUncertain
				case "inaccessible":
					if err := os.Chmod(resources, 0000); err != nil {
						t.Fatal(err)
					}
					defer os.Chmod(resources, 0700)
					if err := os.Symlink(filepath.Join(resources, "hidden"), path); err != nil {
						t.Fatal(err)
					}
					if _, err := os.Stat(path); !os.IsPermission(err) {
						t.Fatalf("expected actual permission failure: %v", err)
					}
					want = ManagerUncertain
				case "not-directory":
					file := filepath.Join(pkg, "file")
					if err := os.WriteFile(file, []byte("file"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(filepath.Join(file, "child"), path); err != nil {
						t.Fatal(err)
					}
					want = ManagerUncertain
				}
				manifest := `{"pi":{"skills":["` + resourceDeclaration + `"]}}`
				if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(manifest), 0600); err != nil {
					t.Fatal(err)
				}
				input := piInputs{facts: map[RootID]piRootFact{}, global: piSettings{packages: []piPackage{{Source: pkg}}}}
				request := Request{home: root}
				input.addPackages(&request, filepath.Join(root, "agent"))
				var got []ManagerEvidence
				for _, fact := range input.managers {
					if fact.Manager == pkg && fact.Reader == "" {
						got = append(got, fact)
					}
				}
				if want == "" {
					if len(got) != 0 {
						t.Fatalf("external resource acquired ownership: %+v", got)
					}
					return
				}
				if len(got) != 1 || got[0].State != want || got[0].Boundary != path {
					t.Fatalf("resource facts = %+v, want %s at %s", got, want, path)
				}
				if got[0].Evidence.InputPath != filepath.Join(root, "agent", "settings.json") {
					t.Fatal("registration provenance lost", got)
				}
				if want == ManagerUncertain && (got[0].Evidence.Kind != EvidenceUnknown || !strings.Contains(got[0].Evidence.Reason, pkg)) {
					t.Fatal("unresolved relationship lost uncertainty/provenance", got)
				}
			})
		}
	}
}
