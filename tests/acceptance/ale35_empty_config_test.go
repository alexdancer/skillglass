package acceptance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func ale35EmptyConfigJourney(t *testing.T, fx fixture, env []string, state, permission, path, layerState string) {
	t.Helper()
	ale35InputJourney(t, fx, env, "safe", state, permission, "")
	before := treeManifest(t, fx.base)
	exit := 0
	if state == "unknown" {
		exit = 1
	}
	args := []string{"--agent", "opencode", "--project", fx.project, "--json"}
	result := opencodeEnvRun(t, fx, env, append([]string{"list"}, args...)...)
	var list codexList
	decodeJSON(t, result.stdout, &list)
	source := codexNamed(t, list, "safe")
	if len(list.Sources) != 1 || len(list.References) != 1 || len(source.Availability.References) != 1 || len(source.Availability.Precedence.RelatedSources) != 0 {
		t.Errorf("empty config altered physical identity: %+v", list)
	}
	found := false
	for _, layer := range list.Scope.Agent.ConfigLayers {
		if layer.Path == path {
			found = true
			if layer.State != layerState {
				t.Errorf("layer %s state=%s want %s", path, layer.State, layerState)
			}
		}
	}
	if path != "" && !found {
		t.Errorf("missing file-layer provenance %s", path)
	}
	for _, filter := range []string{"available", "disabled", "ask", "unknown"} {
		result := opencodeEnvRun(t, fx, env, append(append([]string{"list"}, args...), "--availability", filter)...)
		if result.code != exit {
			t.Errorf("filter %s exit=%d want %d", filter, result.code, exit)
		}
		var filtered codexList
		decodeJSON(t, result.stdout, &filtered)
		if filter == state {
			if len(filtered.Sources) != 1 || !reflect.DeepEqual(filtered.Sources[0], source) {
				t.Errorf("matching filter changes source explanation: %+v", filtered)
			}
		} else if len(filtered.Sources) != 0 {
			t.Errorf("nonmatching filter %s includes sources", filter)
		}
	}
	for _, operation := range []string{"list", "read", "explain"} {
		command := []string{operation, "--agent", "opencode", "--project", fx.project}
		if operation != "list" {
			command = append(command, source.ID)
		}
		result := opencodeEnvRun(t, fx, env, command...)
		if result.code != exit || !strings.Contains(string(result.stdout), "Availability: "+state) {
			t.Errorf("%s text exit/state differs: %d: %s", operation, result.code, result.stdout)
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE35EmptyConfigFiles(t *testing.T) {
	for _, route := range []string{"global-config-json", "global-opencode-json", "global-opencode-jsonc", "custom", "project-json", "project-jsonc", "home-directory-json", "home-directory-jsonc", "configured-directory-json", "configured-directory-jsonc", "managed-json", "managed-jsonc", "project-directory-json", "project-directory-jsonc", "global-exact-reload", "global-slash-reload"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			global := filepath.Join(fx.xdgConfig, "opencode")
			codexSkill(t, filepath.Join(global, "skills"), "safe", "safe")
			env := []string{}
			name := "opencode.json"
			if strings.HasSuffix(route, "jsonc") {
				name = "opencode.jsonc"
			}
			path := filepath.Join(global, name)
			switch {
			case route == "global-config-json":
				path = filepath.Join(global, "config.json")
			case route == "custom":
				path = filepath.Join(fx.base, "allowed", "custom-empty")
				env = append(env, "OPENCODE_CONFIG="+path)
			case strings.HasPrefix(route, "project-directory"):
				path = filepath.Join(fx.project, ".opencode", name)
			case strings.HasPrefix(route, "project"):
				path = filepath.Join(fx.project, name)
			case strings.HasPrefix(route, "home-directory"):
				path = filepath.Join(fx.home, ".opencode", name)
			case strings.HasPrefix(route, "configured-directory"):
				dir := filepath.Join(fx.base, "allowed", "configured")
				path = filepath.Join(dir, name)
				env = append(env, "OPENCODE_CONFIG_DIR="+dir)
			case strings.HasPrefix(route, "managed"):
				dir := filepath.Join(fx.base, "allowed", "managed")
				path = filepath.Join(dir, name)
				env = append(env, "OPENCODE_TEST_MANAGED_CONFIG_DIR="+dir)
			case strings.HasPrefix(route, "global-") && strings.HasSuffix(route, "reload"):
				dir := global
				if route == "global-slash-reload" {
					dir += "/"
				}
				env = append(env, "OPENCODE_CONFIG_DIR="+dir)
			}
			opencodeConfig(t, path, "")
			env = append(env, `OPENCODE_CONFIG_CONTENT={"permission":{"skill":"ask"}}`)
			ale35EmptyConfigJourney(t, fx, env, "ask", "ask", path, "loaded")
		})
	}
}

func TestALE35EmptyConfigPreservesEarlierInputs(t *testing.T) {
	for _, tc := range []struct{ name, config, state, permission string }{
		{"context-permission", `{"permission":{"skill":"ask"}}`, "ask", "ask"},
		{"agent-and-default", `{"default_agent":"custom","agent":{"custom":{"permission":{"skill":"ask"}}}}`, "ask", "ask"},
		{"context-tools", `{"tools":{"skill":false}}`, "disabled", "denied"},
		{"agent-tools", `{"agent":{"build":{"tools":{"skill":false}}}}`, "disabled", "denied"},
		{"earlier-invalid", `{"permission":null}`, "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			global := filepath.Join(fx.xdgConfig, "opencode")
			codexSkill(t, filepath.Join(global, "skills"), "safe", "safe")
			opencodeConfig(t, filepath.Join(global, "opencode.json"), tc.config)
			path := filepath.Join(fx.project, "opencode.json")
			opencodeConfig(t, path, "")
			opencodeConfig(t, filepath.Join(fx.project, "opencode.jsonc"), "")
			var env []string
			if tc.name == "earlier-invalid" {
				env = []string{`OPENCODE_CONFIG_CONTENT={"permission":{"skill":"allow"}}`}
			}
			ale35EmptyConfigJourney(t, fx, env, tc.state, tc.permission, path, "loaded")
		})
	}
}

