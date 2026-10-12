package terminal

import "testing"

func TestResolveBrowserThemeAndBackgroundHint(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "15;0")
	theme, err := resolveBrowserTheme("auto")
	if err != nil || theme != themeDark {
		t.Fatalf("dark auto theme = %v, %v", theme, err)
	}
	t.Setenv("COLORFGBG", "0;15")
	theme, err = resolveBrowserTheme("auto")
	if err != nil || theme != themeLight {
		t.Fatalf("light auto theme = %v, %v", theme, err)
	}
	t.Setenv("NO_COLOR", "1")
	theme, err = resolveBrowserTheme("dark")
	if err != nil || theme != themeNone {
		t.Fatalf("NO_COLOR theme = %v, %v", theme, err)
	}
}

func TestBrowserThemesKeepNavigationBlueAndUseLavenderForContent(t *testing.T) {
	tests := []struct {
		name           string
		theme          browserTheme
		navigation     string
		content        string
		warning        string
		danger         string
		markdown       string
		codeBackground string
	}{
		{
			name:           "dark",
			theme:          themeDark,
			navigation:     "38;2;147;197;253",
			content:        "38;2;196;181;253",
			warning:        "38;2;246;193;119",
			danger:         "38;2;248;113;113",
			markdown:       "#C4B5FD",
			codeBackground: "#342B47",
		},
		{
			name:           "light",
			theme:          themeLight,
			navigation:     "38;2;3;105;161",
			content:        "38;2;109;40;217",
			warning:        "38;2;146;64;14",
			danger:         "38;2;185;28;28",
			markdown:       "#6D28D9",
			codeBackground: "#EFE8FF",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			palette := paletteFor(test.theme)
			if palette.accent != test.navigation {
				t.Fatalf("navigation accent = %q, want %q", palette.accent, test.navigation)
			}
			if palette.contentAccent != test.content {
				t.Fatalf("content accent = %q, want %q", palette.contentAccent, test.content)
			}
			if palette.warning != test.warning || palette.danger != test.danger {
				t.Fatalf("warning/danger = %q/%q, want %q/%q", palette.warning, palette.danger, test.warning, test.danger)
			}

			style := test.theme.markdownStyle()
			for name, color := range map[string]*string{
				"heading":     style.Heading.Color,
				"link":        style.Link.Color,
				"link text":   style.LinkText.Color,
				"inline code": style.Code.Color,
			} {
				if color == nil || *color != test.markdown {
					t.Fatalf("%s color = %v, want %q", name, color, test.markdown)
				}
			}
			if style.Code.BackgroundColor == nil || *style.Code.BackgroundColor != test.codeBackground {
				t.Fatalf("inline code background = %v, want %q", style.Code.BackgroundColor, test.codeBackground)
			}
			if style.CodeBlock.BackgroundColor == nil || *style.CodeBlock.BackgroundColor != test.codeBackground {
				t.Fatalf("code block background = %v, want %q", style.CodeBlock.BackgroundColor, test.codeBackground)
			}
		})
	}
}
