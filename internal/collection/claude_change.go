package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func claudeControlName(s *Snapshot, source Source) (string, string) {
	if s.scope.Completeness == Incomplete {
		return "", "native discovery or configuration is incomplete; name-based effects cannot be bounded"
	}
	if !changeWriterEligible(source) {
		return "", "an ordinary proven native Claude Code source is required"
	}
	if source.Ownership.State == "managed" {
		return "", "managed or plugin sources require whole plugins or manager controls; isolated ordinary skill toggles are unsupported"
	}
	if source.Availability.NativeNameEvidence == nil || source.Availability.NativeNameEvidence.Kind == EvidenceUnknown {
		return "", "native command name is unverified"
	}
	name := ""
	for _, ref := range source.Availability.References {
		if ref.Provenance.Kind == "system" {
			return "", "enterprise source controls are unverified"
		}
		if ref.Provenance.Kind == "plugin" {
			return "", "plugin settings govern whole plugins; isolated skill controls are unsupported"
		}
		if ref.NativeName != "" {
			// Native settings select the discovered directory, not the displayed command.
			if filepath.Base(s.references[ref.Reference].DiscoveredPath) != ref.NativeName {
				return "", "native settings selector differs from the command name; controls are unsupported"
			}
			if name != "" && name != ref.NativeName {
				return "", "shared references have different native names"
			}
			name = ref.NativeName
		}
	}
	if name == "" {
		return "", "native command name is unknown"
	}
	for _, other := range s.sources {
		if other.ID == source.ID {
			continue
		}
		for _, ref := range other.Availability.References {
			if ref.NativeName == name || (ref.NativeName != "" && ref.Provenance.Kind != "plugin" && filepath.Base(s.references[ref.Reference].DiscoveredPath) == name) {
				return "", "settings selector or command name would affect an independent source"
			}
		}
	}
	return name, ""
}
func previewClaudeChange(ctx context.Context, s *Snapshot, intent ChangeIntent) (preview *ChangePreview) {
	defer func() {
		if s != nil && preview != nil {
			preview.view.InspectionProject = s.request.Project
		}
	}()
	if s == nil || s.request.Agent != "claude-code" {
		return rejectedChange(intent, "unsupported", "explicit --agent claude-code is required")
	}
	if intent.Scope != ControlGlobal && intent.Scope != ControlProject {
		return rejectedChange(intent, "unsupported", "choose explicit global or project scope")
	}
	if intent.Choice != ControlEnable && intent.Choice != ControlDisable && intent.Choice != ControlInherit {
		return rejectedChange(intent, "refused", "choose enable, disable or inherit")
	}
	if intent.Choice == ControlInherit && intent.Scope != ControlProject {
		return rejectedChange(intent, "unsupported", "inherit removes only a project override")
	}
	if intent.Scope == ControlProject && !s.request.Project.Selected {
		return rejectedChange(intent, "unsupported", "select the exact launch project with --project")
	}
	fresh, err := Build(ctx, cloneRequest(s.request))
	if err != nil {
		return rejectedChange(intent, "failed", "could not reinspect original context")
	}
	return previewClaudeSnapshot(fresh, intent, true)
}
func previewClaudeSnapshot(fresh *Snapshot, intent ChangeIntent, pin bool) *ChangePreview {
	s := fresh
	if intent.Scope == ControlProject && !s.request.Project.Selected {
		return rejectedChange(intent, "unsupported", "select the exact launch project with --project")
	}
	source, found := fresh.Source(intent.Source)
	if !found {
		return rejectedChange(intent, "refused", "selected source is no longer reachable")
	}
	name, reason := claudeControlName(fresh, source)
	if reason != "" {
		return rejectedChange(intent, "unsupported", reason)
	}
	manifest := filepath.Join(source.Directory, "SKILL.md")
	fact, err := observeChangePath(manifest)
	if err != nil || fact.Links != 1 {
		return rejectedChange(intent, "refused", "manifest identity is unsafe or hardlinked")
	}
	inputs := readClaudeInputs(fresh.request)
	if inputs.uncertain || inputs.safe || inputs.strict {
		return rejectedChange(intent, "refused", "native policy is uncertain or restricts ordinary customization")
	}
	path := filepath.Join(fresh.request.claudeHome, "settings.json")
	if intent.Scope == ControlProject {
		path = filepath.Join(fresh.request.Project.Path, ".claude", "settings.json")
	}
	// A broader write must never pretend to overcome a narrower saved choice.
	for _, layer := range inputs.scope.ConfigLayers {
		if layer.State == "absent" {
			continue
		}
		raw, e := nativeBytes(layer.Path)
		if e != nil {
			return rejectedChange(intent, "refused", "settings cannot be pinned")
		}
		if _, e = parseSettingsJSON(raw); e != nil {
			return rejectedChange(intent, "refused", e.Error())
		}
		higher := layer.Scope == "managed" || layer.Scope == "project-local" || (intent.Scope == ControlGlobal && layer.Scope == "project")
		if higher {
			var doc map[string]json.RawMessage
			_ = json.Unmarshal(raw, &doc)
			var overrides map[string]string
			_ = json.Unmarshal(doc["skillOverrides"], &overrides)
			if _, ok := overrides[name]; ok && intent.Choice != ControlInherit {
				return rejectedChange(intent, "refused", "higher-priority "+layer.Scope+" override controls this name; no broader fallback")
			}
			if layer.Scope == "managed" {
				return rejectedChange(intent, "unsupported", "managed settings enforcement is unverified")
			}
		}
	}
	dest, err := inspectChangeDestinationPath(path)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	if intent.Scope == ControlProject {
		global, err := inspectChangeDestinationPath(filepath.Join(fresh.request.claudeHome, "settings.json"))
		if err != nil {
			return rejectedChange(intent, "refused", "global settings cannot be separated safely: "+err.Error())
		}
		if dest.Parent.Identity.Device == global.Parent.Identity.Device && dest.Parent.Identity.Inode == global.Parent.Identity.Inode && strings.EqualFold(filepath.Base(dest.CreateDirectory), filepath.Base(global.CreateDirectory)) {
			return rejectedChange(intent, "refused", "project destination aliases global settings; isolated project controls are unsupported")
		}
	}
	post, change, write, err := patchClaudeChoice(dest.Config.Bytes, name, intent.Choice)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	if !pin {
		return &ChangePreview{view: ChangeView{State: "ready", Intent: intent}}
	}
	facts, deps, err := changeObservations(fresh, dest)
	if err != nil {
		return rejectedChange(intent, "refused", "relevant inputs cannot be pinned: "+err.Error())
	}
	plan := &changePlan{request: cloneRequest(s.request), intent: intent, postimage: post, destination: dest, snapshotFacts: facts, dependencies: deps}
	plan.reviewPatch = func(raw []byte, _ bool) ([]byte, []SettingChange, bool, error) {
		post, effect, write, err := patchClaudeChoice(raw, name, intent.Choice)
		return post, []SettingChange{effect}, write, err
	}
	data, _ := json.Marshal(struct {
		Version, Home, ClaudeHome string
		Intent                    ChangeIntent
		Facts                     []byte
		Dependencies              []changeFact
		Destination               changeDestination
		Post                      []byte
	}{"claude-controls-v1", plan.request.home, plan.request.claudeHome, intent, facts, deps, dest, post})
	sum := sha256.Sum256(data)
	plan.digest = "cl1:" + hex.EncodeToString(sum[:])
	view := ChangeView{State: "ready", Reason: "preview only; no settings changed", Intent: intent, SourcePath: source.Directory, ManifestPath: manifest, ConfigPath: path, InspectionProject: fresh.request.Project, Changes: []SettingChange{change}, KnownReferences: fresh.References(source.ID), Before: source.Availability, NoWriteNeeded: !write, CreateDirectory: dest.CreateDirectory, Confirmation: plan.digest, Notes: []string{fmt.Sprintf("Validated ordinary skillOverrides on/off and inheritance: Claude Code 2.1.282 and 2.1.300; command name %q.", name), "Only the selected settings key changes. Skill content and other applications remain unchanged.", "Saved choice and contextual effective availability are distinct. Existing session adoption is unverified; a fresh launch can load the choice. Whether restart is necessary is unknown.", "Enable restores native defaults; frontmatter and Skill permission restrictions remain in effect."}}
	return &ChangePreview{view: view, plan: plan}
}

