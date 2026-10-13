package collection

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type codexPluginFact struct {
	directory     string
	activation    Evidence
	participation ParticipationState
	provenance    Provenance
}
type pluginManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Skills  string `json:"skills"`
}
type localPluginRegistration struct {
	manifest pluginManifest
	registry string
}

func readPluginManifest(directory string) (pluginManifest, error) {
	var manifest pluginManifest
	raw, err := nativeBytes(filepath.Join(directory, ".codex-plugin", "plugin.json"))
	if err != nil {
		return manifest, err
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.Name == "" || manifest.Version == "" {
		return manifest, errors.New("invalid plugin manifest")
	}
	return manifest, nil
}
func discoverCodexPlugins(request *Request, inputs *codexInputs, ancestors []string) map[string]codexPluginFact {
	registrations := make(map[string]localPluginRegistration)
	for _, ancestor := range ancestors {
		path := filepath.Join(ancestor, ".agents", "plugins", "marketplace.json")
		raw, err := nativeBytes(path)
		if nativeAbsent(path, err) {
			continue
		}
		var market struct {
			Name    string `json:"name"`
			Plugins []struct {
				Name   string `json:"name"`
				Source struct {
					Kind string `json:"source"`
					Path string `json:"path"`
				} `json:"source"`
			} `json:"plugins"`
		}
		if err != nil || json.Unmarshal(raw, &market) != nil || market.Name == "" {
			inputs.problem(path, "local plugin marketplace could not be interpreted")
			continue
		}
		for _, plugin := range market.Plugins {
			if plugin.Source.Kind != "local" {
				continue
			}
			source := plugin.Source.Path
			if !filepath.IsAbs(source) {
				source = filepath.Join(ancestor, source)
			}
			manifest, err := readPluginManifest(source)
			if err != nil || manifest.Name != plugin.Name {
				inputs.scope.Gaps = append(inputs.scope.Gaps, "plugin registration lacks matching source manifest: "+plugin.Name+"@"+market.Name)
				continue
			}
			id := plugin.Name + "@" + market.Name
			if _, exists := registrations[id]; !exists {
				registrations[id] = localPluginRegistration{manifest: manifest, registry: path}
			}
		}
	}
	facts := make(map[string]codexPluginFact)
	cache := filepath.Join(request.codexHome, "plugins", "cache")
	markets, err := os.ReadDir(cache)
	if errors.Is(err, os.ErrNotExist) {
		return facts
	}
	if err != nil {
		inputs.problem(cache, "plugin cache could not be listed")
		return facts
	}
	for _, market := range markets {
		if !market.IsDir() {
			continue
		}
		plugins, err := os.ReadDir(filepath.Join(cache, market.Name()))
		if err != nil {
			inputs.problem(cache, "plugin cache registrations could not be listed")
			continue
		}
		for _, plugin := range plugins {
			if !plugin.IsDir() {
				continue
			}
			directory := filepath.Join(cache, market.Name(), plugin.Name())
			versions, err := os.ReadDir(directory)
			if err != nil {
				inputs.problem(directory, "plugin versions could not be listed")
				continue
			}
			for _, version := range versions {
				if !version.IsDir() {
					continue
				}
				versionDir := filepath.Join(directory, version.Name())
				root := filepath.Join(versionDir, "skills")
				id := plugin.Name() + "@" + market.Name()
				fact := codexPluginFact{participation: ParticipationUnknown, provenance: Provenance{Kind: "plugin", PluginID: id, Version: version.Name(), Evidence: codexEvidence(EvidenceUnknown, "plugin-cache", versionDir, "", "cache presence alone does not prove installation activation")}}
				installed, manifestErr := readPluginManifest(versionDir)
				safeRoot := false
				if manifestErr == nil && installed.Skills != "" {
					candidate := filepath.Clean(filepath.Join(versionDir, installed.Skills))
					if relative, err := filepath.Rel(versionDir, candidate); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
						root = candidate
						resolvedVersion, versionErr := filepath.EvalSymlinks(versionDir)
						resolvedRoot, rootErr := filepath.EvalSymlinks(root)
						if versionErr == nil && rootErr == nil {
							relative, err := filepath.Rel(resolvedVersion, resolvedRoot)
							safeRoot = err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
						}
					} else {
						inputs.scope.Gaps = append(inputs.scope.Gaps, "plugin skill root escapes cache version: "+id)
					}
				}
				if enabled, configured := inputs.plugins[id]; configured && !enabled {
					fact.participation = ParticipationExcluded
					fact.provenance.Evidence = codexEvidence(EvidenceInferred, "plugin-disabled", versionDir, "plugins."+id+".enabled=false", "configured plugin is inactive; cached content remains")
				} else if registration, ok := registrations[id]; ok && manifestErr == nil && installed.Name == plugin.Name() {
					if installed.Version != version.Name() || registration.manifest.Version != version.Name() {
						fact.participation = ParticipationExcluded
						fact.provenance.Evidence = codexEvidence(EvidenceInferred, "stale-plugin-version", registration.registry, "", "registered source version differs from this cached version")
					} else if configured && enabled && safeRoot && inputs.trustedProject && !inputs.uncertain && installed.Skills == registration.manifest.Skills {
						fact.participation = ParticipationEligible
						fact.provenance.Evidence = codexEvidence(EvidenceInferred, "local-plugin-installation", registration.registry, "plugins."+id+".enabled=true", "local marketplace, source manifest, installed cache manifest and exact version agree; inference-session enforcement is unverified")
					}
				}
				fact.directory, _ = filepath.EvalSymlinks(versionDir)
				fact.activation = fact.provenance.Evidence
				fact.provenance.Evidence = codexEvidence(EvidenceInferred, "managed-cache-boundary", fact.directory, "", "canonical bundle is within a versioned Codex plugin cache; manager ownership is distinct from native discovery participation")
				facts[root] = fact
			}
		}
	}
	paths := make([]string, 0, len(facts))
	for path := range facts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		appendNativeRoot(request, path, ContextManaged, "codex-plugin-cache")
	}
	return facts
}

func managedCodexSource(directory string, plugins map[string]codexPluginFact) (codexPluginFact, bool) {
	for _, fact := range plugins {
		if fact.directory == "" {
			continue
		}
		if relative, err := filepath.Rel(fact.directory, directory); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fact, true
		}
	}
	return codexPluginFact{}, false
}
