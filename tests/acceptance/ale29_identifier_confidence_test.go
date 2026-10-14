package acceptance_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const pluginIdentifierUncertainty = "plugin name identifier is empty or contains ASCII space or colon"

func claudeIdentifierFixture(t *testing.T, fx fixture, name, mode string) string {
	t.Helper()
	market := filepath.Join(fx.base, "allowed", "identifier-market")
	local := filepath.Join(market, "demo")
	id := name + "@market"
	action := claudeSkill(t, filepath.Join(local, "skills"), "action", "action", "")
	manifest := map[string]any{"name": name, "version": "1.0.0"}
	if mode == "missing-name" {
		delete(manifest, "name")
	}
	claudeSettings(t, filepath.Join(local, ".claude-plugin", "plugin.json"), manifest)
	claudeSettings(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), map[string]any{"name": "market", "owner": map[string]string{"name": "Fixture"}, "plugins": []map[string]string{{"name": name, "source": "./demo"}}})
	plugins := filepath.Join(fx.home, ".claude", "plugins")
	if mode != "inactive" {
		claudeSettings(t, filepath.Join(plugins, "known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-10-13T00:00:00Z"}})
	}
	entry := map[string]string{"scope": "user", "installPath": local, "version": "1.0.0"}
	if mode == "nonapplicable" {
		entry["scope"], entry["projectPath"] = "project", filepath.Join(fx.base, "allowed", "other-project")
	}
	if mode == "inactive" {
		entry["version"] = "2.0.0"
	}
	if mode == "cache-copy" {
		cache := filepath.Join(fx.base, "allowed", "installed-copy")
		entry["installPath"] = cache
		action = claudeSkill(t, filepath.Join(cache, "skills"), "action", "action", "")
		claudeSettings(t, filepath.Join(cache, ".claude-plugin", "plugin.json"), manifest)
	}
	claudeSettings(t, filepath.Join(plugins, "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{id: []map[string]string{entry}}})
	claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{id: mode != "disabled"}})
	return filepath.Dir(action)
}

