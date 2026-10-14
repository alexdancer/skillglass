package acceptance_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func aliasUnknown(t *testing.T, target, possible codexSource) {
	t.Helper()
	if target.Availability.Precedence.State != "unknown" || target.Availability.Precedence.Winner != "" || !slices.Contains(target.Availability.Precedence.RelatedSources, possible.ID) {
		t.Errorf("possible source must withhold winner and remain related: %+v", target.Availability)
	}
	if !strings.Contains(target.Availability.Precedence.Evidence.Reason, "participation") {
		t.Error("unknown participation reason missing")
	}
}

func aliasManaged(t *testing.T, fx fixture, name string, enabled bool) string {
	t.Helper()
	plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
	claudeSkill(t, filepath.Dir(plugin), filepath.Base(plugin), name, "")
	claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": enabled}})
	return plugin
}

func TestALE29UnknownManagedAliasCompetitor(t *testing.T) {
	for _, mode := range []string{"no-alias-control", "excluded-alias-control", "canonical-personal-higher-control", "unknown-project-below-known-personal-control", "known-higher-above-unknown-project-control", "unknown-personal-alias"} {
		t.Run(mode, func(t *testing.T) {
			fx, repo := codexFixture(t)
			plugin := aliasManaged(t, fx, "work", false)
			if mode == "excluded-alias-control" || mode == "unknown-personal-alias" || strings.Contains(mode, "unknown-project") {
				name := "work"
				if mode == "excluded-alias-control" {
					name = "synced"
				}
				root := filepath.Join(fx.home, ".claude/skills")
				if strings.Contains(mode, "unknown-project") {
					root = filepath.Join(fx.project, ".claude/skills")
				}
				claudePrecedenceAlias(t, plugin, filepath.Join(root, name))
			}
			root := filepath.Join(fx.project, ".claude/skills")
			if mode == "unknown-project-below-known-personal-control" {
				root = filepath.Join(fx.home, ".claude/skills")
			}
			if mode == "known-higher-above-unknown-project-control" {
				root = filepath.Join(repo, ".claude/skills")
			}
			target := filepath.Dir(claudeSkill(t, root, "lower", "work", ""))
			state, managedState := "available", "unavailable"
			if strings.Contains(mode, "unknown-project") || mode == "unknown-personal-alias" {
				managedState = "unknown"
			}
			higher := ""
			if mode == "canonical-personal-higher-control" || mode == "known-higher-above-unknown-project-control" {
				higher = filepath.Dir(claudeSkill(t, filepath.Join(fx.home, ".claude/skills"), "higher", "work", ""))
				state = "shadowed"
			}
			if mode == "unknown-personal-alias" {
				state = "unknown"
			}
			want := map[string]string{target: state, plugin: managedState}
			if higher != "" {
				want[higher] = "available"
			}
			list := claudePrecedenceInspect(t, fx, "alias-"+mode, want)
			low, managed := claudeAt(t, list, target), claudeAt(t, list, plugin)
			if mode == "unknown-personal-alias" {
				aliasUnknown(t, low, managed)
			} else if higher != "" {
				if low.Availability.Precedence.Winner != claudeAt(t, list, higher).ID {
					t.Error("known higher winner lost")
				}
			} else if low.Availability.Precedence.State != "unique" || low.Availability.Precedence.Winner != low.ID {
				t.Error("definite control lost winner")
			}
			if managed.Ownership.State != "managed" {
				t.Error("managed ownership lost")
			}
			for _, ref := range managed.Availability.References {
				if ref.NativeName == "demo:work" && (ref.Participation != "excluded" || ref.Evidence.Rule != "plugin-disabled") {
					t.Errorf("plugin route changed: %+v", ref)
				}
				if ref.NativeName == "work" && ref.Evidence.Rule != "reserved-skill-name" && (ref.Participation != "unknown" || ref.Evidence.Rule != "managed-alias-unverified") {
					t.Errorf("ordinary route promoted: %+v", ref)
				}
			}
		})
	}
}

