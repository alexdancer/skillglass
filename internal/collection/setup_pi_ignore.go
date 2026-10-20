package collection

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
)

// Only adapter-classified ignore uncertainty can be separated from a prospective
// install. Reference paths alone cannot prove this: a root-level unknown rule
// attaches the same reference as a leaf-level rule but can affect every sibling.
func setupPiIgnoreUnrelated(ctx context.Context, s *Snapshot, p Problem, destination string) bool {
	if p.Code != ProblemNativeInput || !p.piIgnoreInput || s.piControls == nil || len(p.ReferenceIDs) == 0 || len(p.RootIDs) == 0 {
		return false
	}
	checked := map[RootID]bool{}
	checkedSources := map[SourceID]bool{}
	for _, id := range p.ReferenceIDs {
		ref, ok := s.references[id]
		source, sourceOK := s.sources[ref.Source]
		fact, factOK := s.piControls.facts[ref.Root]
		if !ok || ref.ID != id || !sourceOK || source.ID != ref.Source || !slices.Contains(source.References, id) || !slices.Contains(p.RootIDs, ref.Root) || !slices.Contains(p.SourceIDs, ref.Source) || !factOK || fact.discovery == nil {
			return false
		}
		decision, observed := fact.discovery.decisions[piOccurrenceFile(source, ref)]
		if !observed || !decision.unknown {
			return false
		}
		found := false
		for _, root := range s.scope.Roots {
			if root.ID != ref.Root {
				continue
			}
			found = root.Requested == p.Path && pathWithin(root.Requested, ref.DiscoveredPath) && fact.discovery.root == root.Requested && setupPiProspectiveIgnoreKnown(ctx, s, root, destination)
			break
		}
		if !found {
			return false
		}
		checked[ref.Root] = true
		checkedSources[ref.Source] = true
	}
	for _, id := range p.RootIDs {
		if !checked[id] {
			return false
		}
	}
	for _, id := range p.SourceIDs {
		if !checkedSources[id] {
			return false
		}
	}
	return true
}

// Replay only the prospective occurrence's actual ignore-input chain. The native
// matcher qualifies child inputs to that child's subtree, so an unknown sibling
// input cannot affect this occurrence. Read root and every prospective ancestor,
// even when no current manifest exists there to produce an inventory diagnostic.
func setupPiProspectiveIgnoreKnown(ctx context.Context, s *Snapshot, root RootReport, destination string) bool {
	if s.piControls == nil {
		return false
	}
	fact, ok := s.piControls.facts[root.ID]
	if !ok || fact.discovery == nil {
		return false
	}
	relation := setupPathsOverlap(destination, root.Requested)
	if relation == deleteUnrelated {
		return true
	}
	if relation != deleteRelated || !pathWithin(root.Requested, destination) {
		return false
	}
	relative, err := filepath.Rel(root.Requested, destination)
	if err != nil {
		return false
	}
	discovery := newPiDiscovery()
	matcher := &piIgnoreMatcher{}
	dir := root.Requested
	parts := []string{}
	if relative != "." {
		parts = strings.Split(relative, string(filepath.Separator))
	}
	if len(parts) > maxResourceEntries {
		return false
	}
	for i := 0; ; i++ {
		if ctx.Err() != nil {
			return false
		}
		// Resolve existing components before reading. Missing tails remain prospective;
		// alias cycles, non-directories and inaccessible ancestry cannot establish scope.
		if _, err := setupProspectivePath(dir); err != nil {
			return false
		}
		matcher.addDirectory(discovery, root.Requested, dir)
		if i == len(parts) {
			break
		}
		dir = filepath.Join(dir, parts[i])
	}
	occurrence := filepath.Join(destination, "SKILL.md")
	return !matcher.uncertain(piIgnoreRelative(discovery, occurrence))
}
