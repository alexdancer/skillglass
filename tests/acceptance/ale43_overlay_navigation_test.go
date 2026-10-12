package acceptance_test

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBrowserHelpPreservesUnderlyingChrome(t *testing.T) {
	for _, width := range []int{48, 120} {
		width := width
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx := newALE24Fixture(t)
			before := treeManifest(t, fx.base)
			tmux, socket, target := startALE43ReviewBrowser(
				t,
				fx.fixture,
				width,
				30,
				"--root", fx.rootA,
				"--project", fx.project,
				"--project-root", fx.rootB,
				"--theme", "dark",
			)

			initial := waitForPaneAt(t, tmux, fx.fixture, socket, target, false, "Focused:", "Search / type to filter")
			sourceFocus := frameLineWithPrefix(t, initial, "Focused: ")
			assertDirectHelpChrome(t, tmux, fx.fixture, socket, target, width, "sources", "Sources", sourceFocus, "Search / type to filter")
			assertScopeHelpChrome(t, tmux, fx.fixture, socket, target, width, "sources", "Sources", sourceFocus, "Search / type to filter")

			_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "/")
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", "normal-a")
			}, "Search / normal-a█", "Enter keep")
			_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
			}, "Search / normal-a", "Focused: normal-a", "Focus: 1/1")
			reader := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
			}, "Selected: normal-a", "Rendered instructions")
			selectedSource := frameLineWithPrefix(t, reader, "Selected: ")
			assertDirectHelpChrome(t, tmux, fx.fixture, socket, target, width, "source-reader", "Sources", selectedSource, "Rendered instructions")
			assertScopeHelpChrome(t, tmux, fx.fixture, socket, target, width, "source-reader", "Sources", selectedSource, "Rendered instructions")

			hiddenReader := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", "c")
			}, "Filters:", "normal-a", "context project", "Selected:", "hidden by filters")
			hiddenSelection := frameLineWithPrefix(t, hiddenReader, "Selected: ")
			assertDirectHelpChrome(t, tmux, fx.fixture, socket, target, width, "hidden-reader", "Sources", hiddenSelection, "Rendered instructions")
			assertScopeHelpChrome(t, tmux, fx.fixture, socket, target, width, "hidden-reader", "Sources", hiddenSelection, "Rendered instructions")

			hiddenInventory := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Escape")
			}, "Focused: no matching sources", "Selected:", "hidden by filters", "No matching sources.")
			if got := frameLineWithPrefix(t, hiddenInventory, "Selected: "); got != hiddenSelection {
				t.Fatalf("hidden inventory identity = %q, want reader identity %q\n%s", got, hiddenSelection, hiddenInventory)
			}
			assertDirectHelpChrome(t, tmux, fx.fixture, socket, target, width, "hidden-inventory", "Sources", hiddenSelection, "No matching sources.")
			assertScopeHelpChrome(t, tmux, fx.fixture, socket, target, width, "hidden-inventory", "Sources", hiddenSelection, "No matching sources.")

			_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "/")
			}, "Enter keep", "Esc cancel")
			_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "C-u")
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
			}, "Search / type to filter", "Filters: context project")
			_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", "cc")
			}, "Filters: all", "Focused:")

			problems := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Tab")
			}, "Enter problem")
			problemFocus := frameLineWithPrefix(t, problems, "Focused: ")
			assertDirectHelpChrome(t, tmux, fx.fixture, socket, target, width, "problems", "Problems", problemFocus, "Enter problem")
			assertScopeHelpChrome(t, tmux, fx.fixture, socket, target, width, "problems", "Problems", problemFocus, "Enter problem")

			problemDetail := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
			}, "Selected problem:", "Problem lines")
			selectedProblem := frameLineWithPrefix(t, problemDetail, "Selected problem: ")
			assertDirectHelpChrome(t, tmux, fx.fixture, socket, target, width, "problem-detail", "Problems", selectedProblem, "Problem lines")
			assertScopeHelpChrome(t, tmux, fx.fixture, socket, target, width, "problem-detail", "Problems", selectedProblem, "Problem lines")

			if !bytes.Equal(before, treeManifest(t, fx.base)) {
				t.Fatal("Help overlay journey mutated its fixture")
			}
		})
	}
}

