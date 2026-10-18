package acceptance_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type inspectionResult struct {
	SchemaVersion int    `json:"schema_version"`
	Operation     string `json:"operation"`
	ExitCode      int    `json:"exit_code"`
	Outcome       string `json:"outcome"`
	Coverage      struct {
		Discovery string `json:"discovery"`
		Selected  string `json:"selected"`
	} `json:"coverage"`
	Failure *struct {
		Code   string `json:"code"`
		Action string `json:"action"`
	} `json:"failure"`
	ResourceRead struct {
		SourceID       string  `json:"source_id"`
		State          string  `json:"state"`
		Representation string  `json:"representation"`
		Text           *string `json:"text"`
		BytesBase64    string  `json:"bytes_base64"`
		BytesRead      int64   `json:"bytes_read"`
		Diagnostics    []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	} `json:"resource_read"`
}

func inspectResult(t *testing.T, result commandResult) inspectionResult {
	t.Helper()
	var output inspectionResult
	decodeJSON(t, result.stdout, &output)
	if output.SchemaVersion != 1 || output.ExitCode != result.code || output.Operation == "" {
		t.Fatalf("invalid inspection contract: %s", result.stdout)
	}
	if len(result.stderr) != 0 {
		t.Fatalf("machine inspection wrote human stderr: %s", result.stderr)
	}
	return output
}

