package collection

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type hermesPlugin struct {
	path, key, name, namespace, version string
	portable                            bool
	state                               ParticipationState
	reason                              string
}

func hermesEnvEnabled(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
func hermesPortableNamespace(key string) string {
	var slug strings.Builder
	for _, char := range strings.ToLower(key) {
		if char < 128 && ((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-') {
			slug.WriteRune(char)
		} else {
			slug.WriteByte('-')
		}
	}
	text := strings.Trim(slug.String(), "-_")
	if text == "" {
		text = "plugin"
	}
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("agent-plugin-%s-%x", text, sum[:4])
}
func discoverHermesPlugins(request *Request, inputs *hermesInputs) {
	var plugins []hermesPlugin
	scan := func(root string, project bool) {
		var walk func(string, string, int)
		walk = func(path, prefix string, depth int) {
			entries, err := os.ReadDir(path)
			if os.IsNotExist(err) {
				return
			}
			if err != nil {
				inputs.problem(path, "Hermes plugin directory cannot be listed")
				return
			}
			for _, entry := range entries {
				child := filepath.Join(path, entry.Name())
				info, err := os.Stat(child)
				if err != nil || !info.IsDir() {
					continue
				}
				manifest := filepath.Join(child, "plugin.yaml")
				_, yamlErr := os.Lstat(manifest)
				if os.IsNotExist(yamlErr) {
					manifest = filepath.Join(child, "plugin.yml")
					_, yamlErr = os.Lstat(manifest)
				}
				portable := false
				if os.IsNotExist(yamlErr) {
					manifest = filepath.Join(child, "plugin.json")
					if _, err = os.Lstat(manifest); os.IsNotExist(err) {
						if depth == 0 {
							walk(child, entry.Name(), 1)
						}
						continue
					}
					portable = true
				}
				plugin := hermesPlugin{path: child, portable: portable, state: ParticipationUnknown, reason: "native plugin code controls registration; it is never executed by the viewer"}
				var data map[string]any
				if portable {
					raw, err := nativeBytes(manifest)
					if err != nil || json.Unmarshal(raw, &data) != nil {
						inputs.problem(manifest, "portable plugin manifest is unreadable or invalid JSON")
						data = nil
					}
				} else {
					data, _ = inputs.yaml(manifest)
				}
				name, _ := data["name"].(string)
				if name == "" && !portable {
					name = entry.Name()
				}
				plugin.name = name
				plugin.key = name
				if prefix != "" {
					plugin.key = prefix + "/" + entry.Name()
				}
				plugin.namespace = name
				plugin.version, _ = data["version"].(string)
				if portable {
					plugin.namespace = hermesPortableNamespace(plugin.key)
					if !hermesPortableManifestValid(data) || !hermesPluginContained(manifest, child) {
						plugin.reason = "portable manifest schema, identity or containment could not be established"
					} else {
						plugin.state = ParticipationEligible
						plugin.reason = "declarative portable plugin registration; native namespace derived from registry key"
					}
				}
				config := hermesMap(inputs.configData, "plugins")
				enabled, enabledOK := hermesStrings(config["enabled"])
				if config["enabled"] != nil {
					_, enabledOK = config["enabled"].([]any)
				}
				disabled, disabledOK := hermesStrings(config["disabled"])
				if config["disabled"] != nil {
					_, disabledOK = config["disabled"].([]any)
				}
				switch {
				case hermesEnvEnabled("HERMES_SAFE_MODE"):
					plugin.state = ParticipationExcluded
					plugin.reason = "HERMES_SAFE_MODE skips plugin discovery"
				case project && !hermesEnvEnabled("HERMES_ENABLE_PROJECT_PLUGINS"):
					plugin.state = ParticipationExcluded
					plugin.reason = "project plugins require HERMES_ENABLE_PROJECT_PLUGINS opt-in; selected project has no ancestor inheritance"
				case hermesHas(disabled, plugin.key) || hermesHas(disabled, name):
					plugin.state = ParticipationExcluded
					plugin.reason = "plugins.disabled overrides plugins.enabled"
				case !enabledOK || !disabledOK || inputs.uncertain:
					plugin.state = ParticipationUnknown
					plugin.reason = "plugin configuration could not be reliably interpreted"
				case !hermesHas(enabled, plugin.key) && !hermesHas(enabled, name):
					plugin.state = ParticipationExcluded
					plugin.reason = "plugins are opt-in; registry key/name is absent from plugins.enabled"
				}
				plugins = append(plugins, plugin)
			}
		}
		walk(root, "", 0)
	}
	userPlugins := filepath.Join(inputs.home, "plugins")
	request.Roots = append(request.Roots, DiscoveryRoot{ID: RootID(stableID("root_hermes_plugin_inventory", userPlugins)), Path: userPlugins, Context: ContextManaged, Origin: "hermes-plugin-inventory", Optional: true})
	scan(userPlugins, false)
	if request.Project.Selected {
		projectPlugins := filepath.Join(request.Project.Path, ".hermes", "plugins")
		request.Roots = append(request.Roots, DiscoveryRoot{ID: RootID(stableID("root_hermes_plugin_inventory", projectPlugins)), Path: projectPlugins, Context: ContextManaged, Origin: "hermes-plugin-inventory", Optional: true})
		scan(projectPlugins, true)
	}
	winners := make(map[string]int)
	for index, plugin := range plugins {
		if !strings.Contains(plugin.reason, "project plugins require") && !strings.Contains(plugin.reason, "SAFE_MODE") {
			winners[plugin.key] = index
		}
	}
	for index, plugin := range plugins {
		if winner, ok := winners[plugin.key]; ok && winner != index {
			plugin.state = ParticipationExcluded
			plugin.reason = "later project plugin manifest with the same registry key overrides this user plugin"
		}
		skillsRoot := filepath.Join(plugin.path, "skills")
		if !hermesPluginContained(skillsRoot, plugin.path) {
			plugin.state = ParticipationExcluded
			plugin.reason = "plugin skills escape plugin ownership boundary"
		}
		entries, err := os.ReadDir(skillsRoot)
		if err != nil {
			if !os.IsNotExist(err) {
				inputs.problem(skillsRoot, "plugin skills cannot be read")
			}
			continue
		}
		for _, entry := range entries {
			bundle := filepath.Join(skillsRoot, entry.Name())
			info, err := os.Stat(bundle)
			if err != nil || !info.IsDir() {
				continue
			}
			if present, _ := hasManifest(bundle); !present {
				continue
			}
			current := plugin
			if !hermesPluginContained(filepath.Join(bundle, "SKILL.md"), plugin.path) {
				current.state = ParticipationExcluded
				current.reason = "plugin skill manifest escapes the plugin ownership boundary"
			}
			id := RootID(stableID("root_hermes_plugin", plugin.key, bundle))
			request.Roots = append(request.Roots, DiscoveryRoot{ID: id, Path: bundle, Context: ContextManaged, Origin: "hermes-plugin", Optional: true})
			inputs.plugins[id] = current
		}
	}
}
func hermesPortableManifestValid(data map[string]any) bool {
	name, ok := data["name"].(string)
	if !ok || len(name) > 64 || !regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`).MatchString(name) || strings.Contains(name, "--") || strings.Contains(name, "..") || data["$schema"] != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" {
		return false
	}
	for _, key := range []string{"version", "description", "homepage", "repository", "license"} {
		if value, present := data[key]; present {
			if _, ok := value.(string); !ok {
				return false
			}
		}
	}
	if value, present := data["keywords"]; present {
		items, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				return false
			}
		}
	}
	if value, present := data["author"]; present {
		fields, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for key, value := range fields {
			if key != "name" && key != "email" && key != "url" {
				return false
			}
			if _, ok := value.(string); !ok {
				return false
			}
		}
	}
	if fields, ok := data["extensions"].(map[string]any); ok {
		for _, value := range fields {
			if _, ok := value.(map[string]any); !ok {
				return false
			}
		}
	}
	return true
}
func hermesPluginContained(path, root string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	owner, err := filepath.EvalSymlinks(root)
	return err == nil && pathWithin(resolved, owner)
}
func hermesPluginFact(fact *ReferenceAvailability, explanation *AvailabilityExplanation, source Source, ref Reference, plugin hermesPlugin, inputs *hermesInputs) {
	fact.Participation = plugin.state
	fact.Evidence = hermesEvidence(EvidenceInferred, "plugin-registration", plugin.path, "plugins.enabled, plugins.disabled", plugin.reason)
	fact.Provenance = Provenance{Kind: "plugin", PluginID: plugin.key, Version: plugin.version, Evidence: hermesEvidence(EvidenceObserved, "plugin-manifest", plugin.path, "", plugin.reason)}
	bare := source.Manifest.Name
	if plugin.portable {
		bare = filepath.Base(ref.DiscoveredPath)
	}
	qualified := plugin.namespace + ":" + bare
	explanation.NativeName = qualified
	fact.Permission = hermesPermission(qualified, inputs)
	if plugin.portable && !hermesPortableSkillValid(source, bare) {
		fact.Participation = ParticipationExcluded
		fact.Evidence.Reason = "portable skill frontmatter does not satisfy native directory/name/description constraints"
	}

	explanation.NativeNameEvidence = &Evidence{Kind: EvidenceInferred, Rule: "plugin-qualified-name", InputPath: plugin.path, AgentVersion: hermesValidatedVersion, Reason: "portable namespace derives from registry key; native dynamic registration names remain unverified"}
}

func hermesPortableSkillValid(source Source, bare string) bool {
	if source.Manifest.Status != ManifestParsed || source.Manifest.Name != bare || source.Manifest.Description == "" || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(bare) || len(bare) > 64 || len([]rune(source.Manifest.Description)) > 1024 {
		return false
	}
	data := source.Manifest.Metadata
	for _, key := range []string{"license", "allowed-tools"} {
		if value, present := data[key]; present {
			if _, ok := value.(string); !ok {
				return false
			}
		}
	}
	if value, present := data["compatibility"]; present {
		text, ok := value.(string)
		if !ok || len([]rune(text)) == 0 || len([]rune(text)) > 500 {
			return false
		}
	}
	if value, present := data["metadata"]; present {
		fields, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for _, value := range fields {
			if _, ok := value.(string); !ok {
				return false
			}
		}
	}
	return true
}
