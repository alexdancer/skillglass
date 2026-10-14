package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVerifyReviewC086QuotedRootDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name, declaration, key, shell, state, root string
		managed                                    bool
		exit                                       int
	}{
		{"single-quoted-key", "'SKILL_ROOT'=fresh\n", "SKILL_ROOT", "stale", "unknown", "", false, 1},
		{"export-single-quoted-key", "export 'SKILL_ROOT'=fresh\n", "SKILL_ROOT", "stale", "unknown", "", false, 1},
		{"tab-export-single-quoted-key", "export\t'SKILL_ROOT'=fresh\n", "SKILL_ROOT", "stale", "unknown", "", false, 1},
		{"unquoted-declaration-control", "SKILL_ROOT=fresh\n", "SKILL_ROOT", "stale", "unknown", "", false, 1},
		{"export-unquoted-control", "export SKILL_ROOT=fresh\n", "SKILL_ROOT", "stale", "unknown", "", false, 1},
		{"no-declaration-control", "", "SKILL_ROOT", "stale", "available", "stale", false, 0},
		{"unrelated-quoted-key-control", "'OTHER_ROOT'=fresh\n", "SKILL_ROOT", "stale", "available", "stale", false, 0},
		{"double-quoted-literal-key-control", "\"SKILL_ROOT\"=fresh\n", "SKILL_ROOT", "stale", "available", "stale", false, 0},
		{"managed-single-quoted-key", "'SKILL_ROOT'=fresh\n", "SKILL_ROOT", "stale", "unknown", "", true, 1},
		{"supported-behavioral-key-control", "HERMES_PLATFORM=fresh\n", "HERMES_PLATFORM", "stale", "available", "fresh", false, 0},
		{"quoted-behavioral-key-control", "'HERMES_PLATFORM'=fresh\n", "HERMES_PLATFORM", "stale", "unknown", "", false, 1},
		{"equals-in-quoted-key-control", "'SKILL_ROOT=SUFFIX'=fresh\n", "SKILL_ROOT=SUFFIX", "", "unknown", "", false, 1},
		{"equals-key-does-not-declare-prefix-control", "'SKILL_ROOT=SUFFIX'=fresh\n", "SKILL_ROOT", "stale", "available", "stale", false, 0},
		{"managed-double-quoted-literal-key-control", "\"SKILL_ROOT\"=fresh\n", "SKILL_ROOT", "stale", "available", "stale", true, 0},
		{"quoted-without-shell-control", "'SKILL_ROOT'=fresh\n", "SKILL_ROOT", "", "unknown", "", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			local := codexSkill(t, filepath.Join(p, "skills"), "local", "local")
			codexSkill(t, filepath.Join(p, "stale"), "old", "stale-shell-source")
			codexSkill(t, filepath.Join(p, "fresh"), "new", "fresh-profile-source")
			writeFile(t, filepath.Join(p, "config.yaml"), []byte("skills:\n  external_dirs: ['${"+tc.key+"}']\n"), 0600)
			var env []string
			if tc.name == "equals-in-quoted-key-control" {
				env = append(env, "SKILL_ROOT=stale")
			}
			if tc.shell != "" {
				env = append(env, tc.key+"="+tc.shell)
			}
			if tc.declaration != "" {
				envRoot := p
				if tc.managed {
					envRoot = filepath.Join(fx.home, "managed")
					env = append(env, "HERMES_MANAGED_DIR="+envRoot)
				}
				writeFile(t, filepath.Join(envRoot, ".env"), []byte(tc.declaration), 0600)
			}
			raw, _ := json.MarshalIndent(append(fx.environment(), env...), "", "  ")
			writeEvidence(t, "environments/"+strings.ReplaceAll(t.Name(), "/", "_")+".json", raw)
			defer g2Seal(t, fx)()
			list := g2List(t, fx, tc.exit, env)
			g2Inspect(t, fx, env, list, local, tc.state)
			if tc.state == "unknown" {
				if list.Scope.Completeness != "incomplete" {
					t.Errorf("referenced persisted binding must retain incomplete scope; got %s", list.Scope.Completeness)
				}
				for _, s := range list.Sources {
					if s.Name == "stale-shell-source" && s.Availability.State != "unknown" {
						t.Errorf("stale inherited root must not claim inferred availability: %+v", s)
					}
				}
			} else {
				want, unwanted := "stale-shell-source", "fresh-profile-source"
				if tc.root == "fresh" {
					want, unwanted = unwanted, want
				}
				found := false
				for _, s := range list.Sources {
					if s.Name == want {
						found = true
						if s.Availability.State != "available" {
							t.Errorf("selected root source must be available: %+v", s)
						}
					}
					if s.Name == unwanted {
						t.Errorf("wrong environment root selected: %+v", s)
					}
				}
				if !found || list.Scope.Completeness != "complete" {
					t.Errorf("expected complete %s root: %+v", tc.root, list)
				}
			}
		})
	}
}

