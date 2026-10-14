package collection

import (
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type hermesYAMLObservation struct {
	root *yaml.Node
}

func observeHermesYAML(raw []byte) hermesYAMLObservation {
	var node yaml.Node
	if yaml.Unmarshal(raw, &node) != nil || len(node.Content) != 1 {
		return hermesYAMLObservation{}
	}
	return hermesYAMLObservation{root: node.Content[0]}
}

func hermesYAMLResolve(node *yaml.Node) *yaml.Node {
	seen := make(map[*yaml.Node]bool)
	for node != nil && node.Kind == yaml.AliasNode {
		if seen[node] {
			return nil
		}
		seen[node] = true
		node = node.Alias
	}
	return node
}

func (observation hermesYAMLObservation) nativeConstructible() bool {
	return hermesYAMLConstructorValid(observation.root, make(map[*yaml.Node]bool))
}

func (observation hermesYAMLObservation) mappingAt(path ...string) bool {
	node := hermesYAMLResolve(observation.root)
	for _, name := range path {
		if node == nil || node.Kind != yaml.MappingNode || node.ShortTag() != "!!map" {
			return false
		}
		entries, known := hermesYAMLEffectiveEntries(node, make(map[*yaml.Node]bool))
		if !known {
			return false
		}
		node = nil
		for index := 0; index < len(entries); index += 2 {
			var key any
			if entries[index].Decode(&key) == nil && key == name {
				node = hermesYAMLResolve(entries[index+1])
				break
			}
		}
	}
	return node != nil && node.Kind == yaml.MappingNode && node.ShortTag() == "!!map"
}

func hermesYAMLConstructorValid(node *yaml.Node, seen map[*yaml.Node]bool) bool {
	if node == nil {
		return false
	}
	if seen[node] {
		return true
	}
	seen[node] = true
	if node.Kind == yaml.AliasNode {
		return hermesYAMLConstructorValid(node.Alias, seen)
	}
	switch hermesYAMLNativeTag(node) {
	case "!!null", "!!bool", "!!int", "!!float", "!!binary", "!!str":
		scalar := hermesYAMLScalarNode(node)
		return scalar != nil && hermesYAMLScalarValid(hermesYAMLNativeTag(node), scalar.Value)
	case "!!timestamp":
		var value time.Time
		if node.Kind != yaml.ScalarNode || node.Decode(&value) != nil {
			return false
		}
		_, offset := value.Zone()
		return value.Year() >= 1 && value.Year() <= 9999 && offset > -24*60*60 && offset < 24*60*60
	case "!!map", "!!set":
		return hermesYAMLMappingValid(node, seen, make(map[*yaml.Node]bool))
	case "!!seq":
		if node.Kind != yaml.SequenceNode {
			return false
		}
		for _, child := range node.Content {
			if !hermesYAMLConstructorValid(child, seen) {
				return false
			}
		}
		return true
	case "!!omap", "!!pairs":
		if node.Kind != yaml.SequenceNode {
			return false
		}
		for _, child := range node.Content {
			entry := hermesYAMLResolve(child)
			if entry == nil || entry.Kind != yaml.MappingNode || len(entry.Content) != 2 || !hermesYAMLConstructorValid(entry.Content[0], seen) || !hermesYAMLConstructorValid(entry.Content[1], seen) {
				return false
			}
		}
		return true
	}
	return false
}

func hermesYAMLScalarNode(node *yaml.Node) *yaml.Node {
	seen := make(map[*yaml.Node]bool)
	for node = hermesYAMLResolve(node); node != nil && !seen[node]; node = hermesYAMLResolve(node) {
		seen[node] = true
		if node.Kind == yaml.ScalarNode {
			return node
		}
		if node.Kind != yaml.MappingNode {
			return nil
		}
		var value *yaml.Node
		for index := 0; index < len(node.Content); index += 2 {
			key := hermesYAMLResolve(node.Content[index])
			if key != nil && hermesYAMLNativeTag(key) == "!!value" {
				value = node.Content[index+1]
				break
			}
		}
		node = value
	}
	return nil
}

func hermesYAMLMappingValid(node *yaml.Node, seen, active map[*yaml.Node]bool) bool {
	node = hermesYAMLResolve(node)
	if node == nil || node.Kind != yaml.MappingNode || active[node] {
		return false
	}
	active[node] = true
	defer delete(active, node)
	for index := 0; index < len(node.Content); index += 2 {
		key, value := hermesYAMLResolve(node.Content[index]), node.Content[index+1]
		if key == nil {
			return false
		}
		switch hermesYAMLNativeTag(key) {
		case "!!merge":
			operand := hermesYAMLResolve(value)
			if operand == nil {
				return false
			}
			if operand.Kind == yaml.SequenceNode {
				for _, child := range operand.Content {
					if !hermesYAMLMappingValid(child, seen, active) {
						return false
					}
				}
			} else if !hermesYAMLMappingValid(operand, seen, active) {
				return false
			}
			continue
		case "!!value":
			if hermesYAMLScalarNode(key) == nil {
				return false
			}
		default:
			if !hermesYAMLConstructorValid(key, seen) {
				return false
			}
			if key.ShortTag() == "!!map" || key.ShortTag() == "!!set" || key.ShortTag() == "!!seq" || key.ShortTag() == "!!omap" || key.ShortTag() == "!!pairs" {
				return false
			}
		}
		if !hermesYAMLConstructorValid(value, seen) {
			return false
		}
	}
	return true
}

func (observation hermesYAMLObservation) known(paths ...string) bool {
	if !observation.nativeConstructible() {
		return false
	}
	for _, path := range paths {
		known, _ := hermesYAMLPathKnown(observation.root, strings.Split(path, "."), make(map[*yaml.Node]bool))
		if !known {
			return false
		}
	}
	return true
}

func hermesYAMLPathKnown(node *yaml.Node, path []string, active map[*yaml.Node]bool) (bool, bool) {
	if node == nil || active[node] || len(active) >= 128 {
		return false, true
	}
	active[node] = true
	defer delete(active, node)
	if node.Kind == yaml.AliasNode {
		return hermesYAMLPathKnown(node.Alias, path, active)
	}
	if (node.Kind == yaml.MappingNode && node.ShortTag() != "!!map") || (node.Kind == yaml.SequenceNode && node.ShortTag() != "!!seq") {
		return false, true
	}
	if len(path) > 0 {
		if node.Kind != yaml.MappingNode {
			return true, false
		}
		entries, known := hermesYAMLEffectiveEntries(node, make(map[*yaml.Node]bool))
		if !known {
			return false, true
		}
		for index := 0; index < len(entries); index += 2 {
			key := entries[index]
			var value any
			if key.Decode(&value) == nil {
				if text, ok := value.(string); ok && text == path[0] {
					keyKnown := hermesYAMLKeyKnown(key, active)
					valueKnown, present := hermesYAMLPathKnown(entries[index+1], path[1:], active)
					return keyKnown && valueKnown, present
				}
			}
		}
		return true, false
	}
	if node.Kind == yaml.ScalarNode && (node.ShortTag() == "!!binary" || hermesYAMLNativeTag(node) != node.ShortTag()) {
		return false, true
	}
	if node.Kind == yaml.MappingNode {
		entries, known := hermesYAMLEffectiveEntries(node, make(map[*yaml.Node]bool))
		if !known {
			return false, true
		}
		for index := 0; index < len(entries); index += 2 {
			valueKnown, _ := hermesYAMLPathKnown(entries[index+1], nil, active)
			if !hermesYAMLKeyKnown(entries[index], active) || !valueKnown {
				return false, true
			}
		}
		return true, true
	}
	for _, child := range node.Content {
		known, _ := hermesYAMLPathKnown(child, nil, active)
		if !known {
			return false, true
		}
	}
	return true, true
}

func hermesYAMLKeyKnown(node *yaml.Node, active map[*yaml.Node]bool) bool {
	resolved := hermesYAMLResolve(node)
	if resolved != nil && resolved.Kind == yaml.ScalarNode && hermesYAMLNativeTag(resolved) == "!!value" {
		return true
	}
	known, _ := hermesYAMLPathKnown(node, nil, active)
	return known
}

var hermesYAMLSexagesimalInt = regexp.MustCompile(`^[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+$`)
var hermesYAMLSexagesimalFloat = regexp.MustCompile(`^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*$`)

func hermesYAMLNativeTag(node *yaml.Node) string {
	tag := node.ShortTag()
	if tag == "!!str" && node.Style == 0 {
		switch {
		case node.Value == "=":
			return "!!value"
		case hermesYAMLImplicitBoolean(node.Value):
			return "!!bool"
		case hermesYAMLSexagesimalInt.MatchString(node.Value):
			return "!!int"
		case hermesYAMLSexagesimalFloat.MatchString(node.Value):
			return "!!float"
		}
	}
	return tag
}

func hermesYAMLImplicitBoolean(value string) bool {
	switch value {
	case "yes", "Yes", "YES", "no", "No", "NO", "on", "On", "ON", "off", "Off", "OFF":
		return true
	}
	return false
}

func hermesYAMLEffectiveEntries(node *yaml.Node, active map[*yaml.Node]bool) ([]*yaml.Node, bool) {
	if node == nil || active[node] || len(active) >= 128 {
		return nil, false
	}
	active[node] = true
	defer delete(active, node)
	if node.Kind == yaml.AliasNode {
		return hermesYAMLEffectiveEntries(node.Alias, active)
	}
	var entries []*yaml.Node
	seen := make(map[string]bool)
	add := func(key, value *yaml.Node) bool {
		resolved := key
		for depth := 0; resolved != nil && resolved.Kind == yaml.AliasNode && depth < 128; depth++ {
			resolved = resolved.Alias
		}
		if resolved == nil || resolved.Kind != yaml.ScalarNode {
			return false
		}
		tag := hermesYAMLNativeTag(resolved)
		if tag == "!!value" {
			tag = "!!str"
		}
		identity := tag + "\x00" + resolved.Value
		if !seen[identity] {
			seen[identity] = true
			entries = append(entries, key, value)
		}
		return true
	}
	var merges []*yaml.Node
	switch node.Kind {
	case yaml.MappingNode:
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.ShortTag() == "!!merge" {
				merges = append(merges, value)
			} else if !add(key, value) {
				return nil, false
			}
		}
	case yaml.SequenceNode:
		merges = node.Content
	default:
		return nil, false
	}
	for _, merge := range merges {
		inherited, known := hermesYAMLEffectiveEntries(merge, active)
		if !known {
			return nil, false
		}
		for index := 0; index < len(inherited); index += 2 {
			if !add(inherited[index], inherited[index+1]) {
				return nil, false
			}
		}
	}
	return entries, true
}
