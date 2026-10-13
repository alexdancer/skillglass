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
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
			opencodeConfig(t, path, tc.config)
			// A valid later layer must not launder a rejected input.
			opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":"allow","plugin":[],"agent":{"build":{"hidden":false}}}`)
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
