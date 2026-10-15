package acceptance_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func reviewManagedDirectoryFixture(t *testing.T, mode, assignment string) (fixture, string, []string, string, string) {
	t.Helper()
	fx, p := hermesFixture(t)
	stale := filepath.Join(fx.base, "allowed", "stale-managed")
	fresh := filepath.Join(fx.base, "allowed", "fresh-managed")
	writeFile(t, filepath.Join(stale, ".env"), []byte("HERMES_SAFE_MODE=1\nHERMES_ENABLE_PROJECT_PLUGINS=0\nHERMES_PLATFORM=stale\n"), 0600)
	writeFile(t, filepath.Join(fresh, ".env"), []byte("HERMES_SAFE_MODE=0\nHERMES_ENABLE_PROJECT_PLUGINS=1\nHERMES_PLATFORM=fresh\n"), 0600)
	root := p
	if mode == "project" {
		root = filepath.Join(fx.project, ".hermes")
	}
	dir := d2Plugin(t, fx, root, "portable")
	writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
	text := ""
	switch assignment {
	case "quoted":
		text = "'HERMES_MANAGED_DIR'=" + fresh + "\n"
	case "interpolated":
		text = "HERMES_MANAGED_DIR=${MANAGED_CHOICE}\n"
	case "restored":
		text = "'HERMES_MANAGED_DIR'=" + stale + "\nHERMES_MANAGED_DIR=" + fresh + "\n"
	case "supported":
		text = "HERMES_MANAGED_DIR=" + fresh + "\n"
	case "multiline":
		text = "UNRELATED='unclosed\nHERMES_MANAGED_DIR=" + fresh + "\n"
	}
	writeFile(t, filepath.Join(p, ".env"), []byte(text), 0600)
	env := []string{"HERMES_MANAGED_DIR=" + stale, "MANAGED_CHOICE=" + fresh, "HERMES_SAFE_MODE=1", "HERMES_ENABLE_PROJECT_PLUGINS=0"}
	return fx, filepath.Join(dir, "SKILL.md"), env, stale, fresh
}

func TestReviewManagedDirectoryConfidence(t *testing.T) {
	for _, mode := range []string{"safe", "project"} {
		for _, assignment := range []string{"quoted", "interpolated", "multiline", "restored", "supported", "inherited"} {
			t.Run(mode+"/"+assignment, func(t *testing.T) {
				fx, file, env, stale, fresh := reviewManagedDirectoryFixture(t, mode, assignment)
				defer g2Seal(t, fx)()
				list := d2List(t, fx, env)
				want := "unknown"
				if assignment == "restored" || assignment == "supported" {
					want = "explicit-only"
				}
				if assignment == "inherited" {
					want = "unavailable"
				}
				g2Inspect(t, fx, env, list, file, want)
				if want == "unknown" {
					for _, layer := range list.Scope.Agent.ConfigLayers {
						if strings.HasPrefix(layer.Path, stale) || strings.HasPrefix(layer.Path, fresh) {
							t.Errorf("unresolved managed selection followed candidate layer: %+v", layer)
						}
					}
				}
			})
		}
	}
}

func TestReviewManagedDirectoryDownstreamConsumers(t *testing.T) {
	fx, file, env, stale, fresh := reviewManagedDirectoryFixture(t, "safe", "quoted")
	profile := filepath.Join(fx.home, ".hermes", "profiles", "research")
	staleRoot := filepath.Join(fx.base, "allowed", "stale-external")
	codexSkill(t, staleRoot, "leaked", "stale-external")
	writeFile(t, filepath.Join(stale, ".env"), []byte("HERMES_SAFE_MODE=0\nHERMES_PLATFORM="+staleRoot+"\n"), 0600)
	writeFile(t, filepath.Join(stale, "config.yaml"), []byte("plugins:\n  disabled: [portable]\n"), 0600)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\nskills:\n  external_dirs: ['${HERMES_PLATFORM}', '${SKILL_ROOT}']\n"), 0600)
	env = append(env, "HERMES_PLATFORM="+staleRoot, "SKILL_ROOT="+staleRoot)
	excluded := codexSkill(t, filepath.Join(profile, "skills"), "excluded", "excluded")
	writeFile(t, excluded, []byte(g2Text("excluded", "platforms: [never-this-os]\n")), 0600)
	defer g2Seal(t, fx)()
	list := d2List(t, fx, env)
	g2Inspect(t, fx, env, list, file, "unknown")
	g2Inspect(t, fx, env, list, excluded, "unavailable")
	for _, source := range list.Sources {
		if source.Name == "stale-external" {
			t.Error("unresolved environment selected a stale external root")
		}
	}
	for _, layer := range list.Scope.Agent.ConfigLayers {
		if strings.HasPrefix(layer.Path, stale) || strings.HasPrefix(layer.Path, fresh) {
			t.Errorf("unresolved managed selection followed candidate layer: %+v", layer)
		}
	}
}

func TestReviewManagedDirectoryBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"safe", "project"} {
		for _, width := range []int{48, 80} {
			t.Run(fmt.Sprintf("%s/%d", mode, width), func(t *testing.T) {
				fx, _, env, _, _ := reviewManagedDirectoryFixture(t, mode, "quoted")
				defer g2Seal(t, fx)()
				socket := "review-managed-directory"
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := fx.productCommand(t, "browse", "--agent", "hermes", "--project", fx.project)
				command.Env = append(fx.environment(), env...)
				runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				waitForPane(t, tmux, fx, socket, false, "Focused: explicit")
				frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Availability: unknown", "Read lines")
				assertPaneCells(t, frame, width, 30)
				writeEvidence(t, fmt.Sprintf("review-managed/%s-%d.txt", mode, width), []byte(frame))
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
