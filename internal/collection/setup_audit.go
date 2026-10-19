package collection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SetupLocation is read-only evidence for an exact prospective companion bundle.
// All native adapters participate, including applications not selected for setup.
type SetupConsumer struct {
	Application      string `json:"application"`
	State            string `json:"state"`
	ValidatedVersion string `json:"validated_version"`
	Reason           string `json:"reason"`
}

type SetupLocation struct {
	ConsumerEvidence []SetupConsumer `json:"consumer_evidence"`
	Consumers        []string        `json:"consumers"`
	Roots            []RootReport    `json:"roots"`
	Refusals         []string        `json:"refusals"`
	Warnings         []string        `json:"warnings"`
}

func AuditSetupLocation(ctx context.Context, destination string) SetupLocation {
	out := SetupLocation{ConsumerEvidence: []SetupConsumer{}, Consumers: []string{}, Roots: []RootReport{}, Refusals: []string{}, Warnings: []string{}}
	for _, app := range applicationCatalog {
		req, err := NewRequestWithAgent(nil, nil, nil, app.Application)
		var s *Snapshot
		if err == nil {
			s, err = Build(ctx, req)
		}
		if err != nil {
			out.Refusals = append(out.Refusals, app.Application+": cannot establish native discovery: "+err.Error())
			continue
		}
		scope := s.Scope()
		consumer := false
		for _, root := range scope.Roots {
			if setupRootParticipates(root) {
				aliases, e := setupAliasEvidence(ctx, root.Requested, destination, &out.Warnings)
				if e != nil {
					out.Refusals = append(out.Refusals, app.Application+": prospective alias isolation is unverified: "+e.Error())
				}
				if len(aliases) > 0 {
					consumer = true
					out.Roots = append(out.Roots, aliases...)
				}
			}
			path := root.Requested
			if root.Resolved != "" {
				path = root.Resolved
			}
			relation := setupPathsOverlap(destination, path)
			if relation == deleteUnresolved {
				out.Refusals = append(out.Refusals, app.Application+": unresolved root relationship: "+path)
				continue
			}
			if relation != deleteRelated {
				continue
			}
			out.Roots = append(out.Roots, root)
			if root.Context == ContextManaged || root.Context == ContextSystem {
				out.Refusals = append(out.Refusals, app.Application+": managed/system root: "+path)
			}
			if setupRootParticipates(root) {
				consumer = true
				if app.Application == "pi" && !setupPiProspectiveIgnoreKnown(ctx, s, root, destination) {
					out.Refusals = append(out.Refusals, "pi: prospective companion ignore inputs are unverified: "+root.Requested)
				}
			}
		}
		// A child symlink can expose this destination even when its collection root
		// has no ancestor relationship. Inspect resolved native references as well.
		view := s.Query(Filters{Context: FilterAll}, QuerySources)
		for _, ref := range view.References {
			if setupReferenceParticipates(scope.Roots, ref.Root) && ref.ResolvedPath != "" && setupPathsOverlap(destination, filepath.Dir(ref.ResolvedPath)) == deleteRelated {
				consumer = true
			}
		}
		for _, problem := range view.Problems {
			if !problem.LimitsCoverage {
				continue
			}
			message := fmt.Sprintf("%s: incomplete discovery: %s: %s", app.Application, problem.Path, problem.Summary)
			unrelated := setupPathsOverlap(destination, problem.Path) == deleteUnrelated
			if (unrelated && problem.Code != ProblemNativeInput) || (app.Application == "pi" && setupPiIgnoreUnrelated(ctx, s, problem, destination)) {
				out.Warnings = append(out.Warnings, message)
			} else {
				out.Refusals = append(out.Refusals, message)
			}
		}
		managers := append([]ManagerEvidence(nil), s.managers...)
		required := append(requiredManagerReaders(req), s.managerReaders...)
		managers = append(managers, missingManagerReaders(required, managers)...)
		for _, m := range managers {
			relevant := m.Boundary == "" || setupPathsOverlap(destination, m.Boundary) != deleteUnrelated
			if m.State == ManagerUncertain && len(m.UncertainBoundaries) > 0 {
				relevant = false
				for _, b := range m.UncertainBoundaries {
					if setupPathsOverlap(destination, b) != deleteUnrelated {
						relevant = true
					}
				}
			}
			if relevant && (m.State == ManagerBoundary || m.State == ManagerUncertain) {
				out.Refusals = append(out.Refusals, app.Application+": managed or uncertain ownership: "+m.Boundary+": "+m.Evidence.Reason)
			} else if m.State == ManagerUncertain {
				out.Warnings = append(out.Warnings, app.Application+": unrelated ownership uncertainty: "+m.Boundary+": "+m.Evidence.Reason)
			}
		}
		if consumer {
			out.Consumers = append(out.Consumers, app.Application)
			e := SetupConsumer{Application: app.Application, State: "known-loader-rules", Reason: "Known user-level loader roots are conservatively treated as possible discovery; running sessions and actual use are not observed."}
			if scope.Agent != nil {
				e.ValidatedVersion = scope.Agent.ValidatedVersion
			}
			switch app.Application {
			case "codex":
				e.State = "version-specific-native-baseline"
				e.ValidatedVersion = "Codex 0.162.1"
				e.Reason = "Fresh disposable native skills/list observed this companion at the default user root; this environment's routes are inferred from loader rules; running-session adoption is separate."
			case "claude-code":
				e.State = "version-specific-native-baseline"
				e.ValidatedVersion = "Claude Code 2.1.303"
				e.Reason = "Fresh disposable initialization catalog observed this companion at the default personal skills root; this environment's routes are inferred from loader rules; running-session adoption is separate."
			case "opencode":
				e.State = "version-specific-native-baseline"
				e.ValidatedVersion = "OpenCode 1.17.15"
				e.Reason = "Fresh disposable debug skill catalog observed this companion in default native and shared Claude roots; this environment's routes are inferred from loader rules; running-session adoption is separate."
			case "codeaf":
				e.State = "known-loader-native-unverified"
				e.Reason = "Recorded CodeAF source imports this root, but installed dirty-build discovery is unverified. Consent acknowledges possible indirect exposure and does not certify CodeAF setup support."
			}
			out.ConsumerEvidence = append(out.ConsumerEvidence, e)
		}
	}
	sort.Strings(out.Consumers)
	sort.Strings(out.Refusals)
	sort.Strings(out.Warnings)
	return out
}

