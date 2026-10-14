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
	if active[node] {
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
	if node.Kind == yaml.ScalarNode && node.Style == 0 && node.ShortTag() == "!!str" {
		switch node.Value {
		case "yes", "Yes", "YES", "no", "No", "NO", "on", "On", "ON", "off", "Off", "OFF":
			return false, true
		}
	}
	for _, child := range node.Content {
		known, _ := hermesYAMLPathKnown(child, nil, active)
		if !known {
			return false, true
		}
	}
	return true, true
}
