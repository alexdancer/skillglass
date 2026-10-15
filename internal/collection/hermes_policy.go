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

func (inputs *hermesInputs) envEnabled(key string) (bool, bool) {
	if inputs.policyEnvUnknown[key] {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(inputs.policyEnv[key])) {
	case "1", "true", "yes", "on":
		return true, true
	}
	return false, true
}
func (inputs *hermesInputs) policyEnvProblem(path, key, reason string) {
	if key == "" {
		for name := range hermesPolicyKeys {
			inputs.policyEnvUnknown[name] = true
			delete(inputs.policyEnv, name)
		}
	}
	if key == "HERMES_SAFE_MODE" || key == "HERMES_ENABLE_PROJECT_PLUGINS" || key == "HERMES_MANAGED_DIR" {
		inputs.policyEnvUnknown[key] = true
		delete(inputs.policyEnv, key)
		inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
		return
	}
	inputs.problem(path, reason)
}
func (inputs *hermesInputs) policyLayer(path, scope, state, reason string) {
	inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, PermissionLayer{Path: path, Scope: scope, State: state, Reason: reason, Evidence: hermesEvidence(EvidenceObserved, scope, path, "Hermes behavioral policy", reason)})
}

type hermesPolicyLeaf struct {
	names []string
	known bool
	path  string
}

func (inputs *hermesInputs) pluginProblem(path, reason string) {
	failure := hermesEvidence(EvidenceUnknown, "plugin-policy-input", path, "", reason)
	inputs.pluginFailure = &failure
	inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
}
func (inputs *hermesInputs) readPolicy(request Request) {
	inputs.policyEnv = make(map[string]string)
	inputs.policyEnvUnknown = make(map[string]bool)
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
			inputs.policyEnvProblem(inputs.config, "", "configured external secret sources are not evaluated; startup environment policy is unknown")
		}
	}
	managed := strings.TrimSpace(inputs.policyEnv["HERMES_MANAGED_DIR"])
	if managed == "" && !inputs.policyEnvUnknown["HERMES_MANAGED_DIR"] && os.Getenv("PYTEST_CURRENT_TEST") == "" {
		managed = "/etc/hermes"
	}
	if inputs.policyEnvUnknown["HERMES_MANAGED_DIR"] {
		inputs.policyEnvProblem(filepath.Join(inputs.home, ".env"), "", "managed directory selection is unresolved; dependent environment and plugin policy are unknown")
		inputs.policyLayer("", "managed-directory", "unknown", "unresolved selection; no candidate directory is inspected")
	} else if managed != "" {
		if !filepath.IsAbs(managed) {
			managed = filepath.Join(request.Project.Path, managed)
		}
		managed = filepath.Clean(managed)
		info, err := os.Stat(managed)
		switch {
		case os.IsNotExist(err):
			inputs.policyLayer(managed, "managed-directory", "absent", "selected managed directory is absent; no fallback from an explicit override")
		case err != nil:
			inputs.policyEnvProblem(managed, "", "managed directory cannot be inspected; persisted environment policy is unknown")
			inputs.policyLayer(managed, "managed-directory", "unknown", "managed policy cannot be established")
		case !info.IsDir():
			inputs.policyLayer(managed, "managed-directory", "absent", "selected managed path is not a directory")
		default:
			beforeDir := inputs.policyEnv["HERMES_MANAGED_DIR"]
			inputs.readPolicyEnv(filepath.Join(managed, ".env"), true)
			if inputs.policyEnvUnknown["HERMES_MANAGED_DIR"] || inputs.policyEnv["HERMES_MANAGED_DIR"] != beforeDir {
				inputs.pluginProblem(filepath.Join(managed, ".env"), "managed environment redirects or obscures its own directory; plugin policy resolution is unsupported")
			} else {
				inputs.readManagedPlugins(filepath.Join(managed, "config.yaml"))
			}
		}
	}
	inputs.platform = inputs.policyEnv["HERMES_PLATFORM"]
	if inputs.platform == "" {
		inputs.platform = inputs.policyEnv["HERMES_SESSION_PLATFORM"]
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{0,64}$`).MatchString(inputs.platform) {
		inputs.problem(filepath.Join(inputs.home, ".env"), "messaging platform policy is outside the supported literal shape")
	}
	platformState := "read"
	if inputs.policyEnvUnknown["HERMES_PLATFORM"] || inputs.policyEnvUnknown["HERMES_SESSION_PLATFORM"] {
		platformState = "unknown"
	}
	inputs.policyLayer(filepath.Join(inputs.home, ".env"), "session-platform", platformState, "profile environment overrides shell policy; managed environment wins last; HERMES_PLATFORM precedes HERMES_SESSION_PLATFORM")
	user := hermesMap(inputs.configData, "plugins")
	inputs.pluginPolicy = make(map[string]hermesPolicyLeaf)
	for _, key := range []string{"enabled", "disabled"} {
		value, path, observation := user[key], inputs.config, inputs.configYAML
		if managed, present := inputs.pluginManaged[key]; present || (inputs.pluginManaged != nil && user != nil && !inputs.configYAML.mappingAt("plugins")) {
			value, path, observation = managed, inputs.managedConfigPath, inputs.managedYAML
		}
		leaf := inputs.expandPluginPolicy(value, path, key)
		leaf.known = leaf.known && !inputs.policyEnvUnknown["HERMES_MANAGED_DIR"] && observation.known("plugins."+key)
		inputs.pluginPolicy[key] = leaf
		state := "read"
		if !leaf.known {
			state = "unknown"
		}
		inputs.policyLayer(path, "plugins."+key, state, "effective plugin leaf; managed policy replaces only present leaves")
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
		inputs.policyEnvProblem(path, "", "environment policy cannot be read")
		inputs.policyLayer(path, scope, "unknown", "environment policy cannot be read")
		return
	}
	text := string(raw)
	if !managed {
		text = strings.TrimPrefix(text, "\ufeff")
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		inputs.policyEnvProblem(path, "", "environment policy requires unsupported native encoding or normalization")
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
		if key := hermesDotenvDeclarationKey(line); key != "" {
			inputs.policyDeclared[key] = true
		}
		key, value, present := hermesDotenvAssignment(line)
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !present {
			continue
		}
		if !managed && !hermesDotenvQuoteClosed(value) {
			uncertain = true
			inputs.policyEnvProblem(path, "", "multiline or unclosed environment binding is outside the bounded inert parser")
			break // A later line may be part of this value rather than another assignment.
		}
		if !managed && key != strings.Trim(key, `"'`) && hermesPolicyKeys[strings.Trim(key, `"'`)] {
			uncertain = true
			inputs.policyEnvProblem(path, strings.Trim(key, `"'`), "quoted behavioral policy key is outside the supported syntax")
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
				inputs.policyEnvProblem(path, key, "unsupported interpolation or syntax for behavioral policy key "+key)
				continue
			}
		}
		inputs.policyEnv[key] = value
		delete(inputs.policyEnvUnknown, key)
	}
	state, reason := "read", "only literal behavioral policy is read; other environment values are omitted"
	if uncertain {
		state, reason = "unknown", "a relevant environment policy assignment cannot be interpreted"
	}
	inputs.policyLayer(path, scope, state, reason)
}