// Empty containers and broken directory links can become discovery routes only
// after setup creates the new bundle. Existing-source references cannot prove
// their absence. Refuse related aliases instead of claiming native isolation.
func setupAliasEvidence(ctx context.Context, root, destination string, warnings *[]string) ([]RootReport, error) {
	result := []RootReport{}
	count := 0
	ancestors := map[string]bool{}
	var walk func(string) error
	walk = func(path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > maxResourceEntries {
			return fmt.Errorf("alias scan exceeds directory entry limit: %s", root)
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(path)
			if e != nil {
				return e
			}
			if !filepath.IsAbs(target) {
				target = filepath.Dir(path) + string(filepath.Separator) + target
			}
			if setupPathsOverlap(destination, target) != deleteUnrelated {
				result = append(result, RootReport{Requested: path, Resolved: target, Origin: "setup-prospective-alias", State: RootIncomplete})
				return fmt.Errorf("related or unresolved alias %s targets %s; unsupported isolation", path, target)
			}
			info, err = os.Stat(path)
			if errors.Is(err, os.ErrNotExist) {
				*warnings = append(*warnings, "Unrelated broken alias preserved: "+path+" -> "+target)
				return nil
			}
			if err != nil {
				return err
			}
		}
		if !info.IsDir() {
			return nil
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		if ancestors[canonical] {
			return fmt.Errorf("alias scan encountered a directory cycle: %s", path)
		}
		ancestors[canonical] = true
		defer delete(ancestors, canonical)
		directory, err := os.Open(path)
		if err != nil {
			return err
		}
		entries, err := directory.ReadDir(maxResourceEntries - count + 1)
		directory.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		count += len(entries)
		if count > maxResourceEntries {
			return fmt.Errorf("alias scan exceeds directory entry limit: %s", root)
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				if err := walk(filepath.Join(path, entry.Name())); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err := walk(root)
	return result, err
}

// Inactive package storage and explicitly disabled external imports are inspected
// for ownership, but are not loader routes. Active registered package roots are
// separately reported by their adapter and still undergo the complete alias scan.
func setupRootParticipates(root RootReport) bool {
	return root.Origin != "opencode-external-disabled" && root.Origin != "pi-inactive-cache"
}
func setupReferenceParticipates(roots []RootReport, id RootID) bool {
	for _, root := range roots {
		if root.ID == id {
			return setupRootParticipates(root)
		}
	}
	return true // Missing provenance cannot establish non-participation.
}

// Resolve every existing symlink component before comparing prospective paths.
// A missing tail is safe to retain only after its existing ancestors have been
// resolved. Stripping a dangling link as though it were an ordinary missing
// component would lose its target and could conceal a future discovery route.
func setupProspectivePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("prospective path is not absolute: %s", path)
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
		candidate := filepath.Join(resolved, part)
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return filepath.Join(append([]string{candidate}, pending...)...), nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 40 {
				return "", fmt.Errorf("prospective path has cyclic or excessive aliases: %s", path)
			}
			target, err := os.Readlink(candidate)
			if err != nil {
				return "", err
			}
			if filepath.IsAbs(target) {
				resolved = string(filepath.Separator)
			}
			pending = append(strings.Split(target, string(filepath.Separator)), pending...)
			continue
		}
		if len(pending) > 0 && !info.IsDir() {
			return "", fmt.Errorf("prospective path ancestor is not a directory: %s", candidate)
		}
		resolved = candidate
	}
	return resolved, nil
}
func setupPathsOverlap(a, b string) deleteRelation {
	left, le := setupProspectivePath(a)
	right, re := setupProspectivePath(b)
	if le != nil || re != nil {
		return deleteUnresolved
	}
	return deletePathsOverlap(left, right)
}
