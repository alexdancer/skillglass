package acceptance_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserHelpQQuitsOutsideSearch(t *testing.T) {
	fx := newFixture(t)
	tmux, socket, target := startALE43ReviewBrowser(
		t,
		fx,
		80,
		30,
		"--root", fx.cleanRoot,
		"--project", fx.project,
	)

	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "?")
	help := waitForPaneAt(t, tmux, fx, socket, target, false, "Keys", "q", "Quit")
	writeEvidence(t, "ale43-review/help-q/01-help.txt", []byte(help))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "q")

	deadline := time.Now().Add(time.Second)
	status := ""
	for time.Now().Before(deadline) {
		status = strings.TrimSpace(runTmux(
			t,
			tmux,
			fx,
			socket,
			"display-message", "-p", "-t", target, "#{pane_dead}:#{pane_dead_status}",
		))
		if strings.HasPrefix(status, "1:") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	writeEvidence(t, "ale43-review/help-q/02-pane-status.txt", []byte(status+"\n"))
	if status != "1:0" {
		frame := capturePaneAt(t, tmux, fx, socket, target, false)
		writeEvidence(t, "ale43-review/help-q/03-after-q.txt", []byte(frame))
		t.Fatalf("q from Help left pane status %q, want literal exited status 1:0:\n%s", status, frame)
	}
}

func TestBrowserRetainedHiddenSelectionStaysVisible(t *testing.T) {
	fx := newFixture(t)
	tmux, socket, target := startALE43ReviewBrowser(
		t,
		fx,
		120,
		30,
		"--root", fx.cleanRoot,
		"--project", fx.project,
	)

	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Selected: normal-skill", "Rendered instructions")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Focused: normal-skill", "Search /")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "/")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "second-skill")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "second-skill", "Enter keep")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	filtered := waitForPaneAt(t, tmux, fx, socket, target, false, "Focused: second-skill", "Search / second-skill", "Focus: 1/1", "? help")
	writeEvidence(t, "ale43-review/hidden-selection/filtered-inventory.txt", []byte(filtered))

	const expected = "Selected: normal-skill (hidden by filters)"
	if !paneContains(filtered, expected) {
		t.Fatalf("filtered inventory does not contain literal retained-selection label %q:\n%s", expected, filtered)
	}
	narrow := resizePaneAt(t, tmux, fx, socket, target, 48, 20, "Focused: second-skill", expected, "? help", "q quit")
	writeEvidence(t, "ale43-review/hidden-selection/filtered-inventory-48x20.txt", []byte(narrow))
}

func TestBrowserRetainedHiddenSelectionIsLabeledInReader(t *testing.T) {
	fx := newFixture(t)
	tmux, socket, target := startALE43ReviewBrowser(
		t,
		fx,
		48,
		20,
		"--root", fx.cleanRoot,
		"--project", fx.project,
	)

	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Selected: normal-skill", "Rendered instructions")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "c")
	hiddenReader := waitForPaneAt(t, tmux, fx, socket, target, false, "Filters: context project", "Selected: normal-skill")
	writeEvidence(t, "ale43-review/hidden-selection/hidden-reader-48x20.txt", []byte(hiddenReader))

	const expected = "Selected: normal-skill (hidden by filters)"
	if !paneContains(hiddenReader, expected) {
		t.Fatalf("filtered reader does not contain literal hidden-selection label %q:\n%s", expected, hiddenReader)
	}
}

func TestBrowserLongHiddenSelectionMarkerSurvivesNarrowLayout(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "roots", "long-hidden-selection")
	longName := strings.Repeat("long-selected-", 6) + "skill"
	writeSkill(t, filepath.Join(root, "a-long"), longName, "selected before filtering", "LONG_SELECTED_INSTRUCTIONS")
	writeSkill(t, filepath.Join(root, "b-visible"), "visible-skill", "visible after filtering", "VISIBLE_INSTRUCTIONS")
	tmux, socket, target := startALE43ReviewBrowser(t, fx, 48, 20, "--root", root, "--project", fx.project)

	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Selected:", "Rendered instructions")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Focused:", "Search /")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "/")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "visible-skill")
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "visible-skill", "Enter keep")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	filtered := waitForPaneAt(t, tmux, fx, socket, target, false, "Focused: visible-skill", "Selected: (hidden by filters)", "Focus: 1/1", "? help", "q quit")
	writeEvidence(t, "ale43-review/hidden-selection/long-marker-48x20.txt", []byte(filtered))
}

