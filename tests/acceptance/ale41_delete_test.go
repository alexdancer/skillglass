package acceptance_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func ale41Delete(t *testing.T, fx fixture, id, token, policy string, want int) commandResult {
	t.Helper()
	args := []string{"delete", "--agent", "codex", "--project", fx.project, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	args = append(args, id)
	r := changeCommand(t, fx, policy, args...)
	assertExit(t, r, want)
	return r
}
func ale41Preview(t *testing.T, fx fixture, id string) collection.DeleteView {
	t.Helper()
	r := ale41Delete(t, fx, id, "", "", 1)
	var p collection.DeleteView
	decodeJSON(t, r.stdout, &p)
	return p
}
func TestALE41PublicSharedDelete(t *testing.T) {
	fx, id, target := ale40Fixture(t, false)
	dir := filepath.Dir(target)
	external := filepath.Join(fx.base, "allowed", "external.txt")
	writeFile(t, external, []byte("external sentinel"), 0600)
	if err := os.Symlink(external, filepath.Join(dir, "external-link")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".hidden"), []byte("hidden resource"), 0600)
	copy := codexSkill(t, filepath.Join(fx.home, ".codex/skills"), "independent-copy", "ale40-target")
	diverged := codexSkill(t, filepath.Join(fx.home, ".codex/skills"), "diverged-copy", "ale40-target")
	writeFile(t, copy, readBytes(t, target), 0600)
	divergedBefore := readBytes(t, diverged)
	before := treeManifest(t, fx.base)
	p := ale41Preview(t, fx, id)
	if p.State != "ready" {
		t.Fatalf("%s: %s", p.State, p.Reason)
	}
	if p.Target.Path != dir || len(p.Consumers) != 7 || len(p.Contents) != 4 || p.Ownership.State != "local-user-controlled" {
		t.Fatalf("preview = %+v", p)
	}
	for _, consumer := range p.Consumers {
		if len(consumer.References) == 0 {
			t.Fatalf("missing %s consumer", consumer.Application)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	wrong := ale41Delete(t, fx, id, "yes", "", 1)
	var refused collection.DeleteOutcome
	decodeJSON(t, wrong.stdout, &refused)
	if refused.State != "refused" {
		t.Fatal(refused)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	r := ale41Delete(t, fx, id, p.Confirmation, "", 0)
	var out collection.DeleteOutcome
	decodeJSON(t, r.stdout, &out)
	if out.State != "removed" || !out.Verified || len(out.Removed) != 4 {
		t.Fatalf("outcome = %+v", out)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal("selected source survives", err)
	}
	if string(readBytes(t, external)) != "external sentinel" || !bytes.Contains(readBytes(t, copy), []byte("ale40-target")) || !bytes.Equal(readBytes(t, diverged), divergedBefore) {
		t.Fatal("independent content changed")
	}
	for _, root := range []string{".claude/skills", ".hermes/skills", ".pi/agent/skills", ".gemini/skills"} {
		if info, err := os.Lstat(filepath.Join(fx.home, root, "ale40-target")); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("shared reference removed", root, err)
		}
	}
	for _, path := range []string{".codex/config.toml", ".claude/settings.json", ".hermes/config.yaml", ".pi/agent/settings.json", ".gemini/settings.json"} {
		if !bytes.Contains(readBytes(t, filepath.Join(fx.home, path)), []byte("keep")) {
			t.Fatal("settings changed", path)
		}
	}
}
func TestALE41PublicStaleDelete(t *testing.T) {
	for _, mutation := range []string{"bytes", "membership", "root", "consumer", "native-input", "alias"} {
		t.Run(mutation, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			dir := filepath.Dir(target)
			resource := filepath.Join(dir, "resource")
			writeFile(t, resource, []byte("before"), 0600)
			p := ale41Preview(t, fx, id)
			if p.State != "ready" {
				t.Fatal(p.Reason)
			}
			switch mutation {
			case "bytes":
				writeFile(t, resource, []byte("after!"), 0600)
			case "membership":
				writeFile(t, filepath.Join(dir, ".new"), []byte("new"), 0600)
			case "root":
				if err := os.Rename(dir, dir+"-moved"); err != nil {
					t.Fatal(err)
				}
				codexSkill(t, filepath.Dir(dir), filepath.Base(dir), "ale40-target")
			case "consumer":
				root := filepath.Join(fx.home, ".codeaf/skills")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(dir, filepath.Join(root, "new")); err != nil {
					t.Fatal(err)
				}
			case "native-input":
				writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte("{\"keep\":false}"), 0600)
			case "alias":
				path := filepath.Join(fx.home, ".claude/skills/ale40-target")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(fx.project, path); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			r := ale41Delete(t, fx, id, p.Confirmation, "", 1)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			if out.State != "stale" && out.State != "refused" {
				t.Fatal(out.State, out.Reason)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE41PublicBoundaryRefusal(t *testing.T) {
	for _, kind := range []string{"nested", "writable", "special", "large", "managed"} {
		t.Run(kind, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			dir := filepath.Dir(target)
			switch kind {
			case "nested":
				codexSkill(t, dir, "nested", "nested")
			case "writable":
				if err := os.Chmod(dir, 0777); err != nil {
					t.Fatal(err)
				}
			case "special":
				if err := os.Symlink(strings.Repeat("x", 200), filepath.Join(dir, "link")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(dir, "dir"), 0777); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(dir, "dir"), 0777); err != nil {
					t.Fatal(err)
				}
			case "large":
				f, err := os.Create(filepath.Join(dir, "large"))
				if err != nil {
					t.Fatal(err)
				}
				if err = f.Truncate(257 << 20); err != nil {
					t.Fatal(err)
				}
				f.Close()
			case "managed":
				managed := filepath.Join(fx.home, ".codex/plugins/cache/managed/1/skills/target")
				if err := os.MkdirAll(filepath.Dir(managed), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(dir, managed); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(managed, dir); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			p := ale41Preview(t, fx, id)
			if p.State != "refused" || p.Confirmation != "" {
				t.Fatalf("unsafe preview %+v", p)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE41PublicCustomAndLinkBoundaries(t *testing.T) {
	for _, kind := range []string{"direct-custom", "directory-alias", "manifest-link", "internal-hardlink", "nested-link"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			root := filepath.Join(fx.base, "allowed", "custom")
			target := codexSkill(t, root, "target", "delete-custom")
			dir := filepath.Dir(target)
			external := codexSkill(t, filepath.Join(fx.base, "allowed", "external"), "independent", "delete-custom")
			original := readBytes(t, external)
			selectedRoot := root
			switch kind {
			case "direct-custom":
				selectedRoot = dir
			case "directory-alias":
				selectedRoot = filepath.Join(fx.base, "allowed", "alias")
				if err := os.Symlink(dir, selectedRoot); err != nil {
					t.Fatal(err)
				}
			case "manifest-link":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, target); err != nil {
					t.Fatal(err)
				}
			case "internal-hardlink":
				if err := os.Link(external, filepath.Join(dir, "hardlinked")); err != nil {
					t.Fatal(err)
				}
			case "nested-link":
				if err := os.Symlink(filepath.Dir(external), filepath.Join(dir, "nested")); err != nil {
					t.Fatal(err)
				}
			}
			list := fx.run(t, "list", "--root", selectedRoot, "--json")
			assertExit(t, list, 0)
			var v collection.View
			decodeJSON(t, list.stdout, &v)
			if len(v.Sources) != 1 {
				t.Fatal(v)
			}
			id := string(v.Sources[0].ID)
			args := []string{"delete", "--root", selectedRoot, "--json"}
			before := treeManifest(t, fx.base)
			r := changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if p.State != "ready" {
				t.Fatal(p.Reason)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			r = changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, r, 0)
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Fatal("canonical content remains", err)
			}
			if !bytes.Equal(readBytes(t, external), original) {
				t.Fatal("external manifest or hardlink content changed")
			}
			if kind == "directory-alias" {
				if info, err := os.Lstat(selectedRoot); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatal("alias was removed", err)
				}
			}
		})
	}
}
func TestALE41PublicStandalone(t *testing.T) {
	for _, hardlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "single-file", true: "hardlink-refused"}[hardlink], func(t *testing.T) {
			fx := piFixture(t)
			target := filepath.Join(fx.home, ".pi/agent/skills/standalone.md")
			writeFile(t, target, []byte("---\nname: standalone\ndescription: Inert Markdown.\n---\nStandalone body.\n"), 0600)
			if hardlink {
				if err := os.Link(target, filepath.Join(fx.base, "allowed", "other.md")); err != nil {
					t.Fatal(err)
				}
			}
			list := piList(t, fx, 0)
			id := ""
			for _, s := range list.Sources {
				if s.Name == "standalone" {
					id = string(s.ID)
				}
			}
			if id == "" {
				t.Fatal("standalone source missing")
			}
			args := []string{"delete", "--agent", "pi", "--project", fx.project, "--json"}
			before := treeManifest(t, fx.base)
			r := changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if hardlink {
				if p.State != "refused" || !strings.Contains(p.Reason, "single-link") {
					t.Fatal(p.State, p.Reason)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			if p.State != "ready" || p.Target.Kind != "standalone" || len(p.Contents) != 1 {
				t.Fatal(p.State, p.Reason, p.Target)
			}
			r = changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, r, 0)
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("standalone survives", err)
			}
			if info, err := os.Stat(filepath.Dir(target)); err != nil || !info.IsDir() {
				t.Fatal("standalone parent deleted", err)
			}
		})
	}
}
func TestALE41PublicBroadRootAndManagedAliasRefused(t *testing.T) {
	for _, kind := range []string{"home", "project", "native-collection", "managed-alias", "fifo", "acl", "depth"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			dir := filepath.Join(fx.base, "allowed", "custom", "target")
			switch kind {
			case "home":
				dir = fx.home
			case "project":
				dir = fx.project
			case "native-collection":
				dir = filepath.Join(fx.home, ".agents/skills")
			case "managed-alias":
				dir = filepath.Join(fx.home, ".codex/plugins/cache/demo/1/skills/target")
			}
			writeFile(t, filepath.Join(dir, "SKILL.md"), []byte("---\nname: broad\ndescription: Inert fixture.\n---\nBody.\n"), 0600)
			selected := dir
			if kind == "managed-alias" {
				selected = filepath.Join(fx.base, "allowed", "custom-alias")
				if err := os.Symlink(dir, selected); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "fifo" {
				if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "acl" {
				cmd := exec.Command("/bin/chmod", "+a", "everyone allow write", dir)
				if raw, err := cmd.CombinedOutput(); err != nil {
					t.Fatal(string(raw), err)
				}
			}
			if kind == "depth" {
				deep := dir
				for i := 0; i < 129; i++ {
					deep = filepath.Join(deep, "d")
				}
				if err := os.MkdirAll(deep, 0700); err != nil {
					t.Fatal(err)
				}
			}
			r := fx.run(t, "list", "--root", selected, "--json")
			var v collection.View
			decodeJSON(t, r.stdout, &v)
			id := ""
			for _, s := range v.Sources {
				if s.Directory == dir {
					id = string(s.ID)
				}
			}
			if id == "" {
				t.Fatal("missing selected source", r.code)
			}
			before := treeManifest(t, fx.base)
			r = changeCommand(t, fx, "", "delete", "--root", selected, "--project", fx.project, "--json", id)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if p.State != "refused" {
				t.Fatal("unsafe boundary accepted", kind, p.Reason)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE41PublicIncompleteUnrelatedScope(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	hidden := filepath.Join(fx.base, "allowed", "unreadable-unrelated")
	if err := os.MkdirAll(hidden, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(hidden, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(hidden, 0700)
	args := []string{"delete", "--agent", "codex", "--project", fx.project, "--root", hidden, "--json"}
	before := treeManifest(t, fx.home)
	r := changeCommand(t, fx, "", append(args, id)...)
	assertExit(t, r, 1)
	var p collection.DeleteView
	decodeJSON(t, r.stdout, &p)
	if p.State != "ready" {
		t.Fatal(p.Reason)
	}
	found := false
	for _, c := range p.Consumers {
		for _, root := range c.Scope.Roots {
			if root.Requested == hidden && root.State == collection.RootUnreadable && c.Scope.Completeness == collection.Incomplete {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("inaccessible scope was not disclosed")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.home))
	r = changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
	assertExit(t, r, 0)
}

func TestALE41PublicACLProof(t *testing.T) {
	for _, scenario := range []string{"home-deny-delete", "read-only-target", "write-grant", "external-target-write", "stale-ancestor-read", "stale-target-read"} {
		t.Run(scenario, func(t *testing.T) {
			fx, id, target := ale40Fixture(t, false)
			dir := filepath.Dir(target)
			acl := func(path, rule string) {
				t.Helper()
				cmd := exec.Command("/bin/chmod", "+a", rule, path)
				if raw, err := cmd.CombinedOutput(); err != nil {
					t.Fatal(string(raw), err)
				}
				t.Cleanup(func() {
					if _, err := os.Lstat(path); err == nil {
						if raw, err := exec.Command("/bin/chmod", "-N", path).CombinedOutput(); err != nil {
							t.Error(string(raw), err)
						}
					}
				})
			}
			switch scenario {
			case "home-deny-delete":
				acl(fx.home, "everyone deny delete")
			case "read-only-target":
				acl(dir, "everyone allow read")
			case "write-grant":
				acl(dir, "everyone allow write")
			case "external-target-write":
				external := filepath.Join(fx.base, "allowed", "external-acl")
				writeFile(t, external, []byte("external ACL sentinel"), 0600)
				acl(external, "everyone allow write")
				if err := os.Symlink(external, filepath.Join(dir, "outside")); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			p := ale41Preview(t, fx, id)
			if scenario == "write-grant" {
				if p.State != "refused" || !strings.Contains(p.Reason, "ACL grants mutation") {
					t.Fatal(p.State, p.Reason)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			if p.State != "ready" {
				t.Fatal(p.State, p.Reason)
			}
			if strings.HasPrefix(scenario, "stale-") {
				path := fx.home
				if scenario == "stale-target-read" {
					path = dir
				}
				acl(path, "everyone allow read")
				before = treeManifest(t, fx.base)
				r := ale41Delete(t, fx, id, p.Confirmation, "", 1)
				var out collection.DeleteOutcome
				decodeJSON(t, r.stdout, &out)
				if out.State != "stale" {
					t.Fatal(out.State, out.Reason)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			r := ale41Delete(t, fx, id, p.Confirmation, "", 0)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			if !out.Verified || out.State != "removed" {
				t.Fatal(out.State, out.Reason)
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Fatal("selected content remains", err)
			}
			if scenario == "external-target-write" {
				if raw := readBytes(t, filepath.Join(fx.base, "allowed", "external-acl")); string(raw) != "external ACL sentinel" {
					t.Fatal("external target changed")
				}
			}
		})
	}
}

func TestALE41PublicCaseAliases(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct-custom", true: "managed"}[managed], func(t *testing.T) {
			fx, _ := codexFixture(t)
			root := filepath.Join(fx.base, "allowed", "case-root")
			if managed {
				root = filepath.Join(fx.home, ".codex/plugins/cache/demo/1/skills")
			}
			manifest := codexSkill(t, root, "case-bundle", "case-alias")
			alias := filepath.Join(root, "CASE-BUNDLE")
			actual, err := os.Stat(filepath.Dir(manifest))
			if err != nil {
				t.Fatal(err)
			}
			aliasInfo, err := os.Stat(alias)
			if err != nil || !os.SameFile(actual, aliasInfo) {
				t.Skip("filesystem has no case alias for this fixture")
			}
			r := fx.run(t, "list", "--root", alias, "--json")
			var view collection.View
			decodeJSON(t, r.stdout, &view)
			if len(view.Sources) != 1 {
				t.Fatal("exact source missing")
			}
			id := string(view.Sources[0].ID)
			args := []string{"delete", "--root", alias, "--json"}
			before := treeManifest(t, fx.base)
			r = changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if managed {
				if p.State != "refused" {
					t.Fatal("case alias bypassed manager", p.Reason)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			if p.State != "ready" {
				t.Fatal(p.State, p.Reason)
			}
			r = changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, r, 0)
			if _, err := os.Lstat(filepath.Dir(manifest)); !os.IsNotExist(err) {
				t.Fatal("canonical directory remains", err)
			}
		})
	}
}

func TestALE41PublicCodeAFSelectedSource(t *testing.T) {
	fx, id, target := ale40Fixture(t, false)
	args := []string{"delete", "--agent", "codeaf", "--project", fx.project, "--json"}
	r := changeCommand(t, fx, "", append(args, id)...)
	assertExit(t, r, 1)
	var p collection.DeleteView
	decodeJSON(t, r.stdout, &p)
	if p.State != "ready" {
		t.Fatal(p.State, p.Reason)
	}
	r = changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
	assertExit(t, r, 0)
	var out collection.DeleteOutcome
	decodeJSON(t, r.stdout, &out)
	if !out.Verified || out.State != "removed" {
		t.Fatal(out.State, out.Reason)
	}
	if _, err := os.Lstat(filepath.Dir(target)); !os.IsNotExist(err) {
		t.Fatal("selected CodeAF source remains", err)
	}
}
