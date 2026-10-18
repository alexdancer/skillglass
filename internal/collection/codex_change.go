package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

func rejectedChange(intent ChangeIntent, state, reason string) *ChangePreview {
	return &ChangePreview{view: ChangeView{State: state, Reason: reason, Intent: intent}}
}
func previewCodexChange(ctx context.Context, s *Snapshot, intent ChangeIntent) *ChangePreview {
	if s == nil || intent.Application != "codex" || s.request.Agent != "codex" {
		return rejectedChange(intent, "unsupported", "explicit --agent codex is required for this validated writer")
	}
	if intent.Scope == ControlProject {
		preview := rejectedChange(intent, "unsupported", codexProjectControlReason)
		preview.view.InspectionProject = s.request.Project
		return preview
	}
	if intent.Scope != ControlGlobal {
		return rejectedChange(intent, "unsupported", "choose explicit global or project scope; no settings changed")
	}
	if intent.Choice == ControlInherit {
		return rejectedChange(intent, "unsupported", "inherit removes a project override; global scope has no project override to remove; no settings changed")
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
	p := &changePlan{request: cloneRequest(s.request), intent: intent, postimage: postimage, destination: destination, snapshotFacts: facts, dependencies: dependencies}
	p.reviewPatch = func(raw []byte, _ bool) ([]byte, []SettingChange, bool, error) {
		post, effect, write, err := patchCodexChoice(raw, target, intent.Choice == ControlEnable)
		return post, []SettingChange{effect}, write, err
	}
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
