package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
)

const hermesProjectControlReason = "Hermes 0.20.0 has no validated project override or inheritance setting; project configuration is not consumed. No global fallback or settings change."

func previewHermesChange(ctx context.Context, s *Snapshot, intent ChangeIntent) *ChangePreview {
	if s == nil || s.request.Agent != "hermes" {
		return rejectedChange(intent, "unsupported", "explicit --agent hermes is required")
	}
	if intent.Scope != ControlGlobal || intent.Choice == ControlInherit {
		p := rejectedChange(intent, "unsupported", hermesProjectControlReason)
		p.view.InspectionProject = s.request.Project
		return p
	}
	if intent.Choice != ControlEnable && intent.Choice != ControlDisable {
		return rejectedChange(intent, "refused", "choose enable or disable")
	}
	fresh, err := Build(ctx, cloneRequest(s.request))
	if err != nil {
		return rejectedChange(intent, "failed", "could not reinspect original Hermes context")
	}
	return previewHermesSnapshot(fresh, intent)
}

type hermesControlFacts struct {
	full          map[SourceID]hermesMetadataObservation
	selectors     map[string]map[SourceID]bool
	unknownNames  map[SourceID]bool
	nativeReasons map[SourceID]string
}

func indexHermesControls(s *Snapshot, index, full map[SourceID]hermesMetadataObservation) hermesControlFacts {
	facts := hermesControlFacts{full: full, selectors: make(map[string]map[SourceID]bool), unknownNames: make(map[SourceID]bool), nativeReasons: make(map[SourceID]string)}
	for id, source := range s.sources {
		for _, ref := range source.Availability.References {
			if ref.Participation == ParticipationInspection {
				continue
			}
			if !index[id].nameKnown() || !full[id].nameKnown() {
				facts.unknownNames[id] = true
				continue
			}
			path := s.references[ref.Reference].DiscoveredPath
			for _, name := range []string{hermesReferenceName(index[id].metadata, path), hermesReferenceName(full[id].metadata, path), filepath.Base(path)} {
				if facts.selectors[name] == nil {
					facts.selectors[name] = make(map[SourceID]bool)
				}
				facts.selectors[name][id] = true
			}
		}
	}
	return facts
}

func hermesControlName(s *Snapshot, source Source, inputs hermesInputs) (string, string) {
	if !s.Complete() || inputs.uncertain || !inputs.platformKnown || inputs.policyEnvUnknown["HERMES_MANAGED_DIR"] || inputs.pluginFailure != nil {
		return "", "native discovery, profile or platform policy is incomplete; name-based effects cannot be bounded"
	}
	if !inputs.controlConfigKnown {
		return "", "native profile agent normalization or complete YAML construction is unverified"
	}
	skills := hermesMap(inputs.configData, "skills")
	policies := []struct {
		setting string
		value   any
	}{{"skills.disabled", skills["disabled"]}}
	if inputs.platform != "" {
		platforms := hermesMap(skills, "platform_disabled")
		policies = append(policies, struct {
			setting string
			value   any
		}{"skills.platform_disabled." + inputs.platform, platforms[inputs.platform]})
	}
	for _, policy := range policies {
		if inputs.managedConfigData != nil {
			if !inputs.managedYAML.known(policy.setting) {
				return "", "managed disabled policy interpretation is unverified"
			}
			managed := inputs.managedConfigData
			keys := strings.Split(policy.setting, ".")
			for i, key := range keys {
				value, present := managed[key]
				if !present {
					break
				}
				next, mapping := value.(map[string]any)
				if !mapping || i == len(keys)-1 {
					return "", "managed policy overrides global or selected messaging-platform disabled settings for native explicit loading; isolated profile controls are unsupported"
				}
				managed = next
			}
		}
		if !inputs.configYAML.known(policy.setting) {
			return "", "native disabled policy interpretation is unverified"
		}
		if policy.value == nil {
			continue
		}
		entries, ok := policy.value.([]any)
		if !ok {
			return "", "global and selected messaging-platform disabled policies require literal lists or null; scalar/mapping membership differs across native consumers"
		}
		for _, entry := range entries {
			text, ok := entry.(string)
			if !ok || hermesConfigReference.MatchString(text) {
				return "", "native disabled policy requires literal string list entries without environment references; credentials and providers are not evaluated"
			}
		}
	}
	if source.Identity.Certainty != "proven" || source.Ownership.State == "managed" {
		return "", "native source identity or managed registration is unverified"
	}
	name := source.Availability.NativeName
	full := s.hermesControls.full[source.ID]
	if name == "" || source.Availability.NativeNameEvidence == nil || source.Availability.NativeNameEvidence.Kind == EvidenceUnknown || !full.nameKnown() || strings.TrimFunc(name, hermesDotenvSpace) != name || strings.ContainsAny(name, ":\r\n") || hermesConfigReference.MatchString(name) {
		return "", "native name interpretation is unverified"
	}
	for _, ref := range source.Availability.References {
		if ref.Provenance.Kind == "plugin" {
			return "", "plugin-level controls would broaden this isolated source action"
		}
		actual := s.references[ref.Reference]
		rootNative := ref.Evidence.Rule == "hermes-profile" || ref.Evidence.Rule == "hermes-external" || ref.Participation == ParticipationEligible
		if rootNative && (hermesReferenceName(full.metadata, actual.DiscoveredPath) != name || filepath.Base(actual.DiscoveredPath) != name) {
			return "", "directory, full metadata and index selectors differ; isolated disable is unverified"
		}
	}
	if raw, _ := full.metadata["name"].(string); len([]rune(raw)) > 64 {
		return "", "native index truncates this name while other consumers use its full name; isolated controls are unsupported"
	}
	unknown := s.hermesControls.unknownNames
	if len(unknown) > 1 || (len(unknown) == 1 && !unknown[source.ID]) {
		return "", "another native source has uncertain name scope"
	}
	owners := s.hermesControls.selectors[name]
	if len(owners) > 1 || (len(owners) == 1 && !owners[source.ID]) {
		return "", "name-based setting would affect an additional independent source"
	}
	return name, ""
}

