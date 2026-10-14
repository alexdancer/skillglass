package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE29ClaudeSharedNameUsesEligibleReferences(t *testing.T) {
	for _, mode := range []string{"off", "deny", "precedence", "project-only", "explicit-name", "explicit-name-off", "competing-names"} {
		t.Run(mode, func(t *testing.T) {
			fx, _ := codexFixture(t)
			name := ""
			if strings.HasPrefix(mode, "explicit-name") {
				name = "action"
			}
			bundle := filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed", "shared"), "bundle", name, ""))
			personal := filepath.Join(fx.home, ".claude", "skills")
			project := filepath.Join(fx.project, ".claude", "skills")
			aliases := []string{filepath.Join(project, "work")}
			if mode != "project-only" {
				aliases = append(aliases, filepath.Join(personal, "synced"))
			}
			if mode == "competing-names" {
				aliases = append(aliases, filepath.Join(project, "other"))
			}
			for _, alias := range aliases {
				if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(bundle, alias); err != nil {
					t.Fatal(err)
				}
			}
			settings := map[string]any{"skillOverrides": map[string]string{"work": "off"}}
			state, nativeName, rule := "disabled", "work", "skill-visibility-override"
			independent := ""
			switch mode {
			case "deny":
				settings = map[string]any{"permissions": map[string]any{"deny": []string{"Skill(work)"}}}
				state, rule = "explicit-only", "skill-tool-deny"
			case "precedence":
				settings = map[string]any{}
				independent = filepath.Dir(claudeSkill(t, personal, "independent", "work", ""))
				state, rule = "shadowed", "startup-name-precedence"
			case "explicit-name":
				state, nativeName, rule = "available", "action", "invocation-frontmatter"
			case "explicit-name-off":
				settings = map[string]any{"skillOverrides": map[string]string{"action": "off"}}
				nativeName = "action"
			case "competing-names":
				state, nativeName, rule = "unknown", "", "shared-native-name"
			}
			claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), settings)
			before := treeManifest(t, fx.base)
			list := claudeListResult(t, fx, 0)
			source := claudeAt(t, list, bundle)
			detail := fx.run(t, "explain", "--agent", "claude-code", "--project", fx.project, source.ID)
			assertExit(t, detail, 0)
			filtered := claudeListResult(t, fx, 0, "--availability", "unknown")
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if len(source.Availability.References) != len(aliases) {
				t.Errorf("shared references lost: %+v", source.Availability)
			}
			if source.Availability.State != state || source.Availability.Evidence.Rule != rule || (nativeName != "" && source.Availability.NativeName != nativeName) {
				t.Errorf("eligible name policy: got %+v, want %s/%s/%s", source.Availability, state, nativeName, rule)
			}
			if !strings.Contains(string(detail.stdout), "Availability: "+state) || !strings.Contains(string(detail.stdout), rule) {
				t.Errorf("public explanation contradicts eligible name policy:\n%s", detail.stdout)
			}
			if state == "unknown" {
				if len(filtered.Sources) != 1 || filtered.Sources[0].ID != source.ID {
					t.Errorf("unknown filter: %+v", filtered.Sources)
				}
			} else if len(filtered.Sources) != 0 {
				t.Errorf("unknown filter: %+v", filtered.Sources)
			}
			for _, fact := range source.Availability.References {
				want := "eligible"
				if fact.Evidence.Rule == "reserved-skill-name" {
					want = "excluded"
				}
				if fact.Participation != want {
					t.Errorf("reference eligibility changed: %+v", fact)
				}
			}
			if mode == "competing-names" {
				if source.Availability.NativeNameEvidence.Kind != "unknown" || source.Availability.Precedence.State != "unknown" || source.Availability.Invocation.State != "unknown" {
					t.Errorf("competing eligible names gained confidence: %+v", source.Availability)
				}
				if source.Availability.NativeName != "work" && source.Availability.NativeName != "other" {
					t.Errorf("excluded name used instead of competing eligible names: %+v", source.Availability)
				}
			}
			if independent != "" {
				winner := claudeAt(t, list, independent)
				if winner.ID == source.ID || winner.Availability.State != "available" || source.Availability.Precedence.Winner != winner.ID {
					t.Errorf("independent source precedence: %+v %+v", source, winner)
				}
			}
		})
	}
}

func TestALE29ClaudeExcludedPluginKeepsUnknownNativeAlias(t *testing.T) {
	for _, mode := range []string{"native-alias", "native-and-viewer-alias", "viewer-only", "all-excluded", "eligible-plugin"} {
		t.Run(mode, func(t *testing.T) {
			fx, _ := codexFixture(t)
			plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
			state, rule := "unknown", "managed-alias-unverified"
			if mode != "eligible-plugin" {
				claudeSettings(t, filepath.Join(fx.home, ".claude", "settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}})
			}
			aliasRoots := []string{}
			if mode == "native-alias" || mode == "native-and-viewer-alias" || mode == "eligible-plugin" {
				aliasRoots = append(aliasRoots, filepath.Join(fx.home, ".claude", "skills"))
			}
			args := []string{}
			if mode == "viewer-only" || mode == "native-and-viewer-alias" {
				viewer := filepath.Join(fx.base, "allowed", "inspection")
				aliasRoots = append(aliasRoots, viewer)
				args = append(args, "--root", viewer)
			}
			for _, root := range aliasRoots {
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(plugin, filepath.Join(root, "alias")); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "viewer-only" || mode == "all-excluded" {
				state, rule = "unavailable", "excluded-native-reference"
			} else if mode == "eligible-plugin" {
				state, rule = "available", "invocation-frontmatter"
			}
			before := treeManifest(t, fx.base)
			list := claudeListResult(t, fx, 0, args...)
			source := claudeAt(t, list, plugin)
			detailArgs := append([]string{"explain", "--agent", "claude-code", "--project", fx.project}, args...)
			detail := fx.run(t, append(detailArgs, source.ID)...)
			assertExit(t, detail, 0)
			filtered := claudeListResult(t, fx, 0, append(args, "--availability", "unknown")...)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if source.Availability.State != state || source.Availability.Evidence.Rule != rule {
				t.Errorf("unresolved native alias suppressed: %+v, want %s/%s", source.Availability, state, rule)
			}
			if !strings.Contains(string(detail.stdout), "Availability: "+state) || !strings.Contains(string(detail.stdout), rule) {
				t.Errorf("public explanation contradicts native routes:\n%s", detail.stdout)
			}
			if state == "unknown" {
				if len(filtered.Sources) != 1 || filtered.Sources[0].ID != source.ID {
					t.Errorf("unknown filter dropped unresolved native route: %+v", filtered.Sources)
				}
			} else if len(filtered.Sources) != 0 {
				t.Errorf("unknown filter: %+v", filtered.Sources)
			}
			unknownNative := 0
			for _, fact := range source.Availability.References {
				if fact.Participation == "unknown" && fact.Evidence.Rule == "managed-alias-unverified" {
					unknownNative++
				}
			}
			wantUnknown := 0
			if mode == "native-alias" || mode == "native-and-viewer-alias" || mode == "eligible-plugin" {
				wantUnknown = 1
			}
			if unknownNative != wantUnknown {
				t.Errorf("native versus inspection-only routes changed: %+v", source.Availability.References)
			}
			if source.Ownership.State != "managed" || len(source.Availability.References) != len(aliasRoots)+1 {
				t.Errorf("managed provenance or sharing lost: %+v", source)
			}
		})
	}
}
