package collection

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// piDiscovery records native decisions separately from the retained inspection
// inventory. Ignored content remains inspectable, but cannot contribute native
// occurrences or hide an incomplete participating search.
type piDiscovery struct {
	decisions map[string]piIgnoreDecision
	reads     map[string]bool
	root      string
}
type piIgnoreDecision struct{ excluded, unknown bool }
type piIgnoreMatcher struct {
	patterns        []gitignore.Pattern
	unknownPrefixes []string
	bytes           int
}

func newPiDiscovery() *piDiscovery {
	return &piDiscovery{decisions: map[string]piIgnoreDecision{}, reads: map[string]bool{}}
}
func (d *piDiscovery) record(file string, excluded bool, m *piIgnoreMatcher) {
	d.decisions[file] = piIgnoreDecision{excluded: excluded, unknown: m.uncertain(piIgnoreRelative(d, file))}
}
func (m *piIgnoreMatcher) addDirectory(d *piDiscovery, root, dir string) {
	d.root = root
	d.reads[dir] = true
	relative, _ := filepath.Rel(root, dir)
	prefix := ""
	if relative != "." {
		prefix = filepath.ToSlash(relative) + "/"
	}
	for _, name := range []string{".gitignore", ".ignore", ".fdignore"} {
		path := filepath.Join(dir, name)
		d.reads[path] = true
		raw, err := nativeBytes(path)
		if nativeAbsent(path, err) {
			continue
		}
		if err != nil || !utf8.Valid(raw) {
			m.unknownPrefixes = append(m.unknownPrefixes, strings.ToLower(prefix))
			continue
		}
		m.bytes += len(raw)
		if m.bytes > 1<<20 {
			m.unknownPrefixes = append(m.unknownPrefixes, strings.ToLower(prefix))
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSuffix(line, "\r")
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			// These are the exact native prefix transformations, rather than generic
			// per-directory Git matching. Unqualified forms retain honest uncertainty.
			if strings.TrimRight(line, " \t") != line || strings.ContainsRune(line, '\\') {
				m.unknownPrefixes = append(m.unknownPrefixes, strings.ToLower(prefix))
				continue
			}
			negated := strings.HasPrefix(line, "!")
			if negated {
				line = line[1:]
			}
			line = strings.TrimPrefix(line, "/")
			pattern := prefix + line
			if negated {
				pattern = "!" + pattern
			}
			if len(m.patterns) >= maxResourceEntries || !piIgnorePatternKnown(pattern) {
				m.unknownPrefixes = append(m.unknownPrefixes, strings.ToLower(prefix))
				continue
			}
			m.patterns = append(m.patterns, newPiIgnorePattern(strings.ToLower(pattern)))
		}
	}
}

// Native rules match the terminal occurrence; exclusion inherited from a
// parent is evaluated separately by ignored. Go Git also matches ancestor
// components, so qualify each rule at its exact terminal path before delegating.
// This preserves a child's own exclusion after its parent is re-included without
// inventing a child exclusion for a rule that matched only the restored parent.
type piIgnoreTerminal struct {
	gitPattern              gitignore.Pattern
	pattern                 string
	directoryOnly, anchored bool
}

func newPiIgnorePattern(pattern string) gitignore.Pattern {
	gitPattern := gitignore.ParsePattern(pattern, nil)
	pattern = strings.TrimPrefix(pattern, "!")
	directoryOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	anchored := strings.Contains(pattern, "/")
	return piIgnoreTerminal{gitPattern: gitPattern, pattern: strings.TrimPrefix(pattern, "/"), directoryOnly: directoryOnly, anchored: anchored}
}
func (p piIgnoreTerminal) Match(parts []string, dir bool) gitignore.MatchResult {
	if len(parts) == 0 || p.pattern == "" || p.directoryOnly && !dir {
		return gitignore.NoMatch
	}
	candidate := parts[len(parts)-1]
	if p.anchored {
		candidate = strings.Join(parts, "/")
	}
	match, err := filepath.Match(p.pattern, candidate)
	if err == nil && match {
		return p.gitPattern.Match(parts, dir)
	}
	return gitignore.NoMatch
}

func piIgnorePatternKnown(pattern string) bool {
	if !piIgnoreASCII(pattern) || strings.Contains(pattern, "//") || strings.ContainsAny(pattern, `\?`) {
		return false
	}
	// The native matcher operates on JavaScript code units. Restrict classes to
	// validated positive ASCII sets; negated/ranged/escaped classes remain unknown.
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '[' {
			continue
		}
		end := strings.IndexByte(pattern[i+1:], ']')
		if end < 0 {
			return false
		}
		body := pattern[i+1 : i+1+end]
		if body == "" {
			return false
		}
		for _, c := range body {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
				return false
			}
		}
		i += end + 1
	}
	for _, part := range strings.Split(pattern, "/") {
		if strings.Contains(part, "**") {
			return false
		}
	}
	return true
}
func piIgnoreRelative(d *piDiscovery, file string) string {
	relative, _ := filepath.Rel(d.root, file)
	return filepath.ToSlash(relative)
}

func (m *piIgnoreMatcher) uncertain(relative string) bool {
	if len(m.patterns) > 0 && !piIgnoreASCII(relative) {
		return true
	}
	relative = strings.ToLower(filepath.ToSlash(relative))
	for _, prefix := range m.unknownPrefixes {
		if prefix == "" || strings.HasPrefix(relative, prefix) {
			return true
		}
	}
	return false
}

func piIgnoreASCII(value string) bool {
	for _, c := range value {
		if c > 127 {
			return false
		}
	}
	return true
}

func (m *piIgnoreMatcher) ignored(relative string, dir bool) bool {
	if m.uncertain(relative) {
		return false
	}
	parts := strings.Split(strings.ToLower(filepath.ToSlash(relative)), "/")
	matcher := gitignore.NewMatcher(m.patterns)
	// A negated child cannot restore traversal through an excluded parent.
	for i := 1; i <= len(parts); i++ {
		if matcher.Match(parts[:i], i < len(parts) || dir) {
			return true
		}
	}
	return false
}
