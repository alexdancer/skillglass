package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func piSettingsFile(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, raw, 0600)
}

func TestALE33FindingTraversal(t *testing.T) {
	for _, origin := range []string{"configured-bundle", "configured-container", "package", "cache-git", "cache-npm", "auto"} {
		for _, valid := range []bool{false, true} {
			t.Run(origin+"/"+map[bool]string{false: "ordinary", true: "valid"}[valid], func(t *testing.T) {
				fx := piFixture(t)
				root := filepath.Join(fx.base, "allowed", "selected")
				switch origin {
				case "cache-git":
					root = filepath.Join(fx.home, ".pi", "agent", "git", "owner", "repo")
				case "cache-npm":
					root = filepath.Join(fx.home, ".pi", "agent", "npm", "node_modules", "pkg")
				case "auto":
					root = filepath.Join(fx.home, ".pi", "agent", "skills")
				}
				manifest := codexSkill(t, root, "bundle", "bundle")
				note := filepath.Join(filepath.Dir(manifest), "references", "note.md")
				content := []byte("Ordinary supporting documentation.\n")
				if valid {
					content = []byte("---\nname: fabricated\ndescription: Supporting documentation.\n---\nNote.\n")
				}
				writeFile(t, note, content, 0600)
				selected := root
				if origin == "configured-bundle" {
					selected = filepath.Dir(manifest)
				}
				switch origin {
				case "configured-bundle", "configured-container":
					piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), map[string]any{"skills": []string{selected}})
				case "package":
					writeFile(t, filepath.Join(root, "package.json"), []byte(`{"pi":{"skills":["bundle"]}}`), 0600)
					piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), map[string]any{"packages": []string{root}})
				}
				before := treeManifest(t, fx.base)
				list := piList(t, fx, 0)
				if len(list.Sources) != 1 {
					t.Fatalf("supporting Markdown became a skill: %+v", list.Sources)
				}
				want := "available"
				if strings.HasPrefix(origin, "cache-") {
					want = "unavailable"
				}
				if list.Sources[0].Availability.State != want {
					t.Fatalf("state: %+v", list.Sources[0])
				}
				r := fx.run(t, "read", "--agent", "pi", "--project", fx.project, "--json", list.Sources[0].ID)
				assertExit(t, r, 0)
				var read publicRead
				decodeJSON(t, r.stdout, &read)
				assertResource(t, read.Source.Resources, "references/note.md", "file")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
	t.Run("independent-directory-and-explicit-file", func(t *testing.T) {
		fx := piFixture(t)
		root := filepath.Join(fx.base, "allowed", "selected")
		writeFile(t, filepath.Join(root, "direct.md"), []byte("---\nname: direct\ndescription: Direct.\n---\nBody.\n"), 0600)
		nested := filepath.Join(root, "nested", "note.md")
		writeFile(t, nested, []byte("---\nname: nested\ndescription: Nested.\n---\nBody.\n"), 0600)
		codexSkill(t, filepath.Join(root, "deep"), "bundle", "deep-bundle")
		settings := filepath.Join(fx.project, ".pi", "settings.json")
		piSettingsFile(t, settings, map[string]any{"skills": []string{root}})
		list := piList(t, fx, 0)
		if len(list.Sources) != 2 {
			t.Fatalf("native directory traversal: %+v", list.Sources)
		}
		piSettingsFile(t, settings, map[string]any{"skills": []string{root, nested}})
		if codexNamed(t, piList(t, fx, 0), "nested").Availability.State != "available" {
			t.Fatal("explicit nested file missing")
		}
		writeFile(t, filepath.Join(fx.project, ".agents", "skills", "folder", "note.md"), []byte("---\nname: agents-nested\ndescription: Agents.\n---\nBody.\n"), 0600)
		if codexNamed(t, piList(t, fx, 0), "agents-nested").Availability.State != "available" {
			t.Fatal("agents nested Markdown missing")
		}
	})
}

