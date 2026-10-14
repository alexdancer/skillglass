package collection

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	packagePatterns []string
	packageFilter   bool
	delta           bool
	provenance      Provenance
	trust           string
	uncertain       bool
}
type piInputs struct {
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
		if json.Unmarshal(value, &s.skills) != nil || string(value) == "null" {
			// Native migration accepts the legacy customDirectories shape.
			var legacy struct {
				CustomDirectories []string `json:"customDirectories"`
				Enable            *bool    `json:"enableSkillCommands"`
			}
			if json.Unmarshal(value, &legacy) == nil && len(value) > 0 && value[0] == '{' {
				s.skills = legacy.CustomDirectories
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
					var shape map[string]json.RawMessage
					if json.Unmarshal(raw, &p) != nil || json.Unmarshal(raw, &shape) != nil {
						s.uncertain = true
						continue
					}
					p.object = true
					_, p.hasSkills = shape["skills"]
					if p.hasSkills && string(shape["skills"]) == "null" {
						s.uncertain = true
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
	if !piRequiresTrust(project.Path) {
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
	if strings.ContainsAny(pattern, "{}()") {
		return false, false
	}
	var expr strings.Builder
	expr.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					expr.WriteString("(?:.*/)?")
				} else {
					expr.WriteString(".*")
				}
			} else {
				expr.WriteString("[^/]*")
			}
		case '?':
			expr.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				return false, false
			}
			part := pattern[i : i+end+2]
			if strings.HasPrefix(part, "[!") {
				part = "[^" + part[2:]
			}
			expr.WriteString(part)
			i += end + 1
		default:
			expr.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	expr.WriteString("$")
	re, err := regexp.Compile(expr.String())
	if err != nil {
		return false, false
	}
	for _, value := range values {
		if re.MatchString(value) {
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
