package collection

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"unicode/utf8"
)

type opencodeContextFacts struct {
	snapshot  *Snapshot
	invalid   error
	sources   map[SourceID]error
	manifests map[SourceID]changeFact
	names     map[string][]SourceID
}

type opencodeDestinationKey struct {
	scope ControlScope
	path  string
}

type opencodeDestinationFact struct {
	destination changeDestination
	err         error
}

type opencodeControlFacts struct {
	selected      *opencodeContextFacts
	global        *opencodeContextFacts
	globalErr     error
	projectInputs map[string]bool
	destinations  map[opencodeDestinationKey]opencodeDestinationFact
}

func opencodeGlobalSnapshot(ctx context.Context, s *Snapshot) (*Snapshot, error) {
	if !s.request.Project.Selected {
		return s, nil
	}
	request := cloneRequest(s.request)
	request.Project = ProjectContext{}
	request.opencodePlanning = true
	return Build(ctx, request)
}

func newOpencodeContextFacts(s *Snapshot) *opencodeContextFacts {
	f := &opencodeContextFacts{snapshot: s, sources: map[SourceID]error{}, manifests: map[SourceID]changeFact{}, names: map[string][]SourceID{}}
	if s.opencodeControls == nil || !s.Complete() || s.opencodeControls.uncertain {
		f.invalid = errors.New("native OpenCode discovery or configuration is incomplete or unverified; isolated effects cannot be bounded")
	} else {
		for _, input := range s.opencodeControls.inputs {
			if !utf8.Valid(input.Raw) {
				f.invalid = errors.New("native configuration is not valid UTF-8; no writer enabled")
			}
		}
	}
	for _, id := range s.order {
		source := s.sources[id]
		for _, ref := range source.Availability.References {
			if ref.Participation == ParticipationEligible || ref.Participation == ParticipationUnknown {
				f.names[source.Availability.NativeName] = append(f.names[source.Availability.NativeName], id)
				break
			}
		}
		if !changeWriterEligible(source) || source.Ownership.State == "managed" {
			f.sources[id] = errors.New("a proven ordinary native OpenCode source is required; managed, plugin and inspection-only targets are unsupported")
			continue
		}
		manifest, err := observeChangePath(source.Manifest.Path)
		if err != nil || manifest.Links != 1 {
			f.sources[id] = opencodeControlRefusal{"manifest identity is unsafe or hardlinked"}
			continue
		}
		f.manifests[id] = manifest
	}
	return f
}

func (f *opencodeContextFacts) validate(id SourceID) error {
	if f.invalid != nil {
		return f.invalid
	}
	source, found := f.snapshot.sources[id]
	if !found {
		return errors.New("selected source is no longer reachable in the native control context")
	}
	if err := f.sources[id]; err != nil {
		return err
	}
	for _, other := range f.names[source.Availability.NativeName] {
		if other != id {
			return opencodeControlRefusal{"name-based permission would affect an independent same-name source; select an isolated native target"}
		}
	}
	return nil
}

func newOpencodeControlFacts(ctx context.Context, s *Snapshot) *opencodeControlFacts {
	f := &opencodeControlFacts{selected: newOpencodeContextFacts(s), destinations: map[opencodeDestinationKey]opencodeDestinationFact{}, projectInputs: map[string]bool{}}
	global, err := opencodeGlobalSnapshot(ctx, s)
	f.globalErr = err
	if err == nil {
		f.global = f.selected
		if global != s {
			f.global = newOpencodeContextFacts(global)
		}
	}
	if s.opencodeControls != nil {
		for _, input := range s.opencodeControls.inputs {
			if input.Scope == "native-directory" {
				f.projectInputs[input.Path] = opencodeProjectPath(s, input.Path)
			}
		}
	}
	return f
}

func (f *opencodeControlFacts) validateGlobal(id SourceID) error {
	if err := f.selected.validate(id); err != nil {
		return err
	}
	if f.globalErr != nil {
		return errors.New("global native inventory could not be inspected")
	}
	if err := f.global.validate(id); err != nil {
		return err
	}
	if !sameChangeFacts(f.selected.manifests[id], f.global.manifests[id]) {
		return opencodeControlRefusal{"selected source identity differs between project and global inventories"}
	}
	return nil
}

func (f *opencodeControlFacts) destination(recipe opencodeControlPlan) (changeDestination, error) {
	key := opencodeDestinationKey{recipe.Scope, recipe.ConfigPath}
	if cached, found := f.destinations[key]; found {
		return cached.destination, cached.err
	}
	dest, err := inspectChangeDestinationPath(recipe.ConfigPath)
	if err == nil {
		err = opencodeDestinationScope(f.selected.snapshot, recipe)
	}
	if err == nil && recipe.Scope == ControlGlobal && f.global != f.selected {
		err = opencodeDestinationScope(f.global.snapshot, recipe)
	}
	f.destinations[key] = opencodeDestinationFact{dest, err}
	return dest, err
}

func opencodeIndependentEffects(before, after *Snapshot, selected SourceID) bool {
	if len(before.sources) != len(after.sources) {
		return false
	}
	for id, source := range before.sources {
		next, exists := after.sources[id]
		if !exists || id != selected && (!piSamePermissions(source.Availability, next.Availability) || source.Availability.State != next.Availability.State) {
			return false
		}
	}
	return true
}

func opencodeChangeObservations(selected, global *Snapshot, dest changeDestination) ([]byte, []changeFact, error) {
	raw, deps, err := changeObservations(selected, dest)
	if err != nil || global == nil || global == selected {
		return raw, deps, err
	}
	globalRaw, globalDeps, err := changeObservations(global, dest)
	if err != nil {
		return nil, nil, err
	}
	byPath := map[string]changeFact{}
	for _, fact := range append(deps, globalDeps...) {
		if previous, found := byPath[fact.Path]; found && !sameChangeFacts(previous, fact) {
			return nil, nil, errors.New("project and global input observations changed during inspection")
		}
		byPath[fact.Path] = fact
	}
	deps = nil
	for _, fact := range byPath {
		deps = append(deps, fact)
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })
	raw, err = json.Marshal(struct{ Selected, Global json.RawMessage }{raw, globalRaw})
	return raw, deps, err
}

func (p *changePlan) opencodeGlobalResult(ctx context.Context, selected *Snapshot) (string, string) {
	facts := newOpencodeControlFacts(ctx, selected)
	if err := facts.validateGlobal(p.intent.Source); err != nil {
		return "unknown", "global native control certainty: " + err.Error()
	}
	global := facts.global
	expected := p.opencodeGlobalExpected
	if expected == nil || !opencodeIndependentEffects(expected.snapshot, global.snapshot, p.intent.Source) || !sameChangeFacts(expected.manifests, global.manifests) {
		return "unknown", "global native source membership, identity or independent permissions changed after preview"
	}
	permission := global.snapshot.opencodeControls.permission(global.snapshot.sources[p.intent.Source].Availability.NativeName)
	if !opencodeChoiceAchieved(p.intent.Choice, permission.State) {
		achievement := "unmet"
		if permission.State == PermissionUnknown {
			achievement = "unknown"
		}
		return achievement, "global configured permission: " + permission.Evidence.Reason
	}
	return "", ""
}
