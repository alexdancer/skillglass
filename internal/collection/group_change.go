package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// ApplicationCapability describes a completed adapter, not permission to write a
// particular source. The native preview remains authoritative for eligibility.
type ApplicationCapability struct {
	Application string `json:"application"`
	Name        string `json:"name"`
	Controls    bool   `json:"controls"`
	preview     func(context.Context, *Snapshot, ChangeIntent) *ChangePreview
}

var applicationCatalog = []ApplicationCapability{
	{"codex", "Codex", true, previewCodexChange}, {"claude-code", "Claude Code", true, previewClaudeChange},
	{"hermes", "Hermes", true, previewHermesChange}, {"pi", "Pi", true, previewPiChange},
	{"opencode", "OpenCode", true, previewOpencodeChange}, {"gemini", "Gemini CLI", true, previewGeminiChange},
	{"codeaf", "CodeAF", false, nil},
}

func ApplicationCapabilities() []ApplicationCapability {
	return append([]ApplicationCapability(nil), applicationCatalog...)
}
func applicationCapability(app string) (ApplicationCapability, bool) {
	for _, c := range applicationCatalog {
		if c.Application == app {
			return c, true
		}
	}
	return ApplicationCapability{}, false
}

type GroupIntent struct {
	Source       SourceID      `json:"source_id"`
	Applications []string      `json:"applications"`
	Scope        ControlScope  `json:"scope"`
	Choice       ControlChoice `json:"choice"`
}
type GroupConsumer struct {
	Application  string                   `json:"application"`
	Controls     bool                     `json:"controls"`
	Scope        Scope                    `json:"scope"`
	References   []Reference              `json:"references,omitempty"`
	Availability *AvailabilityExplanation `json:"availability,omitempty"`
	Reason       string                   `json:"reason,omitempty"`
}
type GroupMember struct {
	Application string     `json:"application"`
	Preview     ChangeView `json:"preview"`
}
type GroupView struct {
	State             string          `json:"state"`
	Reason            string          `json:"reason"`
	Intent            GroupIntent     `json:"intent"`
	Source            *Source         `json:"source,omitempty"`
	InspectionProject ProjectContext  `json:"inspection_project"`
	Consumers         []GroupConsumer `json:"consumers"`
	Members           []GroupMember   `json:"members"`
	Notes             []string        `json:"notes"`
	Confirmation      string          `json:"confirmation,omitempty"`
}
type GroupResult struct {
	Application string         `json:"application"`
	State       string         `json:"state"` // succeeded, failed, or not-attempted
	Reason      string         `json:"reason"`
	Outcome     *ChangeOutcome `json:"outcome,omitempty"`
}
type GroupOutcome struct {
	View    GroupView     `json:"context"`
	State   string        `json:"state"`
	Reason  string        `json:"reason"`
	Results []GroupResult `json:"results"`
}

func (out GroupOutcome) ExitCode() int {
	if out.State == "applied" {
		return 0
	}
	return 1
}

// GroupPreview composes immutable single-application previews. It owns aggregate
// consent, bounded consumer discovery, interference checks and stop-on-failure.
// It never rebases child consent or rolls back earlier successful changes.
type GroupPreview struct {
	view     GroupView
	request  Request
	children []*ChangePreview
	seal     string
	once     sync.Mutex
	used     bool
}

func (p *GroupPreview) View() GroupView {
	if p == nil {
		return GroupView{State: "refused", Reason: "no group preview"}
	}
	view := p.view
	view.Intent.Applications = append([]string(nil), view.Intent.Applications...)
	if view.Source != nil {
		source := cloneSource(*view.Source)
		view.Source = &source
	}
	view.Consumers = append([]GroupConsumer(nil), view.Consumers...)
	for i := range view.Consumers {
		c := &view.Consumers[i]
		c.Scope = cloneScope(c.Scope)
		c.References = append([]Reference(nil), c.References...)
		for j := range c.References {
			c.References[j] = cloneReference(c.References[j])
		}
		if c.Availability != nil {
			value := cloneAvailability(*c.Availability)
			c.Availability = &value
		}
	}
	view.Members = append([]GroupMember(nil), view.Members...)
	for i := range view.Members {
		view.Members[i].Preview = cloneChangeView(view.Members[i].Preview)
	}
	view.Notes = append([]string(nil), view.Notes...)
	return view
}

