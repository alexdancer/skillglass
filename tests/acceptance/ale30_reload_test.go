package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestALE30ClaudeReloadUnknown(t *testing.T) {
	for _, scenario := range []string{"applied", "noop", "refused"} {
		t.Run(scenario, func(t *testing.T) {
			fx, _ := codexFixture(t)
			id, _ := claudeTarget(t, fx)
			choice, scope := "disable", "global"
			if scenario == "noop" {
				choice, scope = "inherit", "project"
			}
			preview := claudeChange(t, fx, choice, scope, id, "", 0)
			if preview.State != "ready" {
				t.Fatalf("preview: %+v", preview)
			}
			token, wantExit, wantState := preview.Confirmation, 0, "applied"
			if scenario == "refused" {
				token, wantExit, wantState = "forged-token", 1, "refused"
			}
			before := treeManifest(t, fx.base)
			args := []string{choice, "--agent", "claude-code", "--project", fx.project, "--scope", scope, "--json", "--confirm", token, id}
			result := changeCommand(t, fx, "", args...)
			assertExit(t, result, wantExit)
			var out struct {
				State  string
				Reload struct {
					State    string
					Evidence struct{ Kind, Detail string }
				}
			}
			decodeJSON(t, result.stdout, &out)
			if out.State != wantState {
				t.Fatalf("outcome: %s", result.stdout)
			}
			if scenario == "applied" && !strings.Contains(string(readBytes(t, filepath.Join(fx.home, ".claude/settings.json"))), `"off"`) {
				t.Fatal("setting not saved")
			}
			if out.Reload.State != "unknown" {
				t.Errorf("reload.state = %q; want literal unknown: %s", out.Reload.State, result.stdout)
			}
			if out.Reload.Evidence.Kind != "unknown" {
				t.Errorf("reload evidence must remain unknown: %s", result.stdout)
			}
			if scenario != "applied" {
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
			guidance := strings.ToLower(string(result.stdout))
			if !strings.Contains(guidance, "existing session adoption was not verified") || !(strings.Contains(guidance, "fresh") || strings.Contains(guidance, "restart")) {
				t.Errorf("missing conservative fresh-launch guidance: %s", result.stdout)
			}
		})
	}
}
