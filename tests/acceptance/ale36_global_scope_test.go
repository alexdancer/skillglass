package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestALE36GlobalInventoryCoverageAndPreservation(t *testing.T) {
	for _, kind := range []string{"incomplete", "shared-alias", "different-name", "late-membership", "late-content"} {
		t.Run(kind, func(t *testing.T) {
			fx, target, id := opencodeTarget(t)
			extra := filepath.Join(fx.base, "allowed/global-extra")
			global := filepath.Join(fx.xdgConfig, "opencode/opencode.json")
			config, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{extra}}, "permission": map[string]any{"skill": map[string]string{"other": "ask"}}, "theme": "retained"})
			writeFile(t, global, config, 0600)
			opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"skills":{"paths":[]}}`)
			if kind != "incomplete" {
				if err := os.MkdirAll(extra, 0700); err != nil {
					t.Fatal(err)
				}
			}
			var other string
			if kind == "shared-alias" {
				if err := os.Symlink(filepath.Dir(target), filepath.Join(extra, "alias")); err != nil {
					t.Fatal(err)
				}
			} else if kind != "incomplete" {
				other = codexSkill(t, extra, "other", "other")
			}
			var otherBody []byte
			if other != "" {
				otherBody = readBytes(t, other)
			}
			before := treeManifest(t, fx.base)
			if kind == "incomplete" {
				p := opencodeChange(t, fx, "global", "disable", id, "", 1)
				if p.State != "unsupported" && p.State != "refused" {
					t.Fatal(p)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			p := opencodeChange(t, fx, "global", "disable", id, "", 0)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if kind == "late-membership" || kind == "late-content" {
				if kind == "late-membership" {
					codexSkill(t, extra, "new", "another")
				} else {
					writeFile(t, other, append(readBytes(t, other), []byte("external content\n")...), 0600)
				}
				before = treeManifest(t, fx.base)
				out := opencodeChange(t, fx, "global", "disable", id, p.Confirmation, 1)
				if out.State != "refused" {
					t.Fatal(out)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				return
			}
			body := readBytes(t, target)
			out := opencodeChange(t, fx, "global", "disable", id, p.Confirmation, 0)
			assertSavedChange(t, out)
			if !bytes.Equal(body, readBytes(t, target)) {
				t.Fatal("source changed")
			}
			outside := fx
			outside.project = filepath.Join(fx.base, "allowed/other-project")
			if err := os.MkdirAll(outside.project, 0700); err != nil {
				t.Fatal(err)
			}
			list := opencodeListResult(t, outside, 0)
			if kind == "shared-alias" && len(list.Sources) != 1 {
				t.Fatal("shared source split into independent copies", list.Sources)
			}
			if kind == "different-name" {
				otherSource := codexNamed(t, list, "other")
				if otherSource.Availability.State != "ask" {
					t.Fatal("unrelated global permission changed", otherSource)
				}
				if !bytes.Equal(otherBody, readBytes(t, other)) {
					t.Fatal("unrelated source changed")
				}
			}
		})
	}
}
