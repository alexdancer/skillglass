package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type claudePluginFact struct {
	directory     string
	name          string
	applicable    bool
	participation ParticipationState
	activation    Evidence
	provenance    Provenance
}
type claudeLocalPluginSource struct {
	path, limitation string
	input            string
	registry         string
	updated          json.RawMessage
	declaredSource   string
}
type claudeInstallation struct {
	Scope       string          `json:"scope"`
	InstallPath string          `json:"installPath"`
	ProjectPath json.RawMessage `json:"projectPath"`
	Version     json.RawMessage `json:"version"`
}

func discoverClaudePlugins(request *Request, inputs *claudeInputs) map[RootID][]claudePluginFact {
	registry := filepath.Join(request.claudeHome, "plugins", "installed_plugins.json")
	var installed struct {
		Version int                             `json:"version"`
		Plugins map[string][]claudeInstallation `json:"plugins"`
	}
	raw, err := nativeBytes(registry)
	inputs.managers = append(inputs.managers, pluginRegistryManagers(registry, raw, err, 2, claudeEvidence(EvidenceObserved, "installed-plugin-registry", registry, "installPath", "registered whole installation boundaries; activation does not transfer ownership"))...)
	if !nativeAbsent(registry, err) && (err != nil || json.Unmarshal(raw, &installed) != nil || installed.Version != 2 || installed.Plugins == nil) {
		inputs.problem(registry, "plugin installation registry is unreadable or unsupported")
		installed.Plugins = nil
	}
	localSources := claudeLocalPluginSources(request, inputs)
	directories := map[string]map[string]bool{}
	observations := map[string][]PluginRecord{}
	observe := func(path, id string, record PluginRecord) {
		inputs.managers = append(inputs.managers, ManagerEvidence{Manager: id, Boundary: path, State: ManagerBoundary, Evidence: claudeEvidence(EvidenceObserved, "plugin-installation", record.InputPath, "", "observed cache or registered local source owns the whole installation")})
		if directories[path] == nil {
			directories[path] = map[string]bool{}
		}
		directories[path][id] = true
		observations[id] = append(observations[id], record)
	}
	for id, sources := range localSources {
		for _, source := range sources {
			observe(source.path, id, PluginRecord{Kind: "marketplace", InputPath: source.input, Path: source.path, RegistrationInput: source.registry, LastUpdated: source.updated, DeclaredSource: source.declaredSource})
		}
	}
	for id, entries := range installed.Plugins {
		for _, entry := range entries {
			if entry.Scope != "user" && entry.Scope != "project" && entry.Scope != "local" {
				inputs.problem(registry, "plugin scope is missing or unsupported: "+id)
			}
			projectPath, projectOK := claudeManifestString(entry.ProjectPath)
			if (len(entry.ProjectPath) > 0 || entry.Scope == "project" || entry.Scope == "local") && (!projectOK || !filepath.IsAbs(projectPath)) {
				inputs.problem(registry, "plugin projectPath must be an absolute string: "+id)
			}
			if len(entry.Version) > 0 {
				if _, ok := claudeManifestString(entry.Version); !ok {
					inputs.problem(registry, "plugin version must be a non-null string: "+id)
				}
			}
			applicability := "nonapplicable"
			if entry.Scope == "user" || ((entry.Scope == "project" || entry.Scope == "local") && request.Project.Selected && sameClaudePath(projectPath, request.Project.Path)) {
				applicability = "applicable"
			}
			if (entry.Scope != "user" && entry.Scope != "project" && entry.Scope != "local") || ((len(entry.ProjectPath) > 0 || entry.Scope == "project" || entry.Scope == "local") && (!projectOK || !filepath.IsAbs(projectPath))) {
				applicability = "unknown"
			}
			record := PluginRecord{Kind: "installation", InputPath: registry, Path: entry.InstallPath, Scope: entry.Scope, ProjectPath: entry.ProjectPath, Version: entry.Version, Applicability: applicability}
			if !filepath.IsAbs(entry.InstallPath) {
				inputs.problem(registry, "plugin installPath must be absolute: "+id)
				record.Applicability = "unknown"
				observations[id] = append(observations[id], record)
				continue
			}
			observe(filepath.Clean(entry.InstallPath), id, record)
		}
	}
	cache := filepath.Join(request.claudeHome, "plugins", "cache")
	markets, err := os.ReadDir(cache)
	if err != nil && !os.IsNotExist(err) {
		inputs.managers = append(inputs.managers, ManagerEvidence{Manager: "plugin-cache", Boundary: cache, State: ManagerUncertain, Evidence: claudeEvidence(EvidenceUnknown, "plugin-cache-inventory", cache, "", "plugin cache could not be listed")})
		inputs.problem(cache, "plugin cache could not be listed")
	}
	// The cache has exactly marketplace/plugin/version levels. It is never an activation registry.
	for _, market := range markets {
		if !market.IsDir() {
			continue
		}
		marketDir := filepath.Join(cache, market.Name())
		names, err := os.ReadDir(marketDir)
		if err != nil {
			inputs.managers = append(inputs.managers, ManagerEvidence{Manager: market.Name(), Boundary: marketDir, State: ManagerUncertain, Evidence: claudeEvidence(EvidenceUnknown, "plugin-cache-inventory", marketDir, "", "plugin cache could not be listed")})
			inputs.problem(marketDir, "plugin cache could not be listed")
			continue
		}
		for _, name := range names {
			if !name.IsDir() {
				continue
			}
			nameDir := filepath.Join(marketDir, name.Name())
			versions, err := os.ReadDir(nameDir)
			if err != nil {
				inputs.managers = append(inputs.managers, ManagerEvidence{Manager: name.Name(), Boundary: nameDir, State: ManagerUncertain, Evidence: claudeEvidence(EvidenceUnknown, "plugin-cache-inventory", nameDir, "", "plugin versions could not be listed")})
				inputs.problem(nameDir, "plugin versions could not be listed")
				continue
			}
			for _, version := range versions {
				if version.IsDir() {
					path := filepath.Join(nameDir, version.Name())
					observe(path, name.Name()+"@"+market.Name(), PluginRecord{Kind: "cache", InputPath: cache, Path: path})
				}
			}
		}
	}
	paths := make([]string, 0, len(directories))
	for path := range directories {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	byPath := map[string][]claudePluginFact{}
	localRoots := map[string]bool{}
	for _, directory := range paths {
		ids := make([]string, 0, len(directories[directory]))
		for id := range directories[directory] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			name := strings.SplitN(id, "@", 2)[0]
			fact := claudePluginFact{name: name, participation: ParticipationUnknown, activation: claudeEvidence(EvidenceUnknown, "plugin-activation-unverified", registry, "enabledPlugins."+id, "cache, installation registry and enabled setting do not prove native activation; this activation form was not established in native validation")}
			var manifest struct {
				Name    string          `json:"name"`
				Version string          `json:"version"`
				Skills  json.RawMessage `json:"skills"`
			}
			manifestPath := filepath.Join(directory, ".claude-plugin", "plugin.json")
			manifestRaw, manifestErr := nativeBytes(manifestPath)
			topologyMatches := manifestErr == nil && json.Unmarshal(manifestRaw, &manifest) == nil && manifest.Name == name
			manifestLimitation := ""
			if topologyMatches {
				manifestLimitation = claudePluginManifestLimitation(manifestRaw)
				if manifestLimitation == "" && (name == "" || strings.ContainsAny(name, " :")) {
					manifestLimitation = "plugin name identifier is empty or contains ASCII space or colon; native activation for this identifier is unverified"
				}
			}
			fact.directory, _ = filepath.EvalSymlinks(directory)
			fact.provenance = Provenance{Kind: "plugin", PluginID: id, Version: manifest.Version, Records: observations[id], Evidence: claudeEvidence(EvidenceObserved, "managed-plugin-content", manifestPath, "", "plugin manifest and manager cache/registry locate managed content; ownership is separate from activation")}
			if !topologyMatches {
				fact.provenance.Evidence.Kind = EvidenceUnknown
				fact.provenance.Evidence.Reason = "cache or registry indicates managed provenance; matching plugin manifest is not established"
			}
			localSource := claudeLocalPluginSource{}
			localIdentities := map[string]bool{}
			localLimitation := ""
			for _, source := range localSources[id] {
				identity := source.path
				if token, err := directoryObjectToken(source.path); err == nil {
					identity = token.String()
				}
				localIdentities[identity] = true
				if localSource.path == "" || sameClaudePath(source.path, directory) {
					localSource = source
				}
				if source.limitation != "" {
					localLimitation = source.limitation
				}
			}
			localSource.limitation = localLimitation
			active, applicable, registered := false, false, false
			for _, entry := range installed.Plugins[id] {
				registered = true
				projectPath, _ := claudeManifestString(entry.ProjectPath)
				version, _ := claudeManifestString(entry.Version)
				if entry.Scope == "user" || ((entry.Scope == "project" || entry.Scope == "local") && request.Project.Selected && sameClaudePath(projectPath, request.Project.Path)) {
					applicable = true
					if sameClaudePath(entry.InstallPath, directory) && (!topologyMatches || version == "" || manifest.Version == "" || version == manifest.Version) {
						active = true
					}
				}
			}
			fact.applicable = registered && applicable && (active || sameClaudePath(localSource.path, directory))
			if setting, ok := inputs.plugins[id]; ok {
				e := claudeEvidence(EvidenceObserved, "plugin-enabled-setting", setting.path, "enabledPlugins."+id+"="+setting.value, "registration-specific configured plugin setting")
				fact.provenance.Setting = &e
			}
			if setting, ok := inputs.plugins[id]; ok && setting.value == "false" {
				fact.participation = ParticipationExcluded
				fact.activation = claudeEvidence(EvidenceInferred, "plugin-disabled", setting.path, "enabledPlugins."+id+"=false", "configured plugin is inactive; cached content remains")
			} else if localSource.limitation != "" {
				fact.activation = claudeEvidence(EvidenceUnknown, "local-marketplace-unverified", filepath.Join(request.claudeHome, "plugins", "known_marketplaces.json"), "lastUpdated", localSource.limitation)
			} else if len(localIdentities) > 1 && applicable {
				fact.activation = claudeEvidence(EvidenceUnknown, "multiple-local-plugin-sources", registry, id, "multiple independent marketplace source declarations share this registration; native source selection is unverified")
			} else {
				if localSource.path != "" && sameClaudePath(localSource.path, directory) && topologyMatches && manifestLimitation == "" && applicable && !inputs.uncertain {
					setting, enabled := inputs.plugins[id]
					if enabled && setting.value == "true" {
						fact.participation = ParticipationEligible
						fact.activation = claudeEvidence(EvidenceInferred, "local-marketplace-plugin", setting.path, "enabledPlugins."+id+"=true", "Claude 2.1.282 native initialization loads the registered local marketplace source, not its independent installed cache copy; known marketplace, source manifest, applicable installation and enabled setting agree")
					}
				} else if localSource.path != "" && !sameClaudePath(localSource.path, directory) && registered && applicable && !inputs.uncertain {
					fact.participation = ParticipationExcluded
					fact.activation = claudeEvidence(EvidenceInferred, "local-plugin-cache-copy", registry, "installPath", "local marketplace source is the native plugin source; this independent cache copy does not supply another command")
				} else if registered && !inputs.uncertain && (!applicable || (localSource.path == "" && !active)) {
					fact.participation = ParticipationExcluded
					fact.activation = claudeEvidence(EvidenceInferred, "inactive-plugin-installation", registry, "installPath / projectPath", "this cached version or project-scoped installation is not the applicable registered installation")
				}
			}
			if fact.participation == ParticipationUnknown && manifestLimitation != "" {
				fact.activation = claudeEvidence(EvidenceUnknown, "plugin-manifest-unverified", manifestPath, "", manifestLimitation)
			}
			skillsDirectory := filepath.Join(directory, "skills")
			roots := []string{skillsDirectory}
			if topologyMatches && len(manifest.Skills) == 0 {
				if _, err := os.Stat(skillsDirectory); os.IsNotExist(err) {
					if info, err := os.Stat(filepath.Join(directory, "SKILL.md")); err == nil && info.Mode().IsRegular() {
						roots = append(roots, directory)
					}
				}
			}
			if topologyMatches && len(manifest.Skills) > 0 {
				var one string
				var extras []string
				if json.Unmarshal(manifest.Skills, &one) == nil {
					extras = []string{one}
				} else if json.Unmarshal(manifest.Skills, &extras) != nil {
					inputs.problem(manifestPath, "plugin skills paths must be string or array")
					if fact.participation != ParticipationExcluded {
						fact.participation = ParticipationUnknown
					}
				}
				for _, extra := range extras {
					candidate := filepath.Clean(filepath.Join(directory, extra))
					if !claudeWithin(directory, candidate) || filepath.IsAbs(extra) {
						inputs.problem(manifestPath, "configured plugin skill path escapes plugin")
						continue
					}
					resolved, err := filepath.EvalSymlinks(candidate)
					if err == nil && fact.directory != "" && !claudeWithin(fact.directory, resolved) {
						inputs.problem(candidate, "configured plugin skill path resolves outside plugin")
						continue
					}
					roots = append(roots, candidate)
				}
			}
			fact.provenance.Activation = &fact.activation
			for _, root := range roots {
				duplicate := false
				for _, existing := range byPath[root] {
					duplicate = duplicate || (existing.provenance.PluginID == id && existing.provenance.Evidence.InputPath == manifestPath)
				}
				if !duplicate {
					byPath[root] = append(byPath[root], fact)
				}
				localRoots[root] = localRoots[root] || sameClaudePath(localSource.path, directory)
			}
		}
	}
	facts := map[RootID][]claudePluginFact{}
	paths = paths[:0]
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		origin := "claude-plugin-cache"
		if localRoots[path] {
			origin = "claude-plugin-local"
		}
		appendClaudeRoot(request, path, ContextManaged, origin)
		facts[request.Roots[len(request.Roots)-1].ID] = byPath[path]
	}
	return facts
}
func sameClaudePath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	l, le := filepath.EvalSymlinks(left)
	r, re := filepath.EvalSymlinks(right)
	if le == nil && re == nil {
		return l == r
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
func claudeWithin(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func managedClaudeSource(directory string, plugins map[RootID][]claudePluginFact) []claudePluginFact {
	var result []claudePluginFact
	seen := map[string]bool{}
	for _, facts := range plugins {
		for _, fact := range facts {
			key := fact.provenance.PluginID + "\x00" + fact.provenance.Evidence.InputPath
			if !seen[key] && fact.directory != "" && claudeWithin(fact.directory, directory) {
				seen[key] = true
				result = append(result, fact)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].provenance, result[j].provenance
		if left.PluginID != right.PluginID {
			return left.PluginID < right.PluginID
		}
		return left.Evidence.InputPath < right.Evidence.InputPath
	})
	return result
}

func claudeLocalPluginSources(request *Request, inputs *claudeInputs) map[string][]claudeLocalPluginSource {
	result := map[string][]claudeLocalPluginSource{}
	path := filepath.Join(request.claudeHome, "plugins", "known_marketplaces.json")
	coverage := startManagerRead(&inputs.managerReaders, "claude-marketplaces", "local-marketplaces", path, "", claudeEvidence(EvidenceObserved, "local-marketplace-registry", path, "", ""))
	defer coverage.finish(&inputs.managers)
	raw, err := nativeBytes(path)
	if err == nil {
		coverage.result.SHA256 = managerInputDigest(raw)
	}
	if nativeAbsent(path, err) {
		coverage.absent()
		return result
	}
	var markets map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &markets) != nil || markets == nil {
		coverage.uncertain("local marketplace registry cannot be interpreted; external installation boundaries cannot be established")
		inputs.problem(path, "known plugin marketplaces could not be interpreted")
		return result
	}
	coverage.complete(raw)
	names := make([]string, 0, len(markets))
	for name := range markets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var market struct {
			Source *struct {
				Source *string
				Path   string
			}
			InstallLocation string          `json:"installLocation"`
			LastUpdated     json.RawMessage `json:"lastUpdated"`
		}
		if json.Unmarshal(markets[name], &market) != nil || market.Source == nil || market.Source.Source == nil {
			coverage.uncertain("local marketplace record or source discriminator cannot be interpreted: " + name)
			inputs.problem(path, "known plugin marketplace source could not be interpreted: "+name)
			continue
		}
		switch *market.Source.Source {
		case "github", "git", "url", "npm":
			continue // Nonlocal registration does not own external local content.
		case "directory":
		default:
			coverage.uncertain("unsupported marketplace source discriminator: " + name)
			inputs.problem(path, "known plugin marketplace source could not be interpreted: "+name)
			continue
		}
		directory := market.Source.Path
		if !filepath.IsAbs(directory) || !sameClaudePath(directory, market.InstallLocation) {
			coverage.uncertain("local marketplace location cannot be bounded: " + name)
			inputs.scope.Gaps = append(inputs.scope.Gaps, "unverified local marketplace location: "+name)
			continue
		}
		updated, supported := claudeManifestString(market.LastUpdated)
		limitation := ""
		if _, err := time.Parse(time.RFC3339, updated); !supported || err != nil {
			limitation = "local marketplace lastUpdated is missing or unsupported: " + name + "; native local-source selection is unverified"
			inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, limitation, nil, true))
		}
		for _, plugin := range inputs.claudeMarketplaceSources(name, directory) {
			id := plugin.Name + "@" + name
			result[id] = append(result[id], claudeLocalPluginSource{plugin.Source, limitation, filepath.Join(directory, ".claude-plugin", "marketplace.json"), path, market.LastUpdated, plugin.Relative})
		}
	}
	return result
}

