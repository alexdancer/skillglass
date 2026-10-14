package acceptance_test

import (
	"path/filepath"
	"testing"
)

func TestVerifyScalarWrapperConstructorConversion(t *testing.T) {
	for _, tc := range []struct{ name, extra, want string }{
		{"bool-invalid-wrapper", "extra: !!bool {=: nonsense}\n", "explicit-only"},
		{"bool-valid-wrapper", "extra: !!bool {=: on}\n", "unavailable"},
		{"int-invalid-wrapper", "extra: !!int {=: inert}\n", "explicit-only"},
		{"int-valid-wrapper", "extra: !!int {=: 42}\n", "unavailable"},
		{"float-invalid-wrapper", "extra: !!float {=: inert}\n", "explicit-only"},
		{"float-valid-wrapper", "extra: !!float {=: 1.5}\n", "unavailable"},
		{"binary-invalid-wrapper", "extra: !!binary {=: abc}\n", "explicit-only"},
		{"binary-valid-wrapper", "extra: !!binary {=: SW5lcnQu}\n", "unavailable"},
		{"bool-mixed-case-valid-wrapper", "extra: !!bool {=: oN}\n", "unavailable"},
		{"binary-nonalphabet-valid-wrapper", "extra: !!binary {=: SW5lcnQu!}\n", "unavailable"},
		{"str-valid-wrapper", "extra: !!str {=: inert}\n", "unavailable"},
		{"null-valid-wrapper", "extra: !!null {=: inert}\n", "unavailable"},
		{"bool-invalid-direct-control", "extra: !!bool nonsense\n", "unknown"},
		{"bool-valid-direct-control", "extra: !!bool true\n", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			user := filepath.Join(profile, "plugins", "portable")
			project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
			for _, path := range []string{user, project} {
				codexSkill(t, filepath.Join(path, "skills"), "explicit", "explicit")
			}
			writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(project, "plugin.yaml"), []byte("name: portable\n"+tc.extra), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
			defer g2Seal(t, fx)()
			list := d2List(t, fx, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			source := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
			t.Logf("user=%s want=%s completeness=%s", source.Availability.State, tc.want, list.Scope.Completeness)
			if source.Availability.State != tc.want {
				t.Errorf("native constructor admission: user availability=%q; want literal %q", source.Availability.State, tc.want)
			}
			wantParticipation := "excluded"
			if tc.want == "explicit-only" || tc.want == "unknown" {
				wantParticipation = "eligible"
			}
			for _, ref := range source.Availability.References {
				if ref.Provenance.Kind == "plugin" {
					t.Logf("user participation=%s reason=%s", ref.Participation, ref.Evidence.Reason)
					if ref.Participation != wantParticipation {
						t.Errorf("native user participation=%q; want literal %q", ref.Participation, wantParticipation)
					}
				}
			}
			result := hermesRun(t, fx, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "explain", "--agent", "hermes", "--project", fx.project, source.ID)
			if result.code != 0 && result.code != 1 {
				t.Fatalf("explain exit=%d", result.code)
			}
		})
	}
}
