package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type terminalStyle struct {
	foreground string
	background string
	bold       bool
}

type terminalStyleRun struct {
	text  string
	style terminalStyle
}

type colorProbeFrames struct {
	instructions string
	detailsTop   string
	detailsEnd   string
}

const colorProbeMetadata = "name: color-probe\n" +
	"description: \"value: remains neutral\"\n" +
	"\"quoted:key\": \"quoted:value\"\n" +
	"'single:key': single quoted key value\n" +
	"plain:key: plain colon value\n" +
	"# comment: remains neutral\n" +
	"nested:\n" +
	"  child: nested value\n" +
	"flow: {\"flow:key\": flow value, \"clé:key\": \"café:value\", inner: {deep: \"value: neutral\"}}\n" +
	"literal: |\n" +
	"  scalar: body\n" +
	"  \"quoted: body\"\n" +
	"folded: >\n" +
	"  folded: body\n"

func TestBrowserUsesContentAccentForMarkdownAndDetails(t *testing.T) {
	fx := newFixture(t)
	root := writeColorProbe(t, fx)

	themes := []struct {
		name           string
		theme          string
		noColor        bool
		navigation     string
		foreground     string
		codeBackground string
	}{
		{name: "dark", theme: "dark", navigation: "5;117", foreground: "5;183", codeBackground: "5;53"},
		{name: "light", theme: "light", navigation: "5;25", foreground: "5;56", codeBackground: "5;225"},
		{name: "none", theme: "none"},
		{name: "no-color", theme: "dark", noColor: true},
	}

	frames := make(map[string]colorProbeFrames, len(themes))
	for _, testCase := range themes {
		t.Run(testCase.name, func(t *testing.T) {
			captured := captureColorProbe(t, fx, root, 120, testCase.theme, testCase.noColor, "color-probe", "Lavender heading", "Body neutral marker")
			frames[testCase.name] = captured
			writeEvidence(t, "ale43-color/themes/"+testCase.name+"-instructions.ansi", []byte(captured.instructions))
			writeEvidence(t, "ale43-color/themes/"+testCase.name+"-details-top.ansi", []byte(captured.detailsTop))
			writeEvidence(t, "ale43-color/themes/"+testCase.name+"-details-end.ansi", []byte(captured.detailsEnd))
			if got := metadataFromDetails(t, captured.detailsEnd); got != strings.TrimSuffix(colorProbeMetadata, "\n") {
				t.Fatalf("raw metadata changed during rendering:\ngot:\n%s\nwant:\n%s", got, colorProbeMetadata)
			}

			if testCase.foreground == "" {
				for name, frame := range map[string]string{
					"instructions": captured.instructions,
					"details top":  captured.detailsTop,
					"details end":  captured.detailsEnd,
				} {
					if strings.ContainsRune(frame, '\x1b') {
						t.Fatalf("%s emitted terminal styling: %q", name, frame)
					}
				}
				return
			}

			assertTextStyle(t, captured.instructions, "Lavender heading", testCase.foreground, "", true)
			assertTextStyle(t, captured.instructions, "lavender link", testCase.foreground, "", true)
			assertTextStyle(t, captured.instructions, "lavender code", testCase.foreground, testCase.codeBackground, false)
			assertTextNotForeground(t, captured.instructions, "Body neutral marker", testCase.foreground)
			assertTextStyle(t, captured.instructions, "Sources", testCase.navigation, "", true)

			assertTextStyle(t, captured.detailsTop, "Source ID:", testCase.foreground, "", true)
			assertTextStyle(t, captured.detailsTop, "Reference 1 ID:", testCase.foreground, "", true)
			assertTextStyle(t, captured.detailsTop, "Context:", testCase.foreground, "", true)
			assertTextStyle(t, captured.detailsTop, "References", testCase.foreground, "", true)
			assertTextStyle(t, captured.detailsEnd, "Raw metadata", testCase.foreground, "", true)
			assertTextStyle(t, captured.detailsEnd, "Resources", testCase.foreground, "", true)
			for _, key := range []string{"name", "description", `"quoted:key"`, `'single:key'`, "plain:key", "nested", "child", "flow", `"flow:key"`, `"clé:key"`, "inner", "deep", "literal", "folded"} {
				assertTextStyle(t, captured.detailsEnd, key, testCase.foreground, "", true)
			}
			for _, value := range []string{"color-probe", `"value: remains neutral"`, `"quoted:value"`, "single quoted key value", `"café:value"`, `"value: neutral"`, "comment: remains neutral", "scalar: body", "folded: body"} {
				assertTextNotForeground(t, captured.detailsEnd, value, testCase.foreground)
			}
		})
	}

	if stripTerminalStyles(frames["none"].detailsEnd) != stripTerminalStyles(frames["no-color"].detailsEnd) {
		t.Fatal("NO_COLOR and --theme none produced different Details cells")
	}
}

