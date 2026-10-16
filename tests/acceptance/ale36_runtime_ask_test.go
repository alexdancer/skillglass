package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE36RuntimeAskMustNotClaimSaved(t *testing.T) {
	fx, _ := codexFixture(t)
	codexSkill(t, filepath.Join(fx.home, ".agents/skills"), "target", "target")
	id := codexNamed(t, opencodeListResult(t, fx, 0), "target").ID
	configDir := filepath.Join(fx.xdgConfig, "opencode")
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("precondition: global config directory must be absent: %v", err)
	}
	before := treeManifest(t, fx.base)
	r := opencodeEnvRun(t, fx, []string{`OPENCODE_PERMISSION={"skill":"ask"}`}, "enable", "--agent", "opencode", "--scope", "global", "--project", fx.project, id)
	assertExit(t, r, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("global config directory must remain absent: %v", err)
	}
	writeEvidence(t, "ale36/runtime-ask-preview.txt", r.stdout)
	t.Logf("actual public preview:\n%s", r.stdout)
	if !strings.Contains(string(r.stdout), "No write needed; confirmation will verify without rewriting.") {
		t.Fatalf("runtime-only ask must explain verification without claiming persistence: %s", r.stdout)
	}
	if got, want := strings.Contains(string(r.stdout), "Already saved;"), false; got != want {
		t.Fatalf("runtime-only ask has no persisted native choice: saved claim = %v, want %v", got, want)
	}
}
