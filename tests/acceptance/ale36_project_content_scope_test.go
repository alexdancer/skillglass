package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestALE36ProjectContentDiscoveryAndAliases(t *testing.T) {
	for _, kind := range []string{"incomplete", "relative-path", "symlink", "case-alias", "jsonc", "late-membership", "late-content"} {
		t.Run(kind, func(t *testing.T) {
			fx, _, id := opencodeTarget(t)
			extra := filepath.Join(fx.base, "allowed/global-extra")
			if kind != "incomplete" {
				if err := os.MkdirAll(extra, 0700); err != nil {
					t.Fatal(err)
				}
			}
			globalPath := extra
			if kind == "relative-path" {
				globalPath = "unverified-relative-root"
			}
			global, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{globalPath}}, "theme": "unchanged"})
			writeFile(t, filepath.Join(fx.xdgConfig, "opencode/opencode.json"), global, 0600)
			project := filepath.Join(fx.project, "opencode.json")
			raw := []byte(`{"skills":{"paths":[]},"theme":"project-resource"}`)
			writeFile(t, project, raw, 0640)
			var hidden string
			if kind == "symlink" || kind == "case-alias" || kind == "jsonc" {
				repo := filepath.Join(extra, "Repo")
				codexSkill(t, extra, "Repo", "bundle-owner")
				fx.project = repo
				if kind == "symlink" {
					fx.project = filepath.Join(fx.base, "allowed/project-alias")
					if err := os.Symlink(repo, fx.project); err != nil {
						t.Fatal(err)
					}
				} else if kind == "case-alias" {
					fx.project = filepath.Join(extra, "repo")
					actual, err := os.Stat(repo)
					if err != nil {
						t.Fatal(err)
					}
					alias, err := os.Stat(fx.project)
					if os.IsNotExist(err) {
						t.Skip("fixture volume is case-sensitive")
					}
					if err != nil || !os.SameFile(actual, alias) {
						t.Fatal("case alias identity", err)
					}
				}
				project = filepath.Join(fx.project, "opencode.json")
				if kind == "jsonc" {
					project += "c"
				}
				writeFile(t, project, raw, 0640)
			} else if kind == "late-content" {
				hidden = codexSkill(t, extra, "other", "other")
			}
			before := treeManifest(t, fx.base)
			late := kind == "late-membership" || kind == "late-content"
			if !late {
				p := opencodeChange(t, fx, "project", "disable", id, "", 1)
				if p.State != "refused" {
					t.Fatal(p)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			p := opencodeChange(t, fx, "project", "disable", id, "", 0)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if kind == "late-membership" {
				codexSkill(t, extra, "new", "other")
			} else {
				writeFile(t, hidden, append(readBytes(t, hidden), []byte("external edit\n")...), 0600)
			}
			before = treeManifest(t, fx.base)
			out := opencodeChange(t, fx, "project", "disable", id, p.Confirmation, 1)
			if out.State != "refused" {
				t.Fatal(out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE36ProjectActionDoesNotRequireGlobalNameIsolation(t *testing.T) {
	fx, target, id := opencodeTarget(t)
	extra := filepath.Join(fx.base, "allowed/global-extra")
	hidden := codexSkill(t, extra, "independent", "target")
	globalPath := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
	global, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{extra}}, "theme": "unchanged"})
	writeFile(t, globalPath, global, 0600)
	project := filepath.Join(fx.project, "opencode.json")
	writeFile(t, project, []byte(`{"skills":{"paths":[]},"theme":"project-only"}`), 0600)
	targetBody, hiddenBody := readBytes(t, target), readBytes(t, hidden)
	for _, choice := range []string{"disable", "enable", "inherit"} {
		opencodeApply(t, fx, "project", choice, id)
		if !bytes.Equal(global, readBytes(t, globalPath)) || !bytes.Equal(targetBody, readBytes(t, target)) || !bytes.Equal(hiddenBody, readBytes(t, hidden)) {
			t.Fatal("project action changed global settings or skill content")
		}
	}
	before := treeManifest(t, fx.base)
	p := opencodeChange(t, fx, "project", "inherit", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal(p)
	}
	opencodeChange(t, fx, "project", "inherit", id, p.Confirmation, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	writeFile(t, project, []byte(`{"skills":{"paths":[]},"permission":{"skill":{"*":"ask"}},"theme":"project-only"}`), 0600)
	before = treeManifest(t, fx.base)
	p = opencodeChange(t, fx, "project", "enable", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal(p)
	}
	opencodeChange(t, fx, "project", "enable", id, p.Confirmation, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
