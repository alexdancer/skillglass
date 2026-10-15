package acceptance_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifyComparisonCompletesWhileHelpIsOpen(t *testing.T) {
	fx := newFixture(t)
	root := filepath.Join(fx.base, "allowed", "comparison-help")
	for _, name := range []string{"left", "right"} {
		bundle := filepath.Join(root, name)
		writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte("---\nname: comparison-fixture\ndescription: Compare Help regression\n---\nSame instructions\n"), 0600)
		for n := 0; n < 7000; n++ {
			writeFile(t, filepath.Join(bundle, fmt.Sprintf("resource-%05d.txt", n)), []byte("same contents\n"), 0600)
		}
	}
	tmux, socket, target := startALE43ReviewBrowser(t, fx, 120, 30, "--root", root, "--theme", "none")
	runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "m", "j")
	controlStart := time.Now()
	control := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "v")
	}, "Coverage: complete | identical")
	t.Logf("no-Help control completed in %s", time.Since(controlStart))
	writeEvidence(t, "compare-help/control.txt", []byte(control))
	actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
	}, "Compare left:")
	started := time.Now()
	loading := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "v")
	}, "Reading current bundle contents...")
	writeEvidence(t, "compare-help/loading-before-help.txt", []byte(loading))
	help := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "?")
	}, "Keys")
	t.Logf("observed Help %s after starting comparison", time.Since(started))
	writeEvidence(t, "compare-help/help.txt", []byte(help))
	time.Sleep(3 * time.Second)
	stillHelp := capturePaneAt(t, tmux, fx, socket, target, false)
	if !strings.Contains(stillHelp, "Keys") {
		t.Fatalf("comparison completion closed Help:\n%s", stillHelp)
	}
	returned := actForPaneAt(t, tmux, fx, socket, target, false, func() {
		runTmux(t, tmux, fx, socket, "send-keys", "-t", target, "Escape")
	}, "SKILLGLASS | Comparison")
	writeEvidence(t, "compare-help/returned.txt", []byte(returned))
	final := waitForPaneAt(t, tmux, fx, socket, target, false, "Coverage: complete | identical")
	writeEvidence(t, "compare-help/final.txt", []byte(final))
	if !strings.Contains(final, "Coverage: complete | identical") {
		t.Fatalf("comparison after Help =\n%s\nwant literal %q; no-Help control completed", final, "Coverage: complete | identical")
	}
}
