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

func TestALE29MultipleRegistrationBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			fx, _ := codexFixture(t)
			outer := filepath.Join(fx.base, "allowed", "outer")
			inner := filepath.Join(outer, "inner")
			plugin := filepath.Join(inner, "demo")
			claudeSkill(t, filepath.Join(plugin, "skills"), "work", "work", "")
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"), map[string]any{"name": "demo", "version": "1"})
			markets, installed := map[string]any{}, map[string]any{}
			for _, route := range []struct{ id, path, source string }{{"outer", outer, "./inner/demo"}, {"inner", inner, "./demo"}} {
				claudeSettings(t, filepath.Join(route.path, ".claude-plugin", "marketplace.json"), map[string]any{"name": route.id, "plugins": []map[string]string{{"name": "demo", "source": route.source}}})
				markets[route.id] = map[string]any{"source": map[string]string{"source": "directory", "path": route.path}, "installLocation": route.path, "lastUpdated": "2026-01-01T00:00:00Z"}
				installed["demo@"+route.id] = []map[string]any{{"scope": "user", "installPath": plugin, "version": "1"}}
			}
			plugins := filepath.Join(fx.home, ".claude", "plugins")
			claudeSettings(t, filepath.Join(plugins, "known_marketplaces.json"), markets)
			claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": installed})
			claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@outer": false, "demo@inner": true}})
			prefix := fmt.Sprintf("tmux/multireg/%dx%d/", size.width, size.height)
			profile, err := os.ReadFile(fx.policy)
			if err != nil {
				t.Fatal(err)
			}
			writeEvidence(t, prefix+"policy.sb", profile)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("multireg-%d", size.width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--agent", "claude-code", "--project", fx.project, "--availability", "unknown")
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
			frame := waitForPane(t, tmux, fx, socket, false, "Skillglass", "unknown (unknown)", "Focus: 1/1")
			writeEvidence(t, prefix+"sources.txt", []byte(frame))
			frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Read lines")
			rows := map[int]string{}
			for page := 0; page < 100; page++ {
				assertPaneCells(t, frame, size.width, size.height)
				first, last, total, body := sourceDetailBody(t, frame)
				for i, row := range body {
					rows[first+i] = row
				}
				writeEvidence(t, fmt.Sprintf("%sdetail-page-%02d.txt", prefix, page), []byte(frame))
				if last == total {
					ordered := make([]string, total)
					for i := range ordered {
						var ok bool
						ordered[i], ok = rows[i+1]
						if !ok {
							t.Fatal("browser body row missing")
						}
					}
					observed := strings.Join(ordered, "\n")
					writeEvidence(t, prefix+"detail-body.txt", []byte(observed))
					for _, needle := range []string{"Availability: unknown (unknown)", "multiple-plugin-registrations", "Plugin: demo@outer", "Plugin: demo@inner", "enabledPlugins.demo@outer=false", "enabledPlugins.demo@inner=true", "plugin-disabled", "Plugin observation: marketplace", "Plugin observation: installation", "demo:work", "Precedence: unknown", "Session: unknown", "unsupported"} {
						if !paneContains(observed, needle) {
							t.Errorf("browser lacks %q:\n%s", needle, observed)
						}
					}
					if paneContains(observed, "Winning source:") {
						t.Error("browser invented a registration winner")
					}
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
					assertTreeUnchanged(t, before, treeManifest(t, fx.base))
					return
				}
				frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool {
					nextFirst, _, _ := sourceDetailRange(next.cells)
					return nextFirst > first
				}).cells
			}
			t.Fatal("multi-registration pager did not finish")
		})
	}
}
