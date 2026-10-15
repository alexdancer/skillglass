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
	if len(inventory.Problems) == 0 {
		t.Fatal("problem fixture did not report a problem")
	}
	firstProblem := inventory.Problems[0]

	for _, width := range []int{48, 60, 80, 96, 120} {
		width := width
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			socket := fmt.Sprintf("skillglass-ale24-modes-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx.fixture, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--root", fx.rootA, "--project", fx.project, "--project-root", fx.rootB)
			launch := runTmuxProduct(t, tmux, fx.fixture, socket, command,
				"new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser",
			)
			runTmux(t, tmux, fx.fixture, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx.fixture, socket,
				"resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30",
			)

			initial := waitForPane(t, tmux, fx.fixture, socket, false,
				"Skillglass",
				"Sources",
				"Filters: all",
				"Focused:",
				"? help",
			)
			assertPaneCells(t, initial, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/00-sources.txt", width), []byte(initial))

			act := func(action func(), needles ...string) string {
				return actForPaneAt(t, tmux, fx.fixture, socket, "browser:0.0", false, action, needles...)
			}
			searchHint := "Type to filter"
			if width < 60 {
				searchHint = "Enter keep Esc cancel"
			}
			_ = act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "/")
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "-l", "shared-directory")
			}, "Search / shared-directory█", searchHint)
			_ = act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Enter")
			}, "Search / shared-directory", "Focus: 1/1", "? help")
			detailTop := act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Enter")
			},
				"Selected: shared-directory",
				"Rendered instructions",
				"SHARED_DIRECTORY",
			)
			assertPaneCells(t, detailTop, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/01-instructions.txt", width), []byte(detailTop))

			_ = act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "-l", "d")
			}, "Details", "Source ID: "+shared.ID)
			backHint := "← back"
			if width < 60 {
				backHint = "Esc back"
			}
			customReference := scrollALE24Until(
				t, tmux, fx.fixture, socket, "browser:0.0", 40,
				"Context: custom", "directory-alias-one", backHint,
			)
			assertPaneCells(t, customReference, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/02-reference-custom.txt", width), []byte(customReference))

			projectReference := scrollALE24Until(
				t, tmux, fx.fixture, socket, "browser:0.0", 40,
				"Context: project", "directory-alias-two", backHint,
			)
			assertPaneCells(t, projectReference, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/03-reference-project.txt", width), []byte(projectReference))

			_ = act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Escape")
			}, "Sources", "Focus: 1/1", "Search / shared-directory")
			_ = act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Tab")
			}, "Problems", "0 matching problems")
			_ = act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "/")
			}, searchHint)
			problems := act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "C-u")
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Enter")
			},
				"Problems",
				"duplicate-name",
				"Enter problem",
				"Filters: all",
			)
			assertPaneCells(t, problems, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/04-problems.txt", width), []byte(problems))

			problemDetail := act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Enter")
			},
				"Selected problem:",
				string(firstProblem.ID),
				"Code: "+firstProblem.Code,
				"Problem lines",
				backHint,
			)
			assertPaneCells(t, problemDetail, width, 30)
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/05-problem-detail.txt", width), []byte(problemDetail))
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/keys.txt", width), []byte(
				"/\nshared-directory\nEnter\nEnter\nd\n<scroll>\nEsc\nTab\n/\nCtrl+U\nEnter\nEnter\nEsc\nq\n",
			))
			writeEvidence(t, fmt.Sprintf("tmux/ale24-modes/%d/command.txt", width), []byte(strings.Join(launch.Argv, "\n")+"\n"))
			_ = act(func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", "browser:0.0", "Escape")
			}, "Problems", "Enter problem")
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
	command := fx.productCommand(t, "browse", "--root", fx.rootA, "--project", fx.project, "--project-root", fx.rootB)
	launch := runTmuxProduct(t, tmux, fx.fixture, socket, command,
		"new-session", "-d", "-x", "60", "-y", "30", "-s", "refresh",
	)
	runTmux(t, tmux, fx.fixture, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx.fixture, socket, "set-option", "-w", "-t", "refresh:0", "remain-on-exit", "on")
	runTmux(t, tmux, fx.fixture, socket, "resize-window", "-t", "refresh:0", "-x", "60", "-y", "30")
	target := "refresh:0.0"
	_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Skillglass", "Sources", "? help", "q quit")
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

	act := func(action func(), needles ...string) string {
		return actForPaneAt(t, tmux, fx.fixture, socket, target, false, action, needles...)
	}
	_ = act(func() { key("/"); literal("shared-directory") }, "Search / shared-directory█", "Type to filter")
	_ = act(func() { key("Enter") }, "Search / shared-directory", "Focus: 1/1", "? help")
	selected := act(func() { key("Enter") },
		"Selected: shared-directory", "Rendered instructions", "SHARED_DIRECTORY",
	)
	writeEvidence(t, "tmux/ale24-refresh/00-selected.txt", []byte(selected))
	_ = act(func() { literal("d") }, "Details", "Source ID: "+shared.ID)

	renamedDirectory := filepath.Join(filepath.Dir(fx.sharedDir), "shared-directory-renamed")
	if err := os.Rename(fx.sharedDir, renamedDirectory); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, renamedDirectory, "shared-directory-renamed", "rename and insertion refresh marker", "RENAMED_REFRESH")
	writeSkill(t, filepath.Join(fx.rootA, "00-row-insertion"), "00-row-insertion", "inserted before selection", "ROW_INSERTION")
	replaceDirectoryAlias(t, filepath.Join(fx.rootA, "directory-alias-one"), filepath.Join("..", "..", "outside", "shared-directory-renamed"))
	replaceDirectoryAlias(t, filepath.Join(fx.rootB, "directory-alias-two"), filepath.Join("..", "..", "outside", "shared-directory-renamed"))
	renamed := act(func() { key("r") },
		"Selected: shared-directory-renamed",
		"Source ID: "+shared.ID,
		"rename and insertion refresh marker",
		"Refresh: selected directory object retained",
	)
	writeEvidence(t, "tmux/ale24-refresh/01-rename-retained.txt", []byte(renamed))

	hidden := act(func() { key("c"); key("o") },
		"Filters: s:shared-directory c:project r:ale24-a",
		"Source-root filter:",
		"Source ID: "+shared.ID,
		"Selected: shared-directory-renamed (hidden by filters)",
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
	oneAlias := act(func() { key("r") },
		"Source ID: "+shared.ID,
		"Selected: shared-directory-renamed",
		"alias survives refresh",
		"Refresh: selected directory object retained",
	)
	writeEvidence(t, "tmux/ale24-refresh/03-one-alias-retained.txt", []byte(oneAlias))

	replaceDirectoryAlias(t, filepath.Join(fx.rootB, "directory-alias-two"), filepath.Join("..", "..", "outside", "retarget-after-alias"))
	aliasLost := act(func() { key("r") },
		"Sources", "Focused:", "Refresh: selection cleared",
	)
	writeEvidence(t, "tmux/ale24-refresh/04-last-alias-cleared.txt", []byte(aliasLost))

	_ = act(func() {
		key("c")
		key("/")
		key("C-u")
		literal("replacement-target")
	}, "Search / replacement-target█", "Type to filter")
	_ = act(func() { key("Enter") }, "Focus: 1/1", "replacement-target", "Search / replacement-target", "? help")
	_ = act(func() { key("Enter") },
		"Selected: replacement-target", "Rendered instructions", "REPLACEMENT_BYTES",
	)
	_ = act(func() { literal("d") }, "Details", "Source ID: "+replacement.ID)
	replacedOld := filepath.Join(fx.base, "allowed", "outside", "replacement-old")
	if err := os.Rename(replacementDirectory, replacedOld); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, replacementDirectory, "replacement-target", "same bytes after replacement", "REPLACEMENT_BYTES")
	replaced := act(func() { key("r") },
		"Sources", "Focused:", "Refresh: selection cleared",
	)
	writeEvidence(t, "tmux/ale24-refresh/05-replacement-cleared.txt", []byte(replaced))

	_ = act(func() { key("Tab") }, "Problems", "0 matching problems")
	problemSearch := act(func() { key("/"); key("C-u"); paste("qrcfo") },
		"Problems", "Search / qrcfo█", "problem-qrcfo-marker", "Enter keep", "Esc cancel",
	)
	writeEvidence(t, "tmux/ale24-refresh/06-problem-paste.txt", []byte(problemSearch))
	_ = act(func() { key("C-u") }, "Search / █", "Filters: context custom", "Enter keep")
	_ = act(func() { paste("qrcfo") }, "Search / qrcfo█", "problem-qrcfo-marker", "Problem focus: 1/1")
	_ = act(func() { key("Enter") }, "Enter problem", "Problem focus: 1/1")
	problemDetail := act(func() { key("Enter") },
		"Selected problem:", "Code: broken-reference", "problem-qrcfo-marker", "Limits coverage: true",
	)
	writeEvidence(t, "tmux/ale24-refresh/07-problem-detail.txt", []byte(problemDetail))
	assertPaneCells(t, problemDetail, 60, 30)

	expected := treeManifest(t, fx.base)
	writeEvidence(t, "tmux/ale24-refresh/command.txt", []byte(strings.Join(launch.Argv, "\n")+"\n"))
	writeEvidence(t, "tmux/ale24-refresh/keys.txt", []byte(keys.String()+"<q>\n"))
	_ = act(func() { key("q") }, "Pane is dead")
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
		before := capture
		_ = actOnPane(t, tmux, fx, socket, target, func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "j")
		}, func(frame paneSnapshot) bool {
			return frame.cells != before && paneHas(frame, false, "Details")
		})
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
