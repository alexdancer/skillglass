package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
)

func TestBrowserPTYJourney(t *testing.T) {
	fx := newFixture(t)
	before := treeManifest(t, fx.base)
	list := fx.run(
		t,
		"list",
		"--root",
		fx.cleanRoot,
		"--search",
		"description-only-literal",
		"--json",
	)
	assertExit(t, list, 0)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	normal := sourceNamed(t, inventory.Sources, "normal-skill")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		skillglassBinary,
		"browse",
		"--root",
		fx.cleanRoot,
		"--project",
		fx.project,
	)
	command.Dir = fx.project
	command.Env = fx.environment()
	started := time.Now()
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	var raw terminalTranscript
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&raw, terminal)
		close(copyDone)
	}()

	keys := []string{
		"enter",
		"G",
		"d",
		"G",
		"esc",
		"/",
		"definitely-no-match",
		"enter",
		"q",
	}
	raw.wait(t, 0, "Focused: normal-skill", "Focus: 1/2", "normal-skill", "Enter open")
	postconditions := []string{"Rendered instructions", "LAST_INSTRUCTION_SENTINEL", "Details", "scripts/never-run.sh", "Enter open", "Type to filter", "No matching sources.", "Enter open", ""}
	for index, key := range keys {
		checkpoint := len(raw.Bytes())
		payload := key
		if key == "enter" {
			payload = "\r"
		}
		if key == "esc" {
			payload = "\x1b"
		}
		if _, err := io.WriteString(terminal, payload); err != nil {
			t.Fatalf("write %q: %v", key, err)
		}
		if postconditions[index] != "" {
			raw.wait(t, checkpoint, postconditions[index])
		}
	}
	err = command.Wait()
	<-copyDone
	_ = terminal.Close()
	if ctx.Err() != nil {
		t.Fatal("browser PTY journey timed out")
	}
	if err != nil {
		t.Fatalf("browser exit: %v\nraw:\n%q", err, raw.Bytes())
	}
	for _, restored := range []string{"\x1b[?1049l", "\x1b[?25h"} {
		if !bytes.Contains(raw.Bytes(), []byte(restored)) {
			t.Fatalf("PTY did not restore terminal mode %q", restored)
		}
	}
	visibleOutput := ansi.Strip(string(raw.Bytes()))
	for _, expected := range []string{
		"coverage complete",
		"normal-skill",
		normal.ID,
		"description-only-literal",
		"Manifest: parsed",
		"LAST_INSTRUCTION_SENTINEL",
		"Resources",
		"scripts/never-run.sh",
		"executable",
		"0 matches | selected source unchanged",
	} {
		if !strings.Contains(visibleOutput, expected) {
			t.Fatalf("PTY output missing %q", expected)
		}
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("PTY browser journey mutated its fixture")
	}
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatalf("browser executed fixture script: %v", err)
	}
	writeEvidence(t, "pty/raw.ansi", raw.Bytes())
	writeEvidence(t, "pty/keys.txt", []byte(strings.Join(keys, "\n")+"\n"))
	writeEvidence(t, "pty/dimensions.txt", []byte("80x24\n"))
	writeEvidence(t, "pty/assertions.txt", []byte(
		"selector/name/description parity: pass\n"+
			"scroll to final instruction and resources: pass\n"+
			"search miss preserves selection: pass\n"+
			"fixture unchanged and script inert: pass\n",
	))
	recordEvidenceCommand(
		t,
		"browser-pty",
		skillglassBinary,
		[]string{"browse", "--root", fx.cleanRoot, "--project", fx.project},
		fx.project,
		commandResult{stdout: raw.Bytes(), duration: time.Since(started)},
	)
}

