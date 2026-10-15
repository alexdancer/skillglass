package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRound4R15OrderedNormalization(t *testing.T) {
	for _, kind := range []string{"normalized-position", "raw-position", "raw-before-normalized", "raw-type-overwritten", "long-logical-symlink", "short-logical-symlink", "relative-unknown", "invalid-level", "invalid-json"} {
		t.Run(kind, func(t *testing.T) {
			fx := geminiFixture(t)
			writeFile(t, filepath.Join(fx.home, ".gemini/settings.json"), []byte(`{"security":{"folderTrust":{"enabled":true}}}`), 0600)
			scope := filepath.Join(fx.base, "allowed/projects/Scope")
			fx.project = filepath.Join(scope, "ab")
			if err := os.MkdirAll(fx.project, 0755); err != nil {
				t.Fatal(err)
			}
			parent := scope + "/資料"
			first := parent
			last := parent
			initial := `"DO_NOT_TRUST"`
			want, exit := "ask", 0
			switch kind {
			case "normalized-position":
				first = scope + "/long/../資料"
			case "raw-before-normalized":
				first = scope + "/x/../ab"
				last = first
				initial = `"TRUST_FOLDER"`
				want = "unavailable"
			case "raw-type-overwritten":
				initial = `42`
			}
			rules := fmt.Sprintf(`{%q:%s,%q:"DO_NOT_TRUST",%q:"TRUST_PARENT"}`, first, initial, fx.project, last)
			switch kind {
			case "long-logical-symlink", "short-logical-symlink":
				alias := filepath.Join(scope, "a-much-longer-logical-key")
				if kind == "short-logical-symlink" {
					alias = filepath.Join(scope, "a")
					want = "unavailable"
				}
				if err := os.Symlink(fx.project, alias); err != nil {
					t.Fatal(err)
				}
				rules = fmt.Sprintf(`{%q:"TRUST_FOLDER",%q:"DO_NOT_TRUST"}`, alias, fx.project)
			case "relative-unknown":
				rules = `{"relative":"TRUST_FOLDER"}`
				want, exit = "unknown", 1
			case "invalid-level":
				rules = fmt.Sprintf(`{%q:42,%q:"TRUST_FOLDER"}`, parent, fx.project)
				want, exit = "unknown", 1
			case "invalid-json":
				rules = `{`
				want, exit = "unknown", 1
			}
			writeFile(t, filepath.Join(fx.home, ".gemini/trustedFolders.json"), []byte(rules), 0600)
			for _, alias := range []string{".gemini", ".agents"} {
				codexSkill(t, filepath.Join(fx.project, alias, "skills"), "source", "order"+alias)
			}
			before := treeManifest(t, fx.base)
			list, _ := round4List(t, fx, exit)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if len(list.Sources) != 2 {
				t.Fatalf("expected both project aliases, got %d", len(list.Sources))
			}
			for _, source := range list.Sources {
				if source.Availability.State != want {
					t.Errorf("%s: state %s, want %s", source.SourcePath, source.Availability.State, want)
				}
			}
			if exit != 0 {
				found := false
				for _, problem := range list.Problems {
					found = found || problem.Code == "native-input"
				}
				if !found {
					t.Error("uncertain input lacks native-input diagnostic")
				}
			}
			if kind == "relative-unknown" {
				reason := "relative trusted-folder keys require an uninspected native working directory"
				count := 0
				for _, gap := range list.Scope.Agent.Gaps {
					if gap == reason {
						count++
					}
				}
				if count != 1 {
					t.Errorf("relative-key limitation occurs %d times, want once", count)
				}
				ordinary := fx.run(t, "list", "--agent", "gemini", "--project", fx.project)
				assertExit(t, ordinary, exit)
				if count := strings.Count(string(ordinary.stdout), "Unverified: "+reason); count != 1 {
					t.Errorf("ordinary scope repeats or omits relative-key limitation (%d):\n%s", count, ordinary.stdout)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
		})
	}
}

func TestRound4R14TerminalFinalTrust(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, finalTrust := range []bool{true, false} {
		for _, alias := range []string{".gemini", ".agents"} {
			for _, width := range []int{48, 80} {
				t.Run(fmt.Sprintf("%t/%s/%d", finalTrust, alias, width), func(t *testing.T) {
					fx := geminiFixture(t)
					projectSettings := filepath.Join(fx.project, ".gemini/settings.json")
					writeFile(t, projectSettings, []byte(fmt.Sprintf(`{"security":{"folderTrust":{"enabled":%t}}}`, finalTrust)), 0600)
					codexSkill(t, filepath.Join(fx.project, alias, "skills"), "source", "final-trust-control")
					before := treeManifest(t, fx.base)
					list, _ := round4List(t, fx, 0)
					if len(list.Sources) != 1 {
						t.Fatal("expected one source")
					}
					setting := "security.folderTrust.enabled=false"
					input := projectSettings
					state := "ask"
					if finalTrust {
						setting = "trustedFolders"
						input = filepath.Join(fx.home, ".gemini/trustedFolders.json")
						state = "unavailable"
					}
					explained := fx.run(t, "explain", "--agent", "gemini", "--project", fx.project, list.Sources[0].ID)
					assertExit(t, explained, 0)
					for _, expected := range []string{"Discovery setting: " + setting, "Discovery input: " + input, "Availability: " + state} {
						if !strings.Contains(string(explained.stdout), expected) {
							t.Errorf("ordinary explanation omits %s:\n%s", expected, explained.stdout)
						}
					}
					socket := fmt.Sprintf("ale37-r14-%d-%d", os.Getpid(), width)
					defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
					command := fx.productCommand(t, "browse", "--agent", "gemini", "--project", fx.project)
					runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", fmt.Sprint(width), "-y", "24", "-s", "browser")
					runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
					runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", fmt.Sprint(width), "-y", "24")
					waitForPane(t, tmux, fx, socket, false, "Skillglass", "final-trust-control")
					frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Read lines")
					rows := map[int]string{}
					content := ""
					prefix := fmt.Sprintf("round4/r14-browser/%t/%s/%d", finalTrust, alias, width)
					for page := 0; page < 80; page++ {
						assertPaneCells(t, frame, width, 24)
						first, last, total, body := sourceDetailBody(t, frame)
						for i, row := range body {
							rows[first+i] = row
						}
						ordered := make([]string, total)
						for i := range ordered {
							ordered[i] = rows[i+1]
						}
						content = strings.Join(ordered, "\n")
						writeEvidence(t, fmt.Sprintf("%s/page-%02d.txt", prefix, page), []byte(frame))
						if paneContains(content, "Discovery setting: "+setting) && paneContains(content, "Discovery input: "+input) || last == total {
							break
						}
						previous := first
						frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool { first, _, _ := sourceDetailRange(next.cells); return first > previous }).cells
					}
					writeEvidence(t, prefix+"/collected.txt", []byte(content))
					for _, expected := range []string{"Discovery setting: " + setting, "Discovery input: " + input, "Availability: " + state} {
						if !paneContains(content, expected) {
							t.Errorf("browser omits final trust evidence %s", expected)
						}
					}
					assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				})
			}
		}
	}
}
