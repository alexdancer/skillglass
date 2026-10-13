package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewComparisonRequiresRegularManifest(t *testing.T) {
	for _, kind := range []string{"directory", "linked-directory", "linked-file"} {
		t.Run(kind, func(t *testing.T) {
			linked := kind != "directory"
			regular := kind == "linked-file"
			fx, root, a, b := compareFixture(t)
			for _, p := range []string{a, b} {
				manifest := filepath.Join(p, "SKILL.md")
				if err := os.Remove(manifest); err != nil {
					t.Fatal(err)
				}
				target := manifest
				if linked {
					target = filepath.Join(p, "instructions")
				}
				if regular {
					writeFile(t, target, []byte("readable instructions\n"), 0600)
				} else if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if linked {
					if err := os.Symlink("instructions", manifest); err != nil {
						t.Fatal(err)
					}
				}
			}
			left, right := compareIDs(t, fx, root)
			before := treeManifest(t, fx.base)
			r := fx.run(t, "compare", "--root", root, "--json", left, right)
			c := compareReport(t, r)
			if regular {
				assertExit(t, r, 0)
				if c.Report.Coverage != "complete" || c.Report.Outcome != "identical" || len(c.Report.Issues) != 0 {
					t.Fatalf("readable linked manifest rejected: %s", r.stdout)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			assertExit(t, r, 1)
			if c.Report.Coverage != "incomplete" || c.Report.Outcome != "indeterminate" {
				t.Fatalf("unreadable manifest accepted: %s", r.stdout)
			}
			for _, side := range []string{"left", "right"} {
				found := false
				for _, issue := range c.Report.Issues {
					found = found || (issue.Side == side && issue.Path == "SKILL.md")
				}
				if !found {
					t.Fatalf("missing manifest issue for %s: %s", side, r.stdout)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestReviewComparisonUnobservedMembership(t *testing.T) {
	for _, cause := range []string{"unreadable-directory", "entry-limit"} {
		t.Run(cause, func(t *testing.T) {
			fx, root, a, b := compareFixture(t)
			path := "scripts/run.sh"
			if cause == "unreadable-directory" {
				for _, p := range []string{a, b} {
					writeFile(t, filepath.Join(p, path), []byte("same script\n"), 0600)
					writeFile(t, filepath.Join(p, "scripts/nested/run.sh"), []byte("same nested script\n"), 0600)
					writeFile(t, filepath.Join(p, "visible.txt"), []byte("same\n"), 0600)
				}
				writeFile(t, filepath.Join(b, "proven-added.txt"), []byte("added\n"), 0600)
				writeFile(t, filepath.Join(b, "new-directory/nested/added.txt"), []byte("nested added\n"), 0600)
				dir := filepath.Join(a, "scripts")
				if err := os.Chmod(dir, 0000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
				if f, err := os.Open(dir); err == nil {
					f.Close()
					t.Fatal("unreadable-directory control opened directory")
				} else if !os.IsPermission(err) {
					t.Fatalf("permission control: %v", err)
				}
			} else {
				path = "unproven.txt"
				for i := 0; i < 10001; i++ {
					writeFile(t, filepath.Join(a, fmt.Sprintf("entry-%05d", i)), nil, 0600)
				}
				writeFile(t, filepath.Join(b, path), []byte("unproven membership\n"), 0600)
			}
			list := fx.run(t, "list", "--root", root, "--json")
			var inventory publicList
			decodeJSON(t, list.stdout, &inventory)
			var left, right string
			for _, source := range inventory.Sources {
				if source.SourcePath == canonicalPath(t, a) {
					left = source.ID
				}
				if source.SourcePath == canonicalPath(t, b) {
					right = source.ID
				}
			}
			if left == "" || right == "" {
				t.Fatalf("missing selected copies: %s", list.stdout)
			}
			for _, pair := range [][2]string{{left, right}, {right, left}} {
				r := fx.run(t, "compare", "--root", root, "--json", pair[0], pair[1])
				assertExit(t, r, 1)
				c := compareReport(t, r)
				found, proven, nested, ancestor := false, false, false, false
				for _, change := range c.Report.Changes {
					if change.Path == path {
						found = true
						if change.Status != "unknown" || change.Kind != "unknown" || len(change.Diff) != 0 {
							t.Fatalf("unobserved membership claimed: %+v", change)
						}
					}
					if change.Path == "scripts/nested/run.sh" {
						nested = true
						if change.Status != "unknown" || change.Kind != "unknown" || len(change.Diff) != 0 {
							t.Fatalf("unobserved nested membership claimed: %+v", change)
						}
					}
					if change.Path == "visible.txt" {
						t.Fatalf("equal visible file changed: %+v", change)
					}
					if change.Path == "proven-added.txt" || change.Path == "new-directory/nested/added.txt" {
						if change.Path == "proven-added.txt" {
							proven = true
						} else {
							ancestor = true
						}
						status := "added"
						if pair[0] == right {
							status = "removed"
						}
						if change.Status != status || change.Kind != "text" || len(change.Diff) == 0 {
							t.Fatalf("proven absence lost: %+v", change)
						}
					}
				}
				if !found || (cause == "unreadable-directory" && (!proven || !nested || !ancestor)) || c.Report.Coverage != "incomplete" {
					t.Fatalf("membership report: %s", r.stdout)
				}
			}
		})
	}
}

func TestReviewComparisonLongLineNewlineEvidence(t *testing.T) {
	for _, endings := range [][2]string{{"\n", "\n"}, {"", "\n"}, {"\n", ""}, {"", ""}} {
		t.Run(fmt.Sprintf("%q-%q", endings[0], endings[1]), func(t *testing.T) {
			fx, root, a, b := compareFixture(t)
			writeFile(t, filepath.Join(a, "long-line.txt"), []byte(strings.Repeat("left", 1100)+endings[0]), 0600)
			writeFile(t, filepath.Join(b, "long-line.txt"), []byte(strings.Repeat("right", 1100)+endings[1]), 0600)
			left, right := compareIDs(t, fx, root)
			before := treeManifest(t, fx.base)
			r := fx.run(t, "compare", "--root", root, "--json", left, right)
			assertExit(t, r, 0)
			c := compareReport(t, r)
			found := false
			for _, change := range c.Report.Changes {
				if change.Path != "long-line.txt" {
					continue
				}
				found = true
				want := 0
				for _, ending := range endings {
					if ending == "" {
						want++
					}
				}
				got := strings.Count(strings.Join(change.Diff, "\n"), "No newline at end of file")
				if got != want || !change.DiffTruncated || change.Kind != "text" {
					t.Fatalf("newline evidence: markers=%d, want %d; %+v", got, want, change)
				}
			}
			if !found || c.Report.Coverage != "complete" || c.Report.Outcome != "different" {
				t.Fatalf("long-line comparison: %s", r.stdout)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestReviewComparisonAncestorTypeProvesMembership(t *testing.T) {
	for _, cause := range []string{"regular-file", "unreadable-file", "oversized-file", "unreadable-directory", "internal-directory-link", "external-directory-link", "broken-link", "regular-file-link"} {
		t.Run(cause, func(t *testing.T) {
			fx, root, a, b := compareFixture(t)
			paths := []string{"scripts/run.sh", "scripts/nested/run.sh", "scripts/nested/deeper/run.sh"}
			for _, path := range paths {
				writeFile(t, filepath.Join(a, path), []byte("script contents for "+path+"\n"), 0600)
			}
			var before []byte
			var directoryMode os.FileMode
			proven := cause == "regular-file" || cause == "unreadable-file" || cause == "oversized-file"
			switch cause {
			case "regular-file":
				writeFile(t, filepath.Join(b, "scripts"), []byte("regular file\n"), 0600)
			case "unreadable-file":
				writeFile(t, filepath.Join(b, "scripts"), []byte("unreadable contents\n"), 0000)
				if f, err := os.Open(filepath.Join(b, "scripts")); err == nil {
					f.Close()
					t.Fatal("unreadable-file control opened")
				} else if !os.IsPermission(err) {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(filepath.Join(b, "scripts"), 0600) })
			case "oversized-file":
				writeFile(t, filepath.Join(b, "scripts"), []byte(strings.Repeat("x", 8*1024*1024+1)), 0600)
			case "unreadable-directory":
				for _, path := range paths {
					writeFile(t, filepath.Join(b, path), []byte("same target contents\n"), 0600)
				}
				before = treeManifest(t, fx.base)
				dir := filepath.Join(b, "scripts")
				info, err := os.Stat(dir)
				if err != nil {
					t.Fatal(err)
				}
				directoryMode = info.Mode().Perm()
				if err := os.Chmod(dir, 0000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(dir, directoryMode) })
				if _, err := os.ReadDir(dir); !os.IsPermission(err) {
					t.Fatalf("unreadable-directory control: %v", err)
				}
			default:
				target := "target"
				if cause == "external-directory-link" {
					target = filepath.Join(fx.base, "external-target")
				}
				if cause == "broken-link" {
					target = "missing-target"
				} else if cause == "regular-file-link" {
					writeFile(t, filepath.Join(b, target), []byte("regular linked file\n"), 0600)
				} else {
					targetDir := target
					if !filepath.IsAbs(target) {
						targetDir = filepath.Join(b, target)
					}
					for _, path := range paths {
						writeFile(t, filepath.Join(targetDir, strings.TrimPrefix(path, "scripts/")), []byte("linked target contents\n"), 0600)
					}
				}
				if err := os.Symlink(target, filepath.Join(b, "scripts")); err != nil {
					t.Fatal(err)
				}
			}
			inventory := fx.run(t, "list", "--root", root, "--json")
			var list publicList
			decodeJSON(t, inventory.stdout, &list)
			var left, right string
			for _, source := range list.Sources {
				if source.SourcePath == canonicalPath(t, a) {
					left = source.ID
				}
				if source.SourcePath == canonicalPath(t, b) {
					right = source.ID
				}
			}
			if left == "" || right == "" {
				t.Fatalf("sources absent: %s", inventory.stdout)
			}
			if before == nil {
				before = treeManifest(t, fx.base)
			}
			for _, pair := range [][2]string{{left, right}, {right, left}} {
				r := fx.run(t, "compare", "--root", root, "--json", pair[0], pair[1])
				c := compareReport(t, r)
				exit, coverage := 1, "incomplete"
				if cause == "regular-file" {
					exit = 0
					coverage = "complete"
				}
				assertExit(t, r, exit)
				if c.Report.Coverage != coverage {
					t.Fatalf("coverage: %s", r.stdout)
				}
				found := 0
				for _, change := range c.Report.Changes {
					for _, path := range paths {
						if change.Path != path {
							continue
						}
						found++
						if proven {
							want := "removed"
							if pair[0] == right {
								want = "added"
							}
							if change.Status != want || change.Kind != "text" || !strings.Contains(strings.Join(change.Diff, "\n"), "script contents for "+path) {
								t.Fatalf("proven child absence lost: %+v", change)
							}
						} else if change.Status != "unknown" || change.Kind != "unknown" || len(change.Diff) != 0 {
							t.Fatalf("unobserved subtree claimed: %+v", change)
						}
					}
				}
				if found != len(paths) {
					t.Fatalf("children missing: %s", r.stdout)
				}
			}
			if cause == "unreadable-directory" {
				dir := filepath.Join(b, "scripts")
				info, err := os.Stat(dir)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0 {
					t.Fatal("comparison changed unreadable directory permissions")
				}
				if err := os.Chmod(dir, directoryMode); err != nil {
					t.Fatal(err)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestReviewComparisonUnknownChildLimitsCoverage(t *testing.T) {
	fx, root, a, b := compareFixture(t)
	writeFile(t, filepath.Join(a, "scripts/run.sh"), []byte("physical script\n"), 0600)
	writeFile(t, filepath.Join(b, "target/run.sh"), []byte("known target script\n"), 0600)
	if err := os.Symlink("target", filepath.Join(b, "scripts")); err != nil {
		t.Fatal(err)
	}
	inventory := fx.run(t, "list", "--root", root, "--json")
	var list publicList
	decodeJSON(t, inventory.stdout, &list)
	var left, right string
	for _, s := range list.Sources {
		if s.SourcePath == canonicalPath(t, a) {
			left = s.ID
		}
		if s.SourcePath == canonicalPath(t, b) {
			right = s.ID
		}
	}
	if left == "" || right == "" {
		t.Fatal("missing physical sources")
	}
	before := treeManifest(t, fx.base)
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		r := fx.run(t, "compare", "--root", root, "--json", pair[0], pair[1])
		c := compareReport(t, r)
		if r.code != 1 || c.Report.Coverage != "incomplete" || c.Report.Outcome != "indeterminate" {
			t.Errorf("unknown membership claimed complete: exit=%d report=%+v", r.code, c.Report)
		}
		uncertainSide := "right"
		knownStatus := "added"
		if pair[0] == right {
			uncertainSide = "left"
			knownStatus = "removed"
		}
		child, known, issue := false, false, false
		for _, change := range c.Report.Changes {
			if change.Path == "scripts/run.sh" {
				child = change.Status == "unknown" && change.Kind == "unknown" && len(change.Diff) == 0
			}
			if change.Path == "target/run.sh" {
				known = change.Status == knownStatus && change.Kind == "text" && strings.Contains(strings.Join(change.Diff, "\n"), "known target script")
			}
		}
		for _, problem := range c.Report.Issues {
			issue = issue || (problem.Side == uncertainSide && problem.Path == "scripts/run.sh" && strings.Contains(problem.Reason, "membership") && strings.Contains(problem.Reason, "excluded or unobserved subtree"))
		}
		if !child || !known || !issue {
			t.Errorf("child=%t known=%t uncertainty explanation=%t: %s", child, known, issue, r.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestReviewComparisonIdenticalInternalLinksComplete(t *testing.T) {
	fx, root, a, b := compareFixture(t)
	for _, path := range []string{a, b} {
		writeFile(t, filepath.Join(path, "target/run.sh"), []byte("same script\n"), 0600)
		if err := os.Symlink("target", filepath.Join(path, "scripts")); err != nil {
			t.Fatal(err)
		}
	}
	left, right := compareIDs(t, fx, root)
	before := treeManifest(t, fx.base)
	r := fx.run(t, "compare", "--root", root, "--json", left, right)
	assertExit(t, r, 0)
	c := compareReport(t, r)
	if c.Report.Coverage != "complete" || c.Report.Outcome != "identical" || len(c.Report.Issues) != 0 || len(c.Report.Changes) != 0 {
		t.Fatalf("identical internal links made coverage incomplete: %s", r.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
