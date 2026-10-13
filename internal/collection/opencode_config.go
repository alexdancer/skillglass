package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

const opencodeValidatedVersion = "OpenCode 1.17.15"

// JSON object order is native permission semantics, not presentation order.
type opencodeObject struct {
	keys   []string
	values map[string]any
	paths  map[string]string
}

func newOpencodeObject() *opencodeObject {
	return &opencodeObject{values: map[string]any{}, paths: map[string]string{}}
}
func (o *opencodeObject) get(key string) any {
	if o == nil {
		return nil
	}
	return o.values[key]
}
func (o *opencodeObject) put(key string, value any, path string) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
	o.paths[key] = path
}
func mergeOpencode(target, source *opencodeObject) {
	for _, key := range source.keys {
		old, a := target.get(key).(*opencodeObject)
		next, b := source.get(key).(*opencodeObject)
		if a && b {
			mergeOpencode(old, next)
		} else {
			target.put(key, source.get(key), source.paths[key])
		}
	}
}
func opencodeEvidence(kind EvidenceKind, rule, path, setting, reason string) Evidence {
	return Evidence{Kind: kind, Rule: rule, InputPath: path, Setting: setting, AgentVersion: opencodeValidatedVersion, Reason: reason}
}
func opencodeBool(key string) bool {
	return os.Getenv(key) == "1" || strings.EqualFold(os.Getenv(key), "true")
}
func opencodePath(raw, base, home string) string {
	if strings.HasPrefix(raw, "~/") {
		return filepath.Join(home, raw[2:])
	}
	if !filepath.IsAbs(raw) {
		path, _ := filepath.Abs(filepath.Join(base, raw))
		return path
	}
	return filepath.Clean(raw)
}
func opencodeConfigHome(home string) string {
	if raw := os.Getenv("XDG_CONFIG_HOME"); raw != "" {
		return opencodePath(raw, ".", home)
	}
	return filepath.Join(home, ".config")
}
func opencodeCacheHome(home string) string {
	if raw := os.Getenv("XDG_CACHE_HOME"); raw != "" {
		return opencodePath(raw, ".", home)
	}
	return filepath.Join(home, ".cache")
}

// Remove JSONC comments and trailing commas without altering quoted strings.
func opencodeJSONC(raw []byte) ([]byte, error) {
	if int64(len(raw)) > maxManifestBytes {
		return nil, errors.New("config exceeds read limit")
	}
	out := append([]byte(nil), raw...)
	quoted, escaped := false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c != '/' || i+1 >= len(out) {
			continue
		}
		if out[i+1] == '/' {
			for i < len(out) && out[i] != '\n' {
				out[i] = ' '
				i++
			}
			i--
		} else if out[i+1] == '*' {
			out[i] = ' '
			out[i+1] = ' '
			i += 2
			for i+1 < len(out) && !(out[i] == '*' && out[i+1] == '/') {
				if out[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if i+1 >= len(out) {
				return nil, errors.New("unterminated comment")
			}
			out[i] = ' '
			out[i+1] = ' '
			i++
		}
	}
	quoted, escaped = false, false
	for i, c := range out {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(out) && strings.ContainsRune(" \t\r\n", rune(out[j])) {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				out[i] = ' '
			}
		}
	}
	return out, nil
}
func parseOpencodeValue(dec *json.Decoder, path string, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("config nesting exceeds read limit")
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	if d == '{' {
		obj := newOpencodeObject()
		for dec.More() {
			k, e := dec.Token()
			if e != nil {
				return nil, e
			}
			key, ok := k.(string)
			if !ok {
				return nil, errors.New("invalid key")
			}
			if _, dup := obj.values[key]; dup {
				return nil, errors.New("duplicate JSON key")
			}
			v, e := parseOpencodeValue(dec, path, depth+1)
			if e != nil {
				return nil, e
			}
			obj.put(key, v, path)
		}
		_, err = dec.Token()
		return obj, err
	}
	if d == '[' {
		var list []any
		for dec.More() {
			v, e := parseOpencodeValue(dec, path, depth+1)
			if e != nil {
				return nil, e
			}
			list = append(list, v)
		}
		_, err = dec.Token()
		return list, err
	}
	return nil, errors.New("invalid delimiter")
}
func parseOpencode(raw []byte, path string) (*opencodeObject, error) {
	// Substitution may read arbitrary secret files natively. Offline browsing never performs it.
	if bytes.Contains(raw, []byte("{env:")) || bytes.Contains(raw, []byte("{file:")) {
		return nil, errors.New("native substitutions are not evaluated")
	}
	cleaned, err := opencodeJSONC(raw)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	v, err := parseOpencodeValue(dec, path, 0)
	if err != nil {
		return nil, err
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected trailing JSON")
	}
	obj, ok := v.(*opencodeObject)
	if !ok {
		return nil, errors.New("config is not an object")
	}
	return obj, nil
}

