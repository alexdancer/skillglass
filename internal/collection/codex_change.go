package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

type codexChangePlan struct {
	request       Request
	intent        ChangeIntent
	digest        string
	postimage     []byte
	destination   changeDestination
	snapshotFacts []byte
	dependencies  []changeFact
	fault         func(string) error
}

func rejectedChange(intent ChangeIntent, state, reason string) *ChangePreview {
	return &ChangePreview{view: ChangeView{State: state, Reason: reason, Intent: intent}}
}
func previewCodexChange(ctx context.Context, s *Snapshot, intent ChangeIntent) *ChangePreview {
	if s == nil || intent.Application != "codex" || s.request.Agent != "codex" {
		return rejectedChange(intent, "unsupported", "explicit --agent codex is required for this validated writer")
	}
	if intent.Scope != ControlGlobal {
		return rejectedChange(intent, "unsupported", "only explicit global user configuration is supported; project controls do not fall back to global")
	}
	if intent.Choice != ControlEnable && intent.Choice != ControlDisable {
		return rejectedChange(intent, "refused", "choose enable or disable")
	}
	fresh, err := Build(ctx, cloneRequest(s.request))
	if err != nil {
		return rejectedChange(intent, "failed", "could not reinspect the original context")
	}
	source, found := fresh.Source(intent.Source)
	if !found || source.Identity.Certainty != "proven" {
		return rejectedChange(intent, "refused", "selected source is no longer provably reachable; select its exact source ID")
	}
	eligible := false
	for _, ref := range source.Availability.References {
		eligible = eligible || ref.Participation == ParticipationEligible
	}
	if !eligible {
		return rejectedChange(intent, "unsupported", "an eligible native Codex reference to this exact manifest is required; custom inspection alone cannot authorize a write")
	}
	target := canonicalManifest(filepath.Join(source.Directory, "SKILL.md"))
	if target == "" || source.Manifest.IsSymlink {
		return rejectedChange(intent, "refused", "native target must be an ordinary canonical SKILL.md file")
	}
	manifest, err := observeChangePath(target)
	if err != nil || manifest.Links != 1 {
		return rejectedChange(intent, "refused", "manifest identity is unsafe or shared through hardlinks")
	}
	destination, err := inspectChangeDestination(s.request.codexHome)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	raw := destination.Config.Bytes
	postimage, change, writeNeeded, err := patchCodexChoice(raw, target, intent.Choice == ControlEnable)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	// Introducing a higher array must not discard unrelated inherited selectors.
	userDocument := make(map[string]any)
	_ = toml.Unmarshal(raw, &userDocument)
	_, hasUserArray, _ := choiceArray(userDocument)
	if err := protectInheritedChoices(fresh.scope.Agent.ConfigLayers, target, hasUserArray); err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	facts, dependencies, err := changeObservations(fresh, destination)
	if err != nil {
		return rejectedChange(intent, "refused", "relevant source/config evidence cannot be pinned: "+err.Error())
	}
	view := ChangeView{State: "ready", Reason: "preview only; no settings changed", Intent: intent, SourcePath: source.Directory, ManifestPath: target, ConfigPath: destination.Path, InspectionProject: fresh.request.Project, Changes: []SettingChange{change}, KnownReferences: fresh.References(source.ID), Before: source.Availability, NoWriteNeeded: !writeNeeded, CreateDirectory: destination.CreateDirectory, Notes: []string{"The canonical manifest selector affects this source's Codex references; skill content and other applications' settings are retained.", "Session adoption is unknown; fresh load or restart may be required."}}
	if source.Availability.Precedence != nil && len(source.Availability.Precedence.RelatedSources) > 0 {
		view.Notes = append(view.Notes, "Independent same-name sources remain unchanged: "+fmt.Sprint(source.Availability.Precedence.RelatedSources))
	}
	if source.Ownership.State == "managed" {
		view.Notes = append(view.Notes, "Precise per-skill user setting only; manager configuration and managed content stay intact.")
	}
	p := &codexChangePlan{request: cloneRequest(s.request), intent: intent, postimage: postimage, destination: destination, snapshotFacts: facts, dependencies: dependencies}
	tokenData, _ := json.Marshal(struct {
		Version         string
		Request         Request
		Home, CodexHome string
		Intent          ChangeIntent
		Facts           []byte
		Dependencies    []changeFact
		Destination     changeDestination
		Proposal        []byte
	}{"codex-global-v1", p.request, p.request.home, p.request.codexHome, intent, facts, dependencies, destination, postimage})
	sum := sha256.Sum256(tokenData)
	p.digest = "c1:" + hex.EncodeToString(sum[:])
	view.Confirmation = p.digest
	return &ChangePreview{view: view, plan: p}
}
func changeObservations(s *Snapshot, destination changeDestination) ([]byte, []changeFact, error) {
	view := s.Query(Filters{Context: FilterAll}, QuerySources)
	raw, err := json.Marshal(view)
	if err != nil {
		return nil, nil, err
	}
	paths := map[string]bool{}
	add := func(path string) {
		if filepath.IsAbs(path) {
			paths[path] = true
		}
	}
	evidence := func(value Evidence) { add(value.InputPath) }
	layers := func(values []PermissionLayer) {
		for _, layer := range values {
			add(layer.Path)
			evidence(layer.Evidence)
			for _, choice := range layer.Choices {
				add(choice.Path)
			}
		}
	}
	if view.Scope.Agent != nil {
		evidence(view.Scope.Agent.Evidence)
		layers(view.Scope.Agent.ConfigLayers)
	}
	for _, root := range view.Scope.Roots {
		add(root.Requested)
		add(root.Resolved)
		if root.Evidence != nil {
			evidence(*root.Evidence)
		}
	}
	for _, source := range view.Sources {
		add(source.Directory)
		add(source.Manifest.Path)
		add(source.Manifest.ResolvedPath)
		add(filepath.Join(source.Directory, "agents", "openai.yaml"))
		availability := source.Availability
		if availability.Evidence != nil {
			evidence(*availability.Evidence)
		}
		if availability.NativeNameEvidence != nil {
			evidence(*availability.NativeNameEvidence)
		}
		if availability.Precedence != nil {
			evidence(availability.Precedence.Evidence)
		}
		for _, session := range []*SessionExplanation{availability.Invocation, availability.Session, availability.Reload} {
			if session != nil {
				evidence(session.Evidence)
			}
		}
		for _, reference := range availability.References {
			evidence(reference.Evidence)
			evidence(reference.Provenance.Evidence)
			evidence(reference.Permission.Evidence)
			layers(reference.Permission.Layers)
		}
	}
	for _, reference := range view.References {
		add(reference.DiscoveredPath)
		add(reference.ResolvedPath)
	}
	add(destination.Path)
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var facts []changeFact
	for _, path := range ordered {
		fact, err := observeChangePath(path)
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil, nil, errors.New("unreadable relevant input")
			}
			return nil, nil, err
		}
		facts = append(facts, fact)
	}
	return raw, facts, nil
}
func (p *codexChangePlan) recheck(ctx context.Context) error {
	fresh := previewCodexChange(ctx, &Snapshot{request: p.request}, p.intent)
	if fresh.plan == nil || fresh.plan.digest != p.digest {
		return errors.New("preview is stale; source, configuration, context or file identity changed; request a new preview")
	}
	return nil
}
func (p *codexChangePlan) inspectResult(out ChangeOutcome, parent changeIdentity) ChangeOutcome {
	current, err := observeChangePath(p.destination.Path)
	parentFact, parentErr := observeChangePath(filepath.Dir(p.destination.Path))
	parentMatches := parentErr == nil && parentFact.Resolved != ""
	if parentMatches {
		resolvedParent, err := observeChangePath(parentFact.Resolved)
		parentMatches = err == nil && resolvedParent.Identity.Inode == parent.Inode && resolvedParent.Identity.Device == parent.Device && resolvedParent.Identity.BirthSec == parent.BirthSec && resolvedParent.Identity.BirthNano == parent.BirthNano
	}
	metadataMatches := !p.destination.Config.Exists || (current.Identity.Mode == p.destination.Config.Identity.Mode && current.Identity.UID == p.destination.Config.Identity.UID && current.Identity.GID == p.destination.Config.Identity.GID && sameChangeFacts(current.Attributes, p.destination.Config.Attributes))
	if err == nil && bytes.Equal(current.Bytes, p.postimage) && metadataMatches && parentMatches {
		out.Persistence = "verified"
		if p.intent.Choice == ControlEnable {
			out.PersistedPermission = PermissionAllowed
		} else {
			out.PersistedPermission = PermissionDenied
		}
	}
	fresh, err := Build(context.Background(), p.request)
	if err != nil {
		out.Reason += "; reinspection failed"
		out.State = "failed"
		return out
	}
	out.Snapshot = fresh
	if source, ok := fresh.Source(p.intent.Source); ok {
		effective := source.Availability
		out.Effective = &effective
		if p.intent.Choice == ControlDisable && effective.State == AvailabilityDisabled {
			out.Achievement = "achieved"
		} else if p.intent.Choice == ControlEnable && (effective.State == AvailabilityAvailable || effective.State == AvailabilityExplicitOnly || effective.State == AvailabilityAsk) {
			out.Achievement = "achieved"
		} else if effective.State == AvailabilityUnknown {
			out.Achievement = "unknown"
		} else {
			out.Achievement = "unmet"
		}
		if out.Achievement != "achieved" {
			out.Reason += "; effective result " + out.Achievement + ": " + effective.Reason
		}
	} else {
		out.Reason += "; selected source no longer reachable; effective result unknown"
	}
	return out
}
func changeReload() SessionExplanation {
	return SessionExplanation{State: "unknown", Evidence: codexEvidence(EvidenceUnknown, "session-reload", "", "", "local reinspection verifies configuration only; fresh load or restart may be required")}
}
func changeWriterEligible(source Source) bool {
	if source.Identity.Certainty != "proven" || source.Manifest.IsSymlink {
		return false
	}
	for _, ref := range source.Availability.References {
		if ref.Participation == ParticipationEligible {
			return true
		}
	}
	return false
}

func protectInheritedChoices(layers []PermissionLayer, target string, hasUserArray bool) error {
	if hasUserArray {
		return nil
	}
	for _, layer := range layers {
		if layer.Scope != "system" {
			continue
		}
		if layer.State == "unknown" {
			return errors.New("inherited system configuration cannot be established safely; no settings changed")
		}
		for _, choice := range layer.Choices {
			if canonicalManifest(choice.Path) != target {
				return errors.New("new user skills.config array would discard an unrelated inherited selector from " + layer.Path)
			}
		}
	}
	return nil
}