func TestALE33FindingGenericManifestControls(t *testing.T) {
	for _, format := range []struct {
		name, prefix, newline string
		valid                 bool
	}{{"bom", "\xef\xbb\xbf", "\n", false}, {"cr", "", "\r", false}, {"crlf", "", "\r\n", true}} {
		for _, agent := range []string{"generic", "codex"} {
			t.Run(format.name+"/"+agent, func(t *testing.T) {
				fx := piFixture(t)
				root := filepath.Join(fx.base, "allowed", "control")
				if agent == "codex" {
					root = filepath.Join(fx.home, ".codex", "skills")
					codexSkill(t, root, "home-canary", "home-canary")
				}
				path := filepath.Join(root, "bytes", "SKILL.md")
				raw := format.prefix + strings.ReplaceAll("---\nname: bytes\ndescription: Byte control.\n---\nBody.\n", "\n", format.newline)
				writeFile(t, path, []byte(raw), 0600)
				args := []string{"list", "--json"}
				if agent == "generic" {
					args = append(args, "--root", root)
				} else {
					args = append(args, "--agent", "codex")
				}
				r := fx.run(t, args...)
				wantExit := 1
				if format.valid {
					wantExit = 0
				}
				assertExit(t, r, wantExit)
				var list publicList
				decodeJSON(t, r.stdout, &list)
				for _, s := range list.Sources {
					if s.SourcePath == filepath.Dir(path) {
						want := "malformed"
						if format.valid {
							want = "parsed"
						}
						if s.Manifest.Status != want || s.Manifest.RawText != raw {
							t.Fatalf("generic parser changed: %+v", s.Manifest)
						}
						return
					}
				}
				t.Fatal("control source missing")
			})
		}
	}
}

func TestALE33FindingCanonicalSelection(t *testing.T) {
	for _, scenario := range []string{"first", "disabled-first", "project-rank", "inactive", "equal-content"} {
		t.Run(scenario, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.base, "allowed", "canonical")
			raw := []byte("---\ndescription: Shared instruction file.\n---\nBody.\n")
			target := filepath.Join(root, "instruction.md")
			writeFile(t, target, raw, 0600)
			a := filepath.Join(root, "A")
			b := filepath.Join(root, "B")
			if scenario == "inactive" {
				a = filepath.Join(fx.home, ".pi", "agent", "git", "A")
			}
			for _, dir := range []string{a, b} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if scenario == "equal-content" {
					writeFile(t, filepath.Join(dir, "SKILL.md"), raw, 0600)
				} else if err := os.Symlink(target, filepath.Join(dir, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}
			patterns := []string{a, b}
			if scenario == "disabled-first" {
				patterns = append(patterns, "-"+a)
			}
			if scenario == "project-rank" {
				writeFile(t, filepath.Join(a, "package.json"), []byte(`{"pi":{"skills":["."]}}`), 0600)
				piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), map[string]any{"defaultProjectTrust": "always", "packages": []string{a}})
				patterns = []string{b}
			}
			if scenario == "inactive" {
				patterns = []string{b}
			}
			piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), map[string]any{"skills": patterns})
			list := piList(t, fx, 0)
			if len(list.Sources) != 2 {
				t.Fatalf("independent identities merged: %+v", list.Sources)
			}
			winner, loser := a, b
			if scenario == "project-rank" || scenario == "inactive" {
				winner, loser = b, a
			}
			for _, s := range list.Sources {
				want := "available"
				if s.SourcePath == loser && scenario != "equal-content" {
					want = "unavailable"
				}
				if s.SourcePath == winner && scenario == "disabled-first" {
					want = "disabled"
				}
				if s.Availability.State != want {
					t.Fatalf("%s want %s: %+v", s.SourcePath, want, s)
				}
				if s.SourcePath == loser && scenario != "equal-content" && scenario != "inactive" && s.Availability.Evidence.Rule != "native-first-canonical-occurrence" {
					t.Fatalf("missing canonical explanation: %+v", s)
				}
			}
		})
	}
}

