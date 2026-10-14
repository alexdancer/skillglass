package collection

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Only behavioral policy is imported. Credential values and providers are never evaluated.
var hermesPolicyKeys = map[string]bool{
	"HERMES_PLATFORM": true, "HERMES_SESSION_PLATFORM": true,
	"HERMES_SAFE_MODE": true, "HERMES_ENABLE_PROJECT_PLUGINS": true,
	"HERMES_MANAGED_DIR": true,
}

func (inputs *hermesInputs) envEnabled(key string) bool {
	switch strings.ToLower(strings.TrimSpace(inputs.policyEnv[key])) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
func (inputs *hermesInputs) policyLayer(path, scope, state, reason string) {
	inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, PermissionLayer{Path: path, Scope: scope, State: state, Reason: reason, Evidence: hermesEvidence(EvidenceObserved, scope, path, "Hermes behavioral policy", reason)})
}
func (inputs *hermesInputs) pluginProblem(path, reason string) {
	inputs.pluginUncertain = true
	inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
}
func (inputs *hermesInputs) readPolicy(request Request) {
	inputs.policyEnv = make(map[string]string)
	inputs.policyDeclared = make(map[string]bool)
	for key := range hermesPolicyKeys {
		if value, ok := os.LookupEnv(key); ok {
			inputs.policyEnv[key] = value
		}
	}
	inputs.policyLayer("", "process-policy", "read", "process behavioral policy is the fallback for persisted environment policy")
	inputs.readPolicyEnv(filepath.Join(inputs.home, ".env"), false)
	// External secret backends may inject environment policy. They are never started here.
	if value := inputs.configData["secrets"]; value != nil {
		if fields, ok := value.(map[string]any); !ok || len(fields) > 0 {
			inputs.problem(inputs.config, "configured external secret sources are not evaluated; startup environment policy is unknown")
		}
	}
	managed := strings.TrimSpace(inputs.policyEnv["HERMES_MANAGED_DIR"])
	if managed == "" && os.Getenv("PYTEST_CURRENT_TEST") == "" {
		managed = "/etc/hermes"
	}
	if managed != "" {
		if !filepath.IsAbs(managed) {
			managed = filepath.Join(request.Project.Path, managed)
		}
		managed = filepath.Clean(managed)
		info, err := os.Stat(managed)
		switch {
		case os.IsNotExist(err):
			inputs.policyLayer(managed, "managed-directory", "absent", "selected managed directory is absent; no fallback from an explicit override")
		case err != nil:
			inputs.problem(managed, "managed directory cannot be inspected; persisted environment policy is unknown")
			inputs.policyLayer(managed, "managed-directory", "unknown", "managed policy cannot be established")
		case !info.IsDir():
			inputs.policyLayer(managed, "managed-directory", "absent", "selected managed path is not a directory")
		default:
			beforeDir := inputs.policyEnv["HERMES_MANAGED_DIR"]
			inputs.readPolicyEnv(filepath.Join(managed, ".env"), true)
			if inputs.policyEnv["HERMES_MANAGED_DIR"] != beforeDir {
				inputs.pluginProblem(filepath.Join(managed, ".env"), "managed environment redirects its own directory; plugin policy resolution is unsupported")
			}
			inputs.readManagedPlugins(filepath.Join(managed, "config.yaml"))
		}
	}
	inputs.platform = inputs.policyEnv["HERMES_PLATFORM"]
	if inputs.platform == "" {
		inputs.platform = inputs.policyEnv["HERMES_SESSION_PLATFORM"]
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{0,64}$`).MatchString(inputs.platform) {
		inputs.problem(filepath.Join(inputs.home, ".env"), "messaging platform policy is outside the supported literal shape")
	}
	inputs.policyLayer(filepath.Join(inputs.home, ".env"), "session-platform", "read", "profile environment overrides shell policy; managed environment wins last; HERMES_PLATFORM precedes HERMES_SESSION_PLATFORM")
	user := hermesMap(inputs.configData, "plugins")
	inherited := make(map[string]any)
	for _, key := range []string{"enabled", "disabled"} {
		if _, replaced := inputs.pluginManaged[key]; !replaced {
			if value, present := user[key]; present {
				inherited[key] = value
			}
		}
	}
	inputs.pluginConfig = inputs.expandPluginPolicy(inherited, inputs.config)
	inputs.pluginConfigPath = inputs.config
	if inputsManaged := inputs.pluginManaged; inputsManaged != nil {
		for key, value := range inputs.expandPluginPolicy(inputsManaged, inputs.managedConfigPath) {
			inputs.pluginConfig[key] = value
		}
		inputs.pluginConfigPath = inputs.managedConfigPath
	}
	inputs.pluginScalarKnown = true
	for _, key := range []string{"enabled", "disabled"} {
		observation := inputs.configYAML
		if _, present := inputs.pluginManaged[key]; present {
			observation = inputs.managedYAML
		}
		if !observation.known("plugins." + key) {
			inputs.pluginScalarKnown = false
		}
	}
}
func (inputs *hermesInputs) readPolicyEnv(path string, managed bool) {
	scope := "profile-environment"
	if managed {
		scope = "managed-environment"
	}
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		inputs.policyLayer(path, scope, "absent", "optional persisted environment policy is absent")
		return
	}
	if err != nil {
		inputs.problem(path, "environment policy cannot be read")
		inputs.policyLayer(path, scope, "unknown", "environment policy cannot be read")
		return
	}
	text := string(raw)
	if !managed {
		text = strings.TrimPrefix(text, "\ufeff")
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		inputs.problem(path, "environment policy requires unsupported native encoding or normalization")
		inputs.policyLayer(path, scope, "unknown", "unsupported environment encoding or normalization")
		return
	}
	uncertain := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !managed {
			line = hermesDotenvExport.ReplaceAllString(line, "")
		}
		key, value, present := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !present {
			continue
		}
		inputs.policyDeclared[key] = true
		if !managed && !hermesDotenvQuoteClosed(value) {
			uncertain = true
			inputs.problem(path, "multiline or unclosed environment binding is outside the bounded inert parser")
			break // A later line may be part of this value rather than another assignment.
		}
		if !managed && key != strings.Trim(key, `"'`) && hermesPolicyKeys[strings.Trim(key, `"'`)] {
			uncertain = true
			inputs.problem(path, "quoted behavioral policy key is outside the supported syntax")
			continue
		}
		if !hermesPolicyKeys[key] {
			continue
		}
		if managed {
			value = strings.Trim(value, `"'`)
		} else {
			var ok bool
			value, ok = hermesDotenvLiteral(value)
			if !ok {
				uncertain = true
				inputs.problem(path, "unsupported interpolation or syntax for behavioral policy key "+key)
				continue
			}
		}
		inputs.policyEnv[key] = value
	}
	state, reason := "read", "only literal behavioral policy is read; other environment values are omitted"
	if uncertain {
		state, reason = "unknown", "a relevant environment policy assignment cannot be interpreted"
	}
	inputs.policyLayer(path, scope, state, reason)
}