func TestBrowserPreservesLongSourceIdentityHints(t *testing.T) {
	fx := newFixture(t)
	rootOne := filepath.Join(fx.base, "allowed", "long-identity", "one", "skills")
	rootTwo := filepath.Join(fx.base, "allowed", "long-identity", "two", "skills")
	longName := strings.Repeat("duplicate-skill-", 8)
	for _, directory := range []string{
		filepath.Join(rootOne, "copy-one"),
		filepath.Join(rootOne, "copy-two"),
		filepath.Join(rootTwo, "copy-three"),
	} {
		writeSkill(t, directory, longName, "Identical source descriptions.", "# Instructions\nIdentical instruction content.")
	}
	before := treeManifest(t, fx.base)
	tmux, socket, target := startALE43ReviewBrowser(t, fx, 120, 30,
		"--root", rootOne,
		"--root", rootTwo,
		"--theme", "none",
	)

	wide := []string{waitForPaneAt(t, tmux, fx, socket, target, false,
		"Focus: 1/3", "Focused:", "Identical instruction content.",
	)}
	for index := 2; index <= 3; index++ {
		wide = append(wide, actForPaneAt(t, tmux, fx, socket, target, false, func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Down")
		}, fmt.Sprintf("Focus: %d/3", index), "Focused:", "Identical instruction content."))
	}
	writeEvidence(t, "ale43-review/long-identity/00-wide.txt", []byte(strings.Join(wide, "\n--- frame ---\n")))

	assertDistinctVisibleIdentities(t, "wide list", longSourceListRows(wide[0], "duplicate-skill-"), 3, 2)
	wideFocus := make([]string, 0, 3)
	widePreview := make([]string, 0, 3)
	hintedFirstRootIndex := -1
	for index, frame := range wide {
		focus := frameLineWithPrefix(t, frame, "Focused: ")
		wideFocus = append(wideFocus, focus)
		widePreview = append(widePreview, longPreviewLabel(t, frame, "duplicate-skill-"))
		if strings.Contains(focus, "#1 skills") && identityHint(focus) != "" {
			hintedFirstRootIndex = index
		}
	}
	assertDistinctVisibleIdentities(t, "wide Focused chrome", wideFocus, 3, 2)
	assertDistinctVisibleIdentities(t, "wide preview title", widePreview, 3, 2)
	if hintedFirstRootIndex < 0 {
		t.Fatalf("no focused source showed both a source hint and first-root provenance:\n%s", strings.Join(wideFocus, "\n"))
	}

	_ = resizePaneAt(t, tmux, fx, socket, target, 48, 30, "Focus: 3/3", "Focused:")
	narrow := []string{actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "g")
	}, "Focus: 1/3", "Focused:")}
	for index := 2; index <= 3; index++ {
		narrow = append(narrow, actForPaneAt(t, tmux, fx, socket, target, false, func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Down")
		}, fmt.Sprintf("Focus: %d/3", index), "Focused:"))
	}
	writeEvidence(t, "ale43-review/long-identity/01-narrow.txt", []byte(strings.Join(narrow, "\n--- frame ---\n")))
	narrowFocus := make([]string, 0, 3)
	for _, frame := range narrow {
		narrowFocus = append(narrowFocus, frameLineWithPrefix(t, frame, "Focused: "))
	}
	assertDistinctVisibleIdentities(t, "narrow Focused chrome", narrowFocus, 3, 2)

	if hintedFirstRootIndex != 2 {
		_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "g")
		}, "Focus: 1/3", "Focused:")
		for index := 1; index <= hintedFirstRootIndex; index++ {
			_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Down")
			}, fmt.Sprintf("Focus: %d/3", index+1), "Focused:")
		}
	}
	reader := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
	}, "Selected:", "Rendered instructions", "Identical instruction content.")
	selected := frameLineWithPrefix(t, reader, "Selected: ")
	readerTitle := longReaderTitle(t, reader, "duplicate-skill-")
	if identityHint(selected) == "" || identityHint(selected) != identityHint(readerTitle) {
		t.Fatalf("selected chrome and reader title do not retain the same source hint:\n%s", reader)
	}
	writeEvidence(t, "ale43-review/long-identity/02-reader.txt", []byte(reader))

	hiddenReader := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "o")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "o")
	}, "Selected: (hidden by filters)", "Source-root filter:")
	hidden := frameLineWithPrefix(t, hiddenReader, "Selected: ")
	if identityHint(hidden) != identityHint(selected) {
		t.Fatalf("hidden selection lost its stable source hint: selected=%q hidden=%q\n%s", selected, hidden, hiddenReader)
	}
	writeEvidence(t, "ale43-review/long-identity/03-hidden-reader.txt", []byte(hiddenReader))

	hiddenInventory := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
	}, "Focused:", "Selected: (hidden by filters)", "Search / type to filter")
	if identityHint(frameLineWithPrefix(t, hiddenInventory, "Selected: ")) != identityHint(selected) {
		t.Fatalf("inventory hidden-selection line lost its stable source hint:\n%s", hiddenInventory)
	}
	writeEvidence(t, "ale43-review/long-identity/04-hidden-inventory.txt", []byte(hiddenInventory))
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("long-identity browser journey mutated its fixture")
	}
}

