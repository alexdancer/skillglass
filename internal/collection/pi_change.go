package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// piControlPlan separates native lexical selectors from source filesystem identity.
// Shared changePlan owns consent, staleness and safe persistence.
type piControlPlan struct {
	SettingsPath           string
	Targets, GlobalTargets []string
	Choice                 ControlChoice
	Scope                  ControlScope
}

func planPiControl(s *Snapshot, intent ChangeIntent) (piControlPlan, error) {
	p := piControlPlan{Choice: intent.Choice, Scope: intent.Scope}
	if intent.Scope != ControlGlobal && intent.Scope != ControlProject {
		return p, errors.New("choose explicit global or project scope")
	}
	if intent.Choice != ControlEnable && intent.Choice != ControlDisable && intent.Choice != ControlInherit {
		return p, errors.New("choose enable, disable or inherit")
	}
	if intent.Choice == ControlInherit && intent.Scope != ControlProject {
		return p, errors.New("inherit removes only a project override")
	}
	inputs := s.piControls
	if inputs == nil || !s.Complete() || inputs.global.uncertain || inputs.project.uncertain {
		return p, errors.New("native Pi discovery or settings are incomplete; isolated effects cannot be bounded")
	}
	if intent.Scope == ControlProject && (!s.request.Project.Selected || inputs.trust != "allowed") {
		return p, errors.New("project controls require an explicit trusted launch project; no global fallback")
	}
	source, found := s.sources[intent.Source]
	if !found || !changeWriterEligible(source) || source.Ownership.State == "managed" {
		return p, errors.New("a proven ordinary native Pi source is required; package and inspection-only controls are unsupported")
	}
	manifest, err := observeChangePath(source.Manifest.Path)
	if err != nil || manifest.Links != 1 {
		return p, errors.New("manifest identity is unsafe or hardlinked")
	}
	global, local := map[string]bool{}, map[string]bool{}
	for _, rid := range source.References {
		ref := s.references[rid]
		fact, native := inputs.facts[ref.Root]
		if !native {
			continue
		}
		if fact.provenance.Kind == "plugin" {
			return p, errors.New("package-owned resources require their native manager; isolated package controls are unverified")
		}
		if fact.uncertain {
			return p, errors.New("native occurrence semantics are uncertain")
		}
		file := piOccurrenceFile(source, ref)
		if strings.TrimSpace(file) != file || strings.ContainsAny(file, "\r\n") {
			return p, errors.New("native lexical target has unverified whitespace")
		}
		if ref.Context == ContextGlobal {
			global[file] = true
		}
		if ref.Context == ContextProject {
			local[file] = true
		}
	}
	for path := range global {
		p.GlobalTargets = append(p.GlobalTargets, path)
	}
	targets := global
	p.SettingsPath = inputs.global.path
	if intent.Scope == ControlProject {
		p.SettingsPath = inputs.project.path
		targets = local
		for path := range global {
			targets[path] = true
		}
	}
	for path := range targets {
		p.Targets = append(p.Targets, path)
	}
	sort.Strings(p.Targets)
	sort.Strings(p.GlobalTargets)
	if len(p.Targets) == 0 {
		return p, errors.New("requested scope has no native occurrence; no broader fallback")
	}
	return p, nil
}

