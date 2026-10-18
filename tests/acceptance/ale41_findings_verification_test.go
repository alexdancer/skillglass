package acceptance_test

import (
	"github.com/alexdancer/skillglass/internal/collection"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findingCustomPreview(t *testing.T, fx fixture, root string) (collection.DeleteView, string) {
	t.Helper()
	r := fx.run(t, "list", "--root", root, "--json")
	assertExit(t, r, 0)
	var v collection.View
	decodeJSON(t, r.stdout, &v)
	if len(v.Sources) != 1 {
		t.Fatalf("sources=%+v", v.Sources)
	}
	id := string(v.Sources[0].ID)
	r = changeCommand(t, fx, "", "delete", "--root", root, "--json", id)
	assertExit(t, r, 1)
	var p collection.DeleteView
	decodeJSON(t, r.stdout, &p)
	if p.State != "ready" {
		t.Fatalf("preview=%+v", p)
	}
	return p, id
}
func TestALE41FindingInternalHardlinks(t *testing.T) {
	for _, topology := range []string{"siblings", "separate-directories", "external-third-alias", "hardlinked-symlinks"} {
		t.Run(topology, func(t *testing.T) {
			fx := newFixture(t)
			root := filepath.Join(fx.base, "allowed", "custom")
			dir := filepath.Dir(codexSkill(t, root, "target", "hardlinks"))
			a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
			if topology == "separate-directories" {
				a = filepath.Join(dir, "one", "a")
				b = filepath.Join(dir, "two", "b")
				if err := os.MkdirAll(filepath.Dir(b), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if topology == "hardlinked-symlinks" {
				target := filepath.Join(fx.base, "allowed", "symlink-target")
				writeFile(t, target, []byte("hardlink sentinel"), 0600)
				if err := os.Symlink(target, a); err != nil {
					t.Fatal(err)
				}
				if err := unix.Linkat(unix.AT_FDCWD, a, unix.AT_FDCWD, b, 0); err != nil {
					t.Fatal(err)
				}
				ai, ae := os.Lstat(a)
				bi, be := os.Lstat(b)
				if ae != nil || be != nil || !os.SameFile(ai, bi) || ai.Mode()&os.ModeSymlink == 0 {
					t.Fatal("fixture lacks hardlinked symlink objects", ae, be)
				}
			} else {
				writeFile(t, a, []byte("hardlink sentinel"), 0600)
				if err := os.Link(a, b); err != nil {
					t.Fatal(err)
				}
			}
			external := ""
			if topology == "hardlinked-symlinks" {
				external = filepath.Join(fx.base, "allowed", "symlink-target")
			}
			if topology == "external-third-alias" {
				external = filepath.Join(fx.base, "allowed", "external-alias")
				if err := os.Link(a, external); err != nil {
					t.Fatal(err)
				}
			}
			p, id := findingCustomPreview(t, fx, root)
			r := changeCommand(t, fx, "", "delete", "--root", root, "--json", "--confirm", p.Confirmation, id)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			t.Logf("actual exit=%d state=%s verified=%v reason=%s removed=%v remaining=%v", r.code, out.State, out.Verified, out.Reason, out.Removed, out.Remaining)
			if external != "" && string(readBytes(t, external)) != "hardlink sentinel" {
				t.Error("external alias bytes changed")
			}
			if r.code != 0 || out.State != "removed" || !out.Verified {
				t.Errorf("ordinary unchanged supported hardlinks: want exit=0 state=removed verified=true")
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Errorf("selected bundle survives: %v", err)
			}
		})
	}
}
func TestALE41FindingCustomOnlyConsumers(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "custom-only")
	codexSkill(t, root, "target", "custom-only")
	p, id := findingCustomPreview(t, fx, root)
	text := changeCommand(t, fx, "", "delete", "--root", root, id)
	assertExit(t, text, 1)
	t.Logf("rendered preview:\n%s", text.stdout)
	if strings.Contains(string(text.stdout), "1 exact-source references") {
		t.Error("native-consumer section counts inspection-only references")
	}
	if len(p.References) != 1 {
		t.Errorf("selected inspection reference: want 1 got %d", len(p.References))
	}
	if len(p.Consumers) != 7 {
		t.Fatalf("want seven scope observations got %d", len(p.Consumers))
	}
	for _, c := range p.Consumers {
		t.Logf("%s reason=%s references=%+v availability=%+v", c.Application, c.Reason, c.References, c.Availability)
		if len(c.References) != 0 {
			t.Errorf("%s: want zero native consumers for custom inspection only; got %d", c.Application, len(c.References))
		}
	}
}
