package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"
)

// Span offsets address the original JSONC bytes. The cleaned buffer is equal
// length and is used only to recognize tokens; comments never become edits.
func patchOpencodeChoice(raw []byte, name, wanted string, inherit bool) ([]byte, []SettingChange, bool, error) {
	change := SettingChange{Setting: "permission.skill." + name, Before: "inherited", After: wanted}
	if inherit {
		change.After = "inherited"
	}
	if len(raw) == 0 {
		if inherit {
			return raw, []SettingChange{change}, false, nil
		}
		raw = []byte("{}\n")
	}
	if !utf8.Valid(raw) {
		return nil, nil, false, errors.New("configuration is not valid UTF-8")
	}
	commas := opencodeCommaTokens(raw)
	original, err := parseOpencode(raw, "patch")
	if err != nil {
		return nil, nil, false, err
	}
	clean, err := opencodeJSONC(raw)
	if err != nil {
		return nil, nil, false, err
	}
	root, err := parseSettingsJSON(clean)
	if err != nil {
		return nil, nil, false, err
	}
	member := func(obj *jsonObject, key string) *jsonMember {
		if obj != nil {
			for i := range obj.members {
				if obj.members[i].key == key {
					return &obj.members[i]
				}
			}
		}
		return nil
	}
	permission := member(root, "permission")
	var skill *jsonMember
	if permission != nil && permission.object != nil {
		skill = member(permission.object, "skill")
	}
	selected := (*jsonMember)(nil)
	if skill != nil {
		selected = member(skill.object, name)
	}
	if selected != nil {
		var before string
		if json.Unmarshal(clean[selected.value:selected.end], &before) != nil {
			return nil, nil, false, errors.New("selected permission is not an action")
		}
		change.Before = before
	}
	replace := func(start, end int, value []byte) ([]byte, []SettingChange, bool, error) {
		post := append([]byte(nil), raw[:start]...)
		post = append(post, value...)
		post = append(post, raw[end:]...)
		updated, e := parseOpencode(post, "patch")
		if e != nil || !opencodePatchOnlySelected(original, updated, name) {
			return nil, nil, false, errors.New("patch would change unrelated ordered configuration")
		}
		return post, []SettingChange{change}, !bytes.Equal(raw, post), nil
	}
	if selected != nil {
		if !inherit {
			encoded, _ := json.Marshal(wanted)
			return replace(selected.value, selected.end, encoded)
		}
		// Remove the member and exactly one comma token, retaining all trivia.
		post := append([]byte(nil), raw...)
		comma := -1
		for i := selected.end; i < skill.object.end-1; i++ {
			if commas[i] {
				comma = i
				break
			}
			if clean[i] != ' ' && clean[i] != '\t' && clean[i] != '\n' && clean[i] != '\r' {
				break
			}
		}
		if comma < 0 {
			for i := selected.start - 1; i > skill.object.start; i-- {
				if commas[i] {
					comma = i
					break
				}
			}
		}
		start, end := selected.start, selected.end
		if comma >= end {
			post = append(post[:comma], post[comma+1:]...)
		}
		post = append(post[:start], post[end:]...)
		if comma >= 0 && comma < start {
			post = append(post[:comma], post[comma+1:]...)
		}
		updated, e := parseOpencode(post, "patch")
		if e != nil || !opencodePatchOnlySelected(original, updated, name) {
			return nil, nil, false, errors.New("invalid JSONC removal")
		}
		return post, []SettingChange{change}, true, nil
	}
	if inherit {
		return raw, []SettingChange{change}, false, nil
	}
	key, _ := json.Marshal(name)
	value, _ := json.Marshal(wanted)
	entry := fmt.Sprintf("%s:%s", key, value)
	appendMember := func(obj *jsonObject, text string) ([]byte, []SettingChange, bool, error) {
		prefix := ""
		if len(obj.members) > 0 {
			last := obj.members[len(obj.members)-1]
			trailing := false
			for i := last.end; i < obj.end-1; i++ {
				trailing = trailing || commas[i]
			}
			if !trailing {
				prefix = ","
			}
		}
		return replace(obj.end-1, obj.end-1, []byte(prefix+text))
	}
	if skill != nil {
		if skill.object != nil {
			return appendMember(skill.object, entry)
		}
		var baseline string
		if json.Unmarshal(clean[skill.value:skill.end], &baseline) != nil {
			return nil, nil, false, errors.New("skill permission must be action or object")
		}
		change.Before = baseline
		old, _ := json.Marshal(baseline)
		return replace(skill.value, skill.end, []byte(fmt.Sprintf(`{"*":%s,%s}`, old, entry)))
	}
	if permission != nil {
		if permission.object != nil {
			return appendMember(permission.object, `"skill":{`+entry+`}`)
		}
		var baseline string
		if json.Unmarshal(clean[permission.value:permission.end], &baseline) != nil {
			return nil, nil, false, errors.New("permission must be action or object")
		}
		old, _ := json.Marshal(baseline)
		return replace(permission.value, permission.end, []byte(fmt.Sprintf(`{"*":%s,"skill":{%s}}`, old, entry)))
	}
	return appendMember(root, `"permission":{"skill":{`+entry+`}}`)
}

