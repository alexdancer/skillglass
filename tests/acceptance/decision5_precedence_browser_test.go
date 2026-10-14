package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestDecision5PrecedenceBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	fx := piFixture(t)
	global := filepath.Join(fx.home, ".pi", "agent", "skills")
	lowManifest := codexSkill(t, global, "global", "collision")
	highManifest := codexSkill(t, filepath.Join(fx.base, "allowed", "configured"), "🔍", "collision")
	piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), map[string]any{"skills": []string{filepath.Dir(highManifest), "!?"}})
	before := treeManifest(t, fx.base)
	list := piRelatedInventory(t, fx, 1)
	low := piRelatedSource(t, list, canonicalPath(t, filepath.Dir(lowManifest)))
	if low.Availability.State != collection.AvailabilityUnknown || slices.ContainsFunc(low.Problems, func(problem collection.Problem) bool { return problem.Code == collection.ProblemNativeInput }) {
		t.Fatal("lower source must have precedence uncertainty without the higher source diagnostic")
	}
	socket := fmt.Sprintf("ale33-precedence-%d", os.Getpid())
	defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
	runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", "80", "-y", "30", "-s", "browser", "exec "+shellQuote(skillglassBinary)+" browse --agent pi --project "+shellQuote(fx.project)+" --source-root "+shellQuote(global)+" --availability unknown")
	runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
	waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "collision")
	frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
	}, "Read lines", "Availability: unknown")
	for _, line := range []string{"Selector: " + string(low.ID), "Availability: unknown (unknown)", "Configured permission: allowed (inferred)"} {
		if !strings.Contains(frame, line) {
			t.Errorf("lower detail lacks %q: %s", line, frame)
		}
	}
	assertPaneCells(t, frame, 80, 30)
	writeEvidence(t, "precedence/browser-detail.txt", []byte(frame))
	for page := 1; page <= 6 && !strings.Contains(frame, "Precedence: unknown"); page++ {
		previous := frame
		capture := actOnPane(t, tmux, fx, socket, "browser:0.0", func() {
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "NPage")
		}, func(next paneSnapshot) bool {
			return next.cells != previous && strings.Contains(next.cells, "Read lines")
		})
		frame = capture.cells
		assertPaneCells(t, frame, 80, 30)
		writeEvidence(t, fmt.Sprintf("precedence/browser-page-%d.txt", page), []byte(frame))
	}
	if !strings.Contains(frame, "Precedence: unknown") || !strings.Contains(frame, "Related independent source:") {
		t.Errorf("lower detail lost uncertain same-name relationship: %s", frame)
	}
	runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
