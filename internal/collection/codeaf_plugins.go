package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type codeafPlugin struct {
	id, version string
	installRoot string
	folders     []string
	permission  PermissionExplanation
}
type codeafInstall struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	ProjectPath string `json:"projectPath"`
	Version     string `json:"version"`
}

func (inputs *codeafInputs) readJSON(path string, destination any) bool {
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		return false
	}
	if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, destination) != nil {
		inputs.problem(path, "CodeAF plugin metadata is unreadable, oversized or invalid; native source skips that JSON input")
		return false
	}
	return true
}
func (inputs *codeafInputs) readSettings(request Request) {
	files := []string{filepath.Join(request.home, ".claude/settings.json")}
	if request.Project.Selected {
		files = append(files, filepath.Join(request.Project.Path, ".claude/settings.json"), filepath.Join(request.Project.Path, ".claude/settings.local.json"))
	}
	for i, path := range files {
		var settings struct {
			EnabledPlugins map[string]json.RawMessage `json:"enabledPlugins"`
		}
		count := len(inputs.problems)
		ok := inputs.readJSON(path, &settings)
		layer := PermissionLayer{Path: path, Scope: "global", State: "absent", Reason: "optional plugin enablement layer absent", Evidence: codeafEvidence(EvidenceObserved, "claude-plugin-setting-layer", path, "enabledPlugins", "only plugin enablement is read; ordinary per-skill permissions are not inherited")}
		if i > 0 {
			layer.Scope = "project"
		}
		if ok {
			layer.State = "parsed"
			layer.Reason = "later enabledPlugins keys replace earlier keys; only JSON true enables a plugin"
			var keys []string
			for key := range settings.EnabledPlugins {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				var enabled bool
				inputs.enabled[key] = json.Unmarshal(settings.EnabledPlugins[key], &enabled) == nil && enabled
				layer.Choices = append(layer.Choices, PermissionChoice{Path: key, Enabled: inputs.enabled[key]})
			}
		} else if len(inputs.problems) > count {
			layer.State = "unreadable"
			layer.Reason = "native source skips invalid JSON, but intended enablement is uncertain"
			inputs.settingsUnknown = true
		}
		inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer)
	}
}
func codeafSamePlace(a, b string) bool {
	if strings.TrimSpace(a) == "" || b == "" {
		return false
	}
	resolve := func(path string) string {
		path, _ = filepath.Abs(path)
		if real, err := filepath.EvalSymlinks(path); err == nil {
			return real
		}
		return filepath.Clean(path)
	}
	return resolve(strings.TrimSpace(a)) == resolve(b)
}
func (inputs *codeafInputs) readPlugins(request Request) (user, project []codeafPlugin) {
	registryPath := filepath.Join(request.home, ".claude/plugins/installed_plugins.json")
	var registry struct {
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if !inputs.readJSON(registryPath, &registry) {
		return nil, nil
	}
	var ids []string
	for id := range registry.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var installs []codeafInstall
		if json.Unmarshal(registry.Plugins[id], &installs) != nil {
			var install codeafInstall
			if json.Unmarshal(registry.Plugins[id], &install) != nil {
				inputs.problem(registryPath, "invalid plugin installation record: "+id)
				continue
			}
			installs = []codeafInstall{install}
		}
		for _, install := range installs {
			root := strings.TrimSpace(install.InstallPath)
			if !filepath.IsAbs(root) {
				inputs.problem(registryPath, "plugin installPath must be absolute: "+id)
				continue
			}
			scope := strings.TrimSpace(install.Scope)
			isProject := scope == "project" || scope == "local"
			if isProject && (!request.Project.Selected || !codeafSamePlace(install.ProjectPath, request.Project.Path)) {
				continue
			}
			if scope != "" && scope != "user" && !isProject {
				continue
			}
			permission := PermissionExplanation{State: PermissionDenied, Evidence: codeafEvidence(EvidenceInferred, "installed-plugin-enablement", registryPath, "enabledPlugins."+id, "registered plugin is off unless effective layered value is exactly true"), Layers: cloneLayers(inputs.scope.ConfigLayers)}
			if inputs.enabled[id] {
				permission.State = PermissionAllowed
				permission.Evidence.Reason = "registered plugin is enabled by effective layered enabledPlugins=true"
			}
			if inputs.settingsUnknown {
				permission.State = PermissionUnknown
				permission.Evidence.Kind = EvidenceUnknown
				permission.Evidence.Reason = "a plugin setting layer is unreadable; reliable effective enablement cannot be established"
			}
			plugin := codeafPlugin{id: id, version: install.Version, installRoot: root, permission: permission, folders: inputs.pluginFolders(request.home, id, root)}
			target := &user
			if isProject {
				target = &project
			}
			// Keep per-installation version/ownership while deduplicating native paths.
			var unique []string
			for _, folder := range plugin.folders {
				found := false
				for _, existing := range *target {
					if existing.id != id {
						continue
					}
					for _, held := range existing.folders {
						found = found || held == folder
					}
				}
				if !found {
					unique = append(unique, folder)
				}
			}
			plugin.folders = unique
			if len(unique) > 0 {
				*target = append(*target, plugin)
			}
		}
	}
	return user, project
}
func codeafUniquePaths(paths []string, values ...string) []string {
	for _, path := range values {
		path = filepath.Clean(path)
		found := false
		for _, existing := range paths {
			found = found || existing == path
		}
		if !found {
			paths = append(paths, path)
		}
	}
	return paths
}
func (inputs *codeafInputs) declaredPaths(path, entry string) []string {
	var document struct {
		Skills  json.RawMessage `json:"skills"`
		Plugins []struct {
			Name   string          `json:"name"`
			Skills json.RawMessage `json:"skills"`
		} `json:"plugins"`
	}
	if !inputs.readJSON(path, &document) {
		return nil
	}
	raw := document.Skills
	if entry != "" {
		raw = nil
		for _, plugin := range document.Plugins {
			if plugin.Name == entry {
				raw = plugin.Skills
				break
			}
		}
	}
	if len(raw) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return nil
}
func (inputs *codeafInputs) pluginFolders(home, id, root string) []string {
	paths := inputs.declaredPaths(filepath.Join(root, ".claude-plugin/plugin.json"), "")
	if name, market, ok := strings.Cut(id, "@"); ok && name != "" && market != "" {
		var known map[string]struct {
			InstallLocation string `json:"installLocation"`
		}
		var places []string
		if inputs.readJSON(filepath.Join(home, ".claude/plugins/known_marketplaces.json"), &known) {
			location := strings.TrimSpace(known[market].InstallLocation)
			if filepath.IsAbs(location) {
				places = append(places, location)
			}
		}
		places = append(places, filepath.Join(home, ".claude/plugins/marketplaces", market), root)
		for _, place := range places {
			catalog := filepath.Join(place, ".claude-plugin/marketplace.json")
			if _, err := os.Stat(catalog); err != nil {
				continue
			}
			paths = append(paths, inputs.declaredPaths(catalog, name)...)
			break
		}
	}
	if len(paths) == 0 {
		return []string{filepath.Join(root, "skills")}
	}
	var folders []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || filepath.IsAbs(path) {
			continue
		}
		folder := filepath.Join(root, path)
		relative, err := filepath.Rel(root, folder)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			continue
		}
		folders = codeafUniquePaths(folders, folder)
	}
	return folders
}
