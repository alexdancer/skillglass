package collection

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const claudeValidatedVersion = "Claude Code 2.1.282"

type claudeSetting struct {
	value string
	path  string
}
type claudeRule struct{ selector, kind, path string }
type claudeInputs struct {
	scope     AgentScope
	overrides map[string]claudeSetting
	plugins   map[string]claudeSetting
	rules     []claudeRule
	problems  []Problem
	uncertain bool
	strict    bool
	safe      bool
}

func claudeHome(home string) string {
	if value := os.Getenv("CLAUDE_CONFIG_DIR"); value != "" {
		if path, err := filepath.Abs(value); err == nil {
			return path
		}
	}
	return filepath.Join(home, ".claude")
}
func claudeEvidence(kind EvidenceKind, rule, path, setting, reason string) Evidence {
	return Evidence{Kind: kind, Rule: rule, InputPath: path, Setting: setting, AgentVersion: claudeValidatedVersion, Reason: reason}
}
func (inputs *claudeInputs) problem(path, reason string) {
	inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
	inputs.uncertain = true
}
func (inputs *claudeInputs) load(path, scope string) {
	layer := PermissionLayer{Path: path, Scope: scope, State: "absent", Reason: "optional settings file does not exist", Evidence: claudeEvidence(EvidenceObserved, "settings-input", path, "", "filesystem input only; running session not inspected")}
	defer func() { inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer) }()
	raw, err := nativeBytes(path)
	if nativeAbsent(path, err) {
		return
	}
	var data map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &data) != nil || data == nil {
		layer.State = "invalid"
		layer.Reason = "settings cannot be reliably interpreted"
		inputs.problem(path, layer.Reason)
		return
	}
	layer.State = "parsed"
	layer.Reason = "observed relevant settings; user < project < local < managed for scalar entries; permission lists merge"
	for _, key := range []string{"skillOverrides", "enabledPlugins"} {
		value, present := data[key]
		if !present {
			continue
		}
		var entries map[string]json.RawMessage
		if json.Unmarshal(value, &entries) != nil || entries == nil {
			inputs.problem(path, key+" must be an object")
			layer.State = "invalid"
			continue
		}
		names := make([]string, 0, len(entries))
		for name := range entries {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			var text string
			if key == "skillOverrides" {
				if json.Unmarshal(entries[name], &text) != nil || (text != "on" && text != "off" && text != "name-only" && text != "user-invocable-only") {
					inputs.problem(path, "invalid skillOverrides state for "+name)
					layer.State = "invalid"
					continue
				}
				inputs.overrides[name] = claudeSetting{text, path}
			} else {
				var enabled bool
				if json.Unmarshal(entries[name], &enabled) != nil || string(entries[name]) == "null" {
					inputs.problem(path, "enabledPlugins value must be boolean for "+name)
					layer.State = "invalid"
					continue
				}
				text = "false"
				if enabled {
					text = "true"
				}
				inputs.plugins[name] = claudeSetting{text, path}
			}
			layer.Reason += "; " + key + "." + name + "=" + text
		}
	}
	if value, ok := data["permissions"]; ok {
		var permission map[string]json.RawMessage
		if json.Unmarshal(value, &permission) != nil || permission == nil {
			inputs.problem(path, "permissions must be an object")
			layer.State = "invalid"
		} else {
			for _, kind := range []string{"allow", "ask", "deny"} {
				if rawRules, ok := permission[kind]; ok {
					var rules []string
					if json.Unmarshal(rawRules, &rules) != nil || string(rawRules) == "null" {
						inputs.problem(path, "permissions."+kind+" must be a string array")
						layer.State = "invalid"
						continue
					}
					for _, rule := range rules {
						if rule == "Skill" || strings.HasPrefix(rule, "Skill(") {
							inputs.rules = append(inputs.rules, claudeRule{rule, kind, path})
							layer.Reason += "; permissions." + kind + "=" + rule
						}
					}
				}
			}
			if _, ok := permission["additionalDirectories"]; ok {
				layer.Reason += "; additionalDirectories grants file access, not skill discovery"
			}
		}
	}
	if value, ok := data["strictPluginOnlyCustomization"]; ok {
		var strict bool
		if json.Unmarshal(value, &strict) != nil {
			inputs.problem(path, "strictPluginOnlyCustomization must be boolean")
			layer.State = "invalid"
		} else if scope == "managed" {
			inputs.strict = strict
		}
	}
	if _, ok := data["allowManagedPermissionRulesOnly"]; ok {
		inputs.problem(path, "allowManagedPermissionRulesOnly enforcement is unverified")
	}
}
func readClaudeInputs(request Request) claudeInputs {
	inputs := claudeInputs{overrides: map[string]claudeSetting{}, plugins: map[string]claudeSetting{}, safe: os.Getenv("CLAUDE_CODE_SAFE_MODE") == "1"}
	inputs.scope = AgentScope{Application: "claude-code", ValidatedVersion: claudeValidatedVersion, VersionEvidence: "docs/verification/ale-29.md; disposable native SDK initialize catalog, no model prompts", Evidence: claudeEvidence(EvidenceInferred, "local-context", "", "", "local startup inference bounded to the selected home and project"), Gaps: []string{
		"existing session, trust, CLI --setting-sources/--add-dir/--plugin-dir/--disable-slash-commands and bare/restricted modes are not observed",
		"account-synced caches, bundled skills, legacy command files and remote/MDM managed policy are not validated or inventoried as user skill bundles",
		"remote marketplace plugin activation is unverified; a cache, registry entry or folder alone does not establish activation",
		"main-checkout fallback in linked worktrees, non-repository ancestors and runtime nested-directory loading are not validated",
		"enterprise filesystem precedence is a documented assumption; local/remote managed policy enforcement was not exercised",
		"Skill permission enforcement is documented inference; no model invocation or skill workload was performed",
	}}
	inputs.load(filepath.Join(request.claudeHome, "settings.json"), "global")
	if request.Project.Selected {
		inputs.load(filepath.Join(request.Project.Path, ".claude", "settings.json"), "project")
		inputs.load(filepath.Join(request.Project.Path, ".claude", "settings.local.json"), "project-local")
	}
	inputs.load("/Library/Application Support/ClaudeCode/managed-settings.json", "managed")
	return inputs
}
func claudeRuleMatches(selector, name string) bool {
	if selector == "Skill" || selector == "Skill(*)" {
		return true
	}
	if !strings.HasPrefix(selector, "Skill(") || !strings.HasSuffix(selector, ")") {
		return false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(selector, "Skill("), ")")
	if strings.HasSuffix(value, " *") {
		value = strings.TrimSuffix(value, " *")
	}
	return value == name
}
func (inputs *claudeInputs) permission(name string) PermissionExplanation {
	result := PermissionExplanation{State: PermissionAllowed, Layers: cloneLayers(inputs.scope.ConfigLayers), Evidence: claudeEvidence(EvidenceInferred, "skill-permission-default", "", "", "no relevant Skill deny or ask rule observed; invocation is not proof of use")}
	if inputs.uncertain {
		result.State = PermissionUnknown
		result.Evidence = claudeEvidence(EvidenceUnknown, "invalid-native-input", "", "", "a relevant native input could not be interpreted")
		return result
	}
	for _, kind := range []string{"deny", "ask", "allow"} {
		for _, rule := range inputs.rules {
			if rule.kind == kind && claudeRuleMatches(rule.selector, name) {
				if kind == "deny" {
					result.State = PermissionDenied
				}
				result.Evidence = claudeEvidence(EvidenceInferred, "skill-tool-"+kind, rule.path, "permissions."+kind+"="+rule.selector, "permission lists merge across settings scopes; deny precedes ask, then allow; this governs model Skill tool use, not the explicit slash command")
				return result
			}
		}
	}
	return result
}

func claudeProjectAncestors(project ProjectContext) ([]string, error) {
	if !project.Selected {
		return nil, nil
	}
	selected, err := filepath.EvalSymlinks(project.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(selected)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("selected project is not a directory")
	}
	var result []string
	for path := selected; ; path = filepath.Dir(path) {
		result = append(result, path)
		if actualGitBoundary(path) || filepath.Dir(path) == path {
			return result, nil
		}
	}
}
