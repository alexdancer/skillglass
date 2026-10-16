package collection

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func faultFixture(t *testing.T, missingRoot bool) (*Snapshot, SourceID, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	source := filepath.Join(home, ".agents/skills/target")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(source, "SKILL.md")
	if err := os.WriteFile(manifest, []byte("---\nname: target\ndescription: Inert fixture.\n---\nNo execution.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ch := filepath.Join(home, ".codex")
	if !missingRoot {
		if err := os.Mkdir(ch, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ch, "config.toml"), []byte("# unchanged\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", ch)
	request, err := NewRequestWithAgent(nil, nil, nil, "codex")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	id := snapshot.order[0]
	return snapshot, id, filepath.Join(ch, "config.toml")
}
func TestChangeFaultsRetainActualPersistenceAndCleanup(t *testing.T) {
	for _, stage := range []string{"write", "file-sync", "rename", "directory-sync", "postread", "cleanup"} {
		for _, missing := range []bool{false, true} {
			t.Run(stage+map[bool]string{true: "-new-root", false: "-existing"}[missing], func(t *testing.T) {
				snapshot, id, path := faultFixture(t, missing)
				before, _ := os.ReadFile(path)
				preview := snapshot.PreviewChange(context.Background(), ChangeIntent{id, "codex", ControlGlobal, ControlDisable})
				if preview.plan == nil {
					t.Fatal(preview.View())
				}
				preview.plan.fault = func(current string) error {
					if current == stage || (stage == "cleanup" && current == "write") {
						return errors.New("fixture failure")
					}
					return nil
				}
				outcome := preview.Apply(context.Background(), preview.View().Confirmation)
				if outcome.State != "failed" || outcome.ExitCode() != 1 {
					t.Fatalf("outcome %+v", outcome)
				}
				after, err := os.ReadFile(path)
				if stage == "directory-sync" || stage == "postread" {
					if outcome.WriteState != "committed" || err != nil || !bytes.Equal(after, preview.plan.postimage) {
						t.Fatalf("committed facts lost %+v", outcome)
					}
					if stage == "directory-sync" && outcome.Persistence != "verified" {
						t.Fatal("durability failure discarded observed bytes")
					}
				} else if !bytes.Equal(before, after) {
					t.Fatal("precommit failure changed config")
				}
				if stage == "cleanup" {
					if len(outcome.Artifacts) == 0 {
						t.Fatal("retained artifact not reported")
					}
				} else {
					entries, _ := os.ReadDir(filepath.Dir(path))
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), ".skillglass-") {
							t.Fatal("owned temp was leaked")
						}
					}
					if missing && outcome.WriteState == "none" {
						if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
							t.Fatal("new directory not cleaned")
						}
					}
				}
			})
		}
	}
}
func TestChangeFinalRecheckAndOpaqueAuthority(t *testing.T) {
	snapshot, id, path := faultFixture(t, false)
	intent := ChangeIntent{id, "codex", ControlGlobal, ControlDisable}
	preview := snapshot.PreviewChange(context.Background(), intent)
	preview.plan.fault = func(stage string) error {
		if stage == "before-recheck" {
			return os.WriteFile(path, []byte("# external last-check edit\n"), 0600)
		}
		return nil
	}
	out := preview.Apply(context.Background(), preview.View().Confirmation)
	if out.State != "refused" || out.WriteState != "none" {
		t.Fatalf("stale %+v", out)
	}
	after, _ := os.ReadFile(path)
	if string(after) != "# external last-check edit\n" {
		t.Fatal("external edit lost")
	}
	forged := &ChangePreview{view: preview.View()}
	if forged.Apply(context.Background(), preview.View().Confirmation).State != "refused" {
		t.Fatal("forged plan wrote")
	}
	fresh := snapshot.PreviewChange(context.Background(), intent)
	view := fresh.View()
	view.ConfigPath = filepath.Join(t.TempDir(), "forged")
	view.Changes[0].After = "true"
	out = fresh.Apply(context.Background(), fresh.View().Confirmation)
	if out.ExitCode() != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(view.ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("display became authority")
	}
	if fresh.Apply(context.Background(), fresh.View().Confirmation).State != "refused" {
		t.Fatal("same object applied twice")
	}
}
func TestChangeInheritedArrayProtection(t *testing.T) {
	_, _, config := faultFixture(t, false)
	home, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(config)))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".agents/skills/target/SKILL.md")
	unrelated := filepath.Join(filepath.Dir(target), "other.md")
	if err := os.WriteFile(unrelated, []byte("inert"), 0600); err != nil {
		t.Fatal(err)
	}
	layers := []PermissionLayer{{Scope: "system", Path: "fixture-system-config", Choices: []PermissionChoice{{Path: target, Enabled: false}, {Path: unrelated, Enabled: false}}}}
	if protectInheritedChoices(layers, target, false) == nil {
		t.Fatal("unrelated inheritance disappeared")
	}
	if protectInheritedChoices(layers, target, true) != nil {
		t.Fatal("existing user array unnecessarily refused")
	}
	layers[0].Choices = layers[0].Choices[:1]
	if err := protectInheritedChoices(layers, target, false); err != nil {
		t.Fatal("only selected inherited choice refused", err)
	}
}