func TestBrowserContentAccentPreservesMetadataAcrossWidths(t *testing.T) {
	fx := newFixture(t)
	root := writeColorProbe(t, fx)
	for _, width := range []int{48, 80, 120} {
		width := width
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			captured := captureColorProbe(t, fx, root, width, "dark", false, "color-probe", "Lavender heading", "Body neutral marker")
			writeEvidence(t, fmt.Sprintf("ale43-color/widths/%d-details-top.ansi", width), []byte(captured.detailsTop))
			writeEvidence(t, fmt.Sprintf("ale43-color/widths/%d-details-end.ansi", width), []byte(captured.detailsEnd))
			assertPaneCells(t, stripTerminalStyles(captured.detailsTop), width, 30)
			assertPaneCells(t, stripTerminalStyles(captured.detailsEnd), width, 30)
			for _, expected := range []string{`"quoted:key": "quoted:value"`, "plain:key: plain colon value", "scalar: body", "folded: body"} {
				if !paneContains(stripTerminalStyles(captured.detailsEnd), expected) {
					t.Fatalf("%d-column Details lost raw metadata %q:\n%s", width, expected, stripTerminalStyles(captured.detailsEnd))
				}
			}
		})
	}
}

func TestBrowserInvalidMetadataRemainsEscapedAndPlain(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "roots", "invalid-color-probe")
	raw := "---\nname: invalid-color-probe\nbroken: before \x1b]52;c;METADATA_CLIP\a after\n---\nBody remains inert.\n"
	writeFile(t, filepath.Join(root, "probe", "SKILL.md"), []byte(raw), 0o644)

	for _, testCase := range []struct {
		name    string
		theme   string
		noColor bool
	}{
		{name: "dark", theme: "dark"},
		{name: "none", theme: "none"},
		{name: "no-color", theme: "dark", noColor: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			captured := captureColorProbe(t, fx, root, 80, testCase.theme, testCase.noColor, "probe", "Body remains inert")
			writeEvidence(t, "ale43-color/invalid/"+testCase.name+".ansi", []byte(captured.detailsEnd))
			plain := stripTerminalStyles(captured.detailsEnd)
			if !paneContains(plain, `broken: before \x1b]52;c;METADATA_CLIP\a after`) {
				t.Fatalf("invalid raw metadata was not preserved as escaped text:\n%s", plain)
			}
			if strings.Contains(captured.detailsEnd, "\x1b]52;c;METADATA_CLIP") {
				t.Fatalf("invalid raw metadata emitted an active OSC sequence: %q", captured.detailsEnd)
			}
			assertTextNotForeground(t, captured.detailsEnd, "broken:", "5;183")
			if testCase.theme == "none" || testCase.noColor {
				if strings.ContainsRune(captured.detailsEnd, '\x1b') {
					t.Fatalf("unstyled invalid metadata emitted terminal styling: %q", captured.detailsEnd)
				}
			}
		})
	}
}

func writeColorProbe(t *testing.T, fx fixture) string {
	t.Helper()
	root := filepath.Join(fx.base, "allowed", "roots", "color-probe")
	manifest := "---\n" +
		colorProbeMetadata +
		"---\n" +
		"# Lavender heading\n\n" +
		"Body neutral marker.\n\n" +
		"[lavender link](https://example.test/docs) and `lavender code`.\n"
	writeFile(t, filepath.Join(root, "probe", "SKILL.md"), []byte(manifest), 0o644)
	return root
}

func metadataFromDetails(t *testing.T, frame string) string {
	t.Helper()
	lines := strings.Split(stripTerminalStyles(frame), "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == "Raw metadata" {
			start = index + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("Details frame has no Raw metadata section:\n%s", stripTerminalStyles(frame))
	}
	var metadata []string
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == "Resources" {
			break
		}
		line = strings.TrimRight(line, " ")
		if line == "" {
			metadata = append(metadata, "")
			continue
		}
		if !strings.HasPrefix(line, "   ") {
			t.Fatalf("raw metadata line lost its Details indent: %q", line)
		}
		metadata = append(metadata, strings.TrimPrefix(line, "   "))
	}
	for len(metadata) > 0 && metadata[len(metadata)-1] == "" {
		metadata = metadata[:len(metadata)-1]
	}
	return strings.Join(metadata, "\n")
}

