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

func TestVerifySupportAncestorsBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		for _, mode := range []string{"relative", "categorized", "recursive"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				fx, p := hermesFixture(t)
				selector := "category/second"
				if mode == "categorized" {
					selector = "category:second"
				}
				file := codexSkill(t, filepath.Join(p, "skills"), "first", selector)
				writeFile(t, file, []byte(g2Text(selector, "")), 0600)
				owner := filepath.Join(p, "outside-discovery", "owner")
				writeFile(t, filepath.Join(owner, "SKILL.md"), []byte(g2Text("owner", "")), 0600)
				library := filepath.Join(owner, "references", "library")
				other := "other"
				if mode == "recursive" {
					other = selector
				}
				second := codexSkill(t, library, "category/second", other)
				writeFile(t, second, []byte(g2Text(other, "")), 0600)
				writeFile(t, filepath.Join(p, "config.yaml"), []byte(fmt.Sprintf("skills:\n  external_dirs: [%q]\n", library)), 0600)
				defer g2Seal(t, fx)()
				list := g2List(t, fx, 0, nil)
				g2Inspect(t, fx, nil, list, file, "available")
				source := gateSourceAt(t, list, filepath.Dir(file))
				invocation := g2ExplicitInvocation(t, fx, source)
				state, kind, count := "unknown", "unknown", "1"
				if mode == "recursive" {
					state, kind, count = "ambiguous-bare-name", "inferred", "2"
				}
				fact := fmt.Sprintf("selector %q: %s (%s known distinct manifests)", selector, state, count)
				if invocation.State != state || invocation.Evidence.Kind != kind || !strings.Contains(invocation.Evidence.Reason, fact) {
					t.Fatalf("selector confidence: %+v", invocation)
				}
				socket := fmt.Sprintf("ale31-support-%d-%d-%s", os.Getpid(), width, mode)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := "exec " + shellQuote(skillglassBinary) + " browse --agent hermes --project " + shellQuote(fx.project) + " --availability available"
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "available (inferred)")
				prefix := fmt.Sprintf("tmux/ale31-support/%d/%s/", width, mode)
				writeEvidence(t, prefix+"00-sources.txt", []byte(frame))
				frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
				}, "Read lines", "Availability: available")
				seenPermission, seenInvocation, seenFact := false, false, false
				for step := 0; step < 120; step++ {
					assertPaneCells(t, frame, width, 30)
					writeEvidence(t, fmt.Sprintf("%s%03d-detail.txt", prefix, step), []byte(frame))
					seenPermission = seenPermission || paneContains(frame, "Configured permission: allowed")
					seenInvocation = seenInvocation || paneContains(frame, fmt.Sprintf("Invocation: %s (%s)", state, kind))
					seenFact = seenFact || paneContains(frame, fact)
					if seenPermission && seenInvocation && seenFact {
						break
					}
					var advanced bool
					frame, advanced = advanceSourceDetailFrame(t, tmux, fx, socket, frame)
					if !advanced {
						break
					}
				}
				if !seenPermission || !seenInvocation || !seenFact {
					t.Fatalf("browser lost independent index/invocation facts: %s", frame)
				}
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