func previewPiChange(ctx context.Context, s *Snapshot, intent ChangeIntent) *ChangePreview {
	if s == nil || s.request.Agent != "pi" {
		return rejectedChange(intent, "unsupported", "explicit --agent pi is required")
	}
	fresh, err := Build(ctx, cloneRequest(s.request))
	if err != nil {
		return rejectedChange(intent, "failed", "could not reinspect original Pi context")
	}
	if _, found := fresh.sources[intent.Source]; !found {
		return rejectedChange(intent, "refused", "selected source is no longer reachable; select its exact source ID")
	}
	p := previewPiSnapshot(ctx, fresh, intent, true)
	p.view.InspectionProject = s.request.Project
	return p
}
func previewPiSnapshot(ctx context.Context, s *Snapshot, intent ChangeIntent, pin bool) *ChangePreview {
	recipe, err := planPiControl(s, intent)
	if err != nil {
		return rejectedChange(intent, "unsupported", err.Error())
	}
	destination, err := inspectChangeDestinationPath(recipe.SettingsPath)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	if intent.Scope == ControlProject {
		global, e := inspectChangeDestinationPath(s.piControls.global.path)
		if e != nil {
			return rejectedChange(intent, "refused", "global settings cannot be separated safely")
		}
		if destination.Parent.Identity.Device == global.Parent.Identity.Device && destination.Parent.Identity.Inode == global.Parent.Identity.Inode && strings.EqualFold(filepath.Base(destination.CreateDirectory), filepath.Base(global.CreateDirectory)) {
			return rejectedChange(intent, "refused", "project destination aliases global settings")
		}
	}
	post, changes, write, err := patchPiChoice(destination.Config.Bytes, recipe)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	prospective := cloneRequest(s.request)
	prospective.piPlanning = true
	prospective.piSettingsOverlay = map[string][]byte{recipe.SettingsPath: post}
	after, err := Build(ctx, prospective)
	if err != nil {
		return rejectedChange(intent, "failed", "prospective Pi resource evaluation failed")
	}
	selected, ok := after.sources[intent.Source]
	if !ok {
		return rejectedChange(intent, "refused", "removing registration would lose the selected source; inheritance is ambiguous")
	}
	if intent.Choice == ControlDisable && selected.Availability.State != AvailabilityDisabled || intent.Choice == ControlEnable && selected.Availability.State != AvailabilityAvailable && selected.Availability.State != AvailabilityExplicitOnly {
		return rejectedChange(intent, "refused", "requested result is prevented by a higher-priority occurrence, trust or independent same-name winner; no broader fallback")
	}
	for id, before := range s.sources {
		if id == intent.Source {
			continue
		}
		next, exists := after.sources[id]
		if !exists || !piSamePermissions(before.Availability, next.Availability) || piIndependentState(before.Availability.State) != piIndependentState(next.Availability.State) {
			return rejectedChange(intent, "refused", "resource rewrite would change an independent source's native permission or membership")
		}
	}
	if !pin {
		return &ChangePreview{view: ChangeView{State: "ready", Intent: intent}}
	}
	facts, deps, err := changeObservations(s, destination)
	if err != nil {
		return rejectedChange(intent, "refused", "relevant Pi inputs cannot be pinned: "+err.Error())
	}
	plan := &changePlan{request: cloneRequest(s.request), intent: intent, postimage: post, destination: destination, snapshotFacts: facts, dependencies: deps}
	data, _ := json.Marshal(struct {
		Version      string
		Recipe       piControlPlan
		Facts        []byte
		Dependencies []changeFact
		Destination  changeDestination
		Post         []byte
	}{"pi-1.1.0-controls-v1", recipe, facts, deps, destination, post})
	sum := sha256.Sum256(data)
	plan.digest = "pi1:" + hex.EncodeToString(sum[:])
	source := s.sources[intent.Source]
	return &ChangePreview{plan: plan, view: ChangeView{State: "ready", Reason: "preview only; no settings changed", Intent: intent, SourcePath: source.Directory, ManifestPath: source.Manifest.Path, ConfigPath: recipe.SettingsPath, InspectionProject: s.request.Project, Changes: changes, KnownReferences: s.References(source.ID), Before: source.Availability, NoWriteNeeded: !write, CreateDirectory: destination.CreateDirectory, Confirmation: plan.digest, Notes: []string{"Validated Pi 1.1.0 exact lexical resource controls preserve existing roots, patterns, packages and unrelated settings.", "Inherited project controls use an exact instruction-file registration and exact choice; inherit removes the native override pair when inherited discovery remains reachable.", "Disabling a name winner can expose an independent same-name copy; only the selected source is controlled.", "Existing sessions require settings reload; fresh sessions load saved choices. Interactive /reload adoption is unverified."}}}
}
func piSamePermissions(a, b AvailabilityExplanation) bool {
	permissions := func(ex AvailabilityExplanation) map[string]PermissionState {
		out := map[string]PermissionState{}
		for _, ref := range ex.References {
			out[string(ref.Reference)] = ref.Permission.State
		}
		return out
	}
	x, _ := json.Marshal(permissions(a))
	y, _ := json.Marshal(permissions(b))
	return string(x) == string(y)
}
func applyPiCapabilities(s *Snapshot) {
	for _, id := range s.order {
		source := s.sources[id]
		source.Availability.Capabilities = nil
		for _, entry := range []struct {
			op     string
			scope  ControlScope
			choice ControlChoice
		}{{"enable", ControlGlobal, ControlEnable}, {"disable", ControlGlobal, ControlDisable}, {"project-override", ControlProject, ControlDisable}, {"inherit", ControlProject, ControlInherit}} {
			preview := previewPiSnapshot(context.Background(), s, ChangeIntent{Source: id, Application: "pi", Scope: entry.scope, Choice: entry.choice}, false)
			state, kind, reason := "unsupported", EvidenceUnknown, preview.view.Reason
			if preview.view.State == "ready" {
				state, kind, reason = "supported", EvidenceInferred, "validated exact controls; preview, effect checks and confirmation required"
			}
			source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: entry.op, Scope: string(entry.scope), State: state, Evidence: piEvidence(kind, "native-settings-control", source.Manifest.Path, "skills", reason)})
		}
		source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: "delete", Scope: "source", State: "unsupported", Evidence: piEvidence(EvidenceUnknown, "no-delete", source.Directory, "", "deletion is unsupported")})
		s.sources[id] = source
	}
}

func piIndependentState(state AvailabilityState) AvailabilityState {
	if state == AvailabilityAvailable || state == AvailabilityExplicitOnly || state == AvailabilityShadowed {
		return AvailabilityAvailable
	}
	return state
}
