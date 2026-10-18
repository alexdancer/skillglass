package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41SymlinkedManagerInputBytesSealed(t *testing.T) {
	fx, _ := codexFixture(t)
	root := filepath.Join(fx.base, "allowed", "custom")
	manifest := codexSkill(t, root, "target", "input-seal")
	input := filepath.Join(fx.base, "allowed", "registry.json")
	registry := filepath.Join(fx.home, ".claude/plugins/installed_plugins.json")
	writeFile(t, input, []byte(`{"version":2,"plugins":{}}`), 0600)
	if err := os.MkdirAll(filepath.Dir(registry), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(input, registry); err != nil {
		t.Fatal(err)
	}
	p, id := findingCustomPreview(t, fx, root)
	changed := []byte("{\n\"version\":2,\"plugins\":{}\n}\n")
	writeFile(t, input, changed, 0600)
	before := treeManifest(t, fx.home)
	r := changeCommand(t, fx, "", "delete", "--root", root, "--json", "--confirm", p.Confirmation, id)
	assertExit(t, r, 1)
	var out collection.DeleteOutcome
	decodeJSON(t, r.stdout, &out)
	if out.State != "stale" || len(out.Removed) != 0 {
		t.Fatal(out.State, out.Reason)
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal("stale input changed selected content", err)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.home))
	p, _ = findingCustomPreview(t, fx, root)
	r = changeCommand(t, fx, "", "delete", "--root", root, "--json", "--confirm", p.Confirmation, id)
	assertExit(t, r, 0)
	if _, err := os.Lstat(filepath.Dir(manifest)); !os.IsNotExist(err) {
		t.Fatal("selected bundle survives fresh consent", err)
	}
	if !bytes.Equal(changed, readBytes(t, input)) {
		t.Fatal("manager input changed")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.home))
}
