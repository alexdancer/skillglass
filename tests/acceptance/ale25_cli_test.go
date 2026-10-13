package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

type publicComparison struct {
	Operation string
	Report    struct {
		LeftID   string `json:"left_id"`
		RightID  string `json:"right_id"`
		Relation string
		Outcome  string
		Coverage string
		Changes  []struct {
			Path          string
			Status        string
			Kind          string
			Diff          []string
			DiffTruncated bool `json:"diff_truncated"`
		}
		Issues []struct {
			Side   string
			Path   string
			Reason string
		}
	}
}

func compareFixture(t *testing.T) (fixture, string, string, string) {
	t.Helper()
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "compare")
	a, b := filepath.Join(root, "left"), filepath.Join(root, "right")
	for _, p := range []string{a, b} {
		writeFile(t, filepath.Join(p, "SKILL.md"), []byte("---\nname: same-name\ndescription: compare fixture\n---\nSame instructions\n"), 0600)
	}
	return fx, root, a, b
}
func compareIDs(t *testing.T, fx fixture, root string) (string, string) {
	t.Helper()
	r := fx.run(t, "list", "--root", root, "--json")
	var l publicList
	decodeJSON(t, r.stdout, &l)
	if len(l.Sources) != 2 {
		t.Fatalf("sources = %s", r.stdout)
	}
	return l.Sources[0].ID, l.Sources[1].ID
}
func compareReport(t *testing.T, r commandResult) publicComparison {
	t.Helper()
	var c publicComparison
	if err := json.Unmarshal(r.stdout, &c); err != nil {
		t.Fatalf("compare: %v %s %s", err, r.stdout, r.stderr)
	}
	return c
}
func TestALE25PublicIndependentEqualThenDifferencesAndNoWrites(t *testing.T) {
	fx, root, a, b := compareFixture(t)
	left, right := compareIDs(t, fx, root)
	before := treeManifest(t, fx.base)
	r := fx.run(t, "compare", "--root", root, "--json", left, right)
	assertExit(t, r, 0)
	c := compareReport(t, r)
	if c.Report.Relation != "independent-copies" || c.Report.Outcome != "identical" || c.Report.Coverage != "complete" || left == right {
		t.Fatalf("equal: %s", r.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, filepath.Join(b, "SKILL.md"), []byte("malformed but readable\nNEW_INSTRUCTION\n"), 0600)
	writeFile(t, filepath.Join(a, "removed.txt"), []byte("removed text\n"), 0600)
	writeFile(t, filepath.Join(b, "added.txt"), []byte("added text\n"), 0600)
	for i, p := range []string{a, b} {
		writeFile(t, filepath.Join(p, "binary.dat"), []byte{0, byte(i), 255}, 0600)
		writeFile(t, filepath.Join(p, "mode.sh"), []byte("#!/bin/sh\ntouch "+fx.marker+"\n"), 0600+os.FileMode(i)*0100)
	}
	if err := os.Mkdir(filepath.Join(a, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(b, "empty"), []byte("kind changed"), 0600)
	before = treeManifest(t, fx.base)
	r = fx.run(t, "compare", "--root", root, "--json", left, right)
	assertExit(t, r, 0)
	c = compareReport(t, r)
	if c.Report.Outcome != "different" || len(c.Report.Changes) < 6 {
		t.Fatalf("differences: %s", r.stdout)
	}
	binary := false
	for _, v := range c.Report.Changes {
		if v.Path == "binary.dat" {
			binary = v.Kind == "binary" && len(v.Diff) == 0
		}
	}
	if !binary {
		t.Fatalf("binary not classified: %s", r.stdout)
	}
	human := fx.run(t, "compare", "--root", root, left, right)
	assertExit(t, human, 0)
	for _, s := range []string{"NEW_INSTRUCTION", "binary", "added", "removed", "@@"} {
		if !bytes.Contains(human.stdout, []byte(s)) {
			t.Fatalf("missing %q: %s", s, human.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatal("bundle script executed")
	}
	same := fx.run(t, "compare", "--root", root, "--json", left, left)
	assertExit(t, same, 0)
	if compareReport(t, same).Report.Relation != "same-source" {
		t.Fatal("same source misrepresented")
	}
}
func TestALE25PublicIncompleteCoverage(t *testing.T) {
	for _, kind := range []string{"external", "broken", "cycle", "unreadable", "special", "limit", "linked-manifest"} {
		t.Run(kind, func(t *testing.T) {
			fx, root, a, b := compareFixture(t)
			switch kind {
			case "external":
				writeFile(t, filepath.Join(fx.base, "allowed", "external-file"), []byte("external target"), 0600)
				if err := os.Symlink(filepath.Join(fx.base, "allowed", "external-file"), filepath.Join(a, "external")); err != nil {
					t.Fatal(err)
				}
			case "broken":
				_ = os.Symlink("absent", filepath.Join(a, "broken"))
			case "cycle":
				_ = os.Symlink("cycle", filepath.Join(a, "cycle"))
			case "unreadable":
				writeFile(t, filepath.Join(a, "blocked"), []byte("secret"), 0000)
			case "special":
				if err := unix.Mkfifo(filepath.Join(a, "fifo"), 0600); err != nil {
					t.Fatal(err)
				}
			case "limit":
				writeFile(t, filepath.Join(a, "large"), bytes.Repeat([]byte("x"), 8*1024*1024+1), 0600)
			case "linked-manifest":
				external := filepath.Join(fx.base, "allowed", "manifest")
				writeFile(t, external, []byte("---\nname: same-name\n---\nlinked\n"), 0600)
				for _, p := range []string{a, b} {
					if err := os.Remove(filepath.Join(p, "SKILL.md")); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(external, filepath.Join(p, "SKILL.md")); err != nil {
						t.Fatal(err)
					}
				}
				writeFile(t, filepath.Join(b, "support"), []byte("divergent"), 0600)
			}
			left, right := compareIDs(t, fx, root)
			before := treeManifest(t, fx.base)
			r := fx.run(t, "compare", "--root", root, "--json", left, right)
			assertExit(t, r, 1)
			c := compareReport(t, r)
			if c.Report.Coverage != "incomplete" || c.Report.Outcome != "indeterminate" || len(c.Report.Issues) == 0 {
				t.Fatalf("incomplete: %s", r.stdout)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE25PublicSelectorsAndEscaping(t *testing.T) {
	fx, root, a, b := compareFixture(t)
	left, right := compareIDs(t, fx, root)
	writeFile(t, filepath.Join(a, "hostile\x1b[31m.txt"), []byte("plain\n"), 0600)
	writeFile(t, filepath.Join(b, "hostile\x1b[31m.txt"), []byte("changed\n"), 0600)
	r := fx.run(t, "compare", "--root", root, left, right)
	assertExit(t, r, 0)
	if strings.Contains(string(r.stdout), "\x1b") {
		t.Fatal("raw terminal escape emitted")
	}
	assertExit(t, fx.run(t, "compare", "--root", root, left), 2)
	assertExit(t, fx.run(t, "compare", "--root", root, left, "src_missing"), 1)
	assertExit(t, fx.run(t, "compare", "--root", root, "--root", fx.missingRoot, left, right), 0)
}
func assertTreeUnchanged(t *testing.T, a, b []byte) {
	t.Helper()
	if !bytes.Equal(a, b) {
		t.Fatalf("comparison changed fixture tree\nbefore=%s\nafter=%s", a, b)
	}
}

func TestALE25PublicLinksSharedReferencesAndBoundedDiff(t *testing.T) {
	fx, root, a, b := compareFixture(t)
	aliasRoot := filepath.Join(fx.base, "allowed", "alias-root")
	if err := os.Mkdir(aliasRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, filepath.Join(aliasRoot, "alias")); err != nil {
		t.Fatal(err)
	}
	left, right := compareIDs(t, fx, root)
	// Same-source through multiple discovered references still returns a distinct relation.
	r := fx.run(t, "compare", "--root", root, "--root", aliasRoot, "--json", left, left)
	assertExit(t, r, 0)
	c := compareReport(t, r)
	if c.Report.Relation != "same-source" {
		t.Fatalf("shared %s", r.stdout)
	}
	for _, p := range []string{a, b} {
		writeFile(t, filepath.Join(p, "target"), []byte("same target"), 0600)
		if err := os.Symlink("target", filepath.Join(p, "internal-link")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(p, "empty-dir"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(a, "long.txt"), []byte(strings.Repeat("left line\n", 1100)), 0600)
	writeFile(t, filepath.Join(b, "long.txt"), []byte(strings.Repeat("right line\n", 1100)), 0600)
	writeFile(t, filepath.Join(a, "newline.txt"), []byte("same\n"), 0600)
	writeFile(t, filepath.Join(b, "newline.txt"), []byte("same"), 0600)
	before := treeManifest(t, fx.base)
	r = fx.run(t, "compare", "--root", root, "--json", left, right)
	assertExit(t, r, 0)
	c = compareReport(t, r)
	truncated, newline := false, false
	for _, v := range c.Report.Changes {
		if v.Path == "long.txt" {
			truncated = v.DiffTruncated && len(v.Diff) < 1010
		}
		if v.Path == "newline.txt" {
			newline = strings.Contains(strings.Join(v.Diff, "\n"), "No newline")
		}
		if v.Path == "internal-link" || v.Path == "empty-dir" {
			t.Fatalf("equal entry changed: %s", r.stdout)
		}
	}
	if !truncated || !newline || c.Report.Coverage != "complete" || c.Report.Outcome != "different" {
		t.Fatalf("bounded output %s", r.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE25PublicEntryLimit(t *testing.T) {
	fx, root, a, _ := compareFixture(t)
	left, right := compareIDs(t, fx, root)
	for i := 0; i < 10001; i++ {
		writeFile(t, filepath.Join(a, fmt.Sprintf("entry-%05d", i)), nil, 0600)
	}
	before := treeManifest(t, fx.base)
	r := fx.run(t, "compare", "--root", root, "--json", left, right)
	assertExit(t, r, 1)
	c := compareReport(t, r)
	found := false
	for _, v := range c.Report.Issues {
		found = found || strings.Contains(v.Reason, "entry limit")
	}
	if !found {
		t.Fatalf("limit missing: %s", r.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
