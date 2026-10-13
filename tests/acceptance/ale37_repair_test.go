package acceptance_test

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestALE37NativeFallbackContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, fields, native, status string
		exit                         int
	}{
		{"spaces", "name: actual-native\ndescription: A workflow: details\n  name: fictional-native", "actual-native", "malformed", 1},
		{"tabs-crlf", "name: actual-native\r\ndescription: A workflow: details\r\n\tname: fictional-native", "actual-native", "malformed", 1},
		{"blank-terminates", "name: actual-native\ndescription: A workflow: details\n  continuation\n\n  name: replacement-native", "replacement-native", "malformed", 1},
		{"nonindented-terminates", "name: actual-native\ndescription: A workflow: details\n  continuation\nname: replacement-native", "replacement-native", "malformed", 1},
		{"whitespace-terminates", "name: actual-native\ndescription: A workflow: details\n  continuation\n  \n  name: replacement-native", "replacement-native", "malformed", 1},
		{"valid-yaml", "name: actual-native\ndescription: ordinary description", "actual-native", "parsed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := geminiFixture(t)
			raw := "---\n" + tc.fields + "\n---\nINERT_RAW_BYTES\n"
			writeFile(t, filepath.Join(fx.home, ".gemini/skills/fallback/SKILL.md"), []byte(raw), 0600)
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":false}},"skills":{"disabled":["ACTUAL-NATIVE","REPLACEMENT-NATIVE"]}}`), 0600)
			before := treeManifest(t, fx.base)
			listed := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--availability", "disabled", "--json")
			assertExit(t, listed, tc.exit)
			var inventory codexList
			decodeJSON(t, listed.stdout, &inventory)
			if len(inventory.Sources) != 1 {
				t.Fatalf("disabled sources: %s", listed.stdout)
			}
			source := inventory.Sources[0]
			if source.Availability.NativeName != tc.native || source.Availability.State != "disabled" {
				t.Fatalf("native state: %+v", source.Availability)
			}
			var generic publicList
			decodeJSON(t, listed.stdout, &generic)
			if generic.Sources[0].Manifest.RawText != raw || generic.Sources[0].Manifest.Status != tc.status {
				t.Fatalf("generic manifest changed: %+v", generic.Sources[0].Manifest)
			}
			if tc.exit == 1 && !bytes.Contains(listed.stdout, []byte("malformed-manifest")) {
				t.Fatal("generic diagnostic lost")
			}
			explained := fx.run(t, "explain", "--agent", "gemini", "--project", fx.project, "--json", source.ID)
			assertExit(t, explained, tc.exit)
			var detail struct{ Source codexSource }
			decodeJSON(t, explained.stdout, &detail)
			if detail.Source.Availability.NativeName != tc.native || detail.Source.Availability.State != "disabled" {
				t.Fatalf("explain native state: %s", explained.stdout)
			}
			for _, layer := range inventory.Scope.Agent.ConfigLayers {
				if layer.Path == "/Library/Application Support/GeminiCli/settings.json" || layer.Path == "/Library/Application Support/GeminiCli/system-defaults.json" {
					t.Fatalf("live system route: %+v", layer)
				}
			}
			writeEvidence(t, "ale37/repair/"+tc.name+"-list.json", listed.stdout)
			writeEvidence(t, "ale37/repair/"+tc.name+"-explain.json", explained.stdout)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
