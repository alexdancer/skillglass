package collection

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const maxResourceEntries = 10_000

type discoveryBuilder struct {
	request        Request
	sources        map[SourceID]Source
	references     map[ReferenceID]Reference
	referenceOrder []ReferenceID
	problems       []Problem
	identityIndex  map[string]SourceID
}

func discover(ctx context.Context, request Request) (*Snapshot, error) {
	builder := discoveryBuilder{
		request:       request,
		sources:       make(map[SourceID]Source),
		references:    make(map[ReferenceID]Reference),
		identityIndex: make(map[string]SourceID),
	}
	rootReports := make([]RootReport, 0, len(request.Roots))
	for _, root := range request.Roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rootReports = append(rootReports, builder.discoverRoot(ctx, root))
	}
	builder.addRelationshipProblems()
	builder.finalizeProblemRelationships()
	recountRootReports(rootReports, builder.references, builder.problems)

	completeness := Complete
	for _, problem := range builder.problems {
		if problem.LimitsCoverage {
			completeness = Incomplete
			break
		}
	}
	return &Snapshot{
		request: cloneRequest(request),
		scope: Scope{
			Project:      request.Project,
			Roots:        rootReports,
			Completeness: completeness,
		},
		sources:        builder.sources,
		order:          sortedSourceIDs(builder.sources),
		references:     builder.references,
		referenceOrder: append([]ReferenceID(nil), builder.referenceOrder...),
		problems:       append([]Problem(nil), builder.problems...),
	}, nil
}

func recountRootReports(reports []RootReport, references map[ReferenceID]Reference, allProblems []Problem) {
	for index := range reports {
		report := &reports[index]
		sourceCount := 0
		for _, reference := range references {
			if reference.Root == report.ID && reference.Source != "" {
				sourceCount++
			}
		}
		report.SourceCount = sourceCount
		problems := make(map[ProblemID]Problem)
		for _, problem := range allProblems {
			if containsRootID(problem.RootIDs, report.ID) {
				problems[problem.ID] = problem
			}
		}
		report.ProblemCount = len(problems)
		if report.State == RootMissing || report.State == RootUnreadable {
			continue
		}
		if report.SourceCount == 0 && len(problems) == 0 {
			report.State = RootEmpty
		} else {
			report.State = RootSearched
		}
		for _, problem := range problems {
			if problem.LimitsCoverage {
				report.State = RootIncomplete
				break
			}
		}
	}
}