func TestBrowserTmuxCellsAtRequiredWidths(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	before := treeManifest(t, fx.base)
	list := fx.run(t, "list", "--root", fx.cleanRoot, "--project", fx.project, "--search", "description-only-literal", "--json")
	assertExit(t, list, 0)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	normal := sourceNamed(t, inventory.Sources, "normal-skill")
	if len(inventory.Sources) != 1 {
		t.Fatalf("description search returned %d sources, want 1", len(inventory.Sources))
	}
	writeEvidence(t, "tmux/description-search-list.json", list.stdout)
	for _, width := range []int{48, 60, 80, 96, 120} {
		width := width
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			socket := fmt.Sprintf("skillglass-acceptance-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			commandLine := strings.Join([]string{
				"exec",
				shellQuote(skillglassBinary),
				"browse",
				"--root",
				shellQuote(fx.cleanRoot),
				"--project",
				shellQuote(fx.project),
			}, " ")
			runTmux(t, tmux, fx, socket,
				"new-session", "-d", "-x", strconv.Itoa(width), "-y", "30",
				"-s", "browser", commandLine,
			)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket,
				"resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30",
			)

			initial := waitForPane(
				t, tmux, fx, socket, false, "Skillglass", "Focused: normal-skill", "normal-skill", "Focus: 1/2", "Scope: 1 root",
			)
			assertPaneCells(t, initial, width, 30)
			if width >= 96 {
				for _, expected := range []string{"normal-skill", "second-skill", "FIRST_INSTRUCTION_SENTINEL"} {
					if !paneContains(initial, expected) {
						t.Fatalf("wide inventory is missing %q from list plus preview:\n%s", expected, initial)
					}
				}
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/00-inventory.txt", width), []byte(initial))
			if width == 80 {
				if !strings.Contains(initial, "description-only-literal") {
					t.Fatalf("initial inventory did not expose the pre-satisfied description predicate:\n%s", initial)
				}
				scope := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "s")
				}, "Scope and filters", "Root 1 [searched]", "← back")
				assertPaneCells(t, scope, 80, 30)
				writeEvidence(t, "tmux/80/00-scope.txt", []byte(scope))
				resizedScope := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 63, 20, "Scope and filters", "Root 1 [searched]", "← back")
				assertPaneCells(t, resizedScope, 63, 20)
				writeEvidence(t, "tmux/80/00-scope-resized-63x20.txt", []byte(resizedScope))
				restoredScope := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 80, 30, "Scope and filters", "Root 1 [searched]", "← back")
				writeEvidence(t, "tmux/80/00-scope-restored-80x30.txt", []byte(restoredScope))
				_ = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Escape")
				}, "Focused: normal-skill", "normal-skill", "Search / type to filter")
			}

			searchTail := "description-only-literal"
			if width == 96 {
				searchTail = "only-literal"
			}
			search := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "/")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "description-only-literal")
			}, searchTail+"█", normal.Name, "Enter keep")
			assertSearchViewport(t, search, width, 30, searchTail, true, width == 96)
			if strings.Contains(search, "second-skill") {
				t.Fatalf("description search retained a nonmatching inventory row:\n%s", search)
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/01-search.txt", width), []byte(search))

			_ = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
			}, "description-only-literal", "Focus: 1/1", normal.Name, "? help")
			detail := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
			}, "Selected: normal-skill", "Rendered instructions", "FIRST_INSTRUCTION_SENTINEL")
			assertPaneCells(t, detail, width, 30)
			if width == 80 {
				for index := 0; index < 8; index++ {
					expected := fmt.Sprintf("instruction line %02d", index)
					if !paneContains(detail, expected) {
						t.Fatalf("80x30 reader is missing unscrolled body marker %q:\n%s", expected, detail)
					}
				}
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/02-instructions.txt", width), []byte(detail))

			details := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "d")
			}, "Details", "Manifest: parsed", "Source ID: "+normal.ID)
			assertPaneCells(t, details, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/%d/03-details.txt", width), []byte(details))

			endJoined := actForPaneAt(t, tmux, fx, socket, "browser:0.0", true, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "G")
			}, "scripts/never-run.sh", "executable", "q quit")
			end := capturePane(t, tmux, fx, socket, false)
			assertPaneCells(t, end, width, 30)
			backHint := "← back"
			if width < 60 {
				backHint = "Esc back"
			}
			if !strings.Contains(endJoined, backHint) {
				t.Fatalf("joined end capture does not expose complete footer %q:\n%s", backHint, endJoined)
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/04-end.txt", width), []byte(end))
			writeEvidence(t, fmt.Sprintf("tmux/%d/04-end-joined.txt", width), []byte(endJoined))
			dimensions := strings.TrimSpace(runTmux(
				t, tmux, fx, socket,
				"display-message", "-p", "-t", "browser:0.0", "#{pane_width}x#{pane_height}",
			))
			if dimensions != fmt.Sprintf("%dx30", width) {
				t.Fatalf("pane dimensions = %q", dimensions)
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/dimensions.txt", width), []byte(dimensions+"\n"))
			writeEvidence(t, fmt.Sprintf("tmux/%d/keys.txt", width), []byte("/\ndescription-only-literal\nEnter\nEnter\nd\nG\nq\n"))
			writeEvidence(t, fmt.Sprintf("tmux/%d/command.txt", width), []byte(commandLine+"\n"))
			title := strings.TrimSpace(runTmux(
				t, tmux, fx, socket,
				"display-message", "-p", "-t", "browser:0.0", "#{pane_title}",
			))
			cursor := strings.TrimSpace(runTmux(
				t, tmux, fx, socket,
				"display-message", "-p", "-t", "browser:0.0", "#{cursor_x},#{cursor_y}",
			))
			cursorFlag := strings.TrimSpace(runTmux(
				t, tmux, fx, socket,
				"display-message", "-p", "-t", "browser:0.0", "#{cursor_flag}",
			))
			if cursorFlag != "0" {
				t.Fatalf("browser cursor is visible: title=%q cursor=%q flag=%q", title, cursor, cursorFlag)
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/title.txt", width), []byte(title+"\n"))
			writeEvidence(t, fmt.Sprintf("tmux/%d/cursor.txt", width), []byte(cursor+" hidden\n"))
			if width == 80 {
				resized := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 63, 20, "Selected: normal-skill", "Details", "scripts/never-run.sh", "q quit")
				assertPaneCells(t, resized, 63, 20)
				writeEvidence(t, "tmux/80/04-resized-63x20.txt", []byte(resized))
				restored := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 80, 30, "Selected: normal-skill", "Details", "scripts/never-run.sh", "q quit")
				assertPaneCells(t, restored, 80, 30)
				writeEvidence(t, "tmux/80/05-resized-restored.txt", []byte(restored))
			}

			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "q")
		})
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("tmux browser journeys mutated their fixture")
	}
	if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
		t.Fatalf("browser executed fixture script: %v", err)
	}
}

