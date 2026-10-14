package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecision2ALE31NativeAdmissionIdentityControls(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, yaml          string
		nameUnknown, versionUnknown bool
	}{
		{"category-name", "category", "name: on\nversion: '1'\n", true, false},
		{"flat-version", "", "name: portable\nversion: off\n", false, true},
		{"flat-version-bool", "", "name: portable\nversion: false\n", false, true},
		{"category-version", "category", "name: portable\nversion: off\n", false, true},
		{"quoted-version", "", "name: portable\nversion: 'off'\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			key := filepath.Join(tc.prefix, "portable")
			userRoot := filepath.Join(profile, "plugins", key)
			projectRoot := filepath.Join(fx.project, ".hermes", "plugins", key)
			user := codexSkill(t, filepath.Join(userRoot, "skills"), "explicit", "explicit")
			project := codexSkill(t, filepath.Join(projectRoot, "skills"), "explicit", "explicit")
			writeFile(t, filepath.Join(userRoot, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
			writeFile(t, filepath.Join(projectRoot, "plugin.yaml"), []byte(tc.yaml), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: ['"+key+"']\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			u := gateSourceAt(t, list, filepath.Dir(user))
			p := gateSourceAt(t, list, filepath.Dir(project))
			if u.Availability.State != "unavailable" || p.Availability.State != "unknown" {
				t.Fatalf("admission must survive non-key uncertainty: user=%+v project=%+v", u, p)
			}
			if tc.nameUnknown && (p.Availability.NativeName != "" || p.Availability.NativeNameEvidence.Kind != "unknown") {
				t.Fatalf("unsupported name became a native string: %+v", p.Availability)
			}
			for _, ref := range p.Availability.References {
				if ref.Provenance.Kind == "plugin" {
					if ref.Provenance.PluginID != key {
						t.Fatalf("known registry key lost: %+v", ref.Provenance)
					}
					if tc.versionUnknown && ref.Provenance.Version != "" {
						t.Fatalf("unsupported version published: %+v", ref.Provenance)
					}
					if !tc.versionUnknown && !tc.nameUnknown && ref.Provenance.Version != "off" {
						t.Fatalf("quoted version changed: %+v", ref.Provenance)
					}
				}
			}
		})
	}
}

func TestDecision2ALE31RawPythonWhitespaceControls(t *testing.T) {
	for _, field := range []string{"disabled", "platform", "external"} {
		for _, listValue := range []bool{false, true} {
			for _, space := range []string{"\u001c", "\u00a0"} {
				t.Run(fmt.Sprintf("%s/list=%t/space=%U", field, listValue, []rune(space)[0]), func(t *testing.T) {
					fx, profile := hermesFixture(t)
					root := filepath.Join(profile, "skills")
					value := "guarded"
					if field == "external" {
						root, value = filepath.Join(profile, "shared"), "shared"
					}
					file := codexSkill(t, root, "guarded", "guarded")
					value = fmt.Sprintf("%q", space+value+space)
					if listValue {
						value = "[" + value + "]"
					}
					config := "skills:\n  disabled: " + value + "\n"
					env := []string(nil)
					if field == "platform" {
						config = "skills:\n  platform_disabled:\n    telegram: " + value + "\n"
						env = []string{"HERMES_PLATFORM=telegram"}
					} else if field == "external" {
						config = "skills:\n  external_dirs: " + value + "\n"
					}
					writeFile(t, filepath.Join(profile, "config.yaml"), []byte(config), 0600)
					before := treeManifest(t, fx.base)
					defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
					s := gateSourceAt(t, hermesList(t, fx, 0, env), filepath.Dir(file))
					want := "disabled"
					if field == "external" {
						want = "available"
					}
					if s.Availability.State != want {
						t.Fatalf("state=%s; want %s", s.Availability.State, want)
					}
				})
			}
		}
	}
}

func TestDecision2ALE31SelectiveConfidenceControls(t *testing.T) {
	for _, tc := range []struct{ name, config, extra, state string }{
		{"unconsumed-profile", "ignored: on\nplugins:\n  ignored: off\n", "", "available"},
		{"unselected-platform", "skills:\n  platform_disabled:\n    off: [on]\n    telegram: []\n", "", "available"},
		{"platform-empty-tag", "", "platforms: [' ', windows]\n", "available"},
		{"merged-unsupported", "defaults: &defaults {disabled: [off]}\nskills:\n  <<: *defaults\n", "", "unknown"},
		{"explicit-overrides-merge", "defaults: &defaults {disabled: [off]}\nskills:\n  <<: *defaults\n  disabled: []\n", "", "available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			file := codexSkill(t, filepath.Join(profile, "skills"), "guarded", "guarded")
			writeFile(t, file, []byte(strings.Replace(d2Text(tc.extra), "collision", "guarded", 1)), 0600)
			if tc.config != "" {
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte(tc.config), 0600)
			}
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			s := gateSourceAt(t, hermesList(t, fx, 0, []string{"HERMES_PLATFORM=telegram"}), filepath.Dir(file))
			if s.Availability.State != tc.state {
				t.Fatalf("state=%s; want %s", s.Availability.State, tc.state)
			}
		})
	}
}

