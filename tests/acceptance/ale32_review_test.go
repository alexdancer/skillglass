package acceptance_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE32ReviewExpandedAndManagedPolicyRefusal(t *testing.T) {
	for _, field := range []string{"global", "selected-platform"} {
		for _, policy := range []string{"environment-reference", "managed-empty", "managed-null", "managed-deny", "managed-parent-null", "managed-invalid"} {
			t.Run(field+"/"+policy, func(t *testing.T) {
				fx, profile, id, _ := hermesTarget(t)
				config := filepath.Join(profile, "config.yaml")
				text := "skills:\n  external_dirs: [shared]\n  disabled: [ale32-target]\n  platform_disabled:\n    telegram: []\n"
				managed := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=telegram\nHERMES_MANAGED_DIR="+managed+"\nDENY=ale32-target\n"), 0600)
				if policy == "environment-reference" {
					if field == "global" {
						text = strings.Replace(text, "disabled: [ale32-target]", "disabled: [ale32-target, '${DENY}']", 1)
					} else {
						text = strings.Replace(text, "telegram: []", "telegram: ['${env:DENY}']", 1)
					}
				} else {
					value := "[]"
					switch policy {
					case "managed-null":
						value = "null"
					case "managed-deny":
						value = "[ale32-target]"
					}
					managedText := "skills:\n  disabled: " + value + "\n"
					if field == "selected-platform" {
						managedText = "skills:\n  platform_disabled:\n    telegram: " + value + "\n"
					}
					if policy == "managed-parent-null" {
						if field == "global" {
							managedText = "skills: null\n"
						} else {
							managedText = "skills:\n  platform_disabled: null\n"
						}
					}
					if policy == "managed-invalid" {
						managedText = "skills: [\n"
					}
					writeFile(t, filepath.Join(managed, "config.yaml"), []byte(managedText), 0600)
				}
				writeFile(t, config, []byte(text), 0600)
				before := treeManifest(t, fx.base)
				wantListExit := 0
				if policy == "managed-invalid" {
					wantListExit = 1
				}
				list := hermesList(t, fx, wantListExit, nil)
				for _, capability := range codexNamed(t, list, "ale32-target").Availability.Capabilities {
					if capability.Scope == "global" && capability.State != "unsupported" {
						t.Fatalf("unvalidated capability: %+v", capability)
					}
				}
				for _, choice := range []string{"enable", "disable"} {
					out := hermesChange(t, fx, choice, "global", id, "", 1)
					if out.State != "unsupported" || out.Confirmation != "" || out.Achievement == "achieved" {
						t.Fatalf("native interpretation mismatch admitted: %+v", out)
					}
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE32ReviewInterpolatedSelectorRefused(t *testing.T) {
	fx, profile := hermesFixture(t)
	name := "${DENY}"
	path := codexSkill(t, filepath.Join(profile, "skills"), name, name)
	id := gateSourceAt(t, hermesList(t, fx, 0, nil), filepath.Dir(path)).ID
	before := treeManifest(t, fx.base)
	for _, choice := range []string{"enable", "disable"} {
		out := hermesChange(t, fx, choice, "global", id, "", 1)
		if out.State != "unsupported" || out.Confirmation != "" {
			t.Fatalf("interpolated selector admitted: %+v", out)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE32ReviewExplicitYAMLKeysRefuseWithoutPanic(t *testing.T) {
	for _, text := range []string{"?\n skills\n:\n disabled: []\n", "skills:\n  ?\n    disabled\n  : []\n"} {
		t.Run(fmt.Sprint(len(text)), func(t *testing.T) {
			fx, profile, id, _ := hermesTarget(t)
			writeFile(t, filepath.Join(profile, "config.yaml"), []byte(text), 0600)
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, 0, nil)
			if len(list.Sources) != 1 {
				t.Fatalf("inventory lost target: %+v", list.Sources)
			}
			for _, choice := range []string{"enable", "disable"} {
				out := hermesChange(t, fx, choice, "global", id, "", 1)
				if out.State == "ready" || out.Confirmation != "" {
					t.Fatalf("unsupported key layout admitted: %+v", out)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE32ReviewUnrelatedManagedPolicyAndStaleOverride(t *testing.T) {
	fx, profile, id, _ := hermesTarget(t)
	managed := filepath.Join(fx.home, "managed")
	writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_PLATFORM=telegram\nHERMES_MANAGED_DIR="+managed+"\n"), 0600)
	managedConfig := filepath.Join(managed, "config.yaml")
	raw := []byte("skills:\n  platform_disabled:\n    discord: ['${DENY}']\nplugins:\n  disabled: []\nother: true\n")
	writeFile(t, managedConfig, raw, 0600)
	for _, choice := range []string{"disable", "enable"} {
		p := hermesChange(t, fx, choice, "global", id, "", 0)
		assertSavedChange(t, hermesChange(t, fx, choice, "global", id, p.Confirmation, 0))
		if string(readBytes(t, managedConfig)) != string(raw) {
			t.Fatal("unrelated managed policy changed")
		}
	}
	p := hermesChange(t, fx, "disable", "global", id, "", 0)
	writeFile(t, managedConfig, []byte("skills:\n  disabled: []\n"), 0600)
	before := treeManifest(t, fx.base)
	out := hermesChange(t, fx, "disable", "global", id, p.Confirmation, 1)
	if out.State == "applied" || out.Achievement == "achieved" {
		t.Fatalf("stale managed override applied: %+v", out)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE32ReviewLargeProfileInventory(t *testing.T) {
	fx, profile := hermesFixture(t)
	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("skill-%03d", i)
		codexSkill(t, filepath.Join(profile, "skills"), name, name)
	}
	before := treeManifest(t, fx.base)
	result := hermesRun(t, fx, nil, "list", "--agent", "hermes", "--project", fx.project, "--json")
	assertExit(t, result, 0)
	t.Logf("500-source inventory: %s", result.duration)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	if len(list.Sources) != 500 {
		t.Fatalf("inventory size %d", len(list.Sources))
	}
	for _, source := range list.Sources {
		for _, choice := range []string{"enable", "disable"} {
			found := false
			for _, capability := range source.Availability.Capabilities {
				if capability.Operation == choice && capability.Scope == "global" {
					found = capability.State == "supported"
				}
			}
			if !found {
				t.Fatalf("%s %s unsupported", source.Name, choice)
			}
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
