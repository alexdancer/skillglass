package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
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
	State        string           `json:"state"`
	Reason       string           `json:"reason"`
	Target       DeleteTarget     `json:"target"`
	Ownership    DeleteOwnership  `json:"ownership"`
	Project      ProjectContext   `json:"project"`
	References   []Reference      `json:"references"`
	Consumers    []DeleteConsumer `json:"consumers"`
	Contents     []DeleteEntry    `json:"contents"`
	Limits       []string         `json:"limits"`
	Confirmation string           `json:"confirmation,omitempty"`
	Phrase       string           `json:"confirmation_phrase,omitempty"`
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
	raw, _ := json.Marshal(p.view)
	var result DeleteView
	_ = json.Unmarshal(raw, &result)
	return result
}
func (s *Snapshot) PreviewDelete(ctx context.Context, id SourceID) *DeletePreview {
	p := &DeletePreview{view: DeleteView{State: "refused", Reason: "selected source cannot be deleted", Limits: []string{
		"Permanent deletion. No trash, recovery or undo. Remaining references and settings are not repaired. Running sessions may retain loaded content.",
		"Discovery covers the seven listed native integrations and selected project only; other projects and machine-wide consumers are unverified.",
		"Local filesystem control and no known manager ownership within bounded discovery do not prove intellectual-property ownership.",
		"Exact no-follow inventory limits: 10000 entries, depth 128, 256 MiB per regular file, 1 GiB total hashed bytes. Exceeding a limit refuses deletion.",
		"Links inside a bundle are unlinked without following targets. Internal hardlink aliases outside the bundle are not enumerated; deletion does not erase physical bytes.",
		"Changes observed before unlink stop removal. Darwin name-based unlink is not atomic against arbitrary concurrent same-UID writers; external symlink targets are never traversed.",
	}}}
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
	var observations []View
	var inputs []changeFact
	reason := ""
	protect := []string{}
	managed := []string{}
	if source.Ownership.State == "managed" {
		reason = "selected content is managed: " + source.Ownership.Reason
	}
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
			protect = append(protect, deleteApplicationHomes(req)...)
			if exact, found := native.Source(id); found {
				c.References = native.References(id)
				a := cloneAvailability(exact.Availability)
				c.Availability = &a
				c.Reason = fmt.Sprintf("%d proven references to this exact source", len(c.References))
				if exact.Ownership.State == "managed" {
					reason = "managed native content: " + c.Application + ": " + exact.Ownership.Reason
				}
			} else {
				c.Reason = "zero proven references to this exact source"
			}
			for _, r := range c.Scope.Roots {
				path := r.Resolved
				if path == "" {
					path = r.Requested
				}
				if (r.Origin != "" || path != source.Directory) && !(kind == "standalone" && path == source.Directory) {
					protect = append(protect, path)
				}
				if r.Context == ContextManaged || r.Context == ContextSystem {
					managed = append(managed, path)
				}
				if (r.State == RootUnreadable || r.State == RootIncomplete) && deletePathsOverlap(source.Directory, path) {
					reason = "relevant native discovery scope is unverified: " + r.Requested
				}
			}
			for _, other := range v.Sources {
				if other.ID != id && deleteContains(source.Directory, other.Directory) {
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
			for _, problem := range v.Problems {
				text := strings.ToLower(problem.Path + " " + problem.Summary + " " + problem.Detail)
				if problem.LimitsCoverage && (strings.Contains(text, "plugin") || strings.Contains(text, "package") || strings.Contains(text, "registry") || strings.Contains(text, "managed")) {
					reason = "manager evidence is ambiguous: " + problem.Path + ": " + problem.Summary
				}
			}
		}
		p.view.Consumers = append(p.view.Consumers, c)
	}
	protect = append(protect, p.request.Project.Path)
	for _, path := range protect {
		if path != "" && deleteContains(source.Directory, deleteCanonical(path)) {
			reason = "unsafe boundary encompasses a protected home, collection or project: " + path
		}
	}
	for _, path := range managed {
		if path != "" && deletePathsOverlap(source.Directory, deleteCanonical(path)) {
			reason = "managed or system boundary: " + path
		}
	}
	entries, ancestors, ownership, err := inventoryDelete(ctx, p.view.Target)
	p.view.Ownership = ownership
	p.view.Contents = entries
	p.ancestors = ancestors
	if err != nil {
		reason = err.Error()
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
	}{"delete-v1", p.view, deleteStamps(entries), ancestors, observations, inputs})
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
func deletePathsOverlap(a, b string) bool {
	return deleteContains(a, deleteCanonical(b)) || deleteContains(deleteCanonical(b), a)
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
