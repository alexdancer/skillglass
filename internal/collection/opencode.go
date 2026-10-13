package collection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func appendOpencodeRoot(request *Request, path string, context ReferenceContext, origin string, optional bool) {
	request.Roots = append(request.Roots, DiscoveryRoot{ID: RootID(stableID("root_opencode", origin, path)), Path: path, Context: context, Origin: origin, Optional: optional})
}
func buildOpencode(ctx context.Context, request Request) (*Snapshot, error) {
	original := cloneRequest(request)
	ancestors, _, err := nativeProjectAncestors(request.Project)
	in := readOpencodeInputs(request, ancestors)
	if err != nil {
		in.problem(request.Project.Path, "selected project cannot be resolved; native project coverage is unknown")
	}
	for _, folder := range []string{".claude", ".agents"} {
		origin := "opencode-external"
		if opencodeBool("OPENCODE_DISABLE_EXTERNAL_SKILLS") || (folder == ".claude" && (opencodeBool("OPENCODE_DISABLE_CLAUDE_CODE") || opencodeBool("OPENCODE_DISABLE_CLAUDE_CODE_SKILLS"))) {
			origin = "opencode-external-disabled"
		}
		appendOpencodeRoot(&request, filepath.Join(request.home, folder, "skills"), ContextGlobal, origin, true)
		for _, ancestor := range ancestors {
			appendOpencodeRoot(&request, filepath.Join(ancestor, folder, "skills"), ContextProject, origin, true)
		}
	}
	for _, dir := range in.dirs {
		context := ContextGlobal
		for _, ancestor := range ancestors {
			if dir == filepath.Join(ancestor, ".opencode") {
				context = ContextProject
			}
		}
		for _, folder := range []string{"skill", "skills"} {
			appendOpencodeRoot(&request, filepath.Join(dir, folder), context, "opencode-native-directory", true)
		}
	}
	if skills, ok := in.config.get("skills").(*opencodeObject); ok {
		if paths, ok := skills.get("paths").([]any); ok {
			for _, raw := range paths {
				path, ok := raw.(string)
				if !ok || path == "" {
					in.problem("skills.paths", "configured skill paths must be nonempty strings")
					continue
				}
				if !request.Project.Selected && !filepath.IsAbs(path) && !strings.HasPrefix(path, "~/") {
					in.problem("skills.paths", "relative native skill path requires an explicitly selected project")
					continue
				}
				appendOpencodeRoot(&request, opencodePath(path, in.project, in.home), ContextCustom, "opencode-configured-path", false)
			}
		} else if skills.get("paths") != nil {
			in.problem("skills.paths", "configured paths is not an array")
		}
	}
	appendOpencodeRoot(&request, filepath.Join(in.cache, "skills"), ContextManaged, "opencode-remote-cache", true)
	snapshot, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	snapshot.request = original
	roots := map[RootID]DiscoveryRoot{}
	for _, root := range request.Roots {
		roots[root.ID] = root
	}
	for i := range snapshot.scope.Roots {
		root := &snapshot.scope.Roots[i]
		if root.Origin != "" {
			e := opencodeEvidence(EvidenceObserved, "filesystem-root", root.Requested, "", "read-only scanner observed "+string(root.State))
			root.Evidence = &e
		}
	}
	for _, id := range snapshot.order {
		snapshot.sources[id] = explainOpencodeSource(snapshot, id, roots, in)
	}
	applyOpencodeConflicts(snapshot)
	snapshot.scope.Agent = &in.scope
	applyNativeProblems(snapshot, in.problems, roots)
	return snapshot, nil
}

