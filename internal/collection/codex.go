package collection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

func appendNativeRoot(request *Request, path string, context ReferenceContext, origin string) {
	request.Roots = append(request.Roots, DiscoveryRoot{ID: RootID(stableID("root_codex", origin, path)), Path: path, Context: context, Origin: origin, Optional: true})
}
func buildCodex(ctx context.Context, request Request) (*Snapshot, error) {
	original := cloneRequest(request)
	ancestors, boundary, err := nativeProjectAncestors(request.Project)
	inputs := readCodexInputs(request, ancestors, boundary)
	if err != nil {
		inputs.problem(request.Project.Path, "selected project cannot be resolved; project coverage is unknown")
	}
	appendNativeRoot(&request, filepath.Join(request.home, ".agents", "skills"), ContextGlobal, "codex-user")
	appendNativeRoot(&request, filepath.Join(request.codexHome, "skills"), ContextGlobal, "codex-compatibility-user")
	appendNativeRoot(&request, filepath.Join(request.codexHome, "skills", ".system"), ContextSystem, "codex-builtins")
	appendNativeRoot(&request, "/etc/codex/skills", ContextSystem, "codex-system")
	for i := len(ancestors) - 1; i >= 0; i-- {
		appendNativeRoot(&request, filepath.Join(ancestors[i], ".agents", "skills"), ContextProject, "codex-project")
		appendNativeRoot(&request, filepath.Join(ancestors[i], ".codex", "skills"), ContextProject, "codex-compatibility-project")
	}
	plugins := discoverCodexPlugins(&request, &inputs, ancestors)
	snapshot, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	snapshot.request = original // Refresh resolves native roots and registrations again.
	snapshot.scope.Agent = &inputs.scope
	for i := range snapshot.scope.Roots {
		root := &snapshot.scope.Roots[i]
		if root.Origin != "" {
			evidence := codexEvidence(EvidenceObserved, "filesystem-root", root.Requested, "", "read-only scanner observed root state: "+string(root.State))
			root.Evidence = &evidence
		}
	}

	roots := make(map[RootID]DiscoveryRoot, len(request.Roots))
	for _, root := range request.Roots {
		roots[root.ID] = root
	}
	for _, id := range snapshot.order {
		snapshot.sources[id] = explainCodexSource(snapshot, id, roots, plugins, &inputs)
	}
	applyCodexCoexistence(snapshot)
	applyNativeProblems(snapshot, inputs.problems, roots)

	return snapshot, nil
}
func codexInvocation(source Source, inputs *codexInputs) SessionExplanation {
	path := filepath.Join(source.Directory, "agents", "openai.yaml")
	result := SessionExplanation{State: "implicit-allowed", Evidence: codexEvidence(EvidenceInferred, "invocation-default", path, "policy.allow_implicit_invocation", "documented default is true; model invocation enforcement was not tested")}
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		return result
	}
	data, _, decodeErr := decodeMetadata(raw)
	policy, policyOK := data["policy"].(map[string]any)
	_, policyPresent := data["policy"]
	value, valuePresent := policy["allow_implicit_invocation"]
	allowed, valueOK := value.(bool)
	if err != nil || decodeErr != nil || (policyPresent && !policyOK) || (valuePresent && !valueOK) {
		problem := makeProblem(ProblemNativeInput, path, "invocation policy is unreadable or invalid YAML", nil, true)
		problem.SourceIDs = []SourceID{source.ID}
		inputs.problems = append(inputs.problems, problem)
		result.State = "unknown"
		result.Evidence = codexEvidence(EvidenceUnknown, "invocation-policy", path, "policy.allow_implicit_invocation", "policy cannot be reliably interpreted")
		return result
	}
	if valuePresent && !allowed {
		result.State = "explicit-only"
		result.Evidence = codexEvidence(EvidenceInferred, "invocation-policy", path, "policy.allow_implicit_invocation=false", "documented explicit-only policy; skills/list enabled does not describe invocation")
	}

	return result
}

// Native discovery is recursive through ordinary containers and directory aliases,
// but stops at bundles. The generic explicit-root contract remains direct children.
func (builder *discoveryBuilder) discoverNativeChildren(ctx context.Context, root DiscoveryRoot, path string, ancestors map[string]bool, count *int) {
	if err := ctx.Err(); err != nil {
		return
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}
	if ancestors[resolved] {
		problem := makeProblem(ProblemSymlinkCycle, path, "native container traversal forms a cycle", nil, true)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		return
	}
	if *count >= maxResourceEntries {
		problem := makeProblem(ProblemResourceLimit, path, "native discovery exceeds directory limit", nil, true)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		return
	}
	*count++
	ancestors[resolved] = true
	defer delete(ancestors, resolved)
	entries, err := os.ReadDir(path)
	if err != nil {
		problem := makeProblem(ProblemRootUnreadable, path, "native discovery container could not be listed", err, true)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || (!entry.IsDir() && entry.Type()&os.ModeSymlink == 0) {
			continue
		}
		child := filepath.Join(path, entry.Name())
		relative, _ := filepath.Rel(root.Path, child)
		present, presenceErr := hasManifest(child)
		if present || presenceErr != nil {
			builder.discoverCandidate(ctx, root, child, relative)
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if _, err := filepath.EvalSymlinks(child); err != nil {
				builder.discoverCandidate(ctx, root, child, relative)
				continue
			}
		}
		info, err := os.Stat(child)
		if err == nil && info.IsDir() {
			builder.discoverNativeChildren(ctx, root, child, ancestors, count)
		}
	}
}

