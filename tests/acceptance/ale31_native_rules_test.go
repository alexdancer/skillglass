package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func gateSourceAt(t *testing.T, list codexList, directory string) codexSource {
	t.Helper()
	directory, _ = filepath.EvalSymlinks(directory)
	for _, source := range list.Sources {
		if source.SourcePath == directory {
			return source
		}
	}
	t.Fatalf("missing source %s", directory)
	return codexSource{}
}

func TestGateALE31ManifestPathWinner(t *testing.T) {
	for _, rootKind := range []string{"external", "profile"} {
		for _, nested := range []string{"Alpha", "zeta"} {
			t.Run(rootKind+"/"+nested, func(t *testing.T) {
				fx, profile := hermesFixture(t)
				root := filepath.Join(profile, "shared")
				if rootKind == "profile" {
					root = filepath.Join(profile, "skills")
				}
				parent := codexSkill(t, root, "parent", "collision")
				child := codexSkill(t, root, "parent/"+nested, "collision")
				before := treeManifest(t, fx.base)
				defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
				list := hermesList(t, fx, 0, nil)
				p := gateSourceAt(t, list, filepath.Dir(parent))
				c := gateSourceAt(t, list, filepath.Dir(child))
				winner := p
				if nested == "Alpha" {
					winner = c
				}
				for _, s := range []codexSource{p, c} {
					if s.Availability.Precedence.Winner != winner.ID {
						t.Errorf("%s index winner=%s; want %s (%s)", s.SourcePath, s.Availability.Precedence.Winner, winner.ID, winner.SourcePath)
					}
				}
				if rootKind == "external" {
					if nested == "Alpha" {
						repairHermesPublicParity(t, fx, winner, "available", "Native name: collision", nil)
					}
					for _, s := range []codexSource{p, c} {
						want := "shadowed"
						if s.ID == winner.ID {
							want = "available"
						}
						if s.Availability.State != want {
							t.Errorf("%s state=%s; want %s", s.SourcePath, s.Availability.State, want)
						}
					}
				} else {
					for _, s := range []codexSource{p, c} {
						if s.Availability.State != "unknown" || s.Availability.Precedence.State != "unknown" {
							t.Errorf("profile prompt/index conflict must remain unknown: %+v", s)
						}
					}
				}
			})
		}
	}
}