func explainOpencodeSource(snapshot *Snapshot, id SourceID, roots map[RootID]DiscoveryRoot, in *opencodeInputs) Source {
	source := snapshot.sources[id]
	nativeName := source.Manifest.Name
	e := opencodeEvidence(EvidenceUnknown, "native-participation", source.Directory, "", "no eligible native route established")
	availability := AvailabilityExplanation{State: AvailabilityUnknown, Reason: e.Reason, Evidence: &e, NativeName: nativeName}
	nameEvidence := opencodeEvidence(EvidenceObserved, "frontmatter-name", source.Manifest.Path, "name", "native dispatch uses frontmatter name independently of folder and source identity")
	if source.Manifest.Status != ManifestParsed {
		nameEvidence = opencodeEvidence(EvidenceUnknown, "manifest-parser", source.Manifest.Path, "name", "native dispatch name could not be established from the generic parse")
	}
	availability.NativeNameEvidence = &nameEvidence
	permission := in.permission(nativeName)
	eligible, excluded := false, false
	for _, rid := range source.References {
		ref := snapshot.references[rid]
		root := roots[ref.Root]
		fact := ReferenceAvailability{Reference: rid, Participation: ParticipationInspection, Permission: PermissionExplanation{State: PermissionUnknown, Evidence: opencodeEvidence(EvidenceUnknown, "inspection-only", ref.DiscoveredPath, "", "explicit viewer roots do not become native skill paths")}, Evidence: opencodeEvidence(EvidenceUnknown, "inspection-only", ref.DiscoveredPath, "", "arbitrary inspection root does not establish native participation"), Provenance: Provenance{Kind: "custom", Evidence: opencodeEvidence(EvidenceUnknown, "ownership", ref.DiscoveredPath, "", "a path does not prove ownership")}}
		if root.Origin != "" {
			fact.Participation = ParticipationEligible
			fact.Permission = permission
			fact.Evidence = opencodeEvidence(EvidenceInferred, "native-skill-root", root.Path, "", root.Origin+" participates in the selected local discovery context")
			fact.Provenance = Provenance{Kind: string(root.Context), Evidence: opencodeEvidence(EvidenceInferred, "native-root-provenance", root.Path, "", "discovery origin does not prove content ownership")}
			if root.Origin == "opencode-external-disabled" {
				fact.Participation = ParticipationExcluded
				fact.Evidence = opencodeEvidence(EvidenceInferred, "external-skill-flag", root.Path, "OPENCODE_DISABLE_EXTERNAL_SKILLS / OPENCODE_DISABLE_CLAUDE_CODE_SKILLS", "external scan is disabled by supplied environment")
			}
			if root.Origin == "opencode-remote-cache" {
				fact.Participation = ParticipationExcluded
				fact.Evidence = opencodeEvidence(EvidenceInferred, "inactive-cache", root.Path, "skills.urls", "cache alone is not a fresh successful native index activation")
				if in.uncertain {
					fact.Participation = ParticipationUnknown
					fact.Evidence = opencodeEvidence(EvidenceUnknown, "unverified-cache-activation", root.Path, "skills.urls / plugin hooks", "native configuration or remote/plugin activation is unverified; cache presence proves neither active nor inactive state")
				}
			}
			if source.Manifest.Status != ManifestParsed {
				fact.Participation = ParticipationUnknown
				fact.Evidence = opencodeEvidence(EvidenceUnknown, "manifest-parser", source.Manifest.Path, "", "generic parser did not establish native acceptance; native parser differences remain unverified")
			}
		}
		canonicalCache := in.cache
		if resolved, err := filepath.EvalSymlinks(in.cache); err == nil {
			canonicalCache = resolved
		}
		if pathWithin(canonicalCache, source.Directory) {
			source.Ownership = EvidenceState{State: "managed", Reason: "canonical bundle is inside OpenCode's cache; native manager owns cache content"}
			fact.Provenance = Provenance{Kind: "managed-cache", Evidence: opencodeEvidence(EvidenceObserved, "cache-ownership", in.cache, "", "no plugin identity or activation is inferred from a cache folder")}
		}
		eligible = eligible || fact.Participation == ParticipationEligible
		excluded = excluded || fact.Participation == ParticipationExcluded
		availability.References = append(availability.References, fact)
	}
	if eligible {
		availability.State = AvailabilityAvailable
		availability.Reason = "eligible local definition; default agent's configured skill permission allows loading"
		if permission.State == PermissionAsk {
			availability.State = AvailabilityAsk
			availability.Reason = "default agent's configured skill permission asks before loading"
		} else if permission.State == PermissionDenied {
			availability.State = AvailabilityDisabled
			availability.Reason = "default agent's configured skill permission denies loading and hides the skill"
		} else if permission.State == PermissionUnknown {
			availability.State = AvailabilityUnknown
			availability.Reason = "native configuration or dynamic override is unverified"
		} else if _, present := source.Manifest.Metadata["description"]; !present {
			availability.State = AvailabilityExplicitOnly
			availability.Reason = "native name is loadable, but missing description omits model-facing guidance in 1.17.15"
		}
		*availability.Evidence = opencodeEvidence(permission.Evidence.Kind, "configured-availability", permission.Evidence.InputPath, permission.Evidence.Setting, availability.Reason)
	} else if excluded {
		availability.State = AvailabilityUnavailable
		availability.Reason = "all known native routes are excluded; no active discovery established"
		*availability.Evidence = opencodeEvidence(EvidenceInferred, "excluded-native-route", source.Directory, "", availability.Reason)
	}
	availability.Precedence = &PrecedenceExplanation{State: PrecedenceUnique, Evidence: opencodeEvidence(EvidenceInferred, "unique-local-name", source.Directory, "name", "one known local source with this native name; remote/session sources remain unknown")}
	invocation := SessionExplanation{State: "model-guidance", Evidence: opencodeEvidence(EvidenceInferred, "v1-guidance", source.Manifest.Path, "description", "described definitions enter model guidance unless permission denies; autoinvoke metadata is not implemented by v1 1.17.15")}
	if _, present := source.Manifest.Metadata["description"]; !present {
		invocation.State = "explicit-only"
		invocation.Evidence.Reason = "v1 guidance omits definitions without description; exact-name loading remains possible"
	}
	if !eligible {
		invocation.State = "unknown"
		invocation.Evidence.Kind = EvidenceUnknown
	}
	availability.Invocation = &invocation
	availability.Session = &SessionExplanation{State: "unknown", Evidence: opencodeEvidence(EvidenceUnknown, "running-session", "", "", "session agent selection, approvals and actual tool enforcement were not observed")}
	availability.Reload = &SessionExplanation{State: "unknown", Evidence: opencodeEvidence(EvidenceUnknown, "session-reload", "", "", "viewer refresh only rereads files; a fresh native process was used for verification")}
	for _, operation := range []string{"enable", "disable", "project-override", "delete"} {
		availability.Capabilities = append(availability.Capabilities, Capability{Operation: operation, Scope: "selected-context", State: "unsupported", Evidence: opencodeEvidence(EvidenceUnknown, "read-only-ticket", source.Directory, "", "no writer validated or enabled; plugin hooks and native manager operations remain outside this adapter")})
	}
	source.Availability = availability
	return source
}
func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func applyOpencodeConflicts(snapshot *Snapshot) {
	groups := map[string][]SourceID{}
	for _, id := range snapshot.order {
		source := snapshot.sources[id]
		eligible := false
		for _, fact := range source.Availability.References {
			eligible = eligible || fact.Participation == ParticipationEligible
		}
		if eligible {
			groups[source.Availability.NativeName] = append(groups[source.Availability.NativeName], id)
		}
	}
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		for _, id := range ids {
			source := snapshot.sources[id]
			p := source.Availability.Precedence
			p.State = PrecedenceUnknown
			p.Evidence = opencodeEvidence(EvidenceUnknown, "concurrent-name-overwrite", source.Manifest.Path, "name", "OpenCode 1.17.15 loads matches concurrently and overwrites by name; no deterministic winner or shadowing can be asserted")
			for _, other := range ids {
				if other != id {
					p.RelatedSources = append(p.RelatedSources, other)
				}
			}
			source.Availability.State = AvailabilityUnknown
			source.Availability.Reason = p.Evidence.Reason
			*source.Availability.Evidence = p.Evidence
			snapshot.sources[id] = source
		}
	}
}