func (builder *discoveryBuilder) discoverRoot(ctx context.Context, root DiscoveryRoot) RootReport {
	report := RootReport{
		ID:        root.ID,
		Requested: root.Path,
		Context:   root.Context,
		Origin:    root.Origin,
		State:     RootSearched,
	}
	problemStart := len(builder.problems)
	referenceStart := len(builder.referenceOrder)

	resolvedRoot, err := filepath.EvalSymlinks(root.Path)
	if err != nil {
		code := ProblemRootUnreadable
		state := RootUnreadable
		if isSymlinkCycle(err) {
			code = ProblemSymlinkCycle
			state = RootIncomplete
		} else if rootInfo, lstatErr := os.Lstat(root.Path); lstatErr == nil && rootInfo.Mode()&os.ModeSymlink != 0 {
			code = ProblemBrokenReference
			state = RootIncomplete
		} else if errors.Is(err, os.ErrNotExist) {
			code = ProblemRootMissing
			state = RootMissing
		}
		if state == RootMissing && root.Optional {
			report.State = RootMissing
			return report
		}
		problem := makeProblem(code, root.Path, "selected root could not be opened", err, true)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		report.State = state
		report.ProblemCount = 1
		return report
	}
	report.Resolved = resolvedRoot

	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil {
		problem := makeProblem(
			ProblemRootUnreadable,
			root.Path,
			"selected root could not be inspected",
			err,
			true,
		)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		report.State = RootUnreadable
		report.ProblemCount = 1
		return report
	}
	if !rootInfo.IsDir() {
		err := fmt.Errorf("selected root resolves to %s, not a directory", rootInfo.Mode().Type())
		problem := makeProblem(
			ProblemRootUnreadable,
			root.Path,
			"selected root is not a directory",
			err,
			true,
		)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		report.State = RootUnreadable
		report.ProblemCount = 1
		return report
	}

	rootIsBundle, manifestErr := hasManifest(resolvedRoot)
	if rootIsBundle {
		if builder.discoverCandidate(ctx, root, root.Path, ".") {
			report.SourceCount++
		}
		if strings.HasPrefix(root.Origin, "hermes-") && root.Origin != "hermes-plugin" {
			builder.discoverHermesChildren(ctx, root, root.Path, make(map[string]bool), new(int))
		}
	} else {
		entries, readErr := os.ReadDir(resolvedRoot)
		if readErr != nil {
			problem := makeProblem(
				ProblemRootUnreadable,
				root.Path,
				"selected root could not be listed",
				readErr,
				true,
			)
			problem.RootIDs = []RootID{root.ID}
			builder.addProblem(problem)
			report.State = RootUnreadable
			report.ProblemCount = 1
			return report
		}
		if manifestErr != nil {
			problem := manifestPresenceProblem(resolvedRoot, manifestErr)
			problem.RootIDs = []RootID{root.ID}
			builder.addProblem(problem)
		}
		sort.Slice(entries, func(left, right int) bool {
			return entries[left].Name() < entries[right].Name()
		})
		if root.Origin != "" && !strings.HasPrefix(root.Origin, "claude-") {
			if strings.HasPrefix(root.Origin, "hermes-") {
				builder.discoverHermesChildren(ctx, root, root.Path, make(map[string]bool), new(int))
			} else {
				builder.discoverNativeChildren(ctx, root, root.Path, make(map[string]bool), new(int))
			}
			entries = nil
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				problem := makeProblem(
					ProblemRootUnreadable,
					root.Path,
					"discovery was interrupted",
					err,
					true,
				)
				problem.RootIDs = []RootID{root.ID}
				builder.addProblem(problem)
				break
			}
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			discoveredPath := filepath.Join(root.Path, entry.Name())
			if builder.discoverCandidate(ctx, root, discoveredPath, entry.Name()) {
				report.SourceCount++
			}
		}
	}

	rootProblems := make(map[ProblemID]Problem)
	for _, problem := range builder.problems[problemStart:] {
		rootProblems[problem.ID] = problem
	}
	for _, id := range builder.referenceOrder[referenceStart:] {
		reference := builder.references[id]
		for _, problem := range reference.Problems {
			rootProblems[problem.ID] = problem
		}
		for _, problem := range builder.sources[reference.Source].Problems {
			rootProblems[problem.ID] = problem
		}
	}
	report.ProblemCount = len(rootProblems)
	if report.SourceCount == 0 {
		report.State = RootEmpty
	}
	for _, problem := range rootProblems {
		if problem.LimitsCoverage {
			report.State = RootIncomplete
			break
		}
	}
	return report
}

