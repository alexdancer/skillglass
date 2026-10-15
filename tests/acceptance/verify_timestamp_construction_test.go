package acceptance_test

import (
	"path/filepath"
	"testing"
)

// Bounded public proof for native timestamp construction range, not a parser test.
func TestVerifyTimestampConstruction(t *testing.T) {
	for _, ext := range []string{"yaml", "yml"} {
		for _, tc := range []struct{ name, value, want string }{
			{"year-zero", "0000-01-01", "explicit-only"},
			{"offset-plus24", "2026-01-01T00:00:00+24:00", "explicit-only"},
			{"offset-minus24", "2026-01-01T00:00:00-24:00", "explicit-only"},
			{"minimum-year-control", "0001-01-01", "unavailable"},
			{"maximum-year-control", "9999-12-31", "unavailable"},
			{"positive-offset-control", "2026-01-01T00:00:00+23:59", "unavailable"},
			{"negative-offset-control", "2026-01-01T00:00:00-23:59", "unavailable"},
			{"utc-control", "2026-01-01T00:00:00Z", "unavailable"},
			{"date-control", "2026-10-14", "unavailable"},
		} {
			t.Run(ext+"/"+tc.name, func(t *testing.T) {
				fx, p := hermesFixture(t)
				user := filepath.Join(p, "plugins", "portable")
				project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
				for _, dir := range []string{user, project} {
					codexSkill(t, filepath.Join(dir, "skills"), "explicit", "explicit")
				}
				writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
				writeFile(t, filepath.Join(project, "plugin."+ext), []byte("name: portable\nextra: !!timestamp "+tc.value+"\n"), 0600)
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
				defer g2Seal(t, fx)()
				list := d2List(t, fx, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
				u := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
				gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
				t.Logf("user=%s want=%s timestamp=%s", u.Availability.State, tc.want, tc.value)
				if u.Availability.State != tc.want {
					t.Errorf("native timestamp construction admission: user=%q want=%q", u.Availability.State, tc.want)
				}
				for _, ref := range u.Availability.References {
					if ref.Provenance.Kind == "plugin" {
						want := "excluded"
						if tc.want == "explicit-only" {
							want = "eligible"
						}
						if ref.Participation != want {
							t.Errorf("user participation=%q want=%q; %s", ref.Participation, want, ref.Evidence.Reason)
						}
					}
				}
			})
		}
	}
}
