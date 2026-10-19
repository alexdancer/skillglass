package acceptance_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

// Join the delivered bundle, two deliberate installations and the collection
// contract. Agent judgment is exercised separately by fresh model evaluations.
func TestALE49DeliveredInstalledReviewWorkflow(t *testing.T) {
	for _, scenario := range []string{"applied", "external-edit", "runtime-failure"} {
		t.Run(scenario, func(t *testing.T) {
			fx := ale48Fixture(t)
			codexSkill(t, filepath.Join(fx.home, ".codex/skills"), "home-canary", "home-native")
			if err := os.MkdirAll(filepath.Join(fx.project, ".git/objects"), 0700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.project, ".git/HEAD"), []byte("ref: refs/heads/fixture\n"), 0600)
			shared := filepath.Join(fx.home, ".agents/skills")
			first := codexSkill(t, shared, "research", "research")
			second := codexSkill(t, shared, "budget", "budget")
			kept := codexSkill(t, shared, "guardrails", "guardrails")
			independent := codexSkill(t, filepath.Join(fx.home, ".codex/skills"), "research-copy", "research")
			if err := os.MkdirAll(filepath.Join(fx.home, ".claude/skills"), 0700); err != nil {
				t.Fatal(err)
			}
			for _, source := range []string{first, second, kept} {
				destination := filepath.Join(fx.home, ".claude/skills", filepath.Base(filepath.Dir(source)))
				if err := os.Symlink(filepath.Dir(source), destination); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(filepath.Dir(first), "references/evidence.md"), []byte("Use primary sources and retain uncertainty.\n"), 0600)
			writeFile(t, filepath.Join(filepath.Dir(first), "scripts/check.sh"), []byte("#!/bin/sh\ntouch "+shellQuote(fx.marker)+"\n"), 0700)
			writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte("# unrelated setting\nmodel = \"fixture\"\n[[skills.config]]\npath = "+fmt.Sprintf("%q", second)+"\nenabled = false\n"), 0600)
			writeFile(t, filepath.Join(fx.home, ".claude/settings.json"), []byte(`{"keep":true,"skillOverrides":{"budget":"off"}}`), 0600)
			setupBefore := treeManifest(t, fx.base)
			// Shared consent does not create an OpenCode or CodeAF installation target.
			args := []string{"setup", "--app", "codex", "--app", "claude-code", "--allow-shared-app", "opencode", "--allow-shared-app", "codeaf", "--json"}
			assertExit(t, ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), args...), 0)
			assertTreeUnchanged(t, setupBefore, treeManifest(t, fx.base))
			install := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), append(args, "--install")...)
			assertExit(t, install, 0)
			for _, root := range []string{".codex/skills", ".claude/skills"} {
				ale48AssertBundle(t, filepath.Join(fx.home, root, "skillglass-review"))
			}
			for _, root := range []string{".agents/skills", ".config/opencode/skills", ".codeaf/skills"} {
				if _, err := os.Stat(filepath.Join(fx.home, root, "skillglass-review")); !os.IsNotExist(err) {
					t.Fatal("unselected setup destination changed", root, err)
				}
			}
			check := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--check", "--json")
			assertExit(t, check, 0)
			ids := []string{changeSourceID(t, fx, first), changeSourceID(t, fx, second), changeSourceID(t, fx, kept), changeSourceID(t, fx, independent)}
			bodies := treeManifest(t, shared)
			private := treeManifest(t, filepath.Join(fx.home, ".codex/skills"))
			claudeSkills := treeManifest(t, filepath.Join(fx.home, ".claude/skills"))
			draft := ale46Draft([]string{"codex", "claude-code"}, collection.ControlGlobal, "")
			draft.Context.KeepEnabled = []collection.SourceID{collection.SourceID(ids[2])}
			draft.Sources = []collection.ReviewAssessment{ale46Assess(ids[0], "references/evidence.md", "scripts/check.sh"), ale46Assess(ids[1]), ale46Assess(ids[2]), ale46Assess(ids[3])}
			draft.Changes = []collection.ReviewRecommendation{
				ale46Change("codex-research", ids[0], "codex", collection.ControlDisable),
				ale46Change("codex-budget", ids[1], "codex", collection.ControlEnable),
				ale46Change("claude-research", ids[0], "claude-code", collection.ControlDisable),
				ale46Change("claude-budget", ids[1], "claude-code", collection.ControlEnable),
				ale46Change("omitted-copy", ids[3], "codex", collection.ControlDisable),
			}
			draftPath := ale46File(t, fx, "draft.json", draft)
			stable := treeManifest(t, fx.base)
			for _, id := range ids {
				assertExit(t, fx.run(t, "read", "--agent", "codex", "--json", id), 0)
			}
			assertExit(t, fx.run(t, "read-resource", "--agent", "codex", "--json", ids[0], "references/evidence.md"), 0)
			assertExit(t, fx.run(t, "read-resource", "--agent", "codex", "--json", ids[0], "scripts/check.sh"), 0)
			p := ale46Prepare(t, fx, draftPath, 0)
			assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
			proposal := ale46File(t, fx, "proposal.json", p)
			stable = treeManifest(t, fx.base)
			chosen := []string{"claude-budget", "claude-research", "codex-budget", "codex-research"}
			var selection collection.ReviewSelection
			decodeJSON(t, ale46Select(t, fx, proposal, chosen, "", 0).stdout, &selection)
			decodeJSON(t, ale46Select(t, fx, proposal, chosen, selection.SelectionID, 0).stdout, &selection)
			assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
			if len(selection.Omitted) != 1 || selection.Omitted[0] != "omitted-copy" || selection.Effects[0].ConfigPath != selection.Effects[1].ConfigPath || selection.Effects[0].AfterSHA256 != selection.Effects[1].BeforeSHA256 {
				t.Fatal("subset or expected shared progression lost", selection)
			}
			path := ale46File(t, fx, "approved.json", selection)
			policy, want := "", 0
			if scenario == "external-edit" {
				config := filepath.Join(fx.home, ".claude/settings.json")
				writeFile(t, config, []byte(`{"keep":true,"external":true,"skillOverrides":{"budget":"off"}}`), 0600)
				want = 1
			}
			if scenario == "runtime-failure" {
				policy = "(deny file-write* (subpath " + sandboxString(filepath.Join(fx.home, ".claude")) + "))"
				want = 1
			}
			beforeApply := treeManifest(t, fx.base)
			out := ale47Apply(t, fx, path, policy, want)
			if len(out.Results) != 4 || len(out.Omitted) != 1 || out.SelectionID != selection.SelectionID {
				t.Fatal(out)
			}
			if scenario == "external-edit" {
				assertTreeUnchanged(t, beforeApply, treeManifest(t, fx.base))
				for _, r := range out.Results {
					if r.State != "not-attempted" {
						t.Fatal("stale batch wrote", out)
					}
				}
			} else {
				for i, r := range out.Results {
					expected := "succeeded"
					if scenario == "runtime-failure" && i == 2 {
						expected = "failed"
					}
					if scenario == "runtime-failure" && i == 3 {
						expected = "not-attempted"
					}
					if r.State != expected || r.Final == nil || r.FinalObservation != "observed" {
						t.Fatal("untruthful result", out)
					}
					if expected == "succeeded" && (r.Outcome == nil || r.Outcome.Persistence != "verified" || r.Outcome.Achievement != "achieved" || r.Outcome.Reload.State == "") {
						t.Fatal("missing persistence/session evidence", r)
					}
				}
				if scenario == "runtime-failure" && (out.Results[2].Outcome == nil || out.Results[2].Outcome.WriteState != "none" || out.Results[3].Outcome != nil) {
					t.Fatal("partial write receipt lost", out)
				}
				if fmt.Sprintf("%x", sha256.Sum256(readBytes(t, selection.Effects[1].ConfigPath))) != selection.Effects[1].AfterSHA256 {
					t.Fatal("approved shared postimage differs")
				}
			}
			for _, app := range []string{"codex", "claude-code"} {
				r := fx.run(t, "list", "--agent", app, "--json")
				assertExit(t, r, 0)
				var view collection.View
				decodeJSON(t, r.stdout, &view)
				for _, source := range view.Sources {
					if string(source.ID) == ids[2] || string(source.ID) == ids[3] {
						if source.Availability.State != collection.AvailabilityAvailable {
							t.Fatal("omitted/kept changed", source)
						}
					}
				}
			}
			assertTreeUnchanged(t, bodies, treeManifest(t, shared))
			assertTreeUnchanged(t, private, treeManifest(t, filepath.Join(fx.home, ".codex/skills")))
			assertTreeUnchanged(t, claudeSkills, treeManifest(t, filepath.Join(fx.home, ".claude/skills")))
			if !bytes.Contains(readBytes(t, filepath.Join(fx.home, ".codex/config.toml")), []byte("# unrelated setting")) || !bytes.Contains(readBytes(t, filepath.Join(fx.home, ".claude/settings.json")), []byte(`"keep":true`)) {
				t.Fatal("unrelated settings lost")
			}
			if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
				t.Fatal("review executed inspected script", err)
			}
			writeEvidence(t, "ale49/"+scenario+"-outcome.json", mustJSON(t, out))
			writeEvidence(t, "ale49/"+scenario+"-before.json", beforeApply)
			writeEvidence(t, "ale49/"+scenario+"-after.json", treeManifest(t, fx.base))
			// Reinspection cannot turn a customized installed copy into an update target.
			custom := filepath.Join(fx.home, ".codex/skills/skillglass-review/SKILL.md")
			writeFile(t, custom, []byte("CUSTOMIZED INSTALLED COPY"), 0600)
			stable = treeManifest(t, fx.base)
			conflict := ale48Run(t, fx, "", filepath.Join(fx.home, "bin"), "setup", "--app", "codex", "--allow-shared-app", "codeaf", "--install", "--json")
			assertExit(t, conflict, 1)
			assertTreeUnchanged(t, stable, treeManifest(t, fx.base))
		})
	}
}
