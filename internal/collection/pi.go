package collection

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (inputs *piInputs) addRoot(request *Request, path string, context ReferenceContext, origin, mode string, fact piRootFact, optional bool) {
	id := RootID("root_pi_" + stableID(origin, path, fact.path, fact.provenance.PluginID))
	for _, r := range request.Roots {
		if r.ID == id {
			return
		}
	}
	info, err := os.Stat(path)
	file := err == nil && !info.IsDir()
	request.Roots = append(request.Roots, DiscoveryRoot{ID: id, Path: path, Context: context, Origin: origin, Optional: optional, piMode: mode, piFile: file})
	inputs.facts[id] = fact
}
func buildPi(ctx context.Context, request Request) (*Snapshot, error) {
	original := cloneRequest(request)
	agentDir, err := piAgentDir(request.home)
	if err != nil {
		return nil, err
	}
	inputs := piInputs{facts: make(map[RootID]piRootFact), scope: AgentScope{Application: "pi", ValidatedVersion: piValidatedVersion, VersionEvidence: "docs/verification/ale-33.md", Evidence: piEvidence(EvidenceInferred, "version-specific-adapter", "", "", "rules validated with installed native Pi resource manager and skill loader; current running sessions are not inspected"), Gaps: []string{"Running CLI --skill, --no-skills, trust overrides, custom SDK loaders and extension-provided resources are not observed.", "Complex minimatch braces/extglobs, package manifest glob expansion and ignore-file rules remain unknown; bounded discovery reports those cases.", "Legacy global npm installations and remote package version satisfaction are not inferred from a cache folder.", "Within one native root, discovery order follows the recorded macOS filesystem ordering; other platforms remain unvalidated.", "Standalone Markdown resources can be read; bundle comparison has incomplete coverage and deletion is unsupported.", "All writers are unavailable in this read-only ticket."}}}
	inputs.global = inputs.settings(filepath.Join(agentDir, "settings.json"), "global")
	inputs.trust = inputs.projectTrust(request.Project, agentDir)
	ancestors, err := piAncestors(request.Project)
	if err != nil {
		inputs.problem(request.Project.Path, "Pi selected project cannot be resolved")
	}
	inputs.project = piSettings{}
	if request.Project.Selected {
		inputs.project = inputs.settings(filepath.Join(request.Project.Path, ".pi", "settings.json"), "project")
	}
	// Trust gating is a distinct fact from parsing project settings for inspection.
	trustEvidence := piEvidence(EvidenceInferred, "project-trust", filepath.Join(agentDir, "trust.json"), "nearest ancestor decision, then defaultProjectTrust", "project participation is "+inputs.trust+"; session and extension trust decisions are unknown")
	if request.Project.Selected {
		inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, PermissionLayer{Path: trustEvidence.InputPath, Scope: "project", State: inputs.trust, Reason: trustEvidence.Reason, Evidence: trustEvidence})
	}
	inputs.addPackages(&request, agentDir)
	addConfigured := func(settings piSettings, base string, context ReferenceContext, rank int, trust string) {
		for _, entry := range settings.skills {
			if piPattern(entry) {
				continue
			}
			inputs.addRoot(&request, piPath(entry, base, request.home), context, "pi-configured-"+string(context), "pi", piRootFact{rank: rank, base: base, path: settings.path, patterns: settings.skills, trust: trust, uncertain: settings.uncertain, provenance: Provenance{Kind: string(context), Evidence: piEvidence(EvidenceObserved, "configured-resource", settings.path, "skills", "resource root selected by Pi settings")}}, false)
		}
	}
	if request.Project.Selected {
		addConfigured(inputs.project, filepath.Join(request.Project.Path, ".pi"), ContextProject, 0, inputs.trust)
	}
	addConfigured(inputs.global, agentDir, ContextGlobal, 2, "allowed")
	auto := func(path, base string, context ReferenceContext, mode string, rank int, settings piSettings, trust string) {
		inputs.addRoot(&request, path, context, "pi-auto-"+string(context)+"-"+mode, mode, piRootFact{rank: rank, base: base, path: settings.path, patterns: settings.skills, trust: trust, uncertain: settings.uncertain, provenance: Provenance{Kind: string(context), Evidence: piEvidence(EvidenceInferred, "native-root-provenance", path, "", "native discovery scope does not establish ownership")}}, true)
	}
	if request.Project.Selected {
		auto(filepath.Join(request.Project.Path, ".pi", "skills"), filepath.Join(request.Project.Path, ".pi"), ContextProject, "pi", 1, inputs.project, inputs.trust)
	}
	for _, ancestor := range ancestors {
		path := filepath.Join(ancestor, ".agents", "skills")
		if path != filepath.Join(request.home, ".agents", "skills") {
			auto(path, filepath.Join(ancestor, ".agents"), ContextProject, "agents", 1, inputs.project, inputs.trust)
		}
	}
	auto(filepath.Join(agentDir, "skills"), agentDir, ContextGlobal, "pi", 3, inputs.global, "allowed")
	auto(filepath.Join(request.home, ".agents", "skills"), filepath.Join(request.home, ".agents"), ContextGlobal, "agents", 3, inputs.global, "allowed")
	// Managed storage is inspectable, but registration is required for participation.
	for _, base := range []string{agentDir, filepath.Join(request.Project.Path, ".pi")} {
		if !request.Project.Selected && base != agentDir {
			continue
		}
		for _, kind := range []string{"git", "npm/node_modules"} {
			inputs.addRoot(&request, filepath.Join(base, kind), ContextManaged, "pi-inactive-cache", "pi", piRootFact{rank: 9, trust: "inactive", provenance: Provenance{Kind: "plugin", Evidence: piEvidence(EvidenceObserved, "managed-storage", base, "", "cache storage alone is not an active package registration")}}, true)
		}
	}
	snapshot, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	snapshot.request = original
	snapshot.scope.Agent = &inputs.scope
	roots := make(map[RootID]DiscoveryRoot)
	for _, r := range request.Roots {
		roots[r.ID] = r
	}
	for i := range snapshot.scope.Roots {
		root := &snapshot.scope.Roots[i]
		if root.Origin != "" {
			e := piEvidence(EvidenceObserved, "filesystem-root", root.Requested, "", "root observed as "+string(root.State))
			root.Evidence = &e
		}
	}
	suppressed := inputs.occurrenceDuplicates(snapshot)
	ranks := make(map[SourceID]int)
	for _, id := range snapshot.order {
		source, rank := inputs.explain(snapshot, id, roots, suppressed)
		snapshot.sources[id] = source
		ranks[id] = rank
	}
	applyPiPrecedence(snapshot, ranks, inputs.facts)
	applyNativeProblems(snapshot, inputs.problems, roots)
	return snapshot, nil
}

