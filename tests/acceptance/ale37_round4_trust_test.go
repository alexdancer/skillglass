package acceptance_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func round4List(t *testing.T, fx fixture, wantExit int, env ...string) (codexList, []byte) {
	t.Helper()
	command := fx.productCommand(t, context.Background(), "list", "--agent", "gemini", "--project", fx.project, "--json")
	command.Dir = fx.project
	command.Env = append(fx.environment(), env...)
	hashes := make(map[string]string)
	for _, path := range []string{skillglassBinary, fx.productProfile(t)} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hashes[path] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	receipt, err := json.Marshal(hashes)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "round4/" + t.Name()
	writeEvidence(t, prefix+"/invocation-hashes.json", receipt)
	before := treeManifest(t, fx.base)
	raw, err := command.Output()
	code := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	after := treeManifest(t, fx.base)
	for name, manifest := range map[string][]byte{"fixture-before": before, "fixture-after": after} {
		writeEvidence(t, prefix+"/"+name+".json", manifest)
	}
	assertTreeUnchanged(t, before, after)
	result := commandResult{code: code, stdout: raw}
	recordEvidenceCommand(t, "public-cli", command.Path, command.Args[1:], command.Dir, result)
	assertExit(t, result, wantExit)
	var list codexList
	decodeJSON(t, raw, &list)
	return list, raw
}

func TestRound4R14AdmissionAndFinalParticipation(t *testing.T) {
	for _, s := range []struct {
		id                                                    string
		user, project, persisted, disabled                    bool
		env                                                   string
		projectState, globalState, admission, evidenceSetting string
	}{
		{"false-to-true", false, true, false, false, "", "unavailable", "ask", "loaded", "trustedFolders"},
		{"admitted-permission-retained", false, true, false, true, "", "unavailable", "disabled", "loaded", "trustedFolders"},
		{"false-to-false", false, false, false, false, "", "ask", "shadowed", "loaded", "security.folderTrust.enabled=false"},
		{"untrusted-cannot-self-admit", true, false, false, true, "", "unavailable", "ask", "ignored", "trustedFolders"},
		{"trusted-true-to-true", true, true, true, false, "", "ask", "shadowed", "loaded", "TRUST_FOLDER"},
		{"false-to-true-persisted", false, true, true, false, "", "ask", "shadowed", "loaded", "TRUST_FOLDER"},
		{"env-permit", false, true, false, false, "GEMINI_CLI_TRUST_WORKSPACE=true", "ask", "shadowed", "loaded", "GEMINI_CLI_TRUST_WORKSPACE=true"},
		{"env-deny", false, true, false, true, "GEMINI_CLI_TRUST_WORKSPACE=false", "unavailable", "ask", "ignored", "GEMINI_CLI_TRUST_WORKSPACE=false"},
		{"restricted-over-permit", false, true, true, true, "GEMINI_RESTRICTED_MODE=true GEMINI_CLI_TRUST_WORKSPACE=true", "unavailable", "ask", "ignored", "GEMINI_RESTRICTED_MODE=true"},
	} {
		t.Run(s.id, func(t *testing.T) {
			fx := newFixture(t)
			user := filepath.Join(fx.home, ".gemini/settings.json")
			project := filepath.Join(fx.project, ".gemini/settings.json")
			trust := filepath.Join(fx.home, ".gemini/trustedFolders.json")
			data, _ := json.Marshal(map[string]any{"security": map[string]any{"folderTrust": map[string]any{"enabled": s.user}}})
			writeFile(t, user, data, 0600)
			config := map[string]any{"security": map[string]any{"folderTrust": map[string]any{"enabled": s.project}}}
			if s.disabled {
				config["skills"] = map[string]any{"disabled": []string{"collision-native"}}
			}
			data, _ = json.Marshal(config)
			writeFile(t, project, data, 0600)
			if s.persisted {
				data, _ = json.Marshal(map[string]string{fx.project: "TRUST_FOLDER"})
				writeFile(t, trust, data, 0600)
			}
			codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "global", "collision-native")
			for _, alias := range []string{".gemini", ".agents"} {
				codexSkill(t, filepath.Join(fx.project, alias, "skills"), "project", "collision-native")
			}
			before := treeManifest(t, fx.base)
			env := []string{}
			if s.env != "" {
				env = append(env, strings.Fields(s.env)...)
			}
			list, raw := round4List(t, fx, 0, env...)
			if len(list.Sources) != 3 {
				t.Fatalf("expected three independent sources, got %d", len(list.Sources))
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			for _, source := range list.Sources {
				want := s.globalState
				if strings.HasPrefix(source.SourcePath, fx.project) {
					want = s.projectState
					if want == "ask" && strings.Contains(source.SourcePath, "/.gemini/") {
						want = "shadowed"
					}
				}
				if source.Availability.State != want {
					t.Errorf("%s: %s, want %s", source.SourcePath, source.Availability.State, want)
				}
				if strings.HasPrefix(source.SourcePath, fx.project) {
					participation := "eligible"
					if s.projectState == "unavailable" {
						participation = "excluded"
					}
					for _, ref := range source.Availability.References {
						if ref.Participation != participation {
							t.Errorf("final participation %s, want %s", ref.Participation, participation)
						}
					}
				} else if s.projectState == "unavailable" && (source.Availability.Precedence.State != "unique" || source.Availability.Precedence.Winner != source.ID) {
					t.Error("excluded project definitions contaminated the global winner")
				}
			}
			found := false
			for _, layer := range list.Scope.Agent.ConfigLayers {
				if layer.Path == project {
					found = true
					if layer.State != s.admission {
						t.Errorf("project admission %s, want %s", layer.State, s.admission)
					}
				}
			}
			if !found {
				t.Error("project admission layer missing")
			}
			var detail struct {
				Sources []struct {
					SourcePath   string `json:"source_path"`
					Availability struct {
						References []struct {
							Evidence struct {
								Setting   string
								InputPath string `json:"input_path"`
							}
						}
					}
				}
			}
			decodeJSON(t, raw, &detail)
			for _, source := range detail.Sources {
				if strings.HasPrefix(source.SourcePath, fx.project) {
					for _, ref := range source.Availability.References {
						e := ref.Evidence
						if e.Setting != s.evidenceSetting {
							t.Errorf("final evidence %s, want %s", e.Setting, s.evidenceSetting)
						}
						if s.id == "false-to-false" && e.InputPath != project {
							t.Errorf("final false setting path %s, want admitted project %s", e.InputPath, project)
						}
					}
				}
			}
		})
	}
}