func TestALE45ReadSupportingResources(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "resource-review")
	bundle := filepath.Join(root, "one")
	copyPath := filepath.Join(root, "two")
	for _, path := range []string{bundle, copyPath} {
		writeSkill(t, path, "same-name", "Review evidence.", "Read references/deep/guide.md.\nDo not run scripts.")
		writeFile(t, filepath.Join(path, "references/deep/guide.md"), []byte("NESTED_EVIDENCE\nsecond line\n"), 0600)
	}
	writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte("---\nname: same-name\ndescription: Review evidence.\nexact_number: 9007199254740993\n---\nRead references/deep/guide.md.\nDo not run scripts."), 0600)
	writeFile(t, filepath.Join(copyPath, "references/deep/guide.md"), []byte("INDEPENDENT_COPY_EVIDENCE"), 0600)
	binary := []byte{0, 255, 27, 1, 'X'}
	writeFile(t, filepath.Join(bundle, "binary.dat"), binary, 0600)
	writeFile(t, filepath.Join(bundle, "oversized.txt"), bytes.Repeat([]byte("x"), (1<<20)+1), 0600)
	denied := filepath.Join(bundle, "unreadable.txt")
	writeFile(t, denied, []byte("UNREADABLE_CONTENT_MUST_NOT_LEAK"), 0600)
	writeFile(t, filepath.Join(bundle, "run.sh"), []byte("#!/bin/sh\ntouch "+shellQuote(fx.marker)+"\n"), 0700)
	if err := syscall.Mkfifo(filepath.Join(bundle, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(fx.base, "allowed", "external-evidence.txt")
	writeFile(t, external, []byte("EXTERNAL_CONTENT_MUST_NOT_LEAK"), 0600)
	for alias, target := range map[string]string{"internal.md": "references/deep/guide.md", "internal-dir": "references", "absolute.md": filepath.Join(bundle, "references/deep/guide.md"), "outside.md": external, "broken.md": "missing.md", "cycle.md": "cycle.md", "escape-chain": "outside.md", "parent.md": "internal-dir/deep/../deep/guide.md", "cancel-external": "outside.md/../references/deep/guide.md"} {
		if err := os.Symlink(target, filepath.Join(bundle, alias)); err != nil {
			t.Fatal(err)
		}
	}
	aliases := filepath.Join(fx.base, "allowed", "resource-aliases")
	if err := os.MkdirAll(aliases, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bundle, filepath.Join(aliases, "shared")); err != nil {
		t.Fatal(err)
	}
	fx.deniedReads = []string{denied}
	fx.seal(t)
	before := treeManifest(t, fx.base)
	scope := []string{"--root", root, "--root", aliases, "--project", fx.project, "--json"}
	listed := fx.run(t, append([]string{"list"}, scope...)...)
	output := inspectResult(t, listed)
	if listed.code != 0 || output.Outcome != "success" || !bytes.Contains(listed.stdout, []byte(`"exact_number": 9007199254740993`)) {
		t.Fatalf("list: %s", listed.stdout)
	}
	var list publicList
	decodeJSON(t, listed.stdout, &list)
	if len(list.Sources) != 2 {
		t.Fatalf("independent copies collapsed: %s", listed.stdout)
	}
	ids := map[string]string{}
	for _, source := range list.Sources {
		ids[source.SourcePath] = source.ID
	}
	id := ids[bundle]
	if id == "" || id == ids[copyPath] {
		t.Fatalf("missing distinct identity: %s", listed.stdout)
	}
	refs := 0
	for _, ref := range list.References {
		if ref.SourceID == id {
			refs++
		}
	}
	if refs != 2 {
		t.Fatalf("shared reference lost: %s", listed.stdout)
	}
	instructions := fx.run(t, append(append([]string{"read"}, scope...), id)...)
	inspectResult(t, instructions)
	if instructions.code != 0 || !bytes.Contains(instructions.stdout, []byte("Read references/deep/guide.md")) {
		t.Fatalf("read instructions: %s", instructions.stdout)
	}
	for _, path := range []string{"references/deep/guide.md", "internal.md", "internal-dir/deep/guide.md", "absolute.md", "parent.md", "run.sh", "binary.dat"} {
		result := fx.run(t, append(append([]string{"read-resource"}, scope...), id, path)...)
		read := inspectResult(t, result)
		if result.code != 0 || read.Coverage.Selected != "complete" || read.ResourceRead.SourceID != id {
			t.Fatalf("%s: %s", path, result.stdout)
		}
		if path == "binary.dat" {
			raw, err := base64.StdEncoding.DecodeString(read.ResourceRead.BytesBase64)
			if err != nil || !bytes.Equal(raw, binary) || read.ResourceRead.Representation != "base64" {
				t.Fatalf("binary: %s", result.stdout)
			}
		} else if path != "run.sh" && (read.ResourceRead.Text == nil || *read.ResourceRead.Text != "NESTED_EVIDENCE\nsecond line\n") {
			t.Fatalf("text: %s", result.stdout)
		}
	}
	independent := fx.run(t, append(append([]string{"read-resource"}, scope...), ids[copyPath], "references/deep/guide.md")...)
	if read := inspectResult(t, independent); read.ResourceRead.Text == nil || *read.ResourceRead.Text != "INDEPENDENT_COPY_EVIDENCE" {
		t.Fatalf("independent read: %s", independent.stdout)
	}
	for path, want := range map[string]string{"oversized.txt": "resource-too-large", "unreadable.txt": "resource-unreadable", "outside.md": "external-resource-link", "escape-chain": "external-resource-link", "cancel-external": "external-resource-link", "broken.md": "broken-resource-link", "cycle.md": "resource-link-cycle", "references": "unsupported-resource", "pipe": "unsupported-resource", "../external-evidence.txt": "invalid-resource-path", "SKILL.md": "invalid-resource-path"} {
		result := fx.run(t, append(append([]string{"read-resource"}, scope...), id, path)...)
		read := inspectResult(t, result)
		if result.code != 1 || read.Outcome != "failed" || read.Coverage.Selected != "incomplete" || read.ResourceRead.Text != nil || read.ResourceRead.BytesBase64 != "" || len(read.ResourceRead.Diagnostics) != 1 || read.ResourceRead.Diagnostics[0].Code != want {
			t.Fatalf("%s: %s", path, result.stdout)
		}
		if bytes.Contains(result.stdout, []byte("CONTENT_MUST_NOT_LEAK")) {
			t.Fatalf("unread evidence leaked: %s", result.stdout)
		}
	}
	human := fx.run(t, "read-resource", "--root", root, id, "references/deep/guide.md")
	if human.code != 0 || !bytes.Contains(human.stdout, []byte("NESTED_EVIDENCE\nsecond line")) {
		t.Fatalf("human reader: %s", human.stdout)
	}
	writeEvidence(t, "ale45/human-resource.txt", human.stdout)
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatalf("inspected script executed: %v", err)
	}
	after := treeManifest(t, fx.base)
	if !bytes.Equal(before, after) {
		t.Fatal("inspection changed contents or settings")
	}
	writeEvidence(t, "ale45/tree-before.json", before)
	writeEvidence(t, "ale45/tree-after.json", after)
}

