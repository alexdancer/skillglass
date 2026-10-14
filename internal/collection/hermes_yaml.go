package collection

import (
	"strings"

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

func (observation hermesYAMLObservation) known(paths ...string) bool {
	if observation.root == nil {
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
	if len(path) > 0 {
		if node.Kind == yaml.MappingNode {
			for index := 0; index < len(node.Content); index += 2 {
				key := node.Content[index]
				var value any
				if key.Decode(&value) == nil {
					if text, ok := value.(string); ok && text == path[0] {
						keyKnown, _ := hermesYAMLPathKnown(key, nil, active)
						valueKnown, present := hermesYAMLPathKnown(node.Content[index+1], path[1:], active)
						return keyKnown && valueKnown, present
					}
				}
			}
			for index := 0; index < len(node.Content); index += 2 {
				if node.Content[index].ShortTag() == "!!merge" {
					known, present := hermesYAMLPathKnown(node.Content[index+1], path, active)
					if present {
						return known, true
					}
				}
			}
		} else if node.Kind == yaml.SequenceNode {
			for _, child := range node.Content {
				known, present := hermesYAMLPathKnown(child, path, active)
				if present {
					return known, true
				}
			}
		}
		return true, false
	}
	if node.Kind == yaml.ScalarNode && node.Style == 0 && node.ShortTag() == "!!str" && hermesYAMLImplicitBoolean(node.Value) {
		return false, true
	}
	children := node.Content
	if node.Kind == yaml.MappingNode {
		var known bool
		children, known = hermesYAMLEffectiveEntries(node, make(map[*yaml.Node]bool))
		if !known {
			return false, true
		}
	}
	for _, child := range children {
		known, _ := hermesYAMLPathKnown(child, nil, active)
		if !known {
			return false, true
		}
	}
	return true, true
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
		tag := resolved.ShortTag()
		if tag == "!!str" && resolved.Style == 0 && hermesYAMLImplicitBoolean(resolved.Value) {
			tag = "!!bool"
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
