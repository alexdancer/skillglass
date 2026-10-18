package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41PiOwnershipReaderCoverage(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		for _, scenario := range []string{"absent", "empty", "packages-null", "packages-scalar", "record-null", "source-missing", "source-null", "skills-invalid", "autoload-invalid", "skills-invalid-unrelated", "autoload-invalid-unrelated", "partial", "unreadable", "oversized", "activation-only", "unknown-source", "valid-package", "manifest-absent", "manifest-null", "manifest-pi-null", "manifest-skills-null", "manifest-partial", "manifest-oversized", "bounded-unrelated", "external-resource"} {
			t.Run(scope+"/"+scenario, func(t *testing.T) {
				fx, _ := codexFixture(t)
				pkg := filepath.Join(fx.base, "allowed", "package")
				target := filepath.Dir(codexSkill(t, filepath.Join(pkg, "resources"), "target", "reader-target"))
				if scenario == "bounded-unrelated" || scenario == "external-resource" || strings.HasSuffix(scenario, "-invalid-unrelated") {
					target = filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "independent"), "target", "reader-target"))
				}
				settings := filepath.Join(fx.home, ".pi/agent/settings.json")
				if scope == "project" {
					settings = filepath.Join(fx.project, ".pi/settings.json")
				}
				manifest := filepath.Join(pkg, "package.json")
				claudeSettings(t, manifest, map[string]any{"pi": map[string]any{"skills": []string{"resources"}}})
				var document any = map[string]any{"packages": []any{pkg}}
				want := "refused"
				switch scenario {
				case "absent":
					document = nil
					want = "ready"
				case "empty":
					document = map[string]any{"packages": []any{}}
					want = "ready"
				case "packages-null":
					document = map[string]any{"packages": nil}
				case "packages-scalar":
					document = map[string]any{"packages": true}
				case "record-null":
					document = map[string]any{"packages": []any{nil}}
				case "source-missing":
					document = map[string]any{"packages": []any{map[string]any{"autoload": true}}}
				case "source-null":
					document = map[string]any{"packages": []any{map[string]any{"source": nil}}}
				case "skills-invalid", "skills-invalid-unrelated":
					document = map[string]any{"packages": []any{map[string]any{"source": pkg, "skills": nil}}}
				case "autoload-invalid", "autoload-invalid-unrelated":
					document = map[string]any{"packages": []any{map[string]any{"source": pkg, "autoload": "invalid"}}}
				case "partial":
					document = map[string]any{"packages": []any{pkg, nil}}
				case "activation-only":
					document = map[string]any{"packages": []any{}, "enableSkillCommands": "invalid", "defaultProjectTrust": nil, "skills": 17}
					want = "ready"
				case "unknown-source":
					document = map[string]any{"packages": []any{"https://fixture.invalid/package"}}
				case "manifest-absent":
					if err := os.Remove(manifest); err != nil {
						t.Fatal(err)
					}
					want = "ready" // Missing manifest uses the supported default skills root, not resources.
				case "manifest-null":
					writeFile(t, manifest, []byte("null"), 0600)
				case "manifest-pi-null":
					writeFile(t, manifest, []byte(`{"pi":null}`), 0600)
				case "manifest-skills-null":
					writeFile(t, manifest, []byte(`{"pi":{"skills":null}}`), 0600)
				case "manifest-partial":
					writeFile(t, manifest, []byte(`{"pi":{"skills":["resources",null]}}`), 0600)
				case "manifest-oversized":
					writeFile(t, manifest, []byte(strings.Repeat(" ", 4<<20)+"{}"), 0600)
				case "bounded-unrelated":
					writeFile(t, manifest, []byte(`{"pi":`), 0600)
					want = "ready"
				case "external-resource":
					if err := os.Symlink(filepath.Dir(target), filepath.Join(pkg, "linked")); err != nil {
						t.Fatal(err)
					}
					writeFile(t, manifest, []byte(`{"pi":{"skills":["linked"]}}`), 0600)
					want = "ready"
				}
				if strings.HasSuffix(scenario, "-invalid-unrelated") {
					want = "ready"
				}
				if document != nil {
					claudeSettings(t, settings, document)
				}
				if scenario == "unreadable" {
					if err := os.Chmod(settings, 0000); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "oversized" {
					writeFile(t, settings, []byte(strings.Repeat(" ", 4<<20)+"{}"), 0600)
				}
				assertOwnershipPreview(t, fx, nil, target, want, "pi-settings")
			})
		}
	}
}