func TestDecision2ALE31ManagedKnownLeafReplacesUnsupportedProfile(t *testing.T) {
	fx, profile := hermesFixture(t)
	dir := d2Plugin(t, fx, profile, "portable")
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [off]\n"), 0600)
	managed := filepath.Join(fx.home, "managed")
	writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n  ignored: on\n"), 0600)
	before := treeManifest(t, fx.base)
	defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
	s := gateSourceAt(t, hermesList(t, fx, 0, []string{"HERMES_MANAGED_DIR=" + managed}), dir)
	if s.Availability.State != "explicit-only" {
		t.Fatalf("known managed leaf lost: %+v", s.Availability)
	}
}

func TestDecision2ALE31KnownExclusionAndUnknownCompetitor(t *testing.T) {
	for _, excluded := range []bool{false, true} {
		t.Run(fmt.Sprintf("excluded=%t", excluded), func(t *testing.T) {
			fx, profile := hermesFixture(t)
			a := codexSkill(t, filepath.Join(profile, "skills"), "a", "on")
			b := codexSkill(t, filepath.Join(profile, "shared"), "b", "certain")
			extra := ""
			if excluded {
				extra = "platforms: [windows]\n"
			}
			writeFile(t, a, []byte("---\nname: on\ndescription: Inert.\n"+extra+"---\nD2_FULL_CONTENT_SENTINEL\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := hermesList(t, fx, 0, nil)
			first := gateSourceAt(t, list, filepath.Dir(a))
			second := gateSourceAt(t, list, filepath.Dir(b))
			if excluded {
				if first.Availability.State != "unavailable" || second.Availability.State != "available" || second.Availability.Precedence.Winner != second.ID {
					t.Fatalf("known exclusion was erased: first=%+v second=%+v", first.Availability, second.Availability)
				}
			} else if second.Availability.State != "unknown" || second.Availability.Precedence.Winner != "" {
				t.Fatalf("uncertain earlier identity was dropped: %+v", second.Availability)
			}
		})
	}
}

func TestDecision2ALE31PortableMetadataKeys(t *testing.T) {
	for _, quoted := range []bool{false, true} {
		t.Run(fmt.Sprintf("quoted=%t", quoted), func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			key := "on"
			if quoted {
				key = "'on'"
			}
			writeFile(t, filepath.Join(dir, "SKILL.md"), []byte("---\nname: explicit\ndescription: Inert.\nmetadata: {"+key+": value}\n---\nD2_FULL_CONTENT_SENTINEL\n"), 0600)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			s := gateSourceAt(t, hermesList(t, fx, 0, nil), dir)
			want := "unknown"
			if quoted {
				want = "explicit-only"
			}
			if s.Availability.State != want {
				t.Fatalf("metadata key state=%s; want %s", s.Availability.State, want)
			}
			d2Read(t, fx, s, nil)
		})
	}
}

func TestDecision2ALE31EnvironmentAliasPhysicalIdentity(t *testing.T) {
	fx, profile := hermesFixture(t)
	file := codexSkill(t, filepath.Join(profile, "skills"), "a", "collision")
	writeFile(t, file, []byte(d2Text("environments: [docker]\n")), 0600)
	if err := os.Symlink(filepath.Dir(file), filepath.Join(profile, "shared", "alias")); err != nil {
		t.Fatal(err)
	}
	other := codexSkill(t, filepath.Join(profile, "shared"), "z", "collision")
	before := treeManifest(t, fx.base)
	defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
	list := hermesList(t, fx, 0, nil)
	s := gateSourceAt(t, list, filepath.Dir(file))
	o := gateSourceAt(t, list, filepath.Dir(other))
	if len(list.Sources) != 2 || len(s.Availability.References) != 2 || s.Availability.Precedence.Winner != "" || o.Availability.State != "unknown" || len(o.Availability.Precedence.RelatedSources) != 1 {
		t.Fatalf("alias identity or uncertain competition lost: first=%+v other=%+v", s, o)
	}
}