func (inputs *piInputs) explain(snapshot *Snapshot, id SourceID, roots map[RootID]DiscoveryRoot, suppressed map[ReferenceID]Evidence) (Source, int) {
	source := snapshot.sources[id]
	ex := AvailabilityExplanation{State: AvailabilityUnknown, Reason: "explicit viewer root does not establish Pi participation"}
	e := piEvidence(EvidenceUnknown, "inspection-only", source.Directory, "", ex.Reason)
	ex.Evidence = &e
	bestRank := 1000
	var best *ReferenceAvailability
	var bestFact piRootFact
	for _, rid := range source.References {
		ref := snapshot.references[rid]
		root := roots[ref.Root]
		fact, known := inputs.facts[ref.Root]
		ra := ReferenceAvailability{Reference: rid, Participation: ParticipationInspection, Permission: PermissionExplanation{State: PermissionUnknown, Evidence: piEvidence(EvidenceUnknown, "inspection-only", ref.DiscoveredPath, "", "viewer-selected path does not establish native loading")}, Provenance: Provenance{Kind: "custom", Evidence: e}, Evidence: e}
		if known {
			ra.Participation = ParticipationEligible
			ra.Provenance = fact.provenance
			ra.Evidence = piEvidence(EvidenceInferred, "native-resource", root.Path, "", "Pi reference is in the selected known search scope")
			ra.Permission = PermissionExplanation{State: PermissionAllowed, Evidence: piEvidence(EvidenceInferred, "resource-filters", fact.path, "skills", "resource root inclusions and exclusions applied independently"), Layers: cloneLayers(inputs.scope.ConfigLayers)}
			file := piOccurrenceFile(source, ref)
			enabled, patternKnown := piEnabled(file, piFilterPatterns(fact.patterns), fact.base, strings.HasPrefix(root.Origin, "pi-configured"), false)
			ra.Permission.Evidence.Setting = "skills resource rules: " + strings.Join(piFilterPatterns(fact.patterns), ", ")
			if fact.packageFilter {
				enabled, patternKnown = piEnabled(file, fact.packagePatterns, fact.base, true, fact.delta)
				ra.Permission.Evidence.Setting = "packages.skills resource rules: " + strings.Join(fact.packagePatterns, ", ")
				if len(fact.packagePatterns) == 0 {
					enabled = false
				}
			}
			if !enabled {
				ra.Permission.State = PermissionDenied
				ra.Permission.Evidence.Reason = "Pi resource inclusion/exclusion leaves this native occurrence disabled"
			}
			switch fact.trust {
			case "denied", "inactive":
				ra.Participation = ParticipationExcluded
				ra.Evidence = piEvidence(EvidenceInferred, "project-trust-or-registration", fact.path, "", "native reference is excluded by trust or inactive package registration")
			case "ask", "unknown":
				ra.Participation = ParticipationUnknown
				ra.Evidence = piEvidence(EvidenceUnknown, "project-trust", fact.path, "defaultProjectTrust", "project trust requires a decision or cannot be read")
			}
			// Ignore files use gitignore semantics; unsupported forms must not be invented.
			ignoreUnknown := !root.piFile && piHasIgnore(root.Path, file)
			if fact.uncertain || inputs.global.uncertain || !patternKnown || ignoreUnknown {
				ra.Permission.State = PermissionUnknown
				ra.Permission.Evidence = piEvidence(EvidenceUnknown, "unverified-native-input", fact.path, "skills", "configuration, pattern or ignore-file semantics cannot be reliably interpreted")
				if ignoreUnknown {
					inputs.problem(root.Path, "Pi ignore-file rules require native validation; resource availability remains unknown")
				}
			}
			if source.Manifest.Status != ManifestParsed || strings.TrimSpace(source.Description) == "" || !strings.HasSuffix(source.Manifest.Path, ".md") {
				ra.Participation = ParticipationExcluded
				ra.Evidence = piEvidence(EvidenceInferred, "manifest-eligibility", source.Manifest.Path, "description", "Pi omits malformed manifests, missing descriptions and non-Markdown files")
			}
			if fact.provenance.Kind == "plugin" {
				source.Ownership = EvidenceState{State: "managed", Reason: "Pi package registration or managed storage owns this content; no isolated writer is enabled"}
			}
			candidateRank := fact.rank
			if fact.trust == "denied" || fact.trust == "inactive" {
				candidateRank += 100
			}
			if selection, isSuppressed := suppressed[rid]; isSuppressed {
				ra.Participation = ParticipationExcluded
				ra.Evidence = selection
				candidateRank += 10
			}
			if candidateRank < bestRank {
				copy := ra
				best = &copy
				bestRank = candidateRank
				bestFact = fact
			}
		}
		ex.References = append(ex.References, ra)
	}
	if best != nil {
		ex.ReferenceID = best.Reference
		ex.Evidence = &best.Evidence
		switch {
		case bestFact.trust == "ask":
			ex.State = AvailabilityAsk
			ex.Reason = "project participation requires a trust decision; running-session choice is unknown"
		case best.Participation == ParticipationExcluded:
			ex.State = AvailabilityUnavailable
			ex.Reason = best.Evidence.Reason
		case best.Participation != ParticipationEligible || best.Permission.State == PermissionUnknown:
			ex.State = AvailabilityUnknown
			ex.Reason = "native participation or resource configuration remains unknown"
		case best.Permission.State == PermissionDenied:
			ex.State = AvailabilityDisabled
			ex.Reason = best.Permission.Evidence.Reason
		default:
			ex.State = AvailabilityAvailable
			ex.Reason = "eligible Pi resource; configured native occurrence is enabled"
		}
		if ex.State == AvailabilityUnknown {
			e := piEvidence(EvidenceUnknown, "configured-availability", bestFact.path, "skills", ex.Reason)
			ex.Evidence = &e
		}
		file := piOccurrenceFile(source, snapshot.references[best.Reference])
		ex.NativeName, _ = source.Manifest.Metadata["name"].(string)
		if ex.NativeName == "" {
			ex.NativeName = filepath.Base(filepath.Dir(file))
		}
		n := piEvidence(EvidenceObserved, "native-name", file, "name", "manifest name or selected lexical occurrence parent-directory fallback")
		ex.NativeNameEvidence = &n
	}
	invocation := SessionExplanation{State: "implicit-allowed", Evidence: piEvidence(EvidenceInferred, "model-invocation", source.Manifest.Path, "disable-model-invocation", "native default includes the resource in the system prompt")}
	if disabled, _ := source.Manifest.Metadata["disable-model-invocation"].(bool); disabled {
		invocation.State = "explicit-only"
		invocation.Evidence.Setting = "disable-model-invocation=true"
		invocation.Evidence.Reason = "Pi excludes this skill from the model prompt; explicit command registration is a separate setting"
		if ex.State == AvailabilityAvailable {
			ex.State = AvailabilityExplicitOnly
			ex.Reason = invocation.Evidence.Reason
		}
	}
	commands := inputs.global.commands
	if inputs.trust == "allowed" && inputs.project.commands != nil {
		commands = inputs.project.commands
	}
	if commands != nil && !*commands {
		invocation.Evidence.Reason += "; enableSkillCommands=false prevents /skill commands without disabling discovery"
	}
	ex.Invocation = &invocation
	ex.Precedence = &PrecedenceExplanation{State: PrecedenceUnique, Evidence: piEvidence(EvidenceInferred, "resource-precedence", source.Manifest.Path, "", "project configured > project automatic > global configured > global automatic > packages; native first name wins")}
	ex.Session = &SessionExplanation{State: "unknown", Evidence: piEvidence(EvidenceUnknown, "session-not-observed", "", "", "no running Pi session or model invocation was inspected")}
	ex.Reload = &SessionExplanation{State: "unknown", Evidence: piEvidence(EvidenceUnknown, "session-reload", "", "", "viewer refresh rereads files; native /reload and fresh-session enforcement are not verified")}
	for _, op := range []string{"enable", "disable", "project-override", "delete"} {
		scope := "source"
		if op == "project-override" {
			scope = "project"
		}
		ex.Capabilities = append(ex.Capabilities, Capability{Operation: op, Scope: scope, State: "unsupported", Evidence: piEvidence(EvidenceUnknown, "read-only-ticket", source.Directory, "", "Pi writers are not validated or enabled; managed packages need their native manager")})
	}
	source.Availability = ex
	return source, bestRank
}
func piOccurrenceFile(source Source, ref Reference) string {
	if filepath.Base(source.Manifest.Path) == "SKILL.md" {
		return filepath.Join(ref.DiscoveredPath, "SKILL.md")
	}
	return ref.DiscoveredPath
}

