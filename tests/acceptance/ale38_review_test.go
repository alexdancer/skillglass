package acceptance_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE38RecordedBuiltinCollisionsNeverWrite(t *testing.T) {
	for _, name := range []string{"skill-creator", "SKILL-CREATOR", "antigravity-support", "ANTIGRAVITY-SUPPORT"} {
		t.Run(name, func(t *testing.T) {
			fx := geminiFixture(t)
			codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), "selected", name)
			source := codexNamed(t, geminiListResult(t, fx, 0), name)
			if len(source.Availability.Capabilities) != 5 {
				t.Fatal(source.Availability.Capabilities)
			}
			for _, capability := range source.Availability.Capabilities {
				if capability.State != "unsupported" {
					t.Fatal(capability)
				}
			}
			before := treeManifest(t, fx.base)
			for _, scope := range []string{"global", "project"} {
				for _, choice := range []string{"disable", "enable", "inherit"} {
					if scope == "global" && choice == "inherit" {
						continue
					}
					for _, token := range []string{"", "unverified-confirmation"} {
						out := geminiChange(t, fx, scope, choice, source.ID, token, 1)
						if out.State != "unsupported" || out.Confirmation != "" || !strings.Contains(out.Reason, "built-in") {
							t.Fatal(out)
						}
					}
				}
			}
			r := changeCommand(t, fx, "", "disable", "--agent", "gemini", "--scope", "global", "--json", source.ID)
			assertExit(t, r, 1)
			var out changeReceipt
			decodeJSON(t, r.stdout, &out)
			if out.State != "unsupported" || !strings.Contains(out.Reason, "built-in") {
				t.Fatal(out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE38InventoryScale(t *testing.T) {
	for _, count := range []int{100, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			fx := geminiFixture(t)
			for i := 0; i < count; i++ {
				name := fmt.Sprintf("fixture-%d", i)
				codexSkill(t, filepath.Join(fx.home, ".gemini/skills"), name, name)
			}
			before := treeManifest(t, fx.base)
			r := fx.run(t, "list", "--agent", "gemini", "--project", fx.project, "--json")
			assertExit(t, r, 0)
			var list codexList
			decodeJSON(t, r.stdout, &list)
			if len(list.Sources) != count {
				t.Fatalf("sources=%d, want %d", len(list.Sources), count)
			}
			for _, source := range list.Sources {
				if len(source.Availability.Capabilities) != 5 {
					t.Fatal(source.Availability.Capabilities)
				}
				for _, capability := range source.Availability.Capabilities {
					if capability.Operation != "delete" && capability.State != "supported" {
						t.Fatal(capability)
					}
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			t.Logf("public list: sources=%d duration=%s", count, r.duration)
		})
	}
}
