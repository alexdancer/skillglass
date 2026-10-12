package terminal

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/glamour/v2"
)

func renderMarkdownLines(value string, width int, theme browserTheme) []string {
	if value == "" {
		return []string{"(No instructions.)"}
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(theme.markdownStyle()),
		glamour.WithWordWrap(max(width, 1)),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return rawInstructionLines(value, width, "Markdown rendering unavailable; showing escaped raw text.")
	}
	rendered, err := renderer.Render(safeBlock(neutralizeControlEntities(value)))
	if err != nil {
		return rawInstructionLines(value, width, "Markdown rendering unavailable; showing escaped raw text.")
	}
	rendered = sanitizeRenderedTerminal(rendered)
	rendered = strings.Trim(rendered, "\n")
	if rendered == "" {
		return []string{"(No rendered instructions.)"}
	}
	return strings.Split(rendered, "\n")
}

func rawInstructionLines(value string, width int, prefix ...string) []string {
	lines := append([]string(nil), prefix...)
	for _, line := range strings.Split(safeBlock(value), "\n") {
		lines = append(lines, wrapLine(line, width)...)
	}
	return lines
}

// neutralizeControlEntities handles controls which a Markdown parser can
// introduce while decoding HTML numeric entities. Raw controls are escaped by
// safeBlock later; encoded controls need to become visible text before parsing.
func neutralizeControlEntities(value string) string {
	var output strings.Builder
	for index := 0; index < len(value); {
		if value[index] != '&' {
			output.WriteByte(value[index])
			index++
			continue
		}
		if strings.HasPrefix(value[index:], "&Tab;") {
			output.WriteString(`\t`)
			index += len("&Tab;")
			continue
		}
		if strings.HasPrefix(value[index:], "&NewLine;") {
			output.WriteString(`\n`)
			index += len("&NewLine;")
			continue
		}
		end, decoded, ok := numericControlEntity(value, index)
		if ok {
			output.WriteString(safeInline(string(decoded)))
			index = end
			continue
		}
		output.WriteByte(value[index])
		index++
	}
	return output.String()
}

func numericControlEntity(value string, start int) (int, rune, bool) {
	if start+3 > len(value) || value[start] != '&' || value[start+1] != '#' {
		return start, 0, false
	}
	base := 10
	digitsStart := start + 2
	if digitsStart < len(value) && (value[digitsStart] == 'x' || value[digitsStart] == 'X') {
		base = 16
		digitsStart++
	}
	end := digitsStart
	for end < len(value) {
		char := value[end]
		valid := char >= '0' && char <= '9'
		if base == 16 {
			valid = valid || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F'
		}
		if !valid {
			break
		}
		end++
	}
	if end == digitsStart {
		return start, 0, false
	}
	tokenEnd := end
	if tokenEnd < len(value) && value[tokenEnd] == ';' {
		tokenEnd++
	}
	code, err := strconv.ParseInt(value[digitsStart:end], base, 32)
	if err != nil {
		return start, 0, false
	}
	decoded := rune(code)
	if decoded >= 0x20 && decoded != 0x7f && (decoded < 0x80 || decoded >= 0xa0) {
		return start, 0, false
	}
	return tokenEnd, decoded, true
}

// sanitizeRenderedTerminal removes renderer-owned OSC 8 links while keeping
// their visible text and URL. Any other control introduced during Markdown
// decoding is rendered as inert escape notation. Only CSI SGR styling emitted
// by the selected built-in style is allowed through.
func sanitizeRenderedTerminal(value string) string {
	var output strings.Builder
	for index := 0; index < len(value); {
		if value[index] == 0x1b && index+1 < len(value) {
			switch value[index+1] {
			case ']':
				end := oscEnd(value, index+2)
				sequence := value[index:end]
				payload := value[index+2 : oscPayloadEnd(value, index+2, end)]
				if !strings.HasPrefix(payload, "8;") {
					output.WriteString(safeInline(sequence))
				}
				index = end
				continue
			case '[':
				end := csiEnd(value, index+2)
				sequence := value[index:end]
				if validSGR(sequence) {
					output.WriteString(sequence)
				} else {
					output.WriteString(safeInline(sequence))
				}
				index = end
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		if r == utf8.RuneError && size == 1 {
			output.WriteString(`\xff`)
			index++
			continue
		}
		if r == '\n' || r == '\t' || r >= 0x20 && r != 0x7f && (r < 0x80 || r >= 0xa0) {
			output.WriteRune(r)
		} else {
			output.WriteString(safeInline(string(r)))
		}
		index += size
	}
	return output.String()
}

func oscEnd(value string, payloadStart int) int {
	for index := payloadStart; index < len(value); index++ {
		if value[index] == '\a' {
			return index + 1
		}
		if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '\\' {
			return index + 2
		}
	}
	return len(value)
}

func oscPayloadEnd(value string, payloadStart, sequenceEnd int) int {
	end := sequenceEnd
	if end > payloadStart && value[end-1] == '\a' {
		end--
	} else if end >= payloadStart+2 && value[end-2] == 0x1b && value[end-1] == '\\' {
		end -= 2
	}
	return end
}

func csiEnd(value string, parametersStart int) int {
	for index := parametersStart; index < len(value); index++ {
		if value[index] >= 0x40 && value[index] <= 0x7e {
			return index + 1
		}
	}
	return len(value)
}

func validSGR(sequence string) bool {
	if len(sequence) < 3 || sequence[0] != 0x1b || sequence[1] != '[' || sequence[len(sequence)-1] != 'm' {
		return false
	}
	for _, char := range sequence[2 : len(sequence)-1] {
		if char < '0' || char > '9' {
			if char != ';' && char != ':' {
				return false
			}
		}
	}
	return true
}
