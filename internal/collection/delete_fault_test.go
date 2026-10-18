//go:build darwin

package collection

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func deleteFaultFixture(t *testing.T) (*DeletePreview, string, string) {
	t.Helper()
	_, id, config := faultFixture(t, false)
	home := filepath.Dir(filepath.Dir(config))
	dir := filepath.Join(home, ".agents/skills/target")
	for key, value := range map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(home, ".claude"), "HERMES_HOME": filepath.Join(home, ".hermes"), "PI_CODING_AGENT_DIR": filepath.Join(home, ".pi/agent"), "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_CACHE_HOME": filepath.Join(home, "cache"), "XDG_DATA_HOME": filepath.Join(home, "data"), "OPENCODE_TEST_MANAGED_CONFIG_DIR": filepath.Join(home, "managed"), "CODEAF_HOME": home, "CODEAF_PROFILE_DIR": filepath.Join(home, ".codeaf"), "GEMINI_CLI_HOME": home, "HERMES_MANAGED_DIR": filepath.Join(home, "managed-hermes"), "PYTEST_CURRENT_TEST": "delete-fixture"} {
		t.Setenv(key, value)
	}
	for _, name := range []string{".a", "z"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("retained resource"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request, err := NewRequestWithAgent(nil, nil, nil, "codex")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	preview := snapshot.PreviewDelete(context.Background(), id)
	if preview.view.State != "ready" {
		t.Fatal(preview.view.Reason)
	}
	external := filepath.Join(home, "sentinel")
	if err := os.WriteFile(external, []byte("outside survives"), 0600); err != nil {
		t.Fatal(err)
	}
	// The sentinel is outside the sealed target. Native discovery and ancestor bindings remain identical.
	return preview, dir, external
}
func TestDeleteRemovalFaults(t *testing.T) {
	for _, scenario := range []string{"first-error", "after-first-error", "cancel", "symlink-before", "rename-child", "rename-directory", "add-name", "replace-root", "name-gap"} {
		t.Run(scenario, func(t *testing.T) {
			p, dir, external := deleteFaultFixture(t)
			if scenario == "rename-directory" {
				child := filepath.Join(dir, ".dir", "child")
				if err := os.MkdirAll(filepath.Dir(child), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(child, []byte("anchored child survives"), 0600); err != nil {
					t.Fatal(err)
				}
				fresh, err := Build(context.Background(), p.request)
				if err != nil {
					t.Fatal(err)
				}
				p = fresh.PreviewDelete(context.Background(), p.view.Target.Source)
				if p.view.State != "ready" {
					t.Fatal(p.view.Reason)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fired := false
			p.checkpoint = func(phase, path string) error {
				if fired {
					return nil
				}
				switch scenario {
				case "first-error":
					if phase == "before-validation" {
						fired = true
						return errors.New("injected first-operation failure")
					}
				case "after-first-error":
					if phase == "after-unlink" {
						fired = true
						return errors.New("injected interruption")
					}
				case "cancel":
					if phase == "after-unlink" {
						fired = true
						cancel()
					}
				case "symlink-before", "name-gap":
					wanted := "before-unlink-validation"
					if scenario == "name-gap" {
						wanted = "after-unlink-validation"
					}
					if phase == wanted && path == ".a" {
						fired = true
						if err := os.Rename(filepath.Join(dir, ".a"), filepath.Join(filepath.Dir(dir), "moved-a")); err != nil {
							return err
						}
						return os.Symlink(external, filepath.Join(dir, ".a"))
					}
				case "rename-child":
					if phase == "before-unlink-validation" && path == ".a" {
						fired = true
						return os.Rename(filepath.Join(dir, ".a"), filepath.Join(dir, "moved"))
					}
				case "rename-directory":
					if phase == "before-unlink-validation" && path == ".dir/child" {
						fired = true
						return os.Rename(filepath.Join(dir, ".dir"), dir+"-child")
					}
				case "add-name":
					if phase == "after-unlink" {
						fired = true
						return os.WriteFile(filepath.Join(dir, "new"), []byte("new survives"), 0600)
					}
				case "replace-root":
					if phase == "before-validation" {
						fired = true
						if err := os.Rename(dir, dir+"-moved"); err != nil {
							return err
						}
						return os.Symlink(filepath.Dir(external), dir)
					}
				}
				return nil
			}
			out := p.Apply(ctx, p.view.Confirmation)
			if out.ExitCode() != 1 || out.Verified || !fired {
				t.Fatalf("unexpected outcome %+v fired %t", out, fired)
			}
			if raw, err := os.ReadFile(external); err != nil || string(raw) != "outside survives" {
				t.Fatal("external sentinel changed", err)
			}
			switch scenario {
			case "first-error", "symlink-before", "replace-root":
				if len(out.Removed) != 0 {
					t.Fatal(out)
				}
				survivor := filepath.Join(dir, "z")
				if scenario == "replace-root" {
					survivor = filepath.Join(dir+"-moved", "z")
				}
				if raw, err := os.ReadFile(survivor); err != nil || string(raw) != "retained resource" {
					t.Fatal("original content lost", err)
				}
				if len(out.Remaining) == 0 && len(out.Replacements) == 0 {
					t.Fatal("remnants were not reported", out.State, out.Reason)
				}
			case "rename-directory":
				if raw, err := os.ReadFile(filepath.Join(dir+"-child", "child")); err != nil || string(raw) != "anchored child survives" {
					t.Fatal("moved anchored child was traversed", err)
				}
			case "after-first-error", "cancel", "add-name":
				if out.State != "partial" || len(out.Removed) != 1 {
					t.Fatal(out)
				}
				if _, err := os.Stat(filepath.Join(dir, "z")); err != nil {
					t.Fatal("later resource removed", err)
				}
			case "name-gap":
				if out.State != "partial" || len(out.Unverified) == 0 {
					t.Fatal(out)
				}
				if _, err := os.Lstat(filepath.Join(dir, ".a")); !os.IsNotExist(err) {
					t.Fatal("test did not demonstrate name-gap unlink", err)
				}
				if raw, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "moved-a")); err != nil || string(raw) != "retained resource" {
					t.Fatal("original object lost", err)
				}
			}
		})
	}
}
func TestDeletePreviewSingleUse(t *testing.T) {
	p, dir, _ := deleteFaultFixture(t)
	token := p.view.Confirmation
	out := p.Apply(context.Background(), "wrong")
	if out.State != "refused" {
		t.Fatal(out)
	}
	out = p.Apply(context.Background(), token)
	if out.State != "refused" {
		t.Fatal(out)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteSharedInodeRejectsPostUnlinkChanges(t *testing.T) {
	for _, scenario := range []string{"bytes-restored-mtime", "permissions", "extra-link", "symlink-replacement"} {
		t.Run(scenario, func(t *testing.T) {
			preview, dir, external := deleteFaultFixture(t)
			first, second := filepath.Join(dir, ".a"), filepath.Join(dir, ".b")
			replacement := filepath.Join(filepath.Dir(dir), "replacement-sentinel")
			if err := os.WriteFile(replacement, []byte("replacement survives"), 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "symlink-replacement" {
				if err := os.Remove(first); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, first); err != nil {
					t.Fatal(err)
				}
				if err := unix.Linkat(unix.AT_FDCWD, first, unix.AT_FDCWD, second, 0); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Link(first, second); err != nil {
				t.Fatal(err)
			}
			original, err := os.Lstat(second)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := Build(context.Background(), preview.request)
			if err != nil {
				t.Fatal(err)
			}
			preview = fresh.PreviewDelete(context.Background(), preview.view.Target.Source)
			if preview.view.State != "ready" {
				t.Fatal(preview.view.Reason)
			}
			fired := false
			preview.checkpoint = func(phase, path string) error {
				if fired || phase != "after-unlink-syscall" || path != ".a" {
					return nil
				}
				fired = true
				switch scenario {
				case "bytes-restored-mtime":
					if err := os.WriteFile(second, []byte("external mutation"), 0600); err != nil {
						return err
					}
					return os.Chtimes(second, time.Now(), original.ModTime())
				case "permissions":
					return os.Chmod(second, 0640)
				case "extra-link":
					return os.Link(second, filepath.Join(filepath.Dir(dir), "extra-alias"))
				case "symlink-replacement":
					if err := os.Remove(second); err != nil {
						return err
					}
					return os.Symlink(replacement, second)
				}
				return nil
			}
			out := preview.Apply(context.Background(), preview.view.Confirmation)
			if !fired || out.ExitCode() != 1 || out.State != "partial" || out.Verified {
				t.Fatalf("fired=%t state=%s removed=%v reason=%s", fired, out.State, out.Removed, out.Reason)
			}
			if _, err := os.Lstat(first); !os.IsNotExist(err) {
				t.Fatal("first syscall did not remove its name", err)
			}
			if raw, err := os.ReadFile(filepath.Join(dir, "z")); err != nil || string(raw) != "retained resource" {
				t.Fatal("later resource changed", err)
			}
			if raw, err := os.ReadFile(external); err != nil || string(raw) != "outside survives" {
				t.Fatal("external sentinel changed", err)
			}
			if raw, err := os.ReadFile(replacement); err != nil || string(raw) != "replacement survives" {
				t.Fatal("replacement target changed", err)
			}
			if scenario == "symlink-replacement" {
				if target, err := os.Readlink(second); err != nil || target != replacement {
					t.Fatal("replacement link was removed", err)
				}
			} else {
				want := "retained resource"
				if scenario == "bytes-restored-mtime" {
					want = "external mutation"
				}
				if raw, err := os.ReadFile(second); err != nil || string(raw) != want {
					t.Fatal("remaining alias was removed or changed", err)
				}
				if scenario == "extra-link" {
					if raw, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "extra-alias")); err != nil || string(raw) != "retained resource" {
						t.Fatal("external added alias changed", err)
					}
				}
			}
		})
	}
}
