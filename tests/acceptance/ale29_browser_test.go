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

func TestALE29ClaudeBrowserExplainsFiltersAndRefreshesWithoutWrites(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			fx, repo := codexFixture(t)
			root := filepath.Join(fx.home, ".claude", "skills")
			claudeSkill(t, root, strings.Repeat("long-claude-source-", 5), "browser-claude", "disable-model-invocation: YeS\n")
			claudeSkill(t, filepath.Join(repo, ".claude", "skills"), "shadow", "browser-claude", "")
			settings := filepath.Join(fx.project, ".claude", "settings.local.json")
			claudeSettings(t, settings, map[string]any{})
			claudeRootPluginFixture(t, fx, nil, false)
			claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}})
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale29-%d-%d", os.Getpid(), size.width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec " + fx.productShell(t) + " browse --agent claude-code --project " + shellQuote(fx.project) + " --availability explicit-only"
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
			first := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "Agent: claude-code", "browser-claude", "explicit-only (inferred)")
			assertPaneCells(t, first, size.width, size.height)
			writeEvidence(t, fmt.Sprintf("tmux/ale29/%dx%d/01-sources.txt", size.width, size.height), []byte(first))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			send("Enter", "Read lines", "Availability: explicit-only")
			collect := func(label string, needles ...string) {
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "g")
				frame := waitForPane(t, tmux, fx, socket, false, "Read lines 1-")
				rows := map[int]string{}
				for page := 0; page < 60; page++ {
					assertPaneCells(t, frame, size.width, size.height)
					first, last, total, body := sourceDetailBody(t, frame)
					for i, row := range body {
						rows[first+i] = row
					}
					ordered := make([]string, total)
					for i := range ordered {
						ordered[i] = rows[i+1]
					}
					observed := strings.Join(ordered, "\n")
					writeEvidence(t, fmt.Sprintf("tmux/ale29/%dx%d/%s-page-%02d.txt", size.width, size.height, label, page), []byte(frame))
					found := true
					for _, needle := range needles {
						found = found && paneContains(observed, needle)
					}
					if found {
						return
					}
					if last == total {
						t.Fatalf("missing detail needles %+v:\n%s", needles, observed)
					}
					old := first
					frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool { first, _, _ := sourceDetailRange(next.cells); return first > old }).cells
				}
				t.Fatal("Claude detail pager did not finish")
			}
			collect("02-explicit", "Source:", "Reference global:", "Config layer:", "Winning source:", "Related independent source:", "disable-model-invocation", "Capability: disable", "unsupported", "Session: unknown", "Reload: unknown")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			claudeSettings(t, settings, map[string]any{"skillOverrides": map[string]string{"browser-claude": "off"}})
			before = treeManifest(t, fx.base)
			send("r", "Refresh: selected directory object retained")
			collect("03-disabled", "Availability: disabled", "skillOverrides.browser-claude=off", "Skill tool permissions")
			send("Escape", "SKILLGLASS | Sources", "0 matches")
			filter := send("a", "Availability filter: ask")
			assertPaneCells(t, filter, size.width, size.height)
			send("a", "Availability filter: shadowed")
			send("a", "Availability filter: unavailable")
			send("a", "Availability filter: unknown")
			send("a", "Availability filter: all", "browser-claude")
			send("/", "Enter keep")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", "root-action")
			waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
			send("Enter", "Enter read")
			send("Enter", "Read lines", "Availability: unavailable")
			collect("04-plugin", "Plugin: demo@market", "plugin-disabled", "Capability: disable | plugin", "unsupported")
			claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": true}})
			before = treeManifest(t, fx.base)
			send("r", "Refresh: selected directory object retained")
			collect("05-root-plugin-enabled", "Availability: available", "Plugin: demo@market", "local-marketplace-plugin", "Capability: disable | plugin", "unsupported")
			plugin := filepath.Join(fx.base, "allowed", "root-market", "demo")
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "homepage": "https:"})
			before = treeManifest(t, fx.base)
			send("g", "Read lines 1-")
			send("r", "Availability: unknown")
			collect("06-invalid-manifest", "Availability: unknown", "Plugin: demo@market", "plugin-manifest-unverified", "homepage", "Capability: disable | plugin", "unsupported")
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "homepage": "https://example.invalid:99999/docs"})
			before = treeManifest(t, fx.base)
			// Unknown-to-unknown refresh must observe a new frame, not the old status.
			frame := capturePaneAt(t, tmux, fx, socket, "browser:0.0", false)
			_, last, total := sourceDetailRange(frame)
			if last != total {
				actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "G") }, func(next paneSnapshot) bool { _, last, total := sourceDetailRange(next.cells); return last == total })
			}
			manifestInfo, err := os.Stat(filepath.Join(plugin, ".claude-plugin/plugin.json"))
			if err != nil {
				t.Fatal(err)
			}
			resourceSize := fmt.Sprintf("size=%d", manifestInfo.Size())
			actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "r") }, func(next paneSnapshot) bool { return paneContains(next.cells, resourceSize) })
			collect("07-invalid-port", "Availability: unknown", "WHATWG URL parsing", "Plugin: demo@market", "port")
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "homepage": "https:///example.invalid/docs"})
			before = treeManifest(t, fx.base)
			send("g", "Read lines 1-")
			send("r", "Availability: available")
			collect("08-valid-normalized", "Availability: available", "local-marketplace-plugin", "Plugin: demo@market", "unsupported")
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "homepage": "https://xn--ASCII-/docs"})
			before = treeManifest(t, fx.base)
			send("g", "Read lines 1-")
			send("r", "Availability: unknown")
			collect("09-idna-unmodeled", "Availability: unknown", "unmodeled IDNA validity", "Plugin: demo@market", "unsupported")
			for index, shape := range []struct {
				name, field string
				value       any
			}{
				{"repository-number", "repository", 17},
				{"repository-null", "repository", nil},
				{"repository-object", "repository", map[string]string{"url": "https://example.invalid"}},
				{"version-null", "version", nil},
				{"skills-null", "skills", nil},
				{"skills-null-element", "skills", []any{nil}},
			} {
				claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "not-semver", "repository": "not-an-absolute-url"})
				before = treeManifest(t, fx.base)
				send("g", "Read lines 1-")
				send("r", "Availability: available")
				collect(fmt.Sprintf("10-shape-valid-%02d", index), "Availability: available", "local-marketplace-plugin", "Plugin: demo@market", "not-semver", "unsupported")
				fields := map[string]any{"name": "demo", "version": "1.0.0"}
				fields[shape.field] = shape.value
				claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), fields)
				before = treeManifest(t, fx.base)
				send("g", "Read lines 1-")
				send("r", "Availability: unknown")
				collect("11-shape-"+shape.name, "Availability: unknown", shape.field, "non-null", "plugin-manifest-unverified", "Plugin: demo@market", "unsupported")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
			claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "skills": []string{"."}})
			before = treeManifest(t, fx.base)
			send("g", "Read lines 1-")
			send("r", "Availability: available")
			collect("12-shape-valid-array", "Availability: available", "local-marketplace-plugin", "Plugin: demo@market", "unsupported")
			for index, declaration := range []any{"", []string{".", "extra"}} {
				claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "skills": declaration})
				before = treeManifest(t, fx.base)
				send("g", "Read lines 1-")
				send("r", "Availability: unknown")
				collect(fmt.Sprintf("13-unsupported-skills-path-%02d", index), "Availability: unknown", "skills paths must be exactly", "start with", "plugin-manifest-unverified", "Plugin: demo@market", "unsupported")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				claudeSettings(t, filepath.Join(plugin, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "skills": "."})
				before = treeManifest(t, fx.base)
				send("g", "Read lines 1-")
				send("r", "Availability: available")
				collect(fmt.Sprintf("14-supported-skills-path-%02d", index), "Availability: available", "local-marketplace-plugin", "Plugin: demo@market")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
			send("Escape", "SKILLGLASS | Sources")
			scope := send("s", "SKILLGLASS | Scope and filters", "Agent: claude-code", "Validated: Claude Code 2.1.282")
			assertPaneCells(t, scope, size.width, size.height)
			writeEvidence(t, fmt.Sprintf("tmux/ale29/%dx%d/04-scope.txt", size.width, size.height), []byte(scope))
			send("Escape", "SKILLGLASS | Sources")
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