// This crosses the public list/preview/apply boundary and verifies every synthetic
// file after refusal or a successful unrelated-target removal.
func assertOwnershipPreview(t *testing.T, fx fixture, environment []string, target, want, reader string) {
	t.Helper()
	list := ale40EnvironmentChange(t, fx, environment, "list", "--root", target, "--json")
	assertExit(t, list, 0)
	var inventory collection.View
	decodeJSON(t, list.stdout, &inventory)
	if len(inventory.Sources) != 1 {
		t.Fatal(inventory.Sources)
	}
	id := string(inventory.Sources[0].ID)
	args := []string{"delete", "--root", target, "--project", fx.project, "--json"}
	before := treeManifest(t, fx.base)
	r := ale40EnvironmentChange(t, fx, environment, append(args, id)...)
	assertExit(t, r, 1)
	var p collection.DeleteView
	decodeJSON(t, r.stdout, &p)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if p.State != want {
		t.Fatalf("want %s, got %s: %s", want, p.State, p.Reason)
	}
	found := false
	for _, consumer := range p.Consumers {
		for _, fact := range consumer.Managers {
			if fact.Reader == reader {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing explicit reader coverage", reader)
	}
	if want == "refused" {
		if p.Confirmation != "" {
			t.Fatal("refusal permits confirmation")
		}
		return
	}
	r = ale40EnvironmentChange(t, fx, environment, append(append(args, "--confirm", p.Confirmation), id)...)
	assertExit(t, r, 0)
	var out collection.DeleteOutcome
	decodeJSON(t, r.stdout, &out)
	if out.State != "removed" || !out.Verified {
		t.Fatal(out.State, out.Reason)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("selected target survived", err)
	}
	var oldEntries, newEntries []treeEntry
	decodeJSON(t, before, &oldEntries)
	decodeJSON(t, treeManifest(t, fx.base), &newEntries)
	remaining := map[string]treeEntry{}
	for _, entry := range newEntries {
		remaining[entry.Path] = entry
	}
	selected, err := filepath.Rel(fx.base, target)
	if err != nil {
		t.Fatal(err)
	}
	selected = filepath.ToSlash(selected)
	for _, entry := range oldEntries {
		if entry.Path == selected || strings.HasPrefix(entry.Path, selected+"/") {
			continue
		}
		actual, ok := remaining[entry.Path]
		if !ok || actual.Mode != entry.Mode || actual.SHA256 != entry.SHA256 || actual.Target != entry.Target {
			t.Fatal("independent content changed", entry.Path)
		}
	}

}

func TestALE41ClaudeOwnershipReaderCoverage(t *testing.T) {
	for _, scenario := range []string{"registry-absent", "registry-empty", "registry-null", "record-missing-source", "record-null", "discriminator-missing", "unknown-discriminator", "remote-github", "remote-git", "remote-url", "remote-npm", "partial-record", "unreadable-registry", "manifest-absent", "manifest-null", "manifest-empty", "manifest-record-null", "manifest-partial", "manifest-unreadable", "bounded-unrelated", "external-resource"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newFixture(t)
			config := filepath.Join(fx.base, "allowed", "claude-config")
			market := filepath.Join(fx.base, "allowed", "market")
			plugin := filepath.Join(market, "demo")
			target := filepath.Dir(codexSkill(t, filepath.Join(plugin, "skills"), "target", "reader-target"))
			if scenario == "bounded-unrelated" || scenario == "external-resource" {
				target = filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "independent"), "target", "reader-target"))
			}
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "skills": "./skills"})
			registry := filepath.Join(config, "plugins/known_marketplaces.json")
			manifest := filepath.Join(market, ".claude-plugin/marketplace.json")
			record := map[string]any{"source": map[string]any{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-10-13T00:00:00Z"}
			document := map[string]any{"market": record}
			claudeSettings(t, manifest, map[string]any{"name": "market", "plugins": []any{map[string]any{"name": "demo", "source": "./demo"}}})
			want := "refused"
			switch scenario {
			case "registry-absent":
				document = nil
				want = "ready"
			case "registry-empty":
				document = map[string]any{}
				want = "ready"
			case "registry-null":
				document = nil
			case "record-missing-source":
				delete(record, "source")
			case "record-null":
				document["market"] = nil
			case "discriminator-missing":
				record["source"] = map[string]any{"path": market}
			case "unknown-discriminator":
				record["source"] = map[string]any{"source": "unknown", "path": market}
			case "partial-record":
				document["other"] = nil
			case "remote-github", "remote-git", "remote-url", "remote-npm":
				record["source"] = map[string]any{"source": strings.TrimPrefix(scenario, "remote-")}
				want = "ready"
			case "manifest-absent":
				if err := os.Remove(manifest); err != nil {
					t.Fatal(err)
				}
			case "manifest-null":
				writeFile(t, manifest, []byte("null"), 0600)
			case "manifest-empty":
				writeFile(t, manifest, []byte(`{"name":"market","plugins":[]}`), 0600)
				want = "ready"
			case "manifest-record-null":
				writeFile(t, manifest, []byte(`{"name":"market","plugins":[null]}`), 0600)
			case "manifest-partial":
				writeFile(t, manifest, []byte(`{"name":"market","plugins":[{"name":"demo","source":"./demo"},null]}`), 0600)
			case "manifest-unreadable":
				if err := os.Chmod(manifest, 0000); err != nil {
					t.Fatal(err)
				}
			case "bounded-unrelated":
				writeFile(t, manifest, []byte(`{"name":`), 0600)
				want = "ready"
			case "external-resource":
				if err := os.RemoveAll(filepath.Join(plugin, "skills")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(target), filepath.Join(plugin, "skills")); err != nil {
					t.Fatal(err)
				}
				want = "ready"
			}
			if document != nil {
				claudeSettings(t, registry, document)
			}
			if scenario == "registry-null" {
				writeFile(t, registry, []byte("null"), 0600)
			}
			if scenario == "unreadable-registry" {
				if err := os.Chmod(registry, 0000); err != nil {
					t.Fatal(err)
				}
			}
			assertOwnershipPreview(t, fx, []string{"CLAUDE_CONFIG_DIR=" + config}, target, want, "claude-marketplaces")
		})
	}
}

