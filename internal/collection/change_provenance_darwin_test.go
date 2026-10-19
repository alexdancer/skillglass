//go:build darwin

package collection

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReplacementAttributesExceptionIsExact(t *testing.T) {
	original := map[string][]byte{"com.apple.provenance": {1}, "org.skillglass.fixture": {2}}
	for _, tc := range []struct {
		name        string
		replacement map[string][]byte
		want        bool
	}{
		{"unchanged", map[string][]byte{"com.apple.provenance": {1}, "org.skillglass.fixture": {2}}, true},
		{"provenance-regenerated", map[string][]byte{"com.apple.provenance": {3}, "org.skillglass.fixture": {2}}, true},
		{"provenance-absent", map[string][]byte{"org.skillglass.fixture": {2}}, true},
		{"ordinary-value", map[string][]byte{"com.apple.provenance": {3}, "org.skillglass.fixture": {3}}, false},
		{"ordinary-removed", map[string][]byte{"com.apple.provenance": {3}}, false},
		{"ordinary-added", map[string][]byte{"org.skillglass.fixture": {2}, "extra": {}}, false},
		{"similar-apple-name", map[string][]byte{"org.skillglass.fixture": {2}, "com.apple.provenance.extra": {1}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := replacementAttributesMatch(original, tc.replacement); got != tc.want {
				t.Fatalf("match %v, want %v", got, tc.want)
			}
			if tc.name == "provenance-regenerated" && sameChangeFacts(original, tc.replacement) {
				t.Fatal("original freshness discarded provenance")
			}
		})
	}
	if !replacementAttributesMatch(nil, map[string][]byte{"com.apple.provenance": {1}}) {
		t.Fatal("OS-generated replacement provenance refused")
	}
	if replacementAttributesMatch(map[string][]byte{"empty": {}}, nil) {
		t.Fatal("empty-valued ordinary attribute loss accepted")
	}
}

func TestReplacementMetadataTamperingRefuses(t *testing.T) {
	for _, mutate := range []string{"attribute", "mode", "flags", "link"} {
		t.Run(mutate, func(t *testing.T) {
			snapshot, id, config := faultFixture(t, false)
			before, err := os.ReadFile(config)
			if err != nil {
				t.Fatal(err)
			}
			preview := snapshot.PreviewChange(context.Background(), ChangeIntent{id, "codex", ControlGlobal, ControlDisable})
			if preview.plan == nil {
				t.Fatal(preview.View())
			}
			preview.plan.fault = func(stage string) error {
				if stage != "file-sync" {
					return nil
				}
				paths, err := filepath.Glob(filepath.Join(filepath.Dir(config), ".skillglass-*.tmp"))
				if err != nil {
					return err
				}
				if len(paths) != 1 {
					t.Fatalf("expected one temporary replacement, got %v", paths)
				}
				switch mutate {
				case "attribute":
					return unix.Setxattr(paths[0], "org.skillglass.fixture", []byte("unexpected"), 0)
				case "mode":
					return os.Chmod(paths[0], 0640)
				case "flags":
					return unix.Chflags(paths[0], unix.UF_NODUMP)
				case "link":
					alias := filepath.Join(filepath.Dir(config), "external-link")
					t.Cleanup(func() { _ = os.Remove(alias) })
					return os.Link(paths[0], alias)
				}
				return nil
			}
			out := preview.Apply(context.Background(), preview.View().Confirmation)
			if out.WriteState != "none" || out.ExitCode() == 0 {
				t.Fatal(out)
			}
			after, err := os.ReadFile(config)
			if err != nil || string(before) != string(after) {
				t.Fatal("original changed", err)
			}
		})
	}
}
