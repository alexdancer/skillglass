package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestALE26AvailabilityEmptyMessages(t *testing.T) {
	fx, _ := codexFixture(t)
	before := treeManifest(t, fx.base)
	t.Cleanup(func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) })
	for _, state := range []string{"", "all", "available", "disabled"} {
		t.Run("native-"+state, func(t *testing.T) {
			args := []string{"list", "--agent", "codex", "--project", fx.project}
			if state != "" {
				args = append(args, "--availability", state)
			}
			result := fx.run(t, args...)
			assertExit(t, result, 0)
			writeEvidence(t, "empty-filter/cli/native-"+state+".txt", result.stdout)
			want := "Sources: 1\n"
			if state == "disabled" {
				want = "Sources: no matches\n"
			}
			if !strings.Contains(string(result.stdout), want) {
				t.Fatalf("want %q in public output:\n%s", want, result.stdout)
			}
		})
	}
	for _, state := range []string{"", "all", "disabled"} {
		t.Run("empty-"+state, func(t *testing.T) {
			args := []string{"list", "--root", fx.emptyRoot}
			if state != "" {
				args = append(args, "--availability", state)
			}
			result := fx.run(t, args...)
			assertExit(t, result, 0)
			want := "Sources: none found\n"
			if state == "disabled" {
				want = "Sources: no matches\n"
			}
			if !strings.Contains(string(result.stdout), want) {
				t.Fatalf("want %q in public output:\n%s", want, result.stdout)
			}
		})
	}
}

func TestALE26BrowserAvailabilityEmptyAndGenericShortcut(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		for _, kind := range []string{"native-initial", "native-cycle", "generic-sources", "generic-problems", "generic-empty"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size.width, size.height, kind), func(t *testing.T) {
				fx := newFixture(t)
				scope := "--root " + shellQuote(fx.cleanRoot)
				if strings.HasPrefix(kind, "native-") {
					fx, _ = codexFixture(t)
					scope = "--agent codex --project " + shellQuote(fx.project) + " --availability "
					if kind == "native-initial" {
						scope += "disabled"
					} else {
						scope += "available"
					}
				} else if kind == "generic-empty" {
					scope = "--root " + shellQuote(fx.emptyRoot)
				}
				before := treeManifest(t, fx.base)
				t.Cleanup(func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) })
				socket := fmt.Sprintf("ale26-empty-%d-%d-%s", os.Getpid(), size.width, kind)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser", "exec "+fx.productShell(t)+" browse "+scope)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
				pane := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "Root filter: all")
				prefix := fmt.Sprintf("empty-filter/tmux/%dx%d/%s/", size.width, size.height, kind)
				writeEvidence(t, prefix+"before.txt", []byte(pane))
				send := func(keys []string, needles ...string) string {
					return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
						args := append([]string{"send-keys", "-t", "browser:0.0"}, keys...)
						runTmux(t, tmux, fx, socket, args...)
					}, needles...)
				}
				switch kind {
				case "native-cycle":
					pane = send([]string{"a"}, "Availability filter: disabled")
				case "generic-sources", "generic-problems":
					if kind == "generic-problems" {
						send([]string{"Tab"}, "SKILLGLASS | Problems")
					}
					send([]string{"a", "s"}, "SKILLGLASS | Scope and filters")
					writeEvidence(t, prefix+"scope.txt", []byte(capturePane(t, tmux, fx, socket, false)))
					title := "SKILLGLASS | Sources"
					if kind == "generic-problems" {
						title = "SKILLGLASS | Problems"
					}
					pane = send([]string{"Escape"}, title)
					if kind == "generic-problems" {
						pane = send([]string{"Tab"}, "SKILLGLASS | Sources")
					}
				}
				writeEvidence(t, prefix+"after.txt", []byte(pane))
				assertPaneCells(t, pane, size.width, size.height)
				switch kind {
				case "native-initial", "native-cycle":
					if !paneContains(pane, "No matching sources.") || !paneContains(pane, "0 matches") || paneContains(pane, "No sources found") || paneContains(pane, "0 sources") {
						t.Fatalf("availability-only empty result misreports inventory:\n%s", pane)
					}
				case "generic-sources", "generic-problems":
					if !paneContains(pane, "normal-skill") || !paneContains(pane, "second-skill") || !paneContains(pane, "Focus: 1/2") || paneContains(pane, "Availability filter:") {
						t.Fatalf("generic availability shortcut changed inventory:\n%s", pane)
					}
				case "generic-empty":
					if !paneContains(pane, "No sources found in the selected roots.") || !paneContains(pane, "0 sources") {
						t.Fatalf("unfiltered empty inventory changed:\n%s", pane)
					}
				}
			})
		}
	}
}