func (builder *discoveryBuilder) discoverCandidate(
	ctx context.Context,
	root DiscoveryRoot,
	discoveredPath string,
	relativePath string,
) bool {
	id := referenceID(root.ID, filepath.ToSlash(relativePath))
	reference := Reference{
		ID:             id,
		Root:           root.ID,
		Context:        root.Context,
		DiscoveredPath: discoveredPath,
		Status:         "unresolved",
	}

	resolvedPath, err := filepath.EvalSymlinks(discoveredPath)
	if err != nil {
		code := ProblemBrokenReference
		if isSymlinkCycle(err) {
			code = ProblemSymlinkCycle
		}
		problem := makeProblem(code, discoveredPath, "bundle reference could not be resolved", err, true)
		problem.RootIDs = []RootID{root.ID}
		problem.ReferenceIDs = []ReferenceID{id}
		reference.Problems = []Problem{problem}
		builder.addReference(reference)
		builder.addProblem(problem)
		return false
	}
	if err := ctx.Err(); err != nil {
		problem := makeProblem(
			ProblemBrokenReference,
			discoveredPath,
			"bundle discovery was interrupted",
			err,
			true,
		)
		problem.RootIDs = []RootID{root.ID}
		problem.ReferenceIDs = []ReferenceID{id}
		reference.Problems = []Problem{problem}
		builder.addReference(reference)
		builder.addProblem(problem)
		return false
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		problem := makeProblem(
			ProblemBrokenReference,
			discoveredPath,
			"bundle reference could not be inspected",
			err,
			true,
		)
		problem.RootIDs = []RootID{root.ID}
		problem.ReferenceIDs = []ReferenceID{id}
		reference.Problems = []Problem{problem}
		builder.addReference(reference)
		builder.addProblem(problem)
		return false
	}
	if !info.IsDir() {
		if isSymlink(discoveredPath) {
			err := fmt.Errorf("reference resolves to %s, not a directory", info.Mode().Type())
			problem := makeProblem(
				ProblemBrokenReference,
				discoveredPath,
				"bundle reference does not resolve to a directory",
				err,
				true,
			)
			problem.RootIDs = []RootID{root.ID}
			problem.ReferenceIDs = []ReferenceID{id}
			reference.Problems = []Problem{problem}
			builder.addReference(reference)
			builder.addProblem(problem)
		}
		return false
	}

	present, manifestErr := hasManifest(resolvedPath)
	if manifestErr != nil {
		problem := manifestPresenceProblem(resolvedPath, manifestErr)
		problem.RootIDs = []RootID{root.ID}
		problem.ReferenceIDs = []ReferenceID{id}
		reference.Problems = []Problem{problem}
		builder.addReference(reference)
		builder.addProblem(problem)
		return false
	}
	if !present {
		return false
	}

	reference.ResolvedPath = resolvedPath
	token, identityErr := directoryObjectToken(resolvedPath)
	var sourceIdentity SourceID
	identity := Identity{Method: "darwin-directory-object", Certainty: "proven"}
	if identityErr == nil {
		sourceIdentity = sourceID(token)
		if existing, ok := builder.identityIndex[token.String()]; ok {
			sourceIdentity = existing
		}
	} else {
		sourceIdentity = SourceID("src_" + stableID("skillglass-source-occurrence-v1", string(id)))
		identity = Identity{Method: "occurrence", Certainty: "uncertain"}
	}

	reference.Status = "resolved"
	reference.Source = sourceIdentity
	builder.addReference(reference)

	if existing, ok := builder.sources[sourceIdentity]; ok {
		existing.References = append(existing.References, id)
		builder.sources[sourceIdentity] = existing
		return true
	}

	manifest, manifestProblems := inspectManifest(resolvedPath, builder.request.Agent == "claude-code")
	resources, resourceProblems := inventoryResources(ctx, resolvedPath)
	problems := append(manifestProblems, resourceProblems...)
	if identityErr != nil {
		problems = append(problems, makeProblem(
			ProblemIdentityUncertain,
			resolvedPath,
			"bundle identity could not be proven",
			identityErr,
			true,
		))
	}
	for _, problem := range problems {
		builder.addProblem(problem)
	}

	source := Source{
		ID:           sourceIdentity,
		Identity:     identity,
		Directory:    resolvedPath,
		Name:         manifest.Name,
		Description:  manifest.Description,
		Manifest:     manifest,
		Resources:    resources,
		References:   []ReferenceID{id},
		Problems:     problems,
		Ownership:    EvidenceState{State: "unknown", Reason: "an explicit path does not prove ownership"},
		Availability: AvailabilityExplanation{State: "unknown", Reason: "no agent adapter supplied availability evidence"},
	}
	builder.sources[sourceIdentity] = source
	if identityErr == nil {
		builder.identityIndex[token.String()] = sourceIdentity
	}
	return true
}

func (builder *discoveryBuilder) addReference(reference Reference) {
	if _, exists := builder.references[reference.ID]; !exists {
		builder.referenceOrder = append(builder.referenceOrder, reference.ID)
	}
	builder.references[reference.ID] = reference
}

func (builder *discoveryBuilder) addProblem(problem Problem) {
	builder.problems = append(builder.problems, problem)
}