var hermesDotenvExport = regexp.MustCompile(`^export\s+`)

func hermesDotenvQuoteClosed(value string) bool {
	if len(value) == 0 || (value[0] != '\'' && value[0] != '"') {
		return true
	}
	quote := value[0]
	for index := 1; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) {
			index++
			continue
		}
		if value[index] == quote {
			return true
		}
	}
	return false
}

// Python's dotenv parser uses Unicode whitespace, including these control separators.
func hermesDotenvSpace(char rune) bool {
	return unicode.IsSpace(char) || (char >= '\u001c' && char <= '\u001f')
}
func hermesDotenvLiteral(value string) (string, bool) {
	if strings.Contains(value, "${") {
		return "", false
	}
	if value == "" {
		return "", true
	}
	if value[0] != '\'' && value[0] != '"' {
		whitespaceStart := -1
		for index, char := range value {
			if hermesDotenvSpace(char) {
				if whitespaceStart < 0 {
					whitespaceStart = index
				}
				continue
			}
			if char == '#' && whitespaceStart >= 0 {
				value = value[:whitespaceStart]
				break
			}
			whitespaceStart = -1
		}
		return strings.TrimFunc(value, hermesDotenvSpace), true
	}
	quote := value[0]
	var result strings.Builder
	for index := 1; index < len(value); index++ {
		char := value[index]
		if char == quote {
			trailing := strings.TrimSpace(value[index+1:])
			return result.String(), trailing == "" || strings.HasPrefix(trailing, "#")
		}
		if char == '\\' && index+1 < len(value) {
			next := value[index+1]
			if next == '\\' || next == quote {
				result.WriteByte(next)
				index++
				continue
			}
			if quote == '"' {
				escaped := map[byte]byte{'n': '\n', 'r': '\r', 't': '\t', 'a': '\a', 'b': '\b', 'f': '\f', 'v': '\v'}
				if decoded, ok := escaped[next]; ok {
					result.WriteByte(decoded)
					index++
					continue
				}
			}
		}
		result.WriteByte(char)
	}
	return "", false
}
func (inputs *hermesInputs) readManagedPlugins(path string) {
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		inputs.policyLayer(path, "managed-plugin-config", "absent", "optional managed plugin policy is absent")
		return
	}
	if err != nil {
		inputs.pluginProblem(path, "managed plugin policy cannot be read")
		inputs.policyLayer(path, "managed-plugin-config", "unknown", "managed plugin policy cannot be read")
		return
	}
	data, _, err := decodeMetadata(raw)
	if err != nil {
		inputs.pluginProblem(path, "managed plugin policy is invalid YAML")
		inputs.policyLayer(path, "managed-plugin-config", "unknown", "managed plugin policy is invalid YAML")
		return
	}
	value, present := data["plugins"]
	if present && value != nil && hermesMap(data, "plugins") == nil {
		inputs.pluginProblem(path, "managed plugins must be a mapping")
	}
	inputs.managedYAML = observeHermesYAML(raw)
	inputs.pluginManaged = hermesMap(data, "plugins")
	inputs.managedConfigPath = path
	inputs.policyLayer(path, "managed-plugin-config", "read", "managed values replace user values per leaf; raw skills.disabled remains profile-only")
}

