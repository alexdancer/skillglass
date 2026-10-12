package acceptance_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestALE24BrowserModesAndReferencesAtRequiredWidths(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux is required for ALE-24 browser coverage")
	}
	fx := newALE24Fixture(t)
	before := treeManifest(t, fx.base)
	list := fx.run(
		t,
		"list",
		"--root", fx.rootA,
		"--project", fx.project,
		"--project-root", fx.rootB,
		"--json",
	)
	assertExit(t, list, 0)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	shared := sourceNamed(t, inventory.Sources, "shared-directory")

	for _, width := range []int{48, 60, 80, 96, 120} {
		width := width
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			socket := fmt.Sprintf("skillglass-ale24-modes-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx.fixture, socket, "kill-server")
			commandLine := strings.Join([]string{
				"exec", shellQuote(skillglassBinary), "browse",
				"--root", shellQuote(fx.rootA),
				"--project", shellQuote(fx.project),
				"--project-root", shellQuote(fx.rootB),
			}, " ")
			runTmux(t, tmux, fx.fixture, socket,
				"new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", commandLine,
			)
			runTmux(t, tmux, fx.fixture, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx.fixture, socket,
				"resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30",
			)

			initial := waitForPane(t, tmux, fx.fixture, socket, false,
				"SKILLGLASS | Sources",
				"Context filter: all",
				"Problem filter: all",
				"Root filter: all",
				"Tab sources/problems",
			)
			assertPaneCells(t, initial, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/00-sources.txt", width), []byte(initial))

			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "/")
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "-l", "shared-directory")
			_ = waitForPane(t, tmux, fx.fixture, socket, false, "Search: shared-directory", "Focus: 1/1")
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Enter")
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Enter")
			detailTop := waitForPane(t, tmux, fx.fixture, socket, false,
				"Selected: shared-directory",
				"Selector: "+shared.ID,
				"Reference custom",
				"Read lines",
			)
			assertPaneCells(t, detailTop, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/01-reference-custom.txt", width), []byte(detailTop))

			projectReference := scrollALE24Until(
				t, tmux, fx.fixture, socket, "browser:0.0", 40,
				"Reference project", "directory-alias-two", "Esc back",
			)
			assertPaneCells(t, projectReference, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/02-reference-project.txt", width), []byte(projectReference))

			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Escape")
			_ = waitForPane(t, tmux, fx.fixture, socket, false, "SKILLGLASS | Sources", "Focus: 1/1")
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Tab")
			_ = waitForPane(t, tmux, fx.fixture, socket, false, "SKILLGLASS | Problems")
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "/")
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "C-u")
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Enter")
			problems := waitForPane(t, tmux, fx.fixture, socket, false,
				"SKILLGLASS | Problems",
				"duplicate-name",
				"Enter problem",
				"Context filter: all",
			)
			assertPaneCells(t, problems, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/03-problems.txt", width), []byte(problems))
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/keys.txt", width), []byte(
				"/\nshared-directory\nEnter\nEnter\n<scroll>\nEsc\nTab\n/\nCtrl+U\nEnter\nq\n",
			))
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/command.txt", width), []byte(commandLine+"\n"))
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "q")
		})
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("ALE-24 browser mode journeys mutated their fixture")
	}
}

