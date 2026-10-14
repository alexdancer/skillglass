package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyStandardTagRootAdmission(t *testing.T) {
	for _, ext := range []string{"yaml", "yml"} {
		for _, tc := range []struct {
			name, extra, want string
			optin             bool
		}{
			{"omap-scalar-entry", "extra: !!omap [inert]\n", "explicit-only", true},
			{"pairs-scalar-entry", "extra: !!pairs [inert]\n", "explicit-only", true},
			{"str-map-node", "extra: !!str {flag: inert}\n", "explicit-only", true},
			{"omap-multi-entry", "extra: !!omap [{flag: safe, other: safe}]\n", "explicit-only", true},
			{"pairs-empty-entry", "extra: !!pairs [{}]\n", "explicit-only", true},
			{"invalid-overridden-unused", "defaults: &d {flag: !!omap [inert]}\nextra: {<<: *d, flag: safe}\n", "explicit-only", true},
			{"misplaced-merge-tag", "extra: !!merge '<<'\n", "explicit-only", true},
			{"valid-omap-ignored-entry-tag", "extra: !!omap [!!str {flag: inert}]\n", "unavailable", true},
			{"valid-pairs-ignored-entry-tag", "extra: !!pairs [!!set {flag: inert}]\n", "unavailable", true},
			{"valid-unused-omap", "extra: !!omap [{flag: safe}]\n", "unavailable", true},
			{"valid-unused-pairs", "extra: !!pairs [{flag: safe}]\n", "unavailable", true},
			{"valid-unused-set", "extra: !!set {flag: safe}\n", "unavailable", true},
			{"valid-value-wrapper-string", "extra: !!str {=: inert}\n", "unavailable", true},
			{"valid-unused-map-seq", "extra: !!map {flag: !!seq [safe]}\n", "unavailable", true},
			{"valid-omap-alias-entry", "value: &v {flag: safe}\nextra: !!omap [*v]\n", "unavailable", true},
			{"optout-control", "extra: !!omap [inert]\n", "explicit-only", false},
		} {
			t.Run(ext+"/"+tc.name, func(t *testing.T) {
				fx, p := hermesFixture(t)
				user := filepath.Join(p, "plugins", "portable")
				project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
				for _, r := range []string{user, project} {
					codexSkill(t, filepath.Join(r, "skills"), "explicit", "explicit")
				}
				writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
				writeFile(t, filepath.Join(project, "plugin."+ext), []byte("name: portable\n"+tc.extra), 0600)
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
				defer g2Seal(t, fx)()
				var env []string
				if tc.optin {
					env = []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}
				}
				r := hermesRun(t, fx, env, "list", "--agent", "hermes", "--project", fx.project, "--json")
				var list codexList
				decodeJSON(t, r.stdout, &list)
				u := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
				gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
				t.Logf("user=%s want=%s", u.Availability.State, tc.want)
				if u.Availability.State != tc.want {
					t.Errorf("user state=%q, want %q", u.Availability.State, tc.want)
				}
				for _, ref := range u.Availability.References {
					if ref.Provenance.Kind == "plugin" {
						want := "excluded"
						if tc.want == "explicit-only" {
							want = "eligible"
						}
						if ref.Participation != want {
							t.Errorf("native user participation=%q want=%q; reason=%s", ref.Participation, want, ref.Evidence.Reason)
						}
					}
				}
			})
		}
	}
}