var hermesConfigReference = regexp.MustCompile(`\$\{([^}]+)\}`)

func (inputs *hermesInputs) expandPluginPolicy(data map[string]any, path string) map[string]any {
	result := make(map[string]any)
	for _, key := range []string{"enabled", "disabled"} {
		value, present := data[key]
		if !present {
			continue
		}
		items, ok := value.([]any)
		if value == nil {
			result[key] = nil
			continue
		}
		if !ok {
			result[key] = value
			inputs.pluginProblem(path, "plugin policy list cannot be interpreted")
			continue
		}
		expanded := make([]any, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				inputs.pluginProblem(path, "plugin policy list requires strings")
				expanded = append(expanded, item)
				continue
			}
			text = hermesConfigReference.ReplaceAllStringFunc(text, func(ref string) string {
				name := strings.TrimSpace(ref[2 : len(ref)-1])
				name = strings.TrimSpace(strings.TrimPrefix(name, "env:"))
				if hermesPolicyKeys[name] {
					if value, present := inputs.policyEnv[name]; present {
						return value
					}
				}
				inputs.pluginProblem(path, "plugin policy references unsupported or unresolved environment policy; credentials and providers are not evaluated")
				return ref
			})
			expanded = append(expanded, text)
		}
		result[key] = expanded
	}
	return result
}
