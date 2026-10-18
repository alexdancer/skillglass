package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func ale41RegistryFault(t *testing.T, path, scenario string, valid []byte) {
	t.Helper()
	data := valid
	switch scenario {
	case "absent":
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		return
	case "empty":
		data = []byte(`{"version":2,"plugins":{}}`)
	case "invalid-json":
		data = []byte("{invalid")
	case "invalid-schema":
		data = []byte(`{"plugins":"invalid"}`)
	case "missing-plugins":
		data = []byte(`{}`)
	case "null":
		data = []byte(`null`)
	case "null-plugins":
		data = []byte(`{"plugins":null}`)
	case "null-record":
		data = []byte(`{"plugins":{"demo":null}}`)
	case "unknown-schema":
		data = []byte(`{"plugins":{},"installations":[]}`)
	case "unknown-version":
		data = []byte(`{"version":99,"plugins":{}}`)
	case "oversized":
		data = append(bytes.Repeat([]byte(" "), 1<<20), valid...)
	}
	writeFile(t, path, data, 0600)
	if scenario == "unreadable" {
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	}
}

func TestALE41RegistryUncertainty(t *testing.T) {
	for _, app := range []string{"codeaf", "claude-code"} {
		for _, scenario := range []string{"absent", "empty", "valid", "unreadable", "invalid-json", "invalid-schema", "missing-plugins", "null", "null-plugins", "null-record", "unknown-schema", "unknown-version", "oversized"} {
			t.Run(app+"/"+scenario, func(t *testing.T) {
				fx, _ := codexFixture(t)
				manifest, settings := codeafPluginFixture(t, fx)
				registry := filepath.Join(fx.home, ".claude/plugins/installed_plugins.json")
				valid := readBytes(t, registry)
				config := filepath.Join(fx.base, "allowed", "separate-claude")
				if app == "claude-code" {
					if err := os.Remove(registry); err != nil {
						t.Fatal(err)
					}
					registry = filepath.Join(config, "plugins/installed_plugins.json")
					var err error
					valid, err = json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{"example@local": []any{map[string]any{"scope": "user", "installPath": filepath.Dir(filepath.Dir(filepath.Dir(manifest)))}}}})
					if err != nil {
						t.Fatal(err)
					}
					writeFile(t, registry, valid, 0600)
				}
				ale41RegistryFault(t, registry, scenario, valid)
				env := []string{"CLAUDE_CONFIG_DIR=" + config}
				root := filepath.Dir(filepath.Dir(manifest))
				list := ale40EnvironmentChange(t, fx, env, "list", "--root", root, "--json")
				assertExit(t, list, 0)
				var inventory collection.View
				decodeJSON(t, list.stdout, &inventory)
				if len(inventory.Sources) != 1 {
					t.Fatal(inventory.Sources)
				}
				id := string(inventory.Sources[0].ID)
				args := []string{"delete", "--root", root, "--project", fx.project, "--json"}
				before, settingsBefore := treeManifest(t, root), readBytes(t, settings)
				r := ale40EnvironmentChange(t, fx, env, append(args, id)...)
				assertExit(t, r, 1)
				var p collection.DeleteView
				decodeJSON(t, r.stdout, &p)
				want, coverage := "refused", collection.ManagerUncertain
				if scenario == "absent" || scenario == "empty" {
					want, coverage = "ready", collection.ManagerObserved
					if scenario == "absent" {
						coverage = collection.ManagerAbsent
					}
				} else if scenario == "valid" {
					coverage = collection.ManagerBoundary
				}
				if p.State != want {
					t.Fatalf("scenario=%s want=%s got=%s reason=%s", scenario, want, p.State, p.Reason)
				}
				found := false
				for _, c := range p.Consumers {
					if c.Application != app {
						continue
					}
					for _, manager := range c.Managers {
						if manager.Evidence.InputPath == registry && manager.State == coverage {
							found = true
						}
					}
				}
				if !found {
					t.Fatal("actual registry coverage not represented", coverage)
				}
				assertTreeUnchanged(t, before, treeManifest(t, root))
				if !bytes.Equal(settingsBefore, readBytes(t, settings)) {
					t.Fatal("settings changed")
				}
				if want == "refused" {
					if p.Confirmation != "" {
						t.Fatal("refused preview grants consent")
					}
					return
				}
				r = ale40EnvironmentChange(t, fx, env, append(append(args, "--confirm", p.Confirmation), id)...)
				assertExit(t, r, 0)
				var out collection.DeleteOutcome
				decodeJSON(t, r.stdout, &out)
				if out.State != "removed" || !out.Verified {
					t.Fatal(out)
				}
				if _, err := os.Lstat(filepath.Dir(manifest)); !os.IsNotExist(err) {
					t.Fatal("healthy registry control did not delete selected content", err)
				}
				if !bytes.Equal(settingsBefore, readBytes(t, settings)) {
					t.Fatal("settings changed on apply")
				}
			})
		}
	}
}

func TestALE41RegistryChangesInvalidateConsent(t *testing.T) {
	for _, scenario := range []string{"empty", "valid", "unreadable", "invalid-json", "invalid-schema", "missing-plugins", "null", "null-plugins", "null-record", "unknown-schema", "unknown-version", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			fx, _ := codexFixture(t)
			manifest, settings := codeafPluginFixture(t, fx)
			root := filepath.Dir(filepath.Dir(manifest))
			registry := filepath.Join(fx.home, ".claude/plugins/installed_plugins.json")
			valid := readBytes(t, registry)
			ale41RegistryFault(t, registry, "absent", valid)
			env := []string{"CLAUDE_CONFIG_DIR=" + filepath.Join(fx.base, "allowed", "separate-claude")}
			list := ale40EnvironmentChange(t, fx, env, "list", "--root", root, "--json")
			var inventory collection.View
			decodeJSON(t, list.stdout, &inventory)
			id := string(inventory.Sources[0].ID)
			args := []string{"delete", "--root", root, "--project", fx.project, "--json"}
			r := ale40EnvironmentChange(t, fx, env, append(args, id)...)
			var p collection.DeleteView
			decodeJSON(t, r.stdout, &p)
			if p.State != "ready" {
				t.Fatal(p.State, p.Reason)
			}
			ale41RegistryFault(t, registry, scenario, valid)
			before, settingsBefore := treeManifest(t, root), readBytes(t, settings)
			r = ale40EnvironmentChange(t, fx, env, append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, r, 1)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			if out.State != "stale" || len(out.Removed) != 0 || out.Verified {
				t.Fatal(out.State, out.Reason, out.Removed)
			}
			assertTreeUnchanged(t, before, treeManifest(t, root))
			if !bytes.Equal(settingsBefore, readBytes(t, settings)) {
				t.Fatal("stale apply changed settings")
			}
		})
	}
}
