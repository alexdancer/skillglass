package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/transform"
)

const geminiValidatedVersion = "Gemini CLI 0.63.0"

var geminiSettingsPlaceholder = regexp.MustCompile(`\$(?:\w+|\{[^}]+\})`)

type geminiInputs struct {
	controlFacts         geminiControlFacts
	overlay              map[string][]byte
	scope                AgentScope
	home, project        string
	enabled, folderTrust bool
	disabled             map[string]string
	enabledPath          string
	folderTrustPath      string
	uncertain            bool
	trust                ParticipationState
	trustEvidence        Evidence
	problems             []Problem
	extensionPolicy      bool
	allowedPolicy        bool
	blockGitPolicy       bool
}

type geminiTrustRule struct {
	path, level string
}

type geminiTrustRules []geminiTrustRule

func (rules *geminiTrustRules) UnmarshalJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("trusted folders must be a JSON object")
	}
	*rules = make(geminiTrustRules, 0)
	positions := make(map[string]int)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		path := key.(string)
		level, _ := value.(string)
		rule := geminiTrustRule{path, level}
		if index, ok := positions[path]; ok {
			(*rules)[index] = rule
		} else {
			positions[path] = len(*rules)
			*rules = append(*rules, rule)
		}
	}
	_, err = decoder.Token()
	return err
}

func geminiTrustKey(path string) (string, error) {
	key := strings.ReplaceAll(filepath.Clean(path), "\\", "/")
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return geminiComparisonKey(key)
	}
	return key, nil
}

func geminiComparisonKey(value string) (string, error) {
	source := []byte(value)
	destination := make([]byte, max(1, len(source)))
	lower := cases.Lower(language.Und)
	for {
		// Whole-input retries retain the context lost by String's 128-byte chunks.
		lower.Reset()
		written, consumed, err := lower.Transform(destination, source, true)
		if err == transform.ErrShortDst {
			if len(destination) > int(^uint(0)>>1)/2 {
				return "", errors.New("lowercase comparison key exceeds buffer capacity")
			}
			destination = make([]byte, len(destination)*2)
			continue
		}
		if err != nil {
			return "", err
		}
		if consumed != len(source) {
			return "", errors.New("lowercase comparison key did not consume the complete input")
		}
		return string(destination[:written]), nil
	}
}

func geminiEvidence(kind EvidenceKind, rule, path, setting, reason string) Evidence {
	return Evidence{Kind: kind, Rule: rule, InputPath: path, Setting: setting, AgentVersion: geminiValidatedVersion, Reason: reason}
}
func (in *geminiInputs) problem(path, reason string) {
	in.uncertain = true
	in.problems = append(in.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
}
func geminiEnvPath(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		if path, err := filepath.Abs(value); err == nil {
			return path
		}
	}
	return fallback
}

