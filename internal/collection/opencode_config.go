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
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const opencodeValidatedVersion = "OpenCode 1.17.15"

var errOpencodeSurrogate = errors.New("native JSON contains an unpaired UTF-16 surrogate; lossless literal matching is unsupported")

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
func opencodeArrayIndex(key string) (uint64, bool) {
	index, err := strconv.ParseUint(key, 10, 32)
	return index, err == nil && index < 4294967295 && strconv.FormatUint(index, 10) == key
}
func (o *opencodeObject) orderedKeys() []string {
	keys := append([]string(nil), o.keys...)
	sort.SliceStable(keys, func(i, j int) bool {
		a, aIndex := opencodeArrayIndex(keys[i])
		b, bIndex := opencodeArrayIndex(keys[j])
		return aIndex && (!bIndex || a < b)
	})
	return keys
}
func mergeOpencode(target, source *opencodeObject) {
	for _, key := range source.orderedKeys() {
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
func validateOpencodeUnicode(raw []byte) error {
	quoted := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) || raw[i] != 'u' || i+4 >= len(raw) {
			continue
		}
		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		i += 4
		if err != nil || unit < 0xd800 || unit > 0xdfff {
			continue
		}
		if unit >= 0xdc00 || i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return errOpencodeSurrogate
		}
		low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return errOpencodeSurrogate
		}
		i += 6
	}
	return nil
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
	if err := validateOpencodeUnicode(cleaned); err != nil {
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
		if err == nil && len(raw) == 0 {
			raw = []byte("{}")
		}
	}
	obj, parseErr := parseOpencode(raw, path)
	if err != nil || parseErr != nil {
		layer.State = "unknown"
		layer.Reason = "config cannot be interpreted safely; content is not reported"
		if errors.Is(parseErr, errOpencodeSurrogate) {
			layer.Reason = errOpencodeSurrogate.Error() + "; content is not reported"
		}
		in.problem(path, layer.Reason)
	} else {
		layer.State = "loaded"
		layer.Reason = "deep merge in native order; conflicting keys replace values without reordering existing keys"
		before := len(in.problems)
		in.validate(obj, path, false)
		if len(in.problems) > before {
			layer.State = "unknown"
			layer.Reason = "loaded config contains unsupported or malformed interpreted fields"
		}
		normalizeOpencodeConfig(obj)
		mergeOpencode(in.config, obj)
		if scope != "global" {
			in.registerPlugins(obj)
		}
	}
	in.scope.ConfigLayers = append(in.scope.ConfigLayers, layer)
}

