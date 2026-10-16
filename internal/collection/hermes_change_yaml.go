package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Patch a bounded block entry, retaining every unrelated byte. Refuse syntax
// whose aliases or serialization could change the meaning of another setting.
func patchHermesChoice(raw []byte, name string, choice ControlChoice) ([]byte, SettingChange, bool, error) {
	change := SettingChange{Setting: "skills.disabled (" + name + ")"}
	fail := func() ([]byte, SettingChange, bool, error) {
		return nil, change, false, errors.New("Hermes writer requires ordinary block mappings and a literal disabled list; aliases, merges, tags, multiline and ambiguous target syntax are unsupported")
	}
	original := raw
	if len(raw) == 0 {
		raw = []byte("{}\n")
	}
	var doc yaml.Node
	if yaml.Unmarshal(raw, &doc) != nil || len(doc.Content) != 1 {
		return fail()
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || root.Tag != "!!map" {
		return fail()
	}
	var data map[string]any
	if yaml.Unmarshal(raw, &data) != nil {
		return fail()
	}
	observation := observeHermesYAML(raw)
	if !observation.nativeConstructible() || !observation.known("skills.disabled") {
		return fail()
	}
	lines := bytes.SplitAfter(raw, []byte("\n"))
	blockMap := func(node *yaml.Node) bool {
		if node.Kind != yaml.MappingNode || node.Style != 0 || node.Anchor != "" || node.Tag != "!!map" {
			return false
		}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i]
			if k.Tag != "!!str" || k.Anchor != "" || k.Value == "<<" || k.Line < 1 || k.Line > len(lines) || k.Column < 1 {
				return false
			}
			line, col := string(lines[k.Line-1]), k.Column-1
			if col > len(line) || strings.TrimSpace(line[:col]) != "" || !strings.Contains(line[col:], ":") {
				return false
			}
		}
		return true
	}
	var skillKey, skills, disabledKey, disabled *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "skills" {
			skillKey, skills = root.Content[i], root.Content[i+1]
		}
	}
	if skills != nil {
		if !blockMap(root) || !blockMap(skills) {
			return fail()
		}
		for i := 0; i < len(skills.Content); i += 2 {
			if skills.Content[i].Value == "disabled" {
				disabledKey, disabled = skills.Content[i], skills.Content[i+1]
			}
		}
	}
	values := []string{}
	if disabled != nil {
		var nodes []*yaml.Node
		switch disabled.Kind {
		case yaml.ScalarNode:
			if disabled.Tag == "!!null" {
				nodes = nil
			} else {
				return fail()
			}
		case yaml.SequenceNode:
			nodes = disabled.Content
		default:
			return fail()
		}
		if disabled.Anchor != "" || disabled.Style&yaml.TaggedStyle != 0 {
			return fail()
		}
		for _, node := range nodes {
			if node.Kind != yaml.ScalarNode || node.Tag != "!!str" || node.Anchor != "" || node.Style&(yaml.TaggedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 || strings.ContainsAny(node.Value, "\r\n") {
				return fail()
			}
			values = append(values, node.Value)
		}
	}
	wanted := make([]string, 0, len(values)+1)
	present, exact := false, false
	for _, value := range values {
		if strings.TrimFunc(value, hermesDotenvSpace) == name {
			present = true
			exact = exact || value == name
			if choice == ControlDisable {
				wanted = append(wanted, value)
			}
		} else {
			wanted = append(wanted, value)
		}
	}
	if choice == ControlDisable {
		if !exact {
			wanted = append(wanted, name)
		}
	}
	selected := func(entries []string) string {
		matches := []string{}
		for _, value := range entries {
			if strings.TrimFunc(value, hermesDotenvSpace) == name {
				matches = append(matches, value)
			}
		}
		encoded, _ := json.Marshal(matches)
		return string(encoded)
	}
	change.Before, change.After = selected(values), selected(wanted)
	if (choice == ControlDisable && exact) || (choice == ControlEnable && !present) {
		return original, change, false, nil
	}
	encoded, _ := json.Marshal(wanted)
	newline := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		newline = "\r\n"
	}
	offset := func(line int) int {
		n := 0
		for i := 0; i < line-1 && i < len(lines); i++ {
			n += len(lines[i])
		}
		return n
	}
	ownLine := func(node *yaml.Node) bool {
		if node.Line < 1 || node.Line > len(lines) {
			return false
		}
		prefix := string(lines[node.Line-1])
		col := node.Column - 1
		return col >= 0 && col <= len(prefix) && strings.TrimSpace(prefix[:col]) == ""
	}
	start, end := len(raw), len(raw)
	replacement := ""
	if skills == nil {
		if len(root.Content) > 0 && !blockMap(root) {
			return fail()
		}
		if len(root.Content) == 0 {
			if root.Anchor != "" || root.Style&yaml.TaggedStyle != 0 || !ownLine(root) {
				return fail()
			}
			start = offset(root.Line) + root.Column - 1
			if start+2 > len(raw) || string(raw[start:start+2]) != "{}" {
				return fail()
			}
			end = start + 2
			replacement = "skills:" + newline + "  disabled: " + string(encoded)
			post := append([]byte(nil), raw[:start]...)
			post = append(post, []byte(replacement)...)
			post = append(post, raw[end:]...)
			var after map[string]any
			if yaml.Unmarshal(post, &after) != nil {
				return fail()
			}
			return post, change, !bytes.Equal(original, post), nil
		}
		if start > 0 && raw[start-1] != '\n' {
			replacement = newline
		}
		replacement += "skills:" + newline + "  disabled: " + string(encoded) + newline
	} else {
		if !ownLine(skillKey) || skills.Line == skillKey.Line {
			return fail()
		}
		line := string(lines[skillKey.Line-1])
		_, tail, found := strings.Cut(line, ":")
		tail = strings.TrimSpace(tail)
		if !found || (tail != "" && !strings.HasPrefix(tail, "#")) {
			return fail()
		}
		indent := skills.Column - 1
		if disabled == nil {
			start = offset(skillKey.Line + 1)
			end = start
		} else {
			if !ownLine(disabledKey) {
				return fail()
			}
			last := disabled.Line
			for _, node := range disabled.Content {
				if node.Line > last {
					last = node.Line
				}
			}
			// A flow collection may span lines. Limit it to one physical line.
			if disabled.Style&yaml.FlowStyle != 0 && last != disabled.Line {
				return fail()
			}
			if disabled.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
				return fail()
			}
			start = offset(disabledKey.Line)
			end = offset(last + 1)
			if disabled.Kind == yaml.SequenceNode && disabled.Style&yaml.FlowStyle != 0 && !bytes.Contains(lines[disabled.Line-1], []byte("]")) {
				return fail()
			}
		}
		replacement = strings.Repeat(" ", indent) + "disabled: " + string(encoded) + newline
	}
	post := append([]byte(nil), raw[:start]...)
	post = append(post, []byte(replacement)...)
	post = append(post, raw[end:]...)
	var after map[string]any
	if yaml.Unmarshal(post, &after) != nil {
		return fail()
	}
	// Check preservation independently of byte-span calculation.
	beforeSkills, _ := data["skills"].(map[string]any)
	afterSkills, _ := after["skills"].(map[string]any)
	delete(data, "skills")
	delete(after, "skills")
	delete(beforeSkills, "disabled")
	delete(afterSkills, "disabled")
	if len(beforeSkills) == 0 {
		beforeSkills = nil
	}
	if len(afterSkills) == 0 {
		afterSkills = nil
	}
	if !reflect.DeepEqual(data, after) || !reflect.DeepEqual(beforeSkills, afterSkills) {
		return nil, change, false, errors.New("Hermes patch would alter unrelated settings")
	}
	return post, change, !bytes.Equal(original, post), nil
}
