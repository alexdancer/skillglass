package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyALE31YAMLAdmissionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, location, extra, want string
	}{
		{"profile-unused-tag", "profile", "extra: !custom inert\n", "unknown"},
		{"profile-nested-tag", "profile", "extra: {nested: [!custom inert]}\n", "unknown"},
		{"profile-string-control", "profile", "extra: !!str inert\n", "explicit-only"},
		{"managed-unused-tag", "managed", "extra: !custom inert\n", "unknown"},
		{"managed-nested-tag", "managed", "extra: {nested: [!custom inert]}\n", "unknown"},
		{"managed-string-control", "managed", "extra: !!str inert\n", "explicit-only"},
		{"index-fallback-unknown", "index", "extra: !custom inert\n", "unknown"},
		{"portable-fallback-unknown", "portable", "extra: !custom inert\n", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			config := "plugins:\n  enabled: [portable]\n"
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte(config), 0600)
			var env []string
			switch tc.location {
			case "profile":
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte(config+tc.extra), 0600)
			case "managed":
				managed := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(managed, "config.yaml"), []byte(config+tc.extra), 0600)
				env = []string{"HERMES_MANAGED_DIR=" + managed}
			case "index":
				dir = filepath.Join(profile, "skills", "explicit")
				fallthrough
			case "portable":
				writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(g2Text("explicit", tc.extra)), 0600)
			}
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			exit := 0
			if (tc.location == "profile" || tc.location == "managed") && tc.want == "unknown" {
				exit = 1
			}
			list := hermesList(t, fx, exit, env)
			source := gateSourceAt(t, list, dir)
			if source.Availability.State != tc.want {
				t.Fatalf("availability=%s, want %s: %+v", source.Availability.State, tc.want, source.Availability)
			}
			if exit == 1 && list.Scope.Completeness != "incomplete" {
				t.Fatal("configuration fallback uncertainty missing")
			}
		})
	}
}

func TestVerifyALE31YAMLAdmissionDeepUnusedFields(t *testing.T) {
	for _, tc := range []struct{ name, value, want string }{
		{"supported", "inert", "unavailable"},
		{"unsupported", "!custom inert", "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			user := filepath.Join(profile, "plugins", "portable")
			project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
			for _, root := range []string{user, project} {
				codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
			}
			writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			manifest := "name: portable\nextra: " + strings.Repeat("[", 140) + tc.value + strings.Repeat("]", 140) + "\n"
			writeFile(t, filepath.Join(project, "plugin.yaml"), []byte(manifest), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			source := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
			if source.Availability.State != tc.want {
				t.Fatalf("bounded field confidence altered root admission: %+v", source.Availability)
			}
		})
	}
}

func TestVerifyALE31ConsumedScalarBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, extra, want string }{
		{"binary-alias", "value: &v !!binary SW5lcnQu\ndescription: *v\n", "unknown"},
		{"sexagesimal-alias", "value: &v 1:20\ndescription: *v\n", "unknown"},
		{"sexagesimal-float", "description: -1:20.5\n", "unknown"},
		{"sexagesimal-multicomponent", "description: +1_2:03:59\n", "unknown"},
		{"sexagesimal-not-numeric", "description: 1:60\n", "explicit-only"},
		{"sexagesimal-zero-string", "description: 0:20\n", "explicit-only"},
		{"sexagesimal-block-string", "description: |\n  1:20\n", "explicit-only"},
		{"binary-merged-value", "defaults: &d {flag: !!binary SW5lcnQu}\nmetadata: {<<: *d}\n", "unknown"},
		{"binary-overridden-value", "defaults: &d {flag: !!binary SW5lcnQu}\nmetadata: {<<: *d, flag: 'safe'}\n", "explicit-only"},
		{"sexagesimal-overridden-value", "defaults: &d {flag: 1:20}\nmetadata: {<<: *d, flag: 'safe'}\n", "explicit-only"},
		{"sexagesimal-key", "metadata: {1:20: value}\n", "unknown"},
		{"sexagesimal-key-distinct-from-string", "defaults: &d {1:20: value}\nmetadata: {<<: *d, '1:20': quoted}\n", "unknown"},
		{"binary-key-distinct-from-string", "defaults: &d {!!binary ZmxhZw==: value}\nmetadata: {<<: *d, flag: quoted}\n", "unknown"},
		{"quoted-key-control", "metadata: {'1:20': value}\n", "explicit-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			text := strings.Replace(g2Text("explicit", tc.extra), "G2_FULL_CONTENT_SENTINEL", "D2_FULL_CONTENT_SENTINEL", 1)
			if strings.Contains(tc.extra, "description:") {
				text = strings.Replace(text, "description: Inert.\n", "", 1)
			}
			writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(text), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			source := gateSourceAt(t, hermesList(t, fx, 0, nil), dir)
			if source.Availability.State != tc.want {
				t.Fatalf("availability=%s, want %s: %+v", source.Availability.State, tc.want, source.Availability)
			}
			d2Read(t, fx, source, nil)
		})
	}
}