// Compare ordered configuration independently of token splicing. Normalize only
// the scalar forms whose native interpretation is equivalent within one layer.
func opencodePatchOnlySelected(a, b *opencodeObject, name string) bool {
	if a == nil || b == nil {
		return false
	}
	strip := func(root *opencodeObject) {
		if scalar, ok := root.get("permission").(string); ok {
			p := newOpencodeObject()
			p.put("*", scalar, "")
			root.put("permission", p, "")
		}
		p, _ := root.get("permission").(*opencodeObject)
		if p == nil {
			return
		}
		if scalar, ok := p.get("skill").(string); ok {
			s := newOpencodeObject()
			s.put("*", scalar, "")
			p.put("skill", s, "")
		}
		s, _ := p.get("skill").(*opencodeObject)
		if s != nil {
			delete(s.values, name)
			s.keys = opencodeWithoutKey(s.keys, name)
			if len(s.keys) == 0 {
				delete(p.values, "skill")
				p.keys = opencodeWithoutKey(p.keys, "skill")
			}
		}
		if len(p.keys) == 0 {
			delete(root.values, "permission")
			root.keys = opencodeWithoutKey(root.keys, "permission")
		}
	}
	strip(a)
	strip(b)
	return reflect.DeepEqual(opencodeOrderedValue(a), opencodeOrderedValue(b))
}
func opencodeWithoutKey(keys []string, key string) []string {
	out := []string{}
	for _, k := range keys {
		if k != key {
			out = append(out, k)
		}
	}
	return out
}
func opencodeOrderedValue(value any) any {
	switch v := value.(type) {
	case *opencodeObject:
		out := []any{}
		for _, k := range v.orderedKeys() {
			out = append(out, []any{k, opencodeOrderedValue(v.get(k))})
		}
		return out
	case []any:
		out := []any{}
		for _, item := range v {
			out = append(out, opencodeOrderedValue(item))
		}
		return out
	default:
		return value
	}
}
func opencodeSavedPermission(raw []byte, name string) PermissionState {
	if len(raw) == 0 {
		return ""
	}
	o, e := parseOpencode(raw, "")
	if e != nil {
		return PermissionUnknown
	}
	p, _ := o.get("permission").(*opencodeObject)
	s, _ := p.get("skill").(*opencodeObject)
	switch s.get(name) {
	case "allow":
		return PermissionAllowed
	case "deny":
		return PermissionDenied
	case "ask":
		return PermissionAsk
	}
	return ""
}

func opencodeCommaTokens(raw []byte) map[int]bool {
	out := map[int]bool{}
	quoted, escaped := false, false
	for i := 0; i < len(raw); i++ {
		if quoted {
			if escaped {
				escaped = false
			} else if raw[i] == '\\' {
				escaped = true
			} else if raw[i] == '"' {
				quoted = false
			}
			continue
		}
		if raw[i] == '"' {
			quoted = true
			continue
		}
		if raw[i] == '/' && i+1 < len(raw) {
			if raw[i+1] == '/' {
				for i < len(raw) && raw[i] != '\n' {
					i++
				}
				continue
			}
			if raw[i+1] == '*' {
				i += 2
				for i+1 < len(raw) && !(raw[i] == '*' && raw[i+1] == '/') {
					i++
				}
				i++
				continue
			}
		}
		if raw[i] == ',' {
			out[i] = true
		}
	}
	return out
}
