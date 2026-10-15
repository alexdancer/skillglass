package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyALE31NativeScalarTypes(t *testing.T) {
	for _, tc := range []struct {
		name, description, extra, want string
		portable                       bool
	}{
		{"portable-binary-description", "!!binary SW5lcnQu", "", "unknown", true},
		{"portable-quoted-binary-control", "'!!binary SW5lcnQu'", "", "explicit-only", true},
		{"portable-explicit-string-control", "!!str SW5lcnQu", "", "explicit-only", true},
		{"portable-nonconsumed-control", "Inert.", "extra: !!binary SW5lcnQu\n", "explicit-only", true},
		{"index-sexagesimal-description", "1:20", "", "unknown", false},
		{"index-quoted-sexagesimal-control", "'1:20'", "", "available", false},
		{"index-explicit-string-control", "!!str 1:20", "", "available", false},
		{"index-nonconsumed-control", "Inert.", "extra: 1:20\n", "available", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			dir := filepath.Join(profile, "skills", "explicit")
			if tc.portable {
				dir = d2Plugin(t, fx, profile, "portable")
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
			}
			manifest := "---\nname: explicit\ndescription: " + tc.description + "\n" + tc.extra + "---\nD2_FULL_CONTENT_SENTINEL\n"
			writeFile(t, filepath.Join(dir, "SKILL.md"), []byte(manifest), 0600)
			before := treeManifest(t, fx.base)
			defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
			source := gateSourceAt(t, d2List(t, fx, nil), dir)
			t.Logf("public list state=%s want=%s", source.Availability.State, tc.want)
			if source.Availability.State != tc.want {
				t.Errorf("native scalar confidence state=%q; want %q", source.Availability.State, tc.want)
			}
			explain := hermesRun(t, fx, nil, "explain", "--agent", "hermes", "--project", fx.project, source.ID)
			assertExit(t, explain, 0)
			if !strings.Contains(string(explain.stdout), "Availability: "+tc.want) {
				t.Errorf("public explanation lacks literal expected state %q: %s", tc.want, explain.stdout)
			}
			d2Read(t, fx, source, nil)
		})
	}
}