func TestBrowserHelpIsScrollableAt48x20(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	before := treeManifest(t, fx.base)
	socket := fmt.Sprintf("skillglass-help-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{
		"exec", shellQuote(skillglassBinary), "browse",
		"--root", shellQuote(fx.cleanRoot),
		"--project", shellQuote(fx.project),
		"--theme", "none",
	}, " ")
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", "48", "-y", "20", "-s", "help", commandLine,
	)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "help:0", "-x", "48", "-y", "20")
	target := "help:0.0"
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Focused: normal-skill", "? help", "q quit")
	top := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "?")
	}, "Keys", "Move selection or scroll", "Open the focused source or problem; keep search", "Esc/←/? close", "q quit")
	end := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "G")
	}, "g / G", "Move to the top or end", "?", "Toggle this help", "q", "Quit",
		"Themes: browse --theme auto|dark|light|none",
		"NO_COLOR forces no-color output. No theme writes settings.")
	if top == end {
		t.Fatal("G did not expose the end of compact help")
	}
	restored := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
	}, "Focused: normal-skill", "Search / type to filter", "? help", "q quit")
	assertPaneCells(t, top, 48, 20)
	assertPaneCells(t, end, 48, 20)
	assertPaneCells(t, restored, 48, 20)
	writeEvidence(t, "tmux/help-48x20/00-top.txt", []byte(top))
	writeEvidence(t, "tmux/help-48x20/01-end.txt", []byte(end))
	writeEvidence(t, "tmux/help-48x20/02-restored.txt", []byte(restored))
	writeEvidence(t, "tmux/help-48x20/command.txt", []byte(commandLine+"\n"))
	writeEvidence(t, "tmux/help-48x20/keys.txt", []byte("?\nG\nEsc\nq\n"))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "q")
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("help journey mutated its fixture")
	}
}

func TestBrowserDisambiguatesEqualSourcesUnderSameBasenameRoots(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	rootOne := filepath.Join(fx.base, "allowed", "same-basename", "one", "skills")
	rootTwo := filepath.Join(fx.base, "allowed", "same-basename", "two", "skills")
	manifest := []byte("---\nname: same-basename\ndescription: equal description\n---\nEQUAL_INSTRUCTIONS\n")
	writeFile(t, filepath.Join(rootOne, "copy", "SKILL.md"), manifest, 0o644)
	writeFile(t, filepath.Join(rootTwo, "copy", "SKILL.md"), manifest, 0o644)
	before := treeManifest(t, fx.base)

	socket := fmt.Sprintf("skillglass-same-basename-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{
		"exec", shellQuote(skillglassBinary), "browse",
		"--root", shellQuote(rootOne),
		"--root", shellQuote(rootTwo),
		"--theme", "none",
	}, " ")
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", "120", "-y", "30", "-s", "same-basename", commandLine,
	)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "same-basename:0", "-x", "120", "-y", "30")
	target := "same-basename:0.0"
	initial := waitForPaneAt(t, tmux, fx, socket, target, false,
		"Focused: same-basename [", "custom · #1 skills", "custom · #2 skills", "EQUAL_INSTRUCTIONS",
	)
	firstFocused := frameLineWithPrefix(t, initial, "Focused: ")
	firstHint := identityHint(firstFocused)
	if firstHint == "" {
		t.Fatalf("focused source lacks an identity hint:\n%s", initial)
	}
	firstOrdinal := "#1"
	secondOrdinal := "#2"
	if strings.Contains(firstFocused, "#2 skills") {
		firstOrdinal, secondOrdinal = secondOrdinal, firstOrdinal
	}
	compact := resizePaneAt(t, tmux, fx, socket, target, 48, 20,
		"Focus: 1/2", "Focused:", "custom · #1 skills", "custom · #2 skills",
	)
	compactFocused := frameLineWithPrefix(t, compact, "Focused: ")
	if identityHint(compactFocused) != firstHint || !strings.Contains(compactFocused, firstOrdinal) {
		t.Fatalf("resize lost focused identity or root ordinal:\n%s", compact)
	}
	selectedSibling := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Down")
	}, "Focus: 2/2", "Focused:", "custom · #1 skills", "custom · #2 skills")
	secondFocused := frameLineWithPrefix(t, selectedSibling, "Focused: ")
	secondHint := identityHint(secondFocused)
	if secondHint == "" || secondHint == firstHint || !strings.Contains(secondFocused, secondOrdinal) {
		t.Fatalf("sibling focus lost distinct identity or root ordinal:\n%s", selectedSibling)
	}
	reader := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	}, "Selected:", "EQUAL_INSTRUCTIONS", "Details")
	selectedLabel := frameLineWithPrefix(t, reader, "Selected: ")
	if identityHint(selectedLabel) != secondHint || !strings.Contains(selectedLabel, secondOrdinal) {
		t.Fatalf("reader lost selected identity or root ordinal:\n%s", reader)
	}
	assertPaneCells(t, initial, 120, 30)
	assertPaneCells(t, compact, 48, 20)
	assertPaneCells(t, selectedSibling, 48, 20)
	assertPaneCells(t, reader, 48, 20)
	writeEvidence(t, "tmux/same-basename/00-wide.txt", []byte(initial))
	writeEvidence(t, "tmux/same-basename/01-compact.txt", []byte(compact))
	writeEvidence(t, "tmux/same-basename/02-sibling.txt", []byte(selectedSibling))
	writeEvidence(t, "tmux/same-basename/03-reader.txt", []byte(reader))
	writeEvidence(t, "tmux/same-basename/command.txt", []byte(commandLine+"\n"))
	writeEvidence(t, "tmux/same-basename/keys.txt", []byte("resize 48x20\nDown\nEnter\nq\n"))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "q")
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("same-basename browser journey mutated its fixture")
	}
}