// The package manager inserts packages, configured paths, then automatic paths.
// Its lexical map keeps the first registration before rank/canonical selection.
func (inputs *piInputs) occurrenceDuplicates(snapshot *Snapshot) map[ReferenceID]Evidence {
	seen := make(map[string]bool)
	suppressed := make(map[ReferenceID]Evidence)
	var retained []ReferenceID
	for _, rid := range snapshot.referenceOrder {
		ref := snapshot.references[rid]
		fact, native := inputs.facts[ref.Root]
		if !native || fact.trust == "denied" || fact.trust == "inactive" || ref.Source == "" {
			continue
		}
		file := piOccurrenceFile(snapshot.sources[ref.Source], ref)
		if seen[file] {
			suppressed[rid] = piEvidence(EvidenceInferred, "native-first-lexical-occurrence", file, file, "earlier native registration retained this identical lexical resource")
		} else {
			seen[file] = true
			retained = append(retained, rid)
		}
	}
	sort.SliceStable(retained, func(i, j int) bool {
		return inputs.facts[snapshot.references[retained[i]].Root].rank < inputs.facts[snapshot.references[retained[j]].Root].rank
	})
	canonicalSeen := make(map[string]string)
	for _, rid := range retained {
		ref := snapshot.references[rid]
		file := piOccurrenceFile(snapshot.sources[ref.Source], ref)
		canonical, err := filepath.EvalSymlinks(file)
		if err != nil {
			continue
		}
		if winner, exists := canonicalSeen[canonical]; exists {
			suppressed[rid] = piEvidence(EvidenceInferred, "native-first-canonical-occurrence", canonical, winner, "rank-ordered native selection retained the canonical instruction file at "+winner)
		} else {
			canonicalSeen[canonical] = file
		}
	}
	return suppressed
}