func TestBrowserHiddenSelectionPreservesDistinctIdentity(t *testing.T) {
	fx := newFixture(t)
	rootOne := filepath.Join(fx.base, "allowed", "hidden-identity", "one", "skills")
	rootTwo := filepath.Join(fx.base, "allowed", "hidden-identity", "two", "skills")
	rootThree := filepath.Join(fx.base, "allowed", "hidden-identity", "three", "skills")
	longName := strings.Repeat("duplicate-skill-", 8)
	for _, directory := range []string{
		filepath.Join(rootOne, "copy-one"),
		filepath.Join(rootOne, "copy-two"),
		filepath.Join(rootTwo, "copy-three"),
		filepath.Join(rootThree, "copy-four"),
	} {
		writeSkill(t, directory, longName, "Identical source descriptions.", "# Instructions\nIdentical instruction content.")
	}
	before := treeManifest(t, fx.base)

	type observation struct {
		id     string
		hidden map[int]string
	}
	observations := make([]observation, 0, 2)
	for _, targetRoot := range []string{"#2 skills", "#3 skills"} {
		tmux, socket, target := startALE43ReviewBrowser(t, fx, 80, 30,
			"--root", rootOne,
			"--root", rootTwo,
			"--root", rootThree,
			"--theme", "none",
		)

		frame := waitForPaneAt(t, tmux, fx, socket, target, false, "Focus: 1/4", "Focused:")
		found := false
		for index := 1; index <= 4; index++ {
			focused := frameLineWithPrefix(t, frame, "Focused: ")
			if strings.Contains(focused, "@ custom · "+targetRoot) {
				found = true
				break
			}
			if index < 4 {
				frame = actForPaneAt(t, tmux, fx, socket, target, false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Down")
				}, fmt.Sprintf("Focus: %d/4", index+1), "Focused:")
			}
		}
		if !found {
			t.Fatalf("could not focus source from %s:\n%s", targetRoot, frame)
		}

		_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
		}, "Selected:", "Rendered instructions")
		details := actForPaneAt(t, tmux, fx, socket, target, false, func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "d")
		}, "Details", "Source ID: src_")
		selectedID := sourceIDFromDetails(t, details)

		hiddenFrame := actForPaneAt(t, tmux, fx, socket, target, false, func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "o")
		}, "Filters: root #1/3 skills", "Selected: (hidden by filters)", "Source ID: "+selectedID)
		if retainedID := sourceIDFromDetails(t, hiddenFrame); retainedID != selectedID {
			t.Fatalf("selection changed while hidden: before=%q after=%q", selectedID, retainedID)
		}
		rootLabel := strings.ReplaceAll(targetRoot, " ", "-")
		hintPrefix := strings.TrimPrefix(selectedID, "src_")
		if len(hintPrefix) > 4 {
			hintPrefix = hintPrefix[:4]
		}
		hiddenByWidth := map[int]string{
			80: frameLineWithPrefix(t, hiddenFrame, "Selected: "),
		}
		writeEvidence(t, "ale43-review/hidden-identity/"+rootLabel+"-80x30.txt", []byte(hiddenFrame))
		for _, width := range []int{48, 120} {
			resized := resizePaneAt(t, tmux, fx, socket, target, width, 30,
				"Selected: (hidden by filters)", "["+hintPrefix, "Source ID: "+selectedID,
			)
			hiddenByWidth[width] = frameLineWithPrefix(t, resized, "Selected: ")
			writeEvidence(t, fmt.Sprintf("ale43-review/hidden-identity/%s-%dx30.txt", rootLabel, width), []byte(resized))
		}
		observations = append(observations, observation{
			id:     selectedID,
			hidden: hiddenByWidth,
		})
	}

	if observations[0].id == observations[1].id {
		t.Fatalf("expected independent selected SourceIDs, got %q", observations[0].id)
	}
	for _, width := range []int{48, 80, 120} {
		first := observations[0].hidden[width]
		second := observations[1].hidden[width]
		if first == second {
			t.Fatalf(
				"distinct retained SourceIDs render the same hidden-selection identity at %d columns: #2=%q #3=%q line=%q",
				width, observations[0].id, observations[1].id, first,
			)
		}
		for _, line := range []string{first, second} {
			if !strings.Contains(line, "Selected: (hidden by filters)") || identityHint(line) == "" {
				t.Fatalf("hidden selection loses its marker or stable hint at %d columns: %q", width, line)
			}
		}
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("hidden-identity journey mutated its fixture")
	}
}

