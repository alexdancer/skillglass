package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type d4Producer struct {
	name, fields, state, invocation, rule string
	settings                              map[string]any
}

func d4Inspect(t *testing.T, fx fixture, label, target, state, invocation, rule string, uncertain bool) codexList {
	t.Helper()
	before := treeManifest(t, fx.base)
	args := []string{"list", "--agent", "claude-code", "--project", fx.project, "--json"}
	r := fx.run(t, args...)
	assertExit(t, r, 0)
	writeEvidence(t, "matrix/"+label+"-list.json", r.stdout)
	var list codexList
	decodeJSON(t, r.stdout, &list)
	s := claudeAt(t, list, target)
	if s.Availability.State != state {
		t.Errorf("state=%s want %s", s.Availability.State, state)
	}
	if s.Availability.Invocation.State != invocation {
		t.Errorf("invocation=%s want %s", s.Availability.Invocation.State, invocation)
	}
	if state != "shadowed" && rule != "" && s.Availability.Evidence.Rule != rule {
		t.Errorf("availability rule=%s want %s", s.Availability.Evidence.Rule, rule)
	}
	if uncertain && (s.Availability.Precedence.State != "unknown" || s.Availability.Precedence.Winner != "" || len(s.Availability.Precedence.RelatedSources) == 0) {
		t.Errorf("winner uncertainty or related identity lost: %+v", s.Availability.Precedence)
	}
	if state == "shadowed" && s.Availability.Evidence.Kind != "inferred" {
		t.Errorf("known shadow evidence=%s", s.Availability.Evidence.Kind)
	}
	reasons := map[string]string{
		"disabled/skill-visibility-override":    "skillOverrides=off hides and refuses both model and explicit invocation",
		"unavailable/invocation-frontmatter":    "both model and user invocation are excluded by frontmatter",
		"unavailable/skill-visibility-override": "settings hide model invocation and frontmatter hides user invocation",
		"unavailable/skill-tool-deny":           "model Skill tool is denied and user invocation is hidden",
	}
	if reason, excluded := reasons[state+"/"+rule]; excluded {
		if s.Availability.Reason != reason || s.Availability.Evidence.Kind != "inferred" || !reflect.DeepEqual(s.Availability.Evidence, s.Availability.Invocation.Evidence) {
			t.Errorf("independent invocation exclusion lost reason or evidence: %+v", s.Availability)
		}
	}
	if uncertain && (s.Availability.NativeNameEvidence.Kind != "inferred" || s.Availability.Precedence.Evidence.Kind != "unknown") {
		t.Errorf("native name and winner confidence conflated: %+v", s.Availability)
	}
	h := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, s.ID)
	assertExit(t, h, 0)
	writeEvidence(t, "matrix/"+label+"-explain.txt", h.stdout)
	if !strings.Contains(string(h.stdout), "Availability: "+state) {
		t.Errorf("human explain lacks %s", state)
	}
	for _, related := range s.Availability.Precedence.RelatedSources {
		if !strings.Contains(string(h.stdout), related) {
			t.Errorf("human explain missing related %s", related)
		}
	}
	j := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, "--json", s.ID)
	assertExit(t, j, 0)
	writeEvidence(t, "matrix/"+label+"-explain.json", j.stdout)
	var detail struct{ Source codexSource }
	decodeJSON(t, j.stdout, &detail)
	if !reflect.DeepEqual(s.Availability, detail.Source.Availability) {
		t.Error("list/explain mismatch")
	}
	for _, filter := range slices.Compact([]string{state, "unknown", "shadowed"}) {
		f := fx.run(t, append(append([]string{}, args...), "--availability", filter)...)
		assertExit(t, f, 0)
		writeEvidence(t, "matrix/"+label+"-"+filter+"-filter.json", f.stdout)
		var filtered codexList
		decodeJSON(t, f.stdout, &filtered)
		found := false
		for _, v := range filtered.Sources {
			found = found || v.ID == s.ID
		}
		if found != (filter == state) {
			t.Errorf("%s membership=%t want %t", filter, found, filter == state)
		}
	}
	observation, _ := json.MarshalIndent(map[string]any{"label": label, "source": s, "expected_state": state, "expected_invocation": invocation}, "", "  ")
	writeEvidence(t, "matrix/"+label+"-observation.json", observation)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	return list
}

