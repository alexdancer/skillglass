package acceptance_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func d2List(t *testing.T, fx fixture, env []string) codexList {
	t.Helper()
	r := hermesRun(t, fx, env, "list", "--agent", "hermes", "--project", fx.project, "--json")
	if r.code != 0 && r.code != 1 {
		t.Fatalf("unexpected exit %d: %s", r.code, r.stderr)
	}
	var list codexList
	decodeJSON(t, r.stdout, &list)
	return list
}
func d2Read(t *testing.T, fx fixture, s codexSource, env []string) {
	t.Helper()
	r := hermesRun(t, fx, env, "read", "--agent", "hermes", "--project", fx.project, s.ID)
	if !strings.Contains(string(r.stdout), "D2_FULL_CONTENT_SENTINEL") {
		t.Error("full-content read lost")
	}
}
func d2Text(extra string) string {
	return "---\nname: collision\ndescription: Inert.\n" + extra + "---\nD2_FULL_CONTENT_SENTINEL\n"
}
func TestDecision2ALE31EnvironmentEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, extra           string
		sameRoot, long, first bool
	}{
		{"profile-external-index-env", "environments: [docker]\n", false, false, true},
		{"same-root-index-env", "environments: [docker]\n", true, false, true},
		{"no-env-control", "", false, false, true},
		{"env-after-index-window-control", "environments: [docker]\n", false, true, true},
		{"conditional-prompt-only-control", "metadata:\n  hermes:\n    requires_tools: [terminal]\n", false, false, true},
		{"later-env-known-first-control", "environments: [docker]\n", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			root := filepath.Join(p, "skills")
			a := codexSkill(t, root, "a", "collision")
			broot := filepath.Join(p, "shared")
			if tc.sameRoot {
				broot = root
			}
			b := codexSkill(t, broot, "z", "collision")
			text := d2Text(tc.extra)
			if tc.long {
				text = gateWindowText("---\nname: collision\ndescription: Inert.\npadding: ", "x", 4001, false, false)
				text = strings.Replace(text, "\n---\nFULL_CONTENT_READ_SENTINEL", "\n"+tc.extra+"---\nD2_FULL_CONTENT_SENTINEL", 1)
				writeFile(t, b, []byte(strings.Replace(d2Text(""), "collision", "a", 1)), 0600)
			}
			target := a
			if !tc.first {
				target = b
			}
			writeFile(t, target, []byte(text), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := d2List(t, fx, nil)
			s := gateSourceAt(t, list, filepath.Dir(a))
			other := gateSourceAt(t, list, filepath.Dir(b))
			d2Read(t, fx, gateSourceAt(t, list, filepath.Dir(target)), nil)
			uncertain := strings.Contains(tc.name, "index-env")
			if uncertain {
				for _, v := range []codexSource{s, other} {
					if v.Availability.Precedence.Winner != "" || v.Availability.Precedence.State != "unknown" || v.Availability.State == "shadowed" || v.Availability.State == "available" {
						t.Errorf("uncertain earlier eligibility asserts effective winner/shadow: %+v", v.Availability)
					}
					if v.Availability.Invocation.State != "ambiguous-bare-name" || len(v.Availability.Precedence.RelatedSources) != 1 {
						t.Errorf("known explicit-load collision or independent identity lost: %+v", v.Availability)
					}
				}
				if !tc.sameRoot {
					explain := hermesRun(t, fx, nil, "explain", "--agent", "hermes", "--project", fx.project, other.ID)
					assertExit(t, explain, 0)
					if !strings.Contains(string(explain.stdout), "Precedence: unknown") || strings.Contains(string(explain.stdout), "Winning index source:") {
						t.Fatalf("human explanation claimed a winner: %s", explain.stdout)
					}
					filtered := hermesList(t, fx, 0, nil, "--availability", "unknown")
					if len(filtered.Sources) != 2 {
						t.Fatalf("unknown filter lost possible definitions: %+v", filtered.Sources)
					}
				}
			} else if tc.long {
				if other.Availability.Precedence.Winner != s.ID || other.Availability.State != "shadowed" {
					t.Errorf("out-of-window env corrupted known index fallback winner: %+v", other.Availability)
				}
			} else {
				if other.Availability.Precedence.Winner != s.ID {
					t.Errorf("known earlier eligibility lost winner: %+v", other.Availability)
				}
			}
		})
	}
}
func TestDecision2ALE31RawSingletonNormalization(t *testing.T) {
	for _, field := range []string{"disabled", "platform", "external"} {
		for _, list := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/list=%t", field, list), func(t *testing.T) {
				fx, p := hermesFixture(t)
				dir := filepath.Join(p, "skills")
				name := "guarded"
				if field == "external" {
					dir = filepath.Join(p, "shared")
				}
				file := codexSkill(t, dir, name, name)
				writeFile(t, file, []byte(strings.Replace(d2Text(""), "collision", name, 1)), 0600)
				val := "' guarded '"
				if field == "external" {
					val = "' shared '"
				}
				if list {
					val = "[" + val + "]"
				}
				cfg := "skills:\n  disabled: " + val + "\n"
				env := []string(nil)
				if field == "platform" {
					cfg = "skills:\n  platform_disabled:\n    telegram: " + val + "\n"
					env = []string{"HERMES_PLATFORM=telegram"}
				}
				if field == "external" {
					cfg = "skills:\n  external_dirs: " + val + "\n"
				}
				writeFile(t, filepath.Join(p, "config.yaml"), []byte(cfg), 0600)
				before := treeManifest(t, fx.base)
				defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
				inv := d2List(t, fx, env)
				s := gateSourceAt(t, inv, filepath.Dir(file))
				want := "disabled"
				if field == "external" {
					want = "available"
				}
				if s.Availability.State != want {
					t.Errorf("state=%s; want %s", s.Availability.State, want)
				}
				if inv.Scope.Completeness != "complete" {
					t.Errorf("valid normalized scalar/list lost complete discovery: %s", inv.Scope.Completeness)
				}
				d2Read(t, fx, s, env)
			})
		}
	}
}
func d2Plugin(t *testing.T, fx fixture, p, name string) string {
	t.Helper()
	dir := filepath.Join(p, "plugins", name)
	writeFile(t, filepath.Join(dir, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"`+name+`"}`), 0600)
	f := codexSkill(t, filepath.Join(dir, "skills"), "explicit", "explicit")
	writeFile(t, f, []byte("---\nname: explicit\ndescription: Inert.\n---\nD2_FULL_CONTENT_SENTINEL\n"), 0600)
	return filepath.Dir(f)
}
func TestDecision2ALE31PluginPolicyExactControls(t *testing.T) {
	for _, tc := range []struct{ name, cfg, want string }{{"spaces-enabled", "plugins:\n  enabled: [' portable ']\n", "unavailable"}, {"spaces-disabled", "plugins:\n  enabled: [portable]\n  disabled: [' portable ']\n", "explicit-only"}} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			dir := d2Plugin(t, fx, p, "portable")
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(tc.cfg), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			s := gateSourceAt(t, d2List(t, fx, nil), dir)
			if s.Availability.State != tc.want {
				t.Errorf("exact plugin membership: %s; want %s", s.Availability.State, tc.want)
			}
		})
	}
}
func TestDecision2ALE31ManifestScalarParity(t *testing.T) {
	for _, token := range []string{"on", "off", "yes", "no", "ON", "Off"} {
		for _, quoted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/quoted=%t", token, quoted), func(t *testing.T) {
				fx, p := hermesFixture(t)
				f := codexSkill(t, filepath.Join(p, "skills"), "bundle", "bundle")
				val := token
				if quoted {
					val = "'" + token + "'"
				}
				writeFile(t, f, []byte("---\nname: "+val+"\ndescription: Inert.\n---\nD2_FULL_CONTENT_SENTINEL\n"), 0600)
				before := treeManifest(t, fx.base)
				defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
				s := gateSourceAt(t, d2List(t, fx, nil), filepath.Dir(f))
				want := "unknown"
				if quoted {
					want = "available"
				}
				if s.Availability.State != want {
					t.Errorf("native scalar confidence state=%s; want %s", s.Availability.State, want)
				}
				if !quoted && s.Availability.Precedence.Winner != "" {
					t.Error("unresolved native name published winner")
				}
				if token == "on" && !quoted {
					repairHermesPublicParity(t, fx, s, "unknown", "Availability: unknown", nil)
				}
				d2Read(t, fx, s, nil)
			})
		}
	}
}
func TestDecision2ALE31PluginPolicyScalarParity(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, disabled := range []bool{false, true} {
			for _, quoted := range []bool{false, true} {
				t.Run(fmt.Sprintf("managed=%t/disabled=%t/quoted=%t", managed, disabled, quoted), func(t *testing.T) {
					fx, p := hermesFixture(t)
					dir := d2Plugin(t, fx, p, "off")
					val := "off"
					if quoted {
						val = "'off'"
					}
					cfg := "plugins:\n  enabled: [" + val + "]\n"
					if disabled {
						cfg = "plugins:\n  enabled: ['off']\n  disabled: [" + val + "]\n"
					}
					path := filepath.Join(p, "config.yaml")
					env := []string(nil)
					if managed {
						m := filepath.Join(fx.home, "managed")
						path = filepath.Join(m, "config.yaml")
						env = []string{"HERMES_MANAGED_DIR=" + m}
					}
					writeFile(t, path, []byte(cfg), 0600)
					before := treeManifest(t, fx.base)
					defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
					s := gateSourceAt(t, d2List(t, fx, env), dir)
					want := "unknown"
					if quoted {
						want = "explicit-only"
						if disabled {
							want = "unavailable"
						}
					}
					if s.Availability.State != want {
						t.Errorf("policy scalar state=%s; want %s", s.Availability.State, want)
					}
					d2Read(t, fx, s, env)
				})
			}
		}
	}
}
func TestDecision2ALE31NativeYamlKeyParity(t *testing.T) {
	for _, quoted := range []bool{false, true} {
		t.Run(fmt.Sprintf("quoted=%t", quoted), func(t *testing.T) {
			fx, p := hermesFixture(t)
			user := d2Plugin(t, fx, p, "on")
			project := filepath.Join(fx.project, ".hermes", "plugins", "on")
			f := codexSkill(t, filepath.Join(project, "skills"), "explicit", "explicit")
			val := "on"
			if quoted {
				val = "'on'"
			}
			writeFile(t, filepath.Join(project, "plugin.yaml"), []byte("name: "+val+"\nversion: '1'\n"), 0600)
			writeFile(t, f, []byte("---\nname: explicit\ndescription: Inert.\n---\nD2_FULL_CONTENT_SENTINEL\n"), 0600)
			writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: ['on']\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			inv := d2List(t, fx, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			s := gateSourceAt(t, inv, user)
			if !quoted {
				for _, r := range s.Availability.References {
					if r.Participation == "excluded" {
						t.Errorf("unresolved YAML bool key displaced JSON string key: %+v", r)
					}
				}
			} else if s.Availability.State != "unavailable" {
				t.Errorf("quoted YAML exact key must replace JSON control: %+v", s.Availability)
			}
			if !quoted {
				repairHermesPublicParity(t, fx, s, "explicit-only", "Availability: explicit-only", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			}
			d2Read(t, fx, s, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
		})
	}
}
func TestDecision2ALE31PortableScalarParity(t *testing.T) {
	for _, field := range []string{"description", "license", "compatibility", "allowed-tools", "metadata"} {
		for _, quoted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/quoted=%t", field, quoted), func(t *testing.T) {
				fx, p := hermesFixture(t)
				dir := d2Plugin(t, fx, p, "portable")
				val := "on"
				if quoted {
					val = "'on'"
				}
				extra := field + ": " + val + "\n"
				desc := "description: Inert.\n"
				if field == "description" {
					desc = ""
				}
				if field == "metadata" {
					extra = "metadata: {flag: " + val + "}\n"
				}
				writeFile(t, filepath.Join(dir, "SKILL.md"), []byte("---\nname: explicit\n"+desc+extra+"---\nD2_FULL_CONTENT_SENTINEL\n"), 0600)
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
				before := treeManifest(t, fx.base)
				defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
				s := gateSourceAt(t, d2List(t, fx, nil), dir)
				want := "unknown"
				if quoted {
					want = "explicit-only"
				}
				if s.Availability.State != want {
					t.Errorf("portable YAML scalar state=%s; want %s", s.Availability.State, want)
				}
				d2Read(t, fx, s, nil)
			})
		}
	}
}

func TestDecision2ALE31ConsumedYamlSiblingConfidence(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, config string
		external               bool
		env                    []string
		want                   string
	}{
		{"index-description", "---\nname: guarded\ndescription: on\n---\nD2_FULL_CONTENT_SENTINEL\n", "", false, nil, "unknown"},
		{"index-platform-false", "---\nname: guarded\ndescription: Inert.\nplatforms: off\n---\nD2_FULL_CONTENT_SENTINEL\n", "", false, nil, "unknown"},
		{"raw-disabled-list", "", "skills:\n  disabled: [off]\n", false, nil, "unknown"},
		{"raw-platform-value", "", "skills:\n  platform_disabled:\n    telegram: [off]\n", false, []string{"HERMES_PLATFORM=telegram"}, "unknown"},
		{"raw-platform-key", "", "skills:\n  platform_disabled:\n    off: [guarded]\n", false, []string{"HERMES_PLATFORM=off"}, "unknown"},
		{"raw-external-list", "", "skills:\n  external_dirs: [off]\n", true, nil, "unknown"},
		{"nonconsumed-metadata-control", "---\nname: guarded\ndescription: Inert.\npadding: on\n---\nD2_FULL_CONTENT_SENTINEL\n", "", false, nil, "available"},
		{"explicit-str-control", "---\nname: !!str on\ndescription: !!str off\n---\nD2_FULL_CONTENT_SENTINEL\n", "", false, nil, "available"},
		{"block-scalar-control", "---\nname: guarded\ndescription: |\n  on\n---\nD2_FULL_CONTENT_SENTINEL\n", "", false, nil, "available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			root := filepath.Join(p, "skills")
			name := "guarded"
			if tc.name == "raw-disabled-list" || tc.name == "raw-platform-value" {
				name = "off"
			}
			if tc.external {
				root = filepath.Join(p, "off")
			}
			f := codexSkill(t, root, name, name)
			text := tc.manifest
			if text == "" {
				text = "---\nname: " + name + "\ndescription: Inert.\n---\nD2_FULL_CONTENT_SENTINEL\n"
			}
			writeFile(t, f, []byte(text), 0600)
			if tc.config != "" {
				writeFile(t, filepath.Join(p, "config.yaml"), []byte(tc.config), 0600)
			}
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			s := gateSourceAt(t, d2List(t, fx, tc.env), filepath.Dir(f))
			if s.Availability.State != tc.want {
				t.Errorf("consumed native YAML confidence=%s; want %s", s.Availability.State, tc.want)
			}
			d2Read(t, fx, s, tc.env)
		})
	}
}

func TestDecision2ALE31YamlAliasConfidence(t *testing.T) {
	for _, quoted := range []bool{false, true} {
		t.Run(fmt.Sprintf("quoted=%t", quoted), func(t *testing.T) {
			fx, p := hermesFixture(t)
			f := codexSkill(t, filepath.Join(p, "skills"), "bundle", "bundle")
			val := "on"
			if quoted {
				val = "'on'"
			}
			writeFile(t, f, []byte("---\nvalue: &scalar "+val+"\nname: *scalar\ndescription: Inert.\n---\nD2_FULL_CONTENT_SENTINEL\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			s := gateSourceAt(t, d2List(t, fx, nil), filepath.Dir(f))
			want := "unknown"
			if quoted {
				want = "available"
			}
			if s.Availability.State != want {
				t.Errorf("consumed alias confidence=%s; want %s", s.Availability.State, want)
			}
			d2Read(t, fx, s, nil)
		})
	}
}