// OpenCode's glob visits nested manifests, follows aliases and includes dot
// directories only for external compatibility roots. Generic roots stay direct.
func (builder *discoveryBuilder) discoverOpencodeChildren(ctx context.Context, root DiscoveryRoot, path string, ancestors map[string]bool, count *int) {
	if ctx.Err() != nil {
		return
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		relative, _ := filepath.Rel(root.Path, path)
		builder.discoverCandidate(ctx, root, path, relative)
		return
	}
	if ancestors[resolved] || *count >= maxResourceEntries {
		code := ProblemSymlinkCycle
		reason := "native traversal forms a directory cycle"
		if *count >= maxResourceEntries {
			code = ProblemResourceLimit
			reason = "native discovery exceeds directory limit"
		}
		p := makeProblem(code, path, reason, nil, true)
		p.RootIDs = []RootID{root.ID}
		builder.addProblem(p)
		return
	}
	*count++
	ancestors[resolved] = true
	defer delete(ancestors, resolved)
	if present, presenceErr := hasManifest(path); present || presenceErr != nil {
		relative, _ := filepath.Rel(root.Path, path)
		builder.discoverCandidate(ctx, root, path, relative)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		p := makeProblem(ProblemRootUnreadable, path, "native container cannot be listed", err, true)
		p.RootIDs = []RootID{root.ID}
		builder.addProblem(p)
		return
	}
	dot := strings.HasPrefix(root.Origin, "opencode-external")
	for _, entry := range entries {
		if !dot && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		child := filepath.Join(path, entry.Name())
		info, err := os.Stat(child)
		if err == nil && !info.IsDir() {
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		builder.discoverOpencodeChildren(ctx, root, child, ancestors, count)
	}
}
