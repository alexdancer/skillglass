package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41RetryInputConsentVerification(t *testing.T) {
	for _, tc := range []struct{ app, mode string }{
		{"claude", "regular-bytes"},
		{"claude", "symlink-unchanged"},
		{"claude", "symlink-bytes"},
		{"claude", "symlink-identity"},
		{"pi", "regular-bytes"},
		{"pi", "symlink-unchanged"},
		{"pi", "symlink-bytes"},
		{"pi", "symlink-identity"},
	} {
		t.Run(tc.app+"-"+tc.mode, func(t *testing.T) {
			fx, id, manifest := ale40Fixture(t, false)
			config := filepath.Join(fx.home, ".claude/settings.json")
			if tc.app == "pi" {
				config = filepath.Join(fx.home, ".pi/agent/settings.json")
			}
			initial := []byte(`{"keep":true}`)
			changed := []byte(`{"keep":false}`)
			writeFile(t, config, initial, 0600)
			input := config
			if strings.HasPrefix(tc.mode, "symlink-") {
				input = filepath.Join(fx.base, "allowed", tc.app+"-external-settings.json")
				writeFile(t, input, initial, 0600)
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(input, config); err != nil {
					t.Fatal(err)
				}
			}
			independent := codexSkill(t, filepath.Join(fx.home, ".codex/skills"), "independent-survivor", "independent-survivor")
			independentBytes := readBytes(t, independent)
			targetBytes := readBytes(t, manifest)
			old := ale41Preview(t, fx, id)
			if old.State != "ready" || old.Confirmation == "" {
				t.Fatalf("old preview not ready: %+v", old)
			}
			oldLink, err := os.Lstat(config)
			if err != nil {
				t.Fatal(err)
			}
			oldInput, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			oldText := ""
			if strings.HasPrefix(tc.mode, "symlink-") {
				oldText, err = os.Readlink(config)
				if err != nil {
					t.Fatal(err)
				}
			}
			switch tc.mode {
			case "regular-bytes", "symlink-bytes":
				writeFile(t, input, changed, 0600)
			case "symlink-identity":
				if err := os.Rename(input, input+".original"); err != nil {
					t.Fatal(err)
				}
				writeFile(t, input, initial, 0600)
			}
			newInput, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			newLink, err := os.Lstat(config)
			if err != nil {
				t.Fatal(err)
			}
			if tc.mode == "symlink-identity" && os.SameFile(oldInput, newInput) {
				t.Fatal("endpoint identity did not change")
			}
			newText := ""
			if strings.HasPrefix(tc.mode, "symlink-") {
				newText, err = os.Readlink(config)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(oldLink, newLink) || oldText != newText {
					t.Fatal("fixture unexpectedly changed the native symlink")
				}
			}
			fresh := ale41Preview(t, fx, id)
			if fresh.State != "ready" || fresh.Confirmation == "" {
				t.Fatalf("fresh preview not ready: %+v", fresh)
			}
			before := treeManifest(t, fx.base)
			r := changeCommand(t, fx, "", "delete", "--agent", "codex", "--project", fx.project, "--json", "--confirm", old.Confirmation, id)
			var out collection.DeleteOutcome
			decodeJSON(t, r.stdout, &out)
			after := treeManifest(t, fx.base)
			_, targetErr := os.Lstat(filepath.Dir(manifest))
			targetSurvives := targetErr == nil
			if targetErr != nil && !os.IsNotExist(targetErr) {
				t.Fatal(targetErr)
			}
			finalInput := readBytes(t, input)
			finalConfig := readBytes(t, config)
			if !bytes.Equal(finalInput, finalConfig) {
				t.Error("native config link no longer resolves to endpoint bytes")
			}
			wantInput := initial
			if tc.mode == "regular-bytes" || tc.mode == "symlink-bytes" {
				wantInput = changed
			}
			if !bytes.Equal(finalInput, wantInput) {
				t.Error("viewer changed native settings endpoint bytes")
			}
			if !bytes.Equal(readBytes(t, independent), independentBytes) {
				t.Error("viewer changed independent skill")
			}
			finalLink, err := os.Lstat(config)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(newLink, finalLink) {
				t.Error("viewer replaced native configuration entry")
			}
			if targetSurvives && !bytes.Equal(readBytes(t, manifest), targetBytes) {
				t.Error("viewer changed surviving target bytes")
			}
			if tc.mode == "symlink-identity" && !bytes.Equal(readBytes(t, input+".original"), initial) {
				t.Error("original endpoint changed")
			}
			oldStat := oldInput.Sys().(*syscall.Stat_t)
			newStat := newInput.Sys().(*syscall.Stat_t)
			detail := map[string]any{
				"case": tc.app + "-" + tc.mode, "fixture_base": fx.base, "config": config, "endpoint": input, "target": filepath.Dir(manifest),
				"old_preview": old, "fresh_preview": fresh, "old_token": old.Confirmation, "fresh_token": fresh.Confirmation,
				"consumers_identical": reflect.DeepEqual(old.Consumers, fresh.Consumers), "old_endpoint_inode": oldStat.Ino, "new_endpoint_inode": newStat.Ino,
				"config_entry_identity_unchanged": os.SameFile(oldLink, newLink), "old_link_text": oldText, "new_link_text": newText,
				"endpoint_before": string(initial), "endpoint_at_apply": string(wantInput), "endpoint_after": string(finalInput),
				"exit": r.code, "outcome": out, "target_survives": targetSurvives, "independent_survives": true, "config_survives": true,
				"tree_unchanged": bytes.Equal(before, after), "before_tree": json.RawMessage(before), "after_tree": json.RawMessage(after),
			}
			raw, err := json.MarshalIndent(detail, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			writeEvidence(t, "r2-cases/"+tc.app+"-"+tc.mode+".json", raw)
			t.Logf("old_token=%s fresh_token=%s consumers_identical=%v exit=%d state=%s verified=%v target_survives=%v old_endpoint_inode=%d new_endpoint_inode=%d config_unchanged=%v", old.Confirmation, fresh.Confirmation, reflect.DeepEqual(old.Consumers, fresh.Consumers), r.code, out.State, out.Verified, targetSurvives, oldStat.Ino, newStat.Ino, os.SameFile(oldLink, newLink))
			if tc.mode == "symlink-unchanged" {
				if r.code != 0 || out.State != "removed" || !out.Verified || targetSurvives {
					t.Errorf("unchanged healthy input must apply: want exit=0 state=removed verified=true target absent")
				}
				if old.Confirmation != fresh.Confirmation {
					t.Error("unchanged preview token changed")
				}
			} else {
				if r.code != 1 || out.State != "stale" || len(out.Removed) != 0 || !targetSurvives || !bytes.Equal(before, after) {
					t.Errorf("changed native input must invalidate old delete consent: want exit=1 state=stale removed=[] target survives and zero writes; got exit=%d state=%s removed=%v target_survives=%v", r.code, out.State, out.Removed, targetSurvives)
				}
				if old.Confirmation == fresh.Confirmation {
					t.Error("changed native input must produce a different confirmation token")
				}
			}
		})
	}
}