func TestALE45PartialDiscoveryAndChangedIdentity(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "selected-review")
	bundle := filepath.Join(root, "selected")
	writeSkill(t, bundle, "selected", "Review evidence.", "INSTRUCTIONS")
	writeFile(t, filepath.Join(bundle, "guide.md"), []byte("SELECTED_EVIDENCE"), 0600)
	if err := os.Symlink("absent", filepath.Join(root, "broken-reference")); err != nil {
		t.Fatal(err)
	}
	listed := fx.run(t, "list", "--root", root, "--root", fx.missingRoot, "--json")
	var list publicList
	decodeJSON(t, listed.stdout, &list)
	if listed.code != 1 || len(list.Sources) != 1 {
		t.Fatalf("partial list: %s", listed.stdout)
	}
	output := inspectResult(t, listed)
	if output.Outcome != "partial" || output.Coverage.Discovery != "incomplete" {
		t.Fatalf("partial contract: %s", listed.stdout)
	}
	id := list.Sources[0].ID
	before := treeManifest(t, fx.base)
	for _, operation := range []string{"read", "explain", "read-resource"} {
		args := []string{operation, "--root", root, "--root", fx.missingRoot, "--json", id}
		if operation == "read-resource" {
			args = append(args, "guide.md")
		}
		result := fx.run(t, args...)
		read := inspectResult(t, result)
		if result.code != 1 || read.Outcome != "partial" || read.Coverage.Selected != "complete" || read.Coverage.Discovery != "incomplete" {
			t.Fatalf("useful partial %s: %s", operation, result.stdout)
		}
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("partial inspection mutated fixtures")
	}
	// Replace the same path with equal content. Object identity must not silently bind it.
	if err := os.Rename(bundle, filepath.Join(root, "old-kept")); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, bundle, "selected", "Review evidence.", "INSTRUCTIONS")
	writeFile(t, filepath.Join(bundle, "guide.md"), []byte("REPLACEMENT_MUST_NOT_BE_READ"), 0600)
	if err := os.RemoveAll(filepath.Join(root, "old-kept")); err != nil {
		t.Fatal(err)
	}
	before = treeManifest(t, fx.base)
	stale := fx.run(t, "read-resource", "--root", root, "--json", id, "guide.md")
	read := inspectResult(t, stale)
	if stale.code != 1 || read.ResourceRead.Diagnostics[0].Code != "source-not-found" || read.ResourceRead.Text != nil || bytes.Contains(stale.stdout, []byte("REPLACEMENT_MUST_NOT_BE_READ")) {
		t.Fatalf("stale selection: %s", stale.stdout)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("stale read mutated fixtures")
	}
}

func TestALE45MachineUsageAndEarlyErrors(t *testing.T) {
	fx := newFixture(t)
	before := treeManifest(t, fx.base)
	for _, args := range [][]string{
		{"--json"}, {"list", "--json=invalid"}, {"list", "--json"}, {"list", "--json", "--unknown"}, {"problems", "--json", "unexpected"},
		{"read", "--json"}, {"explain", "--json", "--agent", "unknown", "id"},
		{"read-resource", "--json", "--root", fx.cleanRoot, "id"},
		{"compare", "--json", "--root", fx.cleanRoot, "one"}, {"compare", "--json", "--root", fx.cleanRoot, "one", "two"},
		{"read", "--json", "--root", fx.cleanRoot, "unknown"}, {"unknown", "--json"},
	} {
		result := fx.run(t, args...)
		read := inspectResult(t, result)
		if result.code == 0 || read.Failure == nil || read.Failure.Action == "" || read.Outcome != "failed" {
			t.Fatalf("early error %v: %s", args, result.stdout)
		}
	}
	for _, args := range [][]string{{"help", "--json"}, {"list", "--json", "--help"}, {"read-resource", "--json", "--help"}} {
		result := fx.run(t, args...)
		inspectResult(t, result)
		var raw map[string]json.RawMessage
		decodeJSON(t, result.stdout, &raw)
		if result.code != 0 || !strings.Contains(string(raw["help"]), "read-resource") {
			t.Fatalf("machine help: %s", result.stdout)
		}
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("error paths mutated fixtures")
	}
}

func TestALE45UnreadableSelectedInstructions(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "unreadable-instructions")
	writeSkill(t, root, "unreadable-instructions", "Evidence.", "UNREADABLE_INSTRUCTIONS")
	fx.deniedReads = []string{filepath.Join(root, "SKILL.md")}
	fx.seal(t)
	before := treeManifest(t, fx.base)
	listed := fx.run(t, "list", "--root", root, "--json")
	var list publicList
	decodeJSON(t, listed.stdout, &list)
	if listed.code != 1 || len(list.Sources) != 1 {
		t.Fatalf("unreadable source list: %s", listed.stdout)
	}
	for _, operation := range []string{"read", "explain"} {
		result := fx.run(t, operation, "--root", root, "--json", list.Sources[0].ID)
		output := inspectResult(t, result)
		if result.code != 1 || output.Outcome != "failed" || output.Coverage.Selected != "failed" || bytes.Contains(result.stdout, []byte("UNREADABLE_INSTRUCTIONS")) {
			t.Fatalf("unreadable %s: %s", operation, result.stdout)
		}
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("unreadable inspection mutated fixture")
	}
}