func TestRound4R14FinalReferenceConsumers(t *testing.T) {
	for _, alias := range []string{".gemini", ".agents"} {
		for _, kind := range []string{"opaque", "shared", "scalar"} {
			t.Run(alias+"/"+kind, func(t *testing.T) {
				fx := geminiFixture(t)
				config := `{"security":{"folderTrust":{"enabled":true}}}`
				if kind == "scalar" {
					config = `{"security":{"folderTrust":{"enabled":true}},"skills":{"enabled":false}}`
				}
				writeFile(t, filepath.Join(fx.project, ".gemini/settings.json"), []byte(config), 0600)
				known := filepath.Dir(codexSkill(t, filepath.Join(fx.home, alias, "skills"), "global", "consumer-native"))
				project := filepath.Join(fx.project, alias, "skills", "project")
				if kind == "shared" {
					if err := os.MkdirAll(filepath.Dir(project), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(known, project); err != nil {
						t.Fatal(err)
					}
				} else {
					manifest := codexSkill(t, filepath.Dir(project), "project", "consumer-native")
					if kind == "opaque" {
						writeFile(t, manifest, []byte("---\nname: consumer-native\ndescription: Native fixture.\n---\n"+strings.Repeat("x", (4<<20)+1)), 0600)
					}
				}
				exit := 0
				if kind == "opaque" {
					exit = 1
				}
				before := treeManifest(t, fx.base)
				list, _ := round4List(t, fx, exit)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				wantCount := 2
				if kind == "shared" {
					wantCount = 1
				}
				if len(list.Sources) != wantCount {
					t.Fatalf("source identity count %d, want %d", len(list.Sources), wantCount)
				}
				for _, source := range list.Sources {
					if source.SourcePath != known {
						if source.Availability.State != "unavailable" {
							t.Errorf("excluded project source state %s", source.Availability.State)
						}
						continue
					}
					want := "ask"
					if kind == "scalar" {
						want = "disabled"
					}
					if source.Availability.State != want || source.Availability.Precedence.State != "unique" || source.Availability.Precedence.Winner != source.ID {
						t.Errorf("global source lost final precedence/permission: %+v", source.Availability)
					}
					if kind == "scalar" && source.Availability.References[0].Permission.Evidence.InputPath != filepath.Join(fx.project, ".gemini/settings.json") {
						t.Error("admitted scalar permission lost project input")
					}
					if kind == "shared" {
						eligible, excluded := 0, 0
						for _, ref := range source.Availability.References {
							if ref.Participation == "eligible" {
								eligible++
							}
							if ref.Participation == "excluded" {
								excluded++
							}
						}
						if eligible != 1 || excluded != 1 {
							t.Errorf("shared reference participation: %d eligible, %d excluded", eligible, excluded)
						}
					}
				}
			})
		}
	}
}

func TestRound4R15NormalizedUTF16Specificity(t *testing.T) {
	for _, s := range []struct {
		id, parentName, projectName, want string
		denyFirst                         bool
	}{
		{"cjk-denial-longer", "資料", "other", "unavailable", true},
		{"cjk-parent-longer", "other", "資料", "ask", true},
		{"ascii-control", "small", "longer", "unavailable", true},
		{"astral-two-unit-control", "😀😀", "abc", "ask", true},
		{"lowercase-expansion-control", "İİ", "abc", "ask", true},
		{"normalized-dot-segments", "long-segment/../x", "abc", "unavailable", true},
		{"utf16-tie-deny-first", "資料", "ab", "unavailable", true},
		{"utf16-tie-allow-first", "資料", "ab", "ask", false},
		{"astral-tie-deny-first", "😀", "ab", "unavailable", true},
		{"astral-tie-allow-first", "😀", "ab", "ask", false},
		{"normalized-collision-length", "long-segment/../work", "work", "unavailable", false},
		{"normalized-collision-reversed", "long-segment/../work", "work", "ask", true},
		{"ordinary-equal-disagreement-control", "xx", "ab", "unavailable", true},
	} {
		t.Run(s.id, func(t *testing.T) {
			fx := newFixture(t)
			scope := filepath.Join(fx.base, "allowed", "projects", "Scope")
			fx.project = filepath.Join(scope, s.projectName)
			if err := os.MkdirAll(fx.project, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(scope, "long-segment"), 0755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":true}}}`), 0600)
			parentRule := scope + "/" + s.parentName
			parentLevel := "TRUST_PARENT"
			if strings.HasPrefix(s.id, "normalized-collision") {
				parentLevel = "TRUST_FOLDER"
			}
			rule := fmt.Sprintf("{%q:%q,%q:%q}", parentRule, parentLevel, fx.project, "DO_NOT_TRUST")
			if s.denyFirst {
				rule = fmt.Sprintf("{%q:%q,%q:%q}", fx.project, "DO_NOT_TRUST", parentRule, parentLevel)
			}
			writeFile(t, filepath.Join(fx.home, ".gemini/trustedFolders.json"), []byte(rule), 0600)
			codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "global", "specificity-native")
			for _, alias := range []string{".gemini", ".agents"} {
				codexSkill(t, filepath.Join(fx.project, alias, "skills"), "project", "specificity-native")
			}
			before := treeManifest(t, fx.base)
			list, _ := round4List(t, fx, 0)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			for _, source := range list.Sources {
				want := s.want
				if !strings.HasPrefix(source.SourcePath, fx.project) {
					want = "ask"
					if s.want == "ask" {
						want = "shadowed"
					}
				}
				if s.want == "ask" && strings.HasPrefix(source.SourcePath, fx.project) && strings.Contains(source.SourcePath, "/.gemini/") {
					want = "shadowed"
				}
				if source.Availability.State != want {
					t.Errorf("%s: %s, want %s", source.SourcePath, source.Availability.State, want)
				}
			}
		})
	}
}
