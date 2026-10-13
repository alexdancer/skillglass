package collection

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/pelletier/go-toml/v2"
)

const codexValidatedVersion = "codex-cli 0.155.1"

type codexChoice struct {
	Path    string `toml:"path"`
	Enabled *bool  `toml:"enabled"`
}
type codexConfig struct {
	Skills struct {
		Config []codexChoice `toml:"config"`
	} `toml:"skills"`
	Projects map[string]struct {
		Trust string `toml:"trust_level"`
	} `toml:"projects"`
	Plugins map[string]struct {
		Enabled *bool `toml:"enabled"`
	} `toml:"plugins"`
}
type codexInputs struct {
	scope          AgentScope
	effective      []codexChoice
	selectedLayer  string
	plugins        map[string]bool
	problems       []Problem
	uncertain      bool
	trustedProject bool
}

func nativeHome() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", errors.New("native agent scope requires HOME to bound discovery")
	}
	return filepath.Abs(home)
}
func codexHome(home string) string {
	if value := os.Getenv("CODEX_HOME"); value != "" {
		if path, err := filepath.Abs(value); err == nil {
			return path
		}
	}
	return filepath.Join(home, ".codex")
}
func codexEvidence(kind EvidenceKind, rule, path, setting, reason string) Evidence {
	return Evidence{Kind: kind, Rule: rule, InputPath: path, Setting: setting, AgentVersion: codexValidatedVersion, Reason: reason}
}

