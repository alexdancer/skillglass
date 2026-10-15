package acceptance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func ale35RoleJourney(t *testing.T, fx fixture, env []string, state, permission, problemPath string) codexList {
	t.Helper()
	ale35InputJourney(t, fx, env, "role-skill", state, permission, problemPath)
	before := treeManifest(t, fx.base)
	exit := 0
	if state == "unknown" {
		exit = 1
	}
	args := []string{"--agent", "opencode", "--project", fx.project}
	result := opencodeEnvRun(t, fx, env, append([]string{"list", "--json"}, args...)...)
	assertExit(t, result, exit)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	source := codexNamed(t, list, "role-skill")
	if len(list.Sources) != 1 || len(list.References) != 1 || len(source.Availability.References) != 1 || len(source.Availability.Precedence.RelatedSources) != 0 {
		t.Errorf("repeated load created false sources/references/conflicts: %+v", list)
	}
	for _, operation := range []string{"list", "read", "explain"} {
		command := append([]string{operation}, args...)
		if operation != "list" {
			command = append(command, source.ID)
		}
		text := opencodeEnvRun(t, fx, env, command...)
		assertExit(t, text, exit)
		if !strings.Contains(string(text.stdout), "Availability: "+state) {
			t.Errorf("%s text omits state %s: %s", operation, state, text.stdout)
		}
	}
	for _, filter := range []string{"available", "disabled", "ask", "unknown"} {
		filtered := opencodeEnvRun(t, fx, env, append(append([]string{"list", "--json"}, args...), "--availability", filter)...)
		assertExit(t, filtered, exit)
		var value codexList
		decodeJSON(t, filtered.stdout, &value)
		if filter == state {
			if len(value.Sources) != 1 || !reflect.DeepEqual(value.Sources[0], source) {
				t.Errorf("matching filter changes source explanation: %+v", value)
			}
		} else if len(value.Sources) != 0 {
			t.Errorf("nonmatching %s filter includes sources: %+v", filter, value.Sources)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	return list
}

func TestALE35DirectoryRoles(t *testing.T) {
	for _, tc := range []struct{ name, spelling, global, project, directory, inline, state, permission string }{
		{"absent-control", "absent", `{"permission":{"skill":"deny"}}`, `{"permission":{"skill":"allow"}}`, "", "", "available", "allowed"},
		{"exact", "exact", `{"permission":{"skill":"deny"}}`, `{"permission":{"skill":"allow"}}`, "", "", "disabled", "denied"},
		{"trailing-slash", "slash", `{"permission":{"skill":"deny"}}`, `{"permission":{"skill":"allow"}}`, "", "", "disabled", "denied"},
		{"dot-component", "dot", `{"permission":{"skill":"deny"}}`, `{"permission":{"skill":"allow"}}`, "", "", "disabled", "denied"},
		{"exact-order-control", "exact", `{"permission":{"skill":"deny"}}`, `{"permission":{"skill":"allow"}}`, `{"permission":{"skill":"allow"}}`, "", "available", "allowed"},
		{"slash-later-order", "slash", `{"permission":{"skill":"deny"}}`, `{"permission":{"skill":"allow"}}`, `{"permission":{"skill":"allow"}}`, "", "disabled", "denied"},
		{"context-tools", "exact", `{"tools":{"skill":false}}`, `{"tools":{"skill":true}}`, "", "", "disabled", "denied"},
		{"tools-wildcard-control", "exact", `{"tools":{"skill":false},"permission":{"*":"allow"}}`, `{"tools":{"skill":true}}`, "", "", "available", "allowed"},
		{"agent-permission", "exact", `{"agent":{"build":{"permission":{"skill":"deny"}}}}`, `{"agent":{"build":{"permission":{"skill":"allow"}}}}`, "", "", "disabled", "denied"},
		{"agent-tools", "slash", `{"agent":{"build":{"tools":{"skill":false}}}}`, `{"agent":{"build":{"tools":{"skill":true}}}}`, "", "", "disabled", "denied"},
		{"inline-later-control", "slash", `{"permission":{"skill":"deny"}}`, `{"permission":{"skill":"allow"}}`, "", `{"permission":{"skill":"allow"}}`, "available", "allowed"},
		{"plugin-uncertainty-control", "slash", `{"plugin":["./inert.js"],"permission":{"skill":"deny"}}`, `{"plugin":[],"permission":{"skill":"allow"}}`, "", "", "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			global := filepath.Join(fx.xdgConfig, "opencode")
			codexSkill(t, filepath.Join(global, "skills"), "one", "role-skill")
			globalPath := filepath.Join(global, "opencode.json")
			opencodeConfig(t, globalPath, tc.global)
			opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), tc.project)
			if tc.directory != "" {
				opencodeConfig(t, filepath.Join(fx.project, ".opencode", "opencode.json"), tc.directory)
			}
			if tc.name == "plugin-uncertainty-control" {
				writeFile(t, filepath.Join(global, "inert.js"), []byte("throw new Error('must not execute');\n"), 0600)
			}
			env := []string{}
			if tc.spelling != "absent" {
				dir := global
				if tc.spelling == "slash" {
					dir += "/"
				} else if tc.spelling == "dot" {
					dir += "/."
				}
				env = append(env, "OPENCODE_CONFIG_DIR="+dir)
			}
			if tc.inline != "" {
				env = append(env, "OPENCODE_CONFIG_CONTENT="+tc.inline)
			}
			problemPath := ""
			if tc.state == "unknown" {
				problemPath = globalPath
			}
			list := ale35RoleJourney(t, fx, env, tc.state, tc.permission, problemPath)
			loads := 0
			for _, layer := range list.Scope.Agent.ConfigLayers {
				if layer.Path == globalPath && layer.State == "loaded" {
					loads++
				}
			}
			want := 2
			if tc.spelling == "absent" {
				want = 1
			}
			if loads != want {
				t.Errorf("global JSON loads=%d want %d", loads, want)
			}
			if tc.state == "disabled" && codexNamed(t, list, "role-skill").Availability.References[0].Permission.Evidence.InputPath != globalPath {
				t.Error("reload lost controlling input path")
			}
			roots := 0
			for _, root := range list.Scope.Roots {
				if root.RequestedPath == filepath.Join(global, "skills") {
					roots++
				}
			}
			if roots != 1 {
				t.Errorf("physical discovery roots=%d want 1", roots)
			}
		})
	}
}