func TestBrowserDisambiguatesEqualSourcesWithinOneRoot(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "same-root", "skills")
	manifest := []byte("---\nname: same-root\ndescription: equal description\n---\nEQUAL_INSTRUCTIONS\n")
	writeFile(t, filepath.Join(root, "first-bundle", "SKILL.md"), manifest, 0o644)
	writeFile(t, filepath.Join(root, "second-bundle", "SKILL.md"), manifest, 0o644)
	before := treeManifest(t, fx.base)

	socket := fmt.Sprintf("skillglass-same-root-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{
		"exec", shellQuote(skillglassBinary), "browse",
		"--root", shellQuote(root),
		"--theme", "none",
	}, " ")
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", "120", "-y", "30", "-s", "same-root", commandLine,
	)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "same-root:0", "-x", "120", "-y", "30")
	target := "same-root:0.0"
	initial := waitForPaneAt(t, tmux, fx, socket, target, false,
		"Focus: 1/2", "Focused: same-root [", "custom · skills", "EQUAL_INSTRUCTIONS",
	)
	focusedLabel := func(frame string) string {
		const prefix = "Focused: "
		start := strings.Index(frame, prefix)
		if start < 0 {
			t.Fatalf("missing focused label in frame:\n%s", frame)
		}
		start += len(prefix)
		end := strings.Index(frame[start:], " @ ")
		if end < 0 {
			t.Fatalf("missing provenance after focused label in frame:\n%s", frame)
		}
		return frame[start : start+end]
	}
	first := focusedLabel(initial)
	if !strings.Contains(first, "same-root [") {
		t.Fatalf("first source lacks identity hint: %q", first)
	}
	selectedSibling := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Down")
	}, "Focus: 2/2", "Focused: same-root [", "custom · skills", "EQUAL_INSTRUCTIONS")
	second := focusedLabel(selectedSibling)
	if second == first {
		t.Fatalf("independent sources have the same visible label %q", first)
	}
	if !strings.Contains(initial, second) {
		t.Fatalf("wide inventory does not expose sibling label %q:\n%s", second, initial)
	}
	compact := resizePaneAt(t, tmux, fx, socket, target, 48, 20,
		"Focus: 2/2", "Focused: "+second+" @ custom · skills", first, second,
	)
	reader := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	}, "Selected: "+second+" @ custom · skills", second, "EQUAL_INSTRUCTIONS", "Details")
	assertPaneCells(t, initial, 120, 30)
	assertPaneCells(t, selectedSibling, 120, 30)
	assertPaneCells(t, compact, 48, 20)
	assertPaneCells(t, reader, 48, 20)
	writeEvidence(t, "tmux/same-root/00-wide.txt", []byte(initial))
	writeEvidence(t, "tmux/same-root/01-sibling.txt", []byte(selectedSibling))
	writeEvidence(t, "tmux/same-root/02-compact.txt", []byte(compact))
	writeEvidence(t, "tmux/same-root/03-reader.txt", []byte(reader))
	writeEvidence(t, "tmux/same-root/command.txt", []byte(commandLine+"\n"))
	writeEvidence(t, "tmux/same-root/keys.txt", []byte("Down\nresize 48x20\nEnter\nq\n"))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "q")
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("same-root browser journey mutated its fixture")
	}
}

func TestBrowserThemeModesApplyDeterministically(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	type themeCase struct {
		name       string
		theme      string
		noColor    bool
		wantStyles bool
	}
	cases := []themeCase{
		{name: "dark", theme: "dark", wantStyles: true},
		{name: "light", theme: "light", wantStyles: true},
		{name: "none", theme: "none"},
		{name: "no-color", theme: "dark", noColor: true},
	}
	frames := make(map[string]string, len(cases))
	for _, testCase := range cases {
		socket := fmt.Sprintf("skillglass-theme-%s-%d", testCase.name, os.Getpid())
		command := strings.Join([]string{
			"exec", shellQuote(skillglassBinary), "browse",
			"--root", shellQuote(fx.cleanRoot),
			"--project", shellQuote(fx.project),
			"--theme", testCase.theme,
		}, " ")
		if testCase.noColor {
			command = "NO_COLOR=1 " + command
		}
		runTmux(t, tmux, fx, socket,
			"new-session", "-d", "-x", "120", "-y", "30", "-s", "theme", command,
		)
		runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
		runTmux(t, tmux, fx, socket, "resize-window", "-t", "theme:0", "-x", "120", "-y", "30")
		target := "theme:0.0"
		_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Focused: normal-skill", "FIRST_INSTRUCTION_SENTINEL")
		ansiFrame := runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-e", "-t", target)
		hasStyles := strings.Contains(ansiFrame, "\x1b[")
		if hasStyles != testCase.wantStyles {
			t.Fatalf("theme %s styled capture = %t, want %t:\n%q", testCase.name, hasStyles, testCase.wantStyles, ansiFrame)
		}
		frames[testCase.name] = ansiFrame
		writeEvidence(t, "tmux/themes/"+testCase.name+".ansi", []byte(ansiFrame))
		writeEvidence(t, "tmux/themes/"+testCase.name+"-command.txt", []byte(command+"\n"))
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "q")
		runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	}
	if frames["dark"] == frames["light"] {
		t.Fatal("dark and light themes emitted identical styled cells")
	}
	if frames["none"] != frames["no-color"] {
		t.Fatal("NO_COLOR did not produce the same unstyled cells as --theme none")
	}
}