func TestBrowserActiveQueryStaysInChrome(t *testing.T) {
	t.Run("source reader", func(t *testing.T) {
		fx := newALE24Fixture(t)
		tmux, socket, target := startALE43ReviewBrowser(
			t,
			fx.fixture,
			120,
			30,
			"--root", fx.rootA,
			"--project", fx.project,
			"--project-root", fx.rootB,
		)

		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "/")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", "shared-directory")
		_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "shared-directory", "Enter keep")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
		_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Focus: 1/1", "Search / shared-directory", "? help")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
		reader := waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Selected: shared-directory", "Rendered instructions")
		writeEvidence(t, "ale43-review/query-summary/source-reader.txt", []byte(reader))

		const expected = "Filters: search shared-directory"
		if !paneContains(reader, expected) {
			t.Fatalf("source reader does not contain literal active-query summary %q:\n%s", expected, reader)
		}
	})

	t.Run("problem detail", func(t *testing.T) {
		fx := newALE24Fixture(t)
		tmux, socket, target := startALE43ReviewBrowser(
			t,
			fx.fixture,
			120,
			30,
			"--root", fx.rootA,
			"--project", fx.project,
			"--project-root", fx.rootB,
		)

		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Tab")
		_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Problems", "Enter problem")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "/")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", "duplicate-name")
		_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "duplicate-name", "Enter keep")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
		_ = waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Problem focus:", "Search / duplicate-name", "Enter problem", "? help")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
		detail := waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Selected problem: duplicate-name", "Problem lines")
		writeEvidence(t, "ale43-review/query-summary/problem-detail.txt", []byte(detail))

		const expected = "Filters: search duplicate-name"
		if !paneContains(detail, expected) {
			t.Fatalf("problem detail does not contain literal active-query summary %q:\n%s", expected, detail)
		}
	})

	t.Run("narrow reader with context and root filters", func(t *testing.T) {
		fx := newFixture(t)
		tmux, socket, target := startALE43ReviewBrowser(t, fx, 48, 20, "--root", fx.cleanRoot, "--project", fx.project)
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "cc")
		_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Filters: context custom", "normal-skill")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "o")
		_ = waitForPaneAt(t, tmux, fx, socket, target, false, "root clean", "normal-skill")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "/")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "normal-skill")
		_ = waitForPaneAt(t, tmux, fx, socket, target, false, "normal-skill", "Enter keep")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
		_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Filters: s:normal-skill", "c:custom", "r:clean", "Focus: 1/1")
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
		reader := waitForPaneAt(t, tmux, fx, socket, target, false, "Selected: normal-skill", "Filters: s:normal-skill", "c:custom", "r:clean")
		assertActiveFilterDimensions(t, reader, false)
		writeEvidence(t, "ale43-review/query-summary/narrow-reader-48x20.txt", []byte(reader))
	})
}

func TestBrowserHelpListsPagingAndTopEndAliases(t *testing.T) {
	fx := newFixture(t)
	tmux, socket, target := startALE43ReviewBrowser(
		t,
		fx,
		80,
		30,
		"--root", fx.cleanRoot,
		"--project", fx.project,
	)

	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "?")
	help := waitForPaneAt(t, tmux, fx, socket, target, false, "Keys", "Themes:", "q", "Quit")
	writeEvidence(t, "ale43-review/help-keys/help.txt", []byte(help))
	for _, expected := range []string{"PgUp / PgDn", "Home / End"} {
		if !paneContains(help, expected) {
			t.Errorf("complete Help does not contain literal key guidance %q:\n%s", expected, help)
		}
	}
}