// groupRequest preserves explicit roots/project and initializes every native
// home through the same constructor used by singleton commands. Private homes
// already normalized in the originating request keep their original overrides.
func groupRequest(base Request, app string) (Request, error) {
	r, err := NewRequestWithAgent(nil, nil, nil, app)
	if err != nil {
		return Request{}, err
	}
	r.Roots = append([]DiscoveryRoot(nil), base.Roots...)
	r.Project = base.Project
	if base.Agent == app && base.home != "" {
		r.home = base.home
	}
	if base.codexHome != "" {
		r.codexHome = base.codexHome
	}
	if base.claudeHome != "" {
		r.claudeHome = base.claudeHome
	}
	if base.hermesHome != "" {
		r.hermesHome = base.hermesHome
	}
	if base.codeafProfile != "" {
		r.codeafProfile = base.codeafProfile
	}
	return r, nil
}
func (s *Snapshot) PreviewGroupChange(ctx context.Context, intent GroupIntent) *GroupPreview {
	intent.Applications = append([]string(nil), intent.Applications...)
	p := &GroupPreview{view: GroupView{State: "refused", Reason: "whole request is not ready; no settings changed", Intent: intent, Notes: []string{
		"Discovery is bounded by the listed native roots and selected project, not the whole machine.",
		"Apply runs in the listed order and stops at the first non-success. Earlier successful changes remain; later applications are not attempted.",
		"Unsupported members block the whole request. A supported subset requires a new explicit selection and preview.",
		"Session adoption is unknown; fresh load or restart may be required per application.",
	}}}
	if s == nil {
		p.view.Reason = "no selected collection"
		return p
	}
	p.request = cloneRequest(s.request)
	p.view.InspectionProject = s.request.Project
	source, ok := s.Source(intent.Source)
	if !ok || source.Identity.Certainty != "proven" {
		p.view.Reason = "selected exact source is not proven or reachable"
		return p
	}
	p.view.Source = &source
	valid := len(intent.Applications) > 0
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, app := range intent.Applications {
		counts[app]++
		if _, known := applicationCapability(app); !known || seen[app] {
			valid = false
		}
		seen[app] = true
	}
	// Observe all catalog members, including unrequested consumers: new references
	// or coverage changes invalidate the aggregate confirmation before any write.
	snapshots := map[string]*Snapshot{}
	var fingerprints []View
	for _, capability := range applicationCatalog {
		c := GroupConsumer{Application: capability.Application, Controls: capability.Controls}
		request, err := groupRequest(p.request, capability.Application)
		if err == nil {
			snapshots[capability.Application], err = Build(ctx, request)
		}
		if err != nil {
			c.Reason = "native discovery unavailable: " + err.Error()
			c.Scope = Scope{Project: p.request.Project, Completeness: Incomplete}
		} else {
			native := snapshots[capability.Application]
			c.Scope = native.Scope()
			fingerprints = append(fingerprints, native.Query(Filters{Context: FilterAll}, QuerySources))
			if exact, found := native.Source(intent.Source); found && exact.Identity.Certainty == "proven" {
				c.References = native.References(exact.ID)
				availability := cloneAvailability(exact.Availability)
				c.Availability = &availability
			} else {
				c.Reason = "no proven native reference to this exact source; independent same-name copies are not substituted"
			}
		}
		p.view.Consumers = append(p.view.Consumers, c)
	}
	for _, app := range intent.Applications {
		childIntent := ChangeIntent{Source: intent.Source, Application: app, Scope: intent.Scope, Choice: intent.Choice}
		child := rejectedChange(childIntent, "unsupported", "native controls are unsupported for this application")
		capability, known := applicationCapability(app)
		if !known {
			child = rejectedChange(childIntent, "refused", "unknown application: "+app)
		} else if capability.Controls {
			if native := snapshots[app]; native != nil {
				child = native.PreviewChange(ctx, childIntent)
			} else {
				child = rejectedChange(childIntent, "refused", "native discovery could not be established")
			}
		}
		if counts[app] > 1 {
			child = rejectedChange(childIntent, "refused", "duplicate application: "+app)
		}
		view := child.View()
		view.Confirmation = ""
		if child.plan == nil || view.State != "ready" {
			valid = false
		}
		p.children = append(p.children, child)
		p.view.Members = append(p.view.Members, GroupMember{app, view})
	}
	if len(intent.Applications) == 0 {
		p.view.Reason = "select at least one application explicitly"
	} else if !valid {
		p.view.Reason = "unknown, duplicate, unsupported or unverified requested application; whole request unapplied"
	}
	if valid {
		if reason := groupInterference(p.children); reason != "" {
			valid = false
			p.view.Reason = reason
		}
	}
	if !valid {
		return p
	}
	p.view.State = "ready"
	p.view.Reason = "all requested native previews are ready; preview only"
	digests := make([]string, len(p.children))
	for i, c := range p.children {
		digests[i] = c.plan.digest
	}
	raw, err := json.Marshal(struct {
		Version   string
		View      GroupView
		Discovery []View
		Digests   []string
	}{"group-v1", p.view, fingerprints, digests})
	if err != nil {
		p.view.State = "refused"
		p.view.Reason = "group evidence cannot be sealed: " + err.Error()
		return p
	}
	sum := sha256.Sum256(raw)
	p.seal = "g1:" + hex.EncodeToString(sum[:])
	p.view.Confirmation = p.seal
	return p
}

