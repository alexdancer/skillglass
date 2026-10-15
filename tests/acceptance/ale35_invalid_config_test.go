package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestALE35RejectedNativeInputsRemainUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		valid        bool
	}{
		{"absent", `{}`, true},
		{"empty-permission", `{"permission":{}}`, true},
		{"valid-false", `{"plugin":[],"agent":{"build":{"hidden":false}}}`, true},
		{"permission-null", `{"permission":null}`, false},
		{"agent-permission-null", `{"agent":{"build":{"permission":null}}}`, false},
		{"nested-permission-null", `{"permission":{"skill":null}}`, false},
		{"plugin-false", `{"plugin":false}`, false},
		{"plugin-null", `{"plugin":null}`, false},
		{"hidden-string", `{"agent":{"build":{"hidden":"false"}}}`, false},
		{"hidden-null", `{"agent":{"build":{"hidden":null}}}`, false},
		{"tools-null", `{"tools":null}`, false},
		{"agent-tools-null", `{"agent":{"build":{"tools":null}}}`, false},
		{"disable-string", `{"agent":{"build":{"disable":"false"}}}`, false},
		{"mode-invalid", `{"agent":{"build":{"mode":"invalid"}}}`, false},
		{"agent-null", `{"agent":null}`, false},
		{"default-agent-null", `{"default_agent":null}`, false},
		{"tools-scalar", `{"tools":true}`, false},
		{"tools-array", `{"tools":[]}`, false},
		{"tools-value-null", `{"tools":{"skill":null}}`, false},
		{"tools-value-string", `{"tools":{"skill":"false"}}`, false},
		{"agent-entry-null", `{"agent":{"unused":null}}`, false},
		{"unused-agent-disable-null", `{"agent":{"unused":{"disable":null}}}`, false},
		{"disable-null", `{"agent":{"build":{"disable":null}}}`, false},
		{"mode-null", `{"agent":{"build":{"mode":null}}}`, false},
		{"default-agent-array", `{"default_agent":[]}`, false},
		{"skills-null", `{"skills":null}`, false},
		{"skills-paths-null", `{"skills":{"paths":null}}`, false},
		{"skills-urls-null", `{"skills":{"urls":null}}`, false},
		{"skills-paths-scalar", `{"skills":{"paths":"fixture"}}`, false},
		{"skills-urls-item-null", `{"skills":{"urls":[null]}}`, false},
		{"legacy-mode-null", `{"mode":null}`, false},
		{"empty-seams", `{"tools":{},"agent":{},"skills":{"paths":[],"urls":[]},"default_agent":"build"}`, true},
		{"agent-valid-flags", `{"agent":{"build":{"tools":{},"hidden":false,"disable":false,"mode":"primary"}}}`, true},
		{"agent-mode-all", `{"agent":{"build":{"mode":"all"}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
			opencodeConfig(t, path, tc.config)
			// A valid later layer must not launder a rejected input.
			opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"tools":{"skill":true},"permission":"allow","plugin":[],"skills":{"paths":[],"urls":[]},"default_agent":"build","agent":{"build":{"tools":{},"hidden":false,"disable":false,"mode":"primary"},"unused":{"disable":false}}}`)
			before := treeManifest(t, fx.base)
			result := fx.run(t, "list", "--agent", "opencode", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			source := codexNamed(t, list, "safe")
			wantExit, wantState, wantCoverage := 1, "unknown", "incomplete"
			if tc.valid {
				wantExit, wantState, wantCoverage = 0, "available", "complete"
			}
			assertExit(t, result, wantExit)
			if source.Availability.State != wantState || list.Scope.Completeness != wantCoverage {
				t.Fatalf("state=%s coverage=%s; want %s/%s", source.Availability.State, list.Scope.Completeness, wantState, wantCoverage)
			}
			if !tc.valid {
				found := false
				var detailed struct{ Problems []publicProblem }
				decodeJSON(t, result.stdout, &detailed)
				for _, p := range detailed.Problems {
					if p.Code == "native-input" && p.Path == path {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing invalid input provenance for %s: %#v", path, list.Problems)
				}
				for _, ref := range source.Availability.References {
					if ref.Permission.State != "unknown" {
						t.Fatalf("permission=%s", ref.Permission.State)
					}
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE35RejectedInterpretedLayersAndProjectExclusion(t *testing.T) {
	for _, route := range []string{"project", "custom", "inline", "managed", "runtime-permission", "skipped-project"} {
		t.Run(route, func(t *testing.T) {
			fx, _ := codexFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			path := filepath.Join(fx.project, "opencode.json")
			env := []string{}
			bad := `{"tools":null}`
			switch route {
			case "custom":
				path = filepath.Join(fx.base, "allowed", "custom.json")
				env = append(env, "OPENCODE_CONFIG="+path)
			case "inline":
				path = "OPENCODE_CONFIG_CONTENT"
				env = append(env, "OPENCODE_CONFIG_CONTENT="+bad)
			case "managed":
				managed := filepath.Join(fx.base, "allowed", "managed")
				path = filepath.Join(managed, "opencode.json")
				env = append(env, "OPENCODE_TEST_MANAGED_CONFIG_DIR="+managed)
			case "runtime-permission":
				path = "OPENCODE_PERMISSION"
				env = append(env, "OPENCODE_PERMISSION=null")
			case "skipped-project":
				env = append(env, "OPENCODE_DISABLE_PROJECT_CONFIG=true")
			}
			if route != "inline" && route != "runtime-permission" {
				opencodeConfig(t, path, bad)
			}
			if route == "project" || route == "custom" {
				env = append(env, `OPENCODE_CONFIG_CONTENT={"tools":{"skill":true}}`)
			}
			before := treeManifest(t, fx.base)
			result := opencodeEnvRun(t, fx, env, "list", "--agent", "opencode", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			source := codexNamed(t, list, "safe")
			if route == "skipped-project" {
				assertExit(t, result, 0)
				if source.Availability.State != "available" || list.Scope.Completeness != "complete" {
					t.Fatal("excluded project config tainted the scope")
				}
			} else {
				assertExit(t, result, 1)
				if source.Availability.State != "unknown" || list.Scope.Completeness != "incomplete" {
					t.Fatal("rejected loaded layer lost uncertainty")
				}
				for _, ref := range source.Availability.References {
					if ref.Permission.State != "unknown" {
						t.Fatalf("permission=%s", ref.Permission.State)
					}
				}
				var detailed struct{ Problems []publicProblem }
				decodeJSON(t, result.stdout, &detailed)
				found := false
				for _, problem := range detailed.Problems {
					found = found || problem.Code == "native-input" && problem.Path == path
				}
				if !found {
					t.Fatalf("rejected input path %s missing", path)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
