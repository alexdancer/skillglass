package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestALE35OrderedInputTerminal(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		for _, tc := range []struct{ name, config, state, permission, skill string }{
			{"numeric", `{"permission":{"skill":{"*":"deny","10":"allow"}}}`, "disabled", "denied", "10"},
			{"tools", `{"tools":{"skill":false},"permission":{"*":"allow","skill":"ask"}}`, "available", "allowed", "10"},
			{"plugins", `{"plugin":["./hooks.js"]}`, "unknown", "unknown", "10"},
			{"markdown", `{}`, "unknown", "unknown", "10"},
			{"explore-default", `{"default_agent":"explore","agent":{"explore":{"mode":"primary"}}}`, "disabled", "denied", "10"},
			{"summary-hidden", `{"default_agent":"summary","agent":{"summary":{}}}`, "unknown", "unknown", "10"},
			{"action-only", `{"permission":{"skill":"allow","question":{"*":"allow"}}}`, "unknown", "unknown", "10"},
			{"utf16-one", `{"permission":{"skill":{"*":"deny","?":"allow"}}}`, "disabled", "denied", "🔍"},
			{"utf16-two-agent", `{"agent":{"build":{"permission":{"skill":{"*":"deny","??":"allow"}}}}}`, "available", "allowed", "🔍"},
			{"directory-exact", `{"permission":{"skill":"deny"}}`, "available", "allowed", "10"},
			{"directory-slash", `{"permission":{"skill":"deny"}}`, "disabled", "denied", "10"},
			{"empty-default-ask", `{"default_agent":"","agent":{"build":{"permission":{"skill":"ask"}}}}`, "ask", "ask", "10"},
			{"empty-default-tools", `{"default_agent":"","agent":{"build":{"tools":{"skill":false}}}}`, "disabled", "denied", "10"},
			{"zero-byte", "", "ask", "ask", "10"},
		} {
			t.Run(fmt.Sprintf("%s-%dx%d", tc.name, size.width, size.height), func(t *testing.T) {
				fx := newFixture(t)
				quotedName, _ := json.Marshal(tc.skill)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "source", string(quotedName))
				opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), tc.config)
				if tc.name == "plugins" {
					opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"plugin":[]}`)
				}
				if tc.name == "zero-byte" {
					opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":"ask"}}`)
				}
				if tc.name == "markdown" {
					writeFile(t, filepath.Join(fx.project, ".opencode", "agents", "override.md"), []byte("---\nname: build\npermission:\n  skill: deny\n---\nInert prompt\n"), 0600)
				}
				var env []string
				if strings.HasPrefix(tc.name, "directory-") {
					opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":"allow"}}`)
					opencodeConfig(t, filepath.Join(fx.project, ".opencode", "opencode.json"), `{"permission":{"skill":"allow"}}`)
					dir := filepath.Join(fx.xdgConfig, "opencode")
					if tc.name == "directory-slash" {
						dir += "/"
					}
					env = []string{"OPENCODE_CONFIG_DIR=" + dir}
				}
				before := treeManifest(t, fx.base)
				ale35PreserveInputs(t, fx, tc.skill, env, before)
				socket := fmt.Sprintf("ale35-ordered-%d-%s-%d", os.Getpid(), tc.name, size.width)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				line := shellQuote(skillglassBinary) + " browse --agent opencode --project " + shellQuote(fx.project) + "; code=$?; printf '\\nORDERED_BROWSER_EXIT=%s\\n' \"$code\"; /bin/sleep 30"
				if len(env) != 0 {
					line = "OPENCODE_CONFIG_DIR=" + shellQuote(strings.TrimPrefix(env[0], "OPENCODE_CONFIG_DIR=")) + " " + line
				}
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser", line)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
				kind := "inferred"
				if tc.state == "unknown" {
					kind = "unknown"
				}
				frame := waitForPane(t, tmux, fx, socket, false, "Agent: opencode", tc.state+" ("+kind+")")
				prefix := "tmux/ordered-inputs/" + t.Name() + "/"
				assertPaneCells(t, frame, size.width, size.height)
				writeEvidence(t, prefix+"sources.txt", []byte(frame))
				detail := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Configured permission: "+tc.permission, "Session: unknown")
				assertPaneCells(t, detail, size.width, size.height)
				writeEvidence(t, prefix+"detail.txt", []byte(detail))
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
				expectedExit := "0"
				if tc.state == "unknown" {
					expectedExit = "1"
				}
				var exit string
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					exit = capturePane(t, tmux, fx, socket, false)
					if strings.Contains(exit, "ORDERED_BROWSER_EXIT="+expectedExit) {
						break
					}
					time.Sleep(25 * time.Millisecond)
				}
				if !strings.Contains(exit, "ORDERED_BROWSER_EXIT="+expectedExit) {
					t.Fatalf("unexpected browser exit: %s", exit)
				}
				writeEvidence(t, prefix+"exit.txt", []byte(exit))
				after := treeManifest(t, fx.base)
				writeEvidence(t, prefix+"tree-after.json", after)
				assertTreeUnchanged(t, before, after)
			})
		}
	}
}
