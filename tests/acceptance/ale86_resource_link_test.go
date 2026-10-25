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
	createEscapingResourceLinks(t, escapeBundle)
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
	createEscapingResourceLinks(t, bundle)

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

func TestALE86SequentialSymlinkReusePublicBoundaries(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: home-canary\ndescription: inert valid canary\n---\nFixture only.\n"), 0o644)
	customRoot := filepath.Join(fx.base, "allowed", "roots", "sequential-link-reuse")
	if err := os.MkdirAll(customRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	initialBundle := filepath.Join(customRoot, "initial-reuse")
	writeFile(t, filepath.Join(initialBundle, "SKILL.md"), []byte("---\nname: initial-reuse\ndescription: repeated initial target path\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(initialBundle, "dir", "payload"), []byte("SEQUENTIAL_REUSE_PAYLOAD\n"), 0o600)
	if err := os.Symlink("dir", filepath.Join(initialBundle, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("link/../link/payload", filepath.Join(initialBundle, "out")); err != nil {
		t.Fatal(err)
	}

	expandedBundle := filepath.Join(customRoot, "expanded-reuse")
	writeFile(t, filepath.Join(expandedBundle, "SKILL.md"), []byte("---\nname: expanded-reuse\ndescription: repeated expanded target path\n---\nFixture only.\n"), 0o644)
	writeFile(t, filepath.Join(expandedBundle, "dir", "payload"), []byte("SEQUENTIAL_REUSE_PAYLOAD\n"), 0o600)
	if err := os.Symlink("dir", filepath.Join(expandedBundle, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("link/../link/payload", filepath.Join(expandedBundle, "reuse")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("reuse", filepath.Join(expandedBundle, "out")); err != nil {
		t.Fatal(err)
	}

	cycleBundle := filepath.Join(customRoot, "real-cycle")
	writeFile(t, filepath.Join(cycleBundle, "SKILL.md"), []byte("---\nname: real-cycle\ndescription: real symlink cycle control\n---\nFixture only.\n"), 0o644)
	if err := os.Symlink("cycle", filepath.Join(cycleBundle, "cycle")); err != nil {
		t.Fatal(err)
	}
	for label, bundle := range map[string]string{"initial": initialBundle, "expanded": expandedBundle} {
		resolved, err := filepath.EvalSymlinks(filepath.Join(bundle, "out"))
		if err != nil || filepath.Clean(resolved) != filepath.Join(bundle, "dir", "payload") {
			t.Fatalf("%s finite-link fixture resolved to %q: %v", label, resolved, err)
		}
	}

	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	before := treeManifest(t, fx.base)
	scan := fx.run(t, "consolidate", "scan", "--app", "pi", "--root", customRoot, "--library", library, "--json")
	writeEvidence(t, "sequential-reuse/scan.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	if inventory.SelectedCoverage != collection.Complete {
		t.Fatalf("custom-root discovery is incomplete: %s", inventory.SelectedCoverage)
	}
	initial := sourceAtPath(t, inventory, initialBundle)
	expanded := sourceAtPath(t, inventory, expandedBundle)
	cycle := sourceAtPath(t, inventory, cycleBundle)
	for label, source := range map[string]collection.ConsolidationSource{"initial": initial, "expanded": expanded} {
		if !source.Eligible || source.FingerprintCoverage != collection.Complete || source.BundleFingerprint == "" || len(source.BlockedReasons) != 0 {
			t.Fatalf("finite %s symlink reuse was refused: eligible=%t coverage=%s fingerprint=%q reasons=%q", label, source.Eligible, source.FingerprintCoverage, source.BundleFingerprint, source.BlockedReasons)
		}
		read := fx.run(t, "read-resource", "--root", customRoot, "--json", string(source.ID), "out")
		writeEvidence(t, "sequential-reuse/"+label+"-read.json", read.stdout)
		result := inspectResult(t, read)
		if read.code != 0 || result.Outcome != "success" || result.ResourceRead.Text == nil || *result.ResourceRead.Text != "SEQUENTIAL_REUSE_PAYLOAD\n" {
			t.Fatalf("finite %s resource link did not resolve: %s", label, read.stdout)
		}
	}
	if cycle.Eligible || cycle.FingerprintCoverage != collection.Incomplete || cycle.BundleFingerprint != "" || !strings.Contains(strings.Join(cycle.BlockedReasons, " "), "cyclic") {
		t.Fatalf("real symlink cycle was not refused during fingerprinting: eligible=%t coverage=%s fingerprint=%q reasons=%q", cycle.Eligible, cycle.FingerprintCoverage, cycle.BundleFingerprint, cycle.BlockedReasons)
	}
	cycleRead := fx.run(t, "read-resource", "--root", customRoot, "--json", string(cycle.ID), "cycle")
	writeEvidence(t, "sequential-reuse/cycle-read.json", cycleRead.stdout)
	cycleResult := inspectResult(t, cycleRead)
	if cycleRead.code != 1 || cycleResult.ResourceRead.BytesRead != 0 || len(cycleResult.ResourceRead.Diagnostics) != 1 || cycleResult.ResourceRead.Diagnostics[0].Code != "resource-link-cycle" {
		t.Fatalf("real cycle was not refused without content: %s", cycleRead.stdout)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	piSource := filepath.Join(fx.home, ".pi", "agent", "skills", "duplicate-control")
	codexSource := filepath.Join(fx.home, ".codex", "skills", "duplicate-control")
	manifest := []byte("---\nname: duplicate-control\ndescription: complete identical portable fixture\n---\nFixture only.\n")
	writeFile(t, filepath.Join(piSource, "SKILL.md"), manifest, 0o644)
	writeFile(t, filepath.Join(codexSource, "SKILL.md"), manifest, 0o644)
	writeFile(t, filepath.Join(piSource, "references", "guide.md"), []byte("IDENTICAL_RESOURCE\n"), 0o600)
	writeFile(t, filepath.Join(codexSource, "references", "guide.md"), []byte("IDENTICAL_RESOURCE\n"), 0o600)
	beforeNativeScan := treeManifest(t, fx.base)
	nativeScan := fx.run(t, "consolidate", "scan", "--app", "pi", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "sequential-reuse/identical-native-scan.json", nativeScan.stdout)
	assertExit(t, nativeScan, 0)
	var nativeInventory collection.ConsolidationInventory
	decodeJSON(t, nativeScan.stdout, &nativeInventory)
	piItem := sourceAtPath(t, nativeInventory, piSource)
	codexItem := sourceAtPath(t, nativeInventory, codexSource)
	if nativeInventory.SelectedCoverage != collection.Complete || !nativeInventory.LibrarySafe || !piItem.Eligible || !codexItem.Eligible || piItem.FingerprintCoverage != collection.Complete || codexItem.FingerprintCoverage != collection.Complete || piItem.BundleFingerprint == "" || piItem.BundleFingerprint != codexItem.BundleFingerprint {
		t.Fatalf("identical native source controls failed: selected=%s library_safe=%t pi=%t/%s codex=%t/%s", nativeInventory.SelectedCoverage, nativeInventory.LibrarySafe, piItem.Eligible, piItem.FingerprintCoverage, codexItem.Eligible, codexItem.FingerprintCoverage)
	}
	assertTreeUnchanged(t, beforeNativeScan, treeManifest(t, fx.base))
	draft := buildConsolidationDraft(nativeInventory, map[collection.SourceID]string{piItem.ID: "share", codexItem.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "sequential-reuse-identical.json")
	writeConsolidationDraft(t, draftPath, draft)
	beforePreview := treeManifest(t, fx.base)
	preview := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "sequential-reuse/identical-native-preview.json", preview.stdout)
	var previewResult collection.ConsolidationPreview
	decodeJSON(t, preview.stdout, &previewResult)
	if preview.code != 0 || previewResult.State != "ready" {
		t.Fatalf("identical native source controls did not produce a ready preview: state=%s failure=%+v", previewResult.State, previewResult.Failure)
	}
	assertTreeUnchanged(t, beforePreview, treeManifest(t, fx.base))
}

func createEscapingResourceLinks(t *testing.T, bundle string) {
	t.Helper()
	if err := os.Symlink("..", filepath.Join(bundle, "sub", "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("up/../secret", filepath.Join(bundle, "sub", "out")); err != nil {
		t.Fatal(err)
	}
}
