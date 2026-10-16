package collection

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpencodeProjectDestinationContentRechecks(t *testing.T) {
	for _, stage := range []string{"before-recheck", "postread"} {
		for _, choice := range []ControlChoice{ControlEnable, ControlDisable, ControlInherit} {
			t.Run(stage+"/"+string(choice), func(t *testing.T) {
				_, id, codexConfig := faultFixture(t, false)
				home, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(codexConfig)))
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv("HOME", home)
				configHome := filepath.Join(home, "config")
				t.Setenv("XDG_CONFIG_HOME", configHome)
				t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
				t.Setenv("OPENCODE_TEST_MANAGED_CONFIG_DIR", filepath.Join(home, "managed"))
				extra := filepath.Join(home, "global-extra")
				project := filepath.Join(extra, "repo")
				global := filepath.Join(configHome, "opencode/opencode.json")
				for _, dir := range []string{project, filepath.Dir(global)} {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				globalRaw, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{extra}}, "theme": "unchanged"})
				if err := os.WriteFile(global, globalRaw, 0600); err != nil {
					t.Fatal(err)
				}
				config := filepath.Join(project, "opencode.json")
				raw := []byte(`{"skills":{"paths":[]},"theme":"bundle-resource"}`)
				if choice != ControlDisable {
					raw = []byte(`{"skills":{"paths":[]},"permission":{"skill":{"target":"deny"}},"theme":"bundle-resource"}`)
				}
				if err := os.WriteFile(config, raw, 0640); err != nil {
					t.Fatal(err)
				}
				request, err := NewRequestWithAgent(nil, nil, &project, "opencode")
				if err != nil {
					t.Fatal(err)
				}
				snapshot, err := Build(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				preview := snapshot.PreviewChange(context.Background(), ChangeIntent{id, "opencode", ControlProject, choice})
				if preview.plan == nil || preview.View().NoWriteNeeded {
					t.Fatal(preview.View())
				}
				manifest := filepath.Join(project, "SKILL.md")
				manifestRaw := []byte("---\nname: bundle-owner\ndescription: Inert fixture.\n---\nNo execution.\n")
				preview.plan.fault = func(current string) error {
					if current == stage {
						return os.WriteFile(manifest, manifestRaw, 0600)
					}
					return nil
				}
				out := preview.Apply(context.Background(), preview.View().Confirmation)
				if out.ExitCode() != 1 || out.Achievement == "achieved" {
					t.Fatal(out)
				}
				saved, err := os.ReadFile(config)
				if err != nil {
					t.Fatal(err)
				}
				if stage == "before-recheck" {
					if out.State != "refused" || out.WriteState != "none" || !bytes.Equal(raw, saved) {
						t.Fatal(out, string(saved))
					}
				} else if out.Persistence != "verified" || out.PersistedChoice != choice || out.Achievement != "unknown" || !bytes.Equal(preview.plan.postimage, saved) {
					t.Fatal(out, string(saved))
				}
				info, err := os.Stat(config)
				if err != nil || info.Mode().Perm() != 0640 {
					t.Fatal("resource mode changed", err)
				}
				actualManifest, err := os.ReadFile(manifest)
				if err != nil || !bytes.Equal(actualManifest, manifestRaw) {
					t.Fatal("external manifest changed", err)
				}
				actualGlobal, err := os.ReadFile(global)
				if err != nil || !bytes.Equal(actualGlobal, globalRaw) {
					t.Fatal("global settings changed", err)
				}
			})
		}
	}
}
