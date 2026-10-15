package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyPolicyFollowupsManifest(t *testing.T) {
	for _, tc := range []struct {
		name, ext, mode, participation, state string
		exit                                  int
	}{
		{"yaml-self-loop", "yaml", "loop", "eligible", "unknown", 1},
		{"yml-self-loop", "yml", "loop", "eligible", "unknown", 1},
		{"yaml-not-directory", "yaml", "notdir", "eligible", "unknown", 1},
		{"yml-not-directory", "yml", "notdir", "eligible", "unknown", 1},
		{"dangling-control", "yaml", "missing", "eligible", "unknown", 1},
		{"existing-yaml-control", "yaml", "regular", "unknown", "unknown", 0},
		{"absent-yaml-control", "yaml", "absent", "eligible", "explicit-only", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
			manifest := filepath.Join(root, "plugin."+tc.ext)
			target := "plugin." + tc.ext
			switch tc.mode {
			case "notdir":
				writeFile(t, filepath.Join(root, "ordinary-file"), []byte("inert"), 0600)
				target = "ordinary-file/child"
			case "missing":
				target = "missing.yaml"
			case "regular":
				writeFile(t, manifest, []byte("name: portable\nversion: '1'\n"), 0600)
			}
			if tc.mode == "loop" || tc.mode == "notdir" || tc.mode == "missing" {
				if err := os.Symlink(target, manifest); err != nil {
					t.Fatal(err)
				}
			}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, tc.exit, nil)
			g2Inspect(t, fx, nil, list, file, tc.state)
			g2PluginRegistration(t, list, file, tc.participation, "portable")
			if tc.exit == 1 {
				wantProblem := "broken-reference"
				if tc.mode == "loop" {
					wantProblem = "symlink-cycle"
				}
				found := false
				for _, problem := range list.Problems {
					found = found || problem.Code == wantProblem
					if problem.Code == "native-input" {
						t.Error("fallback manufactured native-input problem")
					}
				}
				if !found || list.Scope.Completeness != "incomplete" {
					t.Errorf("generic %s coverage lost", wantProblem)
				}
			}
		})
	}
}

func TestVerifyPolicyFollowupsDenial(t *testing.T) {
	for _, tc := range []struct{ name, kind, config, state, permission, participation string }{
		{"plugin-known-deny-uncertain-enable", "plugin", "plugins:\n  enabled: [off]\n  disabled: [portable]\n", "unavailable", "allowed", "excluded"},
		{"qualified-global-deny-uncertain-platform", "plugin", "plugins:\n  enabled: [portable]\nskills:\n  disabled: [agent-plugin-portable-01e78282:explicit]\n  platform_disabled:\n    telegram: [off]\n", "disabled", "denied", "eligible"},
		{"plugin-unmatched-deny-control", "plugin", "plugins:\n  enabled: [off]\n  disabled: [other]\n", "unknown", "allowed", "unknown"},
		{"plugin-known-deny-control", "plugin", "plugins:\n  enabled: [portable]\n  disabled: [portable]\n", "unavailable", "allowed", "excluded"},
		{"plugin-uncertain-deny-control", "plugin", "plugins:\n  enabled: [portable]\n  disabled: [off]\n", "unknown", "allowed", "unknown"},
		{"global-deny-uncertain-platform", "raw", "skills:\n  disabled: [guarded]\n  platform_disabled:\n    telegram: [off]\n", "disabled", "denied", "eligible"},
		{"platform-deny-uncertain-global", "raw", "skills:\n  disabled: [off]\n  platform_disabled:\n    telegram: [guarded]\n", "disabled", "denied", "eligible"},
		{"raw-unmatched-deny-control", "raw", "skills:\n  disabled: [other]\n  platform_disabled:\n    telegram: [off]\n", "unknown", "unknown", "eligible"},
		{"raw-known-deny-control", "raw", "skills:\n  disabled: [guarded]\n  platform_disabled:\n    telegram: []\n", "disabled", "denied", "eligible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			var file string
			if tc.kind == "plugin" {
				file = g2Plugin(t, p)
			} else {
				file = codexSkill(t, filepath.Join(p, "skills"), "guarded", "guarded")
			}
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(tc.config), 0600)
			env := []string{"HERMES_PLATFORM=telegram"}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, env)
			g2Inspect(t, fx, env, list, file, tc.state)
			source := gateSourceAt(t, list, filepath.Dir(file))
			found := false
			for _, ref := range source.Availability.References {
				if ref.Participation == "inspection-only" {
					continue
				}
				found = true
				if ref.Permission.State != tc.permission || ref.Participation != tc.participation {
					t.Errorf("reference permission=%s participation=%s; want %s %s", ref.Permission.State, ref.Participation, tc.permission, tc.participation)
				}
			}
			if !found {
				t.Error("native reference missing")
			}
		})
	}
}

func TestVerifyPolicyFollowupsOwner(t *testing.T) {
	for _, tc := range []struct{ name, user, managed, owner, state string }{
		{"empty-managed-map", "enabled: [portable]", "{}", "profile", "explicit-only"},
		{"inherited-enable-managed-empty-deny", "enabled: [portable]", "\n  disabled: []", "profile", "explicit-only"},
		{"inherited-deny-managed-enable", "enabled: []\n  disabled: [portable]", "\n  enabled: [portable]", "profile", "unavailable"},
		{"managed-enable-control", "enabled: []", "\n  enabled: [portable]", "managed", "explicit-only"},
		{"managed-deny-control", "enabled: [portable]", "\n  disabled: [portable]", "managed", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			profile := filepath.Join(p, "config.yaml")
			managed := filepath.Join(fx.home, "managed")
			writeFile(t, profile, []byte("plugins:\n  "+tc.user+"\n"), 0600)
			writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins: "+tc.managed+"\n"), 0600)
			env := []string{"HERMES_MANAGED_DIR=" + managed}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, env)
			g2Inspect(t, fx, env, list, file, tc.state)
			source := gateSourceAt(t, list, filepath.Dir(file))
			want := profile
			if tc.owner == "managed" {
				want = filepath.Join(managed, "config.yaml")
			}
			result := hermesRun(t, fx, env, "explain", "--agent", "hermes", "--project", fx.project, source.ID)
			assertExit(t, result, 0)
			if !strings.Contains(string(result.stdout), "Discovery input: "+want+"\n") {
				t.Errorf("controlling %s owner absent from native registration explanation: want Discovery input: %s\n%s", tc.owner, want, result.stdout)
			}
		})
	}
}

func TestVerifyPolicyFollowupsGenuineFailureControl(t *testing.T) {
	fx, p := hermesFixture(t)
	file := g2Plugin(t, p)
	writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n  disabled: [portable]\n"), 0600)
	managed := filepath.Join(fx.home, "managed")
	writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins: [\n"), 0600)
	env := []string{"HERMES_MANAGED_DIR=" + managed}
	defer g2Seal(t, fx)()
	list := g2List(t, fx, 1, env)
	g2Inspect(t, fx, env, list, file, "unknown")
	found := false
	for _, problem := range list.Problems {
		found = found || problem.Code == "native-input"
	}
	if !found || list.Scope.Completeness != "incomplete" {
		t.Error("genuine managed parse failure erased")
	}
}
