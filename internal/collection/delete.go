package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type DeleteTarget struct {
	Source   SourceID `json:"source_id"`
	Path     string   `json:"canonical_path"`
	Kind     string   `json:"kind"`
	Identity Identity `json:"identity"`
}
type DeleteOwnership struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	UID    int    `json:"effective_uid"`
}
type DeleteConsumer struct {
	Application  string                   `json:"application"`
	Scope        Scope                    `json:"scope"`
	References   []Reference              `json:"references"`
	Dependencies []DeleteDependency       `json:"manifest_dependencies,omitempty"`
	Managers     []ManagerEvidence        `json:"manager_evidence"`
	Problems     []Problem                `json:"discovery_problems,omitempty"`
	Availability *AvailabilityExplanation `json:"availability,omitempty"`
	Reason       string                   `json:"reason"`
}
type DeleteEntry struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	Links      uint16 `json:"link_count"`
	LinkTarget string `json:"link_target,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	stamp      deleteStamp
}
type DeleteView struct {
	State        string             `json:"state"`
	Reason       string             `json:"reason"`
	Target       DeleteTarget       `json:"target"`
	Ownership    DeleteOwnership    `json:"ownership"`
	Project      ProjectContext     `json:"project"`
	References   []Reference        `json:"references"`
	Dependencies []DeleteDependency `json:"manifest_dependencies,omitempty"`
	Consumers    []DeleteConsumer   `json:"consumers"`
	Contents     []DeleteEntry      `json:"contents"`
	Limits       []string           `json:"limits"`
	Confirmation string             `json:"confirmation,omitempty"`
	Phrase       string             `json:"confirmation_phrase,omitempty"`
}
type DeleteOutcome struct {
	View         DeleteView `json:"context"`
	State        string     `json:"state"`
	Reason       string     `json:"reason"`
	Removed      []string   `json:"removed"`
	Remaining    []string   `json:"remaining"`
	Unverified   []string   `json:"unverified"`
	Replacements []string   `json:"replacements"`
	Verified     bool       `json:"verified"`
}

func (out DeleteOutcome) ExitCode() int {
	if out.State == "removed" && out.Verified {
		return 0
	}
	return 1
}

type DeletePreview struct {
	view      DeleteView
	request   Request
	seal      string
	ancestors []deleteStamp
	once      sync.Mutex
	used      bool
	// Checkpoints exercise syscall races that cannot be induced reliably through the executable.
	checkpoint func(string, string) error
}

func (p *DeletePreview) View() DeleteView {
	if p == nil {
		return DeleteView{State: "refused", Reason: "no deletion preview"}
	}
	view := p.view
	view.References = make([]Reference, len(p.view.References))
	for i, ref := range p.view.References {
		view.References[i] = cloneReference(ref)
	}
	view.Dependencies = cloneDeleteDependencies(p.view.Dependencies)
	view.Contents = append([]DeleteEntry(nil), p.view.Contents...)
	view.Limits = append([]string(nil), p.view.Limits...)
	view.Consumers = append([]DeleteConsumer(nil), p.view.Consumers...)
	for i := range view.Consumers {
		consumer := &view.Consumers[i]
		consumer.Scope = cloneScope(consumer.Scope)
		consumer.Managers = append([]ManagerEvidence(nil), consumer.Managers...)
		for j := range consumer.Managers {
			consumer.Managers[j].UncertainBoundaries = append([]string(nil), consumer.Managers[j].UncertainBoundaries...)
		}
		consumer.Problems = cloneProblems(consumer.Problems)
		consumer.Dependencies = cloneDeleteDependencies(consumer.Dependencies)
		consumer.References = append([]Reference(nil), consumer.References...)
		for j, ref := range consumer.References {
			consumer.References[j] = cloneReference(ref)
		}
		if consumer.Availability != nil {
			value := cloneAvailability(*consumer.Availability)
			consumer.Availability = &value
		}
	}
	return view
}
func (s *Snapshot) PreviewDelete(ctx context.Context, id SourceID) *DeletePreview {
	p := &DeletePreview{view: DeleteView{State: "refused", Reason: "selected source cannot be deleted", Limits: []string{
		"Permanent deletion. No trash, recovery or undo. Remaining references and settings are not repaired. Running sessions may retain loaded content.",
		"Discovery covers the seven listed native integrations and selected project only; other projects and machine-wide consumers are unverified.",
		"Exact no-follow inventory limits: 10000 entries, depth 128, 256 MiB per regular file, 1 GiB total hashed bytes. Exceeding a limit refuses deletion.",
		"Links inside a bundle are unlinked without following targets. Internal hardlink aliases outside the bundle are not enumerated; deletion does not erase physical bytes.",
		"Changes observed before unlink stop removal. Darwin name-based unlink is not atomic against arbitrary concurrent same-UID writers; external symlink targets are never traversed.",
	}}}
	p.view.Target.Source = id
	if s == nil {
		return p
	}
	p.request = cloneRequest(s.request)
	p.view.Project = s.request.Project
	source, ok := s.Source(id)
	if !ok || source.Identity.Certainty != "proven" {
		p.view.Reason = "select a reachable proven exact SourceID"
		return p
	}
	kind := "directory"
	if source.Identity.Method == "darwin-file-object" {
		kind = "standalone"
	}
	p.view.Target = DeleteTarget{id, source.Directory, kind, source.Identity}
	p.view.References = s.References(id)
	p.view.Phrase = "delete " + string(id)
	entries, ancestors, ownership, inventoryErr := inventoryDelete(ctx, p.view.Target)
	p.view.Ownership = ownership
	p.view.Contents = entries
	p.ancestors = ancestors
	p.view.Dependencies = deleteDependencies(s, p.view.Target, false)
	var observations []View
	var inputs []changeFact
	reason := ""
	if inventoryErr != nil {
		reason = inventoryErr.Error()
	}
	protect := []string{}
	for _, capability := range applicationCatalog {
		c := DeleteConsumer{Application: capability.Application, References: []Reference{}}
		req, err := groupRequest(p.request, capability.Application)
		var native *Snapshot
		if err == nil {
			native, err = Build(ctx, req)
		}
		if err != nil {
			c.Scope = Scope{Completeness: Incomplete}
			c.Reason = "native discovery unavailable: " + err.Error()
			reason = "native ownership evidence cannot be established: " + c.Application
		} else {
			c.Scope = native.Scope()
			v := native.Query(Filters{Context: FilterAll}, QuerySources)
			observations = append(observations, v)
			c.Managers = append([]ManagerEvidence(nil), native.managers...)
			required := append(requiredManagerReaders(req), native.managerReaders...)
			c.Managers = append(c.Managers, missingManagerReaders(required, c.Managers)...)
			c.Problems = cloneProblems(v.Problems)
			c.Dependencies = deleteDependencies(native, p.view.Target, true)
			protect = append(protect, deleteApplicationHomes(req)...)
			if exact, found := native.Source(id); found {
				a, refs := deleteNativeReferences(native, exact)
				c.References = refs
				c.Availability = &a
			}
			c.Reason = fmt.Sprintf("%d proven native references to this exact source; %d dependent manifests in independent sources; availability remains application-specific", len(c.References), len(c.Dependencies))
			for _, r := range c.Scope.Roots {
				path := r.Resolved
				if path == "" {
					path = r.Requested
				}
				if (r.Origin != "" || !deleteBoundaryEqual(path, source.Directory)) && !(kind == "standalone" && deleteBoundaryEqual(path, source.Directory)) {
					protect = append(protect, path)
				}
				if r.Context == ContextSystem {
					e := Evidence{Kind: EvidenceObserved, Rule: "system-storage", InputPath: path, Reason: "native system content is not user-owned"}
					c.Managers = append(c.Managers, ManagerEvidence{Manager: "system", Boundary: path, State: ManagerBoundary, Evidence: e})
				}
			}
			sort.Slice(c.Managers, func(i, j int) bool {
				a, _ := json.Marshal(c.Managers[i])
				b, _ := json.Marshal(c.Managers[j])
				return string(a) < string(b)
			})
			for _, manager := range c.Managers {
				relevant := manager.Boundary == "" || deletePathsOverlap(source.Directory, manager.Boundary)
				if manager.State == ManagerUncertain && len(manager.UncertainBoundaries) > 0 {
					relevant = false
					for _, boundary := range manager.UncertainBoundaries {
						if deletePathsOverlap(source.Directory, boundary) {
							relevant = true
							break
						}
					}
				}
				if relevant && manager.State == ManagerUncertain {
					reason = "manager ownership evidence is uncertain: " + c.Application + ": " + manager.Evidence.InputPath + ": " + manager.Evidence.Reason
					p.view.Ownership.State, p.view.Ownership.Reason = "uncertain", reason
				}
				if relevant && manager.State == ManagerBoundary {
					reason = "managed or system boundary: " + manager.Boundary
					p.view.Ownership.State, p.view.Ownership.Reason = "managed", reason
				}
				if (manager.State != ManagerUncertain || manager.SHA256 != "") && manager.Evidence.InputPath != "" {
					fact, err := observeChangePath(manager.Evidence.InputPath)
					if err == nil {
						inputs = append(inputs, fact)
						observed := fact
						if fact.Resolved != "" && fact.Resolved != fact.Path {
							resolved, e := observeChangePath(fact.Resolved)
							if e == nil {
								inputs = append(inputs, resolved)
								observed = resolved
							} else if relevant {
								reason = "manager input cannot be sealed: " + fact.Resolved
							}
						}
						if relevant && ((manager.State == ManagerAbsent && fact.Exists) || (manager.SHA256 != "" && managerInputDigest(observed.Bytes) != manager.SHA256)) {
							reason = "manager input changed during discovery: " + manager.Evidence.InputPath
						}
					} else if relevant {
						reason = "manager input cannot be sealed: " + manager.Evidence.InputPath
					}
				}
			}
			for _, problem := range v.Problems {
				if problem.LimitsCoverage && problem.Code != ProblemNativeInput && filepath.IsAbs(problem.Path) && deletePathsOverlap(source.Directory, problem.Path) {
					reason = "relevant discovery evidence is unverified: " + problem.Path + ": " + problem.Summary
				}
			}
			for _, other := range v.Sources {
				if other.ID != id && deleteBoundaryContains(source.Directory, other.Directory) {
					reason = "selected boundary encloses another independent source: " + other.Directory
				}
			}
			if c.Scope.Agent != nil {
				for _, layer := range c.Scope.Agent.ConfigLayers {
					if layer.Path == "" {
						continue
					}
					fact, e := observeChangePath(layer.Path)
					if e != nil {
						reason = "native input cannot be sealed: " + layer.Path
					} else {
						inputs = append(inputs, fact)
					}
				}
			}
		}
		p.view.Consumers = append(p.view.Consumers, c)
	}
	protect = append(protect, p.request.Project.Path)
	for _, path := range protect {
		if path != "" && deleteBoundaryContains(source.Directory, path) {
			reason = "unsafe boundary encompasses a protected home, collection or project: " + path
		}
	}
	if reason != "" {
		p.view.Reason = reason
		return p
	}
	p.view.State = "ready"
	p.view.Reason = "preview only; no content or settings changed"
	raw, err := json.Marshal(struct {
		Version   string
		View      DeleteView
		Trees     []deleteStamp
		Ancestors []deleteStamp
		Discovery []View
		Inputs    []changeFact
	}{"delete-v2", p.view, deleteStamps(entries), ancestors, observations, inputs})
	if err != nil {
		p.view.State = "refused"
		p.view.Reason = "evidence cannot be sealed: " + err.Error()
		return p
	}
	sum := sha256.Sum256(raw)
	p.seal = "delete:" + string(id) + ":" + hex.EncodeToString(sum[:])
	p.view.Confirmation = p.seal
	return p
}
func deleteStamps(entries []DeleteEntry) []deleteStamp {
	out := make([]deleteStamp, len(entries))
	for i, e := range entries {
		out[i] = e.stamp
	}
	return out
}
func deleteCanonical(path string) string {
	path = filepath.Clean(path)
	suffix := ""
	for {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Join(resolved, suffix)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(path, suffix)
		}
		suffix = filepath.Join(filepath.Base(path), suffix)
		path = parent
	}
}
func deleteContains(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Object ancestry also covers case aliases on case-insensitive Darwin filesystems.
// Uninspectable unrelated scope remains disclosed by native discovery; lexical
// overlap still refuses when the inaccessible path could enclose this target.
func deleteBoundaryContains(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	parentLocation, parentErr := groupLocationAt(parent)
	childLocation, childErr := groupLocationAt(child)
	if parentErr == nil && childErr == nil {
		return childLocation.within(parentLocation)
	}
	return deleteContains(deleteCanonical(parent), deleteCanonical(child))
}
func deleteBoundaryEqual(a, b string) bool {
	return deleteBoundaryContains(a, b) && deleteBoundaryContains(b, a)
}
func deletePathsOverlap(a, b string) bool {
	return deleteBoundaryContains(a, b) || deleteBoundaryContains(b, a)
}
func deleteApplicationHomes(r Request) []string {
	paths := []string{r.home, r.codexHome, r.claudeHome, r.hermesHome, r.codeafProfile}
	if r.home != "" {
		for _, dir := range []string{".agents", ".codex", ".claude", ".hermes", ".pi", ".config/opencode", ".gemini", ".codeaf", ".aforge"} {
			paths = append(paths, filepath.Join(r.home, dir))
		}
	}
	return paths
}
func (p *DeletePreview) Apply(ctx context.Context, confirmation string) DeleteOutcome {
	out := DeleteOutcome{State: "refused", Reason: "confirmation does not match the current exact target; request a new preview", Removed: []string{}, Remaining: []string{}, Unverified: []string{}, Replacements: []string{}}
	if p == nil {
		return out
	}
	p.once.Lock()
	defer p.once.Unlock()
	out.View = p.View()
	out.View.Confirmation = ""
	if p.used {
		out.Reason = "preview already used; request a new preview"
		return out
	}
	p.used = true
	if p.seal == "" || confirmation == "" || confirmation != p.seal {
		prefix := "delete:" + string(p.view.Target.Source) + ":"
		if strings.HasPrefix(confirmation, prefix) {
			digest, err := hex.DecodeString(strings.TrimPrefix(confirmation, prefix))
			if err == nil && len(digest) == sha256.Size {
				out.State = "stale"
				out.Reason = "confirmation is stale or does not match the current exact target facts; request a new preview"
			}
		}
		return out
	}
	if err := ctx.Err(); err != nil {
		out.State = "cancelled"
		out.Reason = err.Error()
		return out
	}
	fresh, err := Build(ctx, cloneRequest(p.request))
	if err != nil {
		out.State = "stale"
		out.Reason = "reinspection failed: " + err.Error()
		return out
	}
	check := fresh.PreviewDelete(ctx, p.view.Target.Source)
	if check.seal != p.seal {
		out.State = "stale"
		out.Reason = "preview is stale; target, contents, ownership, native inputs, consumers or coverage changed; request a new preview"
		return out
	}
	return p.remove(ctx, out)
}
