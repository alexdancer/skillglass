package collection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComparisonDetectsDeterministicChanges(t *testing.T) {
	for _, kind := range []string{"rewrite", "rename", "add", "remove", "manifest-link-swap", "parent-link-substitution", "mid-read-rewrite", "mid-read-rename"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			root, _ = filepath.EvalSymlinks(root)
			a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
			for _, p := range []string{a, b} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("---\nname: fixture\n---\nbody\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(p, "resource"), []byte("aaaa"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "manifest-link-swap" {
				for _, name := range []string{"target1", "target2"} {
					if err := os.WriteFile(filepath.Join(root, name), []byte("equal manifest"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Remove(filepath.Join(a, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "target1"), filepath.Join(a, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}
			req, err := NewRequest([]string{root}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := Build(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			var left, right SourceID
			for id, s := range snapshot.sources {
				if s.Directory == a {
					left = id
				}
				if s.Directory == b {
					right = id
				}
			}
			done := false
			observer := func(side, p string) {
				wanted := "SKILL.md"
				if strings.HasPrefix(kind, "mid-read-") {
					wanted = "SKILL.md:read"
				}
				if done || side != "left" || p != wanted {
					return
				}
				done = true
				switch kind {
				case "rewrite", "mid-read-rewrite":
					err = os.WriteFile(filepath.Join(a, "SKILL.md"), []byte("---\nname: fixture\n---\nxxxx\n"), 0600)
				case "rename", "mid-read-rename":
					err = os.Rename(filepath.Join(a, "resource"), filepath.Join(a, "renamed"))
				case "add":
					err = os.WriteFile(filepath.Join(a, "new"), []byte("new"), 0600)
				case "remove":
					err = os.Remove(filepath.Join(a, "resource"))
				case "manifest-link-swap":
					err = os.Remove(filepath.Join(a, "SKILL.md"))
					if err == nil {
						err = os.Symlink(filepath.Join(root, "target2"), filepath.Join(a, "SKILL.md"))
					}
				case "parent-link-substitution":
					err = os.Rename(a, a+"-old")
					if err == nil {
						err = os.Symlink(b, a)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			r, err := snapshot.compare(context.Background(), left, right, observer)
			if err != nil {
				t.Fatal(err)
			}
			if !done || r.Coverage != Incomplete || r.Outcome != "indeterminate" {
				t.Fatalf("race result: %+v", r)
			}
			change := false
			for _, issue := range r.Issues {
				if strings.Contains(issue.Reason, "changed") || strings.Contains(issue.Reason, "identity") {
					change = true
				}
			}
			if !change {
				t.Fatalf("missing changing-source reason: %+v", r.Issues)
			}
		})
	}
}
