package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func exactPluginPolicyFixture(t *testing.T) (fixture, string) {
	t.Helper()
	fx, profile := hermesFixture(t)
	plugin := filepath.Join(profile, "plugins", "category", "registry-folder")
	codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
	writeFile(t, filepath.Join(plugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
	writeFile(t, filepath.Join(plugin, "__init__.py"), []byte("raise RuntimeError('inert plugin must never execute')\n"), 0600)
	return fx, profile
}

func TestALE31ExactPluginPolicyMembershipControls(t *testing.T) {
	for _, test := range []struct {
		name, policy, env, state string
		exit                     int
	}{
		{"category-key", "enabled: [category/registry-folder]", "", "explicit-only", 0},
		{"legacy-name", "enabled: [portable]", "", "explicit-only", 0},
		{"category-whitespace", "enabled: [' category/registry-folder ']", "", "unavailable", 0},
		{"legacy-name-case-sensitive", "enabled: [Portable]", "", "unavailable", 0},
		{"empty-entry", "enabled: ['']", "", "unavailable", 0},
		{"empty-list", "enabled: []", "", "unavailable", 0},
		{"absent-enabled", "disabled: []", "", "unavailable", 0},
		{"null-enabled", "enabled: null", "", "unavailable", 0},
		{"empty-disabled", "enabled: [portable]\n  disabled: ['']", "", "explicit-only", 0},
		{"exact-key-deny", "enabled: [portable]\n  disabled: [category/registry-folder]", "", "unavailable", 0},
		{"legacy-name-deny-wins", "enabled: [category/registry-folder]\n  disabled: [portable]", "", "unavailable", 0},
		{"category-whitespace-deny", "enabled: [category/registry-folder]\n  disabled: [' category/registry-folder ']", "", "explicit-only", 0},
		{"non-string-enabled", "enabled: [portable, false]", "", "unknown", 1},
		{"non-string-disabled", "enabled: [portable]\n  disabled: [false]", "", "unknown", 1},
		{"scalar-enabled", "enabled: portable", "", "unknown", 1},
		{"scalar-disabled", "enabled: [portable]\n  disabled: portable", "", "unknown", 1},
		{"expanded-key-exact", "enabled: ['${HERMES_ENABLE_PROJECT_PLUGINS}']", "HERMES_ENABLE_PROJECT_PLUGINS=category/registry-folder\n", "explicit-only", 0},
		{"expanded-key-whitespace", "enabled: ['${HERMES_ENABLE_PROJECT_PLUGINS}']", "HERMES_ENABLE_PROJECT_PLUGINS=' category/registry-folder '\n", "unavailable", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := exactPluginPolicyFixture(t)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  "+test.policy+"\n"), 0600)
			if test.env != "" {
				writeFile(t, filepath.Join(profile, ".env"), []byte(test.env), 0600)
			}
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, test.exit, nil)
			selected := codexNamed(t, list, "explicit")
			if selected.Availability.State != test.state {
				t.Fatalf("exact plugin state %q; want %q", selected.Availability.State, test.state)
			}
			if selected.Availability.NativeName != "agent-plugin-category-registry-folder-3af14b5d:explicit" {
				t.Fatalf("namespace changed: %q", selected.Availability.NativeName)
			}
			if selected.Ownership.State != "managed" {
				t.Fatal("plugin provenance lost")
			}
			found := false
			for _, ref := range selected.Availability.References {
				if ref.Provenance.PluginID == "category/registry-folder" {
					found = true
				}
			}
			if !found {
				t.Fatal("registry key changed to legacy name")
			}
			explain := hermesRun(t, fx, nil, "explain", "--agent", "hermes", "--project", fx.project, "--json", selected.ID)
			assertExit(t, explain, test.exit)
			var detail struct{ Source codexSource }
			decodeJSON(t, explain.stdout, &detail)
			if !reflect.DeepEqual(selected.Availability, detail.Source.Availability) {
				t.Fatal("list/explain facts differ")
			}
			filtered := hermesList(t, fx, test.exit, nil, "--availability", test.state)
			if len(filtered.Sources) != 1 || filtered.Sources[0].ID != selected.ID {
				t.Fatal("matching filter lost source identity")
			}
			other := "unavailable"
			if test.state == other {
				other = "explicit-only"
			}
			empty := hermesList(t, fx, test.exit, nil, "--availability", other)
			if len(empty.Sources) != 0 {
				t.Fatal("nonmatching filter retained source")
			}
			read := hermesRun(t, fx, nil, "read", "--agent", "hermes", "--project", fx.project, selected.ID)
			assertExit(t, read, test.exit)
			if !strings.Contains(string(read.stdout), "Inert instructions.") {
				t.Fatal("policy participation hid readable bundle")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE31ExactPluginPolicyBrowserRefreshAndFilter(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			fx, profile := exactPluginPolicyFixture(t)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [' category/registry-folder ']\n"), 0600)
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("ale31-exact-%d-%d", os.Getpid(), width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--agent", "hermes", "--project", fx.project)
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
			record := func(label, frame string) {
				assertPaneCells(t, frame, width, 30)
				writeEvidence(t, fmt.Sprintf("tmux/ale31-exact/%d/%s.txt", width, label), []byte(frame))
			}
			record("01-sources", waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "explicit", "unavailable (inferred)"))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			frame := send("Enter", "Availability: unavailable", "Configured permission: allowed", "Session: unknown", "Read lines")
			record("02-detail", frame)
			for page := 0; page < 10 && !paneContains(frame, "plugins are opt-in"); page++ {
				previous := frame
				frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PgDn")
				}, func(current paneSnapshot) bool {
					return current.cells != previous && paneContains(current.cells, "Read lines")
				}).cells
			}
			if !paneContains(frame, "plugins are opt-in") {
				t.Fatal("exact policy exclusion explanation missing from browser")
			}
			record("03-exclusion-reason", frame)
			send("g", "Read lines 1-")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [category/registry-folder]\n"), 0600)
			before = treeManifest(t, fx.base)
			record("04-refresh", send("r", "Refresh: selected directory object retained", "Availability: explicit-only", "Configured permission: allowed"))
			send("Escape", "SKILLGLASS | Sources", "explicit-only (inferred)")
			record("05-available-filter", send("a", "Availability filter: available", "0 matches"))
			send("a", "Availability filter: disabled", "0 matches")
			record("06-explicit-filter", send("a", "Availability filter: explicit-only", "explicit", "explicit-only (inferred)"))
			record("07-scope", send("s", "Scope and filters", "Validated: Hermes 0.20.0", "Availability: explicit-only"))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
