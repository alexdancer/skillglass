//go:build darwin

package collection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteBoundaryRelations(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	other := filepath.Join(root, "other")
	private := filepath.Join(root, "private")
	for _, path := range []string{child, other, private} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "alias")
	hidden := filepath.Join(private, "alias")
	loop := filepath.Join(root, "loop")
	for path, target := range map[string]string{alias: parent, hidden: parent, loop: loop} {
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(private, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(private, 0700)
	if _, err := os.Stat(hidden); !os.IsPermission(err) {
		t.Fatalf("expected actual EACCES: %v", err)
	}
	for _, tc := range []struct {
		name, a, b string
		want       [4]deleteRelation
	}{
		{"equal", parent, parent, [4]deleteRelation{deleteRelated, deleteRelated, deleteRelated, deleteRelated}},
		{"descendant", parent, child, [4]deleteRelation{deleteRelated, deleteUnrelated, deleteUnrelated, deleteRelated}},
		{"alias", alias, child, [4]deleteRelation{deleteRelated, deleteUnrelated, deleteUnrelated, deleteRelated}},
		{"alias-equality", alias, parent, [4]deleteRelation{deleteRelated, deleteRelated, deleteRelated, deleteRelated}},
		{"unrelated", parent, other, [4]deleteRelation{deleteUnrelated, deleteUnrelated, deleteUnrelated, deleteUnrelated}},
		{"missing-unrelated", child, filepath.Join(other, "missing", "installation"), [4]deleteRelation{deleteUnrelated, deleteUnrelated, deleteUnrelated, deleteUnrelated}},
		{"missing-descendant", parent, filepath.Join(child, "missing"), [4]deleteRelation{deleteRelated, deleteUnrelated, deleteUnrelated, deleteRelated}},
		{"inaccessible-alias", hidden, child, [4]deleteRelation{deleteUnresolved, deleteUnresolved, deleteUnresolved, deleteUnresolved}},
		{"cycle", loop, child, [4]deleteRelation{deleteUnresolved, deleteUnresolved, deleteUnresolved, deleteUnresolved}},
		{"not-directory", filepath.Join(file, "child"), child, [4]deleteRelation{deleteUnresolved, deleteUnresolved, deleteUnresolved, deleteUnresolved}},
		{"empty", "", child, [4]deleteRelation{deleteUnrelated, deleteUnrelated, deleteUnrelated, deleteUnrelated}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := [4]deleteRelation{deleteBoundaryContains(tc.a, tc.b), deleteBoundaryContains(tc.b, tc.a), deleteBoundaryEqual(tc.a, tc.b), deletePathsOverlap(tc.a, tc.b)}
			if got != tc.want {
				t.Fatalf("contains/reverse/equal/overlap = %v, want %v", got, tc.want)
			}
			if deletePathsOverlap(tc.b, tc.a) != got[3] || deleteBoundaryEqual(tc.b, tc.a) != got[2] {
				t.Fatal("overlap and equality must be symmetric")
			}
		})
	}
	caseAlias := filepath.Join(filepath.Dir(parent), strings.ToUpper(filepath.Base(parent)))
	if info, err := os.Stat(caseAlias); err == nil {
		original, err := os.Stat(parent)
		if err != nil || !os.SameFile(info, original) || deleteBoundaryContains(caseAlias, child) != deleteRelated || deleteBoundaryEqual(caseAlias, parent) != deleteRelated {
			t.Fatal("case alias lost object ancestry")
		}
	}
}