func captureColorProbe(
	t *testing.T,
	fx fixture,
	root string,
	width int,
	theme string,
	noColor bool,
	sourceName string,
	instructionNeedles ...string,
) colorProbeFrames {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux is required for color acceptance")
	}
	socket := fmt.Sprintf("a43-color-%d-%x", os.Getpid(), time.Now().UnixNano()&0xffffff)
	target := "browser:0.0"
	commandLine := strings.Join([]string{
		"exec", shellQuote(skillglassBinary), "browse",
		"--root", shellQuote(root),
		"--theme", shellQuote(theme),
	}, " ")
	if noColor {
		commandLine = "NO_COLOR=1 " + commandLine
	}
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", strconv.Itoa(width), "-y", "30",
		"-s", "browser", commandLine,
	)
	t.Cleanup(func() { runTmuxIgnoringError(tmux, fx, socket, "kill-server") })
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Focused:", sourceName, "? help", "q quit")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	readerNeedles := append([]string{"Selected:", "Rendered instructions"}, instructionNeedles...)
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, readerNeedles...)
	instructions := captureStyledPane(t, tmux, fx, socket, target)
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "d")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Details", "Source ID:", "Manifest:")
	detailsTop := captureStyledPane(t, tmux, fx, socket, target)
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "G")
	_ = waitForPaneAt(t, tmux, fx, socket, target, true, "Raw metadata", "Resources")
	detailsEnd := captureStyledPane(t, tmux, fx, socket, target)
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "q")
	return colorProbeFrames{instructions: instructions, detailsTop: detailsTop, detailsEnd: detailsEnd}
}

func assertTextStyle(t *testing.T, frame, text, foreground, background string, bold bool) {
	t.Helper()
	styles := stylesForText(frame, text)
	if len(styles) == 0 {
		t.Fatalf("missing styled text %q in frame:\n%q", text, frame)
	}
	for _, style := range styles {
		if style.foreground == foreground && (background == "" || style.background == background) && (!bold || style.bold) {
			return
		}
	}
	t.Fatalf("text %q styles = %#v, want foreground %s background %s bold %t", text, styles, foreground, background, bold)
}

func assertTextNotForeground(t *testing.T, frame, text, foreground string) {
	t.Helper()
	styles := stylesForText(frame, text)
	if len(styles) == 0 {
		t.Fatalf("missing text %q in frame:\n%q", text, frame)
	}
	for _, style := range styles {
		if style.foreground == foreground {
			t.Fatalf("text %q unexpectedly uses foreground %s in %#v", text, foreground, styles)
		}
	}
}

func stylesForText(frame, text string) []terminalStyle {
	var styles []terminalStyle
	for _, run := range terminalStyleRuns(frame) {
		if strings.Contains(run.text, text) {
			styles = append(styles, run.style)
		}
	}
	return styles
}

func stripTerminalStyles(value string) string {
	var output strings.Builder
	for _, run := range terminalStyleRuns(value) {
		output.WriteString(run.text)
	}
	return output.String()
}

func terminalStyleRuns(value string) []terminalStyleRun {
	style := terminalStyle{}
	var runs []terminalStyleRun
	var text strings.Builder
	flush := func() {
		if text.Len() == 0 {
			return
		}
		runs = append(runs, terminalStyleRun{text: text.String(), style: style})
		text.Reset()
	}
	for index := 0; index < len(value); {
		if value[index] != '\x1b' || index+1 >= len(value) || value[index+1] != '[' {
			text.WriteByte(value[index])
			index++
			continue
		}
		end := index + 2
		for end < len(value) && value[end] != 'm' {
			end++
		}
		if end == len(value) {
			text.WriteString(value[index:])
			break
		}
		flush()
		applyTerminalStyle(&style, value[index+2:end])
		index = end + 1
	}
	flush()
	return runs
}

func applyTerminalStyle(style *terminalStyle, parameters string) {
	if parameters == "" {
		parameters = "0"
	}
	fields := strings.FieldsFunc(parameters, func(char rune) bool { return char == ';' || char == ':' })
	values := make([]int, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	for index := 0; index < len(values); index++ {
		switch values[index] {
		case 0:
			*style = terminalStyle{}
		case 1:
			style.bold = true
		case 22:
			style.bold = false
		case 38:
			if index+4 < len(values) && values[index+1] == 2 {
				style.foreground = fmt.Sprintf("%d;%d;%d", values[index+2], values[index+3], values[index+4])
				index += 4
			} else if index+2 < len(values) && values[index+1] == 5 {
				style.foreground = fmt.Sprintf("5;%d", values[index+2])
				index += 2
			}
		case 39:
			style.foreground = ""
		case 48:
			if index+4 < len(values) && values[index+1] == 2 {
				style.background = fmt.Sprintf("%d;%d;%d", values[index+2], values[index+3], values[index+4])
				index += 4
			} else if index+2 < len(values) && values[index+1] == 5 {
				style.background = fmt.Sprintf("5;%d", values[index+2])
				index += 2
			}
		case 49:
			style.background = ""
		}
	}
}
