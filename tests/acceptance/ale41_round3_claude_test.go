package acceptance_test

import (
	"encoding/json"
	"github.com/alexdancer/skillglass/internal/collection"
	"os"
	"path/filepath"
	"testing"
)

func TestRound3ClaudeLocalInputProof(t *testing.T) {
	for _, scenario := range []string{"valid", "record-null", "source-null", "discriminator-null", "plugins-missing", "plugins-null", "registry-absent", "registry-empty", "plugins-empty"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newFixture(t)
			config := filepath.Join(fx.base, "allowed", "claude-config")
			market := filepath.Join(fx.base, "allowed", "external-market")
			plugin := filepath.Join(market, "demo")
			manifest := codexSkill(t, filepath.Join(plugin, "skills"), "target", "round3-target")
			selected := filepath.Dir(manifest)
			writeFile(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), []byte(`{"name":"demo","version":"1","skills":["./skills"]}`), 0600)
			registry := filepath.Join(config, "plugins/known_marketplaces.json")
			marketManifest := filepath.Join(market, ".claude-plugin/marketplace.json")
			record := map[string]any{"source": map[string]any{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-10-13T00:00:00Z"}
			claudeSettings(t, registry, map[string]any{"market": record})
			claudeSettings(t, marketManifest, map[string]any{"name": "market", "plugins": []map[string]string{{"name": "demo", "source": "./demo"}}})
			env := []string{"CLAUDE_CONFIG_DIR=" + config}
			list := ale40EnvironmentChange(t, fx, env, "list", "--root", selected, "--json")
			assertExit(t, list, 0)
			var inventory collection.View
			decodeJSON(t, list.stdout, &inventory)
			if len(inventory.Sources) != 1 {
				t.Fatal(inventory.Sources)
			}
			args := []string{"delete", "--root", selected, "--project", fx.project, "--json"}
			id := string(inventory.Sources[0].ID)
			preview := func() collection.DeleteView {
				r := ale40EnvironmentChange(t, fx, env, append(args, id)...)
				assertExit(t, r, 1)
				var p collection.DeleteView
				decodeJSON(t, r.stdout, &p)
				return p
			}
			initial := preview()
			if initial.State != "refused" || initial.Confirmation != "" {
				t.Fatal("valid registration did not refuse", initial.State, initial.Reason)
			}
			switch scenario {
			case "record-null":
				claudeSettings(t, registry, map[string]any{"market": nil})
			case "source-null":
				record["source"] = nil
				claudeSettings(t, registry, map[string]any{"market": record})
			case "discriminator-null":
				record["source"] = map[string]any{"source": nil, "path": market}
				claudeSettings(t, registry, map[string]any{"market": record})
			case "plugins-missing":
				claudeSettings(t, marketManifest, map[string]any{"name": "market"})
			case "plugins-null":
				claudeSettings(t, marketManifest, map[string]any{"name": "market", "plugins": nil})
			case "registry-empty":
				claudeSettings(t, registry, map[string]any{})
			case "plugins-empty":
				claudeSettings(t, marketManifest, map[string]any{"name": "market", "plugins": []any{}})
			case "registry-absent":
				if err := os.Remove(registry); err != nil {
					t.Fatal(err)
				}
			}
			before := treeManifest(t, fx.base)
			p := preview()
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			raw, _ := json.Marshal(p)
			t.Logf("scenario=%s initial=%s preview=%s confirmation=%v literal=%s", scenario, initial.State, p.State, p.Confirmation != "", raw)
			if scenario == "valid" {
				if p.State != "refused" {
					t.Error("valid registration must refuse")
				}
				return
			}
			if p.State == "ready" {
				r := ale40EnvironmentChange(t, fx, env, append(append(args, "--confirm", p.Confirmation), id)...)
				assertExit(t, r, 0)
				var out collection.DeleteOutcome
				decodeJSON(t, r.stdout, &out)
				_, err := os.Lstat(selected)
				t.Logf("scenario=%s apply=%s verified=%v selected_absent=%v registry_survives=%v marketplace_survives=%v", scenario, out.State, out.Verified, os.IsNotExist(err), fileExists(registry), fileExists(marketManifest))
				if out.State != "removed" || !out.Verified || !os.IsNotExist(err) {
					t.Fatal(out)
				}
			}
			want := "refused"
			if scenario == "registry-absent" || scenario == "registry-empty" || scenario == "plugins-empty" {
				want = "ready"
			}
			if p.State != want {
				t.Errorf("uncertain manager input must refuse; want=%s got=%s", want, p.State)
			}
		})
	}
}
func fileExists(path string) bool { _, err := os.Lstat(path); return err == nil }
