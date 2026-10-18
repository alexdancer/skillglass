package collection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Pi has file resources as well as bundles. File identity is separate from its
// containing directory, so two independent Markdown files never merge.
func (builder *discoveryBuilder) discoverPiFile(ctx context.Context, root DiscoveryRoot, path, relative string) {
	if filepath.Base(path) == "SKILL.md" {
		builder.discoverCandidate(ctx, root, filepath.Dir(path), relative)
		return
	}
	rid := referenceID(root.ID, filepath.ToSlash(relative))
	ref := Reference{ID: rid, Root: root.ID, Context: root.Context, DiscoveredPath: path, Status: "unresolved"}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		p := makeProblem(ProblemBrokenReference, path, "Pi Markdown reference cannot be resolved", err, true)
		p.RootIDs = []RootID{root.ID}
		p.ReferenceIDs = []ReferenceID{rid}
		builder.addProblem(p)
		ref.Problems = []Problem{p}
		builder.addReference(ref)
		return
	}
	manifest, problems := inspectManifestFile(path, true)
	token, identityErr := fileObjectToken(resolved)
	id := sourceID(token)
	identity := Identity{Method: "darwin-file-object", Certainty: "proven"}
	if identityErr != nil {
		id = SourceID("src_" + stableID("pi-file", string(rid)))
		identity = Identity{Method: "occurrence", Certainty: "uncertain"}
		problems = append(problems, makeProblem(ProblemIdentityUncertain, path, "Pi resource identity is uncertain", identityErr, true))
	}
	ref.Source = id
	ref.ResolvedPath = resolved
	ref.Status = "resolved"
	builder.addReference(ref)
	if old, ok := builder.sources[id]; ok {
		old.References = append(old.References, rid)
		builder.sources[id] = old
		return
	}
	for _, p := range problems {
		builder.addProblem(p)
	}
	// A standalone resource is a file target, never its parent directory.
	// Bundle comparison therefore reports incomplete coverage.
	builder.sources[id] = Source{ID: id, Identity: identity, Directory: resolved, Name: manifest.Name, Description: manifest.Description, Manifest: manifest, References: []ReferenceID{rid}, Problems: problems, Ownership: EvidenceState{State: "unknown", Reason: "standalone Markdown resource has no established bundle ownership"}}
}

func (builder *discoveryBuilder) discoverPiChildren(ctx context.Context, root DiscoveryRoot, path string, ancestors map[string]bool, count *int, first bool) {
	if ctx.Err() != nil {
		return
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}
	if ancestors[resolved] {
		p := makeProblem(ProblemSymlinkCycle, path, "Pi traversal cycle prevents complete discovery", nil, true)
		p.RootIDs = []RootID{root.ID}
		builder.addProblem(p)
		return
	}
	*count++
	ancestors[resolved] = true
	defer delete(ancestors, resolved)
	if present, _ := hasManifest(path); present {
		rel, _ := filepath.Rel(root.Path, path)
		builder.discoverCandidate(ctx, root, path, rel)
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		p := makeProblem(ProblemRootUnreadable, path, "Pi resource container cannot be read", err, true)
		p.RootIDs = []RootID{root.ID}
		builder.addProblem(p)
		return
	}
	for _, e := range entries {
		if *count >= maxResourceEntries {
			p := makeProblem(ProblemResourceLimit, path, "Pi traversal exceeds the entry limit", nil, true)
			p.RootIDs = []RootID{root.ID}
			builder.addProblem(p)
			return
		}
		*count++
		if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
			continue
		}
		child := filepath.Join(path, e.Name())
		rel, _ := filepath.Rel(root.Path, child)
		info, err := os.Stat(child)
		if err != nil {
			if e.Type()&os.ModeSymlink != 0 {
				builder.discoverCandidate(ctx, root, child, rel)
			}
			continue
		}
		if info.IsDir() {
			builder.discoverPiChildren(ctx, root, child, ancestors, count, false)
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		include := (root.piMode == "pi" && first) || (root.piMode == "agents" && !first)
		if include {
			builder.discoverPiFile(ctx, root, child, rel)
		}
	}
}
