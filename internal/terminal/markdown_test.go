package terminal

import (
	"strings"
	"testing"
)

func TestRenderMarkdownNeutralizesDecodedTerminalControls(t *testing.T) {
	input := "before &#27;]52;c;SECRETDATA&#7; after &#13; [link](https://example.test/path)"
	rendered := strings.Join(renderMarkdownLines(input, 120, themeDark), "\n")
	for _, expected := range []string{`\x1b]52;c;SECRETDATA\a`, `\r`, "link", "https://example.test/path"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered Markdown missing %q: %q", expected, rendered)
		}
	}
	for _, forbidden := range []string{"\x1b]52", "\x1b]8", "\a", "\r"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered Markdown contains active control %q: %q", forbidden, rendered)
		}
	}
}

func TestRenderMarkdownKeepsCommonStructureLegible(t *testing.T) {
	input := "# Heading\n\n> quoted **text**\n\n- parent\n  - nested\n\n```go\nfmt.Println(\"safe\")\n```"
	rendered := strings.Join(renderMarkdownLines(input, 60, themeNone), "\n")
	for _, expected := range []string{"Heading", "quoted", "text", "parent", "nested", "fmt.Println"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered Markdown missing %q: %q", expected, rendered)
		}
	}
	if strings.ContainsRune(rendered, '\x1b') {
		t.Fatalf("no-color Markdown contains ANSI: %q", rendered)
	}
}
