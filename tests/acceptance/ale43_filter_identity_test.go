package acceptance_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserSameNameAcrossLongRootNames(t *testing.T) {
	fx := newFixture(t)
	prefix := strings.Repeat("root-prefix-", 8)
	roots := []string{
		filepath.Join(fx.base, "allowed", "roots", prefix+"one"),
		filepath.Join(fx.base, "allowed", "roots", prefix+"two"),
	}
	for _, root := range roots {
		writeSkill(t, filepath.Join(root, "copy"), "review", "Identical description.", "IDENTICAL_INSTRUCTIONS")
	}
	before := treeManifest(t, fx.base)
	tmux, socket, target := startALE43ReviewBrowser(t, fx, 48, 30,
		"--root", roots[0], "--root", roots[1], "--theme", "none")
	for _, width := range []int{48, 120} {
		frame := waitForPaneAt(t, tmux, fx, socket, target, false, "Focused:", "Search /")
		if width != 48 {
			_ = resizePaneAt(t, tmux, fx, socket, target, width, 30, "Focused:", "Search /")
			frame = actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "g")
			}, "Focus: 1/2")
		}
		var focused, titles, selected []string
		for index := 1; index <= 2; index++ {
			focused = append(focused, frameLineWithPrefix(t, frame, "Focused: "))
			if width >= 96 {
				if hint := identityHint(longPreviewLabel(t, frame, "review")); hint == "" {
					t.Fatalf("preview lost identity hint:\n%s", frame)
				}
			}
			reader := actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
			}, "Selected:", "IDENTICAL_INSTRUCTIONS")
			titles = append(titles, longReaderTitle(t, reader, "review"))
			selected = append(selected, frameLineWithPrefix(t, reader, "Selected: "))
			writeEvidence(t, fmt.Sprintf("ale43-review/long-roots/%d-reader-%d.txt", width, index), []byte(reader))
			frame = actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
			}, "Focused:", "Search /")
			if index == 1 {
				frame = actForPaneAt(t, tmux, fx, socket, target, false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Down")
				}, "Focus: 2/2")
			}
		}
		assertDistinctVisibleIdentities(t, "focused long roots", focused, 2, 2)
		assertDistinctVisibleIdentities(t, "selected long roots", selected, 2, 2)
		assertDistinctVisibleIdentities(t, "reader long roots", titles, 2, 2)
		var rows []string
		for _, line := range strings.Split(frame, "\n") {
			left, _, _ := strings.Cut(line, "│")
			left = strings.TrimSpace(strings.TrimLeft(left, "› "))
			if strings.HasPrefix(left, "review") {
				rows = append(rows, left)
			}
		}
		assertDistinctVisibleIdentities(t, "inventory long roots", rows, 2, 2)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("identity journey mutated fixtures")
	}
}

func TestBrowserEveryActiveFilterSurvivesWidthAllocation(t *testing.T) {
	for _, problem := range []bool{false, true} {
		t.Run(fmt.Sprint(problem), func(t *testing.T) {
			fx := newFixture(t)
			query := "normal-skill-" + strings.Repeat("long-query-", 12)
			for _, copy := range []string{"one", "two"} {
				writeSkill(t, filepath.Join(fx.cleanRoot, query+copy), "review", query, "FILTERED_INSTRUCTIONS")
			}
			args := []string{"--root", fx.cleanRoot, "--context", "custom", "--source-root", fx.cleanRoot, "--theme", "none"}
			if problem {
				args = append(args, "--problem", "duplicate-name")
			}
			before := treeManifest(t, fx.base)
			tmux, socket, target := startALE43ReviewBrowser(t, fx, 48, 30, args...)
			_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "/")
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "-l", query)
			}, "Enter keep", "Focused: review")
			_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
			}, "Enter open", "Focus: 1/2")
			for _, width := range []int{48, 60, 80, 96, 120} {
				frame := waitForPaneAt(t, tmux, fx, socket, target, false, "Focused:", "Search /")
				if width != 48 {
					frame = resizePaneAt(t, tmux, fx, socket, target, width, 30, "Focused:", "Search /")
				}
				assertActiveFilterDimensions(t, frame, problem)
				for _, action := range []struct{ key, ready, back string }{
					{"Enter", "Selected: review", "Escape"},
					{"s", "Scope and filters", "Escape"},
					{"?", "Keys", "Escape"},
					{"Tab", "Enter problem", "Tab"},
				} {
					frame = actForPaneAt(t, tmux, fx, socket, target, false, func() {
						runTmux(t, tmux, fx, socket, "send-keys", "-t", target, action.key)
					}, action.ready)
					assertActiveFilterDimensions(t, frame, problem)
					writeEvidence(t, fmt.Sprintf("ale43-review/filter-budget/%t-%d-%s.txt", problem, width, action.key), []byte(frame))
					if action.key == "Tab" && problem {
						detail := actForPaneAt(t, tmux, fx, socket, target, false, func() {
							runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Enter")
						}, "Selected problem:", "Problem lines")
						assertActiveFilterDimensions(t, detail, problem)
						_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
							runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
						}, "Enter problem")
					}
					_ = actForPaneAt(t, tmux, fx, socket, target, false, func() {
						runTmux(t, tmux, fx, socket, "send-keys", "-t", target, action.back)
					}, "Search /", "Focus: 1/2")
				}
			}
			if !bytes.Equal(before, treeManifest(t, fx.base)) {
				t.Fatal("filter journey mutated fixtures")
			}
		})
	}
}