func piHasIgnore(root, file string) bool {
	path := filepath.Dir(file)
	root = filepath.Clean(root)
	for {
		for _, name := range []string{".gitignore", ".ignore", ".fdignore"} {
			raw, err := nativeBytes(filepath.Join(path, name))
			if !nativeAbsent(filepath.Join(path, name), err) {
				if err != nil {
					return true
				}
				for _, line := range strings.Split(string(raw), "\n") {
					line = strings.TrimSpace(line)
					if line != "" && !strings.HasPrefix(line, "#") {
						return true
					}
				}
			}
		}
		if path == root || !pathWithin(root, path) || filepath.Dir(path) == path {
			break
		}
		path = filepath.Dir(path)
	}
	return false
}
func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func applyPiPrecedence(snapshot *Snapshot, ranks map[SourceID]int, facts map[RootID]piRootFact) {
	order := map[SourceID]int{}
	for index, rid := range snapshot.referenceOrder {
		ref := snapshot.references[rid]
		id := ref.Source
		fact, native := facts[ref.Root]
		if !native || fact.rank != ranks[id] {
			continue
		}
		if _, seen := order[id]; !seen {
			order[id] = index
		}
	}
	groups := make(map[string][]SourceID)
	for _, id := range snapshot.order {
		s := snapshot.sources[id]
		if s.Availability.NativeName != "" && (s.Availability.State == AvailabilityAvailable || s.Availability.State == AvailabilityExplicitOnly) {
			groups[s.Availability.NativeName] = append(groups[s.Availability.NativeName], id)
		}
	}
	for _, ids := range groups {
		sort.SliceStable(ids, func(i, j int) bool {
			if ranks[ids[i]] != ranks[ids[j]] {
				return ranks[ids[i]] < ranks[ids[j]]
			}
			return order[ids[i]] < order[ids[j]]
		})
		if len(ids) < 2 {
			continue
		}
		winner := ids[0]
		for _, id := range ids {
			s := snapshot.sources[id]
			s.Availability.Precedence.Winner = winner
			for _, related := range ids {
				if related != id {
					s.Availability.Precedence.RelatedSources = append(s.Availability.Precedence.RelatedSources, related)
				}
			}
			if id != winner {
				s.Availability.State = AvailabilityShadowed
				s.Availability.Reason = "higher-priority enabled Pi resource wins the native name"
				e := piEvidence(EvidenceInferred, "native-first-name-wins", s.Manifest.Path, "", s.Availability.Reason)
				s.Availability.Evidence = &e
			}
			snapshot.sources[id] = s
		}
	}
}

