package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestALE26BrowserAvailabilityExplanationFilterAndRefresh(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {48, 30}, {80, 30}} {
		width, height := size.width, size.height
		t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
			fx, _ := codexFixture(t)
			manifest := codexSkill(t, filepath.Join(fx.home, ".agents", "skills"), strings.Repeat("long-source-path-", 5), "browser-codex")
			config := filepath.Join(fx.home, ".codex", "config.toml")
			writeFile(t, config, nil, 0600)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale26-%d-%d-%d", os.Getpid(), width, height)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec " + shellQuote(skillglassBinary) + " browse --agent codex --project " + shellQuote(fx.project) + " --availability available"
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", strconv.Itoa(height), "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", strconv.Itoa(height))
			pane := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "browser-codex", "available (inferred)")
			assertPaneCells(t, pane, width, height)
			writeEvidence(t, fmt.Sprintf("tmux/ale26/%dx%d/01-sources.txt", width, height), []byte(pane))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "browser-codex")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter read")
			topSummary := send("Enter", "Read lines", "Availability: available", "Configured permission: allowed", "Session: unknown")
			assertPaneCells(t, topSummary, width, height)
			collect := func(prefix string, needles ...string) string {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "g")
				frame := waitForPane(t, tmux, fx, socket, false, "Read lines 1-")
				rows := make(map[int]string)
				var collected string
				for page := 0; page < 25; page++ {
					assertPaneCells(t, frame, width, height)
					first, last, total, body := sourceDetailBody(t, frame)
					for index, row := range body {
						rows[first+index] = row
					}
					ordered := make([]string, total)
					for index := range ordered {
						var seen bool
						ordered[index], seen = rows[index+1]
						if !seen {
							ordered[index] = "<unobserved row>"
						}
					}
					collected = strings.Join(ordered, "\n")
					writeEvidence(t, fmt.Sprintf("tmux/ale26/%dx%d/%s-page-%02d.txt", width, height, prefix, page), []byte(frame))
					if page == 0 {
						writeEvidence(t, fmt.Sprintf("tmux/ale26/%dx%d/%s-top.txt", width, height, prefix), []byte(frame))
					}
					found := true
					for _, needle := range needles {
						found = found && paneContains(collected, needle)
					}
					if found {
						return collected
					}
					if last == total {
						break
					}
					previousFirst := first
					frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool {
						first, _, _ := sourceDetailRange(next.cells)
						return first > previousFirst
					}).cells
				}
				for _, needle := range needles {
					if !paneContains(collected, needle) {
						t.Fatalf("missing detail %q from observed body rows:\n%s", needle, collected)
					}
				}
				return collected
			}
			collect("02-before", "Availability: available", "Permission: allowed", "Session: unknown", "Reload: unknown")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeFile(t, config, []byte(selector(manifest, false)), 0600)
			before = treeManifest(t, fx.base)
			send("r", "Refresh: selected directory object retained")
			collect("03-after", "Availability: disabled", "Permission: denied", "manifest-selector", "Session: unknown")
			hidden := send("Escape", "SKILLGLASS | Sources", "0 matches", "hidden by filters")
			assertPaneCells(t, hidden, width, height)
			filtered := send("a", "Availability filter: disabled", "browser-codex", "disabled (inferred)")
			assertPaneCells(t, filtered, width, height)
			writeEvidence(t, fmt.Sprintf("tmux/ale26/%dx%d/04-filter.txt", width, height), []byte(filtered))
			scope := send("s", "SKILLGLASS | Scope and filters", "Validated: codex-cli 0.155.1")
			assertPaneCells(t, scope, width, height)
			writeEvidence(t, fmt.Sprintf("tmux/ale26/%dx%d/05-scope.txt", width, height), []byte(scope))
			send("Escape", "SKILLGLASS | Sources")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE26BrowserContextCycleRecoversGenericAndPreservesNativeOrder(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, initial := range []string{"global", "system", "managed", "native"} {
		t.Run(initial, func(t *testing.T) {
			fx := newFixture(t)
			scope := "--root " + shellQuote(fx.cleanRoot) + " --context " + initial
			if initial == "native" {
				fx, _ = codexFixture(t)
				scope = "--agent codex --project " + shellQuote(fx.project)
			}
			socket := fmt.Sprintf("ale26-context-%d-%s", os.Getpid(), initial)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "48", "-y", "24", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse "+scope)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", "48", "-y", "24")
			current := initial
			if initial == "native" {
				current = "all"
			}
			before := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "Context filter: "+current)
			writeEvidence(t, "tmux/ale26/context/"+initial+"-before.txt", []byte(before))
			next := []string{"all"}
			if initial == "native" {
				next = []string{"project", "custom", "global", "system", "managed", "all"}
			}
			for _, context := range next {
				after := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "c") }, "Context filter: "+context)
				assertPaneCells(t, after, 48, 24)
				if initial != "native" && !paneContains(after, "normal-skill") {
					t.Fatal("context control failed to reveal generic fixture")
				}
				writeEvidence(t, "tmux/ale26/context/"+initial+"-"+context+".txt", []byte(after))
			}
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
		})
	}
}

// Reconstruct the visible read body by its public numbered pager. Chrome between
// pages must not interrupt words wrapped across two adjacent body rows.
func sourceDetailBody(t *testing.T, cells string) (first, last, total int, body []string) {
	t.Helper()
	rows := strings.Split(strings.TrimSuffix(cells, "\n"), "\n")
	start := -1
	for index, row := range rows {
		if strings.HasPrefix(row, "Selected:") {
			start = index + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("source detail lacks selection header:\n%s", cells)
	}
	if start < len(rows) && strings.HasPrefix(rows[start], "Selector:") {
		start++
	}
	if start < len(rows) && strings.HasPrefix(rows[start], "Selection:") {
		start++
	}
	first, last, total = sourceDetailRange(cells)
	count := last - first + 1
	if first < 1 || count < 1 || start+count > len(rows) {
		t.Fatalf("invalid source pager/body bounds:\n%s", cells)
	}
	return first, last, total, rows[start : start+count]
}

func sourceDetailRange(cells string) (first, last, total int) {
	for _, row := range strings.Split(cells, "\n") {
		if strings.HasPrefix(row, "Read lines ") {
			if count, err := fmt.Sscanf(row, "Read lines %d-%d of %d", &first, &last, &total); err == nil && count == 3 {
				return first, last, total
			}
		}
	}
	return 0, 0, 0
}

func advanceSourceDetailFrame(t *testing.T, tmux string, fx fixture, socket, frame string) (string, bool) {
	t.Helper()
	start, end, total := sourceDetailRange(frame)
	if start == 0 {
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "Read lines ") {
				t.Fatalf("invalid source detail footer: %s", line)
			}
		}
		return frame, false
	}
	next := min(start+3, total-(end-start))
	if next <= start {
		return frame, false
	}
	return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down", "Down", "Down")
	}, fmt.Sprintf("Read lines %d-", next)), true
}