func TestBrowserManyRootsHasScrollableScopeAt60Columns(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	longProject := filepath.Join(
		fx.base,
		"allowed",
		"projects",
		"project-context-with-a-deliberately-long-name-and-more-context-for-terminal-proof",
	)
	if err := os.MkdirAll(longProject, 0o755); err != nil {
		t.Fatal(err)
	}
	roots := make([]string, 0, 8)
	for index := 1; index <= 8; index++ {
		root := filepath.Join(
			fx.base,
			"allowed",
			"roots",
			fmt.Sprintf("selected-root-%02d-with-a-deliberately-long-name-for-scope-proof", index),
		)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
	}
	bundle := filepath.Join(roots[0], "many-root-source")
	writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte(
		"---\nname: many-root-source\ndescription: source remains visible\n---\nMANY_ROOT_START\n\n"+
			strings.Repeat("A separate instruction paragraph.\n\n", 50)+"MANY_ROOT_INSTRUCTIONS\n",
	), 0o644)

	socket := fmt.Sprintf("skillglass-many-roots-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	parts := []string{
		"exec",
		shellQuote(skillglassBinary),
		"browse",
		"--project",
		shellQuote(longProject),
	}
	for _, root := range roots {
		parts = append(parts, "--root", shellQuote(root))
	}
	commandLine := strings.Join(parts, " ")
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", "60", "-y", "30", "-s", "many", commandLine,
	)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "many:0", "-x", "60", "-y", "30")

	initial := waitForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"many:0.0",
		false,
		"many-root-source",
		"Focus: 1/1",
		"Scope: 8 roots",
		"? help",
		"q quit",
	)
	assertPaneCells(t, initial, 60, 30)
	writeEvidence(t, "tmux/many-roots/00-inventory.txt", []byte(initial))
	searchS := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "/")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s")
	}, "Search / s", "Type to filter")
	if strings.Contains(searchS, "Scope and filters") {
		t.Fatal("s opened scope while search input was active")
	}
	writeEvidence(t, "tmux/many-roots/00-search-s.txt", []byte(searchS))
	_ = actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	}, "many-root-source", "Focus: 1/1", "? help")

	canonicalFirst := canonicalPath(t, roots[0])
	scopeTop := actForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"many:0.0",
		false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s") },
		"Scope and filters",
		longProject,
		roots[0],
		canonicalFirst,
		"Root 1 [searched]",
		"← back",
	)
	assertPaneCells(t, scopeTop, 60, 30)
	writeEvidence(t, "tmux/many-roots/01-scope-top.txt", []byte(scopeTop))

	canonicalLast := canonicalPath(t, roots[len(roots)-1])
	scopeEnd := actForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"many:0.0",
		false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "G") },
		"Root 8 [empty]",
		roots[len(roots)-1],
		canonicalLast,
		"← back",
		"q quit",
	)
	assertPaneCells(t, scopeEnd, 60, 30)
	writeEvidence(t, "tmux/many-roots/02-scope-end.txt", []byte(scopeEnd))

	restoredInventory := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	}, "many-root-source", "Focus: 1/1", "? help")
	assertPaneCells(t, restoredInventory, 60, 30)
	writeEvidence(t, "tmux/many-roots/03-inventory-restored.txt", []byte(restoredInventory))

	detail := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Enter")
	}, "Selected: many-root-source", "Rendered instructions", "MANY_ROOT_START")
	assertPaneCells(t, detail, 60, 30)
	writeEvidence(t, "tmux/many-roots/04-detail.txt", []byte(detail))
	detailLineCount := 0
	for _, line := range strings.Split(detail, "\n") {
		var first, last, total int
		status := strings.TrimSpace(line)
		if offset := strings.Index(status, "Read lines "); offset >= 0 {
			if _, err := fmt.Sscanf(status[offset:], "Read lines %d-%d of %d", &first, &last, &total); err != nil {
				continue
			}
			detailLineCount = total
			break
		}
	}
	if detailLineCount == 0 {
		t.Fatalf("detail frame does not report its line count:\n%s", detail)
	}
	detailEndStatus := fmt.Sprintf("-%d of %d", detailLineCount, detailLineCount)
	detailEnd := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "G")
	}, "MANY_ROOT_INSTRUCTIONS", detailEndStatus)
	if detailEnd == detail || paneContains(detailEnd, "MANY_ROOT_START") {
		t.Fatalf("reader did not reach a distinct nonzero-scroll viewport:\n%s", detailEnd)
	}
	assertPaneCells(t, detailEnd, 60, 30)
	writeEvidence(t, "tmux/many-roots/04-detail-end.txt", []byte(detailEnd))
	_ = actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s")
	}, "Scope and filters", longProject)
	restoredDetail := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	}, "MANY_ROOT_INSTRUCTIONS", detailEndStatus, "? help")
	assertPaneCells(t, restoredDetail, 60, 30)
	if restoredDetail != detailEnd {
		t.Fatalf("scope round trip changed detail viewport\nbefore:\n%s\nafter:\n%s", detailEnd, restoredDetail)
	}
	writeEvidence(t, "tmux/many-roots/05-detail-restored.txt", []byte(restoredDetail))
	writeEvidence(t, "tmux/many-roots/keys.txt", []byte("/\ns\nEsc\ns\nG\nEsc\nEnter\nG\ns\nEsc\nq\n"))
	writeEvidence(t, "tmux/many-roots/command.txt", []byte(commandLine+"\n"))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "q")
}

