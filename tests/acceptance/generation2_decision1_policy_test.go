package acceptance_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneration2Decision1Policy(t *testing.T) {
	for _, tc := range []struct {
		name, user, managed, state string
		exit                       int
	}{
		{"replaced-enabled-expansion", "enabled: ['${env:PRIVATE_PLUGIN}']", "enabled: [portable]", "explicit-only", 0},
		{"replaced-disabled-expansion", "enabled: [portable]\n  disabled: ['${env:PRIVATE_PLUGIN}']", "disabled: []", "explicit-only", 0},
		{"replaced-enabled-type", "enabled: false", "enabled: [portable]", "explicit-only", 0},
		{"replaced-enabled-item-type", "enabled: [true]", "enabled: [portable]", "explicit-only", 0},
		{"inherited-enabled-gap-control", "enabled: ['${env:PRIVATE_PLUGIN}']", "disabled: []", "unknown", 1},
		{"inherited-disabled-gap-control", "enabled: [portable]\n  disabled: ['${env:PRIVATE_PLUGIN}']", "enabled: [portable]", "unknown", 1},
		{"inherited-disabled-known-control", "enabled: ['${env:PRIVATE_PLUGIN}']\n  disabled: [portable]", "enabled: [portable]", "unavailable", 0},
		{"replaced-scalar-confidence-control", "enabled: [off]", "enabled: [portable]", "explicit-only", 0},
		{"replaced-disabled-type", "enabled: [portable]\n  disabled: false", "disabled: []", "explicit-only", 0},
		{"replaced-disabled-item-type", "enabled: [portable]\n  disabled: [true]", "disabled: []", "explicit-only", 0},
		{"replaced-enabled-null", "enabled: ['${env:PRIVATE_PLUGIN}']", "enabled: null", "unavailable", 0},
		{"replaced-disabled-null", "enabled: [portable]\n  disabled: ['${env:PRIVATE_PLUGIN}']", "disabled: null", "explicit-only", 0},
		{"inherited-enabled-type", "enabled: false", "disabled: []", "unknown", 1},
		{"inherited-disabled-type", "enabled: [portable]\n  disabled: [true]", "enabled: [portable]", "unknown", 1},
		{"managed-item-type", "enabled: [portable]", "enabled: [true]", "unknown", 1},
		{"managed-exact-membership", "enabled: ['${env:PRIVATE_PLUGIN}']", "enabled: [' portable ']", "unavailable", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  "+tc.user+"\n"), 0600)
			managed := filepath.Join(fx.home, "managed")
			writeFile(t, filepath.Join(managed, "config.yaml"), []byte("plugins:\n  "+tc.managed+"\n"), 0600)
			env := []string{"HERMES_MANAGED_DIR=" + managed}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, tc.exit, env)
			g2Inspect(t, fx, env, list, file, tc.state)
		})
	}
}

func TestGeneration2Decision1Fallback(t *testing.T) {
	for _, tc := range []struct {
		name, mode, state string
		exit              int
	}{
		{"dangling-yaml", "yaml", "unknown", 1},
		{"dangling-yml", "yml", "unknown", 1},
		{"dangling-both", "both", "unknown", 1},
		{"regular-yaml-precedence-control", "regular", "unknown", 0},
		{"portable-json-symlink-control", "jsonlink", "unknown", 0},
		{"no-yaml-control", "none", "explicit-only", 0},
		{"dangling-yaml-existing-yml", "ymltarget", "unknown", 1},
		{"in-root-json-symlink", "inrootjsonlink", "explicit-only", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := g2Plugin(t, p)
			root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
			if tc.mode == "yaml" || tc.mode == "both" || tc.mode == "ymltarget" {
				if e := os.Symlink("missing.yaml", filepath.Join(root, "plugin.yaml")); e != nil {
					t.Fatal(e)
				}
			}
			if tc.mode == "yml" || tc.mode == "both" {
				if e := os.Symlink("missing.yml", filepath.Join(root, "plugin.yml")); e != nil {
					t.Fatal(e)
				}
			}
			if tc.mode == "regular" {
				writeFile(t, filepath.Join(root, "plugin.yaml"), []byte("name: portable\nversion: '1'\n"), 0600)
			}
			if tc.mode == "ymltarget" {
				writeFile(t, filepath.Join(root, "plugin.yml"), []byte("name: portable\nversion: '1'\n"), 0600)
			}
			if tc.mode == "jsonlink" || tc.mode == "inrootjsonlink" {
				raw, e := os.ReadFile(filepath.Join(root, "plugin.json"))
				if e != nil {
					t.Fatal(e)
				}
				target := filepath.Join(p, "outside-portable.json")
				if tc.mode == "inrootjsonlink" {
					target = filepath.Join(root, "manifest.json")
				}
				writeFile(t, target, raw, 0600)
				if e := os.Remove(filepath.Join(root, "plugin.json")); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, filepath.Join(root, "plugin.json")); e != nil {
					t.Fatal(e)
				}
			}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, tc.exit, nil)
			g2Inspect(t, fx, nil, list, file, tc.state)
			participation := "eligible"
			if tc.mode == "regular" || tc.mode == "jsonlink" || tc.mode == "ymltarget" {
				participation = "unknown"
			}
			g2PluginRegistration(t, list, file, participation, "portable")
			if tc.exit == 1 {
				g2FallbackProblems(t, list)
			}
		})
	}
}