func TestALE35DirectoryPairAndLaterLayers(t *testing.T) {
	for _, tc := range []struct{ name, env, state, permission string }{
		{"pair-order", "", "ask", "ask"},
		{"managed-later", "managed", "available", "allowed"},
		{"runtime-later", "runtime", "disabled", "denied"},
		{"project-disabled", "disabled", "ask", "ask"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			global := filepath.Join(fx.xdgConfig, "opencode")
			codexSkill(t, filepath.Join(global, "skills"), "one", "role-skill")
			opencodeConfig(t, filepath.Join(global, "config.json"), `{"permission":{"skill":"deny"}}`)
			opencodeConfig(t, filepath.Join(global, "opencode.json"), `{"permission":{"skill":"allow"}}`)
			opencodeConfig(t, filepath.Join(global, "opencode.jsonc"), "{ // last of the pair\n\"permission\":{\"skill\":\"ask\"},}")
			opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":"deny"}}`)
			env := []string{"OPENCODE_CONFIG_DIR=" + global + "/"}
			if tc.env == "managed" {
				managed := filepath.Join(fx.base, "allowed", "managed")
				opencodeConfig(t, filepath.Join(managed, "opencode.json"), `{"permission":{"skill":"allow"}}`)
				env = append(env, "OPENCODE_TEST_MANAGED_CONFIG_DIR="+managed)
			} else if tc.env == "runtime" {
				env = append(env, `OPENCODE_CONFIG_CONTENT={"permission":{"skill":"allow"}}`, `OPENCODE_PERMISSION={"skill":"deny"}`)
			} else if tc.env == "disabled" {
				env = append(env, "OPENCODE_DISABLE_PROJECT_CONFIG=1")
			}
			list := ale35RoleJourney(t, fx, env, tc.state, tc.permission, "")
			var loaded []string
			for _, layer := range list.Scope.Agent.ConfigLayers {
				if layer.State == "loaded" && strings.HasPrefix(layer.Path, global+"/") {
					loaded = append(loaded, filepath.Base(layer.Path))
				}
			}
			want := []string{"config.json", "opencode.json", "opencode.jsonc", "opencode.json", "opencode.jsonc"}
			if !reflect.DeepEqual(loaded, want) {
				t.Errorf("pair reload order=%v want %v", loaded, want)
			}
			path := filepath.Join(global, "opencode.jsonc")
			if tc.env == "managed" {
				path = filepath.Join(fx.base, "allowed", "managed", "opencode.json")
			} else if tc.env == "runtime" {
				path = "OPENCODE_PERMISSION"
			}
			if codexNamed(t, list, "role-skill").Availability.References[0].Permission.Evidence.InputPath != path {
				t.Error("last layer lost controlling input path")
			}
		})
	}
	t.Run("config-json-not-reloaded", func(t *testing.T) {
		fx := newFixture(t)
		global := filepath.Join(fx.xdgConfig, "opencode")
		codexSkill(t, filepath.Join(global, "skills"), "one", "role-skill")
		opencodeConfig(t, filepath.Join(global, "config.json"), `{"permission":{"skill":"deny"}}`)
		opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":"allow"}}`)
		ale35RoleJourney(t, fx, []string{"OPENCODE_CONFIG_DIR=" + global}, "available", "allowed", "")
	})
}