func applyClaudeCapabilities(s *Snapshot) {
	for _, id := range s.order {
		source := s.sources[id]
		source.Availability.Capabilities = nil
		for _, entry := range []struct {
			op     string
			scope  ControlScope
			choice ControlChoice
		}{{"enable", ControlGlobal, ControlEnable}, {"disable", ControlGlobal, ControlDisable}, {"project-override", ControlProject, ControlDisable}, {"inherit", ControlProject, ControlInherit}} {
			intent := ChangeIntent{Source: id, Application: "claude-code", Scope: entry.scope, Choice: entry.choice}
			preview := previewClaudeSnapshot(s, intent, false)
			state, kind := "unsupported", EvidenceUnknown
			if preview.view.State == "ready" {
				state, kind = "supported", EvidenceInferred
				preview.view.Reason = "validated ordinary name-based settings control; preview and explicit confirmation required"
			}
			source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: entry.op, Scope: string(entry.scope), State: state, Evidence: claudeEvidence(kind, "native-settings-control", source.Directory, "skillOverrides", preview.view.Reason)})
		}
		plugin := source.Ownership.State == "managed"
		for _, ref := range source.Availability.References {
			plugin = plugin || ref.Provenance.Kind == "plugin"
		}
		if plugin {
			source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: "disable", Scope: "plugin", State: "unsupported", Evidence: claudeEvidence(EvidenceUnknown, "plugin-scope", source.Directory, "", "whole plugins are broader than an isolated skill action")})
		}
		source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: "delete", Scope: "source", State: "unsupported", Evidence: claudeEvidence(EvidenceUnknown, "no-delete", source.Directory, "", "deletion is not supported")})
		s.sources[id] = source
	}
}