func TestALE24BrowserRefreshSelectionAndProblemPaste(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux is required for ALE-24 refresh coverage")
	}
	fx := newALE24Fixture(t)
	replacementDirectory := filepath.Join(fx.rootA, "replacement-target")
	writeSkill(t, replacementDirectory, "replacement-target", "same bytes after replacement", "REPLACEMENT_BYTES")
	initialList := fx.run(
		t,
		"list",
		"--root", fx.rootA,
		"--project", fx.project,
		"--project-root", fx.rootB,
		"--json",
	)
	assertExit(t, initialList, 0)
	var initial publicList
	decodeJSON(t, initialList.stdout, &initial)
	shared := sourceNamed(t, initial.Sources, "shared-directory")
	replacement := sourceNamed(t, initial.Sources, "replacement-target")

	socket := fmt.Sprintf("skillglass-ale24-refresh-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx.fixture, socket, "kill-server")
	commandLine := strings.Join([]string{
		"exec", shellQuote(skillglassBinary), "browse",
		"--root", shellQuote(fx.rootA),
		"--project", shellQuote(fx.project),
		"--project-root", shellQuote(fx.rootB),
	}, " ")
	runTmux(t, tmux, fx.fixture, socket,
		"new-session", "-d", "-x", "60", "-y", "30", "-s", "refresh", commandLine,
	)
	runTmux(t, tmux, fx.fixture, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx.fixture, socket, "set-option", "-w", "-t", "refresh:0", "remain-on-exit", "on")
	runTmux(t, tmux, fx.fixture, socket, "resize-window", "-t", "refresh:0", "-x", "60", "-y", "30")
	target := "refresh:0.0"
	_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "SKILLGLASS | Sources", "r refresh")
	var keys strings.Builder
	key := func(name string) {
		fmt.Fprintf(&keys, "<%s>\n", name)
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, name)
	}
	literal := func(value string) {
		keys.WriteString(value + "\n")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", value)
	}
	paste := func(value string) {
		payload := "\x1b[200~" + value + "\x1b[201~"
		keys.WriteString("<paste:" + value + ">\n")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", payload)
	}

	key("/")
	literal("shared-directory")
	_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Search: shared-directory", "Focus: 1/1")
	key("Enter")
	key("Enter")
	selected := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"Selected: shared-directory", "Selector: "+shared.ID, "Reference custom",
	)
	writeEvidence(t, "tmux/ale24-refresh/00-selected.txt", []byte(selected))

	renamedDirectory := filepath.Join(filepath.Dir(fx.sharedDir), "shared-directory-renamed")
	if err := os.Rename(fx.sharedDir, renamedDirectory); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, renamedDirectory, "shared-directory-renamed", "rename and insertion refresh marker", "RENAMED_REFRESH")
	writeSkill(t, filepath.Join(fx.rootA, "00-row-insertion"), "00-row-insertion", "inserted before selection", "ROW_INSERTION")
	replaceDirectoryAlias(t, filepath.Join(fx.rootA, "directory-alias-one"), filepath.Join("..", "..", "outside", "shared-directory-renamed"))
	replaceDirectoryAlias(t, filepath.Join(fx.rootB, "directory-alias-two"), filepath.Join("..", "..", "outside", "shared-directory-renamed"))
	key("r")
	renamed := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"Selected: shared-directory-renamed",
		"Selector: "+shared.ID,
		"Description: rename and insertion refresh marker",
		"Refresh: selected directory object retained",
	)
	writeEvidence(t, "tmux/ale24-refresh/01-rename-retained.txt", []byte(renamed))

	key("c")
	key("o")
	hidden := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"Context filter: project",
		"Root filter: #1/2 ale24-a",
		"Selector: "+shared.ID,
		"Selection: hidden by filters",
	)
	writeEvidence(t, "tmux/ale24-refresh/02-hidden-selected.txt", []byte(hidden))

	problemPath := filepath.Join(fx.rootA, "problem-qrcfo-marker")
	if err := os.Symlink(filepath.Join("..", "..", "outside", "missing-qrcfo-target"), problemPath); err != nil {
		t.Fatal(err)
	}
	retargetDirectory := filepath.Join(fx.base, "allowed", "outside", "retarget-after-alias")
	writeSkill(t, retargetDirectory, "retarget-after-alias", "new alias target", "RETARGETED_ALIAS")
	writeSkill(t, renamedDirectory, "shared-directory-renamed", "alias survives refresh", "ALIAS_SURVIVES_REFRESH")
	replaceDirectoryAlias(t, filepath.Join(fx.rootA, "directory-alias-one"), filepath.Join("..", "..", "outside", "retarget-after-alias"))
	key("r")
	oneAlias := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"Selector: "+shared.ID,
		"Selection: hidden by filters",
		"Description: alias survives refresh",
		"Refresh: selected directory object retained",
	)
	writeEvidence(t, "tmux/ale24-refresh/03-one-alias-retained.txt", []byte(oneAlias))

	replaceDirectoryAlias(t, filepath.Join(fx.rootB, "directory-alias-two"), filepath.Join("..", "..", "outside", "retarget-after-alias"))
	key("r")
	aliasLost := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"SKILLGLASS | Sources", "Selected: none", "Refresh: selection cleared",
	)
	writeEvidence(t, "tmux/ale24-refresh/04-last-alias-cleared.txt", []byte(aliasLost))

	key("c")
	key("/")
	key("C-u")
	literal("replacement-target")
	key("Enter")
	_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Focus: 1/1", "replacement-target")
	key("Enter")
	_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"Selected: replacement-target", "Selector: "+replacement.ID,
	)
	replacedOld := filepath.Join(fx.base, "allowed", "outside", "replacement-old")
	if err := os.Rename(replacementDirectory, replacedOld); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, replacementDirectory, "replacement-target", "same bytes after replacement", "REPLACEMENT_BYTES")
	key("r")
	replaced := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"SKILLGLASS | Sources", "Selected: none", "Refresh: selection cleared",
	)
	writeEvidence(t, "tmux/ale24-refresh/05-replacement-cleared.txt", []byte(replaced))

	key("Tab")
	_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "SKILLGLASS | Problems")
	key("/")
	key("C-u")
	paste("qrcfo")
	problemSearch := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"SKILLGLASS | Problems", "Search: qrcfo█", "problem-qrcfo-marker", "Problem focus: 1/1",
	)
	writeEvidence(t, "tmux/ale24-refresh/06-problem-paste.txt", []byte(problemSearch))
	key("Enter")
	key("Enter")
	problemDetail := waitForPaneAt(t, tmux, fx.fixture, socket, target, false,
		"SKILLGLASS | Problem", "Code: broken-reference", "problem-qrcfo-marker", "Limits coverage: true",
	)
	writeEvidence(t, "tmux/ale24-refresh/07-problem-detail.txt", []byte(problemDetail))
	assertPaneCells(t, problemDetail, 60, 30)

	expected := treeManifest(t, fx.base)
	writeEvidence(t, "tmux/ale24-refresh/command.txt", []byte(commandLine+"\n"))
	writeEvidence(t, "tmux/ale24-refresh/keys.txt", []byte(keys.String()+"<q>\n"))
	key("q")
	_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Pane is dead")
	status := strings.TrimSpace(runTmux(t, tmux, fx.fixture, socket,
		"display-message", "-p", "-t", target, "#{pane_dead}:#{pane_dead_status}",
	))
	writeEvidence(t, "tmux/ale24-refresh/exit.txt", []byte(status+"\n"))
	if status != "1:1" {
		t.Fatalf("browser exit = %q, want incomplete status 1", status)
	}
	if !bytes.Equal(expected, treeManifest(t, fx.base)) {
		t.Fatal("browser refresh journey mutated the externally edited fixture")
	}
}

func scrollALE24Until(
	t *testing.T,
	tmux string,
	fx fixture,
	socket string,
	target string,
	attempts int,
	needles ...string,
) string {
	t.Helper()
	var capture string
	for attempt := 0; attempt <= attempts; attempt++ {
		capture = capturePaneAt(t, tmux, fx, socket, target, false)
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
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "j")
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pane did not expose %q after scrolling:\n%s", needles, capture)
	return ""
}

func replaceDirectoryAlias(t *testing.T, path, target string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
