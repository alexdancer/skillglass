package acceptance_test

import (
	"path/filepath"
	"testing"
)

// Verification-only test for HERMES-NATIVE-YAML-ADMISSION. Native startup is
// never invoked: the public viewer runs under the ordinary sealed fixture policy.
func TestVerifyNativeYAMLAdmission(t *testing.T) {
	for _, extension := range []string{"yaml", "yml"} {
		for _, tc := range []struct {
			name, extra     string
			optin, rejected bool
		}{
			{"custom-unconsumed", "extra: !custom inert\n", true, true},
			{"custom-nested-unconsumed", "extra: {nested: [!custom inert]}\n", true, true},
			{"plain-control", "extra: inert\n", true, false},
			{"string-tag-control", "extra: !!str inert\n", true, false},
			{"unconsumed-boolean-control", "extra: yes\n", true, false},
			{"project-optout-control", "extra: !custom inert\n", false, true},
		} {
			t.Run(extension+"/"+tc.name, func(t *testing.T) {
				fx, profile := hermesFixture(t)
				user := filepath.Join(profile, "plugins", "portable")
				project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
				for _, root := range []string{user, project} {
					codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
				}
				writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
				writeFile(t, filepath.Join(project, "plugin."+extension), []byte("name: portable\n"+tc.extra), 0600)
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
				before := treeManifest(t, fx.base)
				defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
				var env []string
				if tc.optin {
					env = []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}
				}
				result := hermesRun(t, fx, env, "list", "--agent", "hermes", "--project", fx.project, "--json")
				var list codexList
				decodeJSON(t, result.stdout, &list)
				u := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
				p := gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
				want := "unavailable"
				if tc.rejected || !tc.optin {
					want = "explicit-only"
				}
				if u.Availability.State != want {
					t.Errorf("user availability=%q, want %q; reason=%s", u.Availability.State, want, u.Availability.Reason)
				}
				if tc.rejected && tc.optin {
					for _, ref := range u.Availability.References {
						if ref.Participation == "excluded" {
							t.Errorf("rejected project displaced admitted user: %+v", ref)
						}
					}
					// Finding requires rejected project inventory to remain visible.
					if p.ID == "" {
						t.Error("rejected project inventory missing")
					}
				}
				if !tc.rejected && tc.optin {
					if p.Availability.State != "unknown" {
						t.Errorf("admitted dynamic project state=%q, want unknown", p.Availability.State)
					}
					for _, ref := range u.Availability.References {
						if ref.Provenance.Kind == "plugin" && ref.Participation != "excluded" {
							t.Errorf("valid project did not exclude user: %+v", ref)
						}
					}
				}
			})
		}
	}
}
