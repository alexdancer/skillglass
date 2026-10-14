package collection

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

type hermesPlugin struct {
	path, key, name, namespace, version string
	portable, admitted                  bool
	nameKnown, keyKnown, versionKnown   bool
	state                               ParticipationState
	reason                              string
	policyEvidence                      Evidence
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
func hermesManifestAbsent(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.EBADF) || errors.Is(err, syscall.ELOOP)
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
				_, yamlErr := os.Stat(manifest)
				if hermesManifestAbsent(yamlErr) {
					manifest = filepath.Join(child, "plugin.yml")
					_, yamlErr = os.Stat(manifest)
				}
				portable := false
				if hermesManifestAbsent(yamlErr) {
					manifest = filepath.Join(child, "plugin.json")
					if _, err = os.Lstat(manifest); os.IsNotExist(err) {
						if depth == 0 {
							walk(child, entry.Name(), 1)
						}
						continue
					}
					portable = true
				}
				plugin := hermesPlugin{path: child, portable: portable, nameKnown: true, keyKnown: true, versionKnown: true, state: ParticipationUnknown, reason: "native plugin code controls registration; it is never executed by the viewer"}
				var data map[string]any
				if portable {
					raw, err := nativeBytes(manifest)
					if err != nil || json.Unmarshal(raw, &data) != nil {
						inputs.problem(manifest, "portable plugin manifest is unreadable or invalid JSON")
						data = nil
					}
				} else {
					var state string
					var observation hermesYAMLObservation
					data, state, observation = inputs.yaml(manifest)
					plugin.admitted = state == "read" && observation.nativeConstructible() && observation.mappingAt()
					if state == "read" && !plugin.admitted {
						plugin.reason = "native plugin manifest requires supported YAML constructors and a mapping root"
						inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, manifest, plugin.reason, nil, false))
					}
					plugin.nameKnown = observation.known("name")
					plugin.versionKnown = observation.known("version")
					if value, present := data["name"]; present {
						_, text := value.(string)
						plugin.nameKnown = plugin.nameKnown && text
					}
					if value, present := data["version"]; present {
						_, text := value.(string)
						plugin.versionKnown = plugin.versionKnown && text
					}
				}
				name, _ := data["name"].(string)
				if _, present := data["name"]; !present && !portable {
					name = entry.Name()
				}
				if !plugin.nameKnown {
					name = ""
				}
				plugin.name = name
				plugin.key, plugin.keyKnown = name, plugin.nameKnown
				if prefix != "" {
					plugin.key = prefix + "/" + entry.Name()
					plugin.keyKnown = true
				}
				plugin.namespace = name
				if plugin.versionKnown {
					plugin.version, _ = data["version"].(string)
				}
				if portable {
					plugin.namespace = hermesPortableNamespace(plugin.key)
					if !hermesPortableManifestValid(data) || !hermesPluginContained(manifest, child) {
						plugin.reason = "portable manifest schema, identity or containment could not be established"
					} else {
						plugin.admitted = true
						plugin.state = ParticipationEligible
						plugin.reason = "declarative portable plugin registration; native namespace derived from registry key"
					}
				}
				plugin.admitted = plugin.admitted && !inputs.envEnabled("HERMES_SAFE_MODE") && (!project || inputs.envEnabled("HERMES_ENABLE_PROJECT_PLUGINS"))
				enabled, disabled := inputs.pluginPolicy["enabled"], inputs.pluginPolicy["disabled"]
				policyPath, policySetting := plugin.path, ""
				switch {
				case inputs.envEnabled("HERMES_SAFE_MODE"):
					plugin.state = ParticipationExcluded
					plugin.reason = "HERMES_SAFE_MODE skips plugin discovery"
					policyPath, policySetting = "", "HERMES_SAFE_MODE"
				case project && !inputs.envEnabled("HERMES_ENABLE_PROJECT_PLUGINS"):
					plugin.state = ParticipationExcluded
					plugin.reason = "project plugins require HERMES_ENABLE_PROJECT_PLUGINS opt-in; selected project has no ancestor inheritance"
					policyPath, policySetting = "", "HERMES_ENABLE_PROJECT_PLUGINS"
				case inputs.uncertain:
					plugin.state = ParticipationUnknown
					plugin.reason = "native configuration, environment or discovery inputs could not be reliably interpreted"
					policyPath = ""
				case inputs.pluginFailure != nil:
					plugin.state = ParticipationUnknown
					plugin.reason = inputs.pluginFailure.Reason
					policyPath = inputs.pluginFailure.InputPath
				case disabled.known && ((plugin.keyKnown && hermesHas(disabled.names, plugin.key)) || (plugin.nameKnown && hermesHas(disabled.names, name))):
					plugin.state = ParticipationExcluded
					plugin.reason = "plugins.disabled overrides plugins.enabled"
					policyPath, policySetting = disabled.path, "plugins.disabled"
				case !disabled.known:
					plugin.state = ParticipationUnknown
					plugin.reason = "effective plugins.disabled cannot be reliably interpreted"
					policyPath, policySetting = disabled.path, "plugins.disabled"
				case !enabled.known:
					plugin.state = ParticipationUnknown
					plugin.reason = "effective plugins.enabled cannot be reliably interpreted"
					policyPath, policySetting = enabled.path, "plugins.enabled"
				case !plugin.keyKnown:
					plugin.state = ParticipationUnknown
					plugin.reason = "native registry string identity could not be reliably interpreted"
				case !hermesHas(enabled.names, plugin.key) && (!plugin.nameKnown || !hermesHas(enabled.names, name)):
					plugin.state = ParticipationExcluded
					plugin.reason = "plugins are opt-in; registry key/name is absent from plugins.enabled"
					policyPath, policySetting = enabled.path, "plugins.enabled"
				default:
					if plugin.state == ParticipationEligible {
						policyPath, policySetting = enabled.path, "plugins.enabled"
					}
				}
				plugin.policyEvidence = hermesEvidence(EvidenceInferred, "plugin-registration", policyPath, policySetting, plugin.reason)
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
		if plugin.admitted && plugin.keyKnown {
			winners[plugin.key] = index
		}
	}
	for index, plugin := range plugins {
		if winner, ok := winners[plugin.key]; plugin.admitted && plugin.keyKnown && ok && winner != index {
			plugin.state = ParticipationExcluded
			plugin.reason = "later project plugin manifest with the same registry key overrides this user plugin"
			plugin.policyEvidence.InputPath, plugin.policyEvidence.Setting = plugins[winner].path, ""
		}
		skillsRoot := filepath.Join(plugin.path, "skills")
		if !hermesPluginContained(skillsRoot, plugin.path) {
			plugin.state = ParticipationExcluded
			plugin.reason = "plugin skills escape plugin ownership boundary"
			plugin.policyEvidence.InputPath, plugin.policyEvidence.Setting = skillsRoot, ""
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
				current.policyEvidence.InputPath, current.policyEvidence.Setting = filepath.Join(bundle, "SKILL.md"), ""
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
	return err == nil && hermesPathWithin(resolved, owner)
}
func hermesPluginFact(fact *ReferenceAvailability, explanation *AvailabilityExplanation, source Source, ref Reference, plugin hermesPlugin, inputs *hermesInputs, full hermesMetadataObservation) {
	fact.Participation = plugin.state
	kind := EvidenceInferred
	if plugin.state == ParticipationUnknown {
		kind = EvidenceUnknown
	}
	fact.Evidence = plugin.policyEvidence
	fact.Evidence.Kind, fact.Evidence.Reason = kind, plugin.reason
	fact.Provenance = Provenance{Kind: "plugin", PluginID: plugin.key, Version: plugin.version, Evidence: hermesEvidence(EvidenceObserved, "plugin-manifest", plugin.path, "", plugin.reason)}
	if !plugin.versionKnown {
		fact.Provenance.Evidence.Kind = EvidenceUnknown
		fact.Provenance.Evidence.Reason = "native YAML version stringification is outside supported scalar semantics"
	}
	bare, _ := full.metadata["name"].(string)
	if plugin.portable {
		bare = filepath.Base(ref.DiscoveredPath)
	}
	qualified := plugin.namespace + ":" + bare
	explanation.NativeName = qualified
	fact.Permission = hermesPermission(qualified, inputs)
	if plugin.portable {
		valid, known := hermesPortableSkillValid(full, bare)
		if fact.Participation != ParticipationExcluded {
			if !known {
				fact.Participation = ParticipationUnknown
				fact.Evidence = hermesEvidence(EvidenceUnknown, "portable-yaml-interpretation", source.Manifest.Path, "", "portable YAML string semantics are outside supported native interpretation")
			} else if !valid {
				fact.Participation = ParticipationExcluded
				fact.Evidence = hermesEvidence(EvidenceInferred, "portable-skill-frontmatter", source.Manifest.Path, "name, description", "portable skill frontmatter does not satisfy native directory/name/description constraints")
			}
		}
	}

	explanation.NativeNameEvidence = &Evidence{Kind: EvidenceInferred, Rule: "plugin-qualified-name", InputPath: plugin.path, AgentVersion: hermesValidatedVersion, Reason: "portable namespace derives from registry key; native dynamic registration names remain unverified"}
	if !plugin.nameKnown || !full.framed || !full.nameKnown() {
		explanation.NativeName = ""
		explanation.NativeNameEvidence.Kind = EvidenceUnknown
		explanation.NativeNameEvidence.Reason = "native plugin or skill name string interpretation cannot be established"
		fact.Permission.State = PermissionUnknown
		fact.Permission.Evidence = *explanation.NativeNameEvidence
	}
}

func hermesPortableSkillValid(full hermesMetadataObservation, bare string) (bool, bool) {
	if !full.known {
		return false, false
	}
	if !full.framed {
		return false, true
	}
	if !full.yaml.known("name", "description", "license", "compatibility", "metadata", "allowed-tools") {
		return false, false
	}
	data := full.metadata
	name, _ := data["name"].(string)
	description, _ := data["description"].(string)
	if name != bare || description == "" || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(bare) || len(bare) > 64 || len([]rune(description)) > 1024 {
		return false, true
	}
	for _, key := range []string{"license", "allowed-tools"} {
		if value, present := data[key]; present {
			if _, ok := value.(string); !ok {
				return false, true
			}
		}
	}
	if value, present := data["compatibility"]; present {
		text, ok := value.(string)
		if !ok || len([]rune(text)) == 0 || len([]rune(text)) > 500 {
			return false, true
		}
	}
	if value, present := data["metadata"]; present {
		fields, ok := value.(map[string]any)
		if !ok {
			return false, true
		}
		for _, value := range fields {
			if _, ok := value.(string); !ok {
				return false, true
			}
		}
	}
	return true, true
}
