package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41CodeAFManagerBoundaries(t *testing.T) {
	for _, scope := range []string{"user", "project", "other-project", "unknown-scope"} {
		for _, topology := range []string{"case-native", "case-unlisted", "unlisted", "external-link"} {
			t.Run(scope+"/"+topology, func(t *testing.T) {
				fx := newFixture(t)
				install := filepath.Join(fx.base, "allowed", "PluginInstall")
				root := filepath.Join(install, "skills")
				if topology == "case-unlisted" || topology == "unlisted" {
					root = filepath.Join(install, "other-bundles")
				}
				if topology == "external-link" {
					root = filepath.Join(fx.base, "allowed", "external-skills")
				}
				manifest := codexSkill(t, root, "target", "manager-boundary")
				writeFile(t, filepath.Join(install, ".claude-plugin/plugin.json"), []byte(`{"name":"demo","skills":["skills"]}`), 0600)
				if topology == "external-link" {
					if err := os.Symlink(root, filepath.Join(install, "skills")); err != nil {
						t.Fatal(err)
					}
				}
				registrationScope, project := scope, fx.project
				if scope == "other-project" {
					registrationScope, project = "project", fx.project+"-other"
				}
				registry, err := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{"demo@fixture": []any{map[string]any{"scope": registrationScope, "installPath": install, "projectPath": project, "version": "1"}}}})
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"), registry, 0600)
				writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte(`{"enabledPlugins":{"demo@fixture":false}}`), 0600)
				selected := filepath.Dir(manifest)
				if topology == "case-native" || topology == "case-unlisted" {
					selected = filepath.Join(fx.base, "allowed", "PLUGININSTALL", filepath.Base(root), "target")
					actual, err := os.Stat(filepath.Dir(manifest))
					if err != nil {
						t.Fatal(err)
					}
					alias, err := os.Stat(selected)
					if err != nil || !os.SameFile(actual, alias) {
						t.Skip("fixture filesystem does not support case aliases")
					}
				}
				environment := []string{"CLAUDE_CONFIG_DIR=" + filepath.Join(fx.base, "allowed", "separate-claude")}
				list := ale40EnvironmentChange(t, fx, environment, "list", "--root", selected, "--json")
				assertExit(t, list, 0)
				var inventory collection.View
				decodeJSON(t, list.stdout, &inventory)
				if len(inventory.Sources) != 1 {
					t.Fatal("expected exact selected bundle", inventory.Sources)
				}
				id := string(inventory.Sources[0].ID)
				args := []string{"delete", "--root", selected, "--project", fx.project, "--json"}
				before := treeManifest(t, fx.base)
				preview := ale40EnvironmentChange(t, fx, environment, append(args, id)...)
				assertExit(t, preview, 1)
				var p collection.DeleteView
				decodeJSON(t, preview.stdout, &p)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if topology != "external-link" {
					if p.State != "refused" || p.Confirmation != "" {
						t.Fatalf("registered installation permits deletion: state=%s reason=%s", p.State, p.Reason)
					}
					return
				}
				if p.State != "ready" {
					t.Fatal("external plugin reference must not imply manager ownership", p.State, p.Reason)
				}
				result := ale40EnvironmentChange(t, fx, environment, append(append(args, "--confirm", p.Confirmation), id)...)
				assertExit(t, result, 0)
				if _, err := os.Lstat(filepath.Dir(manifest)); !os.IsNotExist(err) {
					t.Fatal("selected external bundle survives", err)
				}
				if info, err := os.Lstat(filepath.Join(install, "skills")); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatal("plugin reference changed", err)
				}
				if string(readBytes(t, filepath.Join(fx.home, ".claude/plugins/installed_plugins.json"))) != string(registry) {
					t.Fatal("registration changed")
				}
			})
		}
	}
}

func TestALE41UnrelatedDiagnosticNames(t *testing.T) {
	for i, name := range []string{"plugin", "package", "registry", "managed"} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.home, ".codex/skills"), "home-canary", "healthy-canary")
			root := filepath.Join(fx.base, "allowed", "selected")
			manifest := codexSkill(t, root, "target", "diagnostic-name")
			hidden := filepath.Join(fx.base, "allowed", "unreadable-"+name+"-notes")
			if err := os.MkdirAll(hidden, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(hidden, 0000); err != nil {
				t.Fatal(err)
			}
			defer os.Chmod(hidden, 0700)
			p, id := findingCustomPreview(t, fx, root)
			args := []string{"delete", "--root", root, "--root", hidden, "--json"}
			before := treeManifest(t, fx.home)
			r := changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var limited collection.DeleteView
			decodeJSON(t, r.stdout, &limited)
			if limited.State != "ready" {
				t.Fatal("unrelated diagnostic blocks deletion", limited.State, limited.Reason)
			}
			found := false
			for _, consumer := range limited.Consumers {
				for _, observed := range consumer.Scope.Roots {
					if observed.Requested == hidden && observed.State == collection.RootUnreadable && consumer.Scope.Completeness == collection.Incomplete {
						found = true
					}
				}
			}
			if !found || limited.Confirmation == p.Confirmation {
				t.Fatal("discovery limit must be disclosed and sealed")
			}
			stale := changeCommand(t, fx, "", append(append(args, "--confirm", p.Confirmation), id)...)
			assertExit(t, stale, 1)
			if _, err := os.Stat(manifest); err != nil {
				t.Fatal("stale confirmation changed target", err)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.home))
			r = changeCommand(t, fx, "", append(append(args, "--confirm", limited.Confirmation), id)...)
			assertExit(t, r, 0)
			if _, err := os.Lstat(filepath.Dir(manifest)); !os.IsNotExist(err) {
				t.Fatal("selected bundle survives", err)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.home))
		})
	}
}