func TestBrowserLongSearchUsesSingleRowTailViewport(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "roots", "long-search")
	asciiTail := "ASCII_TAIL_987654321"
	longQuery := strings.Repeat("x", 1800-len(asciiTail)) + asciiTail
	visibleSuffix := longQuery[len(longQuery)-40:]
	writeFile(t, filepath.Join(root, "tail-only", "SKILL.md"), []byte(
		"---\nname: tail-only-source\ndescription: "+strings.Repeat("x", 80)+asciiTail+"\n---\nTAIL_ONLY\n",
	), 0o644)
	writeFile(t, filepath.Join(root, "other", "SKILL.md"), []byte(
		"---\nname: other-source\ndescription: unrelated\n---\nOTHER\n",
	), 0o644)

	control := fx.run(t, "list", "--root", root, "--search", visibleSuffix, "--json")
	assertExit(t, control, 0)
	var controlInventory publicList
	decodeJSON(t, control.stdout, &controlInventory)
	if len(controlInventory.Sources) != 1 || controlInventory.Sources[0].Name != "tail-only-source" {
		t.Fatalf("visible suffix control = %s", control.stdout)
	}

	socket := fmt.Sprintf("skillglass-long-search-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{
		"exec",
		shellQuote(skillglassBinary),
		"browse",
		"--root",
		shellQuote(root),
		"--project",
		shellQuote(fx.project),
	}, " ")
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", "60", "-y", "30", "-s", "long", commandLine,
	)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "long:0", "-x", "60", "-y", "30")
	_ = waitForPaneAt(t, tmux, fx, socket, "long:0.0", false, "Focus: 1/2", "Search /")
	writeEvidence(t, "tmux/long-search/command.txt", []byte(commandLine+"\n"))

	longEditing := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "/")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", longQuery)
		},
		asciiTail+"█", "No matching sources.", "Focused: no matching sources",
		"Type to filter", "Enter keep", "Esc cancel", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/01-long-editing-60x30.txt", []byte(longEditing))
	longRow := assertSearchViewport(t, longEditing, 60, 30, asciiTail, true, true)
	assertSearchChrome(t, longEditing)

	narrow := resizePaneAt(
		t, tmux, fx, socket, "long:0.0", 48, 20,
		asciiTail+"█", "No matching sources.", "Focused: no matching sources",
		"Type  Enter keep", "Esc cancel", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/02-long-editing-48x20.txt", []byte(narrow))
	narrowRow := assertSearchViewport(t, narrow, 48, 20, asciiTail, true, true)
	assertSearchChrome(t, narrow)
	if narrowRow == longRow {
		t.Fatalf("search viewport did not change after resize: %q", narrowRow)
	}

	restored := resizePaneAt(
		t, tmux, fx, socket, "long:0.0", 60, 30,
		longRow, "No matching sources.", "Focused: no matching sources",
		"Type to filter", "Enter keep", "Esc cancel", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/03-long-editing-restored.txt", []byte(restored))
	if restoredRow := assertSearchViewport(t, restored, 60, 30, asciiTail, true, true); restoredRow != longRow {
		t.Fatalf("restored search row = %q, want %q", restoredRow, longRow)
	}
	assertSearchChrome(t, restored)

	kept := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "Enter") },
		asciiTail, "No matching sources.", "0 matches", "? help", "q quit",
	)
	writeEvidence(t, "tmux/long-search/04-long-kept.txt", []byte(kept))
	assertSearchViewport(t, kept, 60, 30, asciiTail, false, true)

	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "/")
	}, asciiTail+"█", "Type to filter")
	cancelTail := "CANCEL_TAIL"
	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", cancelTail)
	}, cancelTail+"█", "Type to filter")
	cancelled := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "Escape") },
		asciiTail, "No matching sources.", "0 matches", "? help", "q quit",
	)
	writeEvidence(t, "tmux/long-search/05-long-cancelled.txt", []byte(cancelled))
	assertSearchViewport(t, cancelled, 60, 30, asciiTail, false, true)
	if strings.Contains(cancelled, cancelTail) {
		t.Fatal("Escape retained text appended after queryBefore")
	}

	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "/")
	}, asciiTail+"█", "Type to filter")
	cleared := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "C-u") },
		"Search / █", "tail-only-source", "other-source", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/06-cleared.txt", []byte(cleared))
	assertSearchViewport(t, cleared, 60, 30, "", true, false)

	wideTail := "終端尾"
	wideQuery := strings.Repeat("界", 120) + wideTail
	wide := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", wideQuery) },
		wideTail+"█", "No matching sources.", "Focused: no matching sources", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/07-wide-editing.txt", []byte(wide))
	assertSearchViewport(t, wide, 60, 30, wideTail, true, true)
	assertSearchChrome(t, wide)

	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", "Z")
	}, wideTail+"Z█")
	backspaced := actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "BSpace")
	}, wideTail+"█")
	writeEvidence(t, "tmux/long-search/08-wide-backspaced.txt", []byte(backspaced))
	assertSearchViewport(t, backspaced, 60, 30, wideTail, true, true)

	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "C-u")
	}, "Search / █", "tail-only-source", "other-source")
	clearKept := actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "Enter")
	}, "Search /", "Focus: 1/2", "? help", "q quit")
	writeEvidence(t, "tmux/long-search/09-clear-kept.txt", []byte(clearKept))
	assertSearchViewport(t, clearKept, 60, 30, "", false, false)
	writeEvidence(t, "tmux/long-search/keys.txt", []byte(
		"/\n<1800-character-query>\nresize 48x20\nresize 60x30\nEnter\n/\nCANCEL_TAIL\nEsc\n/\nCtrl+U\n<wide-query>\nZ\nBackspace\nCtrl+U\nEnter\nq\n",
	))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", "q")
}

