package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE36ReviewCaseAliasDestinations(t *testing.T) {
	for _, role := range []string{"native-directory", "global", "project", "skill", "cache", "managed", "custom-directory"} {
		for _, format := range []string{"missing", "json", "jsonc"} {
			t.Run(role+"/"+format, func(t *testing.T) {
				fx, _, id := opencodeTarget(t)
				dir, alias := filepath.Join(fx.home, ".opencode"), filepath.Join(fx.home, ".OpenCode")
				scope := "project"
				var env []string
				switch role {
				case "global", "project", "custom-directory":
					dir, alias = filepath.Join(fx.xdgConfig, "opencode"), filepath.Join(fx.xdgConfig, "OpenCode")
					if role != "global" {
						scope = "global"
					}
					if role == "custom-directory" {
						env = []string{"OPENCODE_CONFIG_DIR=" + alias}
					}
				case "skill":
					dir = filepath.Join(fx.xdgConfig, "opencode/skills/target")
					alias = filepath.Join(fx.xdgConfig, "opencode/skills/TARGET")
				case "cache":
					dir, alias = filepath.Join(fx.xdgCache, "opencode"), filepath.Join(fx.xdgCache, "OpenCode")
				case "managed":
					dir, alias = filepath.Join(fx.base, "allowed/managed"), filepath.Join(fx.base, "allowed/Managed")
					env = []string{"OPENCODE_TEST_MANAGED_CONFIG_DIR=" + dir}
				}
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				actual, err := os.Stat(dir)
				if err != nil {
					t.Fatal(err)
				}
				other, err := os.Stat(alias)
				if os.IsNotExist(err) {
					t.Skip("fixture volume is case-sensitive")
				}
				if err != nil || !os.SameFile(actual, other) {
					t.Fatalf("case alias identity: %v", err)
				}
				if format != "missing" {
					opencodeConfig(t, filepath.Join(dir, "opencode."+format), `{"keep":1}`)
				}
				if role != "custom-directory" {
					fx.project = alias
				}
				before := treeManifest(t, fx.base)
				out := opencodeEnvRun(t, fx, env, "disable", "--agent", "opencode", "--scope", scope, "--project", fx.project, "--json", id)
				assertExit(t, out, 1)
				var receipt changeReceipt
				decodeJSON(t, out.stdout, &receipt)
				if receipt.State != "refused" {
					t.Fatal(receipt)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE36ReviewGlobalResultIndependentOfProject(t *testing.T) {
	for _, choice := range []string{"disable", "enable", "enable-ask"} {
		t.Run(choice, func(t *testing.T) {
			fx, _, id := opencodeTarget(t)
			globalAction, projectAction := "allow", "deny"
			if choice != "disable" {
				globalAction, projectAction = "deny", "allow"
			}
			if choice == "enable-ask" {
				projectAction = "ask"
				choice = "enable"
			}
			opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode/opencode.json"), `{"permission":{"skill":{"target":"`+globalAction+`","*":"`+globalAction+`"}}}`)
			opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":{"*":"`+projectAction+`"}}}`)
			before := treeManifest(t, fx.base)
			out := opencodeChange(t, fx, "global", choice, id, "", 1)
			if out.State != "refused" || !strings.Contains(out.Reason, "global") {
				t.Fatal(out)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
