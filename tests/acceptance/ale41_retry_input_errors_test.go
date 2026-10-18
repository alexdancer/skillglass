package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41RetryUnobservableConfigEndpoint(t *testing.T) {
	for _, mode := range []string{"unreadable", "dangling", "cycle"} {
		t.Run(mode, func(t *testing.T) {
			fx, id, manifest := ale40Fixture(t, false)
			config := filepath.Join(fx.home, ".claude/settings.json")
			endpoint := filepath.Join(fx.base, "allowed", "external-settings.json")
			raw := []byte(`{"keep":true}`)
			writeFile(t, endpoint, raw, 0600)
			if err := os.Remove(config); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(endpoint, config); err != nil {
				t.Fatal(err)
			}
			old := ale41Preview(t, fx, id)
			if old.State != "ready" {
				t.Fatal(old.Reason)
			}
			if mode == "unreadable" {
				if err := os.Chmod(endpoint, 0000); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(endpoint, 0600)
				if _, err := os.ReadFile(endpoint); !os.IsPermission(err) {
					t.Fatalf("fixture must establish EACCES: %v", err)
				}
			} else {
				if err := os.Rename(endpoint, endpoint+".original"); err != nil {
					t.Fatal(err)
				}
				if mode == "cycle" {
					if err := os.Symlink(endpoint, endpoint); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := treeManifest(t, fx.base)
			fresh := ale41Preview(t, fx, id)
			if fresh.State != "refused" || fresh.Confirmation != "" {
				t.Fatalf("unobservable config granted consent: %+v", fresh)
			}
			r := changeCommand(t, fx, "", "delete", "--agent", "codex", "--project", fx.project, "--json", "--confirm", old.Confirmation, id)
			assertExit(t, r, 1)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			if out.State != "stale" || len(out.Removed) != 0 {
				t.Fatalf("unobservable input must invalidate held consent: %+v", out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if _, err := os.Stat(manifest); err != nil {
				t.Fatal(err)
			}
			if mode == "unreadable" {
				if err := os.Chmod(endpoint, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				endpoint += ".original"
			}
			if string(readBytes(t, endpoint)) != string(raw) {
				t.Fatal("viewer changed settings bytes")
			}
			artifact, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			writeEvidence(t, "retry-input-errors/"+mode+".json", artifact)
		})
	}
}