func TestALE29ParticipationConfidenceControls(t *testing.T) {
	for _, mode := range []string{"N1-equal", "N2-lower-shadow", "N3-active-plugin", "N4-matching-rank", "N5-own-upper", "N5-own-possible-control", "N6-hidden", "N7-malformed", "N8-raw-namespace", "N9-unknown-only", "N12-unrelated-inspection"} {
		t.Run(mode, func(t *testing.T) {
			fx, _ := codexFixture(t)
			personal, project := filepath.Join(fx.home, ".claude/skills"), filepath.Join(fx.project, ".claude/skills")
			state, possibleState, targetName := "unknown", "unknown", "work"
			var possible, target, lower string
			extra := []string{}
			switch mode {
			case "N5-own-upper", "N5-own-possible-control":
				possible = filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/shared"), "bundle", "work", ""))
				claudePrecedenceAlias(t, possible, filepath.Join(project, "work"))
				claudePrecedenceAlias(t, possible, filepath.Join(personal, ".work"))
				if mode == "N5-own-possible-control" {
					claudePrecedenceAlias(t, possible, filepath.Join(personal, "work"))
					state, possibleState = "shadowed", "available"
					target = filepath.Dir(claudeSkill(t, project, "target", "work", ""))
				} else {
					target = filepath.Dir(claudeSkill(t, personal, "target", "work", ""))
				}
			case "N6-hidden", "N7-malformed":
				folder := ".work"
				if mode == "N7-malformed" {
					folder = "work"
				}
				possible = filepath.Dir(claudeSkill(t, personal, folder, "work", ""))
				if mode == "N7-malformed" {
					writeFile(t, filepath.Join(possible, "SKILL.md"), []byte("---\nname: [unterminated\n"), 0600)
				}
				target = filepath.Dir(claudeSkill(t, project, "target", "work", ""))
			case "N8-raw-namespace", "N9-unknown-only":
				possible = filepath.Dir(claudeSkill(t, personal, "raw", "demo:work", ""))
				target = aliasManaged(t, fx, "work", true)
				targetName = "demo:work"
				if mode == "N9-unknown-only" {
					claudeSettings(t, filepath.Join(target, ".claude-plugin/plugin.json"), map[string]any{"name": "demo", "version": "1.0.0", "skills": []string{".", "extra"}})
				}
			case "N12-unrelated-inspection":
				possible = filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/viewer"), "work", "work", ""))
				extra = []string{"--root", filepath.Dir(possible)}
				targetName, state = "unrelated", "available"
				target = filepath.Dir(claudeSkill(t, personal, "target", targetName, ""))
			default:
				name := "work"
				if mode == "N4-matching-rank" {
					name = ""
				}
				possible = aliasManaged(t, fx, name, mode == "N3-active-plugin")
				aliasRoot := personal
				if mode == "N4-matching-rank" {
					claudePrecedenceAlias(t, possible, filepath.Join(personal, "other"))
					aliasRoot = project
					state = "available"
				}
				claudePrecedenceAlias(t, possible, filepath.Join(aliasRoot, "work"))
				targetRoot := project
				if mode == "N1-equal" || mode == "N2-lower-shadow" || mode == "N4-matching-rank" {
					targetRoot = personal
				}
				target = filepath.Dir(claudeSkill(t, targetRoot, "target", "work", ""))
				if mode == "N2-lower-shadow" {
					lower = filepath.Dir(claudeSkill(t, project, "lower", "work", ""))
				}
				if mode == "N3-active-plugin" {
					possibleState = "available"
				}
			}
			want := map[string]string{target: state, possible: possibleState}
			if lower != "" {
				want[lower] = "shadowed"
			}
			exit := 0
			if mode == "N7-malformed" {
				exit = 1
			}
			list := claudePrecedenceInspectExit(t, fx, "alias-"+mode, want, exit, extra...)
			definite, maybe := claudeAt(t, list, target), claudeAt(t, list, possible)
			if definite.Availability.NativeName != targetName {
				t.Error("raw name changed")
			}
			if state == "unknown" {
				aliasUnknown(t, definite, maybe)
				if definite.Availability.Evidence.Kind != "unknown" {
					t.Error("positive availability confidence retained")
				}
			} else if state == "available" && (definite.Availability.Precedence.State != "unique" || definite.Availability.Precedence.Winner != definite.ID) {
				t.Error("unrelated/lower possible route contaminated winner")
			}
			if mode == "N3-active-plugin" {
				for _, ref := range maybe.Availability.References {
					if ref.NativeName == "work" && (ref.Participation != "unknown" || ref.Evidence.Rule != "managed-alias-unverified") {
						t.Error("ordinary route gained plugin activation")
					}
					if ref.NativeName == "demo:work" && (ref.Participation != "eligible" || ref.Evidence.Rule != "local-marketplace-plugin") {
						t.Error("eligible plugin route lost activation")
					}
				}
			}
			if mode == "N3-active-plugin" || mode == "N5-own-possible-control" {
				if maybe.Availability.Precedence.Winner != maybe.ID || maybe.Availability.Precedence.State != "unique" {
					t.Error("same-source uncertainty became independent tie")
				}
			}
			if lower != "" {
				low := claudeAt(t, list, lower)
				if low.Availability.Precedence.Winner != "" || low.Availability.Evidence.Kind != "inferred" || !slices.Contains(low.Availability.Precedence.RelatedSources, maybe.ID) {
					t.Error("guaranteed lower exclusion lost")
				}
			}
			if mode == "N5-own-upper" && maybe.Availability.State == "shadowed" {
				t.Error("possible own higher rank erased")
			}
			if mode == "N9-unknown-only" && (definite.Availability.Evidence.Rule != "plugin-manifest-unverified" || maybe.Availability.Precedence.Winner != "") {
				t.Error("unknown-only producer promoted")
			}
			if mode == "N12-unrelated-inspection" && (maybe.Availability.Precedence.Winner != "" || len(definite.Availability.Precedence.RelatedSources) != 0) {
				t.Error("inspection source contaminated unrelated name")
			}
			if mode == "N7-malformed" && maybe.Availability.Evidence.Rule != "manifest-eligibility" {
				t.Error("manifest uncertainty lost")
			}
			for _, source := range []codexSource{definite, maybe} {
				if source.Availability.Session.State != "unknown" || source.Availability.Reload.State != "unknown" {
					t.Error("session facts changed")
				}
				for _, cap := range source.Availability.Capabilities {
					if cap.State != "unsupported" {
						t.Error("writer promoted")
					}
				}
			}
		})
	}
}