type opencodeInputs struct {
	scope                AgentScope
	config               *opencodeObject
	problems             []Problem
	uncertain            bool
	dirs                 []string
	home, project, cache string
	plugins              bool
}

func (in *opencodeInputs) problem(path, reason string) {
	in.problems = append(in.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
	in.uncertain = true
}
func (in *opencodeInputs) load(path, scope string, raw []byte, required bool) {
	layer := PermissionLayer{Path: path, Scope: scope, State: "absent", Reason: "optional local config does not exist", Evidence: opencodeEvidence(EvidenceObserved, "config-layer", path, "", "bounded read-only local input")}
	var err error
	if raw == nil {
		raw, err = nativeBytes(path)
		if nativeAbsent(path, err) && !required {
			in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
			return
		}
	}
	obj, parseErr := parseOpencode(raw, path)
	if err != nil || parseErr != nil {
		layer.State = "unknown"
		layer.Reason = "config cannot be interpreted safely; content is not reported"
		in.problem(path, layer.Reason)
	} else {
		layer.State = "loaded"
		layer.Reason = "deep merge in native order; conflicting keys replace values without reordering existing keys"
		in.validate(obj, path)
		normalizeOpencodeConfig(obj)
		mergeOpencode(in.config, obj)
	}
	in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
}
func (in *opencodeInputs) loadPair(dir, scope string) {
	for _, name := range []string{"opencode.json", "opencode.jsonc"} {
		in.load(filepath.Join(dir, name), scope, nil, false)
	}
}
func readOpencodeInputs(request Request, ancestors []string) *opencodeInputs {
	home := request.home
	global := filepath.Join(opencodeConfigHome(home), "opencode")
	project := request.Project.Path
	if project == "" {
		project = home
	}
	in := &opencodeInputs{config: newOpencodeObject(), home: home, project: project, cache: filepath.Join(opencodeCacheHome(home), "opencode"), scope: AgentScope{Application: "opencode", ValidatedVersion: opencodeValidatedVersion, VersionEvidence: "recorded disposable native debug skill/config/agent receipts; docs/verification/ale-35.md", Evidence: opencodeEvidence(EvidenceInferred, "version-specific-local-rules", "", "", "not the installed version or a running-session observation"), Gaps: []string{"Running session, CLI --agent selection, saved approval rules and remote account/organization settings are unknown.", "OpenCode 1.17.15 debug discovery overwrites equal names concurrently; no deterministic winning/shadowed definition is inferred.", "Embedded customize-opencode skill has no local bundle and is not inventoried.", "Later docs describe metadata opencode/autoinvoke; 1.17.15 v1 guidance does not implement that field."}}}
	for _, key := range []string{"OPENCODE_DISABLE_EXTERNAL_SKILLS", "OPENCODE_DISABLE_CLAUDE_CODE", "OPENCODE_DISABLE_CLAUDE_CODE_SKILLS", "OPENCODE_PURE", "OPENCODE_DISABLE_PROJECT_CONFIG"} {
		value := strings.ToLower(os.Getenv(key))
		if value != "" && value != "0" && value != "1" && value != "true" && value != "false" {
			in.problem(key, "native boolean environment value is invalid")
		}
	}
	if os.Getenv("OPENCODE_TEST_HOME") != "" {
		in.problem("OPENCODE_TEST_HOME", "native test-home override is not part of the validated viewer scope")
	}
	for _, name := range []string{"config.json", "opencode.json", "opencode.jsonc"} {
		in.load(filepath.Join(global, name), "global", nil, false)
	}
	if raw := os.Getenv("OPENCODE_CONFIG"); raw != "" {
		in.load(opencodePath(raw, project, home), "custom-config", nil, true)
	}
	if !opencodeBool("OPENCODE_DISABLE_PROJECT_CONFIG") {
		for i := len(ancestors) - 1; i >= 0; i-- { // ConfigPaths.files reverses nearest-first jsonc/json into root-first json/jsonc.
			in.loadPair(ancestors[i], "project")
		}
	}
	in.dirs = append(in.dirs, global)
	if !opencodeBool("OPENCODE_DISABLE_PROJECT_CONFIG") {
		for _, ancestor := range ancestors {
			in.dirs = append(in.dirs, filepath.Join(ancestor, ".opencode"))
		}
	}
	in.dirs = append(in.dirs, filepath.Join(home, ".opencode"))
	if raw := os.Getenv("OPENCODE_CONFIG_DIR"); raw != "" {
		in.dirs = append(in.dirs, opencodePath(raw, project, home))
	}
	seen := map[string]bool{}
	dirs := in.dirs
	in.dirs = nil
	for _, dir := range dirs {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		in.dirs = append(in.dirs, dir)
		if dir != global {
			in.loadPair(dir, "native-directory")
		}
		in.detectDynamic(dir)
	}
	if raw := os.Getenv("OPENCODE_CONFIG_CONTENT"); raw != "" {
		in.load("OPENCODE_CONFIG_CONTENT", "inline", []byte(raw), true)
	}
	managed := "/Library/Application Support/opencode"
	if raw := os.Getenv("OPENCODE_TEST_MANAGED_CONFIG_DIR"); raw != "" {
		managed = opencodePath(raw, project, home)
	}
	in.loadPair(managed, "managed")
	// Presence is enough to mark MDM unverified, without reading private preference contents.
	username := os.Getenv("USER")
	if current, err := user.Current(); err == nil {
		username = current.Username
	}
	for _, path := range []string{"/Library/Managed Preferences/ai.opencode.managed.plist", filepath.Join("/Library/Managed Preferences", username, "ai.opencode.managed.plist")} {
		if _, err := os.Stat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
			in.problem(path, "managed preferences are present but their effective policy is unverified")
		}
	}
	if raw := os.Getenv("OPENCODE_PERMISSION"); raw != "" {
		in.load("OPENCODE_PERMISSION", "runtime-permission", []byte(`{"permission":`+raw+`}`), true)
	}
	if items, ok := in.config.get("plugin").([]any); ok && len(items) > 0 && !opencodeBool("OPENCODE_PURE") {
		in.plugins = true
		in.problem("plugin", "registered plugins may change configuration; hooks are never executed")
	}
	if skills, ok := in.config.get("skills").(*opencodeObject); ok {
		if urls, ok := skills.get("urls").([]any); ok && len(urls) > 0 {
			in.problem("skills.urls", "remote skill indexes are not fetched; cache presence cannot prove current activation or precedence")
		}
	}
	if in.config.get("mode") != nil {
		in.problem("mode", "legacy mode overrides are unverified")
	}
	return in
}
func (in *opencodeInputs) detectDynamic(dir string) {
	if opencodeBool("OPENCODE_PURE") {
		return
	}
	for _, name := range []string{"plugin", "plugins"} {
		entries, err := os.ReadDir(filepath.Join(dir, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			in.problem(filepath.Join(dir, name), "plugin directory cannot be inspected")
		}
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".ts") || strings.HasSuffix(entry.Name(), ".js")) {
				in.plugins = true
				in.problem(filepath.Join(dir, name), "local plugin hooks are not executed; effective native state is unknown")
				break
			}
		}
	}
}
func opencodeWildcard(value, pattern string) bool {
	value = strings.ReplaceAll(value, `\`, "/")
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*`, ".*")
	quoted = strings.ReplaceAll(quoted, `\?`, ".")
	if strings.HasSuffix(quoted, " .*") {
		quoted = strings.TrimSuffix(quoted, " .*") + "( .*)?"
	}
	ok, _ := regexp.MatchString("(?s)^"+quoted+"$", value)
	return ok
}