func assertActiveFilterDimensions(t *testing.T, frame string, problem bool) {
	t.Helper()
	var summary string
	for _, line := range strings.Split(frame, "\n") {
		if start := strings.Index(line, "Filters: "); start >= 0 {
			summary = line[start:]
			break
		}
	}
	fields := [][2]string{{"search normal", "s:normal"}, {"context custom", "c:cus"}, {"root clean", "r:cle"}}
	if problem {
		fields = append(fields, [2]string{"problem duplicate-name", "p:dup"})
	}
	for _, field := range fields {
		if !strings.Contains(summary, field[0]) && !strings.Contains(summary, field[1]) {
			t.Errorf("active filter %q missing from chrome %q:\n%s", field[0], summary, frame)
		}
	}
}

func TestBrowserDetailsReferenceFilterFlags(t *testing.T) {
	fx := newALE24Fixture(t)
	before := treeManifest(t, fx.base)
	tmux, socket, target := startALE43ReviewBrowser(t, fx.fixture, 120, 80,
		"--root", fx.rootA, "--project", fx.project, "--project-root", fx.rootB, "--root", fx.emptyRoot, "--theme", "none")
	_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "/")
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "-l", "shared-directory")
	}, "Focused: shared-directory", "Enter keep")
	_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
	}, "Focus: 1/1", "? help")
	_ = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "Enter")
	}, "Selected: shared-directory")
	frame := actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
		runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, "d")
	}, "References", "Resources")
	id := sourceIDFromDetails(t, frame)
	assertReferenceFlags(t, frame, true, true)
	for _, step := range []struct {
		key, ready      string
		custom, project bool
	}{
		{"c", "Filters: search shared-directory · context project", false, true},
		{"c", "context custom", true, false},
		{"c", "Context filter: all", true, true},
		{"o", "root ale24-a", true, false},
		{"c", "Context filter: project", false, false},
		{"c", "Context filter: custom", true, false},
		{"c", "Context filter: all", true, false},
		{"o", "root empty", false, false},
		{"o", "root ale24-b", false, true},
		{"o", "Source-root filter: all", true, true},
	} {
		frame = actForPaneAt(t, tmux, fx.fixture, socket, target, false, func() {
			runTmux(t, tmux, fx.fixture, socket, "send-keys", "-t", target, step.key)
		}, step.ready, "Resources")
		assertReferenceFlags(t, frame, step.custom, step.project)
	}
	read := fx.run(t, "read", "--root", fx.rootA, "--project", fx.project, "--project-root", fx.rootB, "--json", id)
	assertExit(t, read, 0)
	var output struct {
		References []publicReference `json:"references"`
	}
	decodeJSON(t, read.stdout, &output)
	if len(output.References) != 2 {
		t.Fatalf("CLI read lost references: %s", read.stdout)
	}
	for _, reference := range output.References {
		if !reference.MatchesFilters {
			t.Fatalf("unfiltered CLI read changed reference semantics: %s", read.stdout)
		}
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("reference journey mutated fixtures")
	}
}

func assertReferenceFlags(t *testing.T, frame string, custom, project bool) {
	t.Helper()
	flags := make(map[string]string)
	context := ""
	for _, line := range strings.Split(frame, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Context: ") {
			context = strings.TrimPrefix(line, "Context: ")
		}
		if strings.HasPrefix(line, "Matches filters: ") {
			flags[context] = strings.TrimPrefix(line, "Matches filters: ")
		}
	}
	if len(flags) != 2 || flags["custom"] != fmt.Sprint(custom) || flags["project"] != fmt.Sprint(project) {
		t.Fatalf("reference flags = %v, want custom=%t project=%t:\n%s", flags, custom, project, frame)
	}
}

func TestBrowserRejectsAdditionalThemeSpellings(t *testing.T) {
	fx := newFixture(t)
	for _, value := range []string{"mono", "Dark", " dark ", "NONE", " Auto "} {
		result := fx.run(t, "browse", "--root", fx.cleanRoot, "--theme", value)
		assertExit(t, result, 2)
		if !bytes.Contains(result.stderr, []byte("unknown browser theme")) {
			t.Fatalf("theme %q did not fail at argument validation: %s", value, result.stderr)
		}
	}
}