func TestALE35EmptyConfigControls(t *testing.T) {
	for _, tc := range []struct{ name, content, state, permission, layer string }{
		{"absent-optional", "", "available", "allowed", "absent"},
		{"valid-object", "{}", "available", "allowed", "loaded"},
		{"whitespace-file", " \n\t", "unknown", "unknown", "unknown"},
		{"malformed-file", "{", "unknown", "unknown", "unknown"},
		{"unreadable-zero-file", "", "unknown", "unknown", "unknown"},
		{"dangling-optional", "", "unknown", "unknown", "unknown"},
		{"missing-required", "", "unknown", "unknown", "unknown"},
		{"dangling-required", "", "unknown", "unknown", "unknown"},
		{"inline-empty", "", "available", "allowed", ""},
		{"inline-whitespace", " \n\t", "unknown", "unknown", "unknown"},
		{"runtime-empty", "", "available", "allowed", ""},
		{"runtime-whitespace", " \n\t", "unknown", "unknown", "unknown"},
		{"legacy-global-zero", "", "unknown", "unknown", "unknown"},
		{"extensionless-project-zero", "", "available", "allowed", ""},
		{"plugin-retained", `{"plugin":["./inert.js"]}`, "unknown", "unknown", "loaded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			global := filepath.Join(fx.xdgConfig, "opencode")
			codexSkill(t, filepath.Join(global, "skills"), "safe", "safe")
			path := filepath.Join(global, "opencode.json")
			env := []string{}
			switch tc.name {
			case "absent-optional":
			case "missing-required", "dangling-required":
				path = filepath.Join(fx.base, "allowed", "custom")
				env = append(env, "OPENCODE_CONFIG="+path)
				if tc.name == "dangling-required" {
					if err := os.Symlink(path+"-missing", path); err != nil {
						t.Fatal(err)
					}
				}
			case "dangling-optional":
				if err := os.Symlink(path+"-missing", path); err != nil {
					t.Fatal(err)
				}
			case "inline-empty", "inline-whitespace":
				path = "OPENCODE_CONFIG_CONTENT"
				env = append(env, "OPENCODE_CONFIG_CONTENT="+tc.content)
			case "runtime-empty", "runtime-whitespace":
				path = "OPENCODE_PERMISSION"
				env = append(env, "OPENCODE_PERMISSION="+tc.content)
			case "legacy-global-zero":
				path = filepath.Join(global, "config")
				opencodeConfig(t, path, "")
				env = append(env, `OPENCODE_CONFIG_CONTENT={"permission":{"skill":"allow"}}`)
			case "extensionless-project-zero":
				opencodeConfig(t, filepath.Join(fx.project, "config"), "")
				path = ""
			default:
				opencodeConfig(t, path, tc.content)
			}
			if tc.name == "unreadable-zero-file" {
				policyPath := filepath.Join(fx.base, "allowed", "fixture.sb")
				policy, err := os.ReadFile(policyPath)
				if err != nil {
					t.Fatal(err)
				}
				policy = append(policy, []byte("\n(deny file-read* (literal "+sandboxString(path)+"))\n")...)
				writeFile(t, policyPath, policy, 0600)
			}
			if tc.name == "malformed-file" || tc.name == "whitespace-file" {
				env = append(env, `OPENCODE_CONFIG_CONTENT={"permission":{"skill":"allow"}}`)
			}
			if tc.name == "plugin-retained" {
				writeFile(t, filepath.Join(global, "inert.js"), []byte(`throw new Error("MUST_NOT_EXECUTE")`), 0600)
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), "")
				env = append(env, `OPENCODE_CONFIG_CONTENT={"plugin":[]}`)
			}
			if tc.layer == "" {
				path = ""
			}
			ale35EmptyConfigJourney(t, fx, env, tc.state, tc.permission, path, tc.layer)
		})
	}
}
