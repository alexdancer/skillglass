package acceptance_test

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestALE45PositionalResourceNamesPreserveOutputMode(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "positional-resources")
	bundle := filepath.Join(root, "one")
	writeSkill(t, bundle, "one", "Review evidence.", "Read supporting evidence.")
	for _, name := range []string{"guide.md", "--json=false", "--json"} {
		writeFile(t, filepath.Join(bundle, name), []byte("EXPECTED_RESOURCE_CONTENT"), 0600)
	}
	fx.seal(t)
	before := treeManifest(t, fx.base)
	listed := fx.run(t, "list", "--root", root, "--json")
	inspectResult(t, listed)
	var list publicList
	decodeJSON(t, listed.stdout, &list)
	if listed.code != 0 || len(list.Sources) != 1 {
		t.Fatalf("inventory: %s", listed.stdout)
	}
	id := list.Sources[0].ID
	for _, tc := range []struct {
		name       string
		path       string
		machine    bool
		terminator bool
		flag       string
	}{
		{"machine ordinary control", "guide.md", true, false, ""},
		{"machine json-false filename", "--json=false", true, false, ""},
		{"human ordinary control", "guide.md", false, false, ""},
		{"human json filename", "--json", false, false, ""},
		{"machine terminator", "--json=false", true, true, ""},
		{"human terminator", "--json", false, true, ""},
		{"machine numeric boolean", "--json=false", true, false, "--json=1"},
		{"machine short boolean", "--json=false", true, false, "-json=t"},
		{"human explicit false", "--json", false, false, "--json=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"read-resource", "--root", root}
			if tc.flag != "" {
				args = append(args, tc.flag)
			} else if tc.machine {
				args = append(args, "--json")
			}
			if tc.terminator {
				args = append(args, "--")
			}
			result := fx.run(t, append(args, id, tc.path)...)
			if result.code != 0 || len(result.stderr) != 0 {
				t.Fatalf("read failed: exit=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
			}
			if tc.machine {
				read := inspectResult(t, result)
				if read.Outcome != "success" || read.Coverage.Selected != "complete" || read.ResourceRead.Text == nil || *read.ResourceRead.Text != "EXPECTED_RESOURCE_CONTENT" {
					t.Fatalf("machine resource contract: %s", result.stdout)
				}
			} else if !bytes.HasPrefix(result.stdout, []byte("Skillglass resource\n")) || !bytes.Contains(result.stdout, []byte("\nEXPECTED_RESOURCE_CONTENT\n")) {
				t.Fatalf("human resource content missing: %s", result.stdout)
			}
		})
	}
	after := treeManifest(t, fx.base)
	writeEvidence(t, "ale45-positional/tree-before.json", before)
	writeEvidence(t, "ale45-positional/tree-after.json", after)
	if !bytes.Equal(before, after) {
		t.Fatal("inspection changed contents or settings")
	}
}
