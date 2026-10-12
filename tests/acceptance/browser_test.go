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
	var raw bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&raw, terminal)
		close(copyDone)
	}()

	keys := []string{
		"/",
		"description-only-literal",
		"enter",
		"enter",
		"G",
		"esc",
		"/",
		"definitely-no-match",
		"enter",
		"q",
	}
	time.Sleep(150 * time.Millisecond)
	for _, key := range keys {
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
		time.Sleep(120 * time.Millisecond)
	}
	err = command.Wait()
	_ = terminal.Close()
	<-copyDone
	if ctx.Err() != nil {
		t.Fatal("browser PTY journey timed out")
	}
	if err != nil {
		t.Fatalf("browser exit: %v\nraw:\n%q", err, raw.Bytes())
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

			initial := waitForPane(t, tmux, fx, socket, false, "Selected: none", "normal-skill")
			assertPaneCells(t, initial, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/%d/00-inventory.txt", width), []byte(initial))
			if width == 80 {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "s")
				scope := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Scope", "Root 1 [searched]", "Esc back")
				assertPaneCells(t, scope, 80, 30)
				writeEvidence(t, "tmux/80/00-scope.txt", []byte(scope))
				runTmux(t, tmux, fx, socket,
					"resize-window", "-t", "browser:0", "-x", "63", "-y", "20",
				)
				resizedScope := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Scope", "Root 1 [searched]", "Esc back")
				assertPaneCells(t, resizedScope, 63, 20)
				writeEvidence(t, "tmux/80/00-scope-resized-63x20.txt", []byte(resizedScope))
				runTmux(t, tmux, fx, socket,
					"resize-window", "-t", "browser:0", "-x", "80", "-y", "30",
				)
				_ = waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Scope", "Root 1 [searched]", "Esc back")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Escape")
				_ = waitForPane(t, tmux, fx, socket, false, "Selected: none", "normal-skill")
			}

			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "/")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "normal-skill")
			search := waitForPane(t, tmux, fx, socket, false, "Search: normal-skill", "1/1")
			assertPaneCells(t, search, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/%d/01-search.txt", width), []byte(search))

			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
			time.Sleep(50 * time.Millisecond)
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
			detail := waitForPane(t, tmux, fx, socket, false, "Manifest: parsed", "Read lines")
			assertPaneCells(t, detail, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/%d/02-detail.txt", width), []byte(detail))

			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "G")
			endJoined := waitForPane(t, tmux, fx, socket, true, "scripts/never-run.sh", "executable", "q quit")
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
			writeEvidence(t, fmt.Sprintf("tmux/%d/keys.txt", width), []byte("/\nnormal-skill\nEnter\nEnter\nG\nq\n"))
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
				runTmux(t, tmux, fx, socket,
					"resize-window", "-t", "browser:0", "-x", "63", "-y", "20",
				)
				resized := waitForPane(t, tmux, fx, socket, false, "scripts/never-run.sh", "q quit")
				assertPaneCells(t, resized, 63, 20)
				writeEvidence(t, "tmux/80/04-resized-63x20.txt", []byte(resized))
				runTmux(t, tmux, fx, socket,
					"resize-window", "-t", "browser:0", "-x", "80", "-y", "30",
				)
				restored := waitForPane(t, tmux, fx, socket, false, "scripts/never-run.sh", "q quit")
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
		"s scope",
		"q quit",
	)
	assertPaneCells(t, initial, 60, 30)
	writeEvidence(t, "tmux/many-roots/00-inventory.txt", []byte(initial))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "/")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s")
	searchS := waitForPaneAt(t, tmux, fx, socket, "many:0.0", false, "Search: s", "type to search")
	if strings.Contains(searchS, "SKILLGLASS | Scope") {
		t.Fatal("s opened scope while search input was active")
	}
	writeEvidence(t, "tmux/many-roots/00-search-s.txt", []byte(searchS))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	_ = waitForPaneAt(
		t, tmux, fx, socket, "many:0.0", false, "many-root-source", "Focus: 1/1", "s scope",
	)

	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s")
	canonicalFirst := canonicalPath(t, roots[0])
	scopeTop := waitForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"many:0.0",
		false,
		"SKILLGLASS | Scope",
		longProject,
		roots[0],
		canonicalFirst,
		"Root 1 [searched]",
		"Esc back",
	)
	assertPaneCells(t, scopeTop, 60, 30)
	writeEvidence(t, "tmux/many-roots/01-scope-top.txt", []byte(scopeTop))

	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "G")
	canonicalLast := canonicalPath(t, roots[len(roots)-1])
	scopeEnd := waitForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"many:0.0",
		false,
		"Root 8 [empty]",
		roots[len(roots)-1],
		canonicalLast,
		"Esc back",
		"q quit",
	)
	assertPaneCells(t, scopeEnd, 60, 30)
	writeEvidence(t, "tmux/many-roots/02-scope-end.txt", []byte(scopeEnd))

	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	restoredInventory := waitForPaneAt(
		t, tmux, fx, socket, "many:0.0", false, "many-root-source", "Focus: 1/1", "s scope",
	)
	assertPaneCells(t, restoredInventory, 60, 30)
	writeEvidence(t, "tmux/many-roots/03-inventory-restored.txt", []byte(restoredInventory))

	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Enter")
	detail := waitForPaneAt(
		t, tmux, fx, socket, "many:0.0", false, "Manifest: parsed", "MANY_ROOT_INSTRUCTIONS", "s scope",
	)
	assertPaneCells(t, detail, 60, 30)
	writeEvidence(t, "tmux/many-roots/04-detail.txt", []byte(detail))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "s")
	_ = waitForPaneAt(t, tmux, fx, socket, "many:0.0", false, "SKILLGLASS | Scope", longProject)
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "Escape")
	restoredDetail := waitForPaneAt(
		t, tmux, fx, socket, "many:0.0", false, "Manifest: parsed", "MANY_ROOT_INSTRUCTIONS", "s scope",
	)
	assertPaneCells(t, restoredDetail, 60, 30)
	writeEvidence(t, "tmux/many-roots/05-detail-restored.txt", []byte(restoredDetail))
	writeEvidence(t, "tmux/many-roots/keys.txt", []byte("/\ns\nEsc\ns\nG\nEsc\nEnter\ns\nEsc\nq\n"))
	writeEvidence(t, "tmux/many-roots/command.txt", []byte(commandLine+"\n"))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "many:0.0", "-l", "q")
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
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "hostile:0.0", "Enter")
	detail := waitForPaneAt(t, tmux, fx, socket, "hostile:0.0", false, `description-START\x1b]0;HOSTILE_TITLE\a-END`)
	assertPaneCells(t, detail, 80, 24)
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "hostile:0.0", "-l", "G")
	end := waitForPaneAt(
		t,
		tmux,
		fx,
		socket,
		"hostile:0.0",
		false,
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
	var capture string
	for time.Now().Before(deadline) {
		capture = capturePaneAt(t, tmux, fx, socket, target, joinWrapped)
		found := true
		for _, needle := range needles {
			if !paneContains(capture, needle) {
				found = false
				break
			}
		}
		if found {
			return capture
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pane did not show %q:\n%s", needles, capture)
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
