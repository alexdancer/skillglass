package collection

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
)

var claudeSkillName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func appendClaudeRoot(request *Request, path string, context ReferenceContext, origin string) {
	request.Roots = append(request.Roots, DiscoveryRoot{ID: RootID(stableID("root_claude", origin, path)), Path: path, Context: context, Origin: origin, Optional: true})
}
func buildClaude(ctx context.Context, request Request) (*Snapshot, error) {
	original := cloneRequest(request)
	ancestors, ancestorErr := claudeProjectAncestors(request.Project)
	inputs := readClaudeInputs(request)
	if ancestorErr != nil {
		inputs.problem(request.Project.Path, "selected project cannot be resolved; project discovery is unknown")
	}
	appendClaudeRoot(&request, filepath.Join(request.claudeHome, "skills"), ContextGlobal, "claude-personal")
	appendClaudeRoot(&request, "/Library/Application Support/ClaudeCode/.claude/skills", ContextSystem, "claude-enterprise")
	for _, ancestor := range ancestors {
		appendClaudeRoot(&request, filepath.Join(ancestor, ".claude", "skills"), ContextProject, "claude-project")
	}
	plugins := discoverClaudePlugins(&request, &inputs)
	snapshot, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	snapshot.request = original
	snapshot.scope.Agent = &inputs.scope
	roots := map[RootID]DiscoveryRoot{}
	for _, root := range request.Roots {
		roots[root.ID] = root
	}
	for i := range snapshot.scope.Roots {
		root := &snapshot.scope.Roots[i]
		if root.Origin != "" {
			e := claudeEvidence(EvidenceObserved, "filesystem-root", root.Requested, "", "read-only scanner observed root state: "+string(root.State))
			root.Evidence = &e
		}
	}
	for _, id := range snapshot.order {
		snapshot.sources[id] = explainClaudeSource(snapshot, id, roots, plugins, &inputs)
	}
	applyClaudePrecedence(snapshot, roots, ancestors)
	applyNativeProblems(snapshot, inputs.problems, roots)
	return snapshot, nil
}
func explainClaudeSource(snapshot *Snapshot, id SourceID, roots map[RootID]DiscoveryRoot, plugins map[string]claudePluginFact, inputs *claudeInputs) Source {
	source := snapshot.sources[id]
	e := claudeEvidence(EvidenceUnknown, "inspection-only", source.Directory, "", "an arbitrary viewer root does not establish native loading")
	result := AvailabilityExplanation{State: AvailabilityUnknown, Reason: e.Reason, Evidence: &e}
	invocation := SessionExplanation{State: "unknown", Evidence: e}
	eligible, excluded := false, false
	var activationDiagnostic, unresolvedDiagnostic *Evidence
	nativeName := ""
	nativeNames := map[string]bool{}
	pluginFact, managed := managedClaudeSource(source.Directory, plugins)
	if managed {
		source.Ownership = EvidenceState{State: "managed", Reason: "canonical bundle lies within Claude plugin manager content"}
	}
	for _, rid := range source.References {
		ref := snapshot.references[rid]
		root := roots[ref.Root]
		fact := ReferenceAvailability{Reference: rid, Participation: ParticipationInspection, Evidence: e, Permission: PermissionExplanation{State: PermissionUnknown, Evidence: e}, Provenance: Provenance{Kind: "custom", Evidence: e}}
		if root.Origin != "" {
			name, _ := source.Manifest.Metadata["name"].(string)
			if name == "" {
				name = filepath.Base(ref.DiscoveredPath)
			}
			if nativeName == "" {
				nativeName = name
			}
			fact.Participation = ParticipationEligible
			fact.Evidence = claudeEvidence(EvidenceInferred, "startup-skill-root", root.Path, "", "Claude 2.1.282 reads direct child bundles, not recursive group containers")
			fact.Provenance = Provenance{Kind: string(root.Context), Evidence: claudeEvidence(EvidenceInferred, "native-root-provenance", root.Path, "", root.Origin+" is discovery scope, not proof of user ownership")}
			isPlugin := root.Origin == "claude-plugin-cache" || root.Origin == "claude-plugin-local"
			if isPlugin {
				fact.Participation = plugins[root.Path].participation
				fact.Evidence = plugins[root.Path].activation
				nativeName = plugins[root.Path].name + ":" + name
			}
			fact.NativeName = name
			if isPlugin {
				fact.NativeName = plugins[root.Path].name + ":" + name
			}
			fact.Permission = inputs.permission(fact.NativeName)
			if fact.Participation != ParticipationExcluded && (!claudeSkillName.MatchString(name) || (!isPlugin && strings.HasPrefix(filepath.Base(ref.DiscoveredPath), "."))) {
				fact.Participation = ParticipationUnknown
				fact.Evidence = claudeEvidence(EvidenceUnknown, "unverified-native-manifest-form", source.Manifest.Path, "name", "native loading of noncanonical command names or hidden ordinary bundles was not validated")
			}
			if fact.Participation != ParticipationExcluded && source.Manifest.Status != ManifestParsed {
				fact.Participation = ParticipationUnknown
				fact.Evidence = claudeEvidence(EvidenceUnknown, "manifest-eligibility", source.Manifest.Path, "", "native loading of unreadable or malformed frontmatter is not established")
			}
			folder := strings.ToLower(filepath.Base(ref.DiscoveredPath))
			if !isPlugin && (folder == "synced" || folder == "anthropic-skills" || strings.HasPrefix(folder, "anthropic-skills:") || strings.ToLower(name) == "anthropic-skills" || strings.HasPrefix(strings.ToLower(name), "anthropic-skills:")) {
				fact.Participation = ParticipationExcluded
				fact.Evidence = claudeEvidence(EvidenceInferred, "reserved-skill-name", ref.DiscoveredPath, "", "reserved account namespace is not an ordinary authored skill")
			}
			if root.Context == ContextSystem {
				source.Ownership = EvidenceState{State: "managed", Reason: "administrator managed enterprise skill"}
			}
			if inputs.safe || (inputs.strict && !isPlugin) {
				fact.Participation = ParticipationExcluded
				fact.Evidence = claudeEvidence(EvidenceInferred, "customization-policy", ref.DiscoveredPath, "strictPluginOnlyCustomization / CLAUDE_CODE_SAFE_MODE", "local customization is excluded by the observed policy")
			}
		}
		if managed {
			fact.Provenance = pluginFact.provenance
			if fact.Participation != ParticipationExcluded && root.Origin != "claude-plugin-cache" && root.Origin != "claude-plugin-local" && root.Origin != "" {
				fact.Participation = ParticipationUnknown
				fact.Evidence = claudeEvidence(EvidenceUnknown, "managed-alias-unverified", ref.DiscoveredPath, "", "an ordinary alias into managed plugin content does not establish independently validated activation")
			}
		}
		if fact.Participation == ParticipationEligible && fact.NativeName != "" {
			nativeNames[fact.NativeName] = true
		}
		if fact.Evidence.Rule == "plugin-manifest-unverified" || fact.Evidence.Rule == "local-marketplace-unverified" {
			diagnostic := fact.Evidence
			activationDiagnostic = &diagnostic
		}
		if fact.Participation == ParticipationUnknown {
			diagnostic := fact.Evidence
			unresolvedDiagnostic = &diagnostic
		}
		eligible = eligible || fact.Participation == ParticipationEligible
		excluded = excluded || fact.Participation == ParticipationExcluded
		result.References = append(result.References, fact)
	}
	for _, fact := range result.References {
		if fact.Participation == ParticipationEligible && fact.NativeName != "" {
			nativeName = fact.NativeName
			break
		}
	}
	if nativeName != "" {
		result.NativeName = nativeName
		nameEvidence := claudeEvidence(EvidenceInferred, "skill-command-name", source.Manifest.Path, "name", "frontmatter name is used when set; otherwise the discovered folder name; plugin commands are namespaced")
		result.NativeNameEvidence = &nameEvidence
	}
	if !eligible {
		if activationDiagnostic != nil && (!excluded || unresolvedDiagnostic != nil) {
			e = *activationDiagnostic
		} else if unresolvedDiagnostic != nil {
			e = *unresolvedDiagnostic
		}
		result.Reason = e.Reason
		invocation.Evidence = e
	}
	if eligible {
		result.State = AvailabilityAvailable
		result.Reason = "eligible native startup reference with automatic and explicit invocation enabled"
		invocation = SessionExplanation{State: "automatic-and-user", Evidence: claudeEvidence(EvidenceInferred, "invocation-frontmatter", source.Manifest.Path, "", "defaults permit model and user invocation; user-invocable=false hides only the user command")}
		disable, disableOK := claudeInvocationBoolean(source.Manifest.Metadata["disable-model-invocation"])
		user, userOK := claudeInvocationBoolean(source.Manifest.Metadata["user-invocable"])
		_, userPresent := source.Manifest.Metadata["user-invocable"]
		userKnown := userOK || !userPresent
		if !userOK {
			user = true
		}
		if disable {
			result.State = AvailabilityExplicitOnly
			invocation.State = "explicit-only"
			result.Reason = "disable-model-invocation=true removes model invocation; the explicit command remains"
		}
		if !user {
			invocation.State = "model-only"
			result.Reason = "user-invocable=false hides the user command; model invocation remains enabled"
			if disable {
				result.State = AvailabilityUnavailable
				invocation.State = "no-invoker"
				result.Reason = "both model and user invocation are excluded by frontmatter"
			}
		}
		if _, present := source.Manifest.Metadata["disable-model-invocation"]; present && !disableOK {
			result.State = AvailabilityUnknown
			invocation.State = "unknown"
			result.Reason = "disable-model-invocation is not a boolean"
		}
		if !userKnown {
			result.State = AvailabilityUnknown
			invocation.State = "unknown"
			result.Reason = "user-invocable is not a boolean"
		}
		controlling := claudeEvidence(EvidenceInferred, "invocation-frontmatter", source.Manifest.Path, "disable-model-invocation / user-invocable", result.Reason)
		ordinary := true
		for _, fact := range result.References {
			if fact.Provenance.Kind == "plugin" {
				ordinary = false
			}
		}
		if setting, present := inputs.overrides[nativeName]; present && ordinary {
			controlling = claudeEvidence(EvidenceInferred, "skill-visibility-override", setting.path, "skillOverrides."+nativeName+"="+setting.value, "local overrides project, which overrides user; settings visibility is distinct from Skill tool permissions")
			switch setting.value {
			case "off":
				result.State = AvailabilityDisabled
				invocation.State = "disabled"
				result.Reason = "skillOverrides=off hides and refuses both model and explicit invocation"
			case "user-invocable-only":
				result.State = AvailabilityExplicitOnly
				invocation.State = "explicit-only"
				result.Reason = "settings hide the skill from Claude while preserving explicit invocation"
				if !userKnown {
					result.State = AvailabilityUnknown
					invocation.State = "unknown"
					result.Reason = "settings hide model invocation but user invocation is unverified"
				} else if !user {
					result.State = AvailabilityUnavailable
					invocation.State = "no-invoker"
					result.Reason = "settings hide model invocation and frontmatter hides user invocation"
				}
			case "name-only":
				if result.State == AvailabilityAvailable {
					invocation.State = "name-only"
					result.Reason = "settings list only the name to Claude; explicit invocation remains available"
				}
			}
		}
		permission := inputs.permission(nativeName)
		if permission.State == PermissionUnknown || inputs.uncertain {
			result.State = AvailabilityUnknown
			invocation.State = "unknown"
			result.Reason = "native settings could not be reliably interpreted"
			controlling = permission.Evidence
		} else if result.State != AvailabilityDisabled && permission.State == PermissionDenied {
			result.State = AvailabilityExplicitOnly
			invocation.State = "explicit-only"
			result.Reason = "Skill tool deny restricts model invocation; explicit slash invocation is not full disable"
			if !userKnown {
				result.State = AvailabilityUnknown
				invocation.State = "unknown"
				result.Reason = "model Skill tool is denied but user invocation is unverified"
			} else if !user {
				result.State = AvailabilityUnavailable
				invocation.State = "no-invoker"
				result.Reason = "model Skill tool is denied and user invocation is hidden"
			}
			controlling = permission.Evidence
		} else if result.State == AvailabilityAvailable && permission.Evidence.Rule == "skill-tool-ask" {
			result.State = AvailabilityAsk
			result.Reason = "model Skill tool invocation requires permission; explicit invocation remains available"
			controlling = permission.Evidence
		}
		if result.State == AvailabilityUnknown {
			controlling.Kind = EvidenceUnknown
			controlling.Reason = result.Reason
		}
		result.Evidence = &controlling
		invocation.Evidence = controlling
	} else if excluded && unresolvedDiagnostic == nil {
		result.State = AvailabilityUnavailable
		result.Reason = "native reference excluded by reserved namespace, inactive plugin, or customization policy"
		e = claudeEvidence(EvidenceInferred, "excluded-native-reference", source.Directory, "", result.Reason)
	}
	if eligible && len(nativeNames) > 1 {
		result.State = AvailabilityUnknown
		result.Reason = "shared unnamed or namespaced references have different native command names; native deduplication winner was not validated"
		e = claudeEvidence(EvidenceUnknown, "shared-native-name", source.Directory, "name", result.Reason)
		result.Evidence = &e
		result.NativeNameEvidence = &e
		invocation = SessionExplanation{State: "unknown", Evidence: e}
	}
	result.Invocation = &invocation
	result.Precedence = &PrecedenceExplanation{State: PrecedenceUnknown, Evidence: claudeEvidence(EvidenceUnknown, "native-name-precedence", source.Directory, "", "no eligible native reference established")}
	result.Session = &SessionExplanation{State: "unknown", Evidence: claudeEvidence(EvidenceUnknown, "session-not-observed", "", "", "no existing inference session was inspected")}
	result.Reload = &SessionExplanation{State: "unknown", Evidence: claudeEvidence(EvidenceUnknown, "session-reload", "", "", "refresh reads local files only; native startup validation does not establish a running session refresh")}
	scope := "source"
	if managed {
		scope = "plugin"
	}
	for _, operation := range []string{"enable", "disable", "project-override", "delete"} {
		capScope := scope
		if operation == "project-override" {
			capScope = "project"
		}
		result.Capabilities = append(result.Capabilities, Capability{Operation: operation, Scope: capScope, State: "unsupported", Evidence: claudeEvidence(EvidenceUnknown, "read-only-adapter", source.Directory, "", "no writer is enabled; name-based settings affect independent copies and plugin settings govern whole plugins")})
	}
	source.Availability = result
	return source
}
func applyClaudePrecedence(snapshot *Snapshot, roots map[RootID]DiscoveryRoot, ancestors []string) {
	type contender struct {
		id   SourceID
		rank int
	}
	groups := map[string][]contender{}
	for _, id := range snapshot.order {
		nameRanks := map[string]int{}
		for _, fact := range snapshot.sources[id].Availability.References {
			if fact.Participation != ParticipationEligible || fact.NativeName == "" {
				continue
			}
			root := roots[snapshot.references[fact.Reference].Root]
			current := 1
			switch root.Origin {
			case "claude-enterprise":
				current = 100000
			case "claude-personal":
				current = 90000
			case "claude-project":
				for i, ancestor := range ancestors {
					if root.Path == filepath.Join(ancestor, ".claude", "skills") {
						current = 80000 - i
					}
				}
			}
			if current > nameRanks[fact.NativeName] {
				nameRanks[fact.NativeName] = current
			}
		}
		for name, rank := range nameRanks {
			groups[name] = append(groups[name], contender{id, rank})
		}
	}
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		if source.Availability.NativeNameEvidence != nil && source.Availability.NativeNameEvidence.Kind == EvidenceUnknown {
			precedence := source.Availability.Precedence
			precedence.Evidence.Reason = source.Availability.NativeNameEvidence.Reason
			for _, fact := range source.Availability.References {
				if fact.Participation != ParticipationEligible {
					continue
				}
				for _, other := range groups[fact.NativeName] {
					if other.id != id {
						precedence.RelatedSources = appendUniqueSourceIDs(precedence.RelatedSources, other.id)
					}
				}
			}
			snapshot.sources[id] = source
			continue
		}
		name := source.Availability.NativeName
		best, possibleBest, ownRank, topCount := 0, 0, 0, 0
		winner := SourceID("")
		for _, other := range groups[name] {
			if other.id == id {
				ownRank = other.rank
			}
			if snapshot.sources[other.id].Availability.NativeNameEvidence.Kind == EvidenceUnknown {
				if other.rank > possibleBest {
					possibleBest = other.rank
				}
				continue
			}
			if other.rank > best {
				best, winner, topCount = other.rank, other.id, 1
			} else if other.rank == best {
				topCount++
			}
		}
		if ownRank == 0 {
			continue
		}
		precedence := PrecedenceExplanation{State: PrecedenceUnique, Winner: winner, Evidence: claudeEvidence(EvidenceInferred, "startup-name-precedence", source.Directory, "name="+name, "Claude 2.1.282 native catalog: enterprise (documented) > personal > nearest project > ancestor project; independent copies remain distinct")}
		for _, other := range groups[name] {
			if other.id != id {
				precedence.RelatedSources = append(precedence.RelatedSources, other.id)
			}
		}
		if topCount > 1 || possibleBest >= best {
			precedence.State = PrecedenceUnknown
			precedence.Winner = ""
			precedence.Evidence.Kind = EvidenceUnknown
			precedence.Evidence.Reason = "equal-priority independent definitions; native winner within one root was not validated"
			if possibleBest >= best {
				precedence.Evidence.Reason = "an eligible shared-name contender may win this command; native name selection is unverified"
			}
			if ownRank == best {
				if source.Availability.State != AvailabilityDisabled && source.Availability.State != AvailabilityUnavailable {
					source.Availability.State = AvailabilityUnknown
					source.Availability.Reason = precedence.Evidence.Reason
					source.Availability.Evidence = &precedence.Evidence
				}
			} else {
				precedence.Evidence.Reason += "; higher-priority eligible definitions establish this lower source is shadowed"
			}
		}
		if ownRank < best {
			source.Availability.State = AvailabilityShadowed
			source.Availability.Reason = "higher-priority independent definition wins the native command name"
			if precedence.Winner == "" {
				source.Availability.Reason = "a higher-priority eligible definition necessarily shadows this command; winning source identity is unverified"
			}
			e := precedence.Evidence
			e.Kind = EvidenceInferred
			e.Reason = source.Availability.Reason
			source.Availability.Evidence = &e
		}
		source.Availability.Precedence = &precedence
		snapshot.sources[id] = source
	}
}

func claudeInvocationBoolean(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(typed) {
		case "true", "yes", "on", "1":
			return true, true
		case "false", "no", "off", "0":
			return false, true
		}
	case int:
		if typed == 0 || typed == 1 {
			return typed == 1, true
		}
	case int64:
		if typed == 0 || typed == 1 {
			return typed == 1, true
		}
	case uint64:
		if typed == 0 || typed == 1 {
			return typed == 1, true
		}
	}
	return false, false
}
