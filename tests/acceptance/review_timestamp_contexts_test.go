package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewTimestampFallbackContexts(t *testing.T) {
	for _, location := range []string{"profile", "managed", "frontmatter"} {
		for _, tc := range []struct{ name, value, want string }{
			{"native-rejected", "0000-01-01", "unknown"},
			{"valid-control", "0001-01-01", "explicit-only"},
		} {
			t.Run(location+"/"+tc.name, func(t *testing.T) {
				fx, profile := hermesFixture(t)
				dir := d2Plugin(t, fx, profile, "portable")
				config := "plugins: {enabled: [portable]}\n"
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte(config), 0600)
				extra := "extra: &stamp !!timestamp " + tc.value + "\ncopy: *stamp\n"
				var env []string
				switch location {
				case "profile":
					writeFile(t, filepath.Join(profile, "config.yaml"), []byte(config+extra), 0600)
				case "managed":
					managed := filepath.Join(fx.home, "managed")
					writeFile(t, filepath.Join(managed, "config.yaml"), []byte(config+extra), 0600)
					env = []string{"HERMES_MANAGED_DIR=" + managed}
				case "frontmatter":
					writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(g2Text("explicit", extra)), 0600)
				}
				defer g2Seal(t, fx)()
				source := gateSourceAt(t, d2List(t, fx, env), dir)
				if source.Availability.State != tc.want {
					t.Fatalf("timestamp context: state=%s, want %s", source.Availability.State, tc.want)
				}
			})
		}
	}
}

func TestReviewTimestampAliasAdmission(t *testing.T) {
	for _, tc := range []struct{ name, extra, want string }{
		{"valid", "extra: &stamp !!timestamp 2026-01-01T00:00:00+23:59\ncopy: *stamp\n", "unavailable"},
		{"rejected", "extra: &stamp !!timestamp 2026-01-01T00:00:00-24:00\ncopy: *stamp\n", "explicit-only"},
		{"wrapper-control", "extra: !!timestamp {=: 2026-01-01}\n", "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			user := filepath.Join(profile, "plugins", "portable")
			project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
			for _, root := range []string{user, project} {
				codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
			}
			writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(project, "plugin.yaml"), []byte("name: portable\n"+tc.extra), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
			defer g2Seal(t, fx)()
			list := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			source := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
			if source.Availability.State != tc.want {
				t.Fatalf("timestamp alias admission: state=%s, want %s", source.Availability.State, tc.want)
			}
		})
	}
}

func TestReviewConfigDiagnosticsPreserveSkillContent(t *testing.T) {
	fx, profile := hermesFixture(t)
	dir := filepath.Join(profile, "skills", "content-control")
	writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(g2Text("content-control", "")+"\n"+disclosureSentinel+"\n"), 0600)
	defer g2Seal(t, fx)()
	source := gateSourceAt(t, hermesList(t, fx, 0, nil), dir)
	result := hermesRun(t, fx, nil, "read", "--agent", "hermes", "--project", fx.project, source.ID)
	assertExit(t, result, 0)
	if !strings.Contains(string(result.stdout), disclosureSentinel) {
		t.Fatal("intentional skill content was redacted")
	}
}