func TestVerifyStandardTagConsumedContainers(t *testing.T) {
	for _, tc := range []struct{ name, extra, want string }{
		{"metadata-set", "metadata: !!set {flag: safe}\n", "unknown"},
		{"metadata-set-alias", "value: &v !!set {flag: safe}\nmetadata: *v\n", "unknown"},
		{"metadata-set-override-within-set", "defaults: &d {flag: initial}\nmetadata: !!set {<<: *d, flag: safe}\n", "unknown"},
		{"metadata-map-control", "metadata: !!map {flag: safe}\n", "explicit-only"},
		{"unused-set-control", "extra: !!set {flag: safe}\n", "explicit-only"},
		{"unused-omap-control", "extra: !!omap [{flag: safe}]\n", "explicit-only"},
		{"unused-pairs-control", "extra: !!pairs [{flag: safe}]\n", "explicit-only"},
		{"effective-metadata-override-control", "defaults: &d {metadata: !!set {flag: initial}}\n<<: *d\nmetadata: {flag: safe}\n", "explicit-only"},
		{"set-merge-into-map-control", "defaults: &d !!set {flag: safe}\nmetadata: {<<: *d}\n", "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			dir := d2Plugin(t, fx, p, "portable")
			writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(strings.Replace(g2Text("explicit", tc.extra), "G2_FULL_CONTENT_SENTINEL", "D2_FULL_CONTENT_SENTINEL", 1)), 0600)
			writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
			defer g2Seal(t, fx)()
			s := gateSourceAt(t, d2List(t, fx, nil), dir)
			t.Logf("portable=%s want=%s", s.Availability.State, tc.want)
			if s.Availability.State != tc.want {
				t.Errorf("portable state=%q want=%q", s.Availability.State, tc.want)
			}
			d2Read(t, fx, s, nil)
		})
	}
}

func TestVerifyStandardTagConfigAncestors(t *testing.T) {
	for _, tc := range []struct{ name, profile, managed, want string }{
		{"profile-plugin-set", "plugins: !!set {enabled: [portable]}\n", "", "unknown"},
		{"managed-plugin-set", "plugins: {enabled: [portable]}\n", "plugins: !!set {enabled: [portable]}\n", "unknown"},
		{"profile-plugin-map-control", "plugins: !!map {enabled: !!seq [portable]}\n", "", "explicit-only"},
		{"managed-replacement-control", "plugins: {enabled: [portable]}\n", "plugins: !!map {enabled: !!seq [portable]}\n", "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			dir := d2Plugin(t, fx, p, "portable")
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(tc.profile), 0600)
			var env []string
			if tc.managed != "" {
				m := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(m, "config.yaml"), []byte(tc.managed), 0600)
				env = []string{"HERMES_MANAGED_DIR=" + m}
			}
			defer g2Seal(t, fx)()
			r := hermesRun(t, fx, env, "list", "--agent", "hermes", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, r.stdout, &list)
			s := gateSourceAt(t, list, dir)
			t.Logf("policy=%s want=%s", s.Availability.State, tc.want)
			if s.Availability.State != tc.want {
				t.Errorf("policy state=%q want=%q", s.Availability.State, tc.want)
			}
		})
	}
}

func TestVerifyStandardTagWholeRootSet(t *testing.T) {
	fx, p := hermesFixture(t)
	user := filepath.Join(p, "plugins", "category", "portable")
	project := filepath.Join(fx.project, ".hermes", "plugins", "category", "portable")
	for _, r := range []string{user, project} {
		codexSkill(t, filepath.Join(r, "skills"), "explicit", "explicit")
	}
	writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
	writeFile(t, filepath.Join(project, "plugin.yaml"), []byte("!!set {name: portable}\n"), 0600)
	writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins: {enabled: [category/portable]}\n"), 0600)
	defer g2Seal(t, fx)()
	list := d2List(t, fx, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
	u := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
	gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
	t.Logf("whole-root-set user=%s want=explicit-only", u.Availability.State)
	if u.Availability.State != "explicit-only" {
		t.Errorf("native non-dict root displaced user: %s", u.Availability.State)
	}
	for _, r := range u.Availability.References {
		if r.Provenance.Kind == "plugin" && r.Participation != "eligible" {
			t.Errorf("non-dict root participation=%s want=eligible; %s", r.Participation, r.Evidence.Reason)
		}
	}
}