type claudeMarketplaceSource struct{ Name, Source, Relative string }

func (inputs *claudeInputs) claudeMarketplaceSources(name, directory string) []claudeMarketplaceSource {
	path := filepath.Join(directory, ".claude-plugin", "marketplace.json")
	coverage := startManagerRead(&inputs.managerReaders, "claude-marketplace-manifest", name, path, directory, claudeEvidence(EvidenceObserved, "local-marketplace-manifest", path, "plugins", ""))
	defer coverage.finish(&inputs.managers)
	raw, err := nativeBytes(path)
	if err == nil {
		coverage.result.SHA256 = managerInputDigest(raw)
	}
	var manifest struct {
		Name    string
		Plugins []json.RawMessage
	}
	if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.Name != name || manifest.Plugins == nil {
		coverage.uncertain("local plugin declarations cannot be interpreted within this marketplace")
		inputs.scope.Gaps = append(inputs.scope.Gaps, "local marketplace lacks matching manifest: "+name)
		return nil
	}
	coverage.complete(raw)
	var result []claudeMarketplaceSource
	for _, raw := range manifest.Plugins {
		var plugin struct {
			Name   string
			Source json.RawMessage
		}
		if json.Unmarshal(raw, &plugin) != nil {
			coverage.uncertain("local plugin declaration is partial or invalid")
			continue
		}
		if plugin.Name == "" {
			coverage.uncertain("local plugin name is missing or empty; recovered source evidence retained")
		}
		relative, supported := claudeManifestString(plugin.Source)
		if !supported || filepath.IsAbs(relative) {
			coverage.uncertain("unsupported plugin source cannot establish an installation boundary")
			coverage.result.Boundary = ""
			inputs.scope.Gaps = append(inputs.scope.Gaps, "unverified local plugin source form: "+plugin.Name+"@"+name)
			continue
		}
		source := filepath.Clean(filepath.Join(directory, relative))
		if !claudeWithin(directory, source) {
			coverage.uncertain("local plugin source escapes marketplace")
			coverage.result.Boundary = ""
			inputs.scope.Gaps = append(inputs.scope.Gaps, "local plugin source escapes marketplace: "+plugin.Name+"@"+name)
			continue
		}
		result = append(result, claudeMarketplaceSource{plugin.Name, source, relative})
	}
	return result
}

