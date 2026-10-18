package acceptance_test

import (
	"bytes"
	"github.com/alexdancer/skillglass/internal/collection"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE41Round2UnreadableSibling(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary-control", true: "unreadable-sibling"}[unreadable], func(t *testing.T) {
			fx, _ := codexFixture(t)
			root := filepath.Join(fx.base, "allowed", "custom")
			manifest := codexSkill(t, root, "target", "healthy-target")
			sibling := filepath.Join(root, "unreadable")
			if err := os.MkdirAll(sibling, 0700); err != nil {
				t.Fatal(err)
			}
			if unreadable {
				if err := os.Chmod(sibling, 0); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(sibling, 0700)
			}
			r := fx.run(t, "list", "--root", root, "--json")
			var v collection.View
			decodeJSON(t, r.stdout, &v)
			id := ""
			for _, s := range v.Sources {
				if s.Name == "healthy-target" {
					id = string(s.ID)
				}
			}
			if id == "" {
				t.Fatal("target absent")
			}
			if unreadable {
				found := false
				for _, r := range v.Scope.Roots {
					if r.Requested == root && r.State == collection.RootIncomplete {
						found = true
					}
				}
				if !found {
					t.Fatal("aggregate incomplete root not observed")
				}
				found = false
				for _, p := range v.Problems {
					if p.Path == filepath.Join(sibling, "SKILL.md") {
						found = true
					}
				}
				if !found {
					t.Fatal("actual inaccessible sibling diagnostic not observed")
				}
			}
			before := readBytes(t, manifest)
			args := []string{"delete", "--root", root, "--project", fx.project, "--json"}
			r = changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			t.Logf("state=%s reason=%s", p.State, p.Reason)
			if !bytes.Equal(before, readBytes(t, manifest)) {
				t.Fatal("preview changed target")
			}
			if p.State != "ready" {
				t.Fatalf("expected ready for healthy selected target, got %s: %s", p.State, p.Reason)
			}
			if unreadable {
				found := false
				for _, consumer := range p.Consumers {
					for _, problem := range consumer.Problems {
						if problem.LimitsCoverage && problem.Path == filepath.Join(sibling, "SKILL.md") {
							found = true
						}
					}
				}
				if !found {
					t.Fatal("preview omitted actual inaccessible sibling evidence")
				}
				text := changeCommand(t, fx, "", "delete", "--root", root, "--project", fx.project, id)
				assertExit(t, text, 1)
				if !strings.Contains(string(text.stdout), "Discovery limit: "+filepath.Join(sibling, "SKILL.md")) {
					t.Fatal("human preview omitted actual inaccessible sibling path")
				}
			}
			r = changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, r, 0)
			if _, err := os.Lstat(filepath.Dir(manifest)); !os.IsNotExist(err) {
				t.Fatal("target survives", err)
			}
			if _, err := os.Lstat(sibling); err != nil {
				t.Fatal("sibling removed", err)
			}
		})
	}
}
func TestALE41Round2ManifestDependency(t *testing.T) {
	for _, kind := range []string{"ordinary-control", "standalone", "directory", "case-directory", "hardlink-survivor", "internal-link", "stale-dependency"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), []byte(`{"defaultProjectTrust":"always"}`), 0600)
			var target string
			if kind != "ordinary-control" && kind != "standalone" {
				target = codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
			} else {
				target = filepath.Join(fx.home, ".pi/agent/skills/standalone.md")
				writeFile(t, target, []byte("---\nname: target\ndescription: Inert fixture.\n---\nInert body.\n"), 0600)
			}
			dep := filepath.Join(fx.home, ".codex/skills/dependent/SKILL.md")
			writeFile(t, dep, []byte("---\nname: dependent\ndescription: Independent fixture.\n---\nIndependent bytes.\n"), 0600)
			depBefore := readBytes(t, dep)
			linkTarget := target
			sentinel := filepath.Join(fx.base, "allowed", "external-manifest.md")
			if kind == "hardlink-survivor" {
				if err := os.Link(target, sentinel); err != nil {
					t.Fatal(err)
				}
				linkTarget = sentinel
			}
			if kind == "internal-link" {
				writeFile(t, sentinel, readBytes(t, target), 0600)
				linkTarget = filepath.Join(filepath.Dir(target), "bridge.md")
				if err := os.Symlink(sentinel, linkTarget); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "case-directory" {
				linkTarget = filepath.Join(filepath.Dir(filepath.Dir(target)), "TARGET", "SKILL.md")
				a, err := os.Stat(linkTarget)
				b, otherErr := os.Stat(target)
				if err != nil || otherErr != nil || !os.SameFile(a, b) {
					t.Skip("fixture filesystem does not support case aliases")
				}
			}
			if kind != "ordinary-control" {
				if err := os.Remove(dep); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(linkTarget, dep); err != nil {
					t.Fatal(err)
				}
			}
			pi := piList(t, fx, 0)
			id := ""
			for _, s := range pi.Sources {
				if s.Name == "target" {
					id = string(s.ID)
				}
			}
			if id == "" {
				t.Fatal("Pi target absent")
			}
			cr := fx.run(t, "list", "--agent", "codex", "--project", fx.project, "--json")
			assertExit(t, cr, 0)
			var cv collection.View
			decodeJSON(t, cr.stdout, &cv)
			depID := ""
			for _, s := range cv.Sources {
				if s.Directory == filepath.Dir(dep) {
					depID = string(s.ID)
				}
			}
			if depID == "" || depID == id {
				t.Fatalf("independent identities missing: pi=%s codex=%s", id, depID)
			}
			args := []string{"delete", "--agent", "pi", "--project", fx.project, "--json"}
			r := changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if p.State != "ready" {
				t.Fatal(p.State, p.Reason)
			}
			count := -1
			for _, c := range p.Consumers {
				if c.Application == "codex" {
					if len(c.References) != 0 {
						t.Fatal("independent source was relabeled as an exact-source reference", c.References)
					}
					count = len(c.Dependencies)
					for _, dependency := range c.Dependencies {
						if string(dependency.Source) != depID || dependency.Manifest != dep || dependency.RemovedPath != p.Target.Path || len(dependency.References) == 0 || dependency.Availability == nil || len(dependency.Availability.References) == 0 {
							t.Fatal("manifest dependency lacks identity, impact or provenance", dependency)
						}
					}
					t.Logf("Codex discovery=%s impact reason=%s", c.Scope.Completeness, c.Reason)
				}
			}
			t.Logf("pi ID=%s dependent ID=%s codex affected=%d", id, depID, count)
			want := 1
			if kind == "ordinary-control" || kind == "hardlink-survivor" {
				want = 0
			}
			if count != want {
				t.Fatalf("expected %d known affected Codex manifest dependencies, got %d", want, count)
			}
			text := changeCommand(t, fx, "", "delete", "--agent", "pi", "--project", fx.project, id)
			assertExit(t, text, 1)
			if want > 0 && (!strings.Contains(string(text.stdout), "Dependent manifest: "+dep) || !strings.Contains(string(text.stdout), "Independent source: "+depID) || !strings.Contains(string(text.stdout), "Dependent provenance:")) {
				t.Fatal("human preview omits dependent identity or provenance", string(text.stdout))
			}
			if kind == "stale-dependency" {
				writeFile(t, sentinel, readBytes(t, target), 0600)
				if err := os.Remove(dep); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(sentinel, dep); err != nil {
					t.Fatal(err)
				}
				before := treeManifest(t, fx.base)
				stale := changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
				assertExit(t, stale, 1)
				var outcome collection.DeleteOutcome
				decodeJSON(t, stale.stdout, &outcome)
				if outcome.State != "stale" || len(outcome.Removed) != 0 {
					t.Fatal(outcome)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			r = changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, r, 0)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			if out.State != "removed" || !out.Verified {
				t.Fatal(out)
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatal("target remains", err)
			}
			if kind == "ordinary-control" {
				if count != 0 || !bytes.Equal(readBytes(t, dep), depBefore) {
					t.Fatal("independent control changed")
				}
				return
			}
			info, err := os.Lstat(dep)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("dependent link changed", err)
			}
			link, err := os.Readlink(dep)
			if err != nil || link != linkTarget {
				t.Fatal("link bytes changed", link, err)
			}
			if kind == "hardlink-survivor" {
				if !bytes.Equal(readBytes(t, dep), readBytes(t, sentinel)) {
					t.Fatal("surviving hardlink name lost its content")
				}
				return
			}
			if kind == "internal-link" {
				if _, err := os.Stat(sentinel); err != nil {
					t.Fatal("external manifest target was removed", err)
				}
			}
			if _, err := os.Stat(dep); !os.IsNotExist(err) {
				t.Fatal("expected dangling dependent", err)
			}
			t.Log("actual removal verified; dependent directory and unchanged symlink survive, but manifest is dangling")
		})
	}
}
