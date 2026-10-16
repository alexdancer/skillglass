package collection

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpencodeGlobalInventoryRaces(t *testing.T) {
	for _, kind := range []string{"postread-duplicate", "postread-membership", "postread-incomplete", "postread-selected-content", "postread-hidden-content", "created-duplicate", "created-membership", "created-hidden-content"} {
		t.Run(kind, func(t *testing.T) {
			_, id, codexConfig := faultFixture(t, false)
			home := filepath.Dir(filepath.Dir(codexConfig))
			configHome := filepath.Join(home, "config")
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
			t.Setenv("OPENCODE_TEST_MANAGED_CONFIG_DIR", filepath.Join(home, "managed"))
			project := filepath.Join(home, "project")
			extra := filepath.Join(home, "global-extra")
			for _, dir := range []string{project, extra, configHome, filepath.Join(home, ".opencode")} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			put := func(path string, raw []byte) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			config, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{extra}}})
			put(filepath.Join(home, ".opencode/opencode.json"), config)
			put(filepath.Join(project, "opencode.json"), []byte(`{"skills":{"paths":[]}}`))
			hidden := filepath.Join(extra, "other/SKILL.md")
			put(hidden, []byte("---\nname: other\ndescription: Inert fixture.\n---\nNo execution.\n"))
			request, err := NewRequestWithAgent(nil, nil, &project, "opencode")
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := Build(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			preview := snapshot.PreviewChange(context.Background(), ChangeIntent{id, "opencode", ControlGlobal, ControlDisable})
			if preview.plan == nil || preview.View().CreateDirectory == "" {
				t.Fatal(preview.View())
			}
			before := snapshot.sources[id].Manifest.RawText
			preview.plan.fault = func(stage string) error {
				wanted := "postread"
				if kind == "created-duplicate" || kind == "created-membership" || kind == "created-hidden-content" {
					wanted = "before-recheck"
				}
				if stage != wanted {
					return nil
				}
				switch kind {
				case "postread-incomplete":
					return os.RemoveAll(extra)
				case "postread-selected-content":
					return os.WriteFile(snapshot.sources[id].Manifest.Path, []byte(before+"external edit\n"), 0600)
				case "postread-hidden-content", "created-hidden-content":
					return os.WriteFile(hidden, []byte("---\nname: other\ndescription: Changed fixture.\n---\nexternal edit\n"), 0600)
				default:
					name := "another"
					if kind == "postread-duplicate" || kind == "created-duplicate" {
						name = "target"
					}
					put(filepath.Join(extra, "new/SKILL.md"), []byte("---\nname: "+name+"\ndescription: Inert fixture.\n---\nNo execution.\n"))
					return nil
				}
			}
			out := preview.Apply(context.Background(), preview.View().Confirmation)
			if out.ExitCode() != 1 || out.Achievement == "achieved" {
				t.Fatal(out)
			}
			saved, err := os.ReadFile(preview.plan.destination.Path)
			if kind == "created-duplicate" || kind == "created-membership" || kind == "created-hidden-content" {
				if out.State != "refused" || out.WriteState != "none" || !os.IsNotExist(err) {
					t.Fatal(out, err)
				}
			} else if out.Persistence != "verified" || out.PersistedChoice != ControlDisable || out.Achievement != "unknown" || err != nil || !bytes.Equal(saved, preview.plan.postimage) {
				t.Fatal(out, err)
			}
		})
	}
}
