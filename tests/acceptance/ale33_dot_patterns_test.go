package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func piDotFixture(t *testing.T, scenario string) (fixture, string) {
	t.Helper()
	fx := piFixture(t)
	agentDir := filepath.Join(fx.home, ".pi", "agent")
	root := filepath.Join(agentDir, "skills")
	name, pattern, state := "example", "!"+fx.home+"/**", "available"
	origin := "automatic"
	switch scenario {
	case "absolute-globstar-file":
		pattern += "/SKILL.md"
	case "absolute-star":
		pattern = "!" + fx.home + "/*/agent/skills/**"
	case "absolute-question":
		pattern = "!" + fx.home + "/?pi/agent/skills/**"
	case "absolute-class":
		pattern = "!" + fx.home + "/[.]pi/agent/skills/**"
	case "absolute-negated-class":
		pattern = "!" + fx.home + "/[!a]pi/agent/skills/**"
	case "nonsegment-globstar":
		pattern = "!" + fx.home + "/**pi/agent/skills/**"
	case "explicit-dot":
		pattern, state = "!"+fx.home+"/.pi/**", "disabled"
	case "relative":
		pattern, state = "!skills/**", "disabled"
	case "basename", "zero-globstar":
		pattern, state = "!**/example", "disabled"
	case "exact-restore":
		pattern, state = "!*", "available"
	case "exact-deny":
		pattern, state = "-*", "available"
	case "unknown":
		pattern, state = "!{example,other}", "unknown"
	case "hidden-directory", "hidden-class", "hidden-explicit", "hidden-relative", "parent-dotdot", "parent-explicit":
		origin = "configured"
		root = filepath.Join(agentDir, ".hidden")
		pattern = "!" + agentDir + "/**"
		if scenario == "hidden-class" {
			pattern = "![.]hidden/example"
		}
		if scenario == "hidden-explicit" {
			pattern, state = "!"+agentDir+"/.hidden/**", "disabled"
		}
		if scenario == "hidden-relative" {
			pattern, state = "!.*/example", "disabled"
		}
		if scenario == "parent-dotdot" || scenario == "parent-explicit" {
			root = filepath.Join(fx.base, "allowed", ".hidden")
			pattern = "!../**/.hidden/example"
			if scenario == "parent-explicit" {
				pattern, state = "!../../../.hidden/example", "disabled"
			}
		}
	case "hidden-bundle-class", "hidden-bundle-literal", "hidden-bundle-star":
		origin, name = "configured", ".hidden"
		root = filepath.Join(fx.base, "allowed", "selected")
		pattern = "![.]hidden"
		if scenario == "hidden-bundle-literal" {
			pattern, state = "!.hidden", "disabled"
		}
		if scenario == "hidden-bundle-star" {
			pattern, state = "!.*", "disabled"
		}
	case "package-hidden", "package-explicit", "package-relative", "package-include-hidden", "package-include-explicit":
		origin = "package"
		root = filepath.Join(fx.base, "allowed", "P", ".hidden")
		pkg := filepath.Dir(root)
		pattern = "!" + pkg + "/**"
		if scenario == "package-explicit" {
			pattern, state = "!"+pkg+"/.hidden/**", "disabled"
		}
		if scenario == "package-relative" {
			pattern, state = "!**/example", "disabled"
		}
		if scenario == "package-include-hidden" {
			pattern, state = pkg+"/**", "disabled"
		}
		if scenario == "package-include-explicit" {
			pattern = ".hidden/**"
		}
	}
	manifest := codexSkill(t, root, name, "dot-target")
	patterns := []string{pattern}
	if scenario == "exact-restore" {
		patterns = append(patterns, "+"+filepath.Dir(manifest))
	}
	if scenario == "exact-deny" {
		patterns = []string{"-" + filepath.Dir(manifest)}
		state = "disabled"
	}
	settings := map[string]any{"defaultProjectTrust": "always", "skills": patterns}
	if origin == "configured" {
		settings["skills"] = append([]string{filepath.Dir(manifest)}, patterns...)
	}
	if origin == "package" {
		pkg := filepath.Dir(root)
		writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"pi":{"skills":[".hidden"]}}`), 0600)
		delete(settings, "skills")
		settings["packages"] = []any{map[string]any{"source": pkg, "skills": patterns}}
	}
	piSettingsFile(t, filepath.Join(agentDir, "settings.json"), settings)
	return fx, state
}

func TestALE33DotPatternCandidates(t *testing.T) {
	for _, scenario := range []string{"absolute-globstar", "absolute-globstar-file", "absolute-star", "absolute-question", "absolute-class", "absolute-negated-class", "nonsegment-globstar", "explicit-dot", "relative", "basename", "zero-globstar", "exact-restore", "exact-deny", "unknown", "hidden-directory", "hidden-class", "hidden-explicit", "hidden-relative", "parent-dotdot", "parent-explicit", "hidden-bundle-class", "hidden-bundle-literal", "hidden-bundle-star", "package-hidden", "package-explicit", "package-relative", "package-include-hidden", "package-include-explicit"} {
		t.Run(scenario, func(t *testing.T) {
			fx, state := piDotFixture(t, scenario)
			before := treeManifest(t, fx.base)
			exit := 0
			if state == "unknown" {
				exit = 1
			}
			list := piList(t, fx, exit)
			source := codexNamed(t, list, "dot-target")
			if source.Availability.State != state {
				t.Errorf("state=%s, want %s", source.Availability.State, state)
			}
			assertPiResourceDetail(t, fx, source, exit)
			filtered := piList(t, fx, exit, "--availability", state)
			if len(filtered.Sources) != 1 {
				t.Errorf("availability filter disagrees: %+v", filtered.Sources)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE33DotPatternBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	fx, _ := piDotFixture(t, "absolute-globstar")
	before := treeManifest(t, fx.base)
	socket := fmt.Sprintf("ale33-dot-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse --agent pi --project "+shellQuote(fx.project))
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "dot-target")
	frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter") }, "Read lines", "Configured permission:")
	for _, line := range []string{"Availability: available (inferred)", "Configured permission: allowed (inferred)"} {
		if !strings.Contains(frame, line) {
			t.Errorf("browser lacks %s: %s", line, frame)
		}
	}
	assertPaneCells(t, frame, 80, 30)
	writeEvidence(t, "scope/dot/browser.txt", []byte(frame))
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
