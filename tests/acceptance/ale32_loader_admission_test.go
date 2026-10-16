package acceptance_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE32LoaderAgentAdmissionRefusals(t *testing.T) {
	for _, shape := range []struct{ name, value string }{
		{"integer", "1"}, {"boolean", "true"}, {"float", "1.5"},
		{"string", "malformed"}, {"reference", "'${AGENT}'"},
		{"list-scalar", "[1]"}, {"list-string", "[x]"}, {"pair-list", "[[key, value]]"},
		{"false", "false"}, {"zero", "0"}, {"empty-string", "''"}, {"empty-list", "[]"},
		{"unknown-construction", "{max_turns: yes}"},
	} {
		for _, legacy := range []bool{false, true} {
			for _, disabled := range []bool{false, true} {
				for _, choice := range []string{"enable", "disable"} {
					name := shape.name + "-normal-clear-" + choice
					if legacy {
						name = strings.Replace(name, "normal", "legacy", 1)
					}
					if disabled {
						name = strings.Replace(name, "clear", "disabled", 1)
					}
					t.Run(name, func(t *testing.T) {
						fx, profile, id, _ := hermesTarget(t)
						raw := "agent: " + shape.value + "\nunrelated_setting: keep\n"
						if legacy {
							raw += "max_turns: 7\n"
						}
						raw += "skills:\n  external_dirs: [shared]\n  disabled: [unrelated"
						if disabled {
							raw += ", ale32-target"
						}
						raw += "]\n"
						config := filepath.Join(profile, "config.yaml")
						writeFile(t, config, []byte(raw), 0600)
						before := treeManifest(t, fx.base)
						list := hermesList(t, fx, 0, nil)
						for _, capability := range codexNamed(t, list, "ale32-target").Availability.Capabilities {
							if capability.Scope == "global" && capability.State != "unsupported" {
								t.Errorf("profile normalization is unverified but capability is %+v", capability)
							}
						}
						result := changeCommand(t, fx, "", choice, "--agent", "hermes", "--project", fx.project, "--scope", "global", "--json", id)
						var preview changeReceipt
						decodeJSON(t, result.stdout, &preview)
						var applied *changeReceipt
						if preview.State == "ready" {
							confirmation := changeCommand(t, fx, "", choice, "--agent", "hermes", "--project", fx.project, "--scope", "global", "--json", "--confirm", preview.Confirmation, id)
							var out changeReceipt
							decodeJSON(t, confirmation.stdout, &out)
							applied = &out
						}
						evidence, err := json.MarshalIndent(map[string]any{"before": raw, "after": string(readBytes(t, config)), "preview": preview, "applied": applied}, "", "  ")
						if err != nil {
							t.Fatal(err)
						}
						writeEvidence(t, "ale32-loader/"+name+".json", evidence)
						assertTreeUnchanged(t, before, treeManifest(t, fx.base))
						assertExit(t, result, 1)
						if preview.State == "ready" || preview.Confirmation != "" || !strings.Contains(preview.Reason, "agent") {
							t.Fatalf("unsafe or unvalidated profile normalization admitted: %+v", preview)
						}
					})
				}
			}
		}
	}
}

func TestALE32LoaderAgentLiteralPositiveControls(t *testing.T) {
	for _, agent := range []struct{ name, raw string }{
		{"absent", ""}, {"null", "agent: null\n"}, {"empty-mapping", "agent: {}\n"},
		{"mapping", "agent:\n  max_turns: 7\n  unrelated: literal\n"},
	} {
		for _, legacy := range []bool{false, true} {
			name := agent.name
			if legacy {
				name += "-legacy"
			}
			t.Run(name, func(t *testing.T) {
				fx, profile, id, _ := hermesTarget(t)
				raw := agent.raw + "# keep byte-for-byte\nunrelated_setting: keep\nmodel: [not, a, provider, schema]\nprovider: 42\n"
				if legacy {
					raw += "max_turns: 11\n"
				}
				raw += "skills:\n  external_dirs: [shared]\n  disabled: [unrelated]\n"
				config := filepath.Join(profile, "config.yaml")
				writeFile(t, config, []byte(raw), 0600)
				for _, choice := range []string{"disable", "disable", "enable", "enable"} {
					before := treeManifest(t, fx.base)
					p := hermesChange(t, fx, choice, "global", id, "", 0)
					assertSavedChange(t, hermesChange(t, fx, choice, "global", id, p.Confirmation, 0))
					list := "[\"unrelated\"]"
					if choice == "disable" {
						list = "[\"unrelated\",\"ale32-target\"]"
					}
					want := strings.Replace(raw, "[unrelated]", list, 1)
					if !bytes.Equal(readBytes(t, config), []byte(want)) {
						t.Fatalf("unrelated profile data changed: got %s, want %s", readBytes(t, config), want)
					}
					if p.NoWriteNeeded {
						assertTreeUnchanged(t, before, treeManifest(t, fx.base))
					}
				}
			})
		}
	}
}

func TestALE32LoaderManagedAgentDoesNotRepairOrObstructProfile(t *testing.T) {
	fx, profile, id, _ := hermesTarget(t)
	managed := filepath.Join(fx.home, "managed")
	writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_MANAGED_DIR="+managed+"\n"), 0600)
	config := filepath.Join(profile, "config.yaml")
	raw := "agent: {}\nunrelated_setting: keep\nskills:\n  external_dirs: [shared]\n  disabled: [unrelated]\n"
	writeFile(t, config, []byte(raw), 0600)
	managedConfig := filepath.Join(managed, "config.yaml")
	for _, value := range []string{"1", "[1]", "malformed"} {
		managedRaw := "agent: " + value + "\nother: keep\n"
		writeFile(t, managedConfig, []byte(managedRaw), 0600)
		for _, choice := range []string{"disable", "enable"} {
			p := hermesChange(t, fx, choice, "global", id, "", 0)
			assertSavedChange(t, hermesChange(t, fx, choice, "global", id, p.Confirmation, 0))
			if string(readBytes(t, managedConfig)) != managedRaw {
				t.Fatal("managed policy changed")
			}
		}
	}
	p := hermesChange(t, fx, "disable", "global", id, "", 0)
	writeFile(t, config, []byte(strings.Replace(raw, "agent: {}", "agent: 1", 1)), 0600)
	writeFile(t, managedConfig, []byte("agent: {}\n"), 0600)
	before := treeManifest(t, fx.base)
	out := hermesChange(t, fx, "disable", "global", id, p.Confirmation, 1)
	if out.State == "applied" || out.Achievement == "achieved" {
		t.Fatalf("unsafe external edit bypassed admission: %+v", out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
