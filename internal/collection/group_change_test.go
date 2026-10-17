package collection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func groupFixture(t *testing.T, missing bool) (*Snapshot, SourceID, string) {
	t.Helper()
	s, id, _ := faultFixture(t, missing)
	home := s.request.home
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("HERMES_HOME", filepath.Join(home, ".hermes"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, ".pi/agent"))
	for _, root := range []string{".claude/skills", ".hermes/skills", ".pi/agent/skills", ".config/opencode/skills", ".gemini/skills"} {
		dir := filepath.Join(home, root)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(home, ".agents/skills/target"), filepath.Join(dir, "target")); err != nil {
			t.Fatal(err)
		}
	}
	for path, raw := range map[string]string{".hermes/config.yaml": "skills:\n  disabled: []\n", ".pi/agent/settings.json": "{}", ".config/opencode/opencode.json": "{}", ".gemini/settings.json": "{}"} {
		if err := os.WriteFile(filepath.Join(home, path), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !missing {
		if err := os.WriteFile(filepath.Join(home, ".claude/settings.json"), []byte("{\"keep\":true}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return s, id, home
}
func TestALE40CompositionPilot(t *testing.T) {
	for _, apps := range [][]string{{"codex", "claude-code"}, {"claude-code", "codex"}, {"codex", "claude-code", "hermes", "pi", "opencode", "gemini"}, {"gemini", "opencode", "pi", "hermes", "claude-code", "codex"}} {
		for _, missing := range []bool{false, true} {
			t.Run(strings.Join(apps, "-")+map[bool]string{true: "-missing", false: "-existing"}[missing], func(t *testing.T) {
				s, id, _ := groupFixture(t, missing)
				p := s.PreviewGroupChange(context.Background(), GroupIntent{id, apps, ControlGlobal, ControlDisable})
				if p.view.State != "ready" {
					for _, m := range p.view.Members {
						t.Log(m.Application, m.Preview.State, m.Preview.Reason)
					}
					t.Fatal(p.view.Reason)
				}
				out := p.Apply(context.Background(), p.View().Confirmation)
				if out.ExitCode() != 0 {
					t.Fatalf("outcome %+v", out)
				}
				for _, r := range out.Results {
					if r.State != "succeeded" || r.Outcome == nil || r.Outcome.Persistence != "verified" {
						t.Fatal(r)
					}
				}
			})
		}
	}
}
func TestALE40StopAndRetain(t *testing.T) {
	for _, stage := range []string{"write", "directory-sync", "external-edit", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			s, id, home := groupFixture(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := s.PreviewGroupChange(ctx, GroupIntent{id, []string{"codex", "claude-code", "hermes"}, ControlGlobal, ControlDisable})
			if p.seal == "" {
				t.Fatal(p.view.Reason)
			}
			before, _ := os.ReadFile(filepath.Join(home, ".hermes/config.yaml"))
			if stage == "external-edit" || stage == "cancel" {
				p.children[0].plan.fault = func(current string) error {
					if current == "postread" {
						if stage == "cancel" {
							cancel()
						} else {
							return os.WriteFile(filepath.Join(home, ".claude/settings.json"), []byte("{\"external\":true}"), 0600)
						}
					}
					return nil
				}
			} else {
				p.children[1].plan.fault = func(current string) error {
					if current == stage {
						return errors.New("injected second writer failure")
					}
					return nil
				}
			}
			out := p.Apply(ctx, p.View().Confirmation)
			if out.ExitCode() == 0 || out.Results[0].State != "succeeded" || out.Results[2].State != "not-attempted" {
				t.Fatalf("%+v", out)
			}
			expected := "failed"
			if stage == "cancel" {
				expected = "not-attempted"
			}
			if out.Results[1].State != expected {
				t.Fatal(out.Results)
			}
			raw, _ := os.ReadFile(filepath.Join(home, ".codex/config.toml"))
			if !strings.Contains(string(raw), "enabled = false") {
				t.Fatal("earlier success lost", string(raw))
			}
			after, _ := os.ReadFile(filepath.Join(home, ".hermes/config.yaml"))
			if string(before) != string(after) {
				t.Fatal("third writer touched")
			}
			if stage == "directory-sync" && out.Results[1].Outcome.WriteState != "committed" {
				t.Fatal("persisted failure evidence lost")
			}
		})
	}
}
func TestALE40ConsentAndCopies(t *testing.T) {
	s, id, home := groupFixture(t, false)
	intent := GroupIntent{id, []string{"codex", "claude-code"}, ControlGlobal, ControlDisable}
	p := s.PreviewGroupChange(context.Background(), intent)
	original := p.View().Confirmation
	view := p.View()
	view.Intent.Applications[0] = "codeaf"
	view.Consumers[0].Scope.Roots = nil
	view.Members[0].Preview.Before.Reason = "mutated"
	if p.View().Confirmation != original || p.View().Intent.Applications[0] != "codex" {
		t.Fatal("view changed authority")
	}
	if p.Apply(context.Background(), "bad").ExitCode() == 0 {
		t.Fatal("wrong token")
	}
	if err := os.WriteFile(filepath.Join(home, ".claude/settings.json"), []byte("{\"external\":true}"), 0600); err != nil {
		t.Fatal(err)
	}
	out := p.Apply(context.Background(), original)
	if out.ExitCode() == 0 || !strings.Contains(out.Reason, "stale") {
		t.Fatal(out)
	}
	if p.Apply(context.Background(), original).ExitCode() == 0 {
		t.Fatal("reused token")
	}
	for _, apps := range [][]string{{"codex", "codex"}, {"codex", "unknown"}, {"codex", "codeaf"}, nil} {
		p = s.PreviewGroupChange(context.Background(), GroupIntent{id, apps, ControlGlobal, ControlDisable})
		if p.View().State == "ready" || p.View().Confirmation != "" {
			t.Fatal(p.view.Reason)
		}
	}
}
