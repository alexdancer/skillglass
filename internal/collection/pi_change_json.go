package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

// Only the skills array changes. Unrelated JSON bytes and untouched element
// spellings survive; duplicate keys and legacy shapes are refused by writers.
func patchPiChoice(raw []byte, p piControlPlan) ([]byte, []SettingChange, bool, error) {
	original := raw
	if len(raw) == 0 {
		if p.Choice == ControlInherit {
			return raw, nil, false, nil
		}
		raw = []byte("{}\n")
	}
	root, err := parseSettingsJSON(raw)
	if err != nil {
		return nil, nil, false, err
	}
	var member *jsonMember
	for i := range root.members {
		if root.members[i].key == "skills" {
			member = &root.members[i]
		}
	}
	var entries []json.RawMessage
	var values []string
	if member != nil {
		value := raw[member.value:member.end]
		var valid bool
		values, valid = piResourceArray(value)
		if !valid {
			return nil, nil, false, errors.New("Pi writer requires a literal skills array; legacy/malformed settings are unsupported")
		}
		_ = json.Unmarshal(value, &entries)
	}
	matches := func(value string) bool {
		for _, target := range p.Targets {
			yes, known := piMatch(target, value, filepath.Dir(p.SettingsPath), true)
			if yes && known {
				return true
			}
		}
		return false
	}
	exact := map[string]bool{}
	for _, value := range values {
		if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
			if matches(value[1:]) {
				exact[strings.TrimLeft(value, "+-")] = true
			}
		}
	}
	global := map[string]bool{}
	for _, target := range p.GlobalTargets {
		global[target] = true
	}
	// The reversible native override is an absolute inherited instruction file
	// plus an exact choice. An existing lone registration is user discovery state.
	for _, value := range values {
		if p.Scope == ControlProject && global[value] && !exact[value] && p.Choice != ControlInherit {
			return nil, nil, false, errors.New("pre-existing project file registration has ambiguous override ownership; preserving configured resources")
		}
	}
	if p.Scope == ControlProject {
		for target := range global {
			registrations, choices := 0, 0
			for _, value := range values {
				if value == target {
					registrations++
				}
				if value == "+"+target || value == "-"+target {
					choices++
				}
			}
			if registrations > 1 || registrations > 0 && choices > 1 {
				return nil, nil, false, errors.New("ambiguous repeated project override representation; existing registrations are preserved")
			}
		}
	}
	removed := false
	var out []json.RawMessage
	present := map[string]bool{}
	for i, value := range values {
		if (strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-")) && matches(value[1:]) {
			removed = true
			continue
		}
		if p.Choice == ControlInherit && global[value] && exact[value] {
			removed = true
			continue
		}
		out = append(out, entries[i])
		present[value] = true
	}
	add := func(value string) {
		if !present[value] {
			encoded, _ := json.Marshal(value)
			out = append(out, encoded)
			present[value] = true
		}
	}
	if p.Choice != ControlInherit {
		if p.Scope == ControlProject {
			for _, target := range p.GlobalTargets {
				if piPattern(target) {
					return nil, nil, false, errors.New("project override registration path contains native resource operators; exact registration is unverified")
				}
				add(target)
			}
		}
		prefix := "+"
		if p.Choice == ControlDisable {
			prefix = "-"
		}
		for _, target := range p.Targets {
			add(prefix + target)
		}
	}
	if p.Choice == ControlInherit && !removed {
		return original, nil, false, nil
	}
	if out == nil {
		out = []json.RawMessage{}
	}
	value := []byte("[")
	for index, entry := range out {
		if index > 0 {
			value = append(value, ',')
		}
		value = append(value, entry...)
	}
	value = append(value, ']')
	var post []byte
	if member != nil {
		post = append(post, raw[:member.value]...)
		post = append(post, value...)
		post = append(post, raw[member.end:]...)
	} else {
		if p.Choice == ControlInherit {
			return original, nil, false, nil
		}
		post = append(post, raw[:root.end-1]...)
		if len(root.members) > 0 {
			post = append(post, ',')
		}
		post = append(post, []byte(`"skills":`)...)
		post = append(post, value...)
		post = append(post, raw[root.end-1:]...)
	}
	if !json.Valid(post) {
		return nil, nil, false, errors.New("invalid Pi resource patch")
	}
	before := "absent"
	if member != nil {
		before = string(raw[member.value:member.end])
	}
	return post, []SettingChange{{Setting: "skills", Before: before, After: string(value)}}, !bytes.Equal(original, post), nil
}
