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
		"esc",
		"/",
		"definitely-no-match",
		"enter",
		"q",
	}
	raw.wait(t, 0, "Selected: none", "Focus: 1/2", "normal-skill", "Enter read")
	postconditions := []string{"Read lines", "LAST_INSTRUCTION_SENTINEL", "Enter read", "type to search", "0 matches | selected source unchanged", "Enter read", ""}
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
	for _, expected := range []string{
		"Project:",
		"Coverage: complete",
		"normal-skill",
		normal.ID,
		"description-only-literal",
		"Manifest: parsed",
		"LAST_INSTRUCTION_SENTINEL",
		"Resources:",
		"scripts/never-run.sh",
		"executable",
		"0 matches | selected source unchanged",
	} {
		if !bytes.Contains(raw.Bytes(), []byte(expected)) {
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
				t, tmux, fx, socket, false, "Selected: none", "normal-skill", "Focus: 1/2", "Scope: 1 root |",
			)
			assertPaneCells(t, initial, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/%d/00-inventory.txt", width), []byte(initial))
			if width == 80 {
				if !strings.Contains(initial, "description-only-literal") {
					t.Fatalf("initial inventory did not expose the pre-satisfied description predicate:\n%s", initial)
				}
				scope := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "s")
				}, "SKILLGLASS | Scope", "Root 1 [searched]", "Esc back")
				assertPaneCells(t, scope, 80, 30)
				writeEvidence(t, "tmux/80/00-scope.txt", []byte(scope))
				resizedScope := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 63, 20, "SKILLGLASS | Scope", "Root 1 [searched]", "Esc back")
				assertPaneCells(t, resizedScope, 63, 20)
				writeEvidence(t, "tmux/80/00-scope-resized-63x20.txt", []byte(resizedScope))
				restoredScope := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 80, 30, "SKILLGLASS | Scope", "Root 1 [searched]", "Esc back")
				writeEvidence(t, "tmux/80/00-scope-restored-80x30.txt", []byte(restoredScope))
				_ = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Escape")
				}, "Selected: none", "normal-skill")
			}

			search := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "/")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "description-only-literal")
			}, "Search: description-only-literal█", "Focus: 1/1", normal.Name, "type to search")
			assertSearchViewport(t, search, width, 30, "description-only-literal", true, false)
			if strings.Contains(search, "second-skill") {
				t.Fatalf("description search retained a nonmatching inventory row:\n%s", search)
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/01-search.txt", width), []byte(search))

			_ = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
			}, "Search: description-only-literal", "Focus: 1/1", normal.Name, "Enter read")
			detail := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
			}, "Manifest: parsed", "Read lines", "Selector: "+normal.ID)
			assertPaneCells(t, detail, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/%d/02-detail.txt", width), []byte(detail))

			endJoined := actForPaneAt(t, tmux, fx, socket, "browser:0.0", true, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "G")
			}, "scripts/never-run.sh", "executable", "q quit")
			end := capturePane(t, tmux, fx, socket, false)
			assertPaneCells(t, end, width, 30)
			if !strings.Contains(endJoined, "Esc back") {
				t.Fatalf("joined end capture does not expose complete footer:\n%s", endJoined)
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/03-end.txt", width), []byte(end))
			writeEvidence(t, fmt.Sprintf("tmux/%d/03-end-joined.txt", width), []byte(endJoined))
			dimensions := strings.TrimSpace(runTmux(
				t, tmux, fx, socket,
				"display-message", "-p", "-t", "browser:0.0", "#{pane_width}x#{pane_height}",
			))
			if dimensions != fmt.Sprintf("%dx30", width) {
				t.Fatalf("pane dimensions = %q", dimensions)
			}
			writeEvidence(t, fmt.Sprintf("tmux/%d/dimensions.txt", width), []byte(dimensions+"\n"))
			writeEvidence(t, fmt.Sprintf("tmux/%d/keys.txt", width), []byte("/\ndescription-only-literal\nEnter\nEnter\nG\nq\n"))
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
				resized := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 63, 20, "SKILLGLASS | Source", "Read lines", "scripts/never-run.sh", "q quit")
				assertPaneCells(t, resized, 63, 20)
				writeEvidence(t, "tmux/80/04-resized-63x20.txt", []byte(resized))
				restored := resizePaneAt(t, tmux, fx, socket, "browser:0.0", 80, 30, "SKILLGLASS | Source", "Read lines", "scripts/never-run.sh", "q quit")
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
		"---\nname: many-root-source\ndescription: source remains visible\n---\nMANY_ROOT_INSTRUCTIONS\n",
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
		"Scope: 8 roots |",
		"s scope",
		"q quit",
	)
	assertPaneCells(t, initial, 60, 30)
	writeEvidence(t, "tmux/many-roots/00-inventory.txt", []byte(initial))
	searchS := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "/")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s")
	}, "Search: s", "type to search")
	if strings.Contains(searchS, "SKILLGLASS | Scope") {
		t.Fatal("s opened scope while search input was active")
	}
	writeEvidence(t, "tmux/many-roots/00-search-s.txt", []byte(searchS))
	_ = actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	}, "many-root-source", "Focus: 1/1", "s scope")

	canonicalFirst := canonicalPath(t, roots[0])
	scopeTop := actForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"many:0.0",
		false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s") },
		"SKILLGLASS | Scope",
		longProject,
		roots[0],
		canonicalFirst,
		"Root 1 [searched]",
		"Esc back",
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
		"Esc back",
		"q quit",
	)
	assertPaneCells(t, scopeEnd, 60, 30)
	writeEvidence(t, "tmux/many-roots/02-scope-end.txt", []byte(scopeEnd))

	restoredInventory := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	}, "many-root-source", "Focus: 1/1", "s scope")
	assertPaneCells(t, restoredInventory, 60, 30)
	writeEvidence(t, "tmux/many-roots/03-inventory-restored.txt", []byte(restoredInventory))

	detail := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Enter")
	}, "Selected: many-root-source", "Read lines", "s scope")
	assertPaneCells(t, detail, 60, 30)
	writeEvidence(t, "tmux/many-roots/04-detail.txt", []byte(detail))
	detailLineCount := 0
	for _, line := range strings.Split(detail, "\n") {
		var first, last, total int
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "Read lines %d-%d of %d", &first, &last, &total); err == nil {
			detailLineCount = total
			break
		}
	}
	if detailLineCount == 0 {
		t.Fatalf("detail frame does not report its line count:\n%s", detail)
	}
	detailEndStatus := fmt.Sprintf("-%d of %d", detailLineCount, detailLineCount)
	// Long fixture prefixes can push instructions below the first viewport.
	// Prove they are reachable and that the scope round trip keeps that position.
	detailEnd := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "G")
	}, "MANY_ROOT_INSTRUCTIONS", detailEndStatus, "s scope")
	assertPaneCells(t, detailEnd, 60, 30)
	writeEvidence(t, "tmux/many-roots/04-detail-end.txt", []byte(detailEnd))
	_ = actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s")
	}, "SKILLGLASS | Scope", longProject)
	restoredDetail := actForPaneAt(t, tmux, fx, socket, "many:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	}, "MANY_ROOT_INSTRUCTIONS", detailEndStatus, "s scope")
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
	_ = waitForPaneAt(t, tmux, fx, socket, "long:0.0", false, "Focus: 1/2", "/ search")
	writeEvidence(t, "tmux/long-search/command.txt", []byte(commandLine+"\n"))

	longEditing := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "/")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", longQuery)
		},
		asciiTail+"█", "No matching sources.", "0 matches | selected source unchanged",
		"type to search", "Enter keep", "Esc cancel", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/01-long-editing-60x30.txt", []byte(longEditing))
	longRow := assertSearchViewport(t, longEditing, 60, 30, asciiTail, true, true)
	assertSearchChrome(t, longEditing)

	narrow := resizePaneAt(
		t, tmux, fx, socket, "long:0.0", 48, 20,
		asciiTail+"█", "No matching sources.", "0 matches | selected source unchanged",
		"type to search", "Enter keep", "Esc cancel", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/02-long-editing-48x20.txt", []byte(narrow))
	narrowRow := assertSearchViewport(t, narrow, 48, 20, asciiTail, true, true)
	assertSearchChrome(t, narrow)
	if narrowRow == longRow {
		t.Fatalf("search viewport did not change after resize: %q", narrowRow)
	}

	restored := resizePaneAt(
		t, tmux, fx, socket, "long:0.0", 60, 30,
		longRow, "No matching sources.", "0 matches | selected source unchanged",
		"type to search", "Enter keep", "Esc cancel", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/03-long-editing-restored.txt", []byte(restored))
	if restoredRow := assertSearchViewport(t, restored, 60, 30, asciiTail, true, true); restoredRow != longRow {
		t.Fatalf("restored search row = %q, want %q", restoredRow, longRow)
	}
	assertSearchChrome(t, restored)

	kept := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "Enter") },
		asciiTail, "No matching sources.", "0 matches | selected source unchanged", "Enter read", "s scope",
	)
	writeEvidence(t, "tmux/long-search/04-long-kept.txt", []byte(kept))
	assertSearchViewport(t, kept, 60, 30, asciiTail, false, true)

	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "/")
	}, asciiTail+"█", "type to search")
	cancelTail := "CANCEL_TAIL"
	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", cancelTail)
	}, cancelTail+"█", "type to search")
	cancelled := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "Escape") },
		asciiTail, "No matching sources.", "0 matches | selected source unchanged", "Enter read", "s scope",
	)
	writeEvidence(t, "tmux/long-search/05-long-cancelled.txt", []byte(cancelled))
	assertSearchViewport(t, cancelled, 60, 30, asciiTail, false, true)
	if strings.Contains(cancelled, cancelTail) {
		t.Fatal("Escape retained text appended after queryBefore")
	}

	_ = actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "/")
	}, asciiTail+"█", "type to search")
	cleared := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "C-u") },
		"Search: █", "Focus: 1/2", "tail-only-source", "other-source", "Ctrl+U clear",
	)
	writeEvidence(t, "tmux/long-search/06-cleared.txt", []byte(cleared))
	assertSearchViewport(t, cleared, 60, 30, "", true, false)

	wideTail := "終端尾"
	wideQuery := strings.Repeat("界", 120) + wideTail
	wide := actForPaneAt(
		t, tmux, fx, socket, "long:0.0", false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "-l", wideQuery) },
		wideTail+"█", "No matching sources.", "0 matches | selected source unchanged", "Ctrl+U clear",
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
	}, "Search: █", "Focus: 1/2")
	clearKept := actForPaneAt(t, tmux, fx, socket, "long:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "long:0.0", "Enter")
	}, "Search:", "Focus: 1/2", "Enter read")
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
	}, "SKILLGLASS | Source", "Read lines", `description-START\x1b]0;HOSTILE_TITLE\a-END`)
	if detail == initial {
		t.Fatal("hostile Source detail equals inventory")
	}
	assertPaneCells(t, detail, 80, 24)
	end := actForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"hostile:0.0",
		false,
		func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "hostile:0.0", "-l", "G") },
		`BODY_START\x1b[31mBODY_END\rCR_END\aBELL_END`,
		"LAST_HOSTILE_SENTINEL",
	)
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
	writeEvidence(t, "tmux/hostile/keys.txt", []byte("Enter\nG\nq\n"))
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
		if strings.HasPrefix(line, "Search:") {
			rows = append(rows, line)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("Search row count = %d, want 1:\n%s", len(rows), capture)
	}
	row := rows[0]
	if cropped && !strings.HasPrefix(row, "Search: …") {
		t.Fatalf("cropped search row has no leading ellipsis: %q", row)
	}
	if !cropped && strings.HasPrefix(row, "Search: …") {
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
	if cells := ansi.StringWidth(row); cells > width {
		t.Fatalf("search row uses %d cells, pane has %d: %q", cells, width, row)
	}
	return row
}

func assertSearchChrome(t *testing.T, capture string) {
	t.Helper()
	for _, expected := range []string{
		"No matching sources.",
		"0 matches | selected source unchanged",
		"type to search",
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
