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

func TestGeneration2Decision1Collisions(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		amb        bool
	}{
		{"platform-filtered-competitor", "platform", true},
		{"disabled-basename-competitor", "disabled", true},
		{"basename-vs-fullname", "basename", true},
		{"distinct-fullnames-shared64prefix", "prefix", false},
		{"same-resolved-manifest-two-bundles", "manifest", false},
		{"fullnames-beyond4000window", "window", true},
		{"independent-same-name-control", "normal", true},
		{"directory-alias-control", "diralias", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			aName, bName := "collision", "collision"
			aDir, bDir := "first", "second"
			aExtra, bExtra := "", ""
			switch tc.mode {
			case "platform":
				aExtra = "platforms: [windows]\n"
			case "disabled":
				aName = "denied"
				aDir = "collision"
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("skills:\n  external_dirs: [shared]\n  disabled: [denied]\n"), 0600)
			case "basename":
				aName = "other"
				aDir = "collision"
			case "prefix":
				aName = strings.Repeat("x", 64) + "a"
				bName = strings.Repeat("x", 64) + "b"
			case "window":
				aExtra = "padding: '" + strings.Repeat("x", 4100) + "'\n"
				bExtra = aExtra
			}
			a := codexSkill(t, filepath.Join(p, "skills"), aDir, aName)
			writeFile(t, a, []byte(g2Text(aName, aExtra)), 0600)
			var b string
			if tc.mode == "diralias" {
				b = filepath.Join(p, "shared/alias/SKILL.md")
				if e := os.Symlink(filepath.Dir(a), filepath.Dir(b)); e != nil {
					t.Fatal(e)
				}
			} else {
				b = codexSkill(t, filepath.Join(p, "shared"), bDir, bName)
				writeFile(t, b, []byte(g2Text(bName, bExtra)), 0600)
				if tc.mode == "manifest" {
					if e := os.Remove(b); e != nil {
						t.Fatal(e)
					}
					if e := os.Symlink(a, b); e != nil {
						t.Fatal(e)
					}
				}
			}
			defer g2Seal(t, fx)()
			list := g2List(t, fx, 0, nil)
			selected := b
			state := "shadowed"
			switch tc.mode {
			case "platform", "disabled", "basename", "window":
				state = "available"
			case "diralias":
				selected = a
				state = "available"
			}
			g2Inspect(t, fx, nil, list, selected, state)
			source := gateSourceAt(t, list, filepath.Dir(selected))
			if (source.Availability.Invocation.State == "ambiguous-bare-name") != tc.amb {
				t.Errorf("invocation=%s; want ambiguous=%t", source.Availability.Invocation.State, tc.amb)
			}
			invocation := g2ExplicitInvocation(t, fx, source)
			want := "unambiguous-bare-name"
			if tc.amb {
				want = "ambiguous-bare-name"
			}
			if invocation.State != want || invocation.Evidence.Kind != "inferred" || invocation.Evidence.Rule != "native-explicit-selectors" {
				t.Errorf("explicit matching facts=%+v; want %s inferred", invocation, want)
			}
			if !strings.Contains(invocation.Evidence.Setting, strconv.Quote(bName)) {
				t.Errorf("full explicit selector lost: %+v", invocation)
			}
			if tc.mode == "basename" || tc.mode == "disabled" {
				for _, fact := range []string{`selector "collision": ambiguous-bare-name (2 known distinct manifests)`, `selector "second": unambiguous-bare-name (1 known distinct manifests)`} {
					if !strings.Contains(invocation.Evidence.Reason, fact) {
						t.Errorf("selector-specific collision set lost %q: %+v", fact, invocation)
					}
				}
			}
			if tc.mode == "manifest" && len(list.Sources) != 2 {
				t.Errorf("independent bundle identities merged")
			}
			if tc.mode == "diralias" && len(gateSourceAt(t, list, filepath.Dir(a)).Availability.References) != 2 {
				t.Errorf("shared references lost")
			}
		})
	}
}

type g2InvocationFact struct {
	State    string
	Evidence struct{ Kind, Rule, Setting, Reason string }
}

func g2ExplicitInvocation(t *testing.T, fx fixture, source codexSource) g2InvocationFact {
	t.Helper()
	result := hermesRun(t, fx, nil, "read", "--agent", "hermes", "--project", fx.project, "--json", source.ID)
	if result.code != 0 && result.code != 1 {
		t.Fatalf("read exit=%d: %s", result.code, result.stderr)
	}
	var detail struct {
		Source struct {
			Availability struct{ Invocation g2InvocationFact }
		}
	}
	decodeJSON(t, result.stdout, &detail)
	return detail.Source.Availability.Invocation
}

