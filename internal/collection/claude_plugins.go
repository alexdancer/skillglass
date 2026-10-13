package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type claudePluginFact struct {
	directory     string
	name          string
	participation ParticipationState
	activation    Evidence
	provenance    Provenance
}
type claudeInstallation struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	ProjectPath string `json:"projectPath"`
	Version     string `json:"version"`
}

func discoverClaudePlugins(request *Request, inputs *claudeInputs) map[string]claudePluginFact {
	registry := filepath.Join(request.claudeHome, "plugins", "installed_plugins.json")
	var installed struct {
		Version int                             `json:"version"`
		Plugins map[string][]claudeInstallation `json:"plugins"`
	}
	raw, err := nativeBytes(registry)
	if !nativeAbsent(registry, err) && (err != nil || json.Unmarshal(raw, &installed) != nil || installed.Version != 2 || installed.Plugins == nil) {
		inputs.problem(registry, "plugin installation registry is unreadable or unsupported")
		installed.Plugins = nil
	}
	localSources := claudeLocalPluginSources(request, inputs)
	directories := map[string]string{}
	for id, path := range localSources {
		directories[path] = id
	}
	for id, entries := range installed.Plugins {
		for _, entry := range entries {
			if !filepath.IsAbs(entry.InstallPath) {
				inputs.problem(registry, "plugin installPath must be absolute: "+id)
				continue
			}
			directories[filepath.Clean(entry.InstallPath)] = id
		}
	}
	cache := filepath.Join(request.claudeHome, "plugins", "cache")
	markets, err := os.ReadDir(cache)
	if err != nil && !os.IsNotExist(err) {
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
				inputs.problem(nameDir, "plugin versions could not be listed")
				continue
			}
			for _, version := range versions {
				if version.IsDir() {
					directories[filepath.Join(nameDir, version.Name())] = name.Name() + "@" + market.Name()
				}
			}
		}
	}
	paths := make([]string, 0, len(directories))
	for path := range directories {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	facts := map[string]claudePluginFact{}
	for _, directory := range paths {
		id := directories[directory]
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
		}
		fact.directory, _ = filepath.EvalSymlinks(directory)
		fact.provenance = Provenance{Kind: "plugin", PluginID: id, Version: manifest.Version, Evidence: claudeEvidence(EvidenceObserved, "managed-plugin-content", manifestPath, "", "plugin manifest and manager cache/registry locate managed content; ownership is separate from activation")}
		if !topologyMatches {
			fact.provenance.Evidence.Kind = EvidenceUnknown
			fact.provenance.Evidence.Reason = "cache or registry indicates managed provenance; matching plugin manifest is not established"
		}
		if setting, ok := inputs.plugins[id]; ok && setting.value == "false" {
			fact.participation = ParticipationExcluded
			fact.activation = claudeEvidence(EvidenceInferred, "plugin-disabled", setting.path, "enabledPlugins."+id+"=false", "configured plugin is inactive; cached content remains")
		} else {
			active, applicable, registered := false, false, false
			for _, entry := range installed.Plugins[id] {
				registered = true
				if entry.Scope == "user" || ((entry.Scope == "project" || entry.Scope == "local") && request.Project.Selected && sameClaudePath(entry.ProjectPath, request.Project.Path)) {
					applicable = true
					if sameClaudePath(entry.InstallPath, directory) {
						active = true
						if topologyMatches && entry.Version != "" && manifest.Version != "" && entry.Version != manifest.Version {
							active = false
						}
					}
				}
			}
			localSource := localSources[id]
			if localSource != "" && sameClaudePath(localSource, directory) && topologyMatches && manifestLimitation == "" && applicable && !inputs.uncertain {
				setting, enabled := inputs.plugins[id]
				if enabled && setting.value == "true" {
					fact.participation = ParticipationEligible
					fact.activation = claudeEvidence(EvidenceInferred, "local-marketplace-plugin", setting.path, "enabledPlugins."+id+"=true", "Claude 2.1.282 native initialization loads the registered local marketplace source, not its independent installed cache copy; known marketplace, source manifest, applicable installation and enabled setting agree")
				}
			} else if localSource != "" && !sameClaudePath(localSource, directory) && registered && applicable && !inputs.uncertain {
				fact.participation = ParticipationExcluded
				fact.activation = claudeEvidence(EvidenceInferred, "local-plugin-cache-copy", registry, "installPath", "local marketplace source is the native plugin source; this independent cache copy does not supply another command")
			} else if registered && (!applicable || !active) {
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
				fact.participation = ParticipationUnknown
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
		for _, root := range roots {
			if _, exists := facts[root]; exists {
				continue
			}
			facts[root] = fact
			origin := "claude-plugin-cache"
			if sameClaudePath(localSources[id], directory) {
				origin = "claude-plugin-local"
			}
			appendClaudeRoot(request, root, ContextManaged, origin)
		}
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
func managedClaudeSource(directory string, plugins map[string]claudePluginFact) (claudePluginFact, bool) {
	paths := make([]string, 0, len(plugins))
	for path := range plugins {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fact := plugins[path]
		if fact.directory != "" && claudeWithin(fact.directory, directory) {
			return fact, true
		}
	}
	return claudePluginFact{}, false
}

func claudeLocalPluginSources(request *Request, inputs *claudeInputs) map[string]string {
	result := map[string]string{}
	path := filepath.Join(request.claudeHome, "plugins", "known_marketplaces.json")
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		return result
	}
	var markets map[string]struct {
		Source          struct{ Source, Path string }
		InstallLocation string `json:"installLocation"`
	}
	if err != nil || json.Unmarshal(raw, &markets) != nil || markets == nil {
		inputs.problem(path, "known plugin marketplaces could not be interpreted")
		return result
	}
	for name, market := range markets {
		if market.Source.Source != "directory" {
			continue
		}
		directory := market.Source.Path
		if !filepath.IsAbs(directory) || !sameClaudePath(directory, market.InstallLocation) {
			inputs.scope.Gaps = append(inputs.scope.Gaps, "unverified local marketplace location: "+name)
			continue
		}
		manifestPath := filepath.Join(directory, ".claude-plugin", "marketplace.json")
		data, err := nativeBytes(manifestPath)
		var manifest struct {
			Name    string
			Plugins []struct {
				Name   string
				Source json.RawMessage
			}
		}
		if err != nil || json.Unmarshal(data, &manifest) != nil || manifest.Name != name {
			inputs.scope.Gaps = append(inputs.scope.Gaps, "local marketplace lacks matching manifest: "+name)
			continue
		}
		for _, plugin := range manifest.Plugins {
			var relative string
			if json.Unmarshal(plugin.Source, &relative) != nil || filepath.IsAbs(relative) {
				inputs.scope.Gaps = append(inputs.scope.Gaps, "unverified local plugin source form: "+plugin.Name+"@"+name)
				continue
			}
			source := filepath.Clean(filepath.Join(directory, relative))
			if !claudeWithin(directory, source) {
				inputs.scope.Gaps = append(inputs.scope.Gaps, "local plugin source escapes marketplace: "+plugin.Name+"@"+name)
				continue
			}
			result[plugin.Name+"@"+name] = source
		}
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
			if _, ok := claudeManifestString(value); ok {
				continue
			}
			var paths []json.RawMessage
			if json.Unmarshal(value, &paths) != nil || paths == nil {
				return "skills must be a non-null string or array of non-null strings; this manifest cannot establish native plugin activation"
			}
			for _, path := range paths {
				if _, ok := claudeManifestString(path); !ok {
					return "skills array elements must be non-null strings; this manifest cannot establish native plugin activation"
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
