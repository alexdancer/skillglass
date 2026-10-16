package acceptance_test

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestALE36InventoryScale(t *testing.T) {
	for _, count := range []int{1, 101} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			fx, _ := codexFixture(t)
			for i := 0; i < count; i++ {
				name := fmt.Sprintf("fixture-%d", i)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode/skills"), name, name)
			}
			before := treeManifest(t, fx.base)
			r := fx.run(t, "list", "--agent", "opencode", "--project", fx.project, "--json")
			assertExit(t, r, 0)
			var list codexList
			decodeJSON(t, r.stdout, &list)
			if len(list.Sources) != count {
				t.Fatalf("sources=%d, want %d", len(list.Sources), count)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			t.Logf("public list: sources=%d duration=%s", count, r.duration)
		})
	}
}
