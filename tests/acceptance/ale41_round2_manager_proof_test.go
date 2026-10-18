package acceptance_test

import (
	"encoding/json"
	"github.com/alexdancer/skillglass/internal/collection"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE41Round2WholeManagerBoundary(t *testing.T) {
	for _, app := range []string{"codex", "claude", "gemini", "hermes", "pi", "opencode"} {
		for _, alias := range []bool{false, true} {
			t.Run(app+map[bool]string{false: "/canonical", true: "/case-alias"}[alias], func(t *testing.T) {
				fx := newFixture(t)
				env := []string{}
				var install string
				switch app {
				case "codex":
					install = filepath.Join(fx.home, ".codex/plugins/cache/fixture/demo/VersionOne")
					writeFile(t, filepath.Join(install, ".codex-plugin/plugin.json"), []byte(`{"name":"demo","version":"VersionOne","skills":"skills"}`), 0600)
				case "claude":
					install = filepath.Join(fx.base, "allowed", "ClaudeInstall")
					config := filepath.Join(fx.base, "allowed", "separate-claude")
					env = append(env, "CLAUDE_CONFIG_DIR="+config)
					registry, _ := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{"demo@fixture": []any{map[string]any{"scope": "user", "installPath": install, "version": "1"}}}})
					writeFile(t, filepath.Join(config, "plugins/installed_plugins.json"), registry, 0600)
					writeFile(t, filepath.Join(install, ".claude-plugin/plugin.json"), []byte(`{"name":"demo","version":"1","skills":["skills"]}`), 0600)
				case "gemini":
					install = filepath.Join(fx.home, ".gemini/extensions/DemoInstall")
					writeFile(t, filepath.Join(install, "gemini-extension.json"), []byte(`{"name":"demo","version":"1"}`), 0600)
				case "hermes":
					install = filepath.Join(fx.home, ".hermes/plugins/DemoInstall")
					writeFile(t, filepath.Join(install, "plugin.yaml"), []byte("name: demo\nversion: '1'\n"), 0600)
				case "pi":
					install = filepath.Join(fx.home, ".pi/agent/git/fixture/DemoInstall")
				case "opencode":
					install = filepath.Join(fx.xdgCache, "opencode/DemoInstall")
				}
				codexSkill(t, filepath.Join(install, "skills"), "native", "manager-native")
				manifest := codexSkill(t, filepath.Join(install, "other-bundles"), "target", "managed-unlisted")
				selected := filepath.Dir(manifest)
				if alias {
					selected = strings.Replace(selected, filepath.Base(install), strings.ToUpper(filepath.Base(install)), 1)
					a, e := os.Stat(selected)
					b, f := os.Stat(filepath.Dir(manifest))
					if e != nil || f != nil || !os.SameFile(a, b) {
						t.Skip("filesystem has no case aliases")
					}
				}
				before := treeManifest(t, fx.base)
				list := ale40EnvironmentChange(t, fx, env, "list", "--root", selected, "--json")
				assertExit(t, list, 0)
				var v collection.View
				decodeJSON(t, list.stdout, &v)
				if len(v.Sources) != 1 {
					t.Fatal(v.Sources)
				}
				r := ale40EnvironmentChange(t, fx, env, "delete", "--root", selected, "--project", fx.project, "--json", string(v.Sources[0].ID))
				assertExit(t, r, 1)
				var p collection.DeleteView
				decodeJSON(t, r.stdout, &p)
				t.Logf("app=%s alias=%v source=%s ownership=%+v preview=%s reason=%s confirmation=%v", app, alias, v.Sources[0].Directory, v.Sources[0].Ownership, p.State, p.Reason, p.Confirmation != "")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if p.State != "refused" || p.Confirmation != "" {
					t.Errorf("whole registered manager installation must refuse preview, got %s %s", p.State, p.Reason)
				}
			})
		}
	}
}