func TestALE29StateProducerPrecedenceMatrix(t *testing.T) {
	producers := []d4Producer{
		{"override-off", "", "disabled", "disabled", "skill-visibility-override", map[string]any{"skillOverrides": map[string]string{"work": "off"}}},
		{"both-invokers", "disable-model-invocation: true\nuser-invocable: false\n", "unavailable", "no-invoker", "invocation-frontmatter", nil},
		{"user-only-hidden", "user-invocable: false\n", "unavailable", "no-invoker", "skill-visibility-override", map[string]any{"skillOverrides": map[string]string{"work": "user-invocable-only"}}},
		{"deny-hidden", "user-invocable: false\n", "unavailable", "no-invoker", "skill-tool-deny", map[string]any{"permissions": map[string]any{"deny": []string{"Skill(work)"}}}},
		{"available", "", "available", "automatic-and-user", "invocation-frontmatter", nil},
		{"explicit-only", "disable-model-invocation: true\n", "explicit-only", "explicit-only", "invocation-frontmatter", nil},
		{"ask", "", "ask", "automatic-and-user", "skill-tool-ask", map[string]any{"permissions": map[string]any{"ask": []string{"Skill(work)"}}}},
		{"unknown-invocation", "user-invocable: maybe\n", "unknown", "unknown", "invocation-frontmatter", nil},
		{"model-only", "user-invocable: false\n", "available", "model-only", "invocation-frontmatter", nil},
	}
	for _, p := range producers {
		for _, branch := range []string{"unique", "possible-higher", "top-tie", "known-higher"} {
			t.Run(p.name+"/"+branch, func(t *testing.T) {
				fx, _ := codexFixture(t)
				personal := filepath.Join(fx.home, ".claude/skills")
				project := filepath.Join(fx.project, ".claude/skills")
				targetRoot := project
				if branch == "top-tie" {
					targetRoot = personal
				}
				target := filepath.Dir(claudeSkill(t, targetRoot, "target", "work", p.fields))
				if p.settings != nil {
					claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), p.settings)
				}
				state, rule := p.state, p.rule
				uncertain := branch == "possible-higher" || branch == "top-tie"
				switch branch {
				case "possible-higher":
					shared := filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/shared"), "bundle", "", ""))
					claudePrecedenceAlias(t, shared, filepath.Join(personal, "work"))
					claudePrecedenceAlias(t, shared, filepath.Join(personal, "other"))
				case "top-tie":
					claudeSkill(t, personal, "contender", "work", "")
				case "known-higher":
					claudeSkill(t, personal, "contender", "work", "")
					state = "shadowed"
					rule = "startup-name-precedence"
				}
				if uncertain && p.invocation != "disabled" && p.invocation != "no-invoker" {
					state = "unknown"
					rule = "startup-name-precedence"
				}
				list := d4Inspect(t, fx, p.name+"-"+branch, target, state, p.invocation, rule, uncertain)
				if branch == "possible-higher" {
					for _, s := range list.Sources {
						if s.SourcePath != canonicalPath(t, target) && (s.Availability.State != "unknown" || s.Availability.NativeNameEvidence.Kind != "unknown") {
							t.Errorf("ambiguous source incorrectly definite: %+v", s)
						}
					}
				}
				if branch == "unique" {
					s := claudeAt(t, list, target)
					if s.Availability.Precedence.Winner != s.ID || s.Availability.Precedence.State != "unique" {
						t.Errorf("unique source lost identity: %+v", s)
					}
				}
			})
		}
	}
}

func TestALE29DisabledContenderStillShadowsLower(t *testing.T) {
	fx, _ := codexFixture(t)
	personal := filepath.Join(fx.home, ".claude/skills")
	project := filepath.Join(fx.project, ".claude/skills")
	top := filepath.Dir(claudeSkill(t, personal, "top", "work", "disable-model-invocation: true\nuser-invocable: false\n"))
	low := filepath.Dir(claudeSkill(t, project, "low", "work", ""))
	list := d4Inspect(t, fx, "disabled-top", top, "unavailable", "no-invoker", "invocation-frontmatter", false)
	winner := claudeAt(t, list, top)
	lower := claudeAt(t, list, low)
	if lower.Availability.State != "shadowed" || lower.Availability.Precedence.Winner != winner.ID || !slices.Contains(lower.Availability.Precedence.RelatedSources, winner.ID) {
		t.Errorf("disabled contender removed from competition: %+v", lower)
	}
}