func TestALE35EmptyDefaultAgent(t *testing.T) {
	for _, tc := range []struct{ name, config, state, permission string }{
		{"absent", `{}`, "available", "allowed"},
		{"empty", `{"default_agent":""}`, "available", "allowed"},
		{"empty-build-ask", `{"default_agent":"","agent":{"build":{"permission":{"skill":"ask"}}}}`, "ask", "ask"},
		{"empty-build-tools", `{"default_agent":"","agent":{"build":{"tools":{"skill":false}}}}`, "disabled", "denied"},
		{"explicit-build", `{"default_agent":"build"}`, "available", "allowed"},
		{"explicit-plan", `{"default_agent":"plan"}`, "available", "allowed"},
		{"custom-ask", `{"default_agent":"custom","agent":{"custom":{"permission":{"skill":"ask"}}}}`, "ask", "ask"},
		{"absent-hidden", `{"agent":{"build":{"hidden":true}}}`, "unknown", "unknown"},
		{"empty-hidden", `{"default_agent":"","agent":{"build":{"hidden":true}}}`, "unknown", "unknown"},
		{"absent-disabled", `{"agent":{"build":{"disable":true}}}`, "unknown", "unknown"},
		{"empty-disabled", `{"default_agent":"","agent":{"build":{"disable":true}}}`, "unknown", "unknown"},
		{"empty-subagent", `{"default_agent":"","agent":{"build":{"mode":"subagent"}}}`, "unknown", "unknown"},
		{"explicit-missing", `{"default_agent":"missing"}`, "unknown", "unknown"},
		{"whitespace", `{"default_agent":" "}`, "unknown", "unknown"},
		{"whitespace-configured", `{"default_agent":" ","agent":{" ":{"permission":{"skill":"ask"}}}}`, "ask", "ask"},
		{"invalid", `{"default_agent":false}`, "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			global := filepath.Join(fx.xdgConfig, "opencode")
			codexSkill(t, filepath.Join(global, "skills"), "one", "role-skill")
			opencodeConfig(t, filepath.Join(global, "opencode.json"), tc.config)
			ale35RoleJourney(t, fx, nil, tc.state, tc.permission, "")
		})
	}
}

func TestALE35UnverifiedDirectorySpellings(t *testing.T) {
	for _, spelling := range []string{"relative", "tilde", "symlink", "parent-component"} {
		t.Run(spelling, func(t *testing.T) {
			fx := newFixture(t)
			global := filepath.Join(fx.xdgConfig, "opencode")
			codexSkill(t, filepath.Join(global, "skills"), "one", "role-skill")
			opencodeConfig(t, filepath.Join(global, "opencode.json"), `{}`)
			dir := global
			if spelling == "relative" {
				var err error
				dir, err = filepath.Rel(fx.project, global)
				if err != nil {
					t.Fatal(err)
				}
			} else if spelling == "tilde" {
				dir = "~/configured"
				if err := os.Symlink(global, filepath.Join(fx.home, "configured")); err != nil {
					t.Fatal(err)
				}
			} else if spelling == "symlink" {
				dir = filepath.Join(fx.base, "allowed", "configured")
				if err := os.Symlink(global, dir); err != nil {
					t.Fatal(err)
				}
			} else {
				dir += "/../opencode"
			}
			ale35InputJourney(t, fx, []string{"OPENCODE_CONFIG_DIR=" + dir}, "role-skill", "unknown", "unknown", "OPENCODE_CONFIG_DIR")
		})
	}
}
