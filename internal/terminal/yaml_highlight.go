package terminal

import (
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type sourceSpan struct {
	start int
	end   int
}

func styledYAMLMetadataLines(value, prefix string, width int, palette browserPalette) []string {
	if !palette.enabled {
		return wrappedBlock(value, prefix, width)
	}
	spans, ok := yamlMappingKeySpans(value)
	if !ok {
		return wrappedBlock(value, prefix, width)
	}

	var styled strings.Builder
	last := 0
	for _, span := range spans {
		styled.WriteString(safeBlock(value[last:span.start]))
		styled.WriteString(palette.contentLabel(safeBlock(value[span.start:span.end])))
		last = span.end
	}
	styled.WriteString(safeBlock(value[last:]))

	inputLines := strings.Split(styled.String(), "\n")
	output := make([]string, 0, len(inputLines))
	for _, line := range inputLines {
		output = append(output, wrapLine(prefix+line, width)...)
	}
	return output
}

func yamlMappingKeySpans(value string) ([]sourceSpan, bool) {
	offsets, ok := yamlSourceOffsets(value)
	if !ok {
		return nil, false
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(value), &document); err != nil || len(document.Content) != 1 {
		return nil, false
	}
	spans := make([]sourceSpan, 0, 8)
	if !collectYAMLMappingKeySpans(value, offsets, document.Content[0], &spans) {
		return nil, false
	}
	sort.Slice(spans, func(left, right int) bool {
		return spans[left].start < spans[right].start
	})
	for index, span := range spans {
		if span.start < 0 || span.end <= span.start || span.end > len(value) {
			return nil, false
		}
		if index > 0 && spans[index-1].end > span.start {
			return nil, false
		}
	}
	return spans, true
}

func collectYAMLMappingKeySpans(value string, offsets [][]int, node *yaml.Node, spans *[]sourceSpan) bool {
	switch node.Kind {
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return false
		}
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode {
				return false
			}
			span, ok := yamlScalarSourceSpan(value, offsets, key)
			if !ok {
				return false
			}
			*spans = append(*spans, span)
			if !collectYAMLMappingKeySpans(value, offsets, node.Content[index+1], spans) {
				return false
			}
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			if !collectYAMLMappingKeySpans(value, offsets, child, spans) {
				return false
			}
		}
	case yaml.ScalarNode, yaml.AliasNode:
		return true
	default:
		return false
	}
	return true
}

func yamlScalarSourceSpan(value string, offsets [][]int, node *yaml.Node) (sourceSpan, bool) {
	start, ok := yamlSourceOffset(offsets, node.Line, node.Column)
	if !ok || start >= len(value) {
		return sourceSpan{}, false
	}

	style := node.Style & (yaml.DoubleQuotedStyle | yaml.SingleQuotedStyle | yaml.LiteralStyle | yaml.FoldedStyle)
	switch style {
	case 0:
		if !strings.HasPrefix(value[start:], node.Value) || strings.ContainsAny(node.Value, "\r\n") {
			return sourceSpan{}, false
		}
		return sourceSpan{start: start, end: start + len(node.Value)}, true
	case yaml.DoubleQuotedStyle:
		end, ok := quotedYAMLScalarEnd(value, start, '"')
		return sourceSpan{start: start, end: end}, ok && !strings.ContainsAny(value[start:end], "\r\n")
	case yaml.SingleQuotedStyle:
		end, ok := quotedYAMLScalarEnd(value, start, '\'')
		return sourceSpan{start: start, end: end}, ok && !strings.ContainsAny(value[start:end], "\r\n")
	default:
		return sourceSpan{}, false
	}
}

func quotedYAMLScalarEnd(value string, start int, quote byte) (int, bool) {
	if start >= len(value) || value[start] != quote {
		return 0, false
	}
	for index := start + 1; index < len(value); index++ {
		if value[index] != quote {
			continue
		}
		if quote == '\'' && index+1 < len(value) && value[index+1] == quote {
			index++
			continue
		}
		if quote == '"' {
			backslashes := 0
			for previous := index - 1; previous > start && value[previous] == '\\'; previous-- {
				backslashes++
			}
			if backslashes%2 == 1 {
				continue
			}
		}
		return index + 1, true
	}
	return 0, false
}

func yamlSourceOffsets(value string) ([][]int, bool) {
	offsets := [][]int{nil}
	for index, char := range value {
		switch char {
		case '\n':
			offsets = append(offsets, nil)
		case '\r':
			if index+1 >= len(value) || value[index+1] != '\n' {
				return nil, false
			}
		case '\u0085', '\u2028', '\u2029':
			return nil, false
		default:
			line := len(offsets) - 1
			offsets[line] = append(offsets[line], index)
		}
	}
	return offsets, true
}

func yamlSourceOffset(offsets [][]int, line, column int) (int, bool) {
	if line < 1 || line > len(offsets) || column < 1 || column > len(offsets[line-1]) {
		return 0, false
	}
	return offsets[line-1][column-1], true
}