type opencodeRule struct{ tool, pattern, action, path, setting string }

func (in *opencodeInputs) rules(value any, setting string) []opencodeRule {
	var rules []opencodeRule
	if action, ok := value.(string); ok {
		return []opencodeRule{{tool: "*", pattern: "*", action: action, setting: setting}}
	}
	obj, ok := value.(*opencodeObject)
	if value == nil {
		return nil
	}
	if !ok {
		in.problem(setting, "permission must be an action or ordered object")
		return nil
	}
	for _, tool := range obj.keys {
		if !opencodeWildcard("skill", tool) {
			continue
		}
		v := obj.get(tool)
		if action, ok := v.(string); ok {
			rules = append(rules, opencodeRule{tool: tool, pattern: "*", action: action, path: obj.paths[tool], setting: setting + "." + tool})
			continue
		}
		patterns, ok := v.(*opencodeObject)
		if !ok {
			in.problem(obj.paths[tool], "skill permission has an unsupported value")
			continue
		}
		for _, pattern := range patterns.keys {
			action, ok := patterns.get(pattern).(string)
			if !ok {
				in.problem(patterns.paths[pattern], "skill permission action is not a string")
				continue
			}
			rules = append(rules, opencodeRule{tool: tool, pattern: expandOpencodePattern(pattern, in.home), action: action, path: patterns.paths[pattern], setting: setting + "." + tool + "." + pattern})
		}
	}
	return rules
}
func (in *opencodeInputs) permission(name string) PermissionExplanation {
	rules := []opencodeRule{{tool: "*", pattern: "*", action: "allow", setting: "native default"}}
	if tools, ok := in.config.get("tools").(*opencodeObject); ok {
		if enabled, ok := tools.get("skill").(bool); ok {
			action := "deny"
			if enabled {
				action = "allow"
			}
			rules = append(rules, opencodeRule{tool: "skill", pattern: "*", action: action, path: tools.paths["skill"], setting: "tools.skill"})
		} else if tools.get("skill") != nil {
			in.problem(tools.paths["skill"], "tools.skill is not boolean")
		}
	}
	rules = append(rules, in.rules(in.config.get("permission"), "permission")...)
	agent := "build"
	if raw := in.config.get("default_agent"); raw != nil {
		selected, ok := raw.(string)
		if !ok {
			in.problem("default_agent", "default agent is not a string")
		} else {
			agent = selected
		}
	}
	if agent != "build" && agent != "plan" {
		agents, ok := in.config.get("agent").(*opencodeObject)
		if !ok || agents.get(agent) == nil {
			in.problem("default_agent", "selected native agent is not known in local config")
		}
	}
	if agents, ok := in.config.get("agent").(*opencodeObject); ok {
		if a, ok := agents.get(agent).(*opencodeObject); ok {
			rules = append(rules, in.rules(a.get("permission"), "agent."+agent+".permission")...)

			if hidden, ok := a.get("hidden").(bool); ok && hidden {
				in.problem(agents.paths[agent], "selected default agent is hidden")
			}
			if mode, ok := a.get("mode").(string); ok && mode == "subagent" {
				in.problem(agents.paths[agent], "selected default agent is subagent-only")
			}
			if disabled, ok := a.get("disable").(bool); ok && disabled {
				in.problem(agents.paths[agent], "selected default agent is disabled")
			}
		}
	}
	for _, dir := range in.dirs {
		for _, folder := range []string{"agent", "agents", "mode", "modes"} {
			path := filepath.Join(dir, folder, agent+".md")
			if _, err := os.Lstat(path); err == nil {
				in.problem(path, "selected agent markdown overrides are unverified; no prompt or plugin is executed")
			}
		}
	}
	selected := rules[0]
	for _, rule := range rules {
		if rule.action != "allow" && rule.action != "ask" && rule.action != "deny" {
			in.problem(rule.path, "unsupported permission action")
			continue
		}
		if opencodeWildcard(name, rule.pattern) {
			selected = rule
		}
	}
	state := PermissionAllowed
	if selected.action == "deny" {
		state = PermissionDenied
	} else if selected.action == "ask" {
		state = PermissionAsk
	}
	kind := EvidenceInferred
	reason := fmt.Sprintf("default agent %s: last matching ordered native rule is %s; running-session agent and approval state are unknown", agent, selected.action)
	if in.uncertain {
		state = PermissionUnknown
		kind = EvidenceUnknown
		reason = "local native inputs or dynamic overrides are unverified; no effective permission is asserted"
	}
	return PermissionExplanation{State: state, Evidence: opencodeEvidence(kind, "ordered-skill-permission", selected.path, selected.setting, reason), Layers: cloneLayers(in.scope.ConfigLayers)}
}

