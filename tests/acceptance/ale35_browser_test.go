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

func TestALE35OpenCodeTerminalExplanationFilterRefreshAndScope(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			fx, _ := codexFixture(t)
			root := filepath.Join(fx.xdgConfig, "opencode", "skills")
			codexSkill(t, root, strings.Repeat("long-folder-", 8), "browser-opencode")
			config := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
			opencodeConfig(t, config, `{"permission":{"skill":{"browser-opencode":"ask"}}}`)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale35-%d-%d", os.Getpid(), size.width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--agent", "opencode", "--project", fx.project, "--availability", "ask")
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
			pane := waitForPane(t, tmux, fx, socket, false, "Agent: opencode", "browser-opencode", "ask (inferred)")
			assertPaneCells(t, pane, size.width, size.height)
			writeEvidence(t, fmt.Sprintf("tmux/ale35/%dx%d/01-sources.txt", size.width, size.height), []byte(pane))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("Enter", "Rendered instructions")
			top := send("d", "Read lines", "Availability: ask", "Configured permission: ask", "Session: unknown")
			assertPaneCells(t, top, size.width, size.height)
			collect := func(prefix string, needles ...string) {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "g")
				frame := waitForPane(t, tmux, fx, socket, false, "Read lines 1-")
				rows := map[int]string{}
				for page := 0; page < 30; page++ {
					first, last, total, body := sourceDetailBody(t, frame)
					for index, row := range body {
						rows[first+index] = row
					}
					assertPaneCells(t, frame, size.width, size.height)
					writeEvidence(t, fmt.Sprintf("tmux/ale35/%dx%d/%s-%02d.txt", size.width, size.height, prefix, page), []byte(frame))
					ordered := make([]string, total)
					for index := range ordered {
						ordered[index] = rows[index+1]
					}
					joined := strings.Join(ordered, "\n")
					found := true
					for _, needle := range needles {
						found = found && paneContains(joined, needle)
					}
					if found {
						return
					}
					if last == total {
						t.Fatalf("detail missing %v:\n%s", needles, joined)
					}
					previous := first
					frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool { first, _, _ := sourceDetailRange(next.cells); return first > previous }).cells
				}
				t.Fatal("detail pager failed to expose explanation")
			}
			collect("02-ask", "Permission: ask", "ordered-skill-permission", "Session: unknown", "Reload: unknown")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			opencodeConfig(t, config, `{"permission":{"skill":{"browser-opencode":"deny"}}}`)
			before = treeManifest(t, fx.base)
			send("r", "Refresh: selected directory object retained")
			collect("03-deny", "Availability: disabled", "Permission: denied", "permission.skill.browser-opencode", "Session: unknown")
			hidden := send("Escape", "0 matches", "hidden by filters")
			assertPaneCells(t, hidden, size.width, size.height)
			// ask -> shadowed -> unavailable -> unknown -> all -> available -> disabled.
			for _, state := range []string{"shadowed", "unavailable", "unknown", "all", "available", "disabled"} {
				send("a", "Availability filter: "+state)
			}
			filter := waitForPane(t, tmux, fx, socket, false, "browser-opencode", "disabled (inferred)")
			assertPaneCells(t, filter, size.width, size.height)
			writeEvidence(t, fmt.Sprintf("tmux/ale35/%dx%d/04-filter.txt", size.width, size.height), []byte(filter))
			scope := send("s", "Scope and filters", "Agent: opencode", "Validated rules: OpenCode 1.17.15")
			assertPaneCells(t, scope, size.width, size.height)
			writeEvidence(t, fmt.Sprintf("tmux/ale35/%dx%d/05-scope.txt", size.width, size.height), []byte(scope))
			send("Escape", "Search /")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE35InvalidConfigTerminalEvidence(t *testing.T) {
	ale35InvalidConfigTerminalEvidence(t, `{"permission":null}`, "ale35-invalid")
}

func TestALE35InvalidAvailabilityConfigTerminalEvidence(t *testing.T) {
	ale35InvalidConfigTerminalEvidence(t, `{"agent":{"build":{"disable":"false"}}}`, "ale35-invalid-availability")
}

func TestALE35LegacyConfigTerminalEvidence(t *testing.T) {
	ale35UnknownConfigTerminalEvidence(t, "[permission]\nskill = \"deny\"\n", "ale35-legacy", "config")
}

func ale35InvalidConfigTerminalEvidence(t *testing.T, configText, evidenceName string) {
	ale35UnknownConfigTerminalEvidence(t, configText, evidenceName, "opencode.json")
}

func ale35UnknownConfigTerminalEvidence(t *testing.T, configText, evidenceName, inputName string) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), strings.Repeat("long-folder-", 8), "invalid-input-source")
			config := filepath.Join(fx.xdgConfig, "opencode", inputName)
			opencodeConfig(t, config, configText)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale35-invalid-%d-%d", os.Getpid(), size.width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--agent", "opencode", "--project", fx.project, "--availability", "unknown")
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
			initial := waitForPane(t, tmux, fx, socket, false, "invalid-input-source", "unknown (unknown)")
			prefix := fmt.Sprintf("tmux/%s/%dx%d/", evidenceName, size.width, size.height)
			capture := func(name, frame string) {
				assertPaneCells(t, frame, size.width, size.height)
				writeEvidence(t, prefix+name+".txt", []byte(frame))
			}
			capture("01-sources", initial)
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("Enter", "Rendered instructions")
			detail := send("d", "Availability: unknown", "Configured permission: unknown", "Session: unknown")
			capture("02-explanation", detail)
			var explained strings.Builder
			for page := 0; page < 40; page++ {
				first, last, total, body := sourceDetailBody(t, detail)
				explained.WriteString(strings.Join(body, "\n"))
				explained.WriteByte('\n')
				capture(fmt.Sprintf("02-explanation-page-%02d", page), detail)
				if last == total {
					break
				}
				previous := first
				detail = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool { first, _, _ := sourceDetailRange(next.cells); return first > previous }).cells
			}
			for _, needle := range []string{"Permission: unknown", "opencode/" + inputName, "Session: unknown"} {
				if !paneContains(explained.String(), needle) {
					t.Fatalf("unknown explanation omitted %s", needle)
				}
			}
			send("Escape", "Search /")
			problems := send("Tab", "Enter problem", "native-input")
			capture("03-problems", problems)
			problem := send("Enter", "Selected problem:", "Code: native-input")
			capture("04-problem", problem)
			send("Escape", "Enter problem")
			scope := send("s", "Scope and filters", "Agent: opencode", "Validated rules: OpenCode 1.17.15")
			capture("06-scope", scope)
			send("Escape", "Enter problem")
			send("Tab", "Focused: invalid-input-source", "Enter open")
			filtered := send("a", "Availability filter: all", "invalid-input-source")
			capture("07-filter", filtered)
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