func TestVerifyReviewC086QuotedRootBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		for _, mode := range []string{"profile", "managed"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				fx, p := hermesFixture(t)
				local := codexSkill(t, filepath.Join(p, "skills"), "local", "local")
				codexSkill(t, filepath.Join(p, "stale"), "old", "stale-shell-source")
				codexSkill(t, filepath.Join(p, "fresh"), "new", "fresh-profile-source")
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("skills:\n  external_dirs: ['${SKILL_ROOT}']\n"), 0600)
				envRoot := p
				if mode == "managed" {
					envRoot = filepath.Join(fx.home, "managed")
					writeFile(t, filepath.Join(p, ".env"), []byte("HERMES_MANAGED_DIR="+envRoot+"\n"), 0600)
				}
				writeFile(t, filepath.Join(envRoot, ".env"), []byte("'SKILL_ROOT'=fresh\n"), 0600)
				env := []string{"SKILL_ROOT=stale"}
				raw, _ := json.MarshalIndent(append(fx.environment(), env...), "", "  ")
				writeEvidence(t, "environments/"+strings.ReplaceAll(t.Name(), "/", "_")+".json", raw)
				defer g2Seal(t, fx)()
				list := g2List(t, fx, 1, env)
				g2Inspect(t, fx, env, list, local, "unknown")
				socket := fmt.Sprintf("ale31-quoted-root-%d-%d-%s", os.Getpid(), width, mode)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := fx.productCommand(t, "browse", "--agent", "hermes", "--project", fx.project, "--availability", "unknown")
				command.Env = append(fx.environment(), env...)
				launch := runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
				assertRecordedContext(t, launch, command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "unknown (unknown)")
				prefix := fmt.Sprintf("tmux/ale31-quoted-root/%d/%s/", width, mode)
				assertPaneCells(t, frame, width, 30)
				writeEvidence(t, prefix+"00-sources.txt", []byte(frame))
				if strings.Contains(frame, "stale-shell-source") {
					t.Fatalf("stale inherited content leaked through unknown filter: %s", frame)
				}
				frame = actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() {
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter")
				}, "Read lines", "Availability: unknown")
				seenPermission, seenReason := false, false
				for step := 0; step < 100; step++ {
					assertPaneCells(t, frame, width, 30)
					writeEvidence(t, fmt.Sprintf("%s%03d-detail.txt", prefix, step), []byte(frame))
					seenPermission = seenPermission || paneContains(frame, "Configured permission: unknown")
					seenReason = seenReason || paneContains(frame, "external directory references unsupported persisted environment policy")
					if seenPermission && seenReason {
						break
					}
					var advanced bool
					frame, advanced = advanceSourceDetailFrame(t, tmux, fx, socket, frame)
					if !advanced {
						break
					}
				}
				if !seenPermission || !seenReason {
					t.Fatalf("browser lost unknown policy explanation: permission=%t reason=%t\n%s", seenPermission, seenReason, frame)
				}
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
