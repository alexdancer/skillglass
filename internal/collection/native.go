package collection

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func nativeProjectAncestors(project ProjectContext) ([]string, string, error) {
	if !project.Selected {
		return nil, "", nil
	}
	selected, err := filepath.EvalSymlinks(project.Path)
	if err != nil {
		return nil, "", err
	}
	info, err := os.Stat(selected)
	if err != nil || !info.IsDir() {
		return nil, "", errors.New("selected project is not a directory")
	}
	var ancestors []string
	for path := selected; ; path = filepath.Dir(path) {
		ancestors = append(ancestors, path)
		if actualGitBoundary(path) {
			return ancestors, path, nil
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	return []string{selected}, "", nil
}
func actualGitBoundary(path string) bool {
	marker := filepath.Join(path, ".git")
	info, err := os.Stat(marker)
	if err != nil {
		return false
	}
	gitdir := marker
	if !info.IsDir() {
		raw, err := nativeBytes(marker)
		if err != nil {
			return false
		}
		text := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(text, "gitdir: ") {
			return false
		}
		gitdir = strings.TrimPrefix(text, "gitdir: ")
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(path, gitdir)
		}
	}
	head, err := nativeBytes(filepath.Join(gitdir, "HEAD"))
	if err != nil || len(head) == 0 {
		return false
	}
	if objects, err := os.Stat(filepath.Join(gitdir, "objects")); err == nil && objects.IsDir() {
		return true
	}
	common, err := nativeBytes(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return false
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitdir, commonDir)
	}
	objects, err := os.Stat(filepath.Join(commonDir, "objects"))
	return err == nil && objects.IsDir()
}
func nativeHome() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", errors.New("native agent scope requires HOME to bound native discovery")
	}
	return filepath.Abs(home)
}

// Only bounded regular files are opened; FIFOs, devices and oversized inputs cannot block browsing.
func nativeBytes(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("native input is not a regular file")
	}
	if info.Size() > maxManifestBytes {
		return nil, errors.New("native input exceeds read limit")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return nil, errors.New("native input changed file kind during read")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxManifestBytes {
		return nil, errors.New("native input exceeds read limit")
	}
	return raw, nil
}
func nativeAbsent(path string, err error) bool {
	if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	_, linkErr := os.Lstat(path)
	return errors.Is(linkErr, os.ErrNotExist)
}
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
				if !containsNativeSource(problem.SourceIDs, reference.Source) {
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

func containsNativeSource(values []SourceID, id SourceID) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}