func TestALE29UnknownParticipationPreservesExclusions(t *testing.T) {
	for _, p := range []d4Producer{
		{"override-off", "", "disabled", "disabled", "skill-visibility-override", map[string]any{"skillOverrides": map[string]string{"work": "off"}}},
		{"both-invokers", "disable-model-invocation: true\nuser-invocable: false\n", "unavailable", "no-invoker", "invocation-frontmatter", nil},
		{"user-only-hidden", "user-invocable: false\n", "unavailable", "no-invoker", "skill-visibility-override", map[string]any{"skillOverrides": map[string]string{"work": "user-invocable-only"}}},
		{"deny-hidden", "user-invocable: false\n", "unavailable", "no-invoker", "skill-tool-deny", map[string]any{"permissions": map[string]any{"deny": []string{"Skill(work)"}}}},
	} {
		t.Run(p.name, func(t *testing.T) {
			fx, _ := codexFixture(t)
			plugin := aliasManaged(t, fx, "work", false)
			claudePrecedenceAlias(t, plugin, filepath.Join(fx.home, ".claude/skills/work"))
			target := filepath.Dir(claudeSkill(t, filepath.Join(fx.project, ".claude/skills"), "target", "work", p.fields))
			settings := map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}}
			for key, value := range p.settings {
				settings[key] = value
			}
			claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), settings)
			list := d4Inspect(t, fx, "alias-N11-"+p.name, target, p.state, p.invocation, p.rule, true)
			aliasUnknown(t, claudeAt(t, list, target), claudeAt(t, list, plugin))
		})
	}
}