func TestGeneration2Decision1ExplicitConfidence(t *testing.T) {
	for _, mode := range []string{"uncertain-competitor", "known-collision-with-uncertainty", "shared-uncertain-name", "inspection-only-competitor", "inactive-org-competitor", "excluded-root-ancestor", "support-directory-competitor", "shared-reference-names"} {
		t.Run(mode, func(t *testing.T) {
			fx, p := hermesFixture(t)
			a := codexSkill(t, filepath.Join(p, "skills"), "first", "target")
			writeFile(t, a, []byte(g2Text("target", "")), 0600)
			wantState, wantKind := "unambiguous-bare-name", "inferred"
			var extra []string
			switch mode {
			case "uncertain-competitor", "known-collision-with-uncertainty":
				codexSkill(t, filepath.Join(p, "shared"), "uncertain", "on")
				wantState, wantKind = "unknown", "unknown"
				if mode == "known-collision-with-uncertainty" {
					codexSkill(t, filepath.Join(p, "shared"), "second", "target")
					wantState, wantKind = "ambiguous-bare-name", "inferred"
				}
			case "shared-uncertain-name":
				writeFile(t, a, []byte(g2Text("on", "")), 0600)
				if err := os.Symlink(filepath.Dir(a), filepath.Join(p, "shared/alias")); err != nil {
					t.Fatal(err)
				}
				wantState, wantKind = "unknown", "unknown"
			case "inspection-only-competitor":
				codexSkill(t, fx.cleanRoot, "second", "target")
				extra = []string{"--root", fx.cleanRoot}
			case "inactive-org-competitor":
				codexSkill(t, filepath.Join(p, "skills"), "_org/stale/second", "target")
			case "excluded-root-ancestor":
				codexSkill(t, filepath.Join(p, "shared/.archive"), "second", "target")
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("skills:\n  external_dirs: [shared/.archive]\n"), 0600)
				wantState = "ambiguous-bare-name"
			case "support-directory-competitor":
				codexSkill(t, filepath.Join(p, "skills"), "first/references/second", "target")
			case "shared-reference-names":
				writeFile(t, a, []byte(g2Text("target", "padding: '"+strings.Repeat("x", 4100)+"'\n")), 0600)
				if err := os.Symlink(filepath.Dir(a), filepath.Join(p, "shared/alias")); err != nil {
					t.Fatal(err)
				}
			}
			defer g2Seal(t, fx)()
			list := hermesList(t, fx, 0, nil, extra...)
			source := gateSourceAt(t, list, filepath.Dir(a))
			invocation := g2ExplicitInvocation(t, fx, source)
			if invocation.State != wantState || invocation.Evidence.Kind != wantKind {
				t.Errorf("invocation=%+v; want %s %s", invocation, wantState, wantKind)
			}
			if mode == "shared-uncertain-name" && !strings.Contains(invocation.Evidence.Reason, `selector "first": unambiguous-bare-name (1 known distinct manifests)`) {
				t.Errorf("known basename match lost to unrelated name uncertainty: %+v", invocation)
			}
			if mode == "shared-reference-names" {
				if len(source.Availability.References) != 2 || source.Availability.Precedence.State != "unknown" || source.Availability.State != "unknown" {
					t.Errorf("reference-specific index facts changed: %+v", source)
				}
				for _, name := range []string{"alias", "first", "target"} {
					if !strings.Contains(invocation.Evidence.Setting, strconv.Quote(name)) {
						t.Errorf("explicit selector %s lost: %+v", name, invocation)
					}
				}
			}
		})
	}
}

func TestGeneration2Decision1ExplicitBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		for _, mode := range []string{"excluded-collision", "distinct-full-names"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				fx, p := hermesFixture(t)
				aName, bName, extra := "collision", "collision", "platforms: [windows]\n"
				state, invocationState := "available", "ambiguous-bare-name"
				if mode == "distinct-full-names" {
					aName, bName = strings.Repeat("x", 64)+"a", strings.Repeat("x", 64)+"b"
					extra, state, invocationState = "", "shadowed", "unambiguous-bare-name"
				}
				a := codexSkill(t, filepath.Join(p, "skills"), "first", aName)
				writeFile(t, a, []byte(g2Text(aName, extra)), 0600)
				b := codexSkill(t, filepath.Join(p, "shared"), "second", bName)
				writeFile(t, b, []byte(g2Text(bName, "")), 0600)
				defer g2Seal(t, fx)()
				socket := fmt.Sprintf("ale31-explicit-%d-%d-%s", os.Getpid(), width, mode)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := "exec " + shellQuote(skillglassBinary) + " browse --agent hermes --project " + shellQuote(fx.project) + " --availability " + state
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", state+" (inferred)")
				prefix := fmt.Sprintf("tmux/ale31-explicit/%d/%s/", width, mode)
				writeEvidence(t, prefix+"00-sources.txt", []byte(frame))
				frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
				}, "Read lines", "Availability: "+state)
				seenInvocation, seenSelector := false, false
				for step := 0; step < 100; step++ {
					assertPaneCells(t, frame, width, 30)
					writeEvidence(t, fmt.Sprintf("%s%03d-detail.txt", prefix, step), []byte(frame))
					seenInvocation = seenInvocation || paneContains(frame, "Invocation: "+invocationState+" (inferred)")
					seenSelector = seenSelector || paneContains(frame, "selector "+strconv.Quote(bName)+": "+invocationState)
					if seenInvocation && seenSelector || strings.Contains(frame, "G2_FULL_CONTENT_SENTINEL") {
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
				if !seenInvocation || !seenSelector {
					t.Fatalf("browser lost selector-specific invocation: %s", frame)
				}
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
