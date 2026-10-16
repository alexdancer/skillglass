package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE30UnverifiedSelectorsRefused(t *testing.T) {
	for _, layout := range []string{"renamed", "shared-alias", "independent-selector"} {
		t.Run(layout, func(t *testing.T) {
			fx, _ := codexFixture(t)
			skills := filepath.Join(fx.home, ".claude/skills")
			folder := "ale30-selector"
			if layout == "renamed" {
				folder = "ordinary"
			}
			target := claudeSkill(t, skills, folder, "ale30-selector", "")
			switch layout {
			case "shared-alias":
				if err := os.Symlink(filepath.Dir(target), filepath.Join(skills, "other-alias")); err != nil {
					t.Fatal(err)
				}
			case "independent-selector":
				claudeSkill(t, filepath.Join(fx.project, ".claude/skills"), "ale30-selector", "different-command", "")
			}
			config := filepath.Join(fx.home, ".claude/settings.json")
			writeFile(t, config, []byte("{\"keep\":true}\n"), 0600)
			before := treeManifest(t, fx.base)
			source := claudeAt(t, claudeListResult(t, fx, 0), filepath.Dir(target))
			for _, capability := range source.Availability.Capabilities {
				if capability.State == "supported" {
					t.Fatalf("unsafe selector advertises control: %+v", capability)
				}
			}
			for _, scope := range []string{"global", "project"} {
				for _, choice := range []string{"enable", "disable", "inherit"} {
					if scope == "global" && choice == "inherit" {
						continue
					}
					out := claudeChange(t, fx, choice, scope, source.ID, "", 1)
					if out.State != "unsupported" || out.Confirmation != "" || !strings.Contains(out.Reason, "selector") {
						t.Fatalf("unsafe selector accepted: %+v", out)
					}
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
