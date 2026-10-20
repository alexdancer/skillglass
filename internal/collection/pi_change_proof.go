package collection

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
)

// piProveControlCoverage distinguishes an incomplete inspection inventory from
// an incomplete participating native search. This is deliberately a closed list
// of proven irrelevant gaps. New/unknown diagnostics refuse rather than inherit
// permission from an available-looking target.
func piProveControlCoverage(s *Snapshot, source Source) error {
	inputs := s.piControls
	for _, root := range s.scope.Roots {
		if root.Origin != "pi-inactive-cache" {
			continue
		}
		// A source reached through an ordinary alias can still live inside managed
		// storage hidden beyond an inspection limit. It is not an ordinary target.
		if deleteBoundaryContains(root.Requested, source.Manifest.Path) != deleteUnrelated {
			return errors.New("selected source cannot be separated from Pi managed storage")
		}
	}
	for _, ra := range source.Availability.References {
		if ra.Participation == ParticipationInspection {
			if !piProvenInspectionReference(s, source, ra) {
				return errors.New("selected Pi inspection occurrence has unproven reference or root provenance")
			}
			continue
		}
		if ra.Permission.State == PermissionUnknown || ra.Participation == ParticipationUnknown {
			return errors.New("selected Pi occurrence has uncertain native participation or permission")
		}
	}
	for _, problem := range s.problems {
		if !problem.LimitsCoverage {
			continue
		}
		inactive := len(problem.RootIDs) > 0
		for _, rid := range problem.RootIDs {
			fact, ok := inputs.facts[rid]
			if !ok || fact.trust != "inactive" {
				inactive = false
			}
		}
		if inactive {
			continue
		}
		// Native stat skips a proven dangling link. Its lexical identity, link text,
		// missing destination and ancestors are pinned and rediscovered before write.
		if problem.Code == ProblemBrokenReference && piDanglingReference(s, problem.Path) {
			continue
		}
		// A valid package manifest may declare a currently absent exact resource.
		// Unknown package registration/membership is never covered by this case.
		if problem.Code == ProblemRootMissing && piAbsentPackageResource(s, problem) {
			continue
		}
		if problem.Code == ProblemNativeInput && problem.piIgnoreInput && len(problem.ReferenceIDs) > 0 && !slices.Contains(problem.SourceIDs, source.ID) {
			// Inventory still enumerates unknown-ignore candidates. No alias of this
			// target has an uncertain ignore decision; all other search gaps are checked
			// independently below. Prospective checks retain each independent permission.
			continue
		}
		return errors.New("participating Pi discovery has an unbounded gap; exact target effects cannot be proven")
	}
	return nil
}
func piProvenInspectionReference(s *Snapshot, source Source, ra ReferenceAvailability) bool {
	ref, found := s.references[ra.Reference]
	if !found || ref.ID != ra.Reference || ref.Source != source.ID || !slices.Contains(source.References, ref.ID) || ra.Provenance.Kind != "custom" {
		return false
	}
	if _, native := s.piControls.facts[ref.Root]; native {
		return false
	}
	for _, root := range s.request.Roots {
		if root.ID != ref.Root || root.Origin != "" || root.Context != ref.Context || (root.Context != ContextCustom && root.Context != ContextProject) {
			continue
		}
		return slices.ContainsFunc(s.scope.Roots, func(report RootReport) bool {
			return report.ID == root.ID && report.Requested == root.Path && report.Context == root.Context && report.Origin == ""
		})
	}
	return false
}

func piDanglingReference(s *Snapshot, path string) bool {
	_, err := os.Stat(path)
	if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	for _, ref := range s.references {
		if ref.DiscoveredPath == path && ref.Source == "" {
			if _, native := s.piControls.facts[ref.Root]; native {
				return true
			}
		}
	}
	for _, root := range s.scope.Roots {
		if root.Requested == path && root.Origin != "" {
			return true
		}
	}
	return false
}
func piAbsentPackageResource(s *Snapshot, p Problem) bool {
	if len(p.RootIDs) == 0 {
		return false
	}
	for _, id := range p.RootIDs {
		fact, ok := s.piControls.facts[id]
		if !ok || fact.uncertain || fact.provenance.Kind != "plugin" || fact.trust == "inactive" {
			return false
		}
		raw, err := nativeBytes(filepath.Join(fact.base, "package.json"))
		if err != nil || len(raw) == 0 {
			return false
		}
		for _, root := range s.scope.Roots {
			if root.ID == id {
				if _, err := os.Lstat(root.Requested); !errors.Is(err, os.ErrNotExist) {
					return false
				}
			}
		}
	}
	return true
}
func piObservationFailureAllowed(s *Snapshot, path string, err error) bool {
	return s.piControls != nil && errors.Is(err, os.ErrNotExist) && piDanglingReference(s, path)
}
