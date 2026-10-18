package acceptance_test

import (
	"bytes"
	"encoding/json"
	"github.com/alexdancer/skillglass/internal/collection"
	"os"
	"path/filepath"
	"testing"
)

func TestALE41RetryAliasRelationProof(t *testing.T) {
	for _, scenario := range []string{"accessible", "inaccessible", "missing-boundary", "stale-relation"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newFixture(t)
			install := filepath.Join(fx.base, "allowed", "installation")
			manifest := codexSkill(t, filepath.Join(install, "skills"), "target", "alias-relation")
			target := filepath.Dir(manifest)
			writeFile(t, filepath.Join(install, ".claude-plugin/plugin.json"), []byte(`{"name":"demo","skills":["skills"]}`), 0600)
			sentinel := filepath.Join(install, "independent-sentinel")
			writeFile(t, sentinel, []byte("preserve"), 0600)
			container := filepath.Join(fx.base, "allowed", "alias-access")
			if err := os.MkdirAll(container, 0700); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(container, "installation")
			aliasTarget := install
			if scenario == "stale-relation" {
				aliasTarget = filepath.Join(fx.base, "allowed", "unrelated-installation")
				if err := os.MkdirAll(aliasTarget, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(aliasTarget, alias); err != nil {
				t.Fatal(err)
			}
			registered := alias
			if scenario == "missing-boundary" {
				registered = filepath.Join(fx.base, "allowed", "missing-installation")
			}
			registry, _ := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{"demo@fixture": []any{map[string]any{"scope": "user", "installPath": registered, "version": "1"}}}})
			registryPath := filepath.Join(fx.home, ".claude/plugins/installed_plugins.json")
			writeFile(t, registryPath, registry, 0600)
			if scenario == "inaccessible" {
				if err := os.Chmod(container, 0000); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(container, 0700)
				if _, err := os.Stat(alias); !os.IsPermission(err) {
					t.Fatalf("fixture does not establish actual permission failure: %v", err)
				}
			}
			environment := []string{"CLAUDE_CONFIG_DIR=" + filepath.Join(fx.base, "allowed", "separate-claude")}
			list := ale40EnvironmentChange(t, fx, environment, "list", "--root", target, "--json")
			assertExit(t, list, 0)
			var inventory collection.View
			decodeJSON(t, list.stdout, &inventory)
			if len(inventory.Sources) != 1 {
				t.Fatal(inventory.Sources)
			}
			id := string(inventory.Sources[0].ID)
			args := []string{"delete", "--root", target, "--project", fx.project, "--json"}
			before := treeManifest(t, target)
			preview := ale40EnvironmentChange(t, fx, environment, append(args, id)...)
			assertExit(t, preview, 1)
			var view collection.DeleteView
			decodeJSON(t, preview.stdout, &view)
			assertTreeUnchanged(t, before, treeManifest(t, target))
			receipt := map[string]any{"scenario": scenario, "state": view.State, "reason": view.Reason, "target": target}
			if scenario == "stale-relation" {
				if view.State != "ready" {
					t.Fatalf("unrelated accessible boundary must permit preview: %+v", view)
				}
				if err := os.Chmod(container, 0000); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(container, 0700)
				if _, err := os.Stat(alias); !os.IsPermission(err) {
					t.Fatalf("relation mutation must establish EACCES: %v", err)
				}
				fresh := ale40EnvironmentChange(t, fx, environment, append(args, id)...)
				assertExit(t, fresh, 1)
				var current collection.DeleteView
				decodeJSON(t, fresh.stdout, &current)
				if current.State != "refused" || current.Confirmation != "" {
					t.Fatalf("unresolved boundary granted consent: %+v", current)
				}
			}
			if view.State == "ready" {
				applied := ale40EnvironmentChange(t, fx, environment, append(append(args, "--confirm", view.Confirmation), id)...)
				var outcome collection.DeleteOutcome
				decodeJSON(t, applied.stdout, &outcome)
				receipt["apply_exit"] = applied.code
				receipt["outcome"] = outcome.State
				receipt["verified"] = outcome.Verified
				if scenario == "stale-relation" {
					assertExit(t, applied, 1)
					if outcome.State != "stale" || len(outcome.Removed) != 0 {
						t.Fatalf("changed relation must stale without removal: %+v", outcome)
					}
					assertTreeUnchanged(t, before, treeManifest(t, target))
				} else {
					assertExit(t, applied, 0)
					if outcome.State != "removed" || !outcome.Verified {
						t.Fatalf("healthy missing boundary control failed: %+v", outcome)
					}
				}
			}
			_, targetErr := os.Lstat(target)
			receipt["target_survives"] = targetErr == nil
			receipt["registry_survives_unchanged"] = bytes.Equal(readBytes(t, registryPath), registry)
			receipt["independent_sentinel"] = string(readBytes(t, sentinel))
			if scenario == "inaccessible" || scenario == "stale-relation" {
				if err := os.Chmod(container, 0700); err != nil {
					t.Fatal(err)
				}
			}
			aliasInfo, aliasErr := os.Lstat(alias)
			receipt["alias_survives"] = aliasErr == nil && aliasInfo.Mode()&os.ModeSymlink != 0
			raw, _ := json.MarshalIndent(receipt, "", "  ")
			t.Log(string(raw))
			if root := os.Getenv("SKILLGLASS_EVIDENCE"); root != "" {
				dir := filepath.Join(root, "alias-proof")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, scenario+".json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := "refused"
			if scenario == "missing-boundary" || scenario == "stale-relation" {
				want = "ready"
			}
			if view.State != want {
				t.Errorf("want %s, got %s: %s", want, view.State, view.Reason)
			}
			if scenario != "missing-boundary" && targetErr != nil {
				t.Error("refused or stale target must survive")
			}
			if scenario == "missing-boundary" && !os.IsNotExist(targetErr) {
				t.Error("confirmed healthy target must be absent")
			}
			if !bytes.Equal(readBytes(t, registryPath), registry) || string(readBytes(t, sentinel)) != "preserve" || aliasErr != nil || aliasInfo.Mode()&os.ModeSymlink == 0 {
				t.Fatal("registry, independent sentinel or alias changed")
			}
		})
	}
}
