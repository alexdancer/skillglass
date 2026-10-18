package collection

import (
	"os"
	"path/filepath"
	"strings"
)

type DeleteDependency struct {
	Source       SourceID                 `json:"source_id"`
	Manifest     string                   `json:"manifest_path"`
	Resolved     string                   `json:"resolved_path"`
	RemovedPath  string                   `json:"removed_path"`
	References   []Reference              `json:"references"`
	Availability *AvailabilityExplanation `json:"availability,omitempty"`
}

func cloneDeleteDependencies(values []DeleteDependency) []DeleteDependency {
	out := append([]DeleteDependency(nil), values...)
	for i := range out {
		out[i].References = append([]Reference(nil), values[i].References...)
		for j, ref := range out[i].References {
			out[i].References[j] = cloneReference(ref)
		}
		if out[i].Availability != nil {
			a := cloneAvailability(*out[i].Availability)
			out[i].Availability = &a
		}
	}
	return out
}

func deleteNativeReferences(s *Snapshot, source Source) (AvailabilityExplanation, []Reference) {
	a := cloneAvailability(source.Availability)
	ids := map[ReferenceID]bool{}
	facts := a.References[:0]
	for _, fact := range a.References {
		switch fact.Participation {
		case ParticipationEligible, ParticipationExcluded, ParticipationUnknown:
			ids[fact.Reference] = true
			facts = append(facts, fact)
		}
	}
	a.References = facts
	if !ids[a.ReferenceID] {
		a.ReferenceID = ""
	}
	refs := []Reference{}
	for _, ref := range s.References(source.ID) {
		if ids[ref.ID] {
			refs = append(refs, ref)
		}
	}
	return a, refs
}

func deleteDependencies(s *Snapshot, target DeleteTarget, nativeOnly bool) []DeleteDependency {
	info, err := os.Stat(target.Path)
	if err != nil {
		return nil
	}
	var out []DeleteDependency
	for _, id := range s.order {
		other := s.sources[id]
		if id == target.Source || !deleteManifestReaches(other.Manifest.Path, info) {
			continue
		}
		a := cloneAvailability(other.Availability)
		refs := s.References(id)
		if nativeOnly {
			a, refs = deleteNativeReferences(s, other)
			if len(refs) == 0 {
				continue
			}
		}
		out = append(out, DeleteDependency{Source: id, Manifest: other.Manifest.Path, Resolved: other.Manifest.ResolvedPath, RemovedPath: target.Path, References: refs, Availability: &a})
	}
	return out
}

func deleteManifestReaches(path string, target os.FileInfo) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	pending := strings.Split(path, string(filepath.Separator))
	resolved := string(filepath.Separator)
	links := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			resolved = filepath.Dir(resolved)
			continue
		}
		current := filepath.Join(resolved, part)
		info, err := os.Lstat(current)
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 40 {
				return false
			}
			link, err := os.Readlink(current)
			if err != nil {
				return false
			}
			if filepath.IsAbs(link) {
				resolved = string(filepath.Separator)
			}
			pending = append(strings.Split(link, string(filepath.Separator)), pending...)
			continue
		}
		if os.SameFile(info, target) {
			return true
		}
		resolved = current
	}
	return false
}
