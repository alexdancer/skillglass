package collection

func applyNativeProblems(snapshot *Snapshot, problems []Problem, roots map[RootID]DiscoveryRoot) {
	merged := make(map[ProblemID]Problem, len(problems))
	for _, problem := range problems {
		current, exists := merged[problem.ID]
		if !exists {
			current = problem
		}
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
		sourceLocal := len(problem.SourceIDs) > 0
		for _, rid := range snapshot.referenceOrder {
			reference := snapshot.references[rid]
			if sourceLocal {
				if !nativeContainsSource(problem.SourceIDs, reference.Source) {
					continue
				}
			} else if roots[reference.Root].Origin == "" {
				continue
			}
			problem.RootIDs = appendUniqueRootIDs(problem.RootIDs, reference.Root)
			problem.ReferenceIDs = appendUniqueReferenceIDs(problem.ReferenceIDs, rid)
			problem.SourceIDs = appendUniqueSourceIDs(problem.SourceIDs, reference.Source)
		}
		if len(problem.RootIDs) == 0 && !sourceLocal {
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

func nativeContainsSource(values []SourceID, id SourceID) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}