func gateWindowText(prefix, unit string, fenceEnd int, bom, crlf bool) string {
	pad := fenceEnd - utf8.RuneCountInString(prefix) - 5
	text := prefix + strings.Repeat(unit, pad) + "\n---\n" + "FULL_CONTENT_READ_SENTINEL\n"
	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if bom {
		text = "\ufeff" + text
	}
	return text
}
func TestGateALE31NativeMetadataWindow(t *testing.T) {
	for _, field := range []string{"name", "platforms", "environments", "conditional"} {
		for _, end := range []int{3999, 4000, 4001, 4150} {
			for _, unit := range []string{"x", "界", "🙂"} {
				t.Run(fmt.Sprintf("%s/end=%d/unit=%s", field, end, unit), func(t *testing.T) {
					fx, profile := hermesFixture(t)
					prefix := "---\nname: working\ndescription: Inert.\n"
					switch field {
					case "platforms":
						prefix += "platforms: [windows]\n"
					case "environments":
						prefix += "environments: [docker]\n"
					case "conditional":
						prefix += "metadata:\n  hermes:\n    requires_tools: [terminal]\n"
					}
					prefix += "padding: "
					text := gateWindowText(prefix, unit, end, false, false)
					file := codexSkill(t, filepath.Join(profile, "skills"), "bundle", "working")
					writeFile(t, file, []byte(text), 0600)
					if field == "name" {
						writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  disabled: [bundle]\n"), 0600)
					}
					before := treeManifest(t, fx.base)
					defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
					list := hermesList(t, fx, 0, nil)
					s := gateSourceAt(t, list, filepath.Dir(file))
					read := hermesRun(t, fx, nil, "read", "--agent", "hermes", "--project", fx.project, s.ID)
					assertExit(t, read, 0)
					if !strings.Contains(string(read.stdout), "FULL_CONTENT_READ_SENTINEL") {
						t.Error("full read truncated")
					}
					wantName := "working"
					wantState := "available"
					if end > 4000 {
						wantName = "bundle"
						if field == "name" {
							wantState = "disabled"
						}
						if field == "environments" || field == "conditional" {
							wantState = "unknown"
						}
					} else {
						switch field {
						case "platforms":
							wantState = "unavailable"
						case "environments", "conditional":
							wantState = "unknown"
						}
					}
					if s.Availability.NativeName != wantName {
						t.Errorf("native_name=%q; want %q", s.Availability.NativeName, wantName)
					}
					if s.Availability.State != wantState {
						t.Errorf("state=%q; want %q", s.Availability.State, wantState)
					}
					if field == "name" && end == 4001 && unit == "x" {
						repairHermesPublicParity(t, fx, s, "disabled", "Native name: bundle", nil)
					}
				})
			}
		}
	}
}
func TestGateALE31MetadataBOMCRLFAndPortableControl(t *testing.T) {
	for _, portable := range []bool{false, true} {
		for _, bom := range []bool{false, true} {
			for _, end := range []int{4000, 4001} {
				t.Run(fmt.Sprintf("portable=%t/bom=%t/end=%d", portable, bom, end), func(t *testing.T) {
					fx, profile := hermesFixture(t)
					var file string
					if portable {
						repairHermesPlugin(t, fx, profile, false)
						file = filepath.Join(profile, "plugins", "portable", "skills", "repair-plugin", "SKILL.md")
					} else {
						file = codexSkill(t, filepath.Join(profile, "skills"), "bundle", "working")
						writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  disabled: [bundle]\n"), 0600)
					}
					name := "working"
					if portable {
						name = "repair-plugin"
					}
					text := gateWindowText("---\nname: "+name+"\ndescription: Inert.\npadding: ", "界", end, bom, true)
					writeFile(t, file, []byte(text), 0600)
					before := treeManifest(t, fx.base)
					defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
					exit := 0
					if bom {
						exit = 1
					}
					list := hermesList(t, fx, exit, nil)
					s := gateSourceAt(t, list, filepath.Dir(file))
					want := "available"
					if end > 4000 {
						want = "disabled"
					}
					if portable {
						want = "explicit-only"
					}
					if bom {
						want = "unknown"
					}
					if s.Availability.State != want {
						t.Errorf("state=%s; want %s", s.Availability.State, want)
					}
					read := hermesRun(t, fx, nil, "read", "--agent", "hermes", "--project", fx.project, s.ID)
					if !strings.Contains(string(read.stdout), "FULL_CONTENT_READ_SENTINEL") {
						t.Error("full content read lost")
					}
				})
			}
		}
	}
}

