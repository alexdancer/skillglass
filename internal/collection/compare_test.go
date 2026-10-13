package collection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewComparisonUnavailableSourceMembership(t *testing.T) {
	for _, kind := range []string{"missing", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
			for _, p := range []string{a, b} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"SKILL.md", "resource"} {
					if err := os.WriteFile(filepath.Join(p, name), []byte("same bytes\n"), 0600); err != nil {
						t.Fatal(err)
					}
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
			for id, source := range snapshot.sources {
				if source.Directory == a {
					left = id
				}
				if source.Directory == b {
					right = id
				}
			}
			if kind == "missing" {
				if err := os.Rename(a, a+"-old"); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Chmod(a, 0111); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(a, 0700) })
				if f, err := os.Open(a); err == nil {
					f.Close()
					t.Fatal("unreadable source control opened directory")
				} else if !os.IsPermission(err) {
					t.Fatalf("permission control: %v", err)
				}
			}
			for _, pair := range [][2]SourceID{{left, right}, {right, left}} {
				report, err := snapshot.Compare(context.Background(), pair[0], pair[1])
				if err != nil {
					t.Fatal(err)
				}
				if report.Coverage != Incomplete || report.Outcome != OutcomeIndeterminate {
					t.Fatalf("unavailable source result: %+v", report)
				}
				found := false
				for _, change := range report.Changes {
					if change.Status != "unknown" || change.Kind != "unknown" || len(change.Diff) != 0 {
						t.Fatalf("unavailable source membership claimed: %+v", change)
					}
					found = found || change.Path == "resource"
				}
				if !found {
					t.Fatalf("missing unknown resource membership: %+v", report)
				}
			}
		})
	}
}

func TestComparisonDetectsDeterministicChanges(t *testing.T) {
	for _, kind := range []string{"rewrite", "rename", "add", "remove", "manifest-link-swap", "parent-link-substitution", "mid-read-rewrite", "mid-read-rename", "ancestor-type-replacement"} {
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
			if kind == "ancestor-type-replacement" {
				if err := os.WriteFile(filepath.Join(a, "scripts"), []byte("file ancestor\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(b, "scripts/nested"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(b, "scripts/nested/run.sh"), []byte("script contents\n"), 0600); err != nil {
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
				if kind == "ancestor-type-replacement" {
					wanted = "scripts"
				}
				if strings.HasPrefix(kind, "mid-read-") {
					wanted = "SKILL.md:read"
				}
				if done || side != "left" || p != wanted {
					return
				}
				done = true
				switch kind {
				case "ancestor-type-replacement":
					err = os.Remove(filepath.Join(a, "scripts"))
					if err == nil {
						err = os.Mkdir(filepath.Join(a, "scripts"), 0700)
					}
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
			if kind == "ancestor-type-replacement" {
				found := false
				for _, entry := range r.Changes {
					if entry.Path == "scripts/nested/run.sh" {
						found = true
						if entry.Status != "unknown" || entry.Kind != "unknown" || len(entry.Diff) != 0 {
							t.Fatalf("unstable ancestor proved absence: %+v", entry)
						}
					}
				}
				if !found {
					t.Fatalf("missing unknown child: %+v", r)
				}
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