// Native schema normalizes permission strings and legacy agent tools before deep merge.
func normalizeOpencodeConfig(obj *opencodeObject) {
	normalizePermission := func(o *opencodeObject) {
		if action, ok := o.get("permission").(string); ok {
			p := newOpencodeObject()
			p.put("*", action, o.paths["permission"])
			o.put("permission", p, o.paths["permission"])
		}
	}
	normalizePermission(obj)
	if agents, ok := obj.get("agent").(*opencodeObject); ok {
		for _, name := range agents.keys {
			a, ok := agents.get(name).(*opencodeObject)
			if !ok {
				continue
			}
			normalizePermission(a)
			p := newOpencodeObject()
			if tools, ok := a.get("tools").(*opencodeObject); ok {
				for _, tool := range tools.keys {
					enabled, ok := tools.get(tool).(bool)
					if !ok {
						continue
					}
					action := "deny"
					if enabled {
						action = "allow"
					}
					p.put(tool, action, tools.paths[tool])
				}
			}
			if permission, ok := a.get("permission").(*opencodeObject); ok {
				for _, tool := range permission.keys {
					p.put(tool, permission.get(tool), permission.paths[tool])
				}
			}
			a.put("permission", p, a.paths["permission"])
		}
	}
}

func expandOpencodePattern(pattern, home string) string {
	if pattern == "~" {
		return home
	}
	if strings.HasPrefix(pattern, "~/") {
		return home + pattern[1:]
	}
	if strings.HasPrefix(pattern, "$HOME") {
		return home + pattern[5:]
	}
	return pattern
}
func (in *opencodeInputs) validate(obj *opencodeObject, path string) {
	var permission func(any)
	permission = func(value any) {
		if value == nil {
			return
		}
		if action, ok := value.(string); ok {
			if action != "allow" && action != "ask" && action != "deny" {
				in.problem(path, "permission action is invalid")
			}
			return
		}
		if p, ok := value.(*opencodeObject); ok {
			for _, key := range p.keys {
				if nested, ok := p.get(key).(*opencodeObject); ok {
					for _, pattern := range nested.keys {
						if _, ok := nested.get(pattern).(string); !ok {
							in.problem(path, "permission pattern action is invalid")
						}
						permission(nested.get(pattern))
					}
				} else {
					permission(p.get(key))
				}
			}
			return
		}
		in.problem(path, "permission action/object is invalid")
	}
	permission(obj.get("permission"))
	if tools := obj.get("tools"); tools != nil {
		if o, ok := tools.(*opencodeObject); ok {
			for _, key := range o.keys {
				if _, ok := o.get(key).(bool); !ok {
					in.problem(path, "tools values must be boolean")
				}
			}
		} else {
			in.problem(path, "tools is not an object")
		}
	}
	if skills := obj.get("skills"); skills != nil {
		if o, ok := skills.(*opencodeObject); ok {
			for _, key := range []string{"paths", "urls"} {
				if values := o.get(key); values != nil {
					list, ok := values.([]any)
					if !ok {
						in.problem(path, "skills paths/urls is not an array")
						continue
					}
					for _, item := range list {
						if _, ok := item.(string); !ok {
							in.problem(path, "skills paths/urls contains a nonstring")
						}
					}
				}
			}
		} else {
			in.problem(path, "skills is not an object")
		}
	}
	if agents := obj.get("agent"); agents != nil {
		if a, ok := agents.(*opencodeObject); ok {
			for _, key := range a.keys {
				if selected, ok := a.get(key).(*opencodeObject); ok {
					in.validate(selected, path)
				} else {
					in.problem(path, "agent configuration is not an object")
				}
			}
		} else {
			in.problem(path, "agent is not an object")
		}
	}
}
