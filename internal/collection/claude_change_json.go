package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// JSON spans retain every byte outside the selected member. The decoder validates
// the full document; this parser additionally refuses duplicate object keys.
type jsonMember struct {
	key               string
	start, value, end int
	object            *jsonObject
}
type jsonObject struct {
	start, end int
	members    []jsonMember
}
type jsonSpans struct {
	raw []byte
	pos int
}

func (p *jsonSpans) space() {
	for p.pos < len(p.raw) && bytes.ContainsRune([]byte(" \r\n\t"), rune(p.raw[p.pos])) {
		p.pos++
	}
}
func (p *jsonSpans) str() string {
	start := p.pos
	p.pos++
	for p.pos < len(p.raw) {
		c := p.raw[p.pos]
		p.pos++
		if c == '\\' {
			p.pos++
		} else if c == '"' {
			break
		}
	}
	var s string
	_ = json.Unmarshal(p.raw[start:p.pos], &s)
	return s
}
func (p *jsonSpans) value() (*jsonObject, error) {
	p.space()
	if p.raw[p.pos] == '{' {
		return p.object()
	}
	if p.raw[p.pos] == '[' {
		p.pos++
		p.space()
		for p.raw[p.pos] != ']' {
			if _, err := p.value(); err != nil {
				return nil, err
			}
			p.space()
			if p.raw[p.pos] == ',' {
				p.pos++
				continue
			}
			break
		}
		p.pos++
		return nil, nil
	}
	if p.raw[p.pos] == '"' {
		p.str()
		return nil, nil
	}
	for p.pos < len(p.raw) && !bytes.ContainsRune([]byte(",]} \r\n\t"), rune(p.raw[p.pos])) {
		p.pos++
	}
	return nil, nil
}
func (p *jsonSpans) object() (*jsonObject, error) {
	o := &jsonObject{start: p.pos}
	p.pos++
	p.space()
	seen := map[string]bool{}
	for p.raw[p.pos] != '}' {
		m := jsonMember{start: p.pos}
		m.key = p.str()
		if seen[m.key] {
			return nil, errors.New("duplicate JSON key: " + m.key)
		}
		seen[m.key] = true
		p.space()
		p.pos++
		p.space()
		m.value = p.pos
		var err error
		m.object, err = p.value()
		if err != nil {
			return nil, err
		}
		m.end = p.pos
		o.members = append(o.members, m)
		p.space()
		if p.raw[p.pos] == ',' {
			p.pos++
			p.space()
			continue
		}
		break
	}
	p.pos++
	o.end = p.pos
	return o, nil
}
func parseClaudeJSON(raw []byte) (*jsonObject, error) {
	if !json.Valid(raw) {
		return nil, errors.New("settings must be valid JSON")
	}
	p := jsonSpans{raw: raw}
	p.space()
	if p.raw[p.pos] != '{' {
		return nil, errors.New("settings must be an object")
	}
	return p.object()
}
func patchClaudeChoice(raw []byte, name string, choice ControlChoice) ([]byte, SettingChange, bool, error) {
	change := SettingChange{Setting: "skillOverrides." + name, Before: "inherited", After: "inherited"}
	if len(raw) == 0 {
		if choice == ControlInherit {
			return raw, change, false, nil
		}
		raw = []byte("{}\n")
	}
	root, err := parseClaudeJSON(raw)
	if err != nil {
		return nil, change, false, err
	}
	var overrides *jsonObject
	for _, m := range root.members {
		if m.key == "skillOverrides" {
			overrides = m.object
			if overrides == nil {
				return nil, change, false, errors.New("skillOverrides must be an object")
			}
		}
	}
	wanted := "on"
	if choice == ControlDisable {
		wanted = "off"
	}
	if choice != ControlInherit {
		change.After = wanted
	}
	replace := func(start, end int, value []byte) ([]byte, SettingChange, bool, error) {
		result := append([]byte(nil), raw[:start]...)
		result = append(result, value...)
		result = append(result, raw[end:]...)
		if !json.Valid(result) || !claudePatchOnlySelected(raw, result, name) {
			return nil, change, false, errors.New("invalid settings patch")
		}
		return result, change, !bytes.Equal(raw, result), nil
	}
	if overrides != nil {
		for i, m := range overrides.members {
			if m.key != name {
				continue
			}
			var before string
			if json.Unmarshal(raw[m.value:m.end], &before) != nil {
				return nil, change, false, errors.New("selected skill override is not a string")
			}
			change.Before = before
			if choice != ControlInherit {
				value, _ := json.Marshal(wanted)
				return replace(m.value, m.end, value)
			}
			start, end := m.start, m.end
			if i+1 < len(overrides.members) {
				end = overrides.members[i+1].start
			} else if i > 0 {
				start = overrides.members[i-1].end
			}
			return replace(start, end, nil)
		}
	}
	if choice == ControlInherit {
		return raw, change, false, nil
	}
	key, _ := json.Marshal(name)
	value, _ := json.Marshal(wanted)
	member := append(append(key, ':'), value...)
	if overrides != nil {
		if len(overrides.members) > 0 {
			member = append([]byte(","), member...)
		}
		return replace(overrides.end-1, overrides.end-1, member)
	}
	member = []byte(fmt.Sprintf("\"skillOverrides\":{%s}", member))
	if len(root.members) > 0 {
		member = append([]byte(","), member...)
	}
	return replace(root.end-1, root.end-1, member)
}

// Check the patch contract independently of span/comma handling. RawMessage
// equality also preserves number spellings and nested unrelated JSON bytes.
func claudePatchOnlySelected(before, after []byte, name string) bool {
	var a, b map[string]json.RawMessage
	if json.Unmarshal(before, &a) != nil || json.Unmarshal(after, &b) != nil {
		return false
	}
	var oldEntries, newEntries map[string]json.RawMessage
	if raw, ok := a["skillOverrides"]; ok {
		if json.Unmarshal(raw, &oldEntries) != nil {
			return false
		}
	}
	if raw, ok := b["skillOverrides"]; ok {
		if json.Unmarshal(raw, &newEntries) != nil {
			return false
		}
	}
	delete(a, "skillOverrides")
	delete(b, "skillOverrides")
	delete(oldEntries, name)
	delete(newEntries, name)
	if len(oldEntries) == 0 && len(newEntries) == 0 {
		oldEntries = nil
		newEntries = nil
	}
	return reflect.DeepEqual(a, b) && reflect.DeepEqual(oldEntries, newEntries)
}
