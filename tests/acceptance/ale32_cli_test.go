package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func hermesChange(t *testing.T, fx fixture, choice, scope, id, token string, want int) changeReceipt {
	t.Helper()
	args := []string{choice, "--agent", "hermes", "--project", fx.project, "--scope", scope, "--json"}
	if token != "" {
		args = append(args, "--confirm", token)
	}
	args = append(args, id)
	result := changeCommand(t, fx, "", args...)
	assertExit(t, result, want)
	var out changeReceipt
	decodeJSON(t, result.stdout, &out)
	return out
}
func hermesTarget(t *testing.T) (fixture, string, string, string) {
	t.Helper()
	fx, profile := hermesFixture(t)
	target := codexSkill(t, filepath.Join(profile, "skills"), "ale32-target", "ale32-target")
	list := hermesList(t, fx, 0, nil)
	id := ""
	for _, source := range list.Sources {
		if source.Name == "ale32-target" {
			id = source.ID
		}
	}
	if id == "" {
		t.Fatal("target not discovered")
	}
	return fx, profile, id, target
}
func TestALE32GlobalProfileLosslessAndPlatformDenial(t *testing.T) {
	fx, profile, id, target := hermesTarget(t)
	config := filepath.Join(profile, "config.yaml")
	raw := []byte("# preserved\r\nnumber: 1.2300e+09\r\nunknown: 'yes'\r\nskills:\r\n  external_dirs: [shared]\r\n  disabled: [unrelated]\r\n  platform_disabled:\r\n    telegram: [ale32-target]\r\n")
	writeFile(t, config, raw, 0640)
	other := filepath.Join(fx.home, ".hermes/config.yaml")
	writeFile(t, other, []byte("skills:\n  disabled: [other-profile]\n"), 0600)
	writeFile(t, filepath.Join(fx.home, ".codex/config.toml"), []byte("# untouched\n"), 0600)
	before := treeManifest(t, fx.base)
	content := readBytes(t, target)
	p := hermesChange(t, fx, "disable", "global", id, "", 0)
	if p.State != "ready" {
		t.Fatalf("preview %+v", p)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	assertSavedChange(t, hermesChange(t, fx, "disable", "global", id, p.Confirmation, 0))
	expected := bytes.Replace(raw, []byte("  disabled: [unrelated]\r\n"), []byte("  disabled: [\"unrelated\",\"ale32-target\"]\r\n"), 1)
	if !bytes.Equal(readBytes(t, config), expected) {
		t.Fatalf("unrelated YAML changed: %s", readBytes(t, config))
	}
	p = hermesChange(t, fx, "enable", "global", id, "", 0)
	assertSavedChange(t, hermesChange(t, fx, "enable", "global", id, p.Confirmation, 0))
	if !bytes.Equal(readBytes(t, config), bytes.Replace(raw, []byte("[unrelated]"), []byte("[\"unrelated\"]"), 1)) {
		t.Fatal("enable did not remove only global name")
	}
	// A persisted enable removes only the global denial; platform rules survive.
	writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=telegram\n"), 0600)
	p = hermesChange(t, fx, "enable", "global", id, "", 0)
	out := hermesChange(t, fx, "enable", "global", id, p.Confirmation, 1)
	if out.State != "applied" || out.Persistence != "verified" || out.Achievement != "unmet" || out.Effective.State != "disabled" {
		t.Fatalf("platform denial hidden %+v", out)
	}
	if string(readBytes(t, other)) != "skills:\n  disabled: [other-profile]\n" || !bytes.Equal(content, readBytes(t, target)) {
		t.Fatal("other profile/content changed")
	}
	if string(readBytes(t, filepath.Join(fx.home, ".codex/config.toml"))) != "# untouched\n" {
		t.Fatal("other application changed")
	}
}
func TestALE32UnsupportedProjectNeverFallsBack(t *testing.T) {
	fx, _, id, _ := hermesTarget(t)
	before := treeManifest(t, fx.base)
	for _, choice := range []string{"disable", "enable", "inherit"} {
		out := hermesChange(t, fx, choice, "project", id, "", 1)
		if out.State != "unsupported" || out.Confirmation != "" || !strings.Contains(out.Reason, "No global fallback") {
			t.Fatalf("unsupported %+v", out)
		}
	}
	out := hermesChange(t, fx, "inherit", "global", id, "", 1)
	if out.State != "unsupported" {
		t.Fatal(out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
func TestALE32RefusalsAndStaleInputs(t *testing.T) {
	for _, scenario := range []string{"collision", "excluded-collision", "long-name", "alias-name", "namespace", "scalar-disabled", "merge", "flow", "duplicate", "hardlink", "manifest-hardlink", "symlink", "external-edit", "same-byte-replacement", "profile-marker", "platform-env", "new-collision", "write-denied", "unknown-platform"} {
		t.Run(scenario, func(t *testing.T) {
			fx, profile, id, target := hermesTarget(t)
			config := filepath.Join(profile, "config.yaml")
			raw := readBytes(t, config)
			switch scenario {
			case "collision":
				codexSkill(t, filepath.Join(profile, "shared"), "copy", "ale32-target")
			case "excluded-collision":
				p := codexSkill(t, filepath.Join(profile, "shared"), "copy", "ale32-target")
				writeFile(t, p, []byte("---\nname: ale32-target\nplatforms: [windows]\n---\nInert\n"), 0600)
			case "long-name":
				writeFile(t, target, []byte("---\nname: "+strings.Repeat("x", 70)+"\n---\nInert\n"), 0600)
			case "alias-name":
				writeFile(t, target, []byte("---\nname: other-name\n---\nInert\n"), 0600)
			case "namespace":
				name := "demo:target"
				codexSkill(t, filepath.Join(profile, "skills"), name, name)
				for _, source := range hermesList(t, fx, 0, nil).Sources {
					if source.Name == name {
						id = source.ID
					}
				}
			case "scalar-disabled":
				writeFile(t, config, []byte("skills:\n  external_dirs: [shared]\n  disabled: other-target\n"), 0600)
			case "merge":
				writeFile(t, config, []byte("defaults: &d {disabled: []}\nskills:\n  <<: *d\n  external_dirs: [shared]\n"), 0600)
			case "flow":
				writeFile(t, config, []byte("skills: {external_dirs: [shared], disabled: []}\n"), 0600)
			case "duplicate":
				writeFile(t, config, []byte("skills:\n  disabled: []\n  disabled: []\n"), 0600)
			case "hardlink":
				if err := os.Link(config, filepath.Join(fx.home, "copy.yaml")); err != nil {
					t.Fatal(err)
				}
			case "manifest-hardlink":
				if err := os.Link(target, filepath.Join(fx.home, "copy.md")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				other := filepath.Join(fx.home, "other.yaml")
				writeFile(t, other, raw, 0600)
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, config); err != nil {
					t.Fatal(err)
				}
			case "unknown-platform":
				writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=${UNRESOLVED}\n"), 0600)
			}
			before := treeManifest(t, fx.base)
			stale := scenario == "external-edit" || scenario == "same-byte-replacement" || scenario == "profile-marker" || scenario == "platform-env" || scenario == "new-collision" || scenario == "write-denied"
			if stale {
				p := hermesChange(t, fx, "disable", "global", id, "", 0)
				if p.State != "ready" {
					t.Fatal(p)
				}
				switch scenario {
				case "external-edit":
					writeFile(t, config, append(raw, []byte("other: saved\n")...), 0600)
				case "same-byte-replacement":
					if err := os.Remove(config); err != nil {
						t.Fatal(err)
					}
					writeFile(t, config, raw, 0600)
				case "profile-marker":
					writeFile(t, filepath.Join(fx.home, ".hermes/active_profile"), []byte("default\n"), 0600)
				case "platform-env":
					writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=telegram\n"), 0600)
				case "new-collision":
					codexSkill(t, filepath.Join(profile, "shared"), "copy", "ale32-target")
				}
				before = treeManifest(t, fx.base)
				if scenario == "write-denied" {
					result := changeCommand(t, fx, "(deny file-write* (subpath "+strconv.Quote(profile)+"))", "disable", "--agent", "hermes", "--project", fx.project, "--scope", "global", "--json", "--confirm", p.Confirmation, id)
					assertExit(t, result, 1)
				} else {
					out := hermesChange(t, fx, "disable", "global", id, p.Confirmation, 1)
					if out.State == "applied" {
						t.Fatal(out)
					}
				}
			} else {
				out := hermesChange(t, fx, "disable", "global", id, "", 1)
				if out.State == "ready" {
					t.Fatal(out)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE32AbsentConfigAndBlockList(t *testing.T) {
	for _, raw := range []string{"", "# keep\nother: 123\n", "skills:\n  external_dirs: [shared]\n  disabled:\n    - unrelated\n    - ale32-target\n  keep: true\n"} {
		t.Run(strconv.Itoa(len(raw)), func(t *testing.T) {
			fx, profile, id, _ := hermesTarget(t)
			config := filepath.Join(profile, "config.yaml")
			if raw == "" {
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, config, []byte(raw), 0600)
			}
			p := hermesChange(t, fx, "disable", "global", id, "", 0)
			assertSavedChange(t, hermesChange(t, fx, "disable", "global", id, p.Confirmation, 0))
			p = hermesChange(t, fx, "enable", "global", id, "", 0)
			assertSavedChange(t, hermesChange(t, fx, "enable", "global", id, p.Confirmation, 0))
			before := treeManifest(t, fx.base)
			p = hermesChange(t, fx, "enable", "global", id, "", 0)
			if !p.NoWriteNeeded {
				t.Fatal("enable should be no-op")
			}
			assertSavedChange(t, hermesChange(t, fx, "enable", "global", id, p.Confirmation, 0))
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE32EmptyMappingCommentsAndExactDisabledEntry(t *testing.T) {
	fx, profile, id, _ := hermesTarget(t)
	config := filepath.Join(profile, "config.yaml")
	raw := []byte("# retained operator note\n{} # empty mapping note\n# retained end note\n")
	writeFile(t, config, raw, 0600)
	p := hermesChange(t, fx, "disable", "global", id, "", 0)
	assertSavedChange(t, hermesChange(t, fx, "disable", "global", id, p.Confirmation, 0))
	expected := bytes.Replace(raw, []byte("{}"), []byte("skills:\n  disabled: [\"ale32-target\"]"), 1)
	if !bytes.Equal(readBytes(t, config), expected) {
		t.Fatalf("empty map comments changed: %s", readBytes(t, config))
	}
	writeFile(t, config, []byte("skills:\n  disabled: [' ale32-target ', unrelated]\n"), 0600)
	p = hermesChange(t, fx, "disable", "global", id, "", 0)
	if p.NoWriteNeeded {
		t.Fatal("trimmed native utility membership cannot stand in for exact explicit denial")
	}
	assertSavedChange(t, hermesChange(t, fx, "disable", "global", id, p.Confirmation, 0))
	if !bytes.Contains(readBytes(t, config), []byte("[\" ale32-target \",\"unrelated\",\"ale32-target\"]")) {
		t.Fatalf("exact selector not added: %s", readBytes(t, config))
	}
	p = hermesChange(t, fx, "enable", "global", id, "", 0)
	assertSavedChange(t, hermesChange(t, fx, "enable", "global", id, p.Confirmation, 0))
	if !bytes.Contains(readBytes(t, config), []byte("[\"unrelated\"]")) {
		t.Fatal("enable failed to remove selected normalized names")
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	before := treeManifest(t, fx.base)
	p = hermesChange(t, fx, "enable", "global", id, "", 0)
	if !p.NoWriteNeeded {
		t.Fatal("absent enable should require no write")
	}
	assertSavedChange(t, hermesChange(t, fx, "enable", "global", id, p.Confirmation, 0))
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