func (inputs *piInputs) addPackages(request *Request, agentDir string) {
	type entry struct {
		pkg         piPackage
		base, trust string
		uncertain   bool
	}
	entries := []entry{}
	if request.Project.Selected {
		for _, p := range inputs.project.packages {
			entries = append(entries, entry{p, filepath.Join(request.Project.Path, ".pi"), inputs.trust, inputs.project.uncertain})
		}
	}
	for _, p := range inputs.global.packages {
		entries = append(entries, entry{p, agentDir, "allowed", inputs.global.uncertain})
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		p := entry.pkg
		root := piPath(p.Source, entry.base, request.home)
		identity := root
		managed := false
		uncertain := entry.uncertain
		if strings.HasPrefix(p.Source, "npm:") {
			name := strings.TrimPrefix(p.Source, "npm:")
			if at := strings.LastIndex(name, "@"); at > 0 {
				name = name[:at]
				uncertain = true
			}
			root = filepath.Join(entry.base, "npm", "node_modules", name)
			identity = "npm:" + name
			managed = true
		} else if strings.HasPrefix(p.Source, "git:") {
			git := strings.TrimPrefix(p.Source, "git:")
			if strings.Contains(git, "://") || strings.Contains(git, ":") || strings.HasPrefix(git, "/") {
				inputs.problem(entry.base, "Pi remote package source form is not validated for offline resolution")
				continue
			}
			if at := strings.LastIndex(git, "@"); at > 0 {
				git = git[:at]
				uncertain = true
			}
			root = filepath.Join(entry.base, "git", git)
			identity = "git:" + git
			managed = true
		} else if strings.Contains(p.Source, "://") || strings.HasPrefix(p.Source, "git@") || strings.Contains(p.Source, ":") {
			inputs.problem(entry.base, "Pi package source is not a validated local, npm: or git: registration")
			continue
		}
		if seen[identity] {
			continue
		}
		if entry.trust == "allowed" {
			seen[identity] = true
		}
		if !pathWithin(entry.base, root) && managed {
			inputs.problem(entry.base, "Pi package registration escapes managed storage")
			continue
		}
		fact := piRootFact{rank: 4, base: root, path: filepath.Join(entry.base, "settings.json"), trust: entry.trust, uncertain: uncertain, packageFilter: p.hasSkills, packagePatterns: p.Skills, delta: p.Autoload != nil && !*p.Autoload, provenance: Provenance{Kind: "plugin", PluginID: p.Source, Evidence: piEvidence(EvidenceObserved, "package-registration", filepath.Join(entry.base, "settings.json"), "packages", "Pi package registration observed; package owns its resources")}}
		if fact.delta {
			fact.uncertain = true
			inputs.problem(fact.path, "Pi autoload=false package deltas need additional native validation")
		}
		raw, err := nativeBytes(filepath.Join(root, "package.json"))
		var manifest struct {
			Version string                      `json:"version"`
			Pi      *map[string]json.RawMessage `json:"pi"`
		}
		if !nativeAbsent(filepath.Join(root, "package.json"), err) && (err != nil || json.Unmarshal(raw, &manifest) != nil) {
			fact.uncertain = true
			inputs.problem(filepath.Join(root, "package.json"), "Pi package manifest is unreadable or invalid")
		}
		fact.provenance.Version = manifest.Version
		skillRoots := []string{"skills"}
		if manifest.Pi != nil {
			value, hasSkills := (*manifest.Pi)["skills"]
			var entries []string
			if hasSkills {
				var valid bool
				entries, valid = piResourceArray(value)
				if !valid {
					fact.uncertain = true
					inputs.problem(filepath.Join(root, "package.json"), "Pi package skills must be an array of strings")
				}
			}
			switch {
			case p.hasSkills || fact.delta:
				if len(entries) > 0 {
					skillRoots = entries
				}
			case !p.object || hasSkills:
				skillRoots = entries
			}
		}
		var manifestPatterns []string
		for _, value := range skillRoots {
			if strings.HasPrefix(value, "!") || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
				manifestPatterns = append(manifestPatterns, value)
			}
		}
		// Manifest filters define package membership before user resource filters.
		if len(manifestPatterns) > 0 {
			fact.uncertain = true
			inputs.problem(filepath.Join(root, "package.json"), "Pi package manifest filters are not yet independently validated")
		}
		for _, value := range skillRoots {
			if piPattern(value) {
				if strings.ContainsAny(value, "*?") {
					fact.uncertain = true
					inputs.problem(filepath.Join(root, "package.json"), "Pi package manifest glob expansion is not independently validated; inventory may be partial")
					matches, err := filepath.Glob(filepath.Join(root, value))
					if err != nil {
						inputs.problem(root, "Pi package glob cannot be resolved")
					}
					for _, path := range matches {
						inputs.addRoot(request, path, ContextManaged, "pi-package", "pi", fact, false)
					}
				}
				continue
			}
			inputs.addRoot(request, piPath(value, root, request.home), ContextManaged, "pi-package", "pi", fact, false)
		}
	}
}
