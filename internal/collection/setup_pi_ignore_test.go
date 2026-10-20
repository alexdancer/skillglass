package collection

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupPiIgnoreRequiresCompleteProvenance(t *testing.T) {
	cases := []string{"valid-editorial-summary", "untyped", "wrong-code", "missing-inputs", "missing-ref", "missing-source", "missing-source-link", "wrong-source-identity", "outside-root-reference", "missing-root-id", "missing-source-id", "extra-source", "extra-root", "missing-root-report", "missing-root-fact", "missing-discovery", "missing-observation", "wrong-path", "wrong-observed-root", "root-unknown"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			leaf := filepath.Join(root, "unrelated")
			if err := os.Mkdir(leaf, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(leaf, ".gitignore"), []byte("ignored?/\n"), 0600); err != nil {
				t.Fatal(err)
			}
			ref := Reference{ID: "ref", Root: "root", Source: "source", DiscoveredPath: leaf}
			source := Source{ID: "source", References: []ReferenceID{ref.ID}, Manifest: Manifest{Path: filepath.Join(leaf, "SKILL.md")}}
			file := piOccurrenceFile(source, ref)
			d := newPiDiscovery()
			d.root = root
			d.decisions[file] = piIgnoreDecision{unknown: true}
			s := &Snapshot{scope: Scope{Roots: []RootReport{{ID: ref.Root, Requested: root}}}, references: map[ReferenceID]Reference{ref.ID: ref}, sources: map[SourceID]Source{source.ID: source}, piControls: &piInputs{facts: map[RootID]piRootFact{ref.Root: {discovery: d}}}}
			p := Problem{Code: ProblemNativeInput, piIgnoreInput: true, Path: root, Summary: "Editorial wording is deliberately independent", ReferenceIDs: []ReferenceID{ref.ID}, RootIDs: []RootID{ref.Root}, SourceIDs: []SourceID{source.ID}}
			switch name {
			case "untyped":
				p.piIgnoreInput = false
			case "wrong-code":
				p.Code = ProblemRootUnreadable
			case "missing-inputs":
				s.piControls = nil
			case "missing-ref":
				delete(s.references, ref.ID)
			case "missing-source":
				delete(s.sources, source.ID)
			case "wrong-source-identity":
				source.ID = "other"
				s.sources[ref.Source] = source
			case "outside-root-reference":
				ref.DiscoveredPath = filepath.Dir(root)
				s.references[ref.ID] = ref
			case "missing-source-link":
				source.References = nil
				s.sources[source.ID] = source
			case "missing-root-id":
				p.RootIDs = nil
			case "missing-source-id":
				p.SourceIDs = nil
			case "extra-source":
				p.SourceIDs = append(p.SourceIDs, "extra")
			case "extra-root":
				p.RootIDs = append(p.RootIDs, "extra")
			case "missing-root-report":
				s.scope.Roots = nil
			case "missing-root-fact":
				delete(s.piControls.facts, ref.Root)
			case "missing-discovery":
				s.piControls.facts[ref.Root] = piRootFact{}
			case "missing-observation":
				delete(d.decisions, file)
			case "wrong-path":
				p.Path = leaf
			case "wrong-observed-root":
				d.root = leaf
			case "root-unknown":
				if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored?/\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got := setupPiIgnoreUnrelated(context.Background(), s, p, filepath.Join(root, "skillglass-review"))
			if got != (name == "valid-editorial-summary") {
				t.Fatalf("scope proven = %t", got)
			}
		})
	}
}