func TestBrowserHostileTerminalDataUsesSafeCells(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is unavailable")
	}
	fx := newFixture(t)
	socket := fmt.Sprintf("skillglass-hostile-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	commandLine := strings.Join([]string{
		"exec",
		shellQuote(skillglassBinary),
		"browse",
		"--root",
		shellQuote(fx.hostileRoot),
	}, " ")
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", "80", "-y", "24", "-s", "hostile", commandLine,
	)
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "resize-window", "-t", "hostile:0", "-x", "80", "-y", "24")

	initial := waitForPaneAt(t, tmux, fx, socket, "hostile:0.0", false, `hostile-name-START\x1b[2J-END`)
	assertPaneCells(t, initial, 80, 24)
	detail := actForPaneAt(t, tmux, fx, socket, "hostile:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "hostile:0.0", "Enter")
	}, "Selected: hostile-name", "Rendered instructions", `description-START\x1b]0;HOSTILE_TITLE\a-END`)
	if detail == initial {
		t.Fatal("hostile Source detail equals inventory")
	}
	assertPaneCells(t, detail, 80, 24)
	end := capturePaneAt(t, tmux, fx, socket, "hostile:0.0", false)
	for _, expected := range []string{`BODY_START\x1b[31mBODY_END\rCR_END\aBELL_END`, "LAST_HOSTILE_SENTINEL"} {
		if !paneContains(end, expected) {
			t.Fatalf("hostile instructions missing %q:\n%s", expected, end)
		}
	}
	assertPaneCells(t, end, 80, 24)
	title := strings.TrimSpace(runTmux(
		t, tmux, fx, socket,
		"display-message", "-p", "-t", "hostile:0.0", "#{pane_title}",
	))
	if title == "HOSTILE_TITLE" {
		t.Fatal("untrusted OSC changed the terminal title")
	}
	writeEvidence(t, "tmux/hostile/00-inventory.txt", []byte(initial))
	writeEvidence(t, "tmux/hostile/01-detail.txt", []byte(detail))
	writeEvidence(t, "tmux/hostile/02-end.txt", []byte(end))
	writeEvidence(t, "tmux/hostile/title.txt", []byte(title+"\n"))
	writeEvidence(t, "tmux/hostile/dimensions.txt", []byte("80x24\n"))
	writeEvidence(t, "tmux/hostile/keys.txt", []byte("Enter\nq\n"))
	controlCommand := "printf '\\033]0;HOSTILE_TITLE\\007'; exec /bin/sleep 2"
	runTmux(t, tmux, fx, socket,
		"new-session", "-d", "-x", "80", "-y", "24", "-s", "control", controlCommand,
	)
	controlTitle := waitForPaneTitle(t, tmux, fx, socket, "control:0.0", "HOSTILE_TITLE")
	writeEvidence(t, "tmux/hostile/negative-control-title.txt", []byte(controlTitle+"\n"))
	runTmux(t, tmux, fx, socket, "kill-session", "-t", "control")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "hostile:0.0", "-l", "q")
}

