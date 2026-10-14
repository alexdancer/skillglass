package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const disclosureSentinel = "FAKE_APIKEY_47X"

func TestVerifyConfigDiagnosticDisclosure(t *testing.T) {
	for _, tc := range []struct {
		name, location, value string
		diagnostic            bool
	}{
		{"profile-bool-error", "profile", "!!bool " + disclosureSentinel, true},
		{"plugin-yaml-bool-error", "plugin.yaml", "!!bool " + disclosureSentinel, true},
		{"plugin-yml-bool-error", "plugin.yml", "!!bool " + disclosureSentinel, true},
		{"managed-bool-safe-diagnostic-control", "managed", "!!bool " + disclosureSentinel, true},
		{"profile-quoted-control", "profile", "'" + disclosureSentinel + "'", false},
		{"plugin-quoted-control", "plugin.yaml", "'" + disclosureSentinel + "'", false},
		{"managed-quoted-control", "managed", "'" + disclosureSentinel + "'", false},
		{"profile-syntax-safe-diagnostic-control", "profile", "__syntax__", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			dir := filepath.Join(p, "skills", "diagnostic-skill")
			codexSkill(t, filepath.Join(p, "skills"), "diagnostic-skill", "diagnostic-skill")
			input := filepath.Join(p, "config.yaml")
			raw := "provider: {api_key: " + tc.value + "}\n"
			var env []string
			switch tc.location {
			case "managed":
				m := filepath.Join(fx.home, "managed")
				input = filepath.Join(m, "config.yaml")
				env = []string{"HERMES_MANAGED_DIR=" + m}
			case "plugin.yaml", "plugin.yml":
				plugin := filepath.Join(p, "plugins", "portable")
				input = filepath.Join(plugin, tc.location)
				codexSkill(t, filepath.Join(plugin, "skills"), "explicit", "explicit")
				raw = "name: portable\n" + raw
			}
			if tc.value == "__syntax__" {
				raw = "provider: [\n"
			}
			writeFile(t, input, []byte(raw), 0600)
			defer g2Seal(t, fx)()
			result := hermesRun(t, fx, env, "list", "--agent", "hermes", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			source := gateSourceAt(t, list, dir)
			explanation := hermesRun(t, fx, env, "explain", "--agent", "hermes", "--project", fx.project, source.ID)
			for _, out := range []struct {
				name string
				raw  []byte
			}{{"list-stdout", result.stdout}, {"list-stderr", result.stderr}, {"explain-stdout", explanation.stdout}, {"explain-stderr", explanation.stderr}} {
				present := strings.Contains(string(out.raw), disclosureSentinel)
				t.Logf("%s synthetic value disclosed=%t", out.name, present)
				if present {
					t.Errorf("%s disclosed unconsumed synthetic provider value", out.name)
				}
			}
			if tc.diagnostic {
				assertExit(t, result, 1)
				assertExit(t, explanation, 1)
				if !strings.Contains(string(result.stdout), input) || !strings.Contains(string(result.stdout), "native-input") || !strings.Contains(string(result.stdout), "invalid YAML") {
					t.Error("safe input path/category/failure diagnostics missing")
				}
				if !strings.Contains(string(explanation.stdout), input) || !strings.Contains(string(explanation.stdout), "invalid YAML") {
					t.Error("safe explain failure provenance missing")
				}
			} else {
				assertExit(t, result, 0)
				assertExit(t, explanation, 0)
			}
		})
	}
}

func TestVerifyConfigDiagnosticDisclosureBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	fx, p := hermesFixture(t)
	codexSkill(t, filepath.Join(p, "skills"), "diagnostic-skill", "diagnostic-skill")
	writeFile(t, filepath.Join(p, "config.yaml"), []byte("provider: {api_key: !!bool "+disclosureSentinel+"}\n"), 0600)
	defer g2Seal(t, fx)()
	socket := fmt.Sprintf("verify-disclosure-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	command := fx.productCommand(t, "browse", "--agent", "hermes", "--project", fx.project)
	runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", "80", "-y", "40", "-s", "browser")
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	frame := waitForPane(t, tmux, fx, socket, false, "Skillglass", "diagnostic-skill")
	writeEvidence(t, "browser/01-sources.txt", []byte(frame))
	actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Tab") }, "Enter problem", "native-input")
	detail := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Summary:", "Hermes input is invalid YAML")
	assertPaneCells(t, detail, 80, 40)
	writeEvidence(t, "browser/02-problem.txt", []byte(detail))
	if strings.Contains(detail, disclosureSentinel) {
		t.Error("browser diagnostic exposed unconsumed synthetic provider value")
	}
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
}
