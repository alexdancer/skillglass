package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRootCyclesRemainClassifiedThroughAncestors(t *testing.T) {
	fx := newFixture(t)
	links := filepath.Join(fx.base, "allowed", "root-cycle-links")
	candidates := filepath.Join(fx.base, "allowed", "candidate-cycle-root")
	manifest := filepath.Join(fx.base, "allowed", "manifest-cycle-root")
	for _, directory := range []string{links, candidates, manifest} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, target := range map[string]string{
		filepath.Join(links, "a"):                     "b",
		filepath.Join(links, "b"):                     "a",
		filepath.Join(links, "broken-too many links"): "missing-too many links",
		filepath.Join(candidates, "cycle"):            filepath.Join(links, "a"),
		filepath.Join(manifest, "SKILL.md"):           "other",
		filepath.Join(manifest, "other"):              "SKILL.md",
	} {
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	regular := filepath.Join(links, "ordinary-file")
	writeFile(t, regular, []byte("not a directory\n"), 0o644)
	before := treeManifest(t, fx.base)
	for _, tc := range []struct {
		name  string
		root  string
		code  string
		state string
	}{
		{"direct-root", filepath.Join(links, "a"), "symlink-cycle", "incomplete"},
		{"ancestor-root", filepath.Join(links, "a", "skills"), "symlink-cycle", "incomplete"},
		{"deep-ancestor-root", filepath.Join(links, "a", "nested", "skills"), "symlink-cycle", "incomplete"},
		{"misleading-broken-link", filepath.Join(links, "broken-too many links"), "broken-reference", "incomplete"},
		{"misleading-missing-root", filepath.Join(links, "missing-too many links"), "root-missing", "missing"},
		{"ordinary-file-root", regular, "root-unreadable", "unreadable"},
		{"candidate-link", candidates, "symlink-cycle", "incomplete"},
		{"manifest-link", manifest, "symlink-cycle", "incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := []string{"--root", fx.cleanRoot, "--root", tc.root}
			listed := fx.run(t, append(append([]string{"list", "--json"}, scope...), "--search", "normal-skill")...)
			assertExit(t, listed, 1)
			var inventory publicList
			decodeJSON(t, listed.stdout, &inventory)
			if inventory.Scope.Completeness != "incomplete" || len(sourcesNamed(inventory.Sources, "normal-skill")) != 1 {
				t.Fatalf("useful partial inventory lost: %#v", inventory)
			}
			foundRoot := false
			for _, root := range inventory.Scope.Roots {
				if root.Requested == tc.root {
					foundRoot = true
					if root.State != tc.state || root.ProblemCount != 1 {
						t.Fatalf("root state = %#v, want %s and one problem", root, tc.state)
					}
				}
			}
			if !foundRoot {
				t.Fatalf("selected root disappeared: %#v", inventory.Scope.Roots)
			}
			filtered := fx.run(t, append(append([]string{"problems", "--json"}, scope...), "--problem", tc.code, "--context", "custom", "--source-root", tc.root)...)
			assertExit(t, filtered, 1)
			var problems publicList
			decodeJSON(t, filtered.stdout, &problems)
			if problems.Scope.Completeness != "incomplete" || len(problems.Problems) != 1 || problems.Problems[0].Code != tc.code || !problems.Problems[0].LimitsCoverage {
				t.Fatalf("filtered diagnostics = %#v, want one coverage-limiting %s", problems, tc.code)
			}
			problem := problems.Problems[0]
			if len(problem.ReferenceIDs) == 0 && len(problem.SourceIDs) == 0 && problem.Path != tc.root {
				t.Fatalf("root problem path = %q, want original %q", problem.Path, tc.root)
			}
			human := fx.run(t, append(append([]string{"problems"}, scope...), "--problem", tc.code)...)
			assertExit(t, human, 1)
			if !bytes.Contains(human.stdout, []byte(tc.code)) {
				t.Fatalf("human output omitted %s: %s", tc.code, human.stdout)
			}
		})
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("root cycle queries mutated source or settings fixtures")
	}
}