// Only bounded regular files are opened; FIFOs, devices and oversized inputs cannot block browsing.
func nativeBytes(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("native input is not a regular file")
	}
	if info.Size() > maxManifestBytes {
		return nil, errors.New("native input exceeds read limit")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return nil, errors.New("native input changed file kind during read")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxManifestBytes {
		return nil, errors.New("native input exceeds read limit")
	}
	return raw, nil
}
func (inputs *codexInputs) problem(path, reason string) {
	inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
	inputs.uncertain = true
}
func (inputs *codexInputs) loadConfig(path, scope string, trusted bool) codexConfig {
	layer := PermissionLayer{Evidence: codexEvidence(EvidenceObserved, "local-config-input", path, "", "only relevant skill settings are reported"), Path: path, Scope: scope, State: "absent", Reason: "optional configuration does not exist"}
	if !trusted {
		_, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer)
			return codexConfig{}
		}
		layer.State = "ignored"
		layer.Reason = "project configuration is untrusted; discovery still participates"
		inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer)
		return codexConfig{}
	}
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer)
		return codexConfig{}
	}
	var parsed codexConfig
	var presence map[string]any
	if err != nil || toml.Unmarshal(raw, &parsed) != nil || toml.Unmarshal(raw, &presence) != nil {
		layer.State = "unknown"
		layer.Reason = "configuration is unreadable or invalid TOML"
		inputs.problem(path, layer.Reason)
		inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer)
		return codexConfig{}
	}
	layer.State = "loaded"
	layer.Reason = "local configuration parsed; unrelated settings are not exposed"
	for _, choice := range parsed.Skills.Config {
		if choice.Path == "" || choice.Enabled == nil {
			inputs.problem(path, "skills.config requires path and boolean enabled")
			continue
		}
		layer.Choices = append(layer.Choices, PermissionChoice{Path: choice.Path, Enabled: *choice.Enabled})
	}
	hasArray := false
	if skills, ok := presence["skills"].(map[string]any); ok {
		_, hasArray = skills["config"]
	}
	if hasArray {
		for i := range inputs.scope.ConfigLayers {
			if inputs.scope.ConfigLayers[i].State == "effective" {
				inputs.scope.ConfigLayers[i].State = "replaced"
				inputs.scope.ConfigLayers[i].Reason = "skills.config array replaced by " + path
			}
		}
		layer.State = "effective"
		layer.Reason = "winning skills.config array replaces lower arrays, including an empty array"
		inputs.effective = parsed.Skills.Config
		inputs.selectedLayer = path
	}
	for id, plugin := range parsed.Plugins {
		if plugin.Enabled != nil {
			inputs.plugins[id] = *plugin.Enabled
		} else {
			inputs.problem(path, "plugin configuration requires boolean enabled")
		}
	}
	for _, key := range []string{"profile", "project_root_markers", "requirements"} {
		if _, ok := presence[key]; ok {
			inputs.problem(path, "native "+key+" behavior is unverified")
			inputs.scope.Gaps = append(inputs.scope.Gaps, "unverified configuration: "+key)
		}
	}
	if scope == "system" && (hasArray || len(parsed.Plugins) > 0) {
		inputs.problem(path, "system skill/plugin configuration precedence was not independently validated")
	}
	inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer)
	return parsed
}
func readCodexInputs(request Request, ancestors []string, boundary string) codexInputs {
	inputs := codexInputs{scope: AgentScope{Evidence: codexEvidence(EvidenceObserved, "recorded-native-verification", "tools/verify_codex.py", "", "disposable app-server verification recorded native outcomes, including independent cache-alias discovery; this is reference-version evidence"), Application: "codex", ValidatedVersion: codexValidatedVersion, VersionEvidence: "recorded disposable native verification; installed executable and active session are not inspected", Gaps: []string{
		"configured permission is a local inference; project enforcement in an inference session is unverified",
		"CLI overrides, runtime extra roots, selected profiles, cloud policy and active-session state are not supplied",
		"native recursion is verified; exact depth and metadata limits remain unverified; hidden folders and file manifest links are omitted",
		"remote and workspace-managed plugin activation is unverified",
	}}, plugins: make(map[string]bool)}
	inputs.loadConfig("/etc/codex/config.toml", "system", true)
	user := inputs.loadConfig(filepath.Join(request.codexHome, "config.toml"), "user", true)
	trusted := false
	// Trust keys use canonical paths. The closest explicit key governs the selected context.
	for _, path := range ancestors {
		if trust, ok := user.Projects[path]; ok {
			trusted = trust.Trust == "trusted"
			break
		}
	}
	inputs.trustedProject = trusted
	for i := len(ancestors) - 1; i >= 0; i-- {
		inputs.loadConfig(filepath.Join(ancestors[i], ".codex", "config.toml"), "project", trusted)
	}
	if request.Project.Selected && boundary == "" {
		inputs.scope.Gaps = append(inputs.scope.Gaps, "no Git repository: only selected directory inspected; native ancestor traversal is unverified")
		inputs.problem(request.Project.Path, "native project traversal without a Git boundary is unverified")
	}
	return inputs
}
func canonicalManifest(path string) string {
	if !filepath.IsAbs(path) {
		return ""
	} // Relative targeting was not validated; never resolve against the viewer CWD.
	result, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	if filepath.Base(result) != "SKILL.md" {
		return ""
	}
	info, err := os.Stat(result)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return filepath.Clean(result)
}
func (inputs *codexInputs) permission(manifest string) PermissionExplanation {
	result := PermissionExplanation{State: PermissionAllowed, Layers: cloneLayers(inputs.scope.ConfigLayers), Evidence: codexEvidence(EvidenceInferred, "default-enabled", manifest, "", "no matching entry in the effective skills.config array")}
	if manifest == "" {
		result.State = PermissionUnknown
		result.Evidence = codexEvidence(EvidenceUnknown, "unresolved-manifest", "", "skills.config.path", "source manifest has no reliable canonical target")
		return result
	}
	if inputs.uncertain {
		result.State = PermissionUnknown
		result.Evidence = codexEvidence(EvidenceUnknown, "incomplete-configuration", inputs.selectedLayer, "skills.config", "native inputs prevent a reliable local calculation")
		return result
	}
	for _, choice := range inputs.effective {
		if !filepath.IsAbs(choice.Path) {
			result.State = PermissionUnknown
			result.Evidence = codexEvidence(EvidenceUnknown, "relative-selector", inputs.selectedLayer, "skills.config.path", "relative selector behavior was not validated")
			return result
		}
		target := canonicalManifest(choice.Path)
		if target == "" || target != manifest {
			continue
		}
		if choice.Enabled == nil {
			continue
		}
		state := PermissionAllowed
		if !*choice.Enabled {
			state = PermissionDenied
		}
		// Multiple selectors for the same canonical target are deliberately not assigned guessed ordering.
		if result.Evidence.Rule == "manifest-selector" && result.State != state {
			result.State = PermissionUnknown
			result.Evidence = codexEvidence(EvidenceUnknown, "conflicting-selectors", inputs.selectedLayer, "skills.config", "conflicting canonical entries in one array have unverified ordering")
			return result
		}
		result.State = state
		result.Evidence = codexEvidence(EvidenceInferred, "manifest-selector", inputs.selectedLayer, "skills.config.path="+choice.Path, "manifest selector matches the canonical source; folder selectors do not match on this version")
	}
	return result
}

func nativeAbsent(path string, err error) bool {
	if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	_, linkErr := os.Lstat(path)
	return errors.Is(linkErr, os.ErrNotExist)
}