// The pinned global loader recognizes this legacy path. Presence is observed;
// its native import/merge/migration behavior is source inference only.
func (in *opencodeInputs) detectGlobalLegacy(path string) {
	_, err := os.Stat(path)
	layer := PermissionLayer{Path: path, Scope: "global", State: "absent", Reason: "optional global legacy config does not exist", Evidence: opencodeEvidence(EvidenceObserved, "config-layer", path, "", "read-only metadata inspection; legacy content is never read")}
	if !nativeAbsent(path, err) {
		layer.State = "unknown"
		layer.Reason = "global legacy config is present; pinned loader may import it, but native legacy loading and migration are unverified"
		if err != nil {
			layer.Evidence.Kind = EvidenceUnknown
			layer.Reason = "global legacy config cannot be inspected safely; pinned loader recognizes this path, but native legacy loading and migration are unverified"
		}
		in.problem(path, layer.Reason)
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
	in.registerPlugins(in.config)
	in.detectGlobalLegacy(filepath.Join(global, "config"))
	if raw := os.Getenv("OPENCODE_CONFIG"); raw != "" {
		in.load(opencodePath(raw, project, home), "custom-config", nil, true)
	}
	if !opencodeBool("OPENCODE_DISABLE_PROJECT_CONFIG") {
		for i := len(ancestors) - 1; i >= 0; i-- { // ConfigPaths.files reverses nearest-first jsonc/json into root-first json/jsonc.
			in.loadPair(ancestors[i], "project")
		}
	}
	dirs := []string{global}
	if !opencodeBool("OPENCODE_DISABLE_PROJECT_CONFIG") {
		for _, ancestor := range ancestors {
			dirs = append(dirs, filepath.Join(ancestor, ".opencode"))
		}
	}
	dirs = append(dirs, filepath.Join(home, ".opencode"))
	configuredDir := os.Getenv("OPENCODE_CONFIG_DIR")
	if configuredDir != "" {
		dirs = append(dirs, configuredDir)
		dir := opencodePath(configuredDir, project, home)
		resolved, err := filepath.EvalSymlinks(dir)
		if !filepath.IsAbs(configuredDir) || (configuredDir != dir && configuredDir != dir+"/" && configuredDir != dir+"/.") || (err == nil && resolved != dir) || (err != nil && !nativeAbsent(dir, err)) {
			in.problem("OPENCODE_CONFIG_DIR", "configured directory spelling or alias resolution is unverified; normalized paths are inspection-only evidence")
		}
	}
	seen, physical := map[string]bool{}, map[string]bool{}
	for _, raw := range dirs {
		if seen[raw] {
			continue
		}
		seen[raw] = true
		dir := opencodePath(raw, project, home)
		if strings.HasSuffix(raw, ".opencode") || raw == configuredDir {
			in.loadPair(dir, "native-directory")
		}
		if physical[dir] {
			continue
		}
		physical[dir] = true
		in.dirs = append(in.dirs, dir)
		in.detectDynamic(dir)
		in.detectMarkdown(dir)
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
	if skills, ok := in.config.get("skills").(*opencodeObject); ok {
		if urls, ok := skills.get("urls").([]any); ok && len(urls) > 0 {
			in.problem("skills.urls", "remote skill indexes are not fetched; cache presence cannot prove current activation or precedence")
		}
	}

	normalizeOpencodeContextTools(in.config)
	return in
}
func (in *opencodeInputs) registerPlugins(obj *opencodeObject) {
	if items, ok := obj.get("plugin").([]any); ok && len(items) > 0 && !opencodeBool("OPENCODE_PURE") {
		in.problem(obj.paths["plugin"], "registered plugin inputs may retain configuration hooks after later list replacement; hooks are never executed")
	}
}
func (in *opencodeInputs) detectMarkdown(dir string) {
	count := 0
	for _, folder := range []string{"agent", "agents", "mode", "modes"} {
		path := filepath.Join(dir, folder)
		if _, err := os.Stat(path); nativeAbsent(path, err) {
			continue
		}
		in.inspectMarkdown(path, folder == "agent" || folder == "agents", map[string]bool{}, &count, 0)
	}
}
func (in *opencodeInputs) inspectMarkdown(path string, recursive bool, ancestors map[string]bool, count *int, depth int) {
	if *count >= maxResourceEntries || depth > 64 {
		in.problem(path, "native markdown metadata traversal exceeds read limit; targeting and validation are unverified")
		return
	}
	*count++
	info, err := os.Stat(path)
	if err != nil {
		in.problem(path, fmt.Sprintf("native markdown input cannot be inspected; targeting and validation are unverified: %v", err))
		return
	}
	if !info.IsDir() {
		in.problem(path, "native markdown input is not a directory; native acceptance is unverified")
		return
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || ancestors[resolved] {
		in.problem(path, "native markdown metadata traversal cannot resolve directory aliases or cycles; coverage is unverified")
		return
	}
	ancestors[resolved] = true
	defer delete(ancestors, resolved)
	entries, err := os.ReadDir(path)
	if err != nil {
		in.problem(path, fmt.Sprintf("native markdown directory cannot be inspected; targeting and validation are unverified: %v", err))
		return
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		if *count >= maxResourceEntries {
			in.problem(child, "native markdown metadata traversal exceeds read limit; targeting and validation are unverified")
			return
		}
		*count++
		markdown := strings.HasSuffix(entry.Name(), ".md")
		if !markdown && (!recursive || (!entry.IsDir() && entry.Type()&os.ModeSymlink == 0)) {
			continue
		}
		childInfo, err := os.Stat(child)
		if err != nil {
			in.problem(child, fmt.Sprintf("native markdown entry cannot be inspected; targeting and validation are unverified: %v", err))
			continue
		}
		if childInfo.IsDir() {
			if recursive {
				in.inspectMarkdown(child, true, ancestors, count, depth+1)
			}
		} else if markdown {
			in.problem(child, "recognized native markdown input is present; effective targeting and native acceptance are unverified; no content is executed")
		}
	}
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
				in.problem(filepath.Join(dir, name), "local plugin hooks are not executed; effective native state is unknown")
				break
			}
		}
	}
}
func opencodeWildcard(value, pattern string) bool {
	value = strings.ReplaceAll(value, `\`, "/")
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	const unitOffset = 0x10000
	var input, expression strings.Builder
	for _, unit := range utf16.Encode([]rune(value)) {
		input.WriteRune(rune(unit) + unitOffset)
	}
	units := utf16.Encode([]rune(pattern))
	optionalTail := strings.HasSuffix(pattern, " *")
	if optionalTail {
		units = units[:len(units)-2]
	}
	expression.WriteString("(?s)^")
	for _, unit := range units {
		switch unit {
		case '*':
			expression.WriteString(".*")
		case '?':
			expression.WriteByte('.')
		default:
			expression.WriteRune(rune(unit) + unitOffset)
		}
	}
	if optionalTail {
		expression.WriteByte('(')
		expression.WriteRune(' ' + unitOffset)
		expression.WriteString(".*)?")
	}
	expression.WriteByte('$')
	ok, _ := regexp.MatchString(expression.String(), input.String())
	return ok
}

type opencodeRule struct{ tool, pattern, action, path, setting string }
type opencodeToolAction string

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
	for _, tool := range obj.orderedKeys() {
		if !opencodeWildcard("skill", tool) {
			continue
		}
		v := obj.get(tool)
		if action, ok := v.(opencodeToolAction); ok {
			rules = append(rules, opencodeRule{tool: tool, pattern: "*", action: string(action), path: obj.paths[tool], setting: strings.TrimSuffix(setting, "permission") + "tools." + tool})
			continue
		}
		if action, ok := v.(string); ok {
			rules = append(rules, opencodeRule{tool: tool, pattern: "*", action: action, path: obj.paths[tool], setting: setting + "." + tool})
			continue
		}
		patterns, ok := v.(*opencodeObject)
		if !ok {
			in.problem(obj.paths[tool], "skill permission has an unsupported value")
			continue
		}
		for _, pattern := range patterns.orderedKeys() {
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
	agent := "build"
	if raw := in.config.get("default_agent"); raw != nil {
		selected, ok := raw.(string)
		if !ok {
			in.problem(in.config.paths["default_agent"], "default agent is not a string")
		} else if selected != "" {
			agent = selected
		}
	}
	action, mode, hidden, builtin := "allow", "all", false, true
	switch agent {
	case "build", "plan":
		mode = "primary"
	case "general":
		mode = "subagent"
	case "explore":
		action, mode = "deny", "subagent"
	case "compaction", "title", "summary":
		action, mode, hidden = "deny", "primary", true
	default:
		builtin = false
	}
	setting := "native default"
	if action == "deny" {
		setting = "native agent " + agent + " default"
	}
	rules := []opencodeRule{{tool: "*", pattern: "*", action: action, setting: setting}}
	rules = append(rules, in.rules(in.config.get("permission"), "permission")...)
	agents, _ := in.config.get("agent").(*opencodeObject)
	a, _ := agents.get(agent).(*opencodeObject)
	hiddenPath, modePath := in.config.paths["default_agent"], in.config.paths["default_agent"]
	if !builtin && a == nil {
		in.problem(in.config.paths["default_agent"], "selected native agent is not known in local config")
	}
	if a != nil {
		rules = append(rules, in.rules(a.get("permission"), "agent."+agent+".permission")...)
		if value, ok := a.get("hidden").(bool); ok {
			hidden, hiddenPath = value, a.paths["hidden"]
		}
		if value, ok := a.get("mode").(string); ok {
			mode, modePath = value, a.paths["mode"]
		}
		if disabled, ok := a.get("disable").(bool); ok && disabled {
			in.problem(a.paths["disable"], "selected default agent is disabled; implicit native fallback is unverified")
		}
	}
	if hidden {
		in.problem(hiddenPath, "selected default agent is hidden; inherited native state includes built-in defaults")
	}
	if mode == "subagent" {
		in.problem(modePath, "selected default agent is subagent-only; inherited native state includes built-in defaults")
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

func normalizeOpencodeContextTools(obj *opencodeObject) {
	permission := newOpencodeObject()
	if tools, ok := obj.get("tools").(*opencodeObject); ok {
		for _, tool := range tools.orderedKeys() {
			enabled, ok := tools.get(tool).(bool)
			if !ok {
				continue
			}
			action := opencodeToolAction("deny")
			if enabled {
				action = "allow"
			}
			key := tool
			if key == "write" || key == "patch" {
				key = "edit"
			}
			permission.put(key, action, tools.paths[tool])
		}
	}
	if explicit, ok := obj.get("permission").(*opencodeObject); ok {
		mergeOpencode(permission, explicit)
	}
	obj.put("permission", permission, obj.paths["permission"])
}

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
		for _, name := range agents.orderedKeys() {
			a, ok := agents.get(name).(*opencodeObject)
			if !ok {
				continue
			}
			normalizePermission(a)
			p := newOpencodeObject()
			if tools, ok := a.get("tools").(*opencodeObject); ok {
				for _, tool := range tools.orderedKeys() {
					enabled, ok := tools.get(tool).(bool)
					if !ok {
						continue
					}
					action := opencodeToolAction("deny")
					if enabled {
						action = "allow"
					}
					p.put(tool, action, tools.paths[tool])
				}
			}
			if permission, ok := a.get("permission").(*opencodeObject); ok {
				for _, tool := range permission.orderedKeys() {
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
func (in *opencodeInputs) validate(obj *opencodeObject, path string, agentConfig bool) {
	validateAction := func(value any) {
		action, ok := value.(string)
		if !ok || (action != "allow" && action != "ask" && action != "deny") {
			in.problem(path, "permission action must be allow, ask or deny")
		}
	}
	permission := func(value any) {
		p, ok := value.(*opencodeObject)
		if !ok {
			validateAction(value)
			return
		}
		for _, key := range p.orderedKeys() {
			value := p.get(key)
			switch key {
			case "todowrite", "question", "webfetch", "websearch", "doom_loop":
				validateAction(value)
				continue
			}
			if patterns, ok := value.(*opencodeObject); ok {
				for _, pattern := range patterns.orderedKeys() {
					validateAction(patterns.get(pattern))
				}
			} else {
				validateAction(value)
			}
		}
	}
	if value, present := obj.values["permission"]; present {
		permission(value)
	}
	if agentConfig {
		for _, key := range []string{"hidden", "disable"} {
			if value, present := obj.values[key]; present {
				if _, ok := value.(bool); !ok {
					in.problem(path, "agent "+key+" must be boolean")
				}
			}
		}
		if value, present := obj.values["mode"]; present {
			mode, ok := value.(string)
			if !ok || (mode != "subagent" && mode != "primary" && mode != "all") {
				in.problem(path, "agent mode must be subagent, primary or all")
			}
		}
	} else if value, present := obj.values["plugin"]; present {
		if _, ok := value.([]any); !ok {
			in.problem(path, "plugin must be an array")
		}
	}
	if tools, present := obj.values["tools"]; present {
		if o, ok := tools.(*opencodeObject); ok {
			for _, key := range o.orderedKeys() {
				if _, ok := o.get(key).(bool); !ok {
					in.problem(path, "tools values must be boolean")
				}
			}
		} else {
			in.problem(path, "tools is not an object")
		}
	}
	if skills, present := obj.values["skills"]; present && !agentConfig {
		if o, ok := skills.(*opencodeObject); ok {
			for _, key := range []string{"paths", "urls"} {
				if values, present := o.values[key]; present {
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
	if !agentConfig {
		if value, present := obj.values["default_agent"]; present {
			if _, ok := value.(string); !ok {
				in.problem(path, "default agent must be a string")
			}
		}
		if _, present := obj.values["mode"]; present {
			in.problem(path, "legacy mode overrides are unverified")
		}
	}
	if agents, present := obj.values["agent"]; present && !agentConfig {
		if a, ok := agents.(*opencodeObject); ok {
			for _, key := range a.orderedKeys() {
				if selected, ok := a.get(key).(*opencodeObject); ok {
					in.validate(selected, path, true)
				} else {
					in.problem(path, "agent configuration is not an object")
				}
			}
		} else {
			in.problem(path, "agent is not an object")
		}
	}
}
