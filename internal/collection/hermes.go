package collection

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

const hermesValidatedVersion = "Hermes 0.20.0 (5a16635f409e0f5e3d754bb6256bd27707bcec4c)"

type hermesInputs struct {
	home, config, platform string
	scope                  AgentScope
	configData             map[string]any
	pluginConfig           map[string]any
	pluginManaged          map[string]any
	managedConfigPath      string
	pluginConfigPath       string
	pluginUncertain        bool
	policyEnv              map[string]string
	policyDeclared         map[string]bool
	uncertain              bool
	problems               []Problem
	plugins                map[RootID]hermesPlugin
}

func hermesEvidence(kind EvidenceKind, rule, path, setting, reason string) Evidence {
	return Evidence{Kind: kind, Rule: rule, InputPath: path, Setting: setting, AgentVersion: hermesValidatedVersion, Reason: reason}
}
func hermesBaseHome(home string) string {
	if value := strings.TrimSpace(os.Getenv("HERMES_HOME")); value != "" {
		path, _ := filepath.Abs(value)
		return path
	}
	return filepath.Join(home, ".hermes")
}
func (inputs *hermesInputs) problem(path, reason string) {
	inputs.uncertain = true
	inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
}
func (inputs *hermesInputs) yaml(path string) (map[string]any, string) {
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		return map[string]any{}, "absent"
	}
	if err != nil {
		inputs.problem(path, "Hermes input cannot be read: "+err.Error())
		return nil, "unknown"
	}
	value, _, err := decodeMetadata(raw)
	if err != nil {
		inputs.problem(path, "Hermes input is invalid YAML: "+err.Error())
		return nil, "unknown"
	}
	return value, "read"
}
func readHermesInputs(request Request) hermesInputs {
	inputs := hermesInputs{home: request.hermesHome, plugins: make(map[RootID]hermesPlugin)}
	inputs.scope = AgentScope{Application: "hermes", ValidatedVersion: hermesValidatedVersion, VersionEvidence: "docs/verification/ale-31.md", Evidence: hermesEvidence(EvidenceInferred, "version-specific-local-scope", inputs.home, "", "rules validated with installed native loader in disposable fixtures"), Gaps: []string{
		"Running-session state, in-process home overrides, toolset conditions and reload are unknown; the viewer never starts an agent session.",
		"This installed version does not load project skill/config directories automatically. Project skills participate only through profile skills.external_dirs; explicit roots remain inspection-only.",
		"Current online documentation describes newer project precedence. It is not applied to the pinned installed source.",
		"Native plugin registrations require executing plugin code and remain unknown. Bundled plugin installation and pip entry points are outside this bounded home/project scope.",
		"Legacy flat .md explicit loads and malformed native frontmatter fallback are outside the SKILL.md bundle contract.",
		"Availability describes the native skills_list index. The system-prompt offer surface additionally filters toolsets and can retain colliding primary definitions; running-session offers are unknown.",
	}}
	if filepath.Base(filepath.Dir(inputs.home)) != "profiles" && os.Getenv("HERMES_S6_SUPERVISED_CHILD") == "" {
		root := inputs.home
		native := filepath.Join(request.home, ".hermes")
		if pathWithin(inputs.home, native) {
			root = native
		}
		marker := filepath.Join(root, "active_profile")
		raw, err := nativeBytes(marker)
		state, reason := "absent", "no sticky profile; native root retained"
		if !nativeAbsent(marker, err) {
			if err != nil {
				inputs.problem(marker, "active profile is unreadable")
				state, reason = "unknown", "profile cannot be resolved"
			} else {
				name := strings.ToLower(strings.TrimSpace(string(raw)))
				state, reason = "read", "sticky profile is "+name
				if name != "" && name != "default" {
					if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`).MatchString(name) || hermesHas([]string{"hermes", "test", "tmp", "root", "sudo"}, name) {
						inputs.problem(marker, "active profile name is invalid")
					} else {
						inputs.home = filepath.Join(root, "profiles", name)
						if info, err := os.Stat(inputs.home); err != nil || !info.IsDir() {
							inputs.problem(inputs.home, "active Hermes profile directory is missing or inaccessible")
						}
					}
				}
			}
		}
		inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, PermissionLayer{Path: marker, Scope: "profile-selection", State: state, Reason: reason, Evidence: hermesEvidence(EvidenceObserved, "active-profile", marker, "active_profile", reason)})
	}
	inputs.config = filepath.Join(inputs.home, "config.yaml")
	var state string
	inputs.configData, state = inputs.yaml(inputs.config)
	for _, key := range []string{"skills", "plugins"} {
		if value, present := inputs.configData[key]; present && value != nil && hermesMap(inputs.configData, key) == nil {
			inputs.problem(inputs.config, key+" must be a mapping")
		}
	}
	if value := hermesMap(inputs.configData, "skills")["platform_disabled"]; value != nil {
		if _, ok := value.(map[string]any); !ok {
			inputs.problem(inputs.config, "skills.platform_disabled must be a mapping")
		}
	}
	reason := "active-profile config only; no default-profile or project config inheritance"
	inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, PermissionLayer{Path: inputs.config, Scope: "active-profile", State: state, Reason: reason, Evidence: hermesEvidence(EvidenceObserved, "profile-config", inputs.config, "skills", reason)})
	inputs.readPolicy(request)
	return inputs
}
func hermesMap(data map[string]any, key string) map[string]any {
	value, _ := data[key].(map[string]any)
	return value
}
func hermesStrings(value any) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	if text, ok := value.(string); ok {
		return []string{text}, true
	}
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(list))
	for _, value := range list {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		result = append(result, strings.TrimSpace(text))
	}
	return result, true
}
func hermesHas(values []string, name string) bool {
	for _, value := range values {
		if value == name {
			return true
		}
	}
	return false
}
func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func (inputs *hermesInputs) roots(request *Request) {
	add := func(path string, context ReferenceContext, origin string, optional bool) {
		request.Roots = append(request.Roots, DiscoveryRoot{ID: RootID(stableID("root_hermes", origin, path)), Path: path, Context: context, Origin: origin, Optional: optional})
	}
	add(filepath.Join(inputs.home, "skills"), ContextGlobal, "hermes-profile", true)
	skills := hermesMap(inputs.configData, "skills")
	if value, present := inputs.configData["skills"]; present && value != nil && skills == nil {
		inputs.problem(inputs.config, "skills must be a mapping")
	}
	dirs, ok := hermesStrings(skills["external_dirs"])
	if !ok {
		inputs.problem(inputs.config, "skills.external_dirs cannot be interpreted")
	}
	seen := make(map[string]bool)
	local, _ := filepath.EvalSymlinks(filepath.Join(inputs.home, "skills"))
	seen[local] = true
	for _, value := range dirs {
		if value == "" {
			continue
		}
		// Python expandvars leaves unknown variables intact. Never replace them with empty text.
		value = os.Expand(value, func(key string) string {
			if hermesPolicyKeys[key] {
				if value, exists := inputs.policyEnv[key]; exists {
					return value
				}
				return "${" + key + "}"
			}
			if inputs.policyDeclared[key] {
				inputs.problem(inputs.config, "external directory references unsupported persisted environment policy; credential values are not evaluated")
				return "${" + key + "}"
			}
			if value, exists := os.LookupEnv(key); exists {
				return value
			}
			return "${" + key + "}"
		})
		if value == "~" {
			value = request.home
		} else if strings.HasPrefix(value, "~/") {
			value = filepath.Join(request.home, value[2:])
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(inputs.home, value)
		}
		value = filepath.Clean(value)
		resolved, err := filepath.EvalSymlinks(value)
		if err == nil {
			if seen[resolved] {
				continue
			}
			seen[resolved] = true
			value = resolved
		}
		context := ContextGlobal
		projectPath := request.Project.Path
		if resolved, err := filepath.EvalSymlinks(projectPath); err == nil {
			projectPath = resolved
		}
		if request.Project.Selected && pathWithin(value, projectPath) {
			context = ContextProject
		}
		add(value, context, "hermes-external", false)
	}
	if request.Project.Selected {
		if info, err := os.Stat(request.Project.Path); err != nil || !info.IsDir() {
			inputs.problem(request.Project.Path, "selected project is missing or inaccessible")
		}
	}
	discoverHermesPlugins(request, inputs)
}
func buildHermes(ctx context.Context, request Request) (*Snapshot, error) {
	original := cloneRequest(request)
	inputs := readHermesInputs(request)
	inputs.roots(&request)
	snapshot, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	snapshot.request = original
	snapshot.scope.Agent = &inputs.scope
	roots := make(map[RootID]DiscoveryRoot)
	for _, root := range request.Roots {
		roots[root.ID] = root
	}
	for index := range snapshot.scope.Roots {
		root := &snapshot.scope.Roots[index]
		if root.Origin != "" {
			value := hermesEvidence(EvidenceObserved, "filesystem-root", root.Requested, "", "read-only scanner observed "+string(root.State))
			root.Evidence = &value
		}
	}
	for _, id := range snapshot.order {
		snapshot.sources[id] = explainHermesSource(snapshot, id, roots, &inputs)
	}
	applyHermesPrecedence(snapshot, request.Roots)
	// Missing/unreadable discovery can hide a same-name candidate or a higher-priority definition.
	uncertain := inputs.uncertain || !snapshot.Complete()
	if uncertain {
		for _, id := range snapshot.order {
			source := snapshot.sources[id]
			if source.Availability.State == AvailabilityAvailable || source.Availability.State == AvailabilityExplicitOnly || source.Availability.State == AvailabilityShadowed {
				source.Availability.State = AvailabilityUnknown
				source.Availability.Reason = "native scope is incomplete; effective permission or precedence cannot be established"
				value := hermesEvidence(EvidenceUnknown, "incomplete-scope", inputs.config, "", source.Availability.Reason)
				source.Availability.Evidence = &value
				snapshot.sources[id] = source
			}
		}
	}
	// The shared problem attachment routine is independent of Codex policy.
	applyNativeInputProblems(snapshot, inputs.problems, roots)
	return snapshot, nil
}
func explainHermesSource(snapshot *Snapshot, id SourceID, roots map[RootID]DiscoveryRoot, inputs *hermesInputs) Source {
	source := snapshot.sources[id]
	name := source.Manifest.Name
	if name == "" {
		name = filepath.Base(source.Directory)
	}
	name = string([]rune(name)[:min(len([]rune(name)), 64)])
	value := hermesEvidence(EvidenceInferred, "hermes-skill-index", source.Manifest.Path, "", "active profile precedes configured external directories; this is inferred index visibility, not observed session use")
	explanation := AvailabilityExplanation{State: AvailabilityUnavailable, NativeName: name, Evidence: &value, Reason: "source has no native discovery reference"}
	nameValue := hermesEvidence(EvidenceObserved, "frontmatter-name-or-directory", source.Manifest.Path, "name", "native index uses name truncated to 64 characters, falling back to directory name")
	explanation.NativeNameEvidence = &nameValue
	explanation.Session = &SessionExplanation{State: "unknown", Evidence: hermesEvidence(EvidenceUnknown, "running-session", inputs.config, "", "running session was not inspected")}
	explanation.Reload = &SessionExplanation{State: "unknown", Evidence: hermesEvidence(EvidenceUnknown, "reload", inputs.config, "", "new snapshot re-reads configuration; running-agent reload was not observed")}
	explanation.Invocation = &SessionExplanation{State: "index-eligible", Evidence: hermesEvidence(EvidenceInferred, "index-vs-explicit-load", source.Manifest.Path, "", "native index membership does not establish explicit-load success, toolset relevance or model use")}
	for _, operation := range []string{"enable", "disable", "remove-project-override", "delete"} {
		explanation.Capabilities = append(explanation.Capabilities, Capability{Operation: operation, Scope: "selected-source", State: "unverified", Evidence: hermesEvidence(EvidenceUnknown, "read-only-capability", inputs.config, "", "this adapter exposes read-only behavior; a native writer has not been validated")})
	}
	nativeNames := make(map[string]bool)
	for _, rid := range source.References {
		ref := snapshot.references[rid]
		root := roots[ref.Root]
		fact := ReferenceAvailability{Reference: rid, Participation: ParticipationInspection, Evidence: hermesEvidence(EvidenceObserved, "explicit-root", ref.DiscoveredPath, "", "explicit viewer roots are inspection-only unless the same source has a native reference"), Permission: PermissionExplanation{State: PermissionUnknown, Evidence: hermesEvidence(EvidenceUnknown, "inspection-only", ref.DiscoveredPath, "", "custom/project root alone is not Hermes load evidence")}, Provenance: Provenance{Kind: "user", Evidence: hermesEvidence(EvidenceObserved, "filesystem-source", ref.DiscoveredPath, "", "bundle observed; ownership is inferred separately")}}
		if root.Origin == "hermes-plugin-inventory" {
			fact.Evidence = hermesEvidence(EvidenceObserved, "plugin-inventory-only", ref.DiscoveredPath, "", "plugin directory/cache observed; only a validated manifest and registration can establish participation")
		}
		if root.Origin != "" && root.Origin != "hermes-plugin-inventory" {
			fact.Participation = ParticipationEligible
			fact.Evidence = hermesEvidence(EvidenceInferred, root.Origin, ref.DiscoveredPath, "", "native skill index searches this profile/configured root")
			referenceName := hermesReferenceName(source, ref)
			fact.Evidence.Setting = "native_name=" + referenceName
			fact.Permission = hermesPermission(referenceName, inputs)
			if root.Origin == "hermes-external" {
				fact.Provenance.Kind = "external"
			}
			if reason := hermesExcluded(ref, root); reason != "" {
				fact.Participation = ParticipationExcluded
				fact.Evidence = hermesEvidence(EvidenceInferred, "index-exclusion", ref.DiscoveredPath, "", reason)
			}
			if plugin, ok := inputs.plugins[root.ID]; ok {
				hermesPluginFact(&fact, &explanation, source, ref, plugin, inputs)
			}
			if !hermesPlatformMatches(source.Manifest.Metadata["platforms"]) {
				fact.Participation = ParticipationExcluded
				fact.Evidence = hermesEvidence(EvidenceInferred, "os-platform", source.Manifest.Path, "platforms", "frontmatter excludes inspected operating system "+runtime.GOOS)
			}
			if plugin, ok := inputs.plugins[root.ID]; ok {
				nativeNames[plugin.namespace+":"+filepath.Base(ref.DiscoveredPath)] = true
			} else {
				nativeNames[referenceName] = true
			}
			if source.Manifest.Status != ManifestParsed {
				fact.Participation = ParticipationUnknown
				fact.Evidence = hermesEvidence(EvidenceUnknown, "manifest-coverage", source.Manifest.Path, "", "strict viewer parse cannot establish native fallback metadata")
			}
		}
		explanation.References = append(explanation.References, fact)
	}
	for _, fact := range explanation.References {
		if fact.Participation == ParticipationEligible {
			state := AvailabilityAvailable
			reason := "native index eligibility inferred from profile discovery and configuration"
			if fact.Provenance.Kind == "plugin" {
				state = AvailabilityExplicitOnly
				reason = "portable plugin skill can be loaded explicitly; it is excluded from the automatic system-prompt index"
			}
			if fact.Permission.State == PermissionDenied {
				state = AvailabilityDisabled
				reason = fact.Permission.Evidence.Reason
			}
			if fact.Permission.State == PermissionUnknown {
				state = AvailabilityUnknown
				reason = fact.Permission.Evidence.Reason
			}
			if explanation.State == AvailabilityUnavailable || state == AvailabilityAvailable || state == AvailabilityExplicitOnly {
				explanation.State, explanation.Reason = state, reason
			}
		} else if fact.Participation == ParticipationUnknown && explanation.State == AvailabilityUnavailable {
			explanation.State, explanation.Reason = AvailabilityUnknown, fact.Evidence.Reason
		}
	}
	if len(nativeNames) == 1 {
		for name := range nativeNames {
			explanation.NativeName = name
		}
	}
	if len(nativeNames) > 1 {
		names := make([]string, 0, len(nativeNames))
		for name := range nativeNames {
			names = append(names, name)
		}
		sort.Strings(names)
		explanation.NativeName = strings.Join(names, ", ")
		explanation.NativeNameEvidence.Rule = "multiple-native-names"
		explanation.NativeNameEvidence.Reason = "shared source has several reference-specific native names; permission facts are separate and aggregate effective precedence is unknown"
		explanation.State = AvailabilityUnknown
		explanation.Reason = explanation.NativeNameEvidence.Reason
	}
	if explanation.State == AvailabilityUnavailable {
		explanation.Reason = "native source excluded, inactive, or inspection-only; see reference rules"
	}
	if (hermesEnvironmentDeclared(source.Manifest.Metadata) || hermesConditional(source.Manifest.Metadata)) && explanation.State == AvailabilityAvailable {
		explanation.State = AvailabilityUnknown
		explanation.Reason = "environment/toolset relevance is session-dependent; explicit native loading remains possible"
		explanation.Invocation.State = "explicit-load-possible"
	}
	explanation.Evidence.Reason = explanation.Reason
	if explanation.State == AvailabilityUnknown {
		explanation.Evidence.Kind = EvidenceUnknown
	}
	source.Availability = explanation
	source.Ownership = EvidenceState{State: "unknown", Reason: "source ownership cannot be established from installation alone"}
	for _, fact := range explanation.References {
		if fact.Provenance.Kind == "plugin" {
			source.Ownership = EvidenceState{State: "managed", Reason: "plugin owns registration and bundle; per-skill cache edits are unsupported"}
			break
		}
		if fact.Provenance.Kind == "external" {
			source.Ownership = EvidenceState{State: "external", Reason: "configured external source; Hermes autonomous curator does not own this bundle"}
		}
	}
	return source
}
func hermesReferenceName(source Source, ref Reference) string {
	name := source.Manifest.Name
	if name == "" {
		name = filepath.Base(ref.DiscoveredPath)
	}
	chars := []rune(name)
	return string(chars[:min(len(chars), 64)])
}
func hermesPermission(name string, inputs *hermesInputs) PermissionExplanation {
	state, reason := PermissionAllowed, "not in global or selected messaging-platform disabled list"
	skills := hermesMap(inputs.configData, "skills")
	global, ok := hermesStrings(skills["disabled"])
	platforms := hermesMap(skills, "platform_disabled")
	selected, selectedOK := hermesStrings(platforms[inputs.platform])
	if !ok || !selectedOK || inputs.uncertain {
		state, reason = PermissionUnknown, "profile config cannot be reliably interpreted"
	} else if hermesHas(global, name) || (inputs.platform != "" && hermesHas(selected, name)) {
		state, reason = PermissionDenied, "name disabled by skills.disabled union skills.platform_disabled for "+inputs.platform
	}
	return PermissionExplanation{State: state, Evidence: hermesEvidence(EvidenceInferred, "disabled-name-union", inputs.config, "skills.disabled, skills.platform_disabled."+inputs.platform, reason), Layers: cloneLayers(inputs.scope.ConfigLayers)}
}
func hermesPlatformMatches(value any) bool {
	list, ok := hermesStrings(value)
	if !ok {
		return false
	}
	if len(list) == 0 {
		return true
	}
	osName := map[string]string{"darwin": "darwin", "windows": "win32", "linux": "linux"}[runtime.GOOS]
	for _, tag := range list {
		tag = strings.ToLower(strings.TrimSpace(tag))
		tag = mapPlatformTag(tag)
		if strings.HasPrefix(osName, tag) {
			return true
		}
	}
	return false
}
func mapPlatformTag(tag string) string {
	switch tag {
	case "macos":
		return "darwin"
	case "windows":
		return "win32"
	}
	return tag
}
func hermesConditional(data map[string]any) bool {
	policy := hermesMap(hermesMap(data, "metadata"), "hermes")
	for _, key := range []string{"requires_tools", "requires_toolsets", "fallback_for_tools", "fallback_for_toolsets"} {
		if value, present := policy[key]; present && value != nil {
			return true
		}
	}
	return false
}
func hermesEnvironmentDeclared(data map[string]any) bool {
	value := data["environments"]
	if value == nil {
		return false
	}
	list, ok := hermesStrings(value)
	return !ok || len(list) > 0
}

var hermesExcludedDirs = map[string]bool{".git": true, ".github": true, ".hub": true, ".archive": true, ".venv": true, "venv": true, "node_modules": true, "site-packages": true, "__pycache__": true, ".tox": true, ".nox": true, ".pytest_cache": true, ".mypy_cache": true, ".ruff_cache": true}

func hermesExcluded(ref Reference, root DiscoveryRoot) string {
	rel, _ := filepath.Rel(root.Path, ref.DiscoveredPath)
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if hermesExcludedDirs[part] {
			return "native index excludes dependency, archive and metadata directories"
		}
	}
	if len(parts) > 1 && parts[0] == "_org" {
		marker := filepath.Join(root.Path, "_org", ".active_org")
		raw, err := nativeBytes(marker)
		if err != nil || strings.TrimSpace(string(raw)) != parts[1] {
			return "organization mirror is inactive; .active_org must match this organization"
		}
	}
	return ""
}
func applyHermesPrecedence(snapshot *Snapshot, roots []DiscoveryRoot) {
	type candidate struct {
		source    SourceID
		reference Reference
		rank      int
	}
	groups := make(map[string][]candidate)
	for rank, root := range roots {
		if root.Origin == "" || strings.HasPrefix(root.Origin, "hermes-plugin") {
			continue
		}
		for _, rid := range snapshot.referenceOrder {
			ref := snapshot.references[rid]
			if ref.Root != root.ID || ref.Source == "" {
				continue
			}
			source := snapshot.sources[ref.Source]
			eligible := false
			for _, fact := range source.Availability.References {
				if fact.Reference == rid && fact.Participation == ParticipationEligible {
					eligible = true
				}
			}
			if eligible {
				nativeName := hermesReferenceName(source, ref)
				groups[nativeName] = append(groups[nativeName], candidate{source.ID, ref, rank})
			}
		}
	}
	for _, candidates := range groups {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].rank != candidates[j].rank {
				return candidates[i].rank < candidates[j].rank
			}
			return candidates[i].reference.DiscoveredPath < candidates[j].reference.DiscoveredPath
		})
		unique := make([]candidate, 0, len(candidates))
		seen := make(map[SourceID]bool)
		for _, candidate := range candidates {
			if !seen[candidate.source] {
				seen[candidate.source] = true
				unique = append(unique, candidate)
			}
		}
		candidates = unique
		winner := candidates[0].source
		var ids []SourceID
		for _, candidate := range candidates {
			ids = appendUniqueSourceIDs(ids, candidate.source)
		}
		for _, id := range ids {
			source := snapshot.sources[id]
			state := PrecedenceUnique
			reason := "native index uses active profile first, then external_dirs in configuration order; lexically sorted paths break ties"
			if len(ids) > 1 {
				state = PrecedenceCoexisting
				reason += ". Index shadows later independent copies; skill_view refuses ambiguous bare names and requires an unambiguous relative path"
				source.Availability.Invocation.State = "ambiguous-bare-name"
				source.Availability.Invocation.Evidence.Reason = "native explicit loading refuses multiple independent candidates for this bare name"
			}
			if len(ids) > 1 && candidates[0].rank == candidates[1].rank && roots[candidates[0].rank].Origin == "hermes-profile" {
				state = PrecedenceUnknown
				reason += ". Multiple primary definitions can coexist in system-prompt offers; effective automatic precedence is unknown"
				if source.Availability.State == AvailabilityAvailable {
					source.Availability.State = AvailabilityUnknown
					source.Availability.Reason = "native inventory index and system-prompt offers differ for this same-root name conflict"
				}
			}
			var related []SourceID
			for _, candidate := range ids {
				if candidate != id {
					related = append(related, candidate)
				}
			}
			source.Availability.Precedence = &PrecedenceExplanation{State: state, Winner: winner, RelatedSources: related, Evidence: hermesEvidence(EvidenceInferred, "native-index-first-name", source.Manifest.Path, "", reason)}
			if id != winner && source.Availability.State == AvailabilityAvailable {
				source.Availability.State = AvailabilityShadowed
				source.Availability.Reason = "later same-name definition is shadowed in the native index; source remains independently selectable"
				source.Availability.Evidence.Reason = source.Availability.Reason
			}
			if source.Availability.State == AvailabilityUnknown {
				source.Availability.Evidence.Kind = EvidenceUnknown
				source.Availability.Evidence.Reason = source.Availability.Reason
			}
			snapshot.sources[id] = source
		}
	}
}

// Hermes scans nested bundles outside support directories. The generic explicit-root behavior stays unchanged.
func (builder *discoveryBuilder) discoverHermesChildren(ctx context.Context, root DiscoveryRoot, path string, ancestors map[string]bool, count *int) {
	if ctx.Err() != nil {
		return
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}
	if ancestors[resolved] || *count >= maxResourceEntries {
		code := ProblemSymlinkCycle
		reason := "Hermes traversal forms a cycle"
		if *count >= maxResourceEntries {
			code = ProblemResourceLimit
			reason = "Hermes traversal exceeds directory limit"
		}
		problem := makeProblem(code, path, reason, nil, true)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		return
	}
	*count++
	ancestors[resolved] = true
	defer delete(ancestors, resolved)
	entries, err := os.ReadDir(path)
	if err != nil {
		problem := makeProblem(ProblemRootUnreadable, path, "Hermes container cannot be listed", err, true)
		problem.RootIDs = []RootID{root.ID}
		builder.addProblem(problem)
		return
	}
	present, _ := hasManifest(path)
	if present && path != root.Path {
		relative, _ := filepath.Rel(root.Path, path)
		builder.discoverCandidate(ctx, root, path, relative)
	}
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		// Dependency directories are outside native scope. Org mirrors are retained for explanation.
		if hermesExcludedDirs[entry.Name()] {
			continue
		}
		if present && (entry.Name() == "references" || entry.Name() == "templates" || entry.Name() == "assets" || entry.Name() == "scripts") {
			continue
		}
		child := filepath.Join(path, entry.Name())
		if _, err := filepath.EvalSymlinks(child); err != nil {
			relative, _ := filepath.Rel(root.Path, child)
			builder.discoverCandidate(ctx, root, child, relative)
			continue
		}
		if info, err := os.Stat(child); err == nil && info.IsDir() {
			builder.discoverHermesChildren(ctx, root, child, ancestors, count)
		}
	}
}
