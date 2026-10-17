package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"
)

// geminiDisabledDocument retains syntax and typed disabled names so scoped
// patches preserve unrelated settings and comments as exact bytes.
type geminiDisabledDocument struct {
	raw, clean   []byte
	root, skills *jsonObject
	disabled     *jsonMember
	names        []string
}

func readGeminiDisabledDocument(raw []byte) (geminiDisabledDocument, error) {
	d := geminiDisabledDocument{raw: raw}
	if len(raw) == 0 {
		raw = []byte("{}\n")
		d.raw = raw
	}
	if !utf8.Valid(raw) {
		return d, errors.New("settings must be UTF-8")
	}
	var err error
	d.clean, err = geminiJSONBytes(raw)
	if err != nil {
		return d, err
	}
	if err = validateOpencodeUnicode(d.clean); err != nil {
		return d, err
	}
	d.root, err = parseSettingsJSON(d.clean)
	if err != nil {
		return d, err
	}
	for i := range d.root.members {
		m := &d.root.members[i]
		if m.key == "skills" {
			d.skills = m.object
			if d.skills == nil {
				return d, errors.New("skills must be an object")
			}
		}
	}
	if d.skills != nil {
		for i := range d.skills.members {
			m := &d.skills.members[i]
			if m.key == "disabled" {
				d.disabled = m
				var entries []any
				if json.Unmarshal(d.clean[m.value:m.end], &entries) != nil {
					return d, errors.New("skills.disabled must be a string array")
				}
				for _, entry := range entries {
					if _, ok := entry.(string); !ok {
						return d, errors.New("skills.disabled must contain strings")
					}
				}
				if json.Unmarshal(d.clean[m.value:m.end], &d.names) != nil || len(d.clean[m.value:m.end]) == 0 || d.clean[m.value] != '[' {
					return d, errors.New("skills.disabled must be a string array")
				}
			}
		}
	}
	for _, name := range d.names {
		if geminiSettingsPlaceholder.MatchString(name) {
			return d, errors.New("environment-expanded disabled names are unverified")
		}
	}
	return d, nil
}
func (d geminiDisabledDocument) contains(name string) bool {
	key, _ := geminiComparisonKey(name)
	for _, n := range d.names {
		k, _ := geminiComparisonKey(n)
		if k == key {
			return true
		}
	}
	return false
}
func (d geminiDisabledDocument) patch(name string, choice ControlChoice, absent bool) ([]byte, []SettingChange, bool, error) {
	before := "inherited"
	if d.contains(name) {
		before = "disabled"
	}
	after := "inherited"
	if choice == ControlDisable {
		after = "disabled"
	}
	changes := []SettingChange{{Setting: "skills.disabled (native name " + name + ")", Before: before, After: after}}
	if (choice == ControlDisable) == d.contains(name) {
		if absent {
			return nil, changes, false, nil
		}
		return d.raw, changes, false, nil
	}
	// Existing arrays are edited by string/comma token, retaining unrelated
	// escaped spellings and all comments, even trivia next to removed entries.
	if d.disabled != nil {
		if choice == ControlDisable {
			encoded, _ := json.Marshal(name)
			prefix := ""
			if len(d.names) > 0 {
				prefix = ","
			}
			at := d.disabled.end - 1
			post := append([]byte(nil), d.raw[:at]...)
			post = append(post, []byte(prefix)...)
			post = append(post, encoded...)
			post = append(post, d.raw[at:]...)
			return post, changes, true, nil
		}
		key, _ := geminiComparisonKey(name)
		type span struct{ start, end int }
		tokens := []span{}
		parser := jsonSpans{raw: d.clean, pos: d.disabled.value + 1}
		parser.space()
		for parser.raw[parser.pos] != ']' {
			start := parser.pos
			parser.str()
			tokens = append(tokens, span{start, parser.pos})
			parser.space()
			if parser.raw[parser.pos] == ',' {
				parser.pos++
				parser.space()
			} else {
				break
			}
		}
		remove := map[int]bool{}
		remaining := []int{}
		for i, n := range d.names {
			k, _ := geminiComparisonKey(n)
			if k == key {
				for pos := tokens[i].start; pos < tokens[i].end; pos++ {
					remove[pos] = true
				}
			} else {
				remaining = append(remaining, i)
			}
		}
		// Keep only separators before retained entries after the first.
		commas := opencodeCommaTokens(d.clean)
		keepComma := map[int]bool{}
		for j, i := range remaining {
			if j == 0 {
				continue
			}
			for pos := tokens[i-1].end; pos < tokens[i].start; pos++ {
				if commas[pos] {
					keepComma[pos] = true
				}
			}
		}
		post := []byte{}
		for pos, b := range d.raw {
			if remove[pos] || pos > d.disabled.value && pos < d.disabled.end && commas[pos] && !keepComma[pos] {
				continue
			}
			post = append(post, b)
		}
		if _, err := readGeminiDisabledDocument(post); err != nil {
			return nil, nil, false, err
		}
		return post, changes, true, nil
	}
	names := []string{name}
	encoded, _ := json.Marshal(names)
	obj := d.skills
	value := append([]byte(`"disabled":`), encoded...)
	if obj == nil {
		obj = d.root
		value = append(append([]byte(`"skills":{`), value...), '}')
	}
	if len(obj.members) > 0 {
		value = append([]byte(","), value...)
	}
	start, end := obj.end-1, obj.end-1
	post := append([]byte(nil), d.raw[:start]...)
	post = append(post, value...)
	post = append(post, d.raw[end:]...)
	if _, err := readGeminiDisabledDocument(post); err != nil {
		return nil, nil, false, err
	}
	return post, changes, !bytes.Equal(d.raw, post), nil
}
