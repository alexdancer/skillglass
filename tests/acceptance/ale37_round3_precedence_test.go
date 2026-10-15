package acceptance_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRound3OpaqueNamePrecedence(t *testing.T) {
	for _, alias := range []string{".gemini", ".agents"} {
		for _, kind := range []string{"oversized-higher", "unreadable-higher", "encoding-higher", "readable-higher", "missing-higher", "known-invalid-higher", "oversized-lower", "oversized-same-rank", "excluded-higher", "inspection-only"} {
			t.Run(alias+"/"+kind, func(t *testing.T) {
				fx := geminiFixture(t)
				knownRoot := filepath.Join(fx.home, alias, "skills")
				opaqueRoot := filepath.Join(fx.project, alias, "skills")
				if kind == "oversized-lower" {
					knownRoot = filepath.Join(fx.project, alias, "skills")
					opaqueRoot = filepath.Join(fx.home, alias, "skills")
				}
				if kind == "oversized-same-rank" {
					opaqueRoot = knownRoot
				}
				if kind == "inspection-only" {
					opaqueRoot = fx.cleanRoot
				}
				known := filepath.Dir(codexSkill(t, knownRoot, "known", "collision-native"))
				opaque := ""
				if kind != "missing-higher" {
					manifest := codexSkill(t, opaqueRoot, "opaque-directory", "collision-native")
					opaque = filepath.Dir(manifest)
					if strings.HasPrefix(kind, "oversized") || kind == "excluded-higher" || kind == "inspection-only" {
						writeFile(t, manifest, []byte("---\nname: collision-native\ndescription: Native fixture.\n---\n"+strings.Repeat("x", (4<<20)+1)), 0600)
					}
					if kind == "unreadable-higher" {
						if err := os.Chmod(manifest, 0); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = os.Chmod(manifest, 0600) })
					}
					if kind == "encoding-higher" {
						writeFile(t, manifest, []byte("---\nname: collision-native\ndescription: Native fixture.\n---\n\xff\n"), 0600)
					}
					if kind == "known-invalid-higher" {
						writeFile(t, manifest, []byte("No native frontmatter.\n"), 0600)
					}
				}
				args := []string{"list", "--agent", "gemini", "--project", fx.project, "--json"}
				if kind == "inspection-only" {
					args = append(args, "--root", fx.cleanRoot)
				}
				command := fx.command(t, context.Background(), args...)
				command.Dir = fx.project
				command.Env = fx.environment()
				if kind == "excluded-higher" {
					command.Env = append(command.Env, "GEMINI_CLI_TRUST_WORKSPACE=false")
				}
				before := treeManifest(t, fx.base)
				launch := recordLaunch(t, command)
				raw, err := command.Output()
				code := 0
				if err != nil {
					if e, ok := err.(*exec.ExitError); ok {
						code = e.ExitCode()
					} else {
						t.Fatal(err)
					}
				}
				result := commandResult{code: code, stdout: raw, launch: launch}
				recordEvidenceCommand(t, "public-cli-contained", result)
				exit := 1
				if kind == "readable-higher" || kind == "missing-higher" {
					exit = 0
				}
				assertExit(t, result, exit)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				var list codexList
				decodeJSON(t, raw, &list)
				wantState, wantPrecedence := "ask", "unique"
				if kind == "readable-higher" {
					wantState, wantPrecedence = "shadowed", "shadowed"
				}
				if kind == "oversized-higher" || kind == "unreadable-higher" || kind == "encoding-higher" || kind == "oversized-same-rank" {
					wantState, wantPrecedence = "unknown", "unknown"
				}
				found := false
				for _, source := range list.Sources {
					if source.SourcePath == known {
						found = true
						if source.Availability.State != wantState {
							t.Errorf("known state %s, want %s", source.Availability.State, wantState)
						}
						if source.Availability.Precedence.State != wantPrecedence {
							t.Errorf("known precedence %s, want %s", source.Availability.Precedence.State, wantPrecedence)
						}
						if wantState == "unknown" {
							if source.Availability.Precedence.Winner != "" {
								t.Error("uncertain known source has definitive winner")
							}
							explained := fx.run(t, "explain", "--agent", "gemini", "--project", fx.project, "--json", source.ID)
							assertExit(t, explained, exit)
							var detail struct{ Source codexSource }
							decodeJSON(t, explained.stdout, &detail)
							if detail.Source.ID != source.ID || detail.Source.SourcePath != known || detail.Source.Availability.State != "unknown" || detail.Source.Availability.Precedence.Winner != "" {
								t.Error("explanation lost source identity or precedence uncertainty")
							}
							filtered := geminiListResult(t, fx, exit, "--availability", "unknown")
							retained := false
							for _, candidate := range filtered.Sources {
								retained = retained || candidate.ID == source.ID
							}
							if !retained {
								t.Error("unknown filter omits uncertain known source")
							}
						}
					}
					if source.SourcePath == opaque && kind == "readable-higher" && source.Availability.State != "ask" {
						t.Error("readable project winner not ask")
					}
				}
				if !found {
					t.Fatal("known source not retained")
				}
			})
		}
	}
}
