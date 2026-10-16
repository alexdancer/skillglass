package collection

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const piValidatedVersion = "pi 1.1.0"

func piEvidence(kind EvidenceKind, rule, path, setting, reason string) Evidence {
	return Evidence{Kind: kind, Rule: rule, InputPath: path, Setting: setting, Reason: reason, AgentVersion: piValidatedVersion}
}

type piSettings struct {
	skills       []string
	packages     []piPackage
	commands     *bool
	defaultTrust string
	uncertain    bool
	path         string
}
type piPackage struct {
	Source    string   `json:"source"`
	Skills    []string `json:"skills"`
	Autoload  *bool    `json:"autoload"`
	hasSkills bool
	object    bool
}
type piRootFact struct {
	rank            int
	base, path      string
	patterns        []string
	resources       []string // Original literal registrations contributing this native root.
	packagePatterns []string
	packageFilter   bool
	delta           bool
	provenance      Provenance
	trust           string
	uncertain       bool
}
type piPackageProblem struct {
	problem          Problem
	source, settings string
}
type piInputs struct {
	overlay         map[string][]byte
	packageProblems []piPackageProblem
	scope           AgentScope
	problems        []Problem
	global, project piSettings
	trust           string
	facts           map[RootID]piRootFact
}

func (inputs *piInputs) problem(path, reason string) {
	inputs.problems = append(inputs.problems, makeProblem(ProblemNativeInput, path, reason, nil, true))
}
func (inputs *piInputs) settings(path, scope string) piSettings {
	s := piSettings{path: path}
	layer := PermissionLayer{Path: path, Scope: scope, State: "absent", Reason: "optional settings file is absent", Evidence: piEvidence(EvidenceObserved, "settings-layer", path, "skills, packages", "read-only settings observation")}
	defer func() { inputs.scope.ConfigLayers = append(inputs.scope.ConfigLayers, layer) }()
	raw, err := nativeBytes(path)
	if replacement, ok := inputs.overlay[path]; ok {
		raw, err = replacement, nil
		if len(raw) == 0 {
			return s
		}
	}
	if nativeAbsent(path, err) {
		return s
	}
	var data map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &data) != nil || data == nil {
		s.uncertain = true
		layer.State = "invalid"
		layer.Reason = "settings must be a bounded JSON object"
		inputs.problem(path, layer.Reason)
		return s
	}
	layer.State = "read"
	layer.Reason = "Pi resource settings observed"
	if value, ok := data["skills"]; ok {
		var valid bool
		s.skills, valid = piResourceArray(value)
		if !valid {
			// Native migration accepts the legacy customDirectories shape.
			var legacy struct {
				CustomDirectories json.RawMessage `json:"customDirectories"`
				Enable            *bool           `json:"enableSkillCommands"`
			}
			if json.Unmarshal(value, &legacy) == nil && len(value) > 0 && value[0] == '{' {
				if len(legacy.CustomDirectories) > 0 && string(legacy.CustomDirectories) != "null" {
					s.skills, valid = piResourceArray(legacy.CustomDirectories)
					if !valid {
						s.uncertain = true
					}
				}
				s.commands = legacy.Enable
			} else {
				s.uncertain = true
			}
		}
	}
	if value, ok := data["packages"]; ok {
		var packages []json.RawMessage
		if json.Unmarshal(value, &packages) != nil || string(value) == "null" {
			s.uncertain = true
		} else {
			for _, raw := range packages {
				var p piPackage
				if json.Unmarshal(raw, &p.Source) != nil {
					var registration struct {
						Source   string `json:"source"`
						Autoload *bool  `json:"autoload"`
					}
					var shape map[string]json.RawMessage
					if json.Unmarshal(raw, &registration) != nil || json.Unmarshal(raw, &shape) != nil {
						s.uncertain = true
						continue
					}
					p.Source, p.Autoload = registration.Source, registration.Autoload
					p.object = true
					_, p.hasSkills = shape["skills"]
					if p.hasSkills {
						var valid bool
						p.Skills, valid = piResourceArray(shape["skills"])
						if !valid {
							s.uncertain = true
						}
					}
				}
				if p.Source == "" {
					s.uncertain = true
					continue
				}
				s.packages = append(s.packages, p)
			}
		}
	}
	if value, ok := data["enableSkillCommands"]; ok {
		if json.Unmarshal(value, &s.commands) != nil || string(value) == "null" {
			s.uncertain = true
		}
	}
	if value, ok := data["defaultProjectTrust"]; ok {
		if json.Unmarshal(value, &s.defaultTrust) != nil || (s.defaultTrust != "always" && s.defaultTrust != "never" && s.defaultTrust != "ask") {
			s.uncertain = true
		}
	}
	if s.uncertain {
		layer.State = "invalid"
		layer.Reason = "Pi resource fields contain an unsupported or invalid shape"
		inputs.problem(path, layer.Reason)
	}
	return s
}
func piResourceArray(raw json.RawMessage) ([]string, bool) {
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || entries == nil {
		return nil, false
	}
	values := make([]string, 0, len(entries))
	valid := true
	for _, entry := range entries {
		var value *string
		if json.Unmarshal(entry, &value) != nil || value == nil {
			valid = false
			continue
		}
		values = append(values, *value)
	}
	return values, valid
}
func piAgentDir(home string) (string, error) {
	if value := os.Getenv("PI_CODING_AGENT_DIR"); value != "" {
		return filepath.Abs(piPath(value, ".", home))
	}
	return filepath.Join(home, ".pi", "agent"), nil
}
func piPath(value, base, home string) string {
	value = strings.TrimSpace(value)
	if value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, value[2:])
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Clean(value)
}
func piPattern(value string) bool {
	return strings.HasPrefix(value, "!") || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "*?")
}
func piAncestors(project ProjectContext) ([]string, error) {
	if !project.Selected {
		return nil, nil
	}
	path, err := filepath.EvalSymlinks(project.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil, errors.New("selected project is not a directory")
	}
	var result []string
	for {
		result = append(result, path)
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return result, nil
}
func (inputs *piInputs) projectTrust(project ProjectContext, agentDir string) string {
	if !project.Selected {
		return "allowed"
	}
	_, prospectiveSettings := inputs.overlay[filepath.Join(project.Path, ".pi", "settings.json")]
	if !piRequiresTrust(project.Path) && !prospectiveSettings {
		return "allowed"
	}
	trustPath := filepath.Join(agentDir, "trust.json")
	raw, err := nativeBytes(trustPath)
	data := map[string]*bool{}
	if !nativeAbsent(trustPath, err) {
		if err != nil || json.Unmarshal(raw, &data) != nil || data == nil {
			inputs.problem(trustPath, "Pi trust store is unreadable or invalid")
			return "unknown"
		}
	}
	path, err := filepath.EvalSymlinks(project.Path)
	if err != nil {
		return "unknown"
	}
	for {
		if decision := data[path]; decision != nil {
			if *decision {
				return "allowed"
			}
			return "denied"
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	switch inputs.global.defaultTrust {
	case "always":
		return "allowed"
	case "never":
		return "denied"
	default:
		return "ask"
	}
}

// Native minimatch checks the file path, basename, and SKILL.md parent path.
// Braces and extglobs are left unknown until their native semantics are validated.
func piMatch(path, pattern, base string, exact bool) (bool, bool) {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	rel, _ := filepath.Rel(base, path)
	values := []string{filepath.ToSlash(path), filepath.ToSlash(rel)}
	if !exact {
		values = append(values, filepath.Base(path))
	}
	if filepath.Base(path) == "SKILL.md" {
		parent := filepath.Dir(path)
		rel, _ = filepath.Rel(base, parent)
		values = append(values, filepath.ToSlash(parent), filepath.ToSlash(rel))
		if !exact {
			values = append(values, filepath.Base(parent))
		}
	}
	if exact {
		for _, value := range values {
			if value == pattern {
				return true, true
			}
		}
		return false, true
	}
	if strings.ContainsAny(pattern, "?[") {
		for _, candidate := range append(values, pattern) {
			for _, literal := range candidate {
				if literal > 0xffff {
					return false, false
				}
			}
		}
	}
	if strings.ContainsAny(pattern, "{}()\\") || strings.Contains(pattern, "//") {
		return false, false
	}
	segments := strings.Split(pattern, "/")
	expressions := make([]*regexp.Regexp, len(segments))
	for index, segment := range segments {
		if segment == "**" {
			continue
		}
		var expr strings.Builder
		expr.WriteString("^")
		for i := 0; i < len(segment); i++ {
			switch segment[i] {
			case '*':
				expr.WriteString("[^/]*")
			case '?':
				expr.WriteString("[^/]")
			case '[':
				end := strings.IndexByte(segment[i+1:], ']')
				if end < 0 {
					return false, false
				}
				part := segment[i : i+end+2]
				if strings.HasPrefix(part, "[!") {
					part = "[^" + part[2:]
				}
				expr.WriteString(part)
				i += end + 1
			default:
				literal, width := utf8.DecodeRuneInString(segment[i:])
				expr.WriteString(regexp.QuoteMeta(string(literal)))
				i += width - 1
			}
		}
		expr.WriteString("$")
		var err error
		expressions[index], err = regexp.Compile(expr.String())
		if err != nil {
			return false, false
		}
	}
	for _, value := range values {
		components := strings.Split(value, "/")
		memo := make(map[[2]int]bool)
		var match func(int, int) bool
		match = func(p, v int) bool {
			key := [2]int{p, v}
			if result, seen := memo[key]; seen {
				return result
			}
			result := false
			if p == len(segments) {
				result = v == len(components)
			} else if segments[p] == "**" {
				result = match(p+1, v) || v < len(components) && !strings.HasPrefix(components[v], ".") && match(p, v+1)
			} else if v < len(components) {
				component, segment := components[v], segments[p]
				magic := strings.ContainsAny(segment, "*?[")
				eligible := !strings.HasPrefix(component, ".") || strings.HasPrefix(segment, ".")
				if magic && (component == "." || component == "..") {
					eligible = false
				}
				result = eligible && expressions[p].MatchString(component) && match(p+1, v+1)
			}
			memo[key] = result
			return result
		}
		if match(0, 0) {
			return true, true
		}
	}
	return false, true
}
func piEnabled(path string, patterns []string, base string, includePatterns bool, delta bool) (bool, bool) {
	enabled := true
	known := true
	if delta {
		enabled = false
		for _, p := range patterns {
			target := strings.TrimLeft(p, "!+-")
			match, ok := piMatch(path, target, base, strings.HasPrefix(p, "+") || strings.HasPrefix(p, "-"))
			known = known && ok
			if match {
				enabled = !strings.HasPrefix(p, "!") && !strings.HasPrefix(p, "-")
			}
		}
		return enabled, known
	}
	includes := false
	included := false
	for _, p := range patterns {
		if !strings.HasPrefix(p, "!") && !strings.HasPrefix(p, "+") && !strings.HasPrefix(p, "-") && includePatterns {
			includes = true
			match, ok := piMatch(path, p, base, false)
			known = known && ok
			included = included || match
		}
	}
	if includes {
		enabled = included
	}
	for _, prefix := range []string{"!", "+", "-"} {
		for _, p := range patterns {
			if !strings.HasPrefix(p, prefix) {
				continue
			}
			match, ok := piMatch(path, p[1:], base, prefix != "!")
			known = known && ok
			if match {
				enabled = prefix == "+"
			}
		}
	}
	return enabled, known
}

func piFilterPatterns(entries []string) []string {
	var patterns []string
	for _, entry := range entries {
		if piPattern(entry) {
			patterns = append(patterns, entry)
		}
	}
	return patterns
}

// Native trust checks ancestors beyond the discovery Git boundary, but skips
// HOME/.agents/skills because that is a trusted global resource.
func piRequiresTrust(project string) bool {
	path, err := filepath.EvalSymlinks(project)
	if err != nil {
		return true
	}
	for _, entry := range []string{"settings.json", "mcp.json", "extensions", "skills", "prompts", "themes", "SYSTEM.md", "APPEND_SYSTEM.md"} {
		if _, err := os.Stat(filepath.Join(path, ".pi", entry)); err == nil {
			return true
		}
	}
	home, _ := nativeHome()
	if canonical, err := filepath.EvalSymlinks(home); err == nil {
		home = canonical
	}
	user := filepath.Join(home, ".agents", "skills")
	for {
		skills := filepath.Join(path, ".agents", "skills")
		if skills != user {
			if _, err := os.Stat(skills); err == nil {
				return true
			}
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}