func TestALE41OwnershipReaderStaleConsent(t *testing.T) {
	for _, reader := range []string{"pi-settings-global", "pi-settings-project", "pi-manifest", "claude-registry", "claude-manifest"} {
		transitions := []string{"valid-to-malformed"}
		if reader == "pi-manifest" || reader == "claude-manifest" {
			transitions = append(transitions, "malformed-to-empty")
		}
		for _, transition := range transitions {
			t.Run(reader+"/"+transition, func(t *testing.T) {
				fx := newFixture(t)
				root := filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "selected"), "target", "stale-reader"))
				var input string
				valid, empty := []byte(`{"packages":[]}`), []byte(`{}`)
				env := []string{}
				switch reader {
				case "pi-settings-global":
					input = filepath.Join(fx.home, ".pi/agent/settings.json")
				case "pi-settings-project":
					input = filepath.Join(fx.project, ".pi/settings.json")
				case "pi-manifest":
					pkg := filepath.Join(fx.base, "allowed", "pkg")
					claudeSettings(t, filepath.Join(fx.home, ".pi/agent/settings.json"), map[string]any{"packages": []string{pkg}})
					input = filepath.Join(pkg, "package.json")
					valid = []byte(`{"pi":{"skills":[]}}`)
					empty = valid
				case "claude-registry":
					input = filepath.Join(fx.home, ".claude/plugins/known_marketplaces.json")
					valid = []byte(`{}`)
					empty = valid
				case "claude-manifest":
					market := filepath.Join(fx.base, "allowed", "market")
					claudeSettings(t, filepath.Join(fx.home, ".claude/plugins/known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]any{"source": "directory", "path": market}, "installLocation": market}})
					input = filepath.Join(market, ".claude-plugin/marketplace.json")
					valid = []byte(`{"name":"market","plugins":[]}`)
					empty = valid
				}
				raw := valid
				if transition == "malformed-to-empty" {

					raw = []byte("{")
				}
				writeFile(t, input, raw, 0600)
				list := ale40EnvironmentChange(t, fx, env, "list", "--root", root, "--json")
				assertExit(t, list, 0)
				var inventory collection.View
				decodeJSON(t, list.stdout, &inventory)
				id := string(inventory.Sources[0].ID)
				args := []string{"delete", "--root", root, "--project", fx.project, "--json"}
				r := ale40EnvironmentChange(t, fx, env, append(args, id)...)
				assertExit(t, r, 1)
				var p collection.DeleteView
				decodeJSON(t, r.stdout, &p)
				if p.State != "ready" {
					t.Fatal(p.State, p.Reason)
				}
				changed := []byte("{")
				if transition == "malformed-to-empty" {
					changed = empty
				}
				writeFile(t, input, changed, 0600)
				before := treeManifest(t, fx.base)
				r = ale40EnvironmentChange(t, fx, env, append(append(args, "--confirm", p.Confirmation), id)...)
				assertExit(t, r, 1)
				var outcome collection.DeleteOutcome
				decodeJSON(t, r.stdout, &outcome)
				if outcome.State != "stale" && outcome.State != "refused" {
					t.Fatal(outcome.State, outcome.Reason)
				}
				if len(outcome.Removed) != 0 {
					t.Fatal("stale consent removed content")
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if string(readBytes(t, input)) != string(changed) {
					t.Fatal("manager input changed")
				}
			})
		}
	}
}
