package collection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type changePlan struct {
	reviewCheck        func(context.Context) error
	reviewCheckCreated func(context.Context, changeFact) error
	committedFact      *changeFact
	createdFact        *changeFact
	// reviewPatch reuses this validated native recipe without writing a destination.
	reviewPatch            func([]byte, bool) ([]byte, []SettingChange, bool, error)
	request                Request
	intent                 ChangeIntent
	digest                 string
	postimage              []byte
	destination            changeDestination
	snapshotFacts          []byte
	dependencies           []changeFact
	fault                  func(string) error
	opencodeGlobalExpected *opencodeContextFacts
}

func changeObservations(s *Snapshot, destination changeDestination) ([]byte, []changeFact, error) {
	view := s.Query(Filters{Context: FilterAll}, QuerySources)
	raw, err := json.Marshal(view)
	if err != nil {
		return nil, nil, err
	}
	ordered := changeObservationPaths(view, destination)
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

// changeObservationPaths describes the native read set without changing the
// native writer's strict observation/error policy.
func changeObservationPaths(view View, destination changeDestination) []string {
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
		if source.Directory != source.Manifest.ResolvedPath {
			add(filepath.Join(source.Directory, "agents", "openai.yaml"))
		}
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
	return ordered
}
func (p *changePlan) recheck(ctx context.Context) error {
	if p.reviewCheck != nil {
		if err := p.reviewCheck(ctx); err != nil {
			return err
		}
	}
	fresh := (&Snapshot{request: p.request}).PreviewChange(ctx, p.intent)
	if fresh.plan == nil || fresh.plan.digest != p.digest {
		return errors.New("preview is stale; source, configuration, context or file identity changed; request a new preview")
	}
	return nil
}
func (p *changePlan) inspectResult(out ChangeOutcome, parent changeIdentity) ChangeOutcome {
	current, err := observeChangePath(p.destination.Path)
	parentFact, parentErr := observeChangePath(filepath.Dir(p.destination.Path))
	parentMatches := parentErr == nil && parentFact.Resolved != ""
	if parentMatches {
		resolvedParent, err := observeChangePath(parentFact.Resolved)
		parentMatches = err == nil && resolvedParent.Identity.Inode == parent.Inode && resolvedParent.Identity.Device == parent.Device && resolvedParent.Identity.BirthSec == parent.BirthSec && resolvedParent.Identity.BirthNano == parent.BirthNano
	}
	metadataMatches := !p.destination.Config.Exists || (current.Identity.Mode == p.destination.Config.Identity.Mode && current.Identity.UID == p.destination.Config.Identity.UID && current.Identity.GID == p.destination.Config.Identity.GID && sameChangeFacts(current.Attributes, p.destination.Config.Attributes))
	expectedExists := p.destination.Config.Exists || len(p.postimage) > 0
	if err == nil && current.Exists == expectedExists && current.Symlink == "" && bytes.Equal(current.Bytes, p.postimage) && metadataMatches && (parentMatches || (len(p.postimage) == 0 && !p.destination.Config.Exists && p.destination.CreateDirectory != "")) {
		out.Persistence = "verified"
		out.PersistedChoice = p.intent.Choice
		if p.intent.Application != "claude-code" && p.intent.Application != "hermes" && p.intent.Application != "pi" && p.intent.Application != "opencode" && p.intent.Application != "gemini" {
			// Resource choice persistence alone does not establish native invocation permission for these agents.
			if p.intent.Choice == ControlEnable {
				out.PersistedPermission = PermissionAllowed
			} else {
				out.PersistedPermission = PermissionDenied
			}
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
		if p.intent.Application == "opencode" && out.Persistence == "verified" {
			out.PersistedPermission = opencodeSavedPermission(current.Bytes, effective.NativeName)
			if p.intent.Choice != ControlInherit {
				switch out.PersistedPermission {
				case PermissionAllowed, PermissionAsk:
					out.PersistedChoice = ControlEnable
				case PermissionDenied:
					out.PersistedChoice = ControlDisable
				default:
					out.PersistedChoice = ""
				}
			}
		}
		if p.intent.Application == "gemini" && out.Persistence == "verified" {
			d, e := readGeminiDisabledDocument(current.Bytes)
			if e == nil {
				out.PersistedPermission = PermissionAllowed
				if d.contains(effective.NativeName) {
					out.PersistedPermission = PermissionDenied
				}
				if p.intent.Choice == ControlInherit {
					out.PersistedPermission = ""
				}
			}
		}
		out.Effective = &effective
		resultReason := effective.Reason
		nativeAchievement, nativeReason := "", ""
		if p.intent.Application == "gemini" {
			recipe, e := geminiControlRecipe(fresh, p.intent)
			if e == nil {
				_, e = geminiDestination(fresh, recipe)
			}
			if e != nil {
				nativeAchievement, nativeReason = "unknown", "native Gemini control certainty: "+e.Error()
			}
		}
		if p.intent.Application == "gemini" && p.intent.Scope == ControlGlobal {
			g, e := geminiGlobalSnapshot(context.Background(), fresh)
			if e != nil {
				nativeAchievement, nativeReason = "unknown", "global context reinspection failed"
			} else if _, e = geminiControlRecipe(g, p.intent); e != nil {
				nativeAchievement, nativeReason = "unknown", "global native Gemini control certainty: "+e.Error()
			} else if p.intent.Choice != ControlInherit && !geminiChoiceAchieved(p.intent.Choice, g.geminiControls.permission(effective.NativeName).State) {
				nativeAchievement, nativeReason = "unmet", "global settings layer prevents requested result"
			}
		}
		if p.intent.Application == "opencode" {
			if p.intent.Scope == ControlGlobal {
				nativeAchievement, nativeReason = p.opencodeGlobalResult(context.Background(), fresh)
			} else {
				facts := newOpencodeControlFacts(context.Background(), fresh)
				if _, err := facts.destination(opencodeControlPlan{ConfigPath: p.destination.Path, Scope: p.intent.Scope}); err != nil {
					nativeAchievement, nativeReason = "unknown", "native destination safety: "+err.Error()
				}
			}
		}
		if reason, known := fresh.hermesControls.nativeReasons[source.ID]; p.intent.Application == "hermes" && (!known || reason != "") {
			out.Achievement = "unknown"
			if reason == "" {
				reason = "native control certainty was not established"
			}
			resultReason = "native Hermes control certainty: " + reason
		} else if nativeAchievement != "" {
			out.Achievement = nativeAchievement
			resultReason = nativeReason
		} else if p.intent.Choice == ControlInherit && out.Persistence == "verified" {
			out.Achievement = "achieved"
		} else if p.intent.Choice == ControlDisable && effective.State == AvailabilityDisabled {
			out.Achievement = "achieved"
		} else if p.intent.Choice == ControlEnable && (effective.State == AvailabilityAvailable || effective.State == AvailabilityExplicitOnly || effective.State == AvailabilityAsk) {
			out.Achievement = "achieved"
		} else if effective.State == AvailabilityUnknown {
			out.Achievement = "unknown"
		} else {
			out.Achievement = "unmet"
		}
		if out.Achievement != "achieved" {
			out.Reason += "; effective result " + out.Achievement + ": " + resultReason
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
