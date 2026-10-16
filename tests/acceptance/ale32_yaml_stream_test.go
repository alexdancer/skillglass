package acceptance_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE32YAMLStreamAdmissionAndPreservation(t *testing.T) {
	type streamCase struct {
		name, raw, choice, after string
		ready, nativeValid       bool
	}
	tests := []streamCase{
		{"append-after-end", "max_turns: 9\n...\n", "disable", "", false, true},
		{"append-after-commented-end", "max_turns: 9\r\n... # retained\r\n", "disable", "", false, true},
		{"append-after-explicit-document", "---\nmax_turns: 9\n...\n# footer\n", "disable", "", false, true},
		{"empty-map-expansion", "# retained\n{} # map\n... # end\n", "disable", "# retained\nskills:\n  disabled: [\"ale32-target\"] # map\n... # end\n", true, true},
		{"replace-disable", "max_turns: 9\nskills:\n  disabled: [unrelated]\n...\n", "disable", "max_turns: 9\nskills:\n  disabled: [\"unrelated\",\"ale32-target\"]\n...\n", true, true},
		{"replace-enable", "max_turns: 9\nskills:\n  disabled: [unrelated, ale32-target]\n...\n", "enable", "max_turns: 9\nskills:\n  disabled: [\"unrelated\"]\n...\n", true, true},
		{"noop-enable", "max_turns: 9\n...\n", "enable", "max_turns: 9\n...\n", true, true},
		{"noop-empty-map", "{}\n...\n", "enable", "{}\n...\n", true, true},
		{"noop-disable", "skills:\n  disabled: [ale32-target]\n...\n", "disable", "skills:\n  disabled: [ale32-target]\n...\n", true, true},
		{"ordinary-append", "# retained\nmax_turns: 9\n", "disable", "# retained\nmax_turns: 9\nskills:\n  disabled: [\"ale32-target\"]\n", true, true},
	}
	for _, root := range []struct{ name, raw string }{
		{"mapping", "max_turns: 9\n"},
		{"empty-map", "{}\n"},
		{"replacement", "skills:\n  disabled: [ale32-target]\n"},
	} {
		for _, suffix := range []struct{ name, raw string }{
			{"second-document", "---\nother: true\n"},
			{"empty-second-document", "---\n"},
			{"ended-empty-document", "...\n---\n...\n"},
			{"trailing-content", "...\nother: true\n"},
		} {
			for _, choice := range []string{"enable", "disable"} {
				tests = append(tests, streamCase{root.name + "-" + suffix.name + "-" + choice, root.raw + suffix.raw, choice, "", false, false})
			}
		}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fx, profile, id, _ := hermesTarget(t)
			config := filepath.Join(profile, "config.yaml")
			writeFile(t, config, []byte(test.raw), 0600)
			before := treeManifest(t, fx.base)
			listing := hermesRun(t, fx, nil, "list", "--agent", "hermes", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, listing.stdout, &list)
			capabilityState := ""
			for _, capability := range codexNamed(t, list, "ale32-target").Availability.Capabilities {
				if capability.Operation == test.choice && capability.Scope == "global" {
					capabilityState = capability.State
				}
			}
			preview := changeCommand(t, fx, "", test.choice, "--agent", "hermes", "--project", fx.project, "--scope", "global", "--json", id)
			var p changeReceipt
			decodeJSON(t, preview.stdout, &p)
			var applied *changeReceipt
			applyCode := -1
			if p.State == "ready" {
				result := changeCommand(t, fx, "", test.choice, "--agent", "hermes", "--project", fx.project, "--scope", "global", "--json", "--confirm", p.Confirmation, id)
				var out changeReceipt
				decodeJSON(t, result.stdout, &out)
				applied = &out
				applyCode = result.code
			}
			actual := readBytes(t, config)
			evidence, err := json.MarshalIndent(map[string]any{
				"before": test.raw, "after": string(actual), "native_valid": test.nativeValid,
				"choice": test.choice, "expected_ready": test.ready, "capability": capabilityState, "preview": p, "applied": applied, "apply_exit": applyCode,
			}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			writeEvidence(t, "ale32-yaml-stream/"+test.name+".json", evidence)
			if (capabilityState == "supported") != test.ready {
				t.Errorf("%s capability = %s, expected ready = %t", test.choice, capabilityState, test.ready)
			}
			if !test.ready {
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if preview.code == 0 || p.State == "ready" || p.Confirmation != "" {
					t.Fatalf("unvalidated stream admitted: %+v", p)
				}
				return
			}
			assertExit(t, preview, 0)
			if applied == nil {
				t.Fatalf("expected executable preview: %+v", p)
			}
			if applyCode != 0 {
				t.Fatalf("apply exit = %d: %+v", applyCode, applied)
			}
			assertSavedChange(t, *applied)
			if !bytes.Equal(actual, []byte(test.after)) {
				t.Fatalf("unrelated bytes or requested membership changed: got %q, want %q", actual, test.after)
			}
			if strings.HasPrefix(test.name, "noop-") {
				if !p.NoWriteNeeded {
					t.Fatal("no-op requested a write")
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
		})
	}
}