func claudePluginManifestLimitation(raw []byte) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "plugin manifest cannot be decoded"
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := fields[name]
		switch name {
		case "name", "version", "repository":
			if _, ok := claudeManifestString(value); !ok {
				return name + " must be a non-null string; this manifest cannot establish native plugin activation"
			}
		case "homepage":
			homepage, ok := claudeManifestString(value)
			if !ok {
				return "homepage must be a non-null URL string; this manifest cannot establish native plugin activation"
			}
			assessment := assessClaudeHomepage(homepage)
			if assessment.validity != claudeHomepageSupported {
				return assessment.reason
			}
		case "skills":
			const unsupportedPath = "skills paths must be exactly \".\" or start with \"./\"; this manifest cannot establish native plugin activation"
			if path, ok := claudeManifestString(value); ok {
				if path != "." && !strings.HasPrefix(path, "./") {
					return unsupportedPath
				}
				continue
			}
			var paths []json.RawMessage
			if json.Unmarshal(value, &paths) != nil || paths == nil {
				return "skills must be a non-null string or array of non-null strings; this manifest cannot establish native plugin activation"
			}
			for _, rawPath := range paths {
				path, ok := claudeManifestString(rawPath)
				if !ok {
					return "skills array elements must be non-null strings; this manifest cannot establish native plugin activation"
				}
				if path != "." && !strings.HasPrefix(path, "./") {
					return unsupportedPath
				}
			}
		default:
			return "manifest field " + name + " has unmodeled native activation constraints; partial manifest decoding is not full native schema validation"
		}
	}
	return ""
}

// JSON null successfully decodes into a string zero value, but an explicitly
// present null does not have the same native contract as an omitted field.
func claudeManifestString(raw json.RawMessage) (string, bool) {
	var value *string
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return "", false
	}
	return *value, true
}
