package terminal

import (
	"fmt"
	"strings"
	"unicode"
)

func safeInline(value string) string {
	return safeText(value, false)
}

func safeBlock(value string) string {
	return safeText(value, true)
}

func safeText(value string, allowLineFeed bool) string {
	var output strings.Builder
	for _, char := range value {
		if char == '\n' && allowLineFeed {
			output.WriteRune(char)
			continue
		}
		if unicode.IsPrint(char) {
			output.WriteRune(char)
			continue
		}
		switch char {
		case '\a':
			output.WriteString(`\a`)
		case '\b':
			output.WriteString(`\b`)
		case '\t':
			output.WriteString(`\t`)
		case '\n':
			output.WriteString(`\n`)
		case '\v':
			output.WriteString(`\v`)
		case '\f':
			output.WriteString(`\f`)
		case '\r':
			output.WriteString(`\r`)
		case 0x1b:
			output.WriteString(`\x1b`)
		case 0x7f:
			output.WriteString(`\x7f`)
		default:
			if char <= 0xff {
				fmt.Fprintf(&output, `\x%02x`, char)
			} else if char <= 0xffff {
				fmt.Fprintf(&output, `\u%04x`, char)
			} else {
				fmt.Fprintf(&output, `\U%08x`, char)
			}
		}
	}
	return output.String()
}

func indentBlock(value, prefix string) string {
	lines := strings.Split(safeBlock(value), "\n")
	for index := range lines {
		lines[index] = prefix + lines[index]
	}
	return strings.Join(lines, "\n")
}