func TestGateALE31RejectedPluginCollision(t *testing.T) {
	cases := []struct {
		name, raw    string
		native, link bool
		exit         int
	}{
		{"unsupported-schema", `{"$schema":"unsupported","name":"portable"}`, false, false, 0},
		{"missing-schema", `{"name":"portable"}`, false, false, 0},
		{"invalid-name", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"invalid--name"}`, false, false, 0},
		{"invalid-version", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable","version":2}`, false, false, 0},
		{"invalid-keywords", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable","keywords":[2]}`, false, false, 0},
		{"invalid-author", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable","author":{"extra":"bad"}}`, false, false, 0},
		{"invalid-extensions", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable","extensions":{"x":2}}`, false, false, 0},
		{"manifest-escape", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`, false, true, 0},
		{"invalid-json", `{`, false, false, 1},
		{"portable-unreadable-directory", "__directory__", false, false, 1},
		{"portable-dangling-manifest", "__dangling__", false, false, 1},
		{"yaml-unreadable-directory", "__directory__", true, false, 1},
		{"json-array", `[]`, false, false, 1},
		{"yaml-invalid", `name: [`, true, false, 1},
		{"yaml-sequence", "- bad\n", true, false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			user := filepath.Join(profile, "plugins", "category", "portable")
			project := filepath.Join(fx.project, ".hermes", "plugins", "category", "portable")
			for _, p := range []string{user, project} {
				codexSkill(t, filepath.Join(p, "skills"), "explicit", "explicit")
			}
			good := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`
			writeFile(t, filepath.Join(user, "plugin.json"), []byte(good), 0600)
			manifest := filepath.Join(project, "plugin.json")
			if tc.native {
				manifest = filepath.Join(project, "plugin.yaml")
			}
			if tc.link {
				outside := filepath.Join(fx.home, "manifest-outside.json")
				writeFile(t, outside, []byte(tc.raw), 0600)
				if err := os.Symlink(outside, manifest); err != nil {
					t.Fatal(err)
				}
			} else if tc.raw == "__directory__" {
				if err := os.Mkdir(manifest, 0700); err != nil {
					t.Fatal(err)
				}
			} else if tc.raw == "__dangling__" {
				if err := os.Symlink(filepath.Join(project, "absent.json"), manifest); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, manifest, []byte(tc.raw), 0600)
			}
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [category/portable]\n"), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := hermesList(t, fx, tc.exit, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			u := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
			p := gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
			for _, ref := range u.Availability.References {
				if ref.Participation == "excluded" {
					t.Errorf("rejected project displaced valid user: %+v", ref)
				}
			}
			if tc.exit == 0 && u.Availability.State != "explicit-only" {
				t.Errorf("user state=%s; want explicit-only", u.Availability.State)
			}
			if p.Availability.State != "unknown" {
				t.Errorf("rejected plugin inventory state=%s; want unknown", p.Availability.State)
			}
			if tc.name == "unsupported-schema" {
				repairHermesPublicParity(t, fx, u, "explicit-only", "Availability: explicit-only", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
			}
		})
	}
}

func TestGateALE31WindowReferenceNamesAndPrecedence(t *testing.T) {
	for _, alias := range []bool{false, true} {
		t.Run(fmt.Sprintf("alias=%t", alias), func(t *testing.T) {
			fx, profile := hermesFixture(t)
			file := codexSkill(t, filepath.Join(profile, "skills"), "bundle", "working")
			text := gateWindowText("---\nname: working\ndescription: Inert.\npadding: ", "🙂", 4001, false, false)
			writeFile(t, file, []byte(text), 0600)
			external := filepath.Join(profile, "shared", "bundle")
			if alias {
				external = filepath.Join(profile, "shared", "alias")
				if err := os.Symlink(filepath.Dir(file), external); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  external_dirs: [shared]\n  disabled: [alias]\n"), 0600)
			} else {
				other := codexSkill(t, filepath.Join(profile, "shared"), "bundle", "other")
				writeFile(t, other, []byte(strings.Replace(text, "working", "another", 1)), 0600)
			}
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := hermesList(t, fx, 0, nil)
			local := gateSourceAt(t, list, filepath.Dir(file))
			if alias {
				if len(list.Sources) != 1 || local.Availability.State != "unknown" || local.Availability.NativeName != "alias, bundle" {
					t.Fatalf("reference-specific fallback identity: %+v", local)
				}
				if local.Availability.Precedence.State != "unknown" || local.Availability.Precedence.Winner != "" {
					t.Fatalf("reference-specific names cannot claim one aggregate winner: %+v", local.Availability.Precedence)
				}
				permissions := map[string]int{}
				for _, ref := range local.Availability.References {
					permissions[ref.Permission.State]++
				}
				if permissions["allowed"] != 1 || permissions["denied"] != 1 {
					t.Fatalf("fallback permissions: %v", permissions)
				}
			} else {
				other := gateSourceAt(t, list, external)
				if local.Availability.NativeName != "bundle" || other.Availability.NativeName != "bundle" || local.Availability.State != "available" || other.Availability.State != "shadowed" || other.Availability.Precedence.Winner != local.ID {
					t.Fatalf("fallback precedence local=%+v external=%+v", local, other)
				}
			}
		})
	}
}

func TestGateALE31UnknownConfigCannotAdmitRejectedPlugin(t *testing.T) {
	fx, profile := hermesFixture(t)
	repairHermesPlugin(t, fx, profile, false)
	project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
	codexSkill(t, filepath.Join(project, "skills"), "repair-plugin", "repair-plugin")
	writeFile(t, filepath.Join(project, "plugin.json"), []byte(`{"$schema":"unsupported","name":"portable"}`), 0600)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable, false]\n"), 0600)
	before := treeManifest(t, fx.base)
	defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
	list := hermesList(t, fx, 1, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
	user := gateSourceAt(t, list, filepath.Join(profile, "plugins", "portable", "skills", "repair-plugin"))
	for _, ref := range user.Availability.References {
		if ref.Participation == "excluded" {
			t.Fatalf("unknown activation admitted a rejected root: %+v", ref)
		}
	}
	if user.Availability.State != "unknown" {
		t.Fatalf("unknown activation state: %+v", user)
	}
}

func TestGateALE31RejectedPluginGatesAndAcceptedRegistrationControls(t *testing.T) {
	for _, tc := range []struct {
		name, kind   string
		env          []string
		config, user string
	}{
		{"rejected-project-opt-out", "reject", nil, "plugins:\n  enabled: [portable]\n", "explicit-only"},
		{"rejected-safe-mode", "reject", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1", "HERMES_SAFE_MODE=1"}, "plugins:\n  enabled: [portable]\n", "unavailable"},
		{"rejected-disabled", "reject", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "plugins:\n  enabled: [portable]\n  disabled: [portable]\n", "unavailable"},
		{"rejected-opt-in-empty", "reject", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "plugins:\n  enabled: []\n", "unavailable"},
		{"accepted-native-registration-unknown", "native", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "plugins:\n  enabled: [portable]\n", "unavailable"},
		{"accepted-manifest-escaped-skill-root", "skill-escape", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "plugins:\n  enabled: [portable]\n", "unavailable"},
		{"accepted-manifest-invalid-skill", "skill-invalid", []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"}, "plugins:\n  enabled: [portable]\n", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			user := filepath.Join(profile, "plugins", "portable")
			project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
			good := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`
			codexSkill(t, filepath.Join(user, "skills"), "explicit", "explicit")
			writeFile(t, filepath.Join(user, "plugin.json"), []byte(good), 0600)
			codexSkill(t, filepath.Join(project, "skills"), "explicit", "explicit")
			raw := good
			if tc.kind == "reject" {
				raw = `{"$schema":"unsupported","name":"portable"}`
			}
			if tc.kind == "native" {
				writeFile(t, filepath.Join(project, "plugin.yaml"), []byte("name: portable\nkind: standalone\n"), 0600)
			} else {
				writeFile(t, filepath.Join(project, "plugin.json"), []byte(raw), 0600)
			}
			if tc.kind == "skill-invalid" {
				writeFile(t, filepath.Join(project, "skills", "explicit", "SKILL.md"), []byte("---\nname: wrong\ndescription: Inert.\n---\nInert.\n"), 0600)
			}
			if tc.kind == "skill-escape" {
				outside := filepath.Join(fx.home, "outside-skills")
				codexSkill(t, outside, "explicit", "explicit")
				if err := os.RemoveAll(filepath.Join(project, "skills")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(project, "skills")); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte(tc.config), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			list := hermesList(t, fx, 0, tc.env)
			u := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
			if u.Availability.State != tc.user {
				t.Errorf("user state=%s; want %s", u.Availability.State, tc.user)
			}
		})
	}
}