// A location retains object-identity anchors and the relative suffix beneath
// each one. Keeping ancestor anchors also permits containment across directory aliases.
type groupAnchor struct {
	object  changeIdentity
	suffix  string
	missing int
}
type groupLocation []groupAnchor

func groupLocationAt(path string) (groupLocation, error) {
	tail := ""
	missing := 0
	resolved := ""
	for {
		var err error
		resolved, err = filepath.EvalSymlinks(path)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil, err
		}
		tail = filepath.Join(filepath.Base(path), tail)
		missing++
		path = parent
	}
	var location groupLocation
	for {
		// Stat follows any remaining alias and observes only metadata, never file
		// content or directory membership. No writes probe filesystem case rules.
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, err
		}
		if tail != "" && !info.IsDir() {
			return nil, errors.New("non-directory footprint ancestor")
		}
		location = append(location, groupAnchor{statIdentity(resolved, info.Sys().(*syscall.Stat_t)), tail, missing})
		parent := filepath.Dir(resolved)
		if parent == resolved {
			break
		}
		tail = filepath.Join(filepath.Base(resolved), tail)
		resolved = parent
	}
	return location, nil
}
func sameGroupAnchor(a, b changeIdentity) bool {
	return a.Device == b.Device && a.Inode == b.Inode && a.BirthSec == b.BirthSec && a.BirthNano == b.BirthNano
}
func (path groupAnchor) within(directory groupAnchor) bool {
	if !sameGroupAnchor(path.object, directory.object) {
		return false
	}
	if directory.suffix == "" {
		return true
	}
	if path.suffix == "" {
		return false
	}
	parts := strings.Split(path.suffix, string(filepath.Separator))
	parents := strings.Split(directory.suffix, string(filepath.Separator))
	if len(parts) < len(parents) {
		return false
	}
	for i, parent := range parents {
		if parts[i] == parent {
			continue
		}
		if i < len(parts)-path.missing || i < len(parents)-directory.missing {
			return false
		}
		fold := cases.Fold()
		if norm.NFD.String(fold.String(norm.NFD.String(parts[i]))) != norm.NFD.String(fold.String(norm.NFD.String(parent))) {
			return false
		}
	}
	return true
}
func (path groupLocation) equal(other groupLocation) bool {
	for _, a := range path {
		for _, b := range other {
			if a.within(b) && b.within(a) {
				return true
			}
		}
	}
	return false
}
func (path groupLocation) within(directory groupLocation) bool {
	for _, a := range path {
		for _, b := range directory {
			if a.within(b) {
				return true
			}
		}
	}
	return false
}
func groupInterference(children []*ChangePreview) string {
	locations := map[string]groupLocation{}
	locate := func(path string) (groupLocation, error) {
		if location, ok := locations[path]; ok {
			return location, nil
		}
		location, err := groupLocationAt(path)
		if err == nil {
			locations[path] = location
		}
		return location, err
	}
	for i, child := range children {
		if child.view.NoWriteNeeded {
			continue
		}
		writer := child.plan
		write, err := locate(writer.destination.Path)
		if err != nil {
			return "configuration footprint cannot be resolved safely; whole request unapplied"
		}
		var create groupLocation
		if writer.destination.CreateDirectory != "" {
			create, err = locate(writer.destination.CreateDirectory)
			if err != nil {
				return "directory creation footprint cannot be resolved safely; whole request unapplied"
			}
		}
		for j, other := range children {
			if i == j || other.plan == nil {
				continue
			}
			// Write/write aliases refuse in either order. An earlier no-op has no
			// replacement effect; only a later read can be invalidated by this write.
			if !other.view.NoWriteNeeded {
				destination, err := locate(other.plan.destination.Path)
				if err != nil {
					return "configuration footprint cannot be resolved safely; whole request unapplied"
				}
				if write.equal(destination) {
					return "requested configuration destinations may alias; whole request unapplied"
				}
			}
			if j < i {
				continue
			}
			for _, fact := range other.plan.dependencies {
				read, err := locate(fact.Path)
				if err != nil {
					return "preview input footprint cannot be resolved safely; whole request unapplied"
				}
				if write.equal(read) || (len(create) > 0 && read.within(create)) {
					return fmt.Sprintf("%s may change %s preview input %s; whole request unapplied", child.view.Intent.Application, other.view.Intent.Application, fact.Path)
				}
			}
			// Directory facts omit entry membership. Preserve the independent semantic
			// discovery check using the same identity-aware containment comparison.
			var observations struct {
				View
				Selected, Global View
			}
			if err := json.Unmarshal(other.plan.snapshotFacts, &observations); err != nil {
				return "native discovery footprint cannot be established; whole request unapplied"
			}
			var paths []string
			for _, view := range []View{observations.View, observations.Selected, observations.Global} {
				for _, root := range view.Scope.Roots {
					paths = append(paths, root.Requested, root.Resolved)
				}
				// Every inspected bundle's resource inventory is part of consent,
				// even when a child alias escapes its discovery root or the bundle
				// is inspection-only. Standalone files remain exact reads; resource
				// symlink targets are not recursively inventoried.
				for _, source := range view.Sources {
					if source.Identity.Method != "darwin-file-object" && source.Directory != source.Manifest.ResolvedPath {
						paths = append(paths, source.Directory)
					}
				}
			}
			for _, path := range paths {
				if path == "" {
					continue
				}
				discovery, err := locate(path)
				if err != nil {
					return "native discovery footprint cannot be resolved safely; whole request unapplied"
				}
				if write.within(discovery) || (len(create) > 0 && discovery.within(create)) {
					return "configuration write potentially intersects later native discovery; whole request unapplied"
				}
			}
		}
	}
	return ""
}
func (p *GroupPreview) Apply(ctx context.Context, confirmation string) GroupOutcome {
	out := GroupOutcome{State: "refused", Reason: "confirmation does not match this current group; request a new preview"}
	if p == nil {
		return out
	}
	p.once.Lock()
	defer p.once.Unlock()
	out.View = p.View()
	out.View.Confirmation = ""
	for _, app := range p.view.Intent.Applications {
		out.Results = append(out.Results, GroupResult{Application: app, State: "not-attempted", Reason: "whole request unapplied"})
	}
	if p.used || p.seal == "" || confirmation == "" || confirmation != p.seal {
		return out
	}
	p.used = true
	if err := ctx.Err(); err != nil {
		out.Reason = "cancelled: " + err.Error()
		for i := range out.Results {
			out.Results[i].Reason = out.Reason
		}
		return out
	}
	fresh, err := Build(ctx, cloneRequest(p.request))
	if err != nil {
		out.Reason = "initial reinspection failed: " + err.Error()
		return out
	}
	check := fresh.PreviewGroupChange(ctx, p.view.Intent)
	if check.seal != p.seal {
		out.Reason = "group preview is stale; source, configuration, project, consumers or coverage changed; request a new preview"
		return out
	}
	out.State = "applied"
	out.Reason = "all requested applications succeeded"
	for i, child := range p.children {
		if err := ctx.Err(); err != nil {
			out.State = "failed"
			out.Reason = "cancelled: " + err.Error()
			for j := i; j < len(out.Results); j++ {
				out.Results[j].Reason = out.Reason
			}
			break
		}
		result := child.Apply(ctx, child.plan.digest)
		out.Results[i] = GroupResult{Application: child.view.Intent.Application, State: "succeeded", Reason: result.Reason, Outcome: &result}
		if result.ExitCode() != 0 {
			out.Results[i].State = "failed"
			out.State = "failed"
			out.Reason = "stopped at first non-success; earlier successful changes remain"
			for j := i + 1; j < len(out.Results); j++ {
				out.Results[j].Reason = "not attempted after " + child.view.Intent.Application + " non-success"
			}
			break
		}
	}
	return out
}