func TestALE29SharedNameSingleOverrideRemainsUnknown(t *testing.T) {
	fx, _ := codexFixture(t)
	personal := filepath.Join(fx.home, ".claude/skills")
	shared := filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/shared"), "bundle", "", ""))
	claudePrecedenceAlias(t, shared, filepath.Join(personal, "work"))
	claudePrecedenceAlias(t, shared, filepath.Join(personal, "other"))
	claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), map[string]any{"skillOverrides": map[string]string{"work": "off"}})
	list := d4Inspect(t, fx, "shared-single-override", shared, "unknown", "unknown", "shared-native-name", false)
	s := claudeAt(t, list, shared)
	if s.Availability.NativeNameEvidence.Kind != "unknown" || s.Availability.Precedence.Winner != "" {
		t.Errorf("shared name gained confidence: %+v", s)
	}
}

func TestALE29UncertainWinnerKeepsInvocationExclusionBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ width, height int }{{48, 24}, {80, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			fx, _ := codexFixture(t)
			personal := filepath.Join(fx.home, ".claude/skills")
			project := filepath.Join(fx.project, ".claude/skills")
			for _, names := range [][]string{{"work", "other"}, {"stop", "another"}} {
				shared := filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/shared"), names[0], "", ""))
				for _, name := range names {
					claudePrecedenceAlias(t, shared, filepath.Join(personal, name))
				}
			}
			claudeSkill(t, project, "disabled-target", "work", "")
			claudeSkill(t, project, "no-invoker-target", "stop", "disable-model-invocation: true\nuser-invocable: false\n")
			claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), map[string]any{"skillOverrides": map[string]string{"work": "off"}})
			before := treeManifest(t, fx.base)
			socket := fmt.Sprintf("invocation-precedence-%d", size.width)
			defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
			command := "exec " + fx.productShell(t) + " browse --agent claude-code --project " + shellQuote(fx.project) + " --availability disabled"
			runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height), "-s", "browser", command)
			runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
			runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height))
			frame := waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", "work", "disabled (inferred)", "Focus: 1/1")
			assertPaneCells(t, frame, size.width, size.height)
			prefix := fmt.Sprintf("tmux/invocation-precedence/%dx%d/", size.width, size.height)
			writeEvidence(t, prefix+"disabled-filter.txt", []byte(frame))
			send := func(key string, needles ...string) string {
				return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
			}
			collect := func(label string, needles ...string) {
				frame := send("Enter", "Read lines")
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
						}
						observed := strings.Join(ordered, "\n")
						writeEvidence(t, prefix+label+"-body.txt", []byte(observed))
						for _, needle := range needles {
							if !paneContains(observed, needle) {
								t.Errorf("browser detail lacks %q:\n%s", needle, observed)
							}
						}
						if paneContains(observed, "Winning source:") {
							t.Error("browser manufactured a winner identity")
						}
						return
					}
					frame = actOnPane(t, tmux, fx, socket, "browser:0.0", func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "PageDown") }, func(next paneSnapshot) bool {
						nextFirst, _, _ := sourceDetailRange(next.cells)
						return nextFirst > first
					}).cells
				}
				t.Fatal("invocation precedence detail pager did not finish")
			}
			collect("disabled", "Availability: disabled (inferred)", "skill-visibility-override", "skillOverrides.work=off", "Precedence: unknown (unknown)", "Related independent source:", "Invocation: disabled (inferred)", "unsupported")
			send("Escape", "SKILLGLASS | Sources")
			for _, state := range []string{"explicit-only", "ask", "shadowed", "unavailable"} {
				frame = send("a", "Availability filter: "+state)
				assertPaneCells(t, frame, size.width, size.height)
			}
			frame = waitForPane(t, tmux, fx, socket, false, "stop", "unavailable (inferred)", "Focus: 1/1")
			writeEvidence(t, prefix+"unavailable-filter.txt", []byte(frame))
			collect("no-invoker", "Availability: unavailable (inferred)", "invocation-frontmatter", "Precedence: unknown (unknown)", "Related independent source:", "Invocation: no-invoker (inferred)", "unsupported")
			send("Escape", "SKILLGLASS | Sources")
			frame = send("a", "Availability filter: unknown", "Focus: 1/2")
			assertPaneCells(t, frame, size.width, size.height)
			writeEvidence(t, prefix+"unknown-filter.txt", []byte(frame))
			runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