func runTmux(t *testing.T, tmux string, fx fixture, socket string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-L", socket, "-f", "/dev/null"}, args...)
	command := exec.Command(tmux, commandArgs...)
	command.Dir = fx.project
	command.Env = fx.environment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func runTmuxIgnoringError(tmux string, fx fixture, socket string, args ...string) {
	commandArgs := append([]string{"-L", socket, "-f", "/dev/null"}, args...)
	command := exec.Command(tmux, commandArgs...)
	command.Dir = fx.project
	command.Env = fx.environment()
	_ = command.Run()
}

func waitForPane(
	t *testing.T,
	tmux string,
	fx fixture,
	socket string,
	joinWrapped bool,
	needles ...string,
) string {
	return waitForPaneAt(t, tmux, fx, socket, "browser:0.0", joinWrapped, needles...)
}

func waitForPaneAt(
	t *testing.T,
	tmux string,
	fx fixture,
	socket string,
	target string,
	joinWrapped bool,
	needles ...string,
) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var frame paneSnapshot
	for time.Now().Before(deadline) {
		frame = snapshotPane(t, tmux, fx, socket, target)
		rows := strings.Split(strings.TrimSuffix(frame.cells, "\n"), "\n")
		if paneHas(frame, joinWrapped, needles...) && len(rows) == frame.height && strings.TrimSpace(rows[len(rows)-1]) != "" {
			if joinWrapped {
				return frame.joined
			}
			return frame.cells
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pane did not show %q on the current grid:\n%s", needles, frame.cells)
	return ""
}

func paneContains(capture, needle string) bool {
	if strings.Contains(capture, needle) {
		return true
	}
	compact := func(value string) string {
		return strings.Join(strings.Fields(value), "")
	}
	return strings.Contains(compact(capture), compact(needle))
}

func assertSearchViewport(
	t *testing.T,
	capture string,
	width int,
	height int,
	tail string,
	editing bool,
	cropped bool,
) string {
	t.Helper()
	if !utf8.ValidString(capture) {
		t.Fatal("search capture is not valid UTF-8")
	}
	assertPaneCells(t, capture, width, height)
	var rows []string
	for _, line := range strings.Split(strings.TrimSuffix(capture, "\n"), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "Search /") {
			rows = append(rows, line)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("Search row count = %d, want 1:\n%s", len(rows), capture)
	}
	fullRow := rows[0]
	row := fullRow
	if left, _, found := strings.Cut(row, "│"); found {
		row = left
	}
	row = strings.TrimRight(row, " ")
	trimmedRow := strings.TrimLeft(row, " ")
	if cropped && !strings.HasPrefix(trimmedRow, "Search / …") {
		t.Fatalf("cropped search row has no leading ellipsis: %q", row)
	}
	if !cropped && strings.HasPrefix(trimmedRow, "Search / …") {
		t.Fatalf("uncropped search row has an ellipsis: %q", row)
	}
	if tail != "" && !strings.Contains(row, tail) {
		t.Fatalf("search row does not show tail %q: %q", tail, row)
	}
	if editing && !strings.HasSuffix(row, "█") {
		t.Fatalf("editing search row has no cursor: %q", row)
	}
	if !editing && strings.Contains(row, "█") {
		t.Fatalf("inventory search row retained editing cursor: %q", row)
	}
	if cells := ansi.StringWidth(fullRow); cells > width {
		t.Fatalf("search row uses %d cells, pane has %d: %q", cells, width, fullRow)
	}
	return row
}

func assertSearchChrome(t *testing.T, capture string) {
	t.Helper()
	for _, expected := range []string{
		"No matching sources.",
		"Focused: no matching sources",
		"Type",
		"Enter keep",
		"Esc cancel",
		"Ctrl+U clear",
	} {
		if !paneContains(capture, expected) {
			t.Fatalf("search frame does not show %q:\n%s", expected, capture)
		}
	}
}

func waitForPaneTitle(
	t *testing.T,
	tmux string,
	fx fixture,
	socket string,
	target string,
	want string,
) string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var title string
	for time.Now().Before(deadline) {
		title = strings.TrimSpace(runTmux(
			t, tmux, fx, socket,
			"display-message", "-p", "-t", target, "#{pane_title}",
		))
		if title == want {
			return title
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pane title = %q, want %q", title, want)
	return ""
}

func capturePane(t *testing.T, tmux string, fx fixture, socket string, joinWrapped bool) string {
	t.Helper()
	return capturePaneAt(t, tmux, fx, socket, "browser:0.0", joinWrapped)
}

func capturePaneAt(
	t *testing.T,
	tmux string,
	fx fixture,
	socket string,
	target string,
	joinWrapped bool,
) string {
	t.Helper()
	args := []string{"capture-pane", "-p", "-t", target}
	if joinWrapped {
		args = append(args, "-J")
	}
	return runTmux(t, tmux, fx, socket, args...)
}

func assertPaneCells(t *testing.T, capture string, width, height int) {
	t.Helper()
	if strings.ContainsRune(capture, '\x1b') {
		t.Fatal("tmux cell capture contains an escape control")
	}
	lines := strings.Split(strings.TrimSuffix(capture, "\n"), "\n")
	if len(lines) > height {
		t.Fatalf("capture has %d rows, pane has %d", len(lines), height)
	}
	for index, line := range lines {
		if cells := ansi.StringWidth(line); cells > width {
			t.Fatalf("capture row %d uses %d cells, pane has %d: %q", index+1, cells, width, line)
		}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func writeEvidence(t *testing.T, relative string, content []byte) {
	t.Helper()
	root := os.Getenv("SKILLGLASS_EVIDENCE")
	if root == "" {
		return
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
