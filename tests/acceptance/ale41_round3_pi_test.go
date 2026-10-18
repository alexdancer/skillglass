package acceptance_test

import (
	"encoding/json"
	"github.com/alexdancer/skillglass/internal/collection"
	"os"
	"path/filepath"
	"testing"
)

func TestALE41Round3PiProof(t *testing.T) {
	for _, kind := range []string{"absent", "valid-default", "invalid-settings", "valid-nondefault", "invalid-manifest", "unreadable-manifest"} {
		t.Run(kind, func(t *testing.T) {
			fx, _ := codexFixture(t)
			pkg := filepath.Join(fx.base, "allowed", "external-package")
			resource := "skills"
			if kind == "valid-nondefault" || kind == "invalid-manifest" || kind == "unreadable-manifest" {
				resource = "resources"
			}
			manifest := codexSkill(t, filepath.Join(pkg, resource), "target", "pi-proof")
			packageJSON := filepath.Join(pkg, "package.json")
			raw, _ := json.Marshal(map[string]any{"pi": map[string]any{"skills": []string{resource}}})
			writeFile(t, packageJSON, raw, 0600)
			settings := filepath.Join(fx.home, ".pi/agent/settings.json")
			registration, _ := json.Marshal(map[string]any{"packages": []string{pkg}})
			if kind != "absent" {
				writeFile(t, settings, registration, 0600)
			}
			if kind == "invalid-settings" {
				writeFile(t, settings, []byte(`{"packages":"invalid"}`), 0600)
			}
			if kind == "invalid-manifest" {
				writeFile(t, packageJSON, []byte(`{"pi":`), 0600)
			}
			if kind == "unreadable-manifest" {
				if err := os.Chmod(packageJSON, 0000); err != nil {
					t.Fatal(err)
				}
			}
			sentinel := filepath.Join(pkg, "independent.txt")
			writeFile(t, sentinel, []byte("survivor"), 0600)
			root := filepath.Dir(manifest)
			r := fx.run(t, "list", "--root", root, "--json")
			assertExit(t, r, 0)
			var inventory collection.View
			decodeJSON(t, r.stdout, &inventory)
			if len(inventory.Sources) != 1 {
				t.Fatal(inventory.Sources)
			}
			id := string(inventory.Sources[0].ID)
			r = changeCommand(t, fx, "", "delete", "--root", root, "--json", id)
			var preview collection.DeleteView
			decodeJSON(t, r.stdout, &preview)
			t.Logf("preview exit=%d state=%s reason=%s", r.code, preview.State, preview.Reason)
			if _, err := os.Stat(manifest); err != nil {
				t.Fatal("preview changed selected content", err)
			}
			if preview.State == "ready" {
				r = changeCommand(t, fx, "", "delete", "--root", root, "--json", "--confirm", preview.Confirmation, id)
				var outcome collection.DeleteOutcome
				decodeJSON(t, r.stdout, &outcome)
				t.Logf("apply exit=%d state=%s verified=%v reason=%s", r.code, outcome.State, outcome.Verified, outcome.Reason)
				if outcome.State != "removed" || !outcome.Verified {
					t.Fatal("claimed public apply failed", outcome)
				}
				if _, err := os.Lstat(root); !os.IsNotExist(err) {
					t.Fatal("selected target survived", err)
				}
			}
			if string(readBytes(t, sentinel)) != "survivor" {
				t.Fatal("independent package content changed")
			}
			_, targetErr := os.Lstat(root)
			result, _ := json.MarshalIndent(map[string]any{"case": kind, "preview_state": preview.State, "target_survives": targetErr == nil, "independent_survives": string(readBytes(t, sentinel)) == "survivor"}, "", "  ")
			writeEvidence(t, "pi-proof/"+kind+"-survivors.json", result)
			if kind == "absent" {
				if preview.State != "ready" {
					t.Fatal("absent control should permit", preview.State)
				}
			} else if preview.State != "refused" {
				t.Errorf("require managed or uncertain ownership refusal, got %q; synthetic target was removed", preview.State)
			}
		})
	}
}
