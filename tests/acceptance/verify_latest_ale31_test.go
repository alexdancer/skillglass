package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyLatestALE31DirectPath(t *testing.T) {
	for _, tc := range []struct{ name, selector, mode, want string }{
		{"relative-path-competitor", "category/second", "distinct", "ambiguous-bare-name"},
		{"categorized-fallthrough-competitor", "category:second", "distinct", "ambiguous-bare-name"},
		{"relative-path-with-excluded-competitor", "category/second", "excluded", "ambiguous-bare-name"},
		{"same-manifest-control", "category/second", "shared", "unambiguous-bare-name"},
		{"absent-path-control", "category/second", "absent", "unambiguous-bare-name"},
		{"different-path-control", "category/second", "different", "unambiguous-bare-name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			a := codexSkill(t, filepath.Join(p, "skills"), "first", tc.selector)
			writeFile(t, a, []byte(g2Text(tc.selector, "")), 0600)
			if tc.mode != "absent" {
				directory := "category/second"
				if tc.mode == "different" {
					directory = "category/third"
				}
				b := codexSkill(t, filepath.Join(p, "shared"), directory, "other")
				extra := ""
				if tc.mode == "excluded" {
					extra = "platforms: [windows]\n"
				}
				writeFile(t, b, []byte(g2Text("other", extra)), 0600)
				if tc.mode == "shared" {
					if err := os.Remove(b); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(a, b); err != nil {
						t.Fatal(err)
					}
				}
			}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, a, "available")
			source := gateSourceAt(t, list, filepath.Dir(a))
			inv := g2ExplicitInvocation(t, fx, source)
			count := "1"
			if tc.want == "ambiguous-bare-name" {
				count = "2"
			}
			fact := "selector \"" + tc.selector + "\": " + tc.want + " (" + count + " known distinct manifests)"
			if inv.State != tc.want || inv.Evidence.Kind != "inferred" || !strings.Contains(inv.Evidence.Reason, fact) {
				t.Errorf("native selector contract want %q and state %s inferred; got %+v", fact, tc.want, inv)
			}
		})
	}
}

func TestVerifyLatestALE31MixedPlatform(t *testing.T) {
	for _, tc := range []struct{ name, origin, platforms, state string }{
		{"profile-bool-match", "profile", "[false, macos]", "available"},
		{"external-bool-match", "external", "[false, macos]", "available"},
		{"portable-bool-match", "portable", "[false, macos]", "explicit-only"},
		{"profile-number-match", "profile", "[0, macos]", "available"},
		{"profile-null-match", "profile", "[null, macos]", "available"},
		{"profile-mapping-match", "profile", "[{inert: value}, macos]", "available"},
		{"matching-string-control", "profile", "[windows, macos]", "available"},
		{"mixed-nonmatching-control", "profile", "[false, windows]", "unavailable"},
		{"string-nonmatching-control", "profile", "[windows]", "unavailable"},
		{"falsy-whole-field-control", "profile", "false", "available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			root := filepath.Join(p, "skills")
			if tc.origin == "external" {
				root = filepath.Join(p, "shared")
			}
			file := ""
			if tc.origin == "portable" {
				file = g2Plugin(t, p)
			} else {
				file = codexSkill(t, root, "selected", "selected")
			}
			name := filepath.Base(filepath.Dir(file))
			writeFile(t, file, []byte(g2Text(name, "platforms: "+tc.platforms+"\n")), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, file, tc.state)
		})
	}
}
