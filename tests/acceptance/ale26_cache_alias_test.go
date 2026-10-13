package acceptance_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestALE26CacheAliasOwnershipAndDiscoveryRoute(t *testing.T) {
	for _, route := range []string{"declared", "outside", "orphan", "stale", "manifest-name", "outside-target"} {
		t.Run(route, func(t *testing.T) {
			fx, root := codexFixture(t)
			source := filepath.Join(root, "plugins", "sample")
			manifest := []byte(`{"name":"sample","version":"1.0.0","skills":"./skills/"}`)
			writeFile(t, filepath.Join(source, ".codex-plugin", "plugin.json"), manifest, 0600)
			codexSkill(t, filepath.Join(source, "skills"), "normal", "declared-source")
			writeFile(t, filepath.Join(root, ".agents", "plugins", "marketplace.json"), []byte(`{"name":"local","plugins":[{"name":"sample","source":{"source":"local","path":"./plugins/sample"}}]}`), 0600)
			cache := filepath.Join(fx.home, ".codex", "plugins", "cache", "local", "sample", "1.0.0")
			if route == "stale" {
				cache = filepath.Join(fx.home, ".codex", "plugins", "cache", "local", "sample", "0.9.0")
				manifest = []byte(`{"name":"sample","version":"0.9.0","skills":"./skills/"}`)
			}
			if route == "manifest-name" {
				manifest = []byte(`{"name":"namespace-from-manifest","version":"1.0.0","skills":"./skills/"}`)
			}
			writeFile(t, filepath.Join(cache, ".codex-plugin", "plugin.json"), manifest, 0600)
			codexSkill(t, filepath.Join(cache, "skills"), "normal", "declared-cache")
			target := filepath.Join(cache, "other")
			if route == "declared" || route == "stale" {
				target = filepath.Join(cache, "skills")
			}
			if route == "orphan" {
				target = filepath.Join(fx.home, ".codex", "plugins", "cache", "orphan", "orphan", "old", "skills")
			}
			outside := codexSkill(t, target, "bundle", "aliased-cache")
			if route == "outside-target" {
				if err := os.RemoveAll(filepath.Dir(outside)); err != nil {
					t.Fatal(err)
				}
				external := codexSkill(t, filepath.Join(fx.base, "allowed", "external"), "source", "aliased-cache")
				if err := os.Symlink(filepath.Dir(external), filepath.Dir(outside)); err != nil {
					t.Fatal(err)
				}
			}
			global := filepath.Join(fx.home, ".agents", "skills")
			if err := os.MkdirAll(global, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Dir(outside), filepath.Join(global, "cache-alias")); err != nil {
				t.Fatal(err)
			}
			pluginState := "false"
			if route == "stale" {
				pluginState = "true"
			}
			writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte(trustProject(root)+"[plugins.'sample@local']\nenabled="+pluginState+"\n"), 0600)
			before := treeManifest(t, fx.base)
			list := codexListResult(t, fx, 0)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			got := codexNamed(t, list, "aliased-cache")
			if got.Availability.State != "available" {
				t.Fatalf("native user-root alias participation must remain available, got %+v", got)
			}
			expectedOwnership := "managed"
			if route == "outside-target" {
				expectedOwnership = "unknown"
			}
			if got.Ownership.State != expectedOwnership {
				t.Fatalf("managed ownership lost: %+v", got)
			}
			expectedName := "sample:aliased-cache"
			if route == "orphan" || route == "outside-target" {
				expectedName = "aliased-cache"
			}
			if route == "manifest-name" {
				expectedName = "namespace-from-manifest:aliased-cache"
			}
			if got.Availability.NativeName != expectedName {
				t.Fatalf("native name=%s want %s", got.Availability.NativeName, expectedName)
			}
			expectedScope := "plugin"
			if route == "outside-target" {
				expectedScope = "global"
			}
			for _, capability := range got.Availability.Capabilities {
				if capability.State != "unsupported" || (capability.Operation != "project-override" && capability.Scope != expectedScope) {
					t.Fatalf("managed capability: %+v", capability)
				}
			}
			if got.Availability.Session.State != "unknown" {
				t.Fatal("inferred session observation")
			}
			eligible, excluded, unknown := 0, 0, 0
			for _, fact := range got.Availability.References {
				switch fact.Participation {
				case "eligible":
					eligible++
				case "excluded":
					excluded++
				case "unknown":
					unknown++
				}
			}
			if eligible != 1 {
				t.Fatalf("user route participation: %+v", got.Availability.References)
			}
			if (route == "declared" || route == "stale") && excluded != 1 {
				t.Fatalf("disabled cache route: %+v", got.Availability.References)
			}
			if route == "orphan" && unknown != 1 {
				t.Fatalf("unknown cache route: %+v", got.Availability.References)
			}
			expectedProvenance := "plugin"
			if route == "outside-target" {
				expectedProvenance = "global"
			}
			if got.Availability.References[0].Provenance.Kind != expectedProvenance {
				t.Fatalf("cache alias must retain plugin provenance, got %+v", got)
			}
			for _, target := range []string{outside, filepath.Join(global, "cache-alias", "SKILL.md")} {
				writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte(trustProject(root)+"[plugins.'sample@local']\nenabled=false\n"+selector(target, false)), 0600)
				before := treeManifest(t, fx.base)
				disabled := codexNamed(t, codexListResult(t, fx, 0), "aliased-cache")
				if disabled.Availability.State != "disabled" {
					t.Fatalf("canonical/alias selector failed: %+v", disabled)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
		})
	}
}
