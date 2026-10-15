package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type choiceSpan struct {
	path    string
	boolean unstable.Range
}
type choiceSyntax struct {
	rows     []choiceSpan
	array    unstable.Range
	hasArray bool
	aot      bool
}

func nodeKey(n *unstable.Node) string {
	var keys []string
	it := n.Key()
	for it.Next() {
		keys = append(keys, string(it.Node().Data))
	}
	return strings.Join(keys, ".")
}
func inspectChoiceSyntax(raw []byte) (choiceSyntax, error) {
	var syntax choiceSyntax
	var table string
	var row *choiceSpan
	var p unstable.Parser
	p.Reset(raw)
	for p.NextExpression() {
		n := p.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = nodeKey(n)
			row = nil
			if n.Kind == unstable.ArrayTable && table == "skills.config" {
				syntax.aot = true
				syntax.rows = append(syntax.rows, choiceSpan{})
				row = &syntax.rows[len(syntax.rows)-1]
			}
		case unstable.KeyValue:
			key := nodeKey(n)
			v := n.Value()
			if row != nil {
				switch key {
				case "path":
					if v.Kind == unstable.String {
						row.path = string(v.Data)
					}
				case "enabled":
					if v.Kind == unstable.Bool {
						row.boolean = p.Range(v.Data)
					}
				}
			}
			full := key
			if table != "" {
				full = table + "." + key
			}
			if full == "skills.config" && v.Kind == unstable.Array {
				syntax.hasArray = true
				lastKey := n.Key()
				keyEnd := 0
				for lastKey.Next() {
					r := lastKey.Node().Raw
					keyEnd = int(r.Offset + r.Length)
				}
				span, err := arraySourceRange(raw, keyEnd)
				if err != nil {
					return syntax, err
				}
				syntax.array = span
				it := v.Children()
				for it.Next() {
					item := it.Node()
					if item.Kind != unstable.InlineTable {
						return syntax, errors.New("skills.config requires inline tables")
					}
					r := choiceSpan{}
					fields := item.Children()
					for fields.Next() {
						f := fields.Node()
						switch nodeKey(f) {
						case "path":
							if f.Value().Kind == unstable.String {
								r.path = string(f.Value().Data)
							}
						case "enabled":
							if f.Value().Kind == unstable.Bool {
								r.boolean = p.Range(f.Value().Data)
							}
						}
					}
					syntax.rows = append(syntax.rows, r)
				}
			}
		}
	}
	return syntax, p.Error()
}
func choiceArray(doc map[string]any) ([]any, bool, error) {
	skills, exists := doc["skills"]
	if !exists {
		return nil, false, nil
	}
	m, ok := skills.(map[string]any)
	if !ok {
		return nil, false, errors.New("skills must be a TOML table")
	}
	v, exists := m["config"]
	if !exists {
		return nil, false, nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil, false, errors.New("skills.config must be an array")
	}
	return a, true, nil
}
func patchCodexChoice(raw []byte, target string, enabled bool) ([]byte, SettingChange, bool, error) {
	before := make(map[string]any)
	if err := toml.Unmarshal(raw, &before); err != nil {
		return nil, SettingChange{}, false, errors.New("configuration is invalid TOML")
	}
	rows, exists, err := choiceArray(before)
	if err != nil {
		return nil, SettingChange{}, false, err
	}
	matches := 0
	selected := -1
	for i, v := range rows {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, SettingChange{}, false, errors.New("skills.config rows must be tables")
		}
		path, ok := m["path"].(string)
		if !ok || !strings.HasPrefix(path, "/") {
			return nil, SettingChange{}, false, errors.New("unverified name or relative selector prevents a safe source-bound change")
		}
		if _, ok := m["enabled"].(bool); !ok {
			return nil, SettingChange{}, false, errors.New("skills.config requires a boolean enabled")
		}
		if _, hasName := m["name"]; hasName {
			return nil, SettingChange{}, false, errors.New("combined path and name selectors are unverified")
		}
		if canonicalManifest(path) == target {
			matches++
			selected = i
		}
	}
	if matches > 1 {
		return nil, SettingChange{}, false, errors.New("duplicate canonical selectors are ambiguous; no settings changed")
	}
	syntax, err := inspectChoiceSyntax(raw)
	if err != nil {
		return nil, SettingChange{}, false, errors.New("configuration syntax cannot be edited safely")
	}
	desired := fmt.Sprint(enabled)
	change := SettingChange{Setting: "skills.config.path=" + target + ", enabled", Before: "absent", After: desired}
	var output []byte
	if selected >= 0 {
		current := rows[selected].(map[string]any)["enabled"].(bool)
		change.Before = fmt.Sprint(current)
		if current == enabled {
			return append([]byte(nil), raw...), change, false, nil
		}
		if selected >= len(syntax.rows) || syntax.rows[selected].boolean.Length == 0 || canonicalManifest(syntax.rows[selected].path) != target {
			return nil, change, false, errors.New("selected enabled token has unsupported TOML syntax")
		}
		span := syntax.rows[selected].boolean
		start, end := int(span.Offset), int(span.Offset+span.Length)
		output = append(output, raw[:start]...)
		output = append(output, desired...)
		output = append(output, raw[end:]...)
	} else {
		path, _ := json.Marshal(target)
		entry := "{ path = " + string(path) + ", enabled = " + desired + " }"
		if syntax.hasArray {
			start, end := int(syntax.array.Offset), int(syntax.array.Offset+syntax.array.Length)
			if end <= start || end > len(raw) || raw[end-1] != ']' {
				return nil, change, false, errors.New("inline skills.config array span unavailable")
			}
			prefix := ""
			if len(rows) > 0 {
				if lastArrayToken(raw[start+1:end-1]) != ',' {
					prefix = ","
				}
			}
			output = append(output, raw[:end-1]...)
			output = append(output, []byte(prefix+" "+entry+" ")...)
			output = append(output, raw[end-1:]...)
		} else if exists && !syntax.aot {
			return nil, change, false, errors.New("this skills.config encoding cannot be edited losslessly")
		} else {
			// AOT declarations are absolute and valid after ordinary named tables.
			newline := "\n"
			if bytes.Contains(raw, []byte("\r\n")) {
				newline = "\r\n"
			}
			output = append(output, raw...)
			if len(output) > 0 && output[len(output)-1] != '\n' {
				output = append(output, []byte(newline)...)
			}
			output = append(output, []byte(newline+"[[skills.config]]"+newline+"path = "+string(path)+newline+"enabled = "+desired+newline)...)
		}
	}
	after := make(map[string]any)
	if err := toml.Unmarshal(output, &after); err != nil {
		return nil, change, false, errors.New("proposed TOML layout would change an existing table; unsupported syntax")
	}
	afterRows, _, err := choiceArray(after)
	if err != nil {
		return nil, change, false, err
	}
	if selected >= 0 {
		before["skills"].(map[string]any)["config"].([]any)[selected].(map[string]any)["enabled"] = enabled
	} else {
		if before["skills"] == nil {
			before["skills"] = map[string]any{}
		}
		expected := append(append([]any(nil), rows...), map[string]any{"path": target, "enabled": enabled})
		before["skills"].(map[string]any)["config"] = expected
	}
	if !reflect.DeepEqual(before, after) || len(afterRows) != len(rows)+boolInt(selected < 0) {
		return nil, change, false, errors.New("proposed change does not preserve unrelated TOML values")
	}
	return output, change, true, nil
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func arraySourceRange(raw []byte, keyEnd int) (unstable.Range, error) {
	start := keyEnd
	for start < len(raw) && raw[start] != '=' {
		start++
	}
	start++
	for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t') {
		start++
	}
	if start >= len(raw) || raw[start] != '[' {
		return unstable.Range{}, errors.New("inline array opening unavailable")
	}
	depth := 0
	for i := start; i < len(raw); i++ {
		b := raw[i]
		if b == '#' {
			for i < len(raw) && raw[i] != '\n' {
				i++
			}
			continue
		}
		if b == '\'' || b == '"' {
			quote := b
			triple := i+2 < len(raw) && raw[i+1] == quote && raw[i+2] == quote
			if triple {
				i += 3
			} else {
				i++
			}
			closed := false
			for i < len(raw) {
				if quote == '"' && raw[i] == '\\' {
					i += 2
					continue
				}
				if raw[i] == quote {
					if !triple {
						closed = true
						break
					}
					if i+2 < len(raw) && raw[i+1] == quote && raw[i+2] == quote {
						i += 2
						closed = true
						break
					}
				}
				i++
			}
			if !closed {
				return unstable.Range{}, errors.New("inline array string span unavailable")
			}
			continue
		}
		if b == '[' || b == '{' {
			depth++
		}
		if b == ']' || b == '}' {
			depth--
			if depth == 0 {
				return unstable.Range{Offset: uint32(start), Length: uint32(i - start + 1)}, nil
			}
		}
	}
	return unstable.Range{}, errors.New("inline array closing unavailable")
}

func lastArrayToken(raw []byte) byte {
	var last byte
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		if b == '#' {
			for i < len(raw) && raw[i] != '\n' {
				i++
			}
			continue
		}
		if b == '\'' || b == '"' {
			quote := b
			triple := i+2 < len(raw) && raw[i+1] == quote && raw[i+2] == quote
			if triple {
				i += 3
			} else {
				i++
			}
			for i < len(raw) {
				if quote == '"' && raw[i] == '\\' {
					i += 2
					continue
				}
				if raw[i] == quote {
					if !triple {
						break
					}
					if i+2 < len(raw) && raw[i+1] == quote && raw[i+2] == quote {
						i += 2
						break
					}
				}
				i++
			}
			last = quote
			continue
		}
		if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			last = b
		}
	}
	return last
}
