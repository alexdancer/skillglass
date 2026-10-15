package acceptance_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestALE30ProjectGlobalSettingsOverlap(t *testing.T) {
	for _, layout := range []string{"home", "project-directory-alias", "global-directory-alias", "project-ancestor-alias", "missing-directory", "disjoint", "disjoint-missing-directories"} {
		for _, saved := range []string{"absent", "on", "off"} {
			t.Run(layout+"/"+saved, func(t *testing.T) {
				fx, repo := codexFixture(t)
				global := filepath.Join(fx.home, ".claude")
				link := func(from, to string) {
					t.Helper()
					if err := os.Symlink(to, from); err != nil {
						t.Fatal(err)
					}
				}
				switch layout {
				case "home":
					fx.project = fx.home
				case "project-directory-alias":
					link(filepath.Join(fx.project, ".claude"), global)
				case "global-directory-alias":
					if err := os.Mkdir(filepath.Join(fx.project, ".claude"), 0700); err != nil {
						t.Fatal(err)
					}
					global = filepath.Join(fx.home, "configured-claude")
					link(global, filepath.Join(fx.project, ".claude"))
				case "project-ancestor-alias":
					alias := filepath.Join(fx.base, "allowed", "home-alias")
					link(alias, fx.home)
					fx.project = alias
				case "missing-directory":
					global = filepath.Join(fx.project, ".claude")
				case "disjoint-missing-directories":
					global = filepath.Join(fx.project, "configured-claude")
				}
				skills := filepath.Join(global, "skills")
				if layout == "missing-directory" || layout == "disjoint-missing-directories" {
					skills = filepath.Join(repo, ".claude", "skills")
				}
				target := claudeSkill(t, skills, "target", "ale30-target", "")
				config := filepath.Join(global, "settings.json")
				if saved != "absent" {
					writeFile(t, config, []byte("{\"skillOverrides\":{\"ale30-target\":\""+saved+"\"},\"keep\":true}\n"), 0600)
				}
				run := func(t *testing.T, args ...string) commandResult {
					t.Helper()
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					argv := fx.launchArgs(t, skillglassBinary, true, "", args...)
					cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
					cmd.Env = append(fx.environment(), "CLAUDE_CONFIG_DIR="+global)
					cmd.Dir = fx.project
					result, err := executeFinalizedCommand(t, cmd)
					if ctx.Err() != nil {
						t.Fatal("scope overlap command timed out")
					}
					if err != nil {
						if _, ok := err.(*exec.ExitError); !ok {
							t.Fatal(err)
						}
					}
					recordEvidenceCommand(t, "claude-scope-overlap", result)
					return result
				}
				listed := run(t, "list", "--agent", "claude-code", "--project", fx.project, "--json")
				assertExit(t, listed, 0)
				var list codexList
				decodeJSON(t, listed.stdout, &list)
				source := claudeAt(t, list, filepath.Dir(target))
				overlap := !strings.HasPrefix(layout, "disjoint")
				for _, choice := range []string{"enable", "disable", "inherit"} {
					t.Run(choice, func(t *testing.T) {
						before := treeManifest(t, fx.base)
						args := []string{choice, "--agent", "claude-code", "--project", fx.project, "--scope", "project", "--json", source.ID}
						result := run(t, args...)
						var preview changeReceipt
						decodeJSON(t, result.stdout, &preview)
						if overlap {
							assertExit(t, result, 1)
							if preview.State != "refused" || preview.Confirmation != "" || !strings.Contains(preview.Reason, "global settings") {
								t.Fatalf("scope overlap accepted: %+v", preview)
							}
							assertTreeUnchanged(t, before, treeManifest(t, fx.base))
						} else {
							assertExit(t, result, 0)
							if preview.State != "ready" {
								t.Fatalf("disjoint project refused: %+v", preview)
							}
							globalBefore, beforeErr := os.ReadFile(config)
							confirmed := append(append([]string{}, args[:len(args)-1]...), "--confirm", preview.Confirmation, source.ID)
							applied := run(t, confirmed...)
							assertExit(t, applied, 0)
							var outcome changeReceipt
							decodeJSON(t, applied.stdout, &outcome)
							assertSavedChange(t, outcome)
							globalAfter, afterErr := os.ReadFile(config)
							if string(globalBefore) != string(globalAfter) || os.IsNotExist(beforeErr) != os.IsNotExist(afterErr) {
								t.Fatal("project control changed global settings")
							}
						}
					})
				}
				projectCapabilities := 0
				for _, capability := range source.Availability.Capabilities {
					if capability.Scope == "project" {
						projectCapabilities++
						want := "supported"
						if overlap {
							want = "unsupported"
						}
						if capability.State != want {
							t.Errorf("project capability %+v, want %s", capability, want)
						}
					}
				}
				if projectCapabilities != 2 {
					t.Fatalf("project capability count = %d, want 2", projectCapabilities)
				}
			})
		}
	}
}

func TestALE30ProjectAliasRetargetToGlobalRefused(t *testing.T) {
	for _, choice := range []string{"enable", "disable", "inherit"} {
		t.Run(choice, func(t *testing.T) {
			fx, _ := codexFixture(t)
			id, _ := claudeTarget(t, fx)
			global := filepath.Join(fx.home, ".claude")
			writeFile(t, filepath.Join(global, "settings.json"), []byte("{\"skillOverrides\":{\"ale30-target\":\"off\"}}\n"), 0600)
			separate := filepath.Join(fx.project, "separate-config")
			writeFile(t, filepath.Join(separate, "settings.json"), []byte("{\"skillOverrides\":{\"ale30-target\":\"on\"}}\n"), 0600)
			alias := filepath.Join(fx.project, ".claude")
			if err := os.Symlink(separate, alias); err != nil {
				t.Fatal(err)
			}
			preview := claudeChange(t, fx, choice, "project", id, "", 0)
			if preview.State != "ready" || preview.Confirmation == "" {
				t.Fatalf("disjoint preview: %+v", preview)
			}
			if err := os.Remove(alias); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(global, alias); err != nil {
				t.Fatal(err)
			}
			before := treeManifest(t, fx.base)
			out := claudeChange(t, fx, choice, "project", id, preview.Confirmation, 1)
			if out.State != "refused" || !strings.Contains(out.Reason, "global settings") {
				t.Fatalf("retargeted consent accepted: %+v", out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
