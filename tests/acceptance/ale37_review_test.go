package acceptance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestALE37ReviewTrustPathCase(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("pinned macOS trust normalization")
	}
	for _, tc := range []struct {
		level   string
		trusted bool
	}{
		{"TRUST_FOLDER", true},
		{"TRUST_PARENT", true},
		{"DO_NOT_TRUST", false},
	} {
		t.Run(tc.level, func(t *testing.T) {
			fx := geminiFixture(t)
			fx.project = filepath.Join(filepath.Dir(fx.project), "MixedCase", "Work")
			if err := os.MkdirAll(fx.project, 0755); err != nil {
				t.Fatal(err)
			}
			user := filepath.Join(fx.home, ".gemini", "settings.json")
			writeFile(t, user, []byte(`{}`), 0600)
			writeFile(t, filepath.Join(fx.project, ".gemini", "settings.json"), []byte(`{"skills":{"enabled":false}}`), 0600)
			codexSkill(t, filepath.Join(fx.home, ".gemini", "skills"), "global", "collision")
			for _, alias := range []string{".gemini", ".agents"} {
				codexSkill(t, filepath.Join(fx.project, alias, "skills"), "project", "collision")
			}
			project, err := filepath.EvalSymlinks(fx.project)
			if err != nil {
				t.Fatal(err)
			}
			rules := map[string]string{strings.ToLower(project): tc.level}
			if tc.level == "DO_NOT_TRUST" {
				rules[filepath.Dir(project)] = "TRUST_FOLDER"
			}
			raw, err := json.Marshal(rules)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(fx.home, ".gemini", "trustedFolders.json"), raw, 0600)
			before := treeManifest(t, fx.base)
			list := geminiListResult(t, fx, 0)
			if len(list.Sources) != 3 {
				t.Fatalf("sources: %+v", list.Sources)
			}
			for _, source := range list.Sources {
				want := "unavailable"
				if strings.HasPrefix(source.SourcePath, fx.home) {
					want = "ask"
				}
				if tc.trusted {
					want = "shadowed"
					if strings.HasPrefix(source.SourcePath, filepath.Join(project, ".agents")) {
						want = "disabled"
					}
				}
				if source.Availability.State != want {
					t.Errorf("%s: state %s, want %s", source.SourcePath, source.Availability.State, want)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE37ReviewEffectiveExtensionPolicy(t *testing.T) {
	for _, field := range []string{"blockGitExtensions", "allowedExtensions"} {
		for _, override := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/override=%t", field, override), func(t *testing.T) {
				fx := geminiFixture(t)
				extension := filepath.Join(fx.home, ".gemini", "extensions", "sample")
				writeFile(t, filepath.Join(extension, "gemini-extension.json"), []byte(`{"name":"sample","version":"1.0.0"}`), 0600)
				codexSkill(t, filepath.Join(extension, "skills"), "skill", "policy-native")
				active, inactive := "true", "false"
				if field == "allowedExtensions" {
					active, inactive = `["sample"]`, `[]`
				}
				writeFile(t, filepath.Join(fx.home, ".gemini", "settings.json"), []byte(fmt.Sprintf(`{"security":{"folderTrust":{"enabled":false},%q:%s}}`, field, active)), 0600)
				if override {
					writeFile(t, filepath.Join(fx.project, ".gemini", "settings.json"), []byte(fmt.Sprintf(`{"security":{%q:%s}}`, field, inactive)), 0600)
				}
				before := treeManifest(t, fx.base)
				list := geminiListResult(t, fx, 0)
				want := "unknown"
				if override {
					want = "ask"
				}
				if source := codexNamed(t, list, "policy-native"); source.Availability.State != want {
					t.Fatalf("effective policy: %+v, want %s", source, want)
				}
				for _, gap := range list.Scope.Agent.Gaps {
					if override && gap == "extension security policy requires unverified install-metadata matching" {
						t.Fatal("overridden policy still reported as a gap")
					}
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE37ReviewTrustEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, env, setting, rule, participation string
	}{
		{"setting", "", "security.folderTrust.enabled=false", "workspace-trust-setting", "eligible"},
		{"env-allow", "GEMINI_CLI_TRUST_WORKSPACE=true", "GEMINI_CLI_TRUST_WORKSPACE=true", "workspace-trust-env", "eligible"},
		{"env-deny", "GEMINI_CLI_TRUST_WORKSPACE=false", "GEMINI_CLI_TRUST_WORKSPACE=false", "workspace-trust-env", "excluded"},
		{"restricted", "GEMINI_RESTRICTED_MODE=true", "GEMINI_RESTRICTED_MODE=true", "workspace-trust-env", "excluded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := geminiFixture(t)
			for _, alias := range []string{".gemini", ".agents"} {
				codexSkill(t, filepath.Join(fx.project, alias, "skills"), "source", "evidence-"+alias)
			}
			command := fx.command(t, context.Background(), "list", "--agent", "gemini", "--project", fx.project, "--json")
			command.Dir = fx.project
			command.Env = fx.environment()
			if tc.env != "" {
				command.Env = append(command.Env, tc.env)
			}
			launch := recordLaunch(t, command)
			raw, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			recordEvidenceCommand(t, "public-cli-contained", commandResult{stdout: raw, launch: launch})
			var output struct {
				Sources []struct {
					Availability struct {
						References []struct {
							Participation string
							Evidence      struct {
								Rule, Setting string
								InputPath     string `json:"input_path"`
							}
							Permission struct {
								Layers []struct {
									Path    string
									Choices []struct {
										Path    string
										Enabled bool
									}
								}
							}
						}
					}
				}
			}
			decodeJSON(t, raw, &output)
			if len(output.Sources) != 2 {
				t.Fatalf("sources: %s", raw)
			}
			wantPath := ""
			if tc.env == "" {
				wantPath = filepath.Join(fx.home, ".gemini", "settings.json")
			}
			for _, source := range output.Sources {
				for _, ref := range source.Availability.References {
					if ref.Participation != tc.participation || ref.Evidence.Rule != tc.rule || ref.Evidence.Setting != tc.setting || ref.Evidence.InputPath != wantPath {
						t.Errorf("trust evidence: %+v", ref)
					}
					found := false
					for _, layer := range ref.Permission.Layers {
						for _, choice := range layer.Choices {
							if layer.Path == filepath.Join(fx.home, ".gemini", "settings.json") && choice.Path == "security.folderTrust.enabled" && !choice.Enabled {
								found = true
							}
						}
					}
					if !found {
						t.Error("folder-trust setting omitted from reported layer choices")
					}
				}
			}
		})
	}
}
