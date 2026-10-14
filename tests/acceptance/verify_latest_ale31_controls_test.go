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

func TestVerifyLatestALE31SelectorControls(t *testing.T) {
	for _, mode := range []string{"inactive-org-direct", "support-path", "support-leaf-gap", "support-category", "dependency-gap", "namespace-dispatch", "absolute", "drive", "unc", "traversal", "nested-traversal", "windows-traversal", "invalid-namespace", "reference-context"} {
		t.Run(mode, func(t *testing.T) {
			fx, p := hermesFixture(t)
			selector, want, fragment := "category/second", "ambiguous-bare-name", "(2 known distinct manifests)"
			switch mode {
			case "inactive-org-direct":
				selector = "_org/stale/second"
				codexSkill(t, filepath.Join(p, "skills"), selector, "other")
			case "support-path":
				selector, want, fragment = "bundle/references/second", "unambiguous-bare-name", "(1 known distinct manifests)"
				codexSkill(t, filepath.Join(p, "shared"), "bundle", "owner")
				codexSkill(t, filepath.Join(p, "shared"), selector, "other")
			case "support-leaf-gap":
				selector, want, fragment = "bundle/references", "unknown", ": unknown (1 known distinct manifests)"
				codexSkill(t, filepath.Join(p, "shared"), "bundle", "owner")
				codexSkill(t, filepath.Join(p, "shared"), selector, "other")
			case "support-category":
				selector = "scripts/second"
				codexSkill(t, filepath.Join(p, "shared"), selector, "other")
			case "dependency-gap":
				selector, want, fragment = ".archive/second", "unknown", ": unknown (1 known distinct manifests)"
				codexSkill(t, filepath.Join(p, "shared"), selector, "other")
			case "namespace-dispatch":
				selector, want, fragment = "category:second", "unknown", "plugin namespace dispatch precedes local lookup"
				codexSkill(t, filepath.Join(p, "shared"), "category/second", "other")
				plugin := filepath.Join(p, "plugins", "native")
				codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
				writeFile(t, filepath.Join(plugin, "plugin.yaml"), []byte("name: category\nversion: '1'\n"), 0600)
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("skills:\n  external_dirs: [shared]\nplugins:\n  enabled: [category]\n"), 0600)
			case "reference-context":
				target := codexSkill(t, filepath.Join(p, "shared"), "original", "other")
				if err := os.MkdirAll(filepath.Join(p, "shared", "category"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(target), filepath.Join(p, "shared", "category", "second")); err != nil {
					t.Fatal(err)
				}
			default:
				want, fragment = "unknown", ": invalid-selector"
				selectors := map[string]string{"absolute": filepath.Join(fx.cleanRoot, "outside"), "drive": "C:second", "unc": `\\host\share\second`, "traversal": "../second", "nested-traversal": "category/../second", "windows-traversal": `category\..\second`, "invalid-namespace": "bad.namespace:second"}
				selector = selectors[mode]
				codexSkill(t, fx.cleanRoot, "outside", "other")
			}
			a := codexSkill(t, filepath.Join(p, "skills"), "first", selector)
			writeFile(t, a, []byte(g2Text(selector, "")), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, a, "available")
			source := gateSourceAt(t, list, filepath.Dir(a))
			inv := g2ExplicitInvocation(t, fx, source)
			if inv.State != want || !strings.Contains(inv.Evidence.Reason, fragment) {
				t.Errorf("selector %q: %+v; want %s containing %s", selector, inv, want, fragment)
			}
		})
	}
}

func TestVerifyLatestALE31PlatformConfidence(t *testing.T) {
	for _, tc := range []struct{ name, extra, state string }{
		{"implicit-sibling", "platforms: [off, macos]\n", "unknown"},
		{"unknown-name", "platforms: [false, macos]\n", "unknown"},
		{"environment-condition", "platforms: [false, macos]\nenvironments: [docker]\n", "unknown"},
		{"full-content-matching", "padding: '" + strings.Repeat("x", 4100) + "'\nplatforms: [false, macos]\n", "available"},
		{"full-content-nonmatching", "padding: '" + strings.Repeat("x", 4100) + "'\nplatforms: [false, windows]\n", "available"},
		{"nonmatching-null-map", "platforms: [null, {inert: value}, windows]\n", "unavailable"},
		{"quoted-tag", "platforms: [true, 'macos']\n", "available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			name := "selected"
			if tc.name == "unknown-name" {
				name = "on"
			}
			file := codexSkill(t, filepath.Join(p, "skills"), "selected", name)
			writeFile(t, file, []byte(g2Text(name, tc.extra)), 0600)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			g2Inspect(t, fx, nil, list, file, tc.state)
			inv := g2ExplicitInvocation(t, fx, gateSourceAt(t, list, filepath.Dir(file)))
			if strings.Contains(inv.Evidence.Reason, "differs") != (tc.name == "full-content-nonmatching") {
				t.Errorf("index/full-content distinction changed: %+v", inv)
			}
		})
	}
}

func TestVerifyLatestALE31Browser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		for _, mode := range []string{"direct", "platform"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				fx, p := hermesFixture(t)
				name, extra := "category/second", ""
				if mode == "platform" {
					name, extra = "selected", "platforms: [false, macos]\n"
				} else {
					codexSkill(t, filepath.Join(p, "shared"), "category/second", "other")
				}
				file := codexSkill(t, filepath.Join(p, "skills"), "first", name)
				writeFile(t, file, []byte(g2Text(name, extra)), 0600)
				defer g2Seal(t, fx)()
				list := g2List(t, fx, 0, nil)
				g2Inspect(t, fx, nil, list, file, "available")
				socket := fmt.Sprintf("ale31-latest-%d-%d-%s", os.Getpid(), width, mode)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := "exec " + shellQuote(skillglassBinary) + " browse --agent hermes --project " + shellQuote(fx.project) + " --availability available"
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "available (inferred)")
				prefix := fmt.Sprintf("tmux/ale31-latest/%d/%s/", width, mode)
				writeEvidence(t, prefix+"00-sources.txt", []byte(frame))
				frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
				}, "Read lines", "Availability: available")
				seenPermission, seenFact := false, false
				fact := `selector "category/second": ambiguous-bare-name (2 known distinct manifests)`
				if mode == "platform" {
					fact = "eligible (inferred)"
				}
				for step := 0; step < 100; step++ {
					assertPaneCells(t, frame, width, 30)
					writeEvidence(t, fmt.Sprintf("%s%03d-detail.txt", prefix, step), []byte(frame))
					seenPermission = seenPermission || paneContains(frame, "Configured permission: allowed")
					seenFact = seenFact || paneContains(frame, fact)
					if seenPermission && seenFact || strings.Contains(frame, "G2_FULL_CONTENT_SENTINEL") {
						break
					}
					var start, end, total int
					for _, line := range strings.Split(frame, "\n") {
						if strings.HasPrefix(line, "Read lines ") {
							if _, err := fmt.Sscanf(line, "Read lines %d-%d of %d", &start, &end, &total); err != nil {
								t.Fatal(err)
							}
						}
					}
					next := min(start+3, total-(end-start))
					if start == 0 || next <= start {
						break
					}
					frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
						runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Down", "Down", "Down")
					}, fmt.Sprintf("Read lines %d-", next))
				}
				if !seenPermission || !seenFact {
					t.Fatalf("browser lost selector/platform facts: %s", frame)
				}
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
