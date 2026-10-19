package collection

import "slices"

func applyNativeProblems(snapshot *Snapshot, problems []Problem, roots map[RootID]DiscoveryRoot) {
	merged := make(map[ProblemID]Problem, len(problems))
	for _, problem := range problems {
		current, exists := merged[problem.ID]
		if !exists {
			current = problem
		}
		current.nativeLocal = current.nativeLocal || problem.nativeLocal
		current.piIgnoreInput = current.piIgnoreInput || problem.piIgnoreInput
		current.SourceIDs = appendUniqueSourceIDs(current.SourceIDs, problem.SourceIDs...)
		current.ReferenceIDs = appendUniqueReferenceIDs(current.ReferenceIDs, problem.ReferenceIDs...)
		current.RootIDs = appendUniqueRootIDs(current.RootIDs, problem.RootIDs...)
		merged[problem.ID] = current
	}
	known := make(map[ProblemID]bool)
	for _, problem := range snapshot.problems {
		known[problem.ID] = true
	}
	for _, problem := range problems {
		if known[problem.ID] {
			continue
		}
		problem = merged[problem.ID]
		targets := problem
		local := targets.nativeLocal || len(targets.SourceIDs) > 0 || len(targets.RootIDs) > 0 || len(targets.ReferenceIDs) > 0
		for _, rid := range snapshot.referenceOrder {
			reference := snapshot.references[rid]
			if local {
				if !slices.Contains(targets.RootIDs, reference.Root) && !slices.Contains(targets.ReferenceIDs, rid) && !slices.Contains(targets.SourceIDs, reference.Source) {
					continue
				}
			} else if roots[reference.Root].Origin == "" {
				continue
			}
			problem.RootIDs = appendUniqueRootIDs(problem.RootIDs, reference.Root)
			problem.ReferenceIDs = appendUniqueReferenceIDs(problem.ReferenceIDs, rid)
			problem.SourceIDs = appendUniqueSourceIDs(problem.SourceIDs, reference.Source)
		}
		if len(problem.RootIDs) == 0 && !local {
			for _, root := range snapshot.scope.Roots {
				if root.Origin != "" {
					problem.RootIDs = append(problem.RootIDs, root.ID)
				}
			}
		}
		snapshot.problems = append(snapshot.problems, problem)
		for _, id := range problem.SourceIDs {
			source := snapshot.sources[id]
			source.Problems = append(source.Problems, problem)
			snapshot.sources[id] = source
		}
		for _, id := range problem.ReferenceIDs {
			reference := snapshot.references[id]
			reference.Problems = append(reference.Problems, problem)
			snapshot.references[id] = reference
		}
		if problem.LimitsCoverage {
			snapshot.scope.Completeness = Incomplete
		}
		known[problem.ID] = true
	}
	recountRootReports(snapshot.scope.Roots, snapshot.references, snapshot.problems)
}
