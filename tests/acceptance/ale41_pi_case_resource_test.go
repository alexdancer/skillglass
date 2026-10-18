package acceptance_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

// Exercise the public executable through the repository's normal contained fixture.
// An in-package resource stays managed when its link uses another spelling of
// the same case-insensitive filesystem object. External resources stay eligible.
func TestALE41PiCaseResourceOwnershipPublic(t *testing.T) {
	for _, declaration := range []string{"literal", "glob-unverified"} {
		for _, topology := range []string{"canonical-contained", "case-alias-contained", "external-link"} {
			t.Run(declaration+"/"+topology, func(t *testing.T) {
				fx, _ := codexFixture(t)
				pkg := filepath.Join(fx.base, "allowed", "PackageRoot")
				resource := filepath.Join(pkg, "resources")
				if topology == "external-link" {
					resource = filepath.Join(fx.base, "allowed", "independent-resources")
				}
				manifest := codexSkill(t, resource, "target", "pi-case-resource")
				target := filepath.Dir(manifest)
				payload := filepath.Join(target, "payload.txt")
				writeFile(t, payload, []byte("selected inert resource bytes\n"), 0600)
				if err := os.MkdirAll(pkg, 0700); err != nil {
					t.Fatal(err)
				}
				resolvedLink := resource
				alias := filepath.Join(filepath.Dir(pkg), "pACKAGErOOT")
				pkgInfo, err := os.Stat(pkg)
				if err != nil {
					t.Fatal(err)
				}
				aliasInfo, aliasErr := os.Stat(alias)
				sameAlias := aliasErr == nil && os.SameFile(pkgInfo, aliasInfo)
				if topology == "case-alias-contained" {
					if !sameAlias {
						t.Skip("fixture filesystem has no proven case alias")
					}
					resolvedLink = filepath.Join(alias, "resources")
				}
				link := filepath.Join(pkg, "skills")
				if err := os.Symlink(resolvedLink, link); err != nil {
					t.Fatal(err)
				}
				resolvedBase, err := filepath.EvalSymlinks(pkg)
				if err != nil {
					t.Fatal(err)
				}
				resolvedResource, err := filepath.EvalSymlinks(link)
				if err != nil {
					t.Fatal(err)
				}
				resourceInfo, err := os.Stat(resource)
				if err != nil {
					t.Fatal(err)
				}
				linkInfo, err := os.Stat(link)
				if err != nil || !os.SameFile(resourceInfo, linkInfo) {
					t.Fatal("link does not resolve to intended resource object", err)
				}
				entry := "skills"
				if declaration == "glob-unverified" {
					entry = "skill*"
				}
				packageBytes, _ := json.Marshal(map[string]any{"name": "inert-fixture-package", "pi": map[string]any{"skills": []string{entry}}})
				packageManifest := filepath.Join(pkg, "package.json")
				writeFile(t, packageManifest, packageBytes, 0600)
				settings := filepath.Join(fx.home, ".pi", "agent", "settings.json")
				settingsBytes, _ := json.Marshal(map[string]any{"packages": []string{pkg}, "keep": "settings sentinel"})
				writeFile(t, settings, settingsBytes, 0600)
				sentinel := filepath.Join(pkg, "unselected-sentinel.txt")
				sentinelBytes := []byte("unselected package sentinel\n")
				writeFile(t, sentinel, sentinelBytes, 0600)
				label := declaration + "/" + topology
				before := treeManifest(t, fx.base)
				writeEvidence(t, "pi-case/"+label+"/tree-before.json", before)
				defer func() {
					writeEvidence(t, "pi-case/"+label+"/tree-after.json", treeManifest(t, fx.base))
					preservePiCaseSurvivors(t, fx.base, "pi-case/"+label+"/surviving-fixture")
				}()
				r := fx.run(t, "list", "--root", target, "--json")
				assertExit(t, r, 0)
				var list collection.View
				decodeJSON(t, r.stdout, &list)
				if len(list.Sources) != 1 {
					t.Fatalf("expected one selected source, got %+v", list.Sources)
				}
				id := string(list.Sources[0].ID)
				args := []string{"delete", "--root", target, "--project", fx.project, "--json"}
				r = changeCommand(t, fx, "", append(args, id)...)
				assertExit(t, r, 1)
				var preview collection.DeleteView
				decodeJSON(t, r.stdout, &preview)
				writeEvidence(t, "pi-case/"+label+"/preview.json", r.stdout)
				managedResource := false
				for _, consumer := range preview.Consumers {
					if consumer.Application != "pi" {
						continue
					}
					for _, manager := range consumer.Managers {
						if manager.Manager == pkg && manager.Reader == "" && manager.Boundary == link && manager.State == collection.ManagerBoundary {
							managedResource = true
						}
					}
				}
				if managedResource != (topology != "external-link") {
					t.Errorf("resource ownership fact = %v for %s", managedResource, topology)
				}
				// Preview cannot modify even an unsafe candidate.
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				var outcome collection.DeleteOutcome
				applyCode := -1
				if preview.State == "ready" {
					r = changeCommand(t, fx, "", append(append(args, "--confirm", preview.Confirmation), id)...)
					applyCode = r.code
					decodeJSON(t, r.stdout, &outcome)
					writeEvidence(t, "pi-case/"+label+"/outcome.json", r.stdout)
				}
				_, targetErr := os.Lstat(target)
				targetSurvives := targetErr == nil
				if targetErr != nil && !os.IsNotExist(targetErr) {
					t.Fatal(targetErr)
				}
				if !bytes.Equal(readBytes(t, settings), settingsBytes) {
					t.Error("settings bytes changed")
				}
				if !bytes.Equal(readBytes(t, packageManifest), packageBytes) {
					t.Error("package manifest bytes changed")
				}
				if !bytes.Equal(readBytes(t, sentinel), sentinelBytes) {
					t.Error("unselected package sentinel changed")
				}
				gotLink, err := os.Readlink(link)
				if err != nil || gotLink != resolvedLink {
					t.Error("resource link changed", err)
				}
				receipt := map[string]any{"declaration": declaration, "topology": topology, "package": pkg, "case_alias": alias, "case_alias_same_file": sameAlias, "package_object": pkgInfo.Sys(), "case_alias_object": func() any {
					if aliasInfo == nil {
						return nil
					}
					return aliasInfo.Sys()
				}(), "resource_link": link, "resource_link_target": resolvedLink, "resolved_package": resolvedBase, "resolved_resource": resolvedResource, "link_target_same_file": os.SameFile(resourceInfo, linkInfo), "source_id": id, "preview_state": preview.State, "preview_reason": preview.Reason, "apply_exit": applyCode, "apply_state": outcome.State, "apply_verified": outcome.Verified, "target_survives": targetSurvives, "settings_unchanged": bytes.Equal(readBytes(t, settings), settingsBytes), "package_manifest_unchanged": bytes.Equal(readBytes(t, packageManifest), packageBytes), "sentinel_unchanged": bytes.Equal(readBytes(t, sentinel), sentinelBytes)}
				raw, _ := json.MarshalIndent(receipt, "", "  ")
				writeEvidence(t, "pi-case/"+label+"/receipt.json", append(raw, '\n'))
				t.Logf("state=%s reason=%s apply=%s verified=%v target_survives=%v case_alias_same_file=%v resolved_base=%q resolved_resource=%q", preview.State, preview.Reason, outcome.State, outcome.Verified, targetSurvives, sameAlias, resolvedBase, resolvedResource)
				if declaration == "glob-unverified" && topology != "external-link" {
					// Native package-manifest glob membership is explicitly unverified in
					// this repository. Contained targets must refuse; no supported-glob claim is made.
					if preview.State != "refused" || !targetSurvives {
						t.Errorf("unverified glob must refuse and preserve target, got %q survives=%v", preview.State, targetSurvives)
					}
					return
				}
				if topology == "external-link" {
					if preview.State != "ready" || applyCode != 0 || outcome.State != "removed" || !outcome.Verified || targetSurvives {
						t.Errorf("external resource must remain independently eligible: preview=%q outcome=%q verified=%v survives=%v", preview.State, outcome.State, outcome.Verified, targetSurvives)
					}
				} else if preview.State != "refused" || !targetSurvives {
					t.Errorf("in-package managed resource must refuse and survive: preview=%q outcome=%q verified=%v survives=%v", preview.State, outcome.State, outcome.Verified, targetSurvives)
				}
			})
		}
	}
}

func preservePiCaseSurvivors(t *testing.T, root, relative string) {
	t.Helper()
	evidence := os.Getenv("SKILLGLASS_EVIDENCE")
	if evidence == "" {
		return
	}
	destination := filepath.Join(evidence, filepath.FromSlash(relative))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dest)
		}
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(dest, raw, 0600)
		}
		return nil
	})
	if err != nil {
		t.Error("preserve actual survivor fixture:", err)
	}
}
