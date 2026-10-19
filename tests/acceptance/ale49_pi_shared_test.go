package acceptance_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE49PiProjectSharedConsumers(t *testing.T) {
	for _, action := range []collection.ControlChoice{collection.ControlEnable, collection.ControlDisable} {
		t.Run(string(action), func(t *testing.T) {
			fx, id, _ := ale40Fixture(t, false)
			writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), []byte(`{"keep":true,"defaultProjectTrust":"always"}`), 0600)
			writeFile(t, filepath.Join(fx.project, ".pi/settings.json"), []byte(`{"keep":true}`), 0600)
			writeFile(t, filepath.Join(fx.project, ".claude/settings.json"), []byte(`{"keep":true}`), 0600)
			draft, _ := ale47Draft(t, fx, id, []string{"pi", "claude-code"})
			draft.Context.Scope, draft.Context.Project = collection.ControlProject, fx.project
			for i := range draft.Changes {
				draft.Changes[i].Action = action
			}
			selection, path := ale47Approved(t, fx, draft, []string{"pi-first", "pi-second", "claude-code-first"})
			home := treeManifest(t, fx.home)
			out := ale47Apply(t, fx, path, "", 0)
			if out.State != "applied" || len(out.Results) != 3 || len(out.Omitted) != 1 || out.Omitted[0] != "claude-code-second" {
				t.Fatalf("%+v", out)
			}
			for i, r := range out.Results {
				if r.Outcome == nil || r.Outcome.Persistence != "verified" || r.Outcome.Achievement != "achieved" || r.FinalObservation != "observed" || r.Final == nil || r.State != "succeeded" && r.State != "no-op" {
					t.Fatalf("%+v", r)
				}
				if action == collection.ControlDisable && r.Final.State != collection.AvailabilityDisabled || action == collection.ControlEnable && r.Final.State != collection.AvailabilityAvailable {
					t.Fatalf("wrong final state: %+v", r)
				}
				if i == 1 || i == 2 {
					effect := selection.Effects[i]
					if fmt.Sprintf("%x", sha256.Sum256(readBytes(t, effect.ConfigPath))) != effect.AfterSHA256 {
						t.Fatal("saved shared configuration differs from approved postimage")
					}
				}
			}
			assertTreeUnchanged(t, home, treeManifest(t, fx.home))
		})
	}
}