func TestBrowserSearchCancellationUsesEscapeOnly(t *testing.T) {
	for _, test := range []struct {
		name        string
		query       string
		openMode    func(t *testing.T, tmux string, fx fixture, socket, target string)
		returnLabel string
	}{
		{
			name:        "sources",
			query:       "normal-a",
			openMode:    func(*testing.T, string, fixture, string, string) {},
			returnLabel: "Enter open",
		},
		{
			name:  "problems",
			query: "duplicate-name",
			openMode: func(t *testing.T, tmux string, fx fixture, socket, target string) {
				_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Tab")
				}, "Problem focus:", "Enter problem")
			},
			returnLabel: "Enter problem",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fx := newALE24Fixture(t)
			before := treeManifest(t, fx.base)
			tmux, socket, target := startALE43ReviewBrowser(
				t,
				fx.fixture,
				80,
				30,
				"--root", fx.rootA,
				"--project", fx.project,
				"--project-root", fx.rootB,
				"--theme", "none",
			)
			test.openMode(t, tmux, fx.fixture, socket, target)

			editing := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "/")
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", test.query)
			}, "Search / "+test.query+"█", "Enter keep", "Esc cancel")
			writeEvidence(t, "ale43-review/search-cancel/"+test.name+"-before-left.txt", []byte(editing))
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Left")
			time.Sleep(250 * time.Millisecond)
			afterLeft := capturePaneAt(t, tmux, fx.fixture, socket, target, false)
			writeEvidence(t, "ale43-review/search-cancel/"+test.name+"-after-left.txt", []byte(afterLeft))
			if afterLeft != editing {
				t.Fatalf("Left changed active %s search:\nbefore:\n%s\nafter:\n%s", test.name, editing, afterLeft)
			}

			cancelled := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
				runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Escape")
			}, "Search / type to filter", test.returnLabel, "? help", "q quit")
			writeEvidence(t, "ale43-review/search-cancel/"+test.name+"-after-escape.txt", []byte(cancelled))
			if paneContains(cancelled, "Esc cancel") {
				t.Fatalf("Escape left %s search editor active:\n%s", test.name, cancelled)
			}
			if !bytes.Equal(before, treeManifest(t, fx.base)) {
				t.Fatalf("%s search cancellation mutated its fixture", test.name)
			}
		})
	}
}

func assertDirectHelpChrome(
	t *testing.T,
	tmux string,
	fx fixture,
	socket string,
	target string,
	width int,
	name string,
	activeTab string,
	wantIdentity string,
	returnNeedle string,
) {
	t.Helper()
	help := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "?")
	}, "Keys", wantIdentity)
	assertChromeIdentity(t, help, wantIdentity)
	writeEvidence(t, fmt.Sprintf("ale43-review/overlay-chrome/%d/%s-direct-help.txt", width, name), []byte(help))
	ansiFrame := captureStyledPane(t, tmux, fx, socket, target)
	assertActiveChromeTab(t, ansiFrame, activeTab)
	writeEvidence(t, fmt.Sprintf("ale43-review/overlay-chrome/%d/%s-direct-help.ansi", width, name), []byte(ansiFrame))
	returned := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "?")
	}, returnNeedle, wantIdentity)
	assertChromeIdentity(t, returned, wantIdentity)
}

func assertScopeHelpChrome(
	t *testing.T,
	tmux string,
	fx fixture,
	socket string,
	target string,
	width int,
	name string,
	activeTab string,
	wantIdentity string,
	returnNeedle string,
) {
	t.Helper()
	scope := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", "s")
	}, "Scope and filters", wantIdentity)
	assertChromeIdentity(t, scope, wantIdentity)
	help := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "?")
	}, "Keys", wantIdentity)
	assertChromeIdentity(t, help, wantIdentity)
	writeEvidence(t, fmt.Sprintf("ale43-review/overlay-chrome/%d/%s-scope-help.txt", width, name), []byte(help))
	ansiFrame := captureStyledPane(t, tmux, fx, socket, target)
	assertActiveChromeTab(t, ansiFrame, activeTab)
	writeEvidence(t, fmt.Sprintf("ale43-review/overlay-chrome/%d/%s-scope-help.ansi", width, name), []byte(ansiFrame))
	returnedScope := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "?")
	}, "Scope and filters", wantIdentity)
	assertChromeIdentity(t, returnedScope, wantIdentity)
	returned := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Left")
	}, returnNeedle, wantIdentity)
	assertChromeIdentity(t, returned, wantIdentity)
}

func assertChromeIdentity(t *testing.T, frame, want string) {
	t.Helper()
	label, _, found := strings.Cut(want, ":")
	if !found {
		t.Fatalf("identity line has no label separator: %q", want)
	}
	if got := frameLineWithPrefix(t, frame, label+": "); got != want {
		t.Fatalf("chrome identity = %q, want %q\n%s", got, want, frame)
	}
}

func captureStyledPane(t *testing.T, tmux string, fx fixture, socket, target string) string {
	t.Helper()
	return runTmux(t, tmux, fx, socket, "capture-pane", "-p", "-e", "-t", target)
}

func assertActiveChromeTab(t *testing.T, frame, want string) {
	t.Helper()
	matches := regexp.MustCompile(`\x1b\[1m(?:\x1b\[[0-9;]+m)*(Sources|Problems [0-9]+)`).FindAllStringSubmatch(frame, -1)
	if len(matches) != 1 {
		t.Fatalf("styled chrome exposes %d active tabs, want one:\n%q", len(matches), frame)
	}
	got := strings.Fields(matches[0][1])[0]
	if got != want {
		t.Fatalf("active chrome tab = %q, want %q:\n%q", got, want, frame)
	}
}
