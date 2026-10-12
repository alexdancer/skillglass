package terminal

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/glamour/v2/ansi"
)

type browserTheme uint8

const (
	themeDark browserTheme = iota
	themeLight
	themeNone
)

func resolveBrowserTheme(value string) (browserTheme, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "mono":
		return themeNone, nil
	case "dark":
		if noColorRequested() {
			return themeNone, nil
		}
		return themeDark, nil
	case "light":
		if noColorRequested() {
			return themeNone, nil
		}
		return themeLight, nil
	case "auto":
		if noColorRequested() {
			return themeNone, nil
		}
		if backgroundIsLight(os.Getenv("COLORFGBG")) {
			return themeLight, nil
		}
		return themeDark, nil
	default:
		return 0, fmt.Errorf("unknown browser theme %q; want auto, dark, light, or none", value)
	}
}

func noColorRequested() bool {
	value, present := os.LookupEnv("NO_COLOR")
	return present && value != ""
}

func backgroundIsLight(colorFGBG string) bool {
	parts := strings.FieldsFunc(colorFGBG, func(char rune) bool {
		return char == ';' || char == ':'
	})
	if len(parts) == 0 {
		return false
	}
	background, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return false
	}
	return background >= 7 && background <= 15
}

type browserPalette struct {
	enabled            bool
	accent             string
	muted              string
	warning            string
	danger             string
	selectedBackground string
}

func paletteFor(theme browserTheme) browserPalette {
	switch theme {
	case themeLight:
		return browserPalette{
			enabled:            true,
			accent:             "38;2;3;105;161",
			muted:              "38;2;90;101;116",
			warning:            "38;2;146;64;14",
			danger:             "38;2;185;28;28",
			selectedBackground: "48;2;219;234;254",
		}
	case themeNone:
		return browserPalette{}
	default:
		return browserPalette{
			enabled:            true,
			accent:             "38;2;147;197;253",
			muted:              "38;2;168;179;195",
			warning:            "38;2;246;193;119",
			danger:             "38;2;248;113;113",
			selectedBackground: "48;2;32;66;91",
		}
	}
}

func (palette browserPalette) styled(value string, codes ...string) string {
	if !palette.enabled || value == "" {
		return value
	}
	active := make([]string, 0, len(codes))
	for _, code := range codes {
		if code != "" {
			active = append(active, code)
		}
	}
	if len(active) == 0 {
		return value
	}
	return "\x1b[" + strings.Join(active, ";") + "m" + value + "\x1b[0m"
}

func (palette browserPalette) brand(value string) string {
	return palette.styled(value, palette.accent, "1")
}

func (palette browserPalette) active(value string) string {
	return palette.styled(value, palette.accent, "1")
}

func (palette browserPalette) secondary(value string) string {
	return palette.styled(value, palette.muted)
}

func (palette browserPalette) caution(value string) string {
	return palette.styled(value, palette.warning)
}

func (palette browserPalette) failure(value string) string {
	return palette.styled(value, palette.danger, "1")
}

func (palette browserPalette) selected(value string) string {
	return palette.styled(value, palette.selectedBackground)
}

func (theme browserTheme) markdownStyle() ansi.StyleConfig {
	if theme == themeNone {
		return plainMarkdownStyle()
	}

	body := "#DCE5EF"
	accent := "#93C5FD"
	muted := "#A8B3C3"
	codeBackground := "#162231"
	if theme == themeLight {
		body = "#1E293B"
		accent = "#0369A1"
		muted = "#5A6574"
		codeBackground = "#E7F1FA"
	}

	one := uint(1)
	two := uint(2)
	bold := true
	italic := true
	underline := true
	crossedOut := true
	quote := "│ "
	center := "┼"
	column := "│"
	row := "─"
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "\n",
				BlockSuffix: "\n",
				Color:       &body,
			},
			Margin: &one,
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: &muted},
			Indent:         &one,
			IndentToken:    &quote,
		},
		List: ansi.StyleList{LevelIndent: two},
		Heading: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			BlockSuffix: "\n",
			Color:       &accent,
			Bold:        &bold,
		}},
		H2:            ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
		H3:            ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
		H4:            ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
		H5:            ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
		H6:            ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
		Strikethrough: ansi.StylePrimitive{CrossedOut: &crossedOut},
		Emph:          ansi.StylePrimitive{Italic: &italic},
		Strong:        ansi.StylePrimitive{Bold: &bold},
		HorizontalRule: ansi.StylePrimitive{
			Color:  &muted,
			Format: "\n────────\n",
		},
		Item:        ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration: ansi.StylePrimitive{BlockPrefix: ". "},
		Task: ansi.StyleTask{
			Ticked:   "[✓] ",
			Unticked: "[ ] ",
		},
		Link: ansi.StylePrimitive{
			Color:     &accent,
			Underline: &underline,
		},
		LinkText: ansi.StylePrimitive{
			Color: &accent,
			Bold:  &bold,
		},
		ImageText: ansi.StylePrimitive{
			Color:  &muted,
			Format: "Image: {{.text}} →",
		},
		Code: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			Prefix:          "\u00a0",
			Suffix:          "\u00a0",
			Color:           &accent,
			BackgroundColor: &codeBackground,
		}},
		CodeBlock: ansi.StyleCodeBlock{StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:           &body,
				BackgroundColor: &codeBackground,
			},
			Margin: &one,
		}},
		Table: ansi.StyleTable{
			CenterSeparator: &center,
			ColumnSeparator: &column,
			RowSeparator:    &row,
		},
		DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n• "},
	}
}

func plainMarkdownStyle() ansi.StyleConfig {
	one := uint(1)
	two := uint(2)
	quote := "| "
	center := "+"
	column := "|"
	row := "-"
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockPrefix: "\n", BlockSuffix: "\n"},
			Margin:         &one,
		},
		BlockQuote: ansi.StyleBlock{Indent: &one, IndentToken: &quote},
		List:       ansi.StyleList{LevelIndent: two},
		Heading:    ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n"}},
		H1:         ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
		H2:         ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
		H3:         ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
		H4:         ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
		H5:         ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
		H6:         ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
		Strikethrough: ansi.StylePrimitive{
			BlockPrefix: "~~",
			BlockSuffix: "~~",
		},
		Emph: ansi.StylePrimitive{
			BlockPrefix: "*",
			BlockSuffix: "*",
		},
		Strong: ansi.StylePrimitive{
			BlockPrefix: "**",
			BlockSuffix: "**",
		},
		HorizontalRule: ansi.StylePrimitive{Format: "\n--------\n"},
		Item:           ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration:    ansi.StylePrimitive{BlockPrefix: ". "},
		Task: ansi.StyleTask{
			Ticked:   "[x] ",
			Unticked: "[ ] ",
		},
		ImageText: ansi.StylePrimitive{Format: "Image: {{.text}} →"},
		Code: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			BlockPrefix: "`",
			BlockSuffix: "`",
		}},
		CodeBlock: ansi.StyleCodeBlock{StyleBlock: ansi.StyleBlock{Margin: &one}},
		Table: ansi.StyleTable{
			CenterSeparator: &center,
			ColumnSeparator: &column,
			RowSeparator:    &row,
		},
		DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n* "},
	}
}
