package collection

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Deterministic interruption after staging/sync cannot be induced through the
// executable interface. Ordinary write denial and stale confirmation use it.
func TestOpencodeChangeWriteFaultOutcomes(t *testing.T) {
	for _, stage := range []string{"write", "rename", "directory-sync", "postread"} {
		t.Run(stage, func(t *testing.T) {
			_, _, codexConfig := faultFixture(t, false)
			home := filepath.Dir(filepath.Dir(codexConfig))
			configHome := filepath.Join(home, "config")
			target := filepath.Join(configHome, "opencode/opencode.json")
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			before := []byte("{ // retained\n\"keep\":123}\n")
			if err := os.WriteFile(target, before, 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
			t.Setenv("OPENCODE_TEST_MANAGED_CONFIG_DIR", filepath.Join(home, "managed"))
			request, err := NewRequestWithAgent(nil, nil, nil, "opencode")
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := Build(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.order) != 1 {
				t.Fatal("fixture source set", snapshot.order)
			}
			preview := snapshot.PreviewChange(context.Background(), ChangeIntent{snapshot.order[0], "opencode", ControlGlobal, ControlDisable})
			if preview.plan == nil {
				t.Fatal(preview.View())
			}
			preview.plan.fault = func(current string) error {
				if current == stage {
					return errors.New("deterministic fixture interruption")
				}
				return nil
			}
			out := preview.Apply(context.Background(), preview.View().Confirmation)
			if out.State != "failed" || out.ExitCode() != 1 {
				t.Fatal(out)
			}
			saved, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "directory-sync" || stage == "postread" {
				if out.WriteState != "committed" || !bytes.Equal(saved, preview.plan.postimage) {
					t.Fatal("committed state hidden", out)
				}
				if stage == "directory-sync" && (out.Persistence != "verified" || out.PersistedPermission != PermissionDenied) {
					t.Fatal(out)
				}
			} else if !bytes.Equal(saved, before) {
				t.Fatal("precommit fault changed output")
			}
		})
	}
}