func explainCodexSource(snapshot *Snapshot, id SourceID, roots map[RootID]DiscoveryRoot, plugins map[string]codexPluginFact, inputs *codexInputs) Source {
	source := snapshot.sources[id]
	explanation := AvailabilityExplanation{State: AvailabilityUnknown, Reason: "no eligible native reference is established"}
	evidence := codexEvidence(EvidenceUnknown, "native-participation", source.Directory, "", explanation.Reason)
	explanation.Evidence = &evidence
	managedPlugin, managed := managedCodexSource(source.Directory, plugins)
	if managed {
		source.Ownership = EvidenceState{State: "managed", Reason: "canonical bundle lies within a versioned plugin cache; plugin manager owns content"}
	}
	permission := inputs.permission(canonicalManifest(filepath.Join(source.Directory, "SKILL.md")))
	invocation := SessionExplanation{State: "unknown", Evidence: codexEvidence(EvidenceUnknown, "inspection-only", source.Directory, "", "native invocation participation is not established")}
	for _, rid := range source.References {
		if roots[snapshot.references[rid].Root].Origin != "" {
			invocation = codexInvocation(source, inputs)
			break
		}
	}
	explanation.Invocation = &invocation
	eligible := false
	excluded := false
	for _, rid := range source.References {
		ref := snapshot.references[rid]
		root := roots[ref.Root]
		fact := ReferenceAvailability{Reference: rid, Participation: ParticipationInspection, Permission: permission, Provenance: Provenance{Kind: "custom", Evidence: codexEvidence(EvidenceUnknown, "ownership", ref.DiscoveredPath, "", "inspection scope does not establish native ownership")}, Evidence: codexEvidence(EvidenceUnknown, "inspection-only", ref.DiscoveredPath, "", "an arbitrary viewer root is not a native runtime extra root")}
		if root.Origin != "" {
			fact.Participation = ParticipationEligible
			fact.Provenance.Kind = string(root.Context)
			fact.Provenance.Evidence = codexEvidence(EvidenceInferred, "native-root-provenance", root.Path, "", root.Origin+" is discovery scope, not proof of content ownership")
			fact.Evidence = codexEvidence(EvidenceInferred, "native-root", root.Path, "", "root participates in the selected local Codex context")
			if root.Context == ContextSystem {
				fact.Provenance.Kind = "system"
				source.Ownership = EvidenceState{State: "managed", Reason: "Codex or administrator managed system source"}
			}
			if plugin, cacheRoute := plugins[root.Path]; cacheRoute && root.Origin == "codex-plugin-cache" {
				fact.Participation = plugin.participation
				fact.Evidence = plugin.activation
			}
			if source.Manifest.Status != ManifestParsed || source.Description == "" || source.Manifest.IsSymlink || (strings.HasPrefix(filepath.Base(ref.DiscoveredPath), ".") && root.Origin != "codex-builtins") {
				fact.Participation = ParticipationUnknown
				fact.Evidence = codexEvidence(EvidenceUnknown, "manifest-eligibility", source.Manifest.Path, "", "native acceptance of this manifest form was not validated")
			}
		}

		if root.Origin != "" && (source.Manifest.IsSymlink || (source.Manifest.Status == ManifestParsed && source.Description == "")) {
			fact.Participation = ParticipationExcluded
			fact.Evidence = codexEvidence(EvidenceInferred, "native-manifest-rejection", source.Manifest.Path, "", "Codex 0.155.1 omits manifest file symlinks and rejects missing description; generic parsing remains visible")
		}
		// A source reached through an inspection alias also has the proven native reference facts.
		if fact.Participation == ParticipationInspection {
			fact.Permission = PermissionExplanation{State: PermissionUnknown, Evidence: codexEvidence(EvidenceUnknown, "inspection-only", ref.DiscoveredPath, "", "no native reference participation established")}
		}
		if managed {
			fact.Provenance = managedPlugin.provenance
		}
		eligible = eligible || fact.Participation == ParticipationEligible
		excluded = excluded || fact.Participation == ParticipationExcluded
		explanation.References = append(explanation.References, fact)
	}
	if eligible {
		explanation.State = AvailabilityAvailable
		explanation.Reason = "eligible native reference; locally configured permission allowed"
		if permission.State == PermissionDenied {
			explanation.State = AvailabilityDisabled
			explanation.Reason = "effective manifest selector disables this source"
		} else if permission.State == PermissionUnknown || invocation.State == "unknown" {
			explanation.State = AvailabilityUnknown
			explanation.Reason = "permission or invocation policy cannot be reliably interpreted"
		} else if invocation.State == "explicit-only" {
			explanation.State = AvailabilityExplicitOnly
			explanation.Reason = "allowed by local configuration; invocation policy requires explicit selection"
		}
		for _, layer := range permission.Layers {
			if layer.Scope == "project" && layer.State == "effective" {
				explanation.Reason += "; project configuration inference only, native skills/list retains startup/user choices and inference-session enforcement is unverified"
				break
			}
		}
		*explanation.Evidence = codexEvidence(EvidenceInferred, "configured-availability", permission.Evidence.InputPath, permission.Evidence.Setting, explanation.Reason)
		if explanation.State == AvailabilityUnknown {
			explanation.Evidence.Kind = EvidenceUnknown
		}
	} else if excluded {
		explanation.State = AvailabilityUnavailable
		explanation.Reason = "native reference is excluded by eligibility or managed activation rules"
		*explanation.Evidence = codexEvidence(EvidenceInferred, "excluded-native-reference", source.Directory, "", explanation.Reason)
	}
	explanation.Precedence = &PrecedenceExplanation{State: PrecedenceUnique, Evidence: codexEvidence(EvidenceInferred, "same-name-coexistence", source.Directory, "", "Codex retains independent same-name definitions; there is no name-based winner")}
	if !eligible {
		explanation.Precedence.State = PrecedenceUnknown
		explanation.Precedence.Evidence = codexEvidence(EvidenceUnknown, "native-precedence", source.Directory, "", "source has no eligible native occurrence")
	}
	if eligible {
		name, nameEvidence, problem := codexNativeName(source)
		explanation.NativeName = name
		explanation.NativeNameEvidence = &nameEvidence
		if problem != nil {
			inputs.problems = append(inputs.problems, *problem)
			explanation.Precedence.State = PrecedenceUnknown
			explanation.Precedence.Evidence = nameEvidence
		}
	}

	explanation.Session = &SessionExplanation{State: "unknown", Evidence: codexEvidence(EvidenceUnknown, "session-not-observed", "", "", "no existing inference session was inspected")}
	explanation.Reload = &SessionExplanation{State: "unknown", Evidence: codexEvidence(EvidenceUnknown, "session-reload", "", "", "refresh rereads local files only; fresh load or restart may be required for a session")}
	controlScope := "source"
	for _, fact := range explanation.References {
		if fact.Provenance.Kind == "plugin" {
			controlScope = "plugin"
			break
		}
		if fact.Provenance.Kind == "system" {
			controlScope = "system"
		} else if controlScope == "source" && fact.Provenance.Kind == "project" {
			controlScope = "project"
		} else if controlScope == "source" && fact.Provenance.Kind == "global" {
			controlScope = "global"
		}
	}
	for _, operation := range []string{"enable", "disable", "project-override", "delete"} {
		scope := controlScope
		if operation == "project-override" {
			scope = "project"
		}
		state := "unsupported"
		reason := "this operation or scope has no validated writer"
		evidenceKind := EvidenceUnknown
		rule := "unsupported-control"
		if operation == "project-override" {
			reason = codexProjectControlReason
			evidenceKind = EvidenceObserved
			rule = "native-project-control-unavailable"
		}
		if operation == "enable" || operation == "disable" {
			scope = "global"
			source.Availability = explanation
			if changeWriterEligible(source) {
				state = "supported"
				evidenceKind = EvidenceInferred
				rule = "validated-global-manifest-control"
				reason = "Codex 0.155.1 global canonical-manifest setting is validated; preview must validate current configuration, source identity and safe write topology"
			} else {
				reason = "eligible native manifest participation is required; no name, project override or plugin-level fallback"
			}
		}
		explanation.Capabilities = append(explanation.Capabilities, Capability{Operation: operation, Scope: scope, State: state, Evidence: codexEvidence(evidenceKind, rule, source.Directory, "skills.config", reason)})
	}
	source.Availability = explanation
	return source
}

func applyCodexCoexistence(snapshot *Snapshot) {
	groups := make(map[string][]SourceID)
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		if source.Availability.NativeName != "" {
			groups[source.Availability.NativeName] = append(groups[source.Availability.NativeName], id)
		}
	}
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		for _, other := range groups[source.Availability.NativeName] {
			if other != id {
				source.Availability.Precedence.State = PrecedenceCoexisting
				source.Availability.Precedence.RelatedSources = append(source.Availability.Precedence.RelatedSources, other)
			}
		}
		snapshot.sources[id] = source
	}
}
