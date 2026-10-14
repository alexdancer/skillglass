package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyStandardTagAdditionalRootContexts(t *testing.T) {
	for _, extension := range []string{"yaml", "yml"} {
		for _, tc := range []struct{ name, extra, want string }{
			{"scalar-sequence", "extra: !!str [inert]\n", "explicit-only"},
			{"seq-mapping", "extra: !!seq {flag: inert}\n", "explicit-only"},
			{"map-sequence", "extra: !!map [inert]\n", "explicit-only"},
			{"set-sequence", "extra: !!set [inert]\n", "explicit-only"},
			{"omap-mapping", "extra: !!omap {flag: inert}\n", "explicit-only"},
			{"pairs-mapping", "extra: !!pairs {flag: inert}\n", "explicit-only"},
			{"value-outside-key", "extra: =\n", "explicit-only"},
			{"omap-merge-key", "extra: !!omap [{<<: {flag: inert}}]\n", "explicit-only"},
			{"pairs-value-key", "extra: !!pairs [{=: inert}]\n", "explicit-only"},
			{"quoted-value-is-not-wrapper", "extra: !!str {'=': inert}\n", "explicit-only"},
			{"ignored-entry-custom-tag", "extra: !!omap [!custom {flag: inert}]\n", "unavailable"},
			{"ignored-wrapper-fields", "extra: !!str {=: inert, ignored: !custom other}\n", "unavailable"},
			{"ignored-wrapper-value-tag", "extra: !!str {=: !custom inert}\n", "unavailable"},
			{"merge-operand-by-shape", "extra: {<<: !!str {flag: inert}}\n", "unavailable"},
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
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
				defer g2Seal(t, fx)()
				list := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
				source := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
				if source.Availability.State != tc.want {
					t.Fatalf("constructor context: state=%s, want %s", source.Availability.State, tc.want)
				}
				gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
			})
		}
	}
}

func TestVerifyStandardTagAdditionalRootTypes(t *testing.T) {
	for _, extension := range []string{"yaml", "yml"} {
		t.Run(extension, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			user := filepath.Join(profile, "plugins", "category", "portable")
			project := filepath.Join(fx.project, ".hermes", "plugins", "category", "portable")
			for _, root := range []string{user, project} {
				codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
			}
			writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(project, "plugin."+extension), []byte("!!set {name: portable}\n"), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins: {enabled: [category/portable]}\n"), 0600)
			defer g2Seal(t, fx)()
			list := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			source := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
			if source.Availability.State != "explicit-only" {
				t.Fatalf("non-dict root displaced user: %+v", source.Availability)
			}
		})
	}
	for _, location := range []string{"profile", "managed"} {
		t.Run(location, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
			path := filepath.Join(profile, "config.yaml")
			var env []string
			if location == "managed" {
				managed := filepath.Join(fx.home, "managed")
				path = filepath.Join(managed, "config.yaml")
				env = []string{"HERMES_MANAGED_DIR=" + managed}
			}
			writeFile(t, path, []byte("!!set {plugins: {enabled: [portable]}}\n"), 0600)
			defer g2Seal(t, fx)()
			list := hermesList(t, fx, 1, env)
			source := gateSourceAt(t, list, dir)
			if source.Availability.State != "unknown" {
				t.Fatalf("unobserved configuration fallback became known: %+v", source.Availability)
			}
		})
	}
}

func TestVerifyStandardTagAdditionalPolicyContexts(t *testing.T) {
	for _, tc := range []struct{ name, profile, managed, want string }{
		{"managed-map-replaces-profile-set", "plugins: !!set {enabled: [portable], disabled: [portable]}\n", "plugins: {enabled: [portable]}\n", "explicit-only"},
		{"managed-set-discards-profile-denial", "plugins: {enabled: [portable], disabled: [portable]}\n", "plugins: !!set {enabled: [portable]}\n", "unknown"},
		{"managed-empty-map-replaces-set", "plugins: !!set {enabled: [portable]}\n", "plugins: {}\n", "unavailable"},
		{"independent-plugin-denial", "plugins: {enabled: !!set {portable: safe}, disabled: [portable]}\n", "", "unavailable"},
		{"independent-skill-denial", "skills: {disabled: [guarded], platform_disabled: !!set {telegram: [guarded]}}\n", "", "disabled"},
		{"platform-set-ancestor", "skills: {platform_disabled: !!set {telegram: [guarded]}}\n", "", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			if strings.Contains(tc.name, "skill") || strings.HasPrefix(tc.name, "platform") {
				dir = filepath.Join(profile, "skills", "guarded")
				writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(g2Text("guarded", "")), 0600)
			}
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte(tc.profile), 0600)
			env := []string{"HERMES_PLATFORM=telegram"}
			if tc.managed != "" {
				managed := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(managed, "config.yaml"), []byte(tc.managed), 0600)
				env = append(env, "HERMES_MANAGED_DIR="+managed)
			}
			defer g2Seal(t, fx)()
			list := d2List(t, fx, env)
			source := gateSourceAt(t, list, dir)
			if source.Availability.State != tc.want {
				t.Fatalf("policy context: state=%s, want %s; %+v", source.Availability.State, tc.want, source.Availability)
			}
		})
	}
}

func TestVerifyStandardTagAdditionalMetadataContexts(t *testing.T) {
	for _, tc := range []struct{ name, extra, want string }{
		{"value-key-normalization", "metadata: {=: inert}\n", "explicit-only"},
		{"pairs-consumed", "metadata: !!pairs [{flag: inert}]\n", "unknown"},
		{"omap-consumed", "metadata: !!omap [{flag: inert}]\n", "unknown"},
		{"malformed-frontmatter-fallback", "extra: !!omap [inert]\n", "unknown"},
		{"raw-operand-merge-sequence", "metadata: {<<: [!!set {flag: inert}, !!str {other: inert}]}\n", "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
			writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(strings.Replace(g2Text("explicit", tc.extra), "G2_FULL_CONTENT_SENTINEL", "D2_FULL_CONTENT_SENTINEL", 1)), 0600)
			defer g2Seal(t, fx)()
			source := gateSourceAt(t, hermesList(t, fx, 0, nil), dir)
			if source.Availability.State != tc.want {
				t.Fatalf("metadata context: state=%s, want %s", source.Availability.State, tc.want)
			}
			d2Read(t, fx, source, nil)
		})
	}
}
