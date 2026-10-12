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

	completeness := Complete
	for _, problem := range builder.problems {
		if problem.LimitsCoverage {
			completeness = Incomplete
			break
		}
	}
	return &Snapshot{
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

func (builder *discoveryBuilder) discoverRoot(ctx context.Context, root DiscoveryRoot) RootReport {
	report := RootReport{
		ID:        root.ID,
		Requested: root.Path,
		State:     RootSearched,
	}
	problemStart := len(builder.problems)

	resolvedRoot, err := filepath.EvalSymlinks(root.Path)
	if err != nil {
		code := ProblemRootUnreadable
		state := RootUnreadable
		if errors.Is(err, os.ErrNotExist) {
			code = ProblemRootMissing
			state = RootMissing
		}
		problem := makeProblem(code, root.Path, "selected root could not be opened", err, true)
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
		builder.addProblem(problem)
		report.State = RootUnreadable
		report.ProblemCount = 1
		return report
	}

	rootIsBundle, err := hasManifest(resolvedRoot)
	if err != nil {
		rootIsBundle = false
	}
	if rootIsBundle {
		if builder.discoverCandidate(ctx, root, root.Path, ".") {
			report.SourceCount++
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
			builder.addProblem(problem)
			report.State = RootUnreadable
			report.ProblemCount = 1
			return report
		}
		sort.Slice(entries, func(left, right int) bool {
			return entries[left].Name() < entries[right].Name()
		})
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				problem := makeProblem(
					ProblemRootUnreadable,
					root.Path,
					"discovery was interrupted",
					err,
					true,
				)
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

	report.ProblemCount = len(builder.problems) - problemStart
	rootProblems := builder.problems[problemStart:]
	if problemsLimitCoverage(rootProblems) {
		report.State = RootIncomplete
	} else if report.SourceCount == 0 {
		report.State = RootEmpty
	}
	return report
}

func problemsLimitCoverage(problems []Problem) bool {
	for _, problem := range problems {
		if problem.LimitsCoverage {
			return true
		}
	}
	return false
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
			reference.Problems = []Problem{problem}
			builder.addReference(reference)
			builder.addProblem(problem)
		}
		return false
	}

	present, manifestErr := hasManifest(resolvedPath)
	if manifestErr != nil {
		problem := makeProblem(
			ProblemUnreadableManifest,
			filepath.Join(resolvedPath, "SKILL.md"),
			"manifest presence could not be determined",
			manifestErr,
			true,
		)
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

	manifest, manifestProblems := inspectManifest(resolvedPath)
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
		Availability: EvidenceState{State: "unknown", Reason: "no agent adapter supplied availability evidence"},
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
	return errors.Is(err, syscall.ELOOP) ||
		strings.Contains(strings.ToLower(err.Error()), "too many levels of symbolic links")
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
