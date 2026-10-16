package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestALE35LegacyConfigPresence(t *testing.T) {
	for _, route := range []string{"absent-control", "present-legacy-deny", "empty", "project-override", "custom-override", "inline-override", "project-disabled", "pure", "broken", "inaccessible", "fifo", "directory", "project-extensionless-control", "custom-directory-extensionless-control"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			legacy := filepath.Join(fx.xdgConfig, "opencode", "config")
			// Valid JSON remains a positive control even with later inputs or exclusions.
			opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"permission":{"skill":"allow"}}`)
			env := []string{}
			absent := strings.HasSuffix(route, "control")
			if !absent {
				writeFile(t, legacy, []byte("[permission]\nskill = \"deny\"\n"), 0600)
			}
			switch route {
			case "empty":
				writeFile(t, legacy, []byte{}, 0600)
			case "project-override":
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":"allow"}}`)
			case "custom-override":
				custom := filepath.Join(fx.base, "allowed", "custom.json")
				opencodeConfig(t, custom, `{"permission":{"skill":"allow"}}`)
				env = append(env, "OPENCODE_CONFIG="+custom)
			case "inline-override":
				env = append(env, `OPENCODE_CONFIG_CONTENT={"permission":{"skill":"allow"}}`)
			case "project-disabled":
				env = append(env, "OPENCODE_DISABLE_PROJECT_CONFIG=true")
			case "pure":
				env = append(env, "OPENCODE_PURE=true")
			case "broken", "inaccessible", "fifo", "directory":
				if err := os.Remove(legacy); err != nil {
					t.Fatal(err)
				}
				switch route {
				case "broken":
					if err := os.Symlink(filepath.Join(fx.base, "allowed", "missing"), legacy); err != nil {
						t.Fatal(err)
					}
				case "inaccessible":
					target := filepath.Join(fx.base, "denied", "legacy-private")
					writeFile(t, target, []byte("PRIVATE_NOT_READ"), 0600)
					if err := os.Symlink(target, legacy); err != nil {
						t.Fatal(err)
					}
				case "fifo":
					if err := syscall.Mkfifo(legacy, 0600); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Mkdir(legacy, 0700); err != nil {
						t.Fatal(err)
					}
				}
			case "project-extensionless-control":
				opencodeConfig(t, filepath.Join(fx.project, "config"), "not a recognized project input")
			case "custom-directory-extensionless-control":
				custom := filepath.Join(fx.base, "allowed", "custom-dir")
				opencodeConfig(t, filepath.Join(custom, "config"), "not a recognized custom input")
				env = append(env, "OPENCODE_CONFIG_DIR="+custom)
			}
			before := treeManifest(t, fx.base)
			result := opencodeEnvRun(t, fx, env, "list", "--agent", "opencode", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			source := codexNamed(t, list, "safe")
			var detail struct {
				Problems []publicProblem
				Scope    struct {
					Agent struct {
						ConfigLayers []struct {
							Path, Scope, State string
							Evidence           struct {
								Kind, Rule string
								InputPath  string `json:"input_path"`
							}
						} `json:"config_layers"`
					}
				}
			}
			decodeJSON(t, result.stdout, &detail)
			writeEvidence(t, "legacy/"+route+".json", result.stdout)
			writeEvidence(t, "legacy/"+route+"-tree-before.json", before)
			found := false
			for _, layer := range detail.Scope.Agent.ConfigLayers {
				if layer.Path != legacy {
					continue
				}
				found = true
				state := "unknown"
				if absent {
					state = "absent"
				}
				if layer.Scope != "global" || layer.State != state || layer.Evidence.InputPath != legacy || layer.Evidence.Rule != "config-layer" {
					t.Fatalf("legacy layer=%+v", layer)
				}
				if (route == "present-legacy-deny" || route == "empty") && layer.Evidence.Kind != "observed" {
					t.Fatalf("presence not observed: %+v", layer)
				}
				if route == "broken" && layer.Evidence.Kind != "unknown" {
					t.Fatalf("inspection uncertainty lost: %+v", layer)
				}
			}
			if !found {
				t.Error("missing actual global legacy config layer")
			}
			state, permission, coverage, exit := "unknown", "unknown", "incomplete", 1
			if absent {
				state, permission, coverage, exit = "available", "allowed", "complete", 0
			}
			assertExit(t, result, exit)
			if source.Availability.State != state || source.Availability.References[0].Permission.State != permission || list.Scope.Completeness != coverage {
				t.Fatalf("availability=%s permission=%s coverage=%s", source.Availability.State, source.Availability.References[0].Permission.State, list.Scope.Completeness)
			}
			found = false
			for _, problem := range detail.Problems {
				found = found || problem.Code == "native-input" && problem.Path == legacy
			}
			if found == absent {
				t.Errorf("legacy native-input problem present=%t", found)
			}
			for _, cap := range source.Availability.Capabilities {
				if (!absent || cap.Operation == "delete") && cap.State != "unsupported" {
					t.Fatal("enabled writer with unknown legacy input or deletion", cap)
				}
				if absent && cap.Operation == "disable" && cap.State != "supported" {
					t.Fatal("validated writer missing for known local inputs", cap)
				}
			}
			read := opencodeEnvRun(t, fx, env, "read", "--agent", "opencode", "--project", fx.project, source.ID)
			assertExit(t, read, exit)
			if !strings.Contains(string(read.stdout), "Inert instructions") {
				t.Fatal("known source no longer inspectable")
			}
			after := treeManifest(t, fx.base)
			writeEvidence(t, "legacy/"+route+"-tree-after.json", after)
			assertTreeUnchanged(t, before, after)
		})
	}
}