// This public journey incorporates the independent final83 failing reproduction.
func TestALE29PluginIdentifierConfidence(t *testing.T) {
	for _, tc := range []struct{ label, name, want string }{
		{"valid", "demo", "available"}, {"empty", "", "unknown"},
		{"colon", "bad:name", "unknown"}, {"space", "bad name", "unknown"},
	} {
		for _, mode := range []string{"enabled", "disabled", "nonapplicable", "inactive", "cache-copy", "missing-name"} {
			if mode == "missing-name" && tc.label != "empty" {
				continue
			}
			t.Run(tc.label+"/"+mode, func(t *testing.T) {
				fx, _ := codexFixture(t)
				target := claudeIdentifierFixture(t, fx, tc.name, mode)
				want, participation, rule := tc.want, "unknown", "plugin-manifest-unverified"
				if tc.want == "available" {
					participation, rule = "eligible", "local-marketplace-plugin"
				}
				switch mode {
				case "disabled":
					want, participation, rule = "unavailable", "excluded", "plugin-disabled"
				case "nonapplicable", "inactive":
					want, participation, rule = "unavailable", "excluded", "inactive-plugin-installation"
				case "cache-copy":
					want, participation, rule = "unavailable", "excluded", "local-plugin-cache-copy"
				}
				before := treeManifest(t, fx.base)
				label := tc.label + "-" + mode
				args := []string{"list", "--agent", "claude-code", "--project", fx.project, "--json"}
				r := fx.run(t, args...)
				assertExit(t, r, 0)
				writeEvidence(t, "identifiers/"+label+"-list.json", r.stdout)
				var list codexList
				decodeJSON(t, r.stdout, &list)
				source := claudeAt(t, list, target)
				if source.Availability.State != want || source.Ownership.State != "managed" || source.Availability.NativeName != tc.name+":action" {
					t.Fatalf("state, ownership or namespace lost: %+v", source)
				}
				if len(source.Availability.References) != 1 {
					t.Fatal("lost managed discovery reference")
				}
				ref := source.Availability.References[0]
				if ref.Participation != participation || ref.Evidence.Rule != rule || ref.Provenance.PluginID != tc.name+"@market" || ref.Provenance.Kind != "plugin" || ref.Provenance.Version != "1.0.0" {
					t.Fatalf("activation or plugin provenance lost: %+v", ref)
				}
				var public publicList
				decodeJSON(t, r.stdout, &public)
				identityFound := false
				for _, v := range public.References {
					identityFound = identityFound || (v.ID == ref.ReferenceID && v.SourceID == source.ID && v.DiscoveredPath == target)
				}
				if !identityFound {
					t.Fatal("availability reference lost public source/path identity")
				}
				if want == "unknown" && (source.Availability.Evidence.Kind != "unknown" || !reflect.DeepEqual(source.Availability.Evidence, ref.Evidence) || !strings.Contains(ref.Evidence.Reason, pluginIdentifierUncertainty) || ref.Evidence.InputPath != filepath.Join(filepath.Dir(filepath.Dir(target)), ".claude-plugin", "plugin.json")) {
					t.Fatalf("identifier-specific primary/reference uncertainty lost: %+v", source.Availability)
				}
				detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
				assertExit(t, detail, 0)
				writeEvidence(t, "identifiers/"+label+"-explain.txt", detail.stdout)
				if !strings.Contains(string(detail.stdout), "Availability: "+want) || !strings.Contains(string(detail.stdout), "Discovery rule: "+rule+" |") || (want == "unknown" && !strings.Contains(string(detail.stdout), pluginIdentifierUncertainty)) {
					t.Fatalf("human explanation contradicts identifier evidence:\n%s", detail.stdout)
				}
				j := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, "--json", source.ID)
				assertExit(t, j, 0)
				writeEvidence(t, "identifiers/"+label+"-explain.json", j.stdout)
				var explained struct{ Source codexSource }
				decodeJSON(t, j.stdout, &explained)
				if !reflect.DeepEqual(source, explained.Source) {
					t.Fatal("list/explain source identity or availability mismatch")
				}
				for _, state := range []string{"available", "unknown", "unavailable"} {
					f := fx.run(t, append(append([]string{}, args...), "--availability", state)...)
					assertExit(t, f, 0)
					writeEvidence(t, "identifiers/"+label+"-"+state+"-filter.json", f.stdout)
					var filtered codexList
					decodeJSON(t, f.stdout, &filtered)
					found := false
					for _, s := range filtered.Sources {
						found = found || s.ID == source.ID
					}
					if found != (want == state) {
						t.Errorf("%s filter membership=%t want %t", state, found, want == state)
					}
				}
				if mode == "cache-copy" {
					local := claudeAt(t, list, filepath.Join(fx.base, "allowed", "identifier-market", "demo", "skills", "action"))
					if local.ID == source.ID || local.Availability.State != tc.want {
						t.Fatalf("independent local/cache identities or confidence conflated: %+v", local)
					}
				}
				t.Logf("name=%q state=%s source=%s reference=%s participation=%s rule=%s", tc.name, want, source.ID, ref.ReferenceID, participation, rule)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE29PluginIdentifierConfidenceBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			fx, _ := codexFixture(t)
			claudeIdentifierFixture(t, fx, "bad name", "enabled")
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("identifier-%d", size.width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := fx.productCommand(t, "browse", "--agent", "claude-code", "--project", fx.project, "--availability", "unknown")
			runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser")
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
			frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "unknown (unknown)", "Focus: 1/1")
			assertPaneCells(t, frame, size.width, size.height)
			prefix := fmt.Sprintf("tmux/identifiers/%dx%d/", size.width, size.height)
			writeEvidence(t, prefix+"sources.txt", []byte(frame))
			frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Read lines")
			rows := map[int]string{}
			for page := 0; page < 60; page++ {
				assertPaneCells(t, frame, size.width, size.height)
				first, last, total, body := sourceDetailBody(t, frame)
				for i, row := range body {
					rows[first+i] = row
				}
				writeEvidence(t, fmt.Sprintf("%sdetail-page-%02d.txt", prefix, page), []byte(frame))
				if last == total {
					ordered := make([]string, total)
					for i := range ordered {
						ordered[i] = rows[i+1]
					}
					observed := strings.Join(ordered, "\n")
					writeEvidence(t, prefix+"detail-body.txt", []byte(observed))
					for _, needle := range []string{"Availability: unknown (unknown)", pluginIdentifierUncertainty, "native activation for this identifier is unverified", "plugin-manifest-unverified", "Plugin: bad name@market", "Ownership: managed", "Capability: disable | plugin", "unsupported"} {
						if !paneContains(observed, needle) {
							t.Errorf("browser explanation lacks %q:\n%s", needle, observed)
						}
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
			t.Fatal("identifier explanation pager did not finish")
		})
	}
}
