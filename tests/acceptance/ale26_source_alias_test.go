package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestALE26SourceAliasNativeNamespaceAndCoexistence(t *testing.T) {
	fx, root := codexFixture(t)
	plugin := filepath.Join(root, "plugins", "sample")
	manifest := []byte(`{"name":"sample","version":"1.0.0","skills":"./skills/"}`)
	writeFile(t, filepath.Join(plugin, ".codex-plugin", "plugin.json"), manifest, 0600)
	sourceManifest := codexSkill(t, filepath.Join(plugin, "skills"), "skill", "duplicate")
	writeFile(t, filepath.Join(root, ".agents", "plugins", "marketplace.json"), []byte(`{"name":"local","plugins":[{"name":"sample","source":{"source":"local","path":"./plugins/sample"}}]}`), 0600)
	cache := filepath.Join(fx.home, ".codex", "plugins", "cache", "local", "sample", "1.0.0")
	writeFile(t, filepath.Join(cache, ".codex-plugin", "plugin.json"), manifest, 0600)
	cacheManifest := codexSkill(t, filepath.Join(cache, "skills"), "skill", "duplicate")
	global := filepath.Join(fx.home, ".agents", "skills")
	if err := os.MkdirAll(global, 0755); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"source-alias": sourceManifest, "cache-alias": cacheManifest} {
		if err := os.Symlink(filepath.Dir(path), filepath.Join(global, name)); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte(trustProject(root)+"[plugins.'sample@local']\nenabled=false\n"), 0600)
	before := treeManifest(t, fx.base)
	list := codexListResult(t, fx, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	var sourceID, cacheID string
	count := 0
	for _, source := range list.Sources {
		if source.Name != "duplicate" {
			continue
		}
		count++
		if strings.Contains(source.SourcePath, "/plugins/sample/skills/skill") {
			sourceID = source.ID
			if source.Ownership.State != "unknown" {
				t.Fatal("namespace transferred local ownership")
			}
			for _, capability := range source.Availability.Capabilities {
				if capability.Scope == "plugin" {
					t.Fatal("namespace invented manager control scope")
				}
			}
		} else {
			cacheID = source.ID
			if source.Ownership.State != "managed" {
				t.Fatal("cache ownership lost")
			}
		}
		if source.Availability.Session.State != "unknown" {
			t.Fatal("session confidence changed")
		}
		if source.Availability.NativeNameEvidence.Rule != "ancestor-plugin-manifest" {
			t.Fatal("namespace evidence missing")
		}

		if source.Availability.NativeName != "sample:duplicate" {
			t.Errorf("native ancestor manifest must name source/cache aliases sample:duplicate, got %s for %s", source.Availability.NativeName, source.SourcePath)
		}
		if source.Availability.Precedence.State != "coexisting" || len(source.Availability.Precedence.RelatedSources) != 1 {
			t.Errorf("independent aliases with same native name must coexist with one related source, got %+v for %s", source.Availability.Precedence, source.SourcePath)
		}
	}
	if count != 2 || sourceID == cacheID || sourceID == "" || cacheID == "" {
		t.Fatalf("expected two independent sources, got %d", count)
	}
	for _, source := range list.Sources {
		if source.ID == sourceID && source.Availability.Precedence.RelatedSources[0] != cacheID {
			t.Fatal("source reciprocal coexistence missing")
		}
		if source.ID == cacheID && source.Availability.Precedence.RelatedSources[0] != sourceID {
			t.Fatal("cache reciprocal coexistence missing")
		}
	}
	writeFile(t, filepath.Join(fx.home, ".codex", "config.toml"), []byte(trustProject(root)+"[plugins.'sample@local']\nenabled=false\n"+selector(cacheManifest, false)), 0600)
	before = treeManifest(t, fx.base)
	denied := codexListResult(t, fx, 0)
	for _, source := range denied.Sources {
		if source.ID == sourceID && source.Availability.State != "available" {
			t.Fatal("cache denial reached independent source")
		}
		if source.ID == cacheID && source.Availability.State != "disabled" {
			t.Fatal("cache manifest denial missing")
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	// Unsupported ancestor metadata stays explicit and cannot block on a FIFO.
	metadata := filepath.Join(plugin, ".codex-plugin", "plugin.json")
	if err := os.Remove(metadata); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(metadata, 0600); err != nil {
		t.Fatal(err)
	}
	uncertain := codexListResult(t, fx, 1)
	for _, source := range uncertain.Sources {
		if source.ID == sourceID && (source.Availability.NativeName != "" || source.Availability.NativeNameEvidence.Kind != "unknown" || source.Availability.Precedence.State != "unknown") {
			t.Fatal("unverified ancestor metadata guessed a namespace")
		}
	}

}