func startALE43ReviewBrowser(
	t *testing.T,
	fx fixture,
	width int,
	height int,
	args ...string,
) (string, string, string) {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux is required for ALE-43 finding verification")
	}
	socket := fmt.Sprintf("a43f-%x-%x", os.Getpid(), time.Now().UnixNano()&0xffffff)
	target := "browser:0.0"
	command := fx.productCommand(t, append([]string{"browse"}, args...)...)
	runTmux(
		t,
		tmux,
		fx,
		socket,
		"new-session", "-d", "-x", fmt.Sprint(width), "-y", fmt.Sprint(height),
		"-s", "browser", "exec /bin/sleep 60",
	)
	t.Cleanup(func() { runTmuxIgnoringError(tmux, fx, socket, "kill-server") })
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	runTmux(t, tmux, fx, socket, "set-option", "-w", "-t", "browser:0", "remain-on-exit", "on")
	runTmux(
		t,
		tmux,
		fx,
		socket,
		"resize-window", "-t", "browser:0", "-x", fmt.Sprint(width), "-y", fmt.Sprint(height),
	)
	runTmuxProduct(t, tmux, fx, socket, command, "respawn-pane", "-k", "-t", target)
	_ = waitForPaneAt(t, tmux, fx, socket, target, false, "Skillglass", "Sources", "? help", "q quit")
	return tmux, socket, target
}

func longSourceListRows(frame, namePrefix string) []string {
	lines := strings.Split(frame, "\n")
	rows := make([]string, 0, 3)
	for index, line := range lines {
		left, _, found := strings.Cut(line, "│")
		if !found || !strings.Contains(left, namePrefix) {
			continue
		}
		row := make([]string, 0, 3)
		for offset := 0; offset < 3 && index+offset < len(lines); offset++ {
			cell, _, _ := strings.Cut(lines[index+offset], "│")
			row = append(row, strings.TrimSpace(strings.TrimLeft(cell, "› ")))
		}
		rows = append(rows, strings.Join(row, "\n"))
	}
	return rows
}

func longPreviewLabel(t *testing.T, frame, namePrefix string) string {
	t.Helper()
	for _, line := range strings.Split(frame, "\n") {
		_, right, found := strings.Cut(line, "│")
		if found && strings.Contains(right, namePrefix) {
			return strings.TrimSpace(right)
		}
	}
	t.Fatalf("missing long source preview title:\n%s", frame)
	return ""
}

func longReaderTitle(t *testing.T, frame, namePrefix string) string {
	t.Helper()
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, " "+namePrefix) {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("missing long source reader title:\n%s", frame)
	return ""
}

func frameLineWithPrefix(t *testing.T, frame, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("missing line with prefix %q:\n%s", prefix, frame)
	return ""
}

func assertDistinctVisibleIdentities(t *testing.T, context string, labels []string, count, minimumHints int) {
	t.Helper()
	if len(labels) != count {
		t.Fatalf("%s exposed %d labels, want %d: %q", context, len(labels), count, labels)
	}
	unique := make(map[string]struct{}, count)
	hints := 0
	for _, label := range labels {
		if identityHint(label) != "" {
			hints++
		}
		unique[label] = struct{}{}
	}
	if len(unique) != count {
		t.Fatalf("%s labels are ambiguous: %q", context, labels)
	}
	if hints < minimumHints {
		t.Fatalf("%s exposed %d labels with source hints, want at least %d: %q", context, hints, minimumHints, labels)
	}
}

func identityHint(line string) string {
	start := strings.LastIndex(line, "[")
	if start < 0 {
		return ""
	}
	end := strings.Index(line[start:], "]")
	if end < 0 {
		return ""
	}
	return line[start : start+end+1]
}

func sourceIDFromDetails(t *testing.T, frame string) string {
	t.Helper()
	for _, line := range strings.Split(frame, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Source ID: ") {
			return strings.TrimPrefix(line, "Source ID: ")
		}
	}
	t.Fatalf("missing Source ID in Details:\n%s", frame)
	return ""
}
