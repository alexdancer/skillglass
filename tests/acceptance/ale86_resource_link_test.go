package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86ConsolidationResolvesResourceLinksBeforeDotDot(t *testing.T) {
	fx := newFixture(t)
	customRoot := filepath.Join(fx.base, "allowed", "roots", "resource-link-resolution")
	if err := os.MkdirAll(customRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: home-canary\ndescription: inert valid canary\n---\nFixture only.\n"), 0o644)

	safeBundle := filepath.Join(customRoot, "safe-nested-links")
	writeFile(t, filepath.Join(safeBundle, "SKILL.md"), []byte("---\nname: safe-nested-links\ndescription: safe relative resource links\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(safeBundle, "assets", "guide.md"), []byte("SAFE_NESTED_RESOURCE\n"), 0o644)
	if err := os.MkdirAll(filepath.Join(safeBundle, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(safeBundle, "references", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(safeBundle, "sub", "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../assets/guide.md", filepath.Join(safeBundle, "references", "nested", "guide.alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("up/references/nested/guide.alias", filepath.Join(safeBundle, "sub", "through")); err != nil {
		t.Fatal(err)
	}

	escapeBundle := filepath.Join(customRoot, "escaping-link")
	writeFile(t, filepath.Join(escapeBundle, "SKILL.md"), []byte("---\nname: escaping-link\ndescription: escaping resource link\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(escapeBundle, "sub", "secret"), []byte("INTERNAL_SECRET\n"), 0o600)
	outside := filepath.Join(customRoot, "secret")
	writeFile(t, outside, []byte("OUTSIDE_SECRET\n"), 0o600)
	if err := os.Symlink("..", filepath.Join(escapeBundle, "sub", "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("up/../secret", filepath.Join(escapeBundle, "sub", "out")); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(escapeBundle, "sub", "out"))
	if err != nil {
		t.Fatal(err)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		t.Fatal(err)
	}
	outsideInfo, err := os.Stat(outside)
	if err != nil || !os.SameFile(resolvedInfo, outsideInfo) {
		t.Fatalf("fixture link does not resolve to the outside sibling: err=%v", err)
	}

	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	before := treeManifest(t, fx.base)
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--root", customRoot, "--library", library, "--json")
	writeEvidence(t, "resource-link/scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	if inventory.SelectedCoverage != collection.Complete {
		t.Fatalf("selected discovery coverage is incomplete: %s", inventory.SelectedCoverage)
	}
	safe := sourceAtPath(t, inventory, safeBundle)
	if !safe.Eligible || safe.FingerprintCoverage != collection.Complete || safe.BundleFingerprint == "" {
		t.Fatalf("safe nested relative links lost complete bundle evidence: eligible=%t coverage=%s reasons=%q", safe.Eligible, safe.FingerprintCoverage, safe.BlockedReasons)
	}
	escaping := sourceAtPath(t, inventory, escapeBundle)
	if escaping.Eligible || escaping.FingerprintCoverage != collection.Incomplete || escaping.BundleFingerprint != "" || !strings.Contains(strings.Join(escaping.BlockedReasons, " "), "link chain escapes the bundle") {
		t.Fatalf("escaping link retained complete or relocatable bundle evidence: eligible=%t coverage=%s fingerprint=%q reasons=%q", escaping.Eligible, escaping.FingerprintCoverage, escaping.BundleFingerprint, escaping.BlockedReasons)
	}

	safeRead := fx.run(t, "read-resource", "--root", customRoot, "--json", string(safe.ID), "sub/through")
	writeEvidence(t, "resource-link/safe-read.json", safeRead.stdout)
	safeResult := inspectResult(t, safeRead)
	if safeRead.code != 0 || safeResult.Outcome != "success" || safeResult.ResourceRead.Text == nil || *safeResult.ResourceRead.Text != "SAFE_NESTED_RESOURCE\n" {
		t.Fatalf("safe nested resource link could not be read: %s", safeRead.stdout)
	}

	escapeRead := fx.run(t, "read-resource", "--root", customRoot, "--json", string(escaping.ID), "sub/out")
	writeEvidence(t, "resource-link/escape-read.json", escapeRead.stdout)
	escapeResult := inspectResult(t, escapeRead)
	if escapeRead.code != 1 || escapeResult.Outcome != "failed" || escapeResult.ResourceRead.BytesRead != 0 || escapeResult.ResourceRead.Text != nil || escapeResult.ResourceRead.BytesBase64 != "" || len(escapeResult.ResourceRead.Diagnostics) != 1 || escapeResult.ResourceRead.Diagnostics[0].Code != "external-resource-link" {
		t.Fatalf("external resource link was not refused without reading bytes: %s", escapeRead.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE86NativeResourceDirectoryCycleStillBlocksPreview(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: home-canary\ndescription: inert valid canary\n---\nFixture only.\n"), 0o644)
	bundle := filepath.Join(fx.home, ".pi", "agent", "skills", "resource-link-cycle")
	writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte("---\nname: resource-link-cycle\ndescription: directory cycle guard\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(bundle, "sub", "secret"), []byte("INTERNAL_SECRET\n"), 0o600)
	writeFile(t, filepath.Join(filepath.Dir(bundle), "secret"), []byte("OUTSIDE_SECRET\n"), 0o600)
	if err := os.Symlink("..", filepath.Join(bundle, "sub", "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("up/../secret", filepath.Join(bundle, "sub", "out")); err != nil {
		t.Fatal(err)
	}

	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--library", library, "--json")
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	if inventory.SelectedCoverage != collection.Complete || inventory.LibrarySafe {
		t.Fatalf("native directory-cycle fixture did not retain complete discovery and unsafe-library evidence: coverage=%s safe=%t reasons=%q", inventory.SelectedCoverage, inventory.LibrarySafe, inventory.LibraryReasons)
	}
	source := sourceAtPath(t, inventory, bundle)
	draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "keep"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "resource-link-cycle.json")
	writeConsolidationDraft(t, draftPath, draft)
	beforePreview := treeManifest(t, fx.base)
	preview := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	assertExit(t, preview, 1)
	var refused collection.ConsolidationPreview
	decodeJSON(t, preview.stdout, &refused)
	if refused.State != "refused" || refused.Failure == nil || refused.Failure.Code != "unsafe-library" {
		t.Fatalf("native directory-cycle guard did not block preview: state=%s failure=%+v", refused.State, refused.Failure)
	}
	assertTreeUnchanged(t, beforePreview, treeManifest(t, fx.base))
}
