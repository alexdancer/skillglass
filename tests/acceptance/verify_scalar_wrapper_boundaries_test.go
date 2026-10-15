package acceptance_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyScalarWrapperLexicalBoundaries(t *testing.T) {
	for _, extension := range []string{"yaml", "yml"} {
		for _, tc := range []struct {
			name, tag, value string
			valid            bool
		}{
			{"bool-mixed-case", "bool", "tRuE", true},
			{"bool-whitespace", "bool", " on ", false},
			{"bool-empty", "bool", "", false},
			{"int-arbitrary-size", "int", strings.Repeat("9", 1000), true},
			{"int-underscores", "int", "_1__2_", true},
			{"int-sign-only", "int", "-__", false},
			{"int-double-sign", "int", "--42", true},
			{"int-three-signs", "int", "---42", false},
			{"int-binary", "int", "-0b1_01", true},
			{"int-binary-inner-prefix", "int", "0b0B11", true},
			{"int-binary-invalid", "int", "0b102", false},
			{"int-hexadecimal", "int", "+0xF_F", true},
			{"int-uppercase-outer-prefix", "int", "0XFF", false},
			{"int-octal", "int", "0o17", true},
			{"int-octal-legacy", "int", "017", true},
			{"int-octal-invalid", "int", "08", false},
			{"int-sexagesimal", "int", "1:70:-2", true},
			{"int-sexagesimal-leading-zero", "int", "0:20", false},
			{"int-sexagesimal-empty", "int", "1::2", false},
			{"int-unicode-digits", "int", "١٢٣", true},
			{"int-unicode-octal", "int", "0٧", true},
			{"int-unicode-nondigit", "int", "²", false},
			{"int-whitespace", "int", "\u00a0-42\u00a0", true},
			{"float-range-overflow", "float", "1e999999999999999999999999", true},
			{"float-underscores", "float", "_1__._5_", true},
			{"float-exponent-invalid", "float", "1e+", false},
			{"float-special", "float", "-.INF", true},
			{"float-special-whitespace", "float", " .inf ", false},
			{"float-python-infinity", "float", " +Infinity ", true},
			{"float-unicode-non-ascii-case", "float", "İNf", false},
			{"float-python-nan", "float", " NaN ", true},
			{"float-double-sign", "float", "--1.5", true},
			{"float-sexagesimal", "float", "1:70.5:-2e2", true},
			{"float-sexagesimal-inf", "float", "1:inf", true},
			{"float-sexagesimal-special-invalid", "float", "1:.inf", false},
			{"float-unicode-digits", "float", "١.٢e٣", true},
			{"float-hex-invalid", "float", "0x1p2", false},
			{"float-empty", "float", "", false},
			{"binary-empty", "binary", "", true},
			{"binary-ignored-padding", "binary", "====", true},
			{"binary-two-padding", "binary", "YQ==", true},
			{"binary-one-padding", "binary", "YWI=", true},
			{"binary-extra-padding", "binary", "YQ===inert", true},
			{"binary-interspersed-padding", "binary", "Y=W=Jj", true},
			{"binary-interspersed-punctuation", "binary", "Y Q! =?=", true},
			{"binary-padding-short", "binary", "YQ=", false},
			{"binary-single-character", "binary", "Y===", false},
			{"binary-nonascii-before", "binary", "éYQ==", false},
			{"binary-nonascii-after", "binary", "YQ==é", false},
		} {
			t.Run(extension+"/"+tc.name, func(t *testing.T) {
				fx, profile := hermesFixture(t)
				user := filepath.Join(profile, "plugins", "portable")
				project := filepath.Join(fx.project, ".hermes", "plugins", "portable")
				for _, root := range []string{user, project} {
					codexSkill(t, filepath.Join(root, "skills"), "explicit", "explicit")
				}
				writeFile(t, filepath.Join(user, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
				manifest := fmt.Sprintf("name: portable\nextra: !!%s {=: %q, ignored: !custom inert}\n", tc.tag, tc.value)
				writeFile(t, filepath.Join(project, "plugin."+extension), []byte(manifest), 0600)
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins: {enabled: [portable]}\n"), 0600)
				defer g2Seal(t, fx)()
				list := hermesList(t, fx, 0, []string{"HERMES_ENABLE_PROJECT_PLUGINS=1"})
				source := gateSourceAt(t, list, filepath.Join(user, "skills", "explicit"))
				want := "explicit-only"
				if tc.valid {
					want = "unavailable"
				}
				if source.Availability.State != want {
					t.Fatalf("outer %s conversion: state=%s, want %s", tc.tag, source.Availability.State, want)
				}
				gateSourceAt(t, list, filepath.Join(project, "skills", "explicit"))
			})
		}
	}
}

func TestVerifyScalarWrapperExtractionContexts(t *testing.T) {
	for _, tc := range []struct{ name, extra, want string }{
		{"nested-valid", "extra: !!int {=: !!str {=: '42'}}\n", "unavailable"},
		{"nested-invalid", "extra: !!int {=: !!str {=: inert}}\n", "explicit-only"},
		{"alias-valid", "extra: !!int {ignored: &payload !custom '42', =: *payload}\n", "unavailable"},
		{"alias-invalid", "extra: !!int {ignored: &payload !custom inert, =: *payload}\n", "explicit-only"},
		{"outer-alias-valid", "extra: &wrapper !!bool {=: oN}\ncopy: *wrapper\n", "unavailable"},
		{"outer-alias-invalid", "extra: &wrapper !!bool {=: nonsense}\ncopy: *wrapper\n", "explicit-only"},
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
				t.Fatalf("outer constructor extraction: state=%s, want %s", source.Availability.State, tc.want)
			}
		})
	}
}

func TestVerifyScalarWrapperFallbackContexts(t *testing.T) {
	for _, location := range []string{"profile", "managed", "frontmatter"} {
		t.Run(location, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := d2Plugin(t, fx, profile, "portable")
			config := "plugins: {enabled: [portable]}\n"
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte(config), 0600)
			var env []string
			switch location {
			case "profile":
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte(config+"extra: !!int {=: inert}\n"), 0600)
			case "managed":
				managed := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(managed, "config.yaml"), []byte(config+"extra: !!int {=: inert}\n"), 0600)
				env = []string{"HERMES_MANAGED_DIR=" + managed}
			case "frontmatter":
				writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(strings.Replace(g2Text("explicit", "extra: !!int {=: inert}\n"), "G2_FULL_CONTENT_SENTINEL", "D2_FULL_CONTENT_SENTINEL", 1)), 0600)
			}
			defer g2Seal(t, fx)()
			source := gateSourceAt(t, d2List(t, fx, env), dir)
			if source.Availability.State != "unknown" {
				t.Fatalf("unobserved native fallback became known: %+v", source.Availability)
			}
		})
	}
}
