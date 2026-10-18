package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41ManagerReferencesDoNotOwnExternalTargets(t *testing.T) {
	for _, app := range []string{"codex", "claude", "gemini"} {
		t.Run(app, func(t *testing.T) {
			fx, _ := codexFixture(t)
			env := []string{}
			var install string
			switch app {
			case "codex":
				install = filepath.Join(fx.home, ".codex/plugins/cache/fixture/demo/1")
				writeFile(t, filepath.Join(install, ".codex-plugin/plugin.json"), []byte(`{"name":"demo","version":"1","skills":"skills"}`), 0600)
			case "claude":
				install = filepath.Join(fx.base, "allowed", "ClaudeInstall")
				config := filepath.Join(fx.base, "allowed", "separate-claude")
				env = append(env, "CLAUDE_CONFIG_DIR="+config)
				registry, err := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{"demo@fixture": []any{map[string]any{"scope": "user", "installPath": install, "version": "1"}}}})
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(config, "plugins/installed_plugins.json"), registry, 0600)
				writeFile(t, filepath.Join(install, ".claude-plugin/plugin.json"), []byte(`{"name":"demo","version":"1","skills":["./skills"]}`), 0600)
			case "gemini":
				install = filepath.Join(fx.home, ".gemini/extensions/DemoInstall")
				writeFile(t, filepath.Join(install, "gemini-extension.json"), []byte(`{"name":"demo","version":"1"}`), 0600)
			}
			root := filepath.Join(fx.base, "allowed", "external")
			manifest := codexSkill(t, root, "target", "external-target")
			if err := os.Symlink(root, filepath.Join(install, "skills")); err != nil {
				t.Fatal(err)
			}
			list := ale40EnvironmentChange(t, fx, env, "list", "--root", filepath.Dir(manifest), "--json")
			assertExit(t, list, 0)
			var inventory collection.View
			decodeJSON(t, list.stdout, &inventory)
			id := string(inventory.Sources[0].ID)
			args := []string{"delete", "--root", filepath.Dir(manifest), "--project", fx.project, "--json"}
			before := treeManifest(t, install)
			r := ale40EnvironmentChange(t, fx, env, append(args, id)...)
			assertExit(t, r, 1)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if p.State != "ready" {
				t.Fatal("external reference was mistaken for manager ownership", p.State, p.Reason)
			}
			r = ale40EnvironmentChange(t, fx, env, append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, r, 0)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			if out.State != "removed" || !out.Verified {
				t.Fatal(out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, install))
			if _, err := os.Lstat(filepath.Dir(manifest)); !os.IsNotExist(err) {
				t.Fatal("selected external target survives", err)
			}
		})
	}
}