// Preserve quoted strings while blanking JSON comments, matching native JSONC settings.
func geminiJSON(raw []byte, output any) error {
	data, err := geminiJSONBytes(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, output)
}
func geminiJSONBytes(raw []byte) ([]byte, error) {
	data := append([]byte(nil), raw...)
	quoted, escaped := false, false
	for i := 0; i < len(data); i++ {
		if quoted {
			if escaped {
				escaped = false
			} else if data[i] == '\\' {
				escaped = true
			} else if data[i] == '"' {
				quoted = false
			}
			continue
		}
		if data[i] == '"' {
			quoted = true
			continue
		}
		if data[i] != '/' || i+1 >= len(data) {
			continue
		}
		if data[i+1] == '/' {
			data[i], data[i+1] = ' ', ' '
			i += 2
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				data[i] = ' '
				i++
			}
			i--
			continue
		}
		if data[i+1] == '*' {
			data[i], data[i+1] = ' ', ' '
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				if data[i] != '\n' && data[i] != '\r' {
					data[i] = ' '
				}
				i++
			}
			if i+1 >= len(data) {
				return nil, errors.New("unterminated JSON comment")
			}
			data[i], data[i+1] = ' ', ' '
			i++
		}
	}
	return data, nil
}
func geminiObject(data map[string]any, keys ...string) (any, bool, bool) {
	var current any = data
	for i, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, true, false
		}
		value, present := object[key]
		if !present {
			return nil, false, true
		}
		if i == len(keys)-1 {
			return value, true, true
		}
		current = value
	}
	return nil, false, true
}
func (in *geminiInputs) loadSettings(path, scope string, apply bool) {
	layer := PermissionLayer{Path: path, Scope: scope, State: "absent", Reason: "optional settings file does not exist", Evidence: geminiEvidence(EvidenceObserved, "settings-input", path, "", "only relevant settings are reported")}
	raw, err := nativeBytes(path)
	if value, ok := in.overlay[path]; ok {
		raw, err = value, nil
	}
	if nativeAbsent(path, err) {
		in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
		return
	}
	if scope == "system" || scope == "system-defaults" {
		if reason := geminiSystemSecurity(path); reason != "" {
			layer.State = "ignored"
			layer.Reason = reason
			in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
			return
		}
	}
	var data map[string]any
	if err != nil || geminiJSON(raw, &data) != nil || data == nil {
		layer.State = "unknown"
		layer.Reason = "settings are unreadable or invalid JSONC"
		in.problem(path, layer.Reason)
		in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
		return
	}
	layer.State = "loaded"
	layer.Reason = "scalar settings use last layer; skills.disabled arrays are unioned"
	if !apply {
		layer.State = "ignored"
		layer.Reason = "workspace settings require established folder trust"
		in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
		return
	}
	for _, field := range []struct {
		keys   []string
		target *bool
	}{{[]string{"skills", "enabled"}, &in.enabled}, {[]string{"security", "folderTrust", "enabled"}, &in.folderTrust}} {
		value, present, valid := geminiObject(data, field.keys...)
		if !present {
			continue
		}
		boolean, ok := value.(bool)
		if !valid || !ok {
			in.problem(path, "relevant setting requires a boolean")
			layer.State = "unknown"
			continue
		}
		*field.target = boolean
		if field.keys[0] == "skills" {
			in.enabledPath = path
		} else {
			in.folderTrustPath = path
		}
		layer.Choices = append(layer.Choices, PermissionChoice{Path: strings.Join(field.keys, "."), Enabled: boolean})
	}
	value, present, valid := geminiObject(data, "skills", "disabled")
	if present {
		names, ok := value.([]any)
		if !valid || !ok {
			in.problem(path, "skills.disabled requires an array of strings")
			layer.State = "unknown"
		} else {
			for _, item := range names {
				name, ok := item.(string)
				if !ok || geminiSettingsPlaceholder.MatchString(name) {
					in.problem(path, "skills.disabled contains an invalid or unverified environment-expanded name")
					layer.State = "unknown"
					continue
				}
				key, err := geminiComparisonKey(name)
				if err != nil {
					in.problem(path, "skills.disabled name could not be lowercased")
					layer.State = "unknown"
					continue
				}
				in.disabled[key] = path
				layer.Choices = append(layer.Choices, PermissionChoice{Path: name, Enabled: false})
			}
		}
	}
	for _, field := range []struct {
		keys   []string
		target *bool
	}{{[]string{"security", "allowedExtensions"}, &in.allowedPolicy}, {[]string{"security", "blockGitExtensions"}, &in.blockGitPolicy}} {
		value, present, valid := geminiObject(data, field.keys...)
		if !present {
			continue
		}
		active, ok := value.(bool)
		if field.keys[1] == "allowedExtensions" {
			var array []any
			array, ok = value.([]any)
			active = len(array) > 0
			for _, item := range array {
				if _, stringOK := item.(string); !stringOK {
					ok = false
				}
			}
		}
		if !valid || !ok {
			in.problem(path, strings.Join(field.keys, ".")+" has an invalid type")
			layer.State = "unknown"
			continue
		}
		*field.target = active
	}
	in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
}
func readGeminiInputs(request Request) geminiInputs {
	home := geminiEnvPath("GEMINI_CLI_HOME", request.home)
	in := geminiInputs{overlay: request.geminiOverlay, home: home, enabled: true, folderTrust: true, disabled: make(map[string]string), trust: ParticipationExcluded, scope: AgentScope{Application: "gemini", ValidatedVersion: geminiValidatedVersion, VersionEvidence: "pinned disposable native skills list and native loader/manager verification; current installation/session not inspected", Evidence: geminiEvidence(EvidenceObserved, "recorded-native-verification", "tools/verify_gemini.py", "", "credential-free native checks recorded for reference version"), Gaps: []string{"running session, IDE trust, remote admin policy, CLI overrides and shell .env overrides are not inspected", "activation consent is inferred from native policy; no model, skill, hook or extension code was executed", "same-name ordering within one directory and between extensions is unspecified; ambiguous candidates remain unknown", "secure root-owned system scalar precedence is source-verified; disposable native validation verifies insecure-system rejection",
		"bundled built-ins are searched only when a local gemini executable identifies its bundle; other packaging layouts are unverified"}}}
	system := "/etc/gemini-cli/settings.json"
	if runtime.GOOS == "darwin" {
		system = "/Library/Application Support/GeminiCli/settings.json"
	}
	system = geminiEnvPath("GEMINI_CLI_SYSTEM_SETTINGS_PATH", system)
	defaults := geminiEnvPath("GEMINI_CLI_SYSTEM_DEFAULTS_PATH", filepath.Join(filepath.Dir(system), "system-defaults.json"))
	in.loadSettings(defaults, "system-defaults", true)
	in.loadSettings(filepath.Join(home, ".gemini", "settings.json"), "user", true)
	// System overrides also govern whether workspace settings may be trusted.
	before := len(in.scope.ConfigLayers)
	in.loadSettings(system, "system", true)
	in.scope.ConfigLayers = in.scope.ConfigLayers[:before]
	if request.Project.Selected {
		project, err := filepath.EvalSymlinks(request.Project.Path)
		info, statErr := os.Stat(project)
		if err != nil || statErr != nil || !info.IsDir() {
			in.problem(request.Project.Path, "selected workspace is not a resolvable directory")
			in.trust = ParticipationUnknown
		} else {
			in.project = project
			in.readTrust()
		}
		if in.project != "" {
			in.loadSettings(filepath.Join(in.project, ".gemini", "settings.json"), "project", in.trust == ParticipationEligible)
		}
	}
	// Reapply the final override without duplicating its report.
	in.loadSettings(system, "system", true)
	if in.project != "" {
		in.readTrust()
	}
	in.extensionPolicy = in.allowedPolicy || in.blockGitPolicy
	if in.extensionPolicy {
		in.scope.Gaps = append(in.scope.Gaps, "extension security policy requires unverified install-metadata matching")
	}
	return in
}
func (in *geminiInputs) readTrust() {
	in.trust = ParticipationExcluded
	path := geminiEnvPath("GEMINI_CLI_TRUSTED_FOLDERS_PATH", filepath.Join(in.home, ".gemini", "trustedFolders.json"))
	reason := "no folder-trust choice exists; native CLI omits workspace skills until trusted"
	in.trustEvidence = geminiEvidence(EvidenceInferred, "workspace-trust", path, "trustedFolders", reason)
	if os.Getenv("GEMINI_RESTRICTED_MODE") == "true" {
		in.trustEvidence = geminiEvidence(EvidenceObserved, "workspace-trust-env", "", "GEMINI_RESTRICTED_MODE=true", "environment excludes workspace skills")
		return
	}
	if os.Getenv("GEMINI_CLI_TRUST_WORKSPACE") == "false" {
		in.trustEvidence = geminiEvidence(EvidenceObserved, "workspace-trust-env", "", "GEMINI_CLI_TRUST_WORKSPACE=false", "environment excludes workspace skills")
		return
	}
	if os.Getenv("GEMINI_CLI_TRUST_WORKSPACE") == "true" {
		in.trust = ParticipationEligible
		in.trustEvidence = geminiEvidence(EvidenceObserved, "workspace-trust-env", "", "GEMINI_CLI_TRUST_WORKSPACE=true", "environment permits workspace skills")
		return
	}
	if !in.folderTrust {
		in.trust = ParticipationEligible
		in.trustEvidence = geminiEvidence(EvidenceInferred, "workspace-trust-setting", in.folderTrustPath, "security.folderTrust.enabled=false", "disabled trust feature permits workspace skills")
		return
	}
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		return
	}
	var rules geminiTrustRules
	if err != nil || geminiJSON(raw, &rules) != nil || rules == nil {
		in.problem(path, "trusted folders input is unreadable or invalid")
		in.trust = ParticipationUnknown
		return
	}
	type normalizedRule struct {
		key, rawPath, level string
	}
	var ordered []normalizedRule
	positions := make(map[string]int)
	for _, rule := range rules {
		if rule.level != "TRUST_FOLDER" && rule.level != "TRUST_PARENT" && rule.level != "DO_NOT_TRUST" {
			in.problem(path, "trusted folders contains an invalid level")
			in.trust = ParticipationUnknown
			return
		}
		if !filepath.IsAbs(rule.path) {
			reason := "relative trusted-folder keys require an uninspected native working directory"
			in.problem(path, reason)
			if !slices.Contains(in.scope.Gaps, reason) {
				in.scope.Gaps = append(in.scope.Gaps, reason)
			}
			in.trust = ParticipationUnknown
			return
		}
		key, err := geminiTrustKey(rule.path)
		if err != nil {
			in.problem(path, "trusted-folder key could not be lowercased")
			in.trust = ParticipationUnknown
			return
		}
		normalized := normalizedRule{key, rule.path, rule.level}
		if index, ok := positions[key]; ok {
			ordered[index] = normalized
		} else {
			positions[key] = len(ordered)
			ordered = append(ordered, normalized)
		}
	}
	length := -1
	project, err := geminiTrustKey(in.project)
	if err != nil {
		in.problem(path, "workspace path could not be lowercased for folder trust")
		in.trust = ParticipationUnknown
		return
	}
	for _, rule := range ordered {
		target := rule.key
		if rule.level == "TRUST_PARENT" {
			target = filepath.Dir(target)
		}
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			target = resolved
		}
		target, err = geminiTrustKey(target)
		if err != nil {
			in.problem(path, "resolved trusted-folder path could not be lowercased")
			in.trust = ParticipationUnknown
			return
		}
		rel, err := filepath.Rel(target, project)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		state := ParticipationEligible
		if rule.level == "DO_NOT_TRUST" {
			state = ParticipationExcluded
		}
		specificity := len(utf16.Encode([]rune(rule.key)))
		if specificity > length {
			length = specificity
			in.trust = state
			in.trustEvidence = geminiEvidence(EvidenceInferred, "workspace-trust", path, rule.level, "longest matching native trust rule: "+rule.rawPath)
		}
	}
}
func (in *geminiInputs) permission(name string) PermissionExplanation {
	result := PermissionExplanation{State: PermissionAllowed, Layers: cloneLayers(in.scope.ConfigLayers), Evidence: geminiEvidence(EvidenceInferred, "skills-default-enabled", "", "", "skills enabled; name absent from unioned disabled lists")}
	if !in.enabled {
		result.State = PermissionDenied
		result.Evidence = geminiEvidence(EvidenceInferred, "skills-enabled", in.enabledPath, "skills.enabled=false", "skills subsystem is disabled")
	}
	key, err := geminiComparisonKey(name)
	if err != nil {
		result.State = PermissionUnknown
		result.Evidence = geminiEvidence(EvidenceUnknown, "invalid-name-comparison", "", "", "native skill name could not be lowercased for disabled-name comparison")
		return result
	}
	if path, ok := in.disabled[key]; ok {
		result.State = PermissionDenied
		result.Evidence = geminiEvidence(EvidenceInferred, "disabled-name-union", path, "skills.disabled", "case-insensitive disabled name remains disabled across settings layers")
	}
	if in.uncertain {
		result.State = PermissionUnknown
		result.Evidence = geminiEvidence(EvidenceUnknown, "incomplete-settings", "", "", "invalid or unverified native inputs prevent reliable availability")
	}
	return result
}

// Gemini requires root ownership and no group/other writes on both lexical and resolved paths.
func geminiSystemSecurity(path string) string {
	if runtime.GOOS == "windows" {
		return "Windows system-settings ownership validation is unverified"
	}
	paths := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		paths = append(paths, resolved)
	}
	for _, candidate := range paths {
		for current := candidate; ; current = filepath.Dir(current) {
			info, err := os.Stat(current)
			if err != nil {
				return "native system settings security could not be established"
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 {
				return "native system settings ignored: file and all parent directories must be root-owned and not writable by group or others"
			}
			if link, err := os.Lstat(current); err == nil && link.Mode()&os.ModeSymlink != 0 {
				owner, ok := link.Sys().(*syscall.Stat_t)
				if !ok || owner.Uid != 0 {
					return "native system settings ignored: parent or file symlink is not root-owned"
				}
			}
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	return ""
}

func geminiHome() (string, error) {
	if value := os.Getenv("GEMINI_CLI_HOME"); value != "" {
		return filepath.Abs(value)
	}
	return nativeHome()
}