func TestALE29UnknownAliasBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
			t.Run(fmt.Sprintf("active-%t/%dx%d", enabled, size.width, size.height), func(t *testing.T) {
				fx, _ := codexFixture(t)
				plugin := aliasManaged(t, fx, "work", enabled)
				claudePrecedenceAlias(t, plugin, filepath.Join(fx.home, ".claude/skills/work"))
				target := filepath.Dir(claudeSkill(t, filepath.Join(fx.project, ".claude/skills"), "target", "work", ""))
				before := treeManifest(t, fx.base)
				list := claudeListResult(t, fx, 0)
				low, managed := claudeAt(t, list, target), claudeAt(t, list, plugin)
				socket := "alias-confidence"
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := "exec " + fx.productShell(t) + " browse --agent claude-code --project " + shellQuote(fx.project) + " --availability unknown"
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
				frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "unknown (unknown)")
				prefix := fmt.Sprintf("tmux/alias-confidence/active-%t/%dx%d/", enabled, size.width, size.height)
				writeEvidence(t, prefix+"unknown-filter.txt", []byte(frame))
				send := func(key string, needles ...string) string {
					return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
				}
				selectRead := func(search string) {
					send("/", "Enter keep")
					runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "-l", search)
					waitForPane(t, tmux, fx, socket, false, "Focus: 1/1")
					send("Enter", "Enter read")
					send("Enter", "Read lines")
				}
				collect := func(label string, winner bool, needles ...string) {
					frame := waitForPane(t, tmux, fx, socket, false, "Read lines 1-")
					rows := map[int]string{}
					for page := 0; page < 60; page++ {
						assertPaneCells(t, frame, size.width, size.height)
						first, last, total, body := sourceDetailBody(t, frame)
						for i, row := range body {
							rows[first+i] = row
						}
						writeEvidence(t, fmt.Sprintf("%s%s-page-%02d.txt", prefix, label, page), []byte(frame))
						if last == total {
							ordered := make([]string, total)
							for i := range ordered {
								ordered[i] = rows[i+1]
								if _, ok := rows[i+1]; !ok {
									t.Fatal("body row missing")
								}
							}
							observed := strings.Join(ordered, "\n")
							writeEvidence(t, prefix+label+"-body.txt", []byte(observed))
							for _, needle := range needles {
								if !paneContains(observed, needle) {
									t.Errorf("browser lacks %q:\n%s", needle, observed)
								}
							}
							if paneContains(observed, "Winning source:") != winner {
								t.Error("browser winner confidence mismatch")
							}
							return
						}
						frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool {
							nextFirst, _, _ := sourceDetailRange(next.cells)
							return nextFirst > first
						}).cells
					}
					t.Fatal("alias pager did not finish")
				}
				selectRead("target")
				collect("target", false, "Availability: unknown (unknown)", "Precedence: unknown (unknown)", "participation", "Related independent source: "+managed.ID, "Session: unknown", "unsupported")
				send("Escape", "SKILLGLASS | Sources")
				// Restart with the managed source alone selected through the same public search.
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
				runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command = "exec " + fx.productShell(t) + " browse --agent claude-code --project " + shellQuote(fx.project)
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
				waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources")
				selectRead("demo")
				rule, state := "plugin-disabled", "unknown"
				if enabled {
					rule, state = "local-marketplace-plugin", "available"
				}
				needles := []string{"Availability: " + state, "managed-alias-unverified", rule, "demo:work", "Plugin: demo@market", "unsupported"}
				if !enabled {
					needles = append(needles, "Related independent source: "+low.ID)
				}
				collect("managed", enabled, needles...)
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}
