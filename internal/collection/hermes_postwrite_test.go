package collection

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHermesPostwriteControlCertainty(t *testing.T) {
	for _, tc := range []struct {
		label, before, change, managed, reason string
		choice                                 ControlChoice
		unknown, noWrite, profileChanged       bool
	}{
		{label: "baseline-enable", choice: ControlEnable, before: "[target]"},
		{label: "baseline-disable", choice: ControlDisable, before: "[]"},
		{label: "noop-enable", choice: ControlEnable, before: "[]", noWrite: true},
		{label: "noop-disable", choice: ControlDisable, before: "[target]", noWrite: true},
		{label: "enable-managed-denial", choice: ControlEnable, before: "[target]", managed: "skills:\n  disabled: [target]\n", unknown: true, reason: "managed policy overrides"},
		{label: "disable-managed-empty", choice: ControlDisable, before: "[]", managed: "skills:\n  disabled: []\n", unknown: true, reason: "managed policy overrides"},
		{label: "enable-managed-platform", choice: ControlEnable, before: "[target]", managed: "skills:\n  platform_disabled:\n    telegram: [target]\n", unknown: true, reason: "managed policy overrides"},
		{label: "disable-managed-platform", choice: ControlDisable, before: "[]", managed: "skills:\n  platform_disabled:\n    telegram: []\n", unknown: true, reason: "managed policy overrides"},
		{label: "profile-normalization-enable", choice: ControlEnable, before: "[target]", change: "agent", unknown: true, profileChanged: true, reason: "profile"},
		{label: "profile-normalization-disable", choice: ControlDisable, before: "[]", change: "agent", unknown: true, profileChanged: true, reason: "profile"},
		{label: "selector-collision-enable", choice: ControlEnable, before: "[target]", change: "collision", unknown: true, reason: "independent source"},
		{label: "selector-collision-disable", choice: ControlDisable, before: "[]", change: "collision", unknown: true, reason: "independent source"},
		{label: "writer-only-config-enable", choice: ControlEnable, before: "[target]", change: "config-hardlink"},
		{label: "writer-only-config-disable", choice: ControlDisable, before: "[]", change: "config-hardlink"},
		{label: "writer-only-manifest-enable", choice: ControlEnable, before: "[target]", change: "manifest-hardlink"},
		{label: "writer-only-manifest-disable", choice: ControlDisable, before: "[]", change: "manifest-hardlink"},
		{label: "managed-agent-sibling", choice: ControlDisable, before: "[]", managed: "agent: 1\n"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			root := t.TempDir()
			home, project, managed := filepath.Join(root, "home"), filepath.Join(root, "project"), filepath.Join(root, "managed")
			hermes := filepath.Join(home, ".hermes")
			source := filepath.Join(hermes, "skills", "target")
			for _, dir := range []string{project, managed, source} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", home)
			t.Setenv("HERMES_HOME", hermes)
			t.Setenv("HERMES_MANAGED_DIR", managed)
			t.Setenv("HERMES_PLATFORM", "telegram")
			config := filepath.Join(hermes, "config.yaml")
			manifest := filepath.Join(source, "SKILL.md")
			original := "unrelated_setting: keep\nskills:\n  disabled: " + tc.before + "\n"
			if err := os.WriteFile(config, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			content := []byte("---\nname: target\ndescription: Inert verification.\n---\nInert.\n")
			if err := os.WriteFile(manifest, content, 0600); err != nil {
				t.Fatal(err)
			}
			request, err := NewRequestWithAgent(nil, nil, &project, "hermes")
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := Build(context.Background(), request)
			if err != nil || len(snapshot.order) != 1 {
				t.Fatalf("discovery: %v %+v", err, snapshot)
			}
			intent := ChangeIntent{snapshot.order[0], "hermes", ControlGlobal, tc.choice}
			preview := snapshot.PreviewChange(context.Background(), intent)
			if preview.plan == nil || preview.View().NoWriteNeeded != tc.noWrite {
				t.Fatalf("preview unavailable or incorrect no-op: %+v", preview.View())
			}
			calls := 0
			preview.plan.fault = func(stage string) error {
				if stage != "postread" {
					return nil
				}
				calls++
				if tc.managed != "" {
					return os.WriteFile(filepath.Join(managed, "config.yaml"), []byte(tc.managed), 0600)
				}
				switch tc.change {
				case "agent":
					return os.WriteFile(config, append([]byte("agent: 1\n"), preview.plan.postimage...), 0600)
				case "collision":
					copy := filepath.Join(hermes, "skills", "copy")
					if err := os.Mkdir(copy, 0700); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(copy, "SKILL.md"), content, 0600)
				case "config-hardlink":
					return os.Link(config, filepath.Join(root, "config-copy"))
				case "manifest-hardlink":
					return os.Link(manifest, filepath.Join(root, "manifest-copy"))
				}
				return nil
			}
			out := preview.Apply(context.Background(), preview.View().Confirmation)
			after, err := os.ReadFile(config)
			if err != nil {
				t.Fatal(err)
			}
			want := preview.plan.postimage
			if tc.profileChanged {
				want = append([]byte("agent: 1\n"), want...)
			}
			if !bytes.Equal(after, want) {
				t.Fatalf("confirmed profile or external edit changed: %q", after)
			}
			if tc.noWrite {
				if calls != 0 || out.WriteState != "none" {
					t.Fatalf("no-op wrote: %+v calls=%d", out, calls)
				}
			} else if calls != 1 || out.WriteState != "committed" {
				t.Fatalf("wrong commit seam: calls=%d outcome=%+v", calls, out)
			}
			if (out.Persistence == "verified") == tc.profileChanged {
				t.Fatalf("persistence facts lost: %+v", out)
			}
			fresh, ok := out.Snapshot.Source(intent.Source)
			if !ok {
				t.Fatal("target disappeared")
			}
			capState, capReason := "", ""
			for _, capability := range fresh.Availability.Capabilities {
				if capability.Operation == string(tc.choice) && capability.Scope == "global" {
					capState, capReason = capability.State, capability.Evidence.Reason
				}
			}
			raw, err := json.Marshal(map[string]any{"case": tc.label, "outcome": out, "exit_code": out.ExitCode(), "profile_after": string(after), "managed_after": tc.managed, "capability": capState, "capability_reason": capReason})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("OBSERVATION %s", raw)
			if tc.unknown {
				if capState != "unsupported" || out.Achievement != "unknown" || out.ExitCode() != 1 || !strings.Contains(out.Reason, tc.reason) {
					t.Fatalf("native certainty lost but result not unknown: achievement=%q exit=%d reason=%q capability=%s", out.Achievement, out.ExitCode(), out.Reason, capState)
				}
			} else if out.Achievement != "achieved" || out.ExitCode() != 0 {
				t.Fatalf("writer capability incorrectly controlled saved achievement: %+v", out)
			}
			if strings.Contains(tc.change, "hardlink") && capState != "unsupported" {
				t.Fatalf("writer-only restriction not exercised: %s %s", capState, capReason)
			}
		})
	}
}