func previewHermesSnapshot(s *Snapshot, intent ChangeIntent) *ChangePreview {
	if intent.Scope != ControlGlobal || intent.Choice == ControlInherit {
		return rejectedChange(intent, "unsupported", hermesProjectControlReason)
	}
	source, found := s.Source(intent.Source)
	if !found {
		return rejectedChange(intent, "refused", "selected source is no longer reachable")
	}
	inputs := readHermesInputs(s.request)
	name, reason := hermesControlName(s, source, inputs)
	if reason != "" {
		return rejectedChange(intent, "unsupported", reason)
	}
	if !changeWriterEligible(source) {
		return rejectedChange(intent, "unsupported", "an ordinary proven native Hermes source is required")
	}
	fact, err := observeChangePath(filepath.Join(source.Directory, "SKILL.md"))
	if err != nil || fact.Links != 1 {
		return rejectedChange(intent, "refused", "manifest identity is unsafe or hardlinked")
	}
	dest, err := inspectChangeDestinationPath(inputs.config)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	post, change, write, err := patchHermesChoice(dest.Config.Bytes, name, intent.Choice)
	if err != nil {
		return rejectedChange(intent, "refused", err.Error())
	}
	facts, deps, err := changeObservations(s, dest)
	if err != nil {
		return rejectedChange(intent, "refused", "relevant Hermes inputs cannot be pinned: "+err.Error())
	}
	plan := &changePlan{request: cloneRequest(s.request), intent: intent, postimage: post, destination: dest, snapshotFacts: facts, dependencies: deps}
	plan.reviewPatch = func(raw []byte, _ bool) ([]byte, []SettingChange, bool, error) {
		post, effect, write, err := patchHermesChoice(raw, name, intent.Choice)
		return post, []SettingChange{effect}, write, err
	}
	data, _ := json.Marshal(struct {
		Version, Home string
		Intent        ChangeIntent
		Facts         []byte
		Dependencies  []changeFact
		Destination   changeDestination
		Post          []byte
	}{"hermes-controls-v1", inputs.home, intent, facts, deps, dest, post})
	sum := sha256.Sum256(data)
	plan.digest = "h1:" + hex.EncodeToString(sum[:])
	view := ChangeView{State: "ready", Reason: "preview only; no settings changed", Intent: intent, SourcePath: source.Directory, ManifestPath: fact.Path, ConfigPath: inputs.config, InspectionProject: s.request.Project, Changes: []SettingChange{change}, KnownReferences: s.References(source.ID), Before: source.Availability, NoWriteNeeded: !write, CreateDirectory: dest.CreateDirectory, Confirmation: plan.digest, Notes: []string{
		"Validated Hermes 0.20.0 global control is confined to this active profile: " + inputs.home,
		"Only skills.disabled name membership changes. Other profiles, messaging-platform rules, skill content and other applications remain unchanged.",
		"Enable removes this global denial. A messaging-platform denial, excluded source or unknown policy can still prevent effective availability.",
		"Persisted configuration is verified separately from running-session use. Context-local platform/home overrides and caches may differ; session adoption and reload/restart requirements are unknown.",
	}}
	return &ChangePreview{view: view, plan: plan}
}

func applyHermesCapabilities(s *Snapshot, inputs hermesInputs) {
	destination, destinationErr := inspectChangeDestinationPath(inputs.config)
	for _, id := range s.order {
		source := s.sources[id]
		name, reason := hermesControlName(s, source, inputs)
		s.hermesControls.nativeReasons[id] = reason
		if reason == "" && !changeWriterEligible(source) {
			reason = "an ordinary proven native Hermes source is required"
		}
		if reason == "" && destinationErr != nil {
			reason = destinationErr.Error()
		}
		if reason == "" {
			fact, err := observeChangePath(filepath.Join(source.Directory, "SKILL.md"))
			if err != nil || fact.Links != 1 {
				reason = "manifest identity is unsafe or hardlinked"
			}
		}
		source.Availability.Capabilities = nil
		for _, choice := range []ControlChoice{ControlEnable, ControlDisable} {
			state, kind, why := "unsupported", EvidenceUnknown, reason
			if why == "" {
				if _, _, _, err := patchHermesChoice(destination.Config.Bytes, name, choice); err != nil {
					why = err.Error()
				} else {
					state, kind, why = "supported", EvidenceInferred, "validated active-profile skills.disabled control; preview and confirmation required"
				}
			}
			source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: string(choice), Scope: string(ControlGlobal), State: state, Evidence: hermesEvidence(kind, "native-settings-control", inputs.config, "skills.disabled", why)})
		}
		for _, op := range []string{"project-override", "inherit"} {
			source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: op, Scope: string(ControlProject), State: "unsupported", Evidence: hermesEvidence(EvidenceUnknown, "project-controls", inputs.config, "", hermesProjectControlReason)})
		}
		source.Availability.Capabilities = append(source.Availability.Capabilities, Capability{Operation: "delete", Scope: "source", State: "unsupported", Evidence: hermesEvidence(EvidenceUnknown, "no-delete", source.Directory, "", "deletion is not supported")})
		s.sources[id] = source
	}
}