func TestALE33FindingPackageShapes(t *testing.T) {
	cases := []struct {
		name, manifest string
		object, filter bool
		want           string
	}{
		{"object-absent", `{"pi":{"extensions":[]}}`, true, false, "available"},
		{"string-absent", `{"pi":{"extensions":[]}}`, false, false, "absent"},
		{"object-empty", `{"pi":{"skills":[]}}`, true, false, "absent"},
		{"string-empty", `{"pi":{"skills":[]}}`, false, false, "absent"},
		{"filtered-empty", `{"pi":{"skills":[]}}`, true, true, "disabled"},
		{"filtered-absent", `{"pi":{"extensions":[]}}`, true, true, "disabled"},
		{"string-no-pi", `{}`, false, false, "available"},
		{"object-no-pi", `{}`, true, false, "available"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := piFixture(t)
			pkg := filepath.Join(fx.base, "allowed", "pkg")
			codexSkill(t, filepath.Join(pkg, "skills"), "example", "example")
			writeFile(t, filepath.Join(pkg, "package.json"), []byte(c.manifest), 0600)
			var registration any = pkg
			if c.object {
				p := map[string]any{"source": pkg}
				if c.filter {
					p["skills"] = []string{}
				}
				registration = p
			}
			piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), map[string]any{"packages": []any{registration}})
			list := piList(t, fx, 0)
			if c.want == "absent" {
				if len(list.Sources) != 0 {
					t.Fatalf("unexpected fallback: %+v", list.Sources)
				}
				return
			}
			if codexNamed(t, list, "example").Availability.State != c.want {
				t.Fatalf("package registration %+v", list.Sources)
			}
		})
	}
}

func TestALE33FindingManifestBytes(t *testing.T) {
	for _, format := range []struct{ name, prefix, newline string }{{"bom", "\xef\xbb\xbf", "\n"}, {"cr", "", "\r"}, {"bom-cr", "\xef\xbb\xbf", "\r"}, {"crlf", "", "\r\n"}} {
		for _, kind := range []string{"bundle", "file"} {
			t.Run(format.name+"/"+kind, func(t *testing.T) {
				fx := piFixture(t)
				root := filepath.Join(fx.home, ".pi", "agent", "skills")
				path := filepath.Join(root, "bytes", "SKILL.md")
				if kind == "file" {
					path = filepath.Join(root, "bytes.md")
				}
				front := strings.ReplaceAll("name: bytes\ndescription: Exact bytes.\n", "\n", format.newline)
				body := "Instructions" + format.newline
				raw := format.prefix + "---" + format.newline + front + "---" + format.newline + body
				writeFile(t, path, []byte(raw), 0600)
				list := piList(t, fx, 0)
				s := codexNamed(t, list, "bytes")
				if s.Availability.State != "available" {
					t.Fatalf("valid native manifest unavailable: %+v", s)
				}
				r := fx.run(t, "read", "--agent", "pi", "--project", fx.project, "--json", s.ID)
				assertExit(t, r, 0)
				var read publicRead
				decodeJSON(t, r.stdout, &read)
				if read.Source.Manifest.RawText != raw || read.Source.Manifest.RawFrontmatter != front || read.Source.Manifest.Instructions != body {
					t.Fatalf("raw byte contract changed: %+v", read.Source.Manifest)
				}
				if kind == "bundle" {
					second := filepath.Join(root, "other", "SKILL.md")
					writeFile(t, second, []byte(strings.TrimPrefix(raw, "\xef\xbb\xbf")), 0600)
					secondList := piList(t, fx, 0)
					var other string
					for _, candidate := range secondList.Sources {
						if candidate.SourcePath == filepath.Dir(second) {
							other = candidate.ID
						}
					}
					compare := fx.run(t, "compare", "--agent", "pi", "--project", fx.project, "--json", s.ID, other)
					assertExit(t, compare, 0)
					report := compareReport(t, compare)
					want := "identical"
					if format.prefix != "" {
						want = "different"
					}
					if report.Report.Outcome != want {
						t.Fatalf("comparison must use original bytes: %s", compare.stdout)
					}
				}
			})
		}
	}
}
