package acceptance_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserYAMLUnicodeKeysAndAliasesAcrossLineEndings(t *testing.T) {
	metadata := "name: probe\n" +
		"flow: {first: café😀, \"clé:key\": \"clé:key neutral\", '字:key': quoted neutral}\n" +
		"anchor: &shared {nested: anchored value}\n" +
		"alias: *shared\n" +
		"items:\n" +
		"  - child: sequence value\n" +
		"literal: |\n" +
		"  scalar: body\n" +
		"folded: >\n" +
		"  folded: body\n"
	for _, ending := range []string{"lf", "crlf"} {
		for _, theme := range []string{"dark", "light", "none"} {
			t.Run(ending+"/"+theme, func(t *testing.T) {
				fx := newFixture(t)
				root := filepath.Join(fx.base, "allowed", "roots", "yaml-index")
				raw := metadata
				if ending == "crlf" {
					raw = strings.ReplaceAll(raw, "\n", "\r\n")
				}
				writeFile(t, filepath.Join(root, "probe", "SKILL.md"), []byte("---\n"+raw+"---\n# Probe\nBody.\n"), 0o644)
				before := treeManifest(t, fx.base)
				frames := captureColorProbe(t, fx, root, 120, theme, false, "probe", "Probe", "Body.")
				writeEvidence(t, "ale43-yaml-index/"+ending+"-"+theme+".ansi", []byte(frames.detailsEnd))
				if got, want := metadataFromDetails(t, frames.detailsEnd), strings.TrimSuffix(strings.ReplaceAll(raw, "\r", `\r`), "\n"); got != want {
					t.Fatalf("raw metadata changed: got %q, want %q", got, want)
				}
				foreground := "5;183"
				if theme == "light" {
					foreground = "5;56"
				}
				if theme == "none" {
					if strings.ContainsRune(frames.detailsEnd, '\x1b') {
						t.Fatal("no-color YAML emitted styling")
					}
				} else {
					for _, key := range []string{"name", "flow", "first", `"clé:key"`, "'字:key'", "anchor", "nested", "alias", "items", "child", "literal", "folded"} {
						assertTextStyle(t, frames.detailsEnd, key, foreground, "", true)
					}
					for _, value := range []string{"café😀", `"clé:key neutral"`, "quoted neutral", "&shared", "anchored value", "*shared", "sequence value", "scalar: body", "folded: body"} {
						assertTextNotForeground(t, frames.detailsEnd, value, foreground)
					}
				}
				if !bytes.Equal(before, treeManifest(t, fx.base)) {
					t.Fatal("browser modified the fixture")
				}
			})
		}
	}
}

func TestBrowserUnusualYAMLLineBoundariesRemainPlain(t *testing.T) {
	for _, boundary := range []struct {
		name    string
		raw     string
		escaped string
	}{
		{name: "cr", raw: "\r", escaped: `\r`},
		{name: "nel", raw: "\u0085", escaped: `\x85`},
		{name: "ls", raw: "\u2028", escaped: `\u2028`},
		{name: "ps", raw: "\u2029", escaped: `\u2029`},
	} {
		for _, theme := range []string{"dark", "light", "none", "no-color"} {
			t.Run(boundary.name+"/"+theme, func(t *testing.T) {
				fx := newFixture(t)
				root := filepath.Join(fx.base, "allowed", "roots", "yaml-boundary")
				metadata := "{name: probe," + boundary.raw + "key: \"first\nkey value\"}\n"
				writeFile(t, filepath.Join(root, "probe", "SKILL.md"), []byte("---\n"+metadata+"---\n# Probe\nBody.\n"), 0o644)
				before := treeManifest(t, fx.base)
				result := fx.run(t, "list", "--root", root, "--json")
				assertExit(t, result, 0)
				var listed publicList
				decodeJSON(t, result.stdout, &listed)
				if len(listed.Sources) != 1 || listed.Sources[0].Manifest.Status != "parsed" || listed.Sources[0].Manifest.RawFrontmatter != metadata {
					t.Fatalf("probe did not preserve valid YAML: %#v", listed.Sources)
				}
				browserTheme := theme
				if theme == "no-color" {
					browserTheme = "dark"
				}
				frames := captureColorProbe(t, fx, root, 120, browserTheme, theme == "no-color", "probe", "Probe", "Body.")
				writeEvidence(t, "ale43-yaml-boundaries/"+boundary.name+"-"+theme+".ansi", []byte(frames.detailsEnd))
				want := "{name: probe," + boundary.escaped + "key: \"first\nkey value\"}"
				if got := metadataFromDetails(t, frames.detailsEnd); got != want {
					t.Fatalf("raw YAML escaped text changed: got %q, want %q", got, want)
				}
				foreground := "5;183"
				if theme == "light" {
					foreground = "5;56"
				}
				assertTextNotForeground(t, frames.detailsEnd, "key", foreground)
				if (theme == "none" || theme == "no-color") && strings.ContainsRune(frames.detailsEnd, '\x1b') {
					t.Fatal("no-color YAML emitted styling")
				}
				if !bytes.Equal(before, treeManifest(t, fx.base)) {
					t.Fatal("browser modified the fixture")
				}
			})
		}
	}
}
