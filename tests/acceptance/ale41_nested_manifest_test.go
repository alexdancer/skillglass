package acceptance_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

// Verify the accepted nested-bundle boundary against normal filesystem lookup,
// using only the acceptance harness's synthetic contained homes and projects.
func TestALE41NestedManifestCasePublic(t *testing.T) {
	for _, name := range []string{"SKILL.md", "skill.md", "sKiLl.Md", "data.txt"} {
		t.Run(name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			root := filepath.Join(fx.base, "allowed", "custom-bundles")
			parent := filepath.Dir(codexSkill(t, root, "parent", "nested-case-parent"))
			nested := filepath.Join(parent, "nested")
			nestedEntry := filepath.Join(nested, name)
			nestedBytes := []byte("---\nname: nested-independent\ndescription: Synthetic nested bundle.\n---\nNested sentinel.\n")
			writeFile(t, nestedEntry, nestedBytes, 0600)
			sibling := codexSkill(t, root, "independent-sibling", "sibling-sentinel")
			siblingBefore := readBytes(t, sibling)
			settings := filepath.Join(fx.home, ".codex", "config.toml")
			settingsBefore := []byte("# settings sentinel\nmodel = 'inert-fixture'\n")
			writeFile(t, settings, settingsBefore, 0600)

			manifestLookup := filepath.Join(nested, "SKILL.md")
			actual, err := os.Lstat(nestedEntry)
			if err != nil {
				t.Fatal(err)
			}
			manifestInfo, lookupErr := os.Lstat(manifestLookup)
			resolvesManifest := lookupErr == nil && os.SameFile(actual, manifestInfo)
			if lookupErr != nil && !os.IsNotExist(lookupErr) {
				t.Fatal(lookupErr)
			}
			if name == "SKILL.md" && !resolvesManifest {
				t.Fatal("canonical control failed normal lookup")
			}
			if name == "data.txt" && resolvesManifest {
				t.Fatal("no-manifest control unexpectedly resolves")
			}
			if resolvesManifest && !bytes.Equal(readBytes(t, manifestLookup), nestedBytes) {
				t.Fatal("manifest lookup bytes differ")
			}

			before := treeManifest(t, fx.base)
			prefix := "nested-manifest/" + map[string]string{"SKILL.md": "canonical", "skill.md": "lowercase", "sKiLl.Md": "mixedcase", "data.txt": "no-manifest"}[name] + "/"
			writeEvidence(t, prefix+"tree-before.json", before)
			preservePiCaseSurvivors(t, fx.base, prefix+"fixture-before")
			result := fx.run(t, "list", "--root", parent, "--json")
			assertExit(t, result, 0)
			var listed collection.View
			decodeJSON(t, result.stdout, &listed)
			id := ""
			for _, source := range listed.Sources {
				if source.Directory == parent {
					id = string(source.ID)
				}
			}
			if id == "" {
				t.Fatal("parent bundle missing from public listing")
			}
			args := []string{"delete", "--root", parent, "--project", fx.project, "--json"}
			result = changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, result, 1)
			var preview collection.DeleteView
			decodeJSON(t, result.stdout, &preview)
			writeEvidence(t, prefix+"preview.json", result.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			want := "ready"
			if resolvesManifest {
				want = "refused"
			}
			if preview.State != want {
				t.Errorf("normal filesystem manifest lookup resolves=%v: want preview %q, got %q (%s)", resolvesManifest, want, preview.State, preview.Reason)
			}
			if resolvesManifest && preview.State == "refused" && (!strings.Contains(preview.Reason, "another bundle") || preview.Confirmation != "") {
				t.Errorf("nested manifest refusal must have no confirmation: %+v", preview)
			}

			var outcome collection.DeleteOutcome
			applyCode := -1
			// Apply a healthy nonmanifest control, or capture the destructive
			// failure if a nested manifest is unexpectedly eligible.
			if preview.State == "ready" {
				if preview.Confirmation == "" {
					t.Fatal("ready preview omitted confirmation")
				}
				result = changeCommand(t, fx, "", append(append(args, "--confirm", preview.Confirmation), id)...)
				applyCode = result.code
				decodeJSON(t, result.stdout, &outcome)
				writeEvidence(t, prefix+"outcome.json", result.stdout)
			}
			_, parentErr := os.Lstat(parent)
			_, nestedErr := os.Lstat(nestedEntry)
			if parentErr != nil && !os.IsNotExist(parentErr) {
				t.Fatal(parentErr)
			}
			if nestedErr != nil && !os.IsNotExist(nestedErr) {
				t.Fatal(nestedErr)
			}
			siblingUnchanged := bytes.Equal(readBytes(t, sibling), siblingBefore)
			settingsUnchanged := bytes.Equal(readBytes(t, settings), settingsBefore)
			if !siblingUnchanged || !settingsUnchanged {
				t.Error("unselected sibling or settings marker changed")
			}
			if applyCode == -1 {
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
			writeEvidence(t, prefix+"tree-after.json", treeManifest(t, fx.base))
			preservePiCaseSurvivors(t, fx.base, prefix+"fixture-after")
			digest := func(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
			receipt := map[string]any{"nested_name": name, "nested_entry": nestedEntry, "normal_manifest_lookup": manifestLookup, "lookup_resolves_same_file": resolvesManifest, "entry_object": actual.Sys(), "lookup_error": func() string {
				if lookupErr != nil {
					return lookupErr.Error()
				}
				return ""
			}(), "want_preview_state": want, "preview_state": preview.State, "preview_reason": preview.Reason, "apply_exit": applyCode, "apply_state": outcome.State, "apply_verified": outcome.Verified, "parent_survives": parentErr == nil, "nested_entry_survives": nestedErr == nil, "sibling_unchanged": siblingUnchanged, "settings_unchanged": settingsUnchanged, "nested_input_sha256": digest(nestedBytes), "sibling_sha256": digest(siblingBefore), "settings_sha256": digest(settingsBefore)}
			raw, err := json.MarshalIndent(receipt, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			writeEvidence(t, prefix+"receipt.json", append(raw, '\n'))
			t.Logf("lookup_resolves=%v want=%s preview=%s reason=%s apply_exit=%d apply=%s verified=%v parent_survives=%v nested_survives=%v sibling_unchanged=%v settings_unchanged=%v", resolvesManifest, want, preview.State, preview.Reason, applyCode, outcome.State, outcome.Verified, parentErr == nil, nestedErr == nil, siblingUnchanged, settingsUnchanged)
			if !resolvesManifest && (applyCode != 0 || outcome.State != "removed" || !outcome.Verified || parentErr == nil || nestedErr == nil) {
				t.Error("healthy resource must remove only the selected parent")
			}
			if resolvesManifest && (parentErr != nil || nestedErr != nil) {
				t.Error("parent containing independently resolvable nested manifest must survive")
			}
		})
	}
}