func TestVerifyALE31ScalarPolicyBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, profile, managed, want string }{
		{"binary-enabled", "plugins: {enabled: [!!binary cG9ydGFibGU=]}\n", "", "unknown"},
		{"binary-disabled", "plugins: {enabled: [portable], disabled: [!!binary cG9ydGFibGU=]}\n", "", "unknown"},
		{"independent-plugin-denial", "plugins: {enabled: [!!binary cG9ydGFibGU=], disabled: [portable]}\n", "", "unavailable"},
		{"managed-replacement", "plugins: {enabled: [!!binary cG9ydGFibGU=]}\n", "plugins: {enabled: [portable]}\n", "explicit-only"},
		{"inherited-uncertainty", "plugins: {enabled: [!!binary cG9ydGFibGU=]}\n", "plugins: {disabled: []}\n", "unknown"},
		{"managed-binary", "plugins: {enabled: [portable]}\n", "plugins: {enabled: [!!binary cG9ydGFibGU=]}\n", "unknown"},
		{"raw-binary-denial", "plugins: {enabled: [portable]}\nskills: {disabled: [!!binary ZXhwbGljaXQ=]}\n", "", "unknown"},
		{"raw-independent-denial", "plugins: {enabled: [portable]}\nskills: {disabled: [explicit], platform_disabled: {telegram: [!!binary ZXhwbGljaXQ=]}}\n", "", "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			if strings.HasPrefix(tc.name, "raw-") {
				dir = filepath.Join(profile, "skills", "explicit")
				writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(g2Text("explicit", "")), 0600)
			}
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte(tc.profile), 0600)
			env := []string{"HERMES_PLATFORM=telegram"}
			if tc.managed != "" {
				managed := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(managed, "config.yaml"), []byte(tc.managed), 0600)
				env = append(env, "HERMES_MANAGED_DIR="+managed)
			}
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			source := gateSourceAt(t, hermesList(t, fx, 0, env), dir)
			if source.Availability.State != tc.want {
				t.Fatalf("availability=%s, want %s: %+v", source.Availability.State, tc.want, source.Availability)
			}
		})
	}
}

func TestVerifyALE31ScalarRegistryBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, category, manifest, userState string
		versionUnknown                      bool
	}{
		{"binary-flat-name", "", "name: !!binary cG9ydGFibGU=\n", "explicit-only", false},
		{"binary-category-name", "category", "name: !!binary cG9ydGFibGU=\n", "unavailable", false},
		{"binary-version", "", "name: portable\nversion: !!binary SW5lcnQu\n", "unavailable", true},
		{"sexagesimal-version", "", "name: portable\nversion: 1:20\n", "unavailable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			key := filepath.Join(tc.category, "portable")
			user := filepath.Join(profile, "plugins", key)
			project := filepath.Join(fx.project, ".hermes", "plugins", key)
			for _, root := range []string{user, project} {
				codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
			}
			writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(project, "plugin.yaml"), []byte(tc.manifest), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: ['"+key+"']\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			source := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
			if source.Availability.State != tc.userState {
				t.Fatalf("identity/admission conflated: %+v", source.Availability)
			}
			other := gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
			for _, ref := range other.Availability.References {
				if tc.versionUnknown && ref.Provenance.Kind == "plugin" && ref.Provenance.Version != "" {
					t.Fatalf("unsupported version published: %+v", ref.Provenance)
				}
			}
		})
	}
}

func TestVerifyALE31ScalarIndexWindow(t *testing.T) {
	fx, profile := hermesFixture(t)
	dir := filepath.Join(profile, "skills", "explicit")
	text := "---\nname: explicit\n" + strings.Repeat("# padding\n", 410) + "description: 1:20\n---\nD2_FULL_CONTENT_SENTINEL\n"
	writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(text), 0600)
	before := treeManifest(t, fx.base)
	defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
	source := gateSourceAt(t, hermesList(t, fx, 0, nil), dir)
	if source.Availability.State != "available" || source.Availability.NativeName != "explicit" {
		t.Fatalf("full description tainted empty index window: %+v", source.Availability)
	}
	d2Read(t, fx, source, nil)
}
