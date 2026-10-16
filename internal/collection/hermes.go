package collection

import (
	"context"
	"fmt"
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
	configYAML             hermesYAMLObservation
	pluginPolicy           map[string]hermesPolicyLeaf
	pluginManaged          map[string]any
	managedConfigPath      string
	pluginFailure          *Evidence
	managedYAML            hermesYAMLObservation
	policyEnv              map[string]string
	policyEnvUnknown       map[string]bool
	platformKnown          bool
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
func (inputs *hermesInputs) yaml(path string) (map[string]any, string, hermesYAMLObservation) {
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		return map[string]any{}, "absent", observeHermesYAML([]byte("{}"))
	}
	if err != nil {
		inputs.problem(path, "Hermes input cannot be read: "+err.Error())
		return nil, "unknown", hermesYAMLObservation{}
	}
	value, _, err := decodeMetadata(raw)
	if err != nil {
		inputs.problem(path, "Hermes input is invalid YAML")
		return nil, "unknown", hermesYAMLObservation{}
	}
	return value, "read", observeHermesYAML(raw)
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
		if hermesPathWithin(inputs.home, native) {
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
	inputs.configData, state, inputs.configYAML = inputs.yaml(inputs.config)
	if state == "read" && (!inputs.configYAML.nativeConstructible() || !inputs.configYAML.mappingAt()) {
		inputs.problem(inputs.config, "profile configuration requires supported native YAML constructors and a mapping root; native fallback and running-session policy are not established")
		inputs.configData, state = nil, "unknown"
	}
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
func hermesRawSkillStrings(value any) ([]string, bool) {
	values, ok := hermesStrings(value)
	result := values[:0]
	for _, value := range values {
		if value = strings.TrimFunc(value, hermesDotenvSpace); value != "" {
			result = append(result, value)
		}
	}
	return result, ok
}
func hermesHas(values []string, name string) bool {
	for _, value := range values {
		if value == name {
			return true
		}
	}
	return false
}
func hermesPathWithin(path, root string) bool {
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
	dirs, ok := hermesRawSkillStrings(skills["external_dirs"])
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
		unresolved := false
		value = os.Expand(value, func(key string) string {
			if inputs.policyEnvUnknown["HERMES_MANAGED_DIR"] || inputs.policyEnvUnknown[key] {
				unresolved = true
				return ""
			}
			if hermesPolicyKeys[key] {
				if value, exists := inputs.policyEnv[key]; exists {
					return value
				}
				return "${" + key + "}"
			}
			if value, exists := os.LookupEnv(key); exists {
				return value
			}
			return "${" + key + "}"
		})
		if unresolved {
			inputs.problem(inputs.config, "external directory references unsupported persisted environment policy; no candidate root is inspected")
			continue
		}
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
		if request.Project.Selected && hermesPathWithin(value, projectPath) {
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
	indexMetadata := make(map[SourceID]hermesMetadataObservation)
	for _, id := range snapshot.order {
		indexMetadata[id] = observeHermesIndex(snapshot.sources[id].Manifest)
		snapshot.sources[id] = explainHermesSource(snapshot, id, roots, &inputs, indexMetadata[id])
	}
	applyHermesPrecedence(snapshot, request.Roots, indexMetadata)
	applyHermesExplicitLookup(snapshot, roots, &inputs)
	// Missing/unreadable discovery can hide a same-name candidate or a higher-priority definition.
	uncertain := inputs.uncertain || inputs.policyEnvUnknown["HERMES_SAFE_MODE"] || (request.Project.Selected && inputs.policyEnvUnknown["HERMES_ENABLE_PROJECT_PLUGINS"]) || !snapshot.Complete()
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
	applyNativeProblems(snapshot, inputs.problems, roots)
	applyHermesCapabilities(snapshot, inputs)
	return snapshot, nil
}

type hermesMetadataObservation struct {
	metadata map[string]any
	known    bool
	framed   bool
	yaml     hermesYAMLObservation
}

func (index hermesMetadataObservation) nameKnown() bool {
	return index.known && index.yaml.known("name")
}

func (index hermesMetadataObservation) eligibleKnown() bool {
	return index.known && index.yaml.known("name", "description", "platforms", "environments") && !hermesEnvironmentDeclared(index.metadata)
}

var hermesClosingFence = regexp.MustCompile(`\n---[\s\p{Z}\x{0085}\x{001c}-\x{001f}]*\n`)

func observeHermesIndex(manifest Manifest) hermesMetadataObservation {
	return observeHermesMetadata(manifest, 4000)
}

func observeHermesMetadata(manifest Manifest, window int) hermesMetadataObservation {
	if manifest.Status != ManifestParsed {
		return hermesMetadataObservation{}
	}
	text := strings.ReplaceAll(manifest.RawText, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if window > 0 {
		chars := []rune(text)
		text = string(chars[:min(len(chars), window)])
	}
	if !strings.HasPrefix(text, "---") {
		return hermesMetadataObservation{metadata: map[string]any{}, known: true, yaml: observeHermesYAML([]byte("{}"))}
	}
	closing := hermesClosingFence.FindStringIndex(text[3:])
	if closing == nil {
		return hermesMetadataObservation{metadata: map[string]any{}, known: true, yaml: observeHermesYAML([]byte("{}"))}
	}
	raw := []byte(text[3 : closing[0]+3])
	metadata, _, err := decodeMetadata(raw)
	return hermesMetadataObservation{metadata: metadata, known: err == nil, framed: true, yaml: observeHermesYAML(raw)}
}

func explainHermesSource(snapshot *Snapshot, id SourceID, roots map[RootID]DiscoveryRoot, inputs *hermesInputs, index hermesMetadataObservation) Source {
	source := snapshot.sources[id]
	full := observeHermesMetadata(source.Manifest, 0)
	name := source.Manifest.Name
	if name == "" {
		name = filepath.Base(source.Directory)
	}
	name = string([]rune(name)[:min(len([]rune(name)), 64)])
	value := hermesEvidence(EvidenceInferred, "hermes-skill-index", source.Manifest.Path, "", "active profile precedes configured external directories; this is inferred index visibility, not observed session use")
	explanation := AvailabilityExplanation{State: AvailabilityUnavailable, NativeName: name, Evidence: &value, Reason: "source has no native discovery reference"}
	nameValue := hermesEvidence(EvidenceInferred, "frontmatter-name-or-directory", source.Manifest.Path, "name", "native index uses metadata from the first 4000 decoded characters, truncating name to 64 characters and falling back to directory name")
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
			metadata := full.metadata
			nativeIndex := root.Origin == "hermes-profile" || root.Origin == "hermes-external"
			if nativeIndex {
				metadata = index.metadata
			}
			referenceName := hermesReferenceName(metadata, ref.DiscoveredPath)
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
				hermesPluginFact(&fact, &explanation, source, ref, plugin, inputs, full)
			}
			platformKnown := full.known && full.yaml.known("platforms")
			if nativeIndex {
				platformKnown = index.known && index.yaml.known("platforms")
			}
			if platformKnown && !hermesPlatformMatches(metadata["platforms"]) {
				fact.Participation = ParticipationExcluded
				fact.Evidence = hermesEvidence(EvidenceInferred, "os-platform", source.Manifest.Path, "platforms", "frontmatter excludes inspected operating system "+runtime.GOOS)
			}
			if plugin, ok := inputs.plugins[root.ID]; ok {
				if explanation.NativeNameEvidence.Kind != EvidenceUnknown {
					nativeNames[plugin.namespace+":"+filepath.Base(ref.DiscoveredPath)] = true
				}
			} else if index.nameKnown() {
				nativeNames[referenceName] = true
			} else {
				explanation.NativeName = ""
				explanation.NativeNameEvidence = &Evidence{Kind: EvidenceUnknown, Rule: "native-name-interpretation", InputPath: source.Manifest.Path, AgentVersion: hermesValidatedVersion, Reason: "native YAML name interpretation is outside supported scalar semantics"}
				fact.Permission.State = PermissionUnknown
				fact.Permission.Evidence = *explanation.NativeNameEvidence
			}
			known := source.Manifest.Status == ManifestParsed && platformKnown
			if nativeIndex {
				known = index.eligibleKnown()
				if root.Origin == "hermes-external" && !inputs.configYAML.known("skills.external_dirs") {
					known = false
				}
			}
			if !known && fact.Participation != ParticipationExcluded {
				fact.Participation = ParticipationUnknown
				fact.Evidence = hermesEvidence(EvidenceUnknown, "native-index-eligibility", source.Manifest.Path, "", "native metadata interpretation or index-window environment eligibility cannot be established")
			} else if nativeIndex && platformKnown && hermesPlatformMatches(metadata["platforms"]) && full.known && full.yaml.known("platforms") && !hermesPlatformMatches(full.metadata["platforms"]) {
				explanation.Invocation.Evidence.Reason = "native index OS eligibility differs from full-content prompt filtering; running-session offers remain unknown"
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
		explanation.Precedence = &PrecedenceExplanation{State: PrecedenceUnknown, Evidence: hermesEvidence(EvidenceUnknown, "multiple-native-names", source.Manifest.Path, "", explanation.Reason)}
	}
	if explanation.State == AvailabilityUnavailable {
		explanation.Reason = "native source excluded, inactive, or inspection-only; see reference rules"
	}
	if explanation.NativeNameEvidence.Kind == EvidenceUnknown {
		explanation.Precedence = &PrecedenceExplanation{State: PrecedenceUnknown, Evidence: *explanation.NativeNameEvidence}
	}
	promptKnown := full.known && full.yaml.known("platforms", "environments", "metadata.hermes.requires_tools", "metadata.hermes.requires_toolsets", "metadata.hermes.fallback_for_tools", "metadata.hermes.fallback_for_toolsets")
	if (!promptKnown || hermesEnvironmentDeclared(full.metadata) || hermesConditional(full.metadata)) && explanation.State == AvailabilityAvailable {
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
func hermesReferenceName(metadata map[string]any, directory string) string {
	name, _ := metadata["name"].(string)
	if name == "" {
		name = filepath.Base(directory)
	}
	chars := []rune(name)
	return string(chars[:min(len(chars), 64)])
}
func hermesPermission(name string, inputs *hermesInputs) PermissionExplanation {
	state, reason := PermissionAllowed, "not in global or selected messaging-platform disabled list"
	skills := hermesMap(inputs.configData, "skills")
	global, ok := hermesRawSkillStrings(skills["disabled"])
	platforms := hermesMap(skills, "platform_disabled")
	selected, selectedOK := hermesRawSkillStrings(platforms[inputs.platform])
	globalKnown := ok && inputs.configYAML.known("skills.disabled")
	selectedKnown := inputs.platformKnown && (inputs.platform == "" || (selectedOK && inputs.configYAML.known("skills.platform_disabled."+inputs.platform)))
	if inputs.uncertain {
		state, reason = PermissionUnknown, "profile config native string semantics cannot be reliably interpreted"
	} else if (globalKnown && hermesHas(global, name)) || (inputs.platform != "" && selectedKnown && hermesHas(selected, name)) {
		state, reason = PermissionDenied, "name disabled by skills.disabled union skills.platform_disabled for "+inputs.platform
	} else if !globalKnown || !selectedKnown {
		state, reason = PermissionUnknown, "profile config native string semantics cannot be reliably interpreted"
	}
	kind := EvidenceInferred
	if state == PermissionUnknown {
		kind = EvidenceUnknown
	}
	return PermissionExplanation{State: state, Evidence: hermesEvidence(kind, "disabled-name-union", inputs.config, "skills.disabled, skills.platform_disabled."+inputs.platform, reason), Layers: cloneLayers(inputs.scope.ConfigLayers)}
}
func hermesFieldFalsy(value any) bool {
	switch value := value.(type) {
	case nil:
		return true
	case bool:
		return !value
	case int:
		return value == 0
	case int64:
		return value == 0
	case uint64:
		return value == 0
	case float64:
		return value == 0
	case string:
		return value == ""
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	}
	return false
}
func hermesPlatformMatches(value any) bool {
	if hermesFieldFalsy(value) {
		return true
	}
	list, ok := value.([]any)
	if !ok {
		list = []any{value}
	}
	osName := map[string]string{"darwin": "darwin", "windows": "win32", "linux": "linux"}[runtime.GOOS]
	for _, item := range list {
		tag, ok := item.(string)
		if !ok {
			continue
		}
		tag = strings.ToLower(strings.TrimFunc(tag, hermesDotenvSpace))
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
	if hermesFieldFalsy(value) {
		return false
	}
	list, ok := hermesStrings(value)
	return !ok || len(list) > 0
}

var hermesExcludedDirs = map[string]bool{".git": true, ".github": true, ".hub": true, ".archive": true, ".venv": true, "venv": true, "node_modules": true, "site-packages": true, "__pycache__": true, ".tox": true, ".nox": true, ".pytest_cache": true, ".mypy_cache": true, ".ruff_cache": true}

func hermesExcluded(ref Reference, root DiscoveryRoot) string {
	for _, part := range strings.Split(filepath.Join(ref.DiscoveredPath, "SKILL.md"), string(filepath.Separator)) {
		if hermesExcludedDirs[part] {
			return "native index excludes dependency, archive and metadata directories"
		}
	}
	return hermesOrgExcluded(ref, root)
}
func hermesOrgExcluded(ref Reference, root DiscoveryRoot) string {
	rel, _ := filepath.Rel(root.Path, ref.DiscoveredPath)
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) > 1 && parts[0] == "_org" {
		marker := filepath.Join(root.Path, "_org", ".active_org")
		raw, err := nativeBytes(marker)
		if err != nil || strings.TrimSpace(string(raw)) != parts[1] {
			return "organization mirror is inactive; .active_org must match this organization"
		}
	}
	return ""
}
func applyHermesPrecedence(snapshot *Snapshot, roots []DiscoveryRoot, indexMetadata map[SourceID]hermesMetadataObservation) {
	type candidate struct {
		source    SourceID
		reference Reference
		rank      int
		certain   bool
		nameKnown bool
	}
	groups := make(map[string][]candidate)
	var unnamed []candidate
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
			for _, fact := range source.Availability.References {
				if fact.Reference != rid || (fact.Participation != ParticipationEligible && fact.Participation != ParticipationUnknown) || fact.Permission.State == PermissionDenied {
					continue
				}
				index := indexMetadata[source.ID]
				current := candidate{source: source.ID, reference: ref, rank: rank, certain: fact.Participation == ParticipationEligible && fact.Permission.State == PermissionAllowed, nameKnown: index.nameKnown()}
				if current.nameKnown {
					name := hermesReferenceName(index.metadata, ref.DiscoveredPath)
					groups[name] = append(groups[name], current)
				} else {
					unnamed = append(unnamed, current)
				}
			}
		}
	}
	for _, candidates := range groups {
		candidates = append(candidates, unnamed...)
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].rank != candidates[j].rank {
				return candidates[i].rank < candidates[j].rank
			}
			return filepath.Join(candidates[i].reference.DiscoveredPath, "SKILL.md") < filepath.Join(candidates[j].reference.DiscoveredPath, "SKILL.md")
		})
		possible := make(map[SourceID]bool)
		certain := false
		for _, candidate := range candidates {
			possible[candidate.source] = true
			if candidate.certain {
				certain = true
				break
			}
		}
		var winner SourceID
		if certain && len(possible) == 1 {
			for id := range possible {
				winner = id
			}
		}
		unique := make([]candidate, 0, len(candidates))
		seen := make(map[SourceID]bool)
		for _, candidate := range candidates {
			if !seen[candidate.source] {
				seen[candidate.source] = true
				unique = append(unique, candidate)
			}
		}
		candidates = unique
		var ids []SourceID
		var named []candidate
		for _, candidate := range candidates {
			ids = appendUniqueSourceIDs(ids, candidate.source)
			if candidate.nameKnown {
				named = append(named, candidate)
			}
		}
		for _, id := range ids {
			source := snapshot.sources[id]
			if source.Availability.NativeNameEvidence.Rule == "multiple-native-names" || !indexMetadata[id].nameKnown() {
				continue
			}
			state := PrecedenceUnique
			reason := "native index uses active profile first, then external_dirs in configuration order; lexically sorted paths break ties"
			if len(ids) > 1 {
				state = PrecedenceCoexisting
			}
			if len(named) > 1 && named[0].rank == named[1].rank && roots[named[0].rank].Origin == "hermes-profile" {
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
			kind := EvidenceInferred
			if winner == "" {
				state, kind = PrecedenceUnknown, EvidenceUnknown
				reason = "native index eligibility leaves the effective winner indeterminate; metadata, permission or index-window environment eligibility is unknown"
				if source.Availability.State == AvailabilityAvailable {
					source.Availability.State = AvailabilityUnknown
					source.Availability.Reason = reason
				}
			}
			source.Availability.Precedence = &PrecedenceExplanation{State: state, Winner: winner, RelatedSources: related, Evidence: hermesEvidence(kind, "native-index-first-name", source.Manifest.Path, "", reason)}
			if winner != "" && id != winner && source.Availability.State == AvailabilityAvailable {
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

var hermesExplicitNamespace = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func hermesExplicitPathValid(name string) bool {
	candidate := strings.TrimFunc(name, hermesDotenvSpace)
	if strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, `\\`) || (len(candidate) >= 2 && candidate[1] == ':' && ((candidate[0] >= 'a' && candidate[0] <= 'z') || (candidate[0] >= 'A' && candidate[0] <= 'Z'))) {
		return false
	}
	for _, part := range strings.FieldsFunc(candidate, func(char rune) bool { return char == '/' || char == '\\' }) {
		if part == ".." {
			return false
		}
	}
	return true
}

func hermesExplicitSupportPath(path, root string, manifests map[string]bool) (bool, bool) {
	known := true
	for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		switch filepath.Base(parent) {
		case "references", "templates", "assets", "scripts":
			owner := filepath.Dir(parent)
			if manifests[owner] {
				return true, true
			}
			if hermesPathWithin(owner, root) {
				_, err := os.Stat(filepath.Join(owner, "SKILL.md"))
				if err == nil {
					return true, true
				}
				if hermesManifestAbsent(err) {
					continue
				}
			}
			known = false
		}
	}
	return false, known
}

func applyHermesExplicitLookup(snapshot *Snapshot, roots map[RootID]DiscoveryRoot, inputs *hermesInputs) {
	matches := make(map[string]map[string]bool)
	directPaths := make(map[string]string)
	manifestDirs := make(map[string]bool)
	searchRoots := make(map[string]bool)
	selectors := make(map[SourceID]map[string]bool)
	unknownNames := make(map[SourceID]bool)
	unknownPaths := make(map[string]bool)
	scopeKnown := snapshot.Complete() && !inputs.uncertain && inputs.configYAML.known("skills.external_dirs")
	for _, rid := range snapshot.referenceOrder {
		ref := snapshot.references[rid]
		root := roots[ref.Root]
		if ref.Source == "" || (root.Origin != "hermes-profile" && root.Origin != "hermes-external") {
			continue
		}
		if selectors[ref.Source] == nil {
			selectors[ref.Source] = make(map[string]bool)
		}
		source := snapshot.sources[ref.Source]
		path := source.Manifest.ResolvedPath
		searchKnown := root.Origin == "hermes-profile" || inputs.configYAML.known("skills.external_dirs")
		manifestDirs[filepath.Clean(ref.DiscoveredPath)] = true
		searchRoots[root.Path] = searchKnown
		if searchKnown && path != "" {
			directPaths[filepath.Clean(ref.DiscoveredPath)] = path
		}
		if hermesOrgExcluded(ref, root) != "" {
			continue
		}
		add := func(name string) {
			selectors[ref.Source][name] = true
			if path == "" || !searchKnown {
				return
			}
			if matches[name] == nil {
				matches[name] = make(map[string]bool)
			}
			matches[name][path] = true
		}
		add(filepath.Base(ref.DiscoveredPath))
		full := observeHermesMetadata(source.Manifest, 0)
		if full.nameKnown() {
			if name, ok := full.metadata["name"].(string); ok && name != "" {
				add(name)
			}
		} else {
			unknownNames[ref.Source] = true
		}
		if !full.nameKnown() || !searchKnown {
			unknownPaths[path] = true
		}
	}
	for id, names := range selectors {
		source := snapshot.sources[id]
		state, kind := "unambiguous-bare-name", EvidenceInferred
		var ordered []string
		for name := range names {
			ordered = append(ordered, name)
		}
		sort.Strings(ordered)
		var facts, settings []string
		if len(ordered) == 0 {
			state = "not-in-native-scope"
			facts = append(facts, "native traversal excludes this source's references")
		} else if unknownNames[id] {
			state, kind = "unknown", EvidenceUnknown
			facts = append(facts, "additional full-content lookup names cannot be established")
		}
		for _, name := range ordered {
			setting := fmt.Sprintf("%q", name)
			settings = append(settings, setting)
			lookupNames := []string{name}
			valid := hermesExplicitPathValid(name)
			if namespace, bare, qualified := strings.Cut(name, ":"); qualified {
				valid = valid && hermesExplicitNamespace.MatchString(namespace)
				observedDispatch := false
				for _, plugin := range inputs.plugins {
					observedDispatch = observedDispatch || (plugin.admitted && plugin.state != ParticipationExcluded && plugin.namespace == namespace)
				}
				if valid && observedDispatch {
					facts = append(facts, fmt.Sprintf("selector %s: unknown (plugin namespace dispatch precedes local lookup; registration/loading was not observed)", setting))
					if state != "ambiguous-bare-name" {
						state, kind = "unknown", EvidenceUnknown
					}
					continue
				}
				if bare != "" {
					categorized := namespace + "/" + bare
					valid = valid && hermesExplicitPathValid(categorized)
					lookupNames = append(lookupNames, categorized)
				}
				facts = append(facts, fmt.Sprintf("selector %s local-tree facts are conditional on namespace dispatch falling through; unobserved plugin registrations are not known absent", setting))
			}
			if !valid {
				facts = append(facts, fmt.Sprintf("selector %s: invalid-selector (native relative-path/namespace validation rejects it before local matching)", setting))
				if state != "ambiguous-bare-name" {
					state, kind = "unknown", EvidenceUnknown
				}
				continue
			}
			certain := scopeKnown
			for _, lookup := range lookupNames {
				for root, known := range searchRoots {
					candidate := filepath.Join(root, lookup)
					if !hermesPathWithin(candidate, root) {
						continue
					}
					if directPaths[candidate] == "" && manifestDirs[filepath.Dir(candidate)] {
						switch filepath.Base(candidate) {
						case "references", "templates", "assets", "scripts":
							certain = false
						}
					}
					if path := directPaths[candidate]; known && path != "" {
						support, supportKnown := hermesExplicitSupportPath(candidate, root, manifestDirs)
						if support {
							continue
						}
						if !supportKnown {
							if !matches[name][path] {
								certain = false
								facts = append(facts, fmt.Sprintf("selector %s: support ownership along direct path %q is unobserved", setting, candidate))
							}
							continue
						}
						if matches[name] == nil {
							matches[name] = make(map[string]bool)
						}
						matches[name][path] = true
					}
				}
				for _, part := range strings.Split(lookup, string(filepath.Separator)) {
					if hermesExcludedDirs[part] {
						certain = false
					}
				}
			}
			for path := range unknownPaths {
				if !matches[name][path] {
					certain = false
				}
			}
			count := len(matches[name])
			selectorState := "unambiguous-bare-name"
			switch {
			case count > 1:
				selectorState = "ambiguous-bare-name"
				state, kind = selectorState, EvidenceInferred
			case !certain || count == 0:
				selectorState = "unknown"
				if state != "ambiguous-bare-name" {
					state, kind = selectorState, EvidenceUnknown
				}
			}
			facts = append(facts, fmt.Sprintf("selector %s: %s (%d known distinct manifests)", setting, selectorState, count))
		}
		reason := source.Availability.Invocation.Evidence.Reason + ". Native explicit lookup within the observed SKILL.md scope checks collisions before platform and disabled policy; match counts do not establish loading success. " + strings.Join(facts, "; ")
		source.Availability.Invocation = &SessionExplanation{State: state, Evidence: hermesEvidence(kind, "native-explicit-selectors", source.Manifest.Path, strings.Join(settings, ", "), reason)}
		snapshot.sources[id] = source
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