var hermesDotenvExport = regexp.MustCompile(`^export\s+`)

// Declaration identity is shared by the profile and managed dotenv loaders. Values
// remain inert unless the separate behavioral policy parser supports them.
func hermesDotenvDeclarationKey(line string) string {
	key, _, present := hermesDotenvAssignment(hermesDotenvExport.ReplaceAllString(line, ""))
	if !present {
		return ""
	}
	if strings.HasPrefix(key, "'") {
		return key[1 : len(key)-1]
	}
	return key
}

func hermesDotenvAssignment(line string) (key, value string, present bool) {
	if strings.HasPrefix(line, "'") {
		end := strings.IndexByte(line[1:], '\'')
		if end < 0 {
			return "", "", false
		}
		value, present = strings.CutPrefix(strings.TrimSpace(line[end+2:]), "=")
		return line[:end+2], value, present
	}
	key, value, present = strings.Cut(line, "=")
	return strings.TrimSpace(key), value, present
}

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
	observation := observeHermesYAML(raw)
	if !observation.nativeConstructible() || !observation.mappingAt() {
		inputs.pluginProblem(path, "managed plugin policy requires supported native YAML constructors and a mapping root; native fail-open fallback is not observed")
		inputs.policyLayer(path, "managed-plugin-config", "unknown", "managed policy interpretation failed; its values are not applied")
		return
	}
	value, present := data["plugins"]
	if present && value != nil && (hermesMap(data, "plugins") == nil || !observation.mappingAt("plugins")) {
		inputs.pluginProblem(path, "managed plugins must be a mapping")
	}
	inputs.managedYAML = observation
	inputs.pluginManaged = hermesMap(data, "plugins")
	inputs.managedConfigPath = path
	inputs.policyLayer(path, "managed-plugin-config", "read", "managed values replace user values per leaf; raw skills.disabled remains profile-only")
}

var hermesConfigReference = regexp.MustCompile(`\$\{([^}]+)\}`)

func (inputs *hermesInputs) expandPluginPolicy(value any, path, key string) hermesPolicyLeaf {
	leaf := hermesPolicyLeaf{path: path, known: true}
	problem := func(reason string) {
		leaf.known = false
		inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, "plugins."+key+": "+reason, nil, true))
	}
	if value == nil {
		return leaf
	}
	items, ok := value.([]any)
	if !ok {
		problem("plugin policy list cannot be interpreted")
		return leaf
	}
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			problem("plugin policy list requires strings")
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
			problem("plugin policy references unsupported or unresolved environment policy; credentials and providers are not evaluated")
			return ref
		})
		leaf.names = append(leaf.names, text)
	}
	return leaf
}