func (builder *discoveryBuilder) addRelationshipProblems() {
	byName := make(map[string][]SourceID)
	byManifest := make(map[string][]SourceID)
	for id, source := range builder.sources {
		if source.Name != "" {
			byName[source.Name] = append(byName[source.Name], id)
		}
		if source.Manifest.ResolvedPath != "" {
			byManifest[source.Manifest.ResolvedPath] = append(byManifest[source.Manifest.ResolvedPath], id)
		}
	}
	for name, ids := range byName {
		if len(ids) < 2 {
			continue
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
		selectors := make([]string, len(ids))
		for index, id := range ids {
			selectors[index] = string(id)
		}
		for _, id := range ids {
			source := builder.sources[id]
			problem := makeProblem(
				ProblemDuplicateName,
				source.Directory,
				fmt.Sprintf("display name %q is used by independent sources", name),
				errors.New("selectors: "+strings.Join(selectors, ", ")),
				false,
			)
			problem.SourceIDs = []SourceID{id}
			source.Problems = append(source.Problems, problem)
			builder.sources[id] = source
			builder.addProblem(problem)
		}
	}
	for target, ids := range byManifest {
		if len(ids) < 2 {
			continue
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
		selectors := make([]string, len(ids))
		for index, id := range ids {
			selectors[index] = string(id)
		}
		for _, id := range ids {
			source := builder.sources[id]
			problem := makeProblem(
				ProblemSharedManifest,
				source.Manifest.Path,
				"independent bundle shares a manifest target",
				fmt.Errorf("target %q is shared by selectors %s", target, strings.Join(selectors, ", ")),
				false,
			)
			problem.SourceIDs = []SourceID{id}
			source.Problems = append(source.Problems, problem)
			builder.sources[id] = source
			builder.addProblem(problem)
		}
	}
}

func (builder *discoveryBuilder) finalizeProblemRelationships() {
	order := make([]ProblemID, 0, len(builder.problems))
	merged := make(map[ProblemID]Problem, len(builder.problems))
	merge := func(problem Problem, roots []RootID, references []ReferenceID, sources []SourceID) {
		current, exists := merged[problem.ID]
		if !exists {
			current = problem
			order = append(order, problem.ID)
		}
		current.RootIDs = appendUniqueRootIDs(current.RootIDs, problem.RootIDs...)
		current.RootIDs = appendUniqueRootIDs(current.RootIDs, roots...)
		current.ReferenceIDs = appendUniqueReferenceIDs(current.ReferenceIDs, problem.ReferenceIDs...)
		current.ReferenceIDs = appendUniqueReferenceIDs(current.ReferenceIDs, references...)
		current.SourceIDs = appendUniqueSourceIDs(current.SourceIDs, problem.SourceIDs...)
		current.SourceIDs = appendUniqueSourceIDs(current.SourceIDs, sources...)
		merged[problem.ID] = current
	}
	for _, problem := range builder.problems {
		merge(problem, nil, nil, nil)
	}
	for id, reference := range builder.references {
		for _, problem := range reference.Problems {
			sources := []SourceID(nil)
			if reference.Source != "" {
				sources = []SourceID{reference.Source}
			}
			merge(problem, []RootID{reference.Root}, []ReferenceID{id}, sources)
		}
	}
	for id, source := range builder.sources {
		roots := make([]RootID, 0, len(source.References))
		for _, referenceID := range source.References {
			roots = append(roots, builder.references[referenceID].Root)
		}
		for _, problem := range source.Problems {
			merge(problem, roots, source.References, []SourceID{id})
		}
	}
	for id, reference := range builder.references {
		for index, problem := range reference.Problems {
			reference.Problems[index] = merged[problem.ID]
		}
		builder.references[id] = reference
	}
	for id, source := range builder.sources {
		for index, problem := range source.Problems {
			source.Problems[index] = merged[problem.ID]
		}
		builder.sources[id] = source
	}
	builder.problems = builder.problems[:0]
	for _, id := range order {
		problem := merged[id]
		sort.Slice(problem.RootIDs, func(left, right int) bool { return problem.RootIDs[left] < problem.RootIDs[right] })
		sort.Slice(problem.ReferenceIDs, func(left, right int) bool { return problem.ReferenceIDs[left] < problem.ReferenceIDs[right] })
		sort.Slice(problem.SourceIDs, func(left, right int) bool { return problem.SourceIDs[left] < problem.SourceIDs[right] })
		builder.problems = append(builder.problems, problem)
	}
	sort.Slice(builder.problems, func(left, right int) bool {
		if builder.problems[left].Code != builder.problems[right].Code {
			return builder.problems[left].Code < builder.problems[right].Code
		}
		if builder.problems[left].Path != builder.problems[right].Path {
			return builder.problems[left].Path < builder.problems[right].Path
		}
		return builder.problems[left].ID < builder.problems[right].ID
	})
}

func appendUniqueRootIDs(values []RootID, additions ...RootID) []RootID {
	for _, addition := range additions {
		if addition != "" && !containsRootID(values, addition) {
			values = append(values, addition)
		}
	}
	return values
}

func containsRootID(values []RootID, target RootID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendUniqueReferenceIDs(values []ReferenceID, additions ...ReferenceID) []ReferenceID {
	for _, addition := range additions {
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if addition != "" && !found {
			values = append(values, addition)
		}
	}
	return values
}

func appendUniqueSourceIDs(values []SourceID, additions ...SourceID) []SourceID {
	for _, addition := range additions {
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if addition != "" && !found {
			values = append(values, addition)
		}
	}
	return values
}

func manifestPresenceProblem(directory string, err error) Problem {
	return makeProblem(
		ProblemUnreadableManifest,
		filepath.Join(directory, "SKILL.md"),
		"manifest presence could not be determined",
		err,
		true,
	)
}

func hasManifest(directory string) (bool, error) {
	_, err := os.Lstat(filepath.Join(directory, "SKILL.md"))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func isSymlinkCycle(err error) bool {
	if errors.Is(err, syscall.ELOOP) {
		return true
	}
	if err == nil || errors.Unwrap(err) != nil {
		return false
	}
	return err.Error() == "EvalSymlinks: too many links" ||
		err.Error() == "EvalSymlinks: too many levels of symbolic links"
}

func inventoryResources(ctx context.Context, bundleDirectory string) ([]Resource, []Problem) {
	resources := make([]Resource, 0)
	problems := make([]Problem, 0)
	errResourceLimit := errors.New("resource inventory entry limit reached")
	err := filepath.WalkDir(bundleDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			problem := makeProblem(
				ProblemUnreadableResource,
				path,
				"resource could not be inspected",
				walkErr,
				true,
			)
			problems = append(problems, problem)
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == bundleDirectory {
			return nil
		}
		relative, relErr := filepath.Rel(bundleDirectory, path)
		if relErr != nil {
			problem := makeProblem(
				ProblemUnreadableResource,
				path,
				"resource path could not be represented",
				relErr,
				true,
			)
			problems = append(problems, problem)
			return nil
		}
		if filepath.Clean(relative) == "SKILL.md" {
			return nil
		}
		if len(resources) >= maxResourceEntries {
			return errResourceLimit
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			problem := makeProblem(
				ProblemUnreadableResource,
				path,
				"resource metadata could not be read",
				infoErr,
				true,
			)
			problems = append(problems, problem)
			return nil
		}
		resource := Resource{
			Path: filepath.ToSlash(relative),
			Mode: fmt.Sprintf("%#o", info.Mode().Perm()),
			Size: info.Size(),
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			resource.Kind = "symlink"
			resource.Size = 0
			resource.LinkTarget, infoErr = os.Readlink(path)
			if infoErr != nil {
				problem := makeProblem(
					ProblemUnreadableResource,
					path,
					"resource link target could not be read",
					infoErr,
					true,
				)
				problems = append(problems, problem)
			} else {
				resource.LinkScope = linkScope(bundleDirectory, path, resource.LinkTarget)
			}
		case info.IsDir():
			resource.Kind = "directory"
			resource.Size = 0
		case info.Mode().IsRegular():
			resource.Kind = "file"
			resource.Executable = info.Mode().Perm()&0o111 != 0
		case info.Mode()&os.ModeNamedPipe != 0:
			resource.Kind = "named-pipe"
			resource.Size = 0
		case info.Mode()&os.ModeSocket != 0:
			resource.Kind = "socket"
			resource.Size = 0
		case info.Mode()&os.ModeDevice != 0:
			resource.Kind = "device"
			resource.Size = 0
		default:
			resource.Kind = "other"
			resource.Size = 0
		}
		resources = append(resources, resource)
		return nil
	})
	if errors.Is(err, errResourceLimit) {
		problems = append(problems, makeProblem(
			ProblemResourceLimit,
			bundleDirectory,
			"resource inventory reached its entry limit",
			fmt.Errorf("limit is %d entries", maxResourceEntries),
			true,
		))
	} else if err != nil {
		problems = append(problems, makeProblem(
			ProblemUnreadableResource,
			bundleDirectory,
			"resource inventory did not complete",
			err,
			true,
		))
	}
	sort.Slice(resources, func(left, right int) bool {
		return resources[left].Path < resources[right].Path
	})
	return resources, problems
}

func linkScope(bundleDirectory, linkPath, rawTarget string) string {
	target := rawTarget
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	relative, err := filepath.Rel(bundleDirectory, filepath.Clean(target))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "external"
	}
	return "internal"
}
