package acceptance_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func ale35InputJourney(t *testing.T, fx fixture, env []string, name, state, permission, problemPath string) {
	t.Helper()
	exit, coverage := 0, "complete"
	if state == "unknown" {
		exit, coverage = 1, "incomplete"
	}
	before := treeManifest(t, fx.base)
	ale35PreserveInputs(t, fx, name, env, before)
	args := []string{"--agent", "opencode", "--project", fx.project, "--json"}
	result := opencodeEnvRun(t, fx, env, append([]string{"list"}, args...)...)
	if result.code != exit {
		t.Errorf("list exit=%d want %d", result.code, exit)
	}
	var list codexList
	decodeJSON(t, result.stdout, &list)
	source := codexNamed(t, list, name)
	if source.Availability.State != state || list.Scope.Completeness != coverage || source.Availability.References[0].Permission.State != permission {
		t.Errorf("%s: availability=%s permission=%s coverage=%s; want %s/%s/%s", name, source.Availability.State, source.Availability.References[0].Permission.State, list.Scope.Completeness, state, permission, coverage)
	}
	if problemPath != "" {
		var detail struct{ Problems []publicProblem }
		decodeJSON(t, result.stdout, &detail)
		found := false
		for _, problem := range detail.Problems {
			found = found || problem.Code == "native-input" && problem.Path == problemPath && problem.LimitsCoverage
		}
		if !found {
			t.Errorf("missing native-input provenance %s: %+v", problemPath, detail.Problems)
		}
	}
	for _, operation := range []string{"read", "explain"} {
		result := opencodeEnvRun(t, fx, env, append(append([]string{operation}, args...), source.ID)...)
		if result.code != exit {
			t.Errorf("%s exit=%d want %d", operation, result.code, exit)
		}
		var detail struct{ Source codexSource }
		decodeJSON(t, result.stdout, &detail)
		if !reflect.DeepEqual(detail.Source.Availability, source.Availability) {
			t.Errorf("%s availability differs from list", operation)
		}
		if operation == "read" && !strings.Contains(string(result.stdout), "Inert instructions") {
			t.Error("known content no longer inspectable")
		}
	}
	after := treeManifest(t, fx.base)
	writeEvidence(t, "ordered-inputs/"+t.Name()+"/"+name+"/tree-after.json", after)
	assertTreeUnchanged(t, before, after)
}

func ale35PreserveInputs(t *testing.T, fx fixture, name string, env []string, manifest []byte) {
	t.Helper()
	if os.Getenv("SKILLGLASS_EVIDENCE") == "" {
		return
	}
	prefix := "ordered-inputs/" + t.Name() + "/" + name + "/"
	writeEvidence(t, prefix+"tree-before.json", manifest)
	links := map[string]string{}
	err := filepath.WalkDir(fx.base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(fx.base, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			links[relative], err = os.Readlink(path)
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeEvidence(t, prefix+"inputs/"+relative, raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(skillglassExecutable)
	if err != nil {
		t.Fatal(err)
	}
	profilePath := fx.policy
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := json.MarshalIndent(struct {
		Executable       string
		ExecutableSHA256 string
		Profile          string
		ProfileSHA256    string
		Environment      []string
		Links            map[string]string
	}{skillglassExecutable, fmt.Sprintf("%x", sha256.Sum256(raw)), profilePath, fmt.Sprintf("%x", sha256.Sum256(profile)), append(fx.environment(), env...), links}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, prefix+"invocation.json", receipt)

}

func TestALE35OrderedInputNumericEnumeration(t *testing.T) {
	for _, agent := range []bool{false, true} {
		label := "context"
		if agent {
			label = "agent"
		}
		t.Run(label, func(t *testing.T) {
			fx := newFixture(t)
			root := filepath.Join(fx.xdgConfig, "opencode", "skills")
			names := []string{"0", "10", "4294967294", "4294967295", "01", "-0", "1e1", "1.0", "ordinary", "+1", " 1"}
			for index, name := range names {
				quoted, _ := json.Marshal(name)
				codexSkill(t, root, string(rune('a'+index)), string(quoted))
			}
			permission := `{"skill":{"*":"deny","10":"allow","0":"allow","4294967294":"allow","4294967295":"allow","01":"allow","-0":"allow","1e1":"allow","1.0":"allow","ordinary":"allow","+1":"allow"," 1":"allow"}}`
			config := `{"permission":` + permission + `}`
			if agent {
				config = `{"agent":{"build":{"permission":` + permission + `}}}`
			}
			opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), config)
			for index, name := range names {
				state, permission := "available", "allowed"
				if index < 3 {
					state, permission = "disabled", "denied"
				}
				ale35InputJourney(t, fx, nil, name, state, permission, "")
			}
		})
	}
}

func TestALE35OrderedInputToolsAndOverwrites(t *testing.T) {
	for _, tc := range []struct{ name, global, project, state, permission string }{
		{"seed-before-wildcard", `{"tools":{"skill":false},"permission":{"*":"allow","skill":"ask"}}`, `{}`, "available", "allowed"},
		{"reversed-explicit", `{"tools":{"skill":false},"permission":{"skill":"ask","*":"allow"}}`, `{}`, "available", "allowed"},
		{"no-tools-control", `{"permission":{"*":"allow","skill":"ask"}}`, `{}`, "ask", "ask"},
		{"wildcard-tool", `{"tools":{"s*":false}}`, `{}`, "disabled", "denied"},
		{"context-layer-overwrite", `{"tools":{"skill":false},"permission":{"*":"allow","skill":"deny"}}`, `{"permission":{"skill":"ask"}}`, "available", "allowed"},
		{"agent-layer-control", `{"agent":{"build":{"permission":{"*":"allow"}}}}`, `{"agent":{"build":{"tools":{"skill":false}}}}`, "disabled", "denied"},
		{"ordinary-overwrite", `{"permission":{"skill":{"safe":"allow","*":"deny"}}}`, `{"permission":{"skill":{"safe":"ask"}}}`, "disabled", "denied"},
		{"ordinary-insertion-control", `{"permission":{"skill":{"*":"deny","safe":"allow"}}}`, `{"permission":{"skill":{"safe":"ask"}}}`, "ask", "ask"},
		{"agent-wildcard-tool", `{"agent":{"build":{"tools":{"s*":false}}}}`, `{}`, "disabled", "denied"},
		{"agent-ordinary-overwrite", `{"agent":{"build":{"permission":{"skill":{"safe":"allow","*":"deny"}}}}}`, `{"agent":{"build":{"permission":{"skill":{"safe":"ask"}}}}}`, "disabled", "denied"},
		{"agent-ordinary-insertion-control", `{"agent":{"build":{"permission":{"skill":{"*":"deny","safe":"allow"}}}}}`, `{"agent":{"build":{"permission":{"skill":{"safe":"ask"}}}}}`, "ask", "ask"},
		{"tool-rule-provenance", `{"tools":{"skill":false}}`, `{}`, "disabled", "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), tc.global)
			opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), tc.project)
			ale35InputJourney(t, fx, nil, "safe", tc.state, tc.permission, "")
			list := opencodeListResult(t, fx, 0)
			selected := codexNamed(t, list, "safe").Availability.References[0].Permission
			expectedPath := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
			if tc.name == "agent-layer-control" || tc.name == "ordinary-insertion-control" || tc.name == "agent-ordinary-insertion-control" {
				expectedPath = filepath.Join(fx.project, "opencode.json")
			}
			if selected.Evidence.InputPath != expectedPath {
				t.Errorf("controlling input=%s want %s", selected.Evidence.InputPath, expectedPath)
			}
			human := fx.run(t, "explain", "--agent", "opencode", "--project", fx.project, codexNamed(t, list, "safe").ID)
			assertExit(t, human, 0)
			if tc.name == "tool-rule-provenance" && !strings.Contains(string(human.stdout), "tools.skill") {
				t.Error("legacy tool rule lost setting provenance")
			}
			if tc.name == "seed-before-wildcard" && !strings.Contains(string(human.stdout), "permission.*") {
				t.Error("explicit rule mislabeled as tools")
			}
		})
	}
}

func TestALE35OrderedInputPluginRegistration(t *testing.T) {
	for _, route := range []string{"project-empty", "custom-empty", "inline-empty", "managed-empty", "native-directory-empty", "global-replaced-control", "all-empty-control", "nonempty", "pure-control", "custom-registration", "native-registration", "inline-registration", "managed-registration"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			codexSkill(t, filepath.Join(fx.xdgCache, "opencode", "skills"), "cached", "cached")
			global := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
			opencodeConfig(t, global, `{"plugin":["./hooks.js"]}`)
			writeFile(t, filepath.Join(fx.xdgConfig, "opencode", "hooks.js"), []byte(`throw new Error("NOT_EXECUTED")`), 0600)
			env := []string{}
			declaring := global
			switch route {
			case "custom-registration", "native-registration", "inline-registration", "managed-registration":
				opencodeConfig(t, global, `{"plugin":[]}`)
				switch route {
				case "custom-registration":
					declaring = filepath.Join(fx.base, "allowed", "custom.json")
					opencodeConfig(t, declaring, `{"plugin":["./hooks.js"]}`)
					opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"plugin":[]}`)
					env = append(env, "OPENCODE_CONFIG="+declaring)
				case "native-registration":
					declaring = filepath.Join(fx.project, ".opencode", "opencode.json")
					opencodeConfig(t, declaring, `{"plugin":["./hooks.js"]}`)
					env = append(env, `OPENCODE_CONFIG_CONTENT={"plugin":[]}`)
				case "inline-registration":
					declaring = "OPENCODE_CONFIG_CONTENT"
					env = append(env, `OPENCODE_CONFIG_CONTENT={"plugin":["./hooks.js"]}`)
					managed := filepath.Join(fx.base, "allowed", "managed")
					opencodeConfig(t, filepath.Join(managed, "opencode.json"), `{"plugin":[]}`)
					env = append(env, "OPENCODE_TEST_MANAGED_CONFIG_DIR="+managed)
				case "managed-registration":
					managed := filepath.Join(fx.base, "allowed", "managed")
					declaring = filepath.Join(managed, "opencode.json")
					opencodeConfig(t, declaring, `{"plugin":["./hooks.js"]}`)
					opencodeConfig(t, filepath.Join(managed, "opencode.jsonc"), `{"plugin":[]}`)
					env = append(env, "OPENCODE_TEST_MANAGED_CONFIG_DIR="+managed)
				}
			case "project-empty", "pure-control":
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"plugin":[]}`)
				if route == "pure-control" {
					env = append(env, "OPENCODE_PURE=true")
				}
			case "custom-empty":
				path := filepath.Join(fx.base, "allowed", "custom.json")
				opencodeConfig(t, path, `{"plugin":[]}`)
				env = append(env, "OPENCODE_CONFIG="+path)
			case "inline-empty":
				env = append(env, `OPENCODE_CONFIG_CONTENT={"plugin":[]}`)
			case "managed-empty":
				dir := filepath.Join(fx.base, "allowed", "managed")
				opencodeConfig(t, filepath.Join(dir, "opencode.json"), `{"plugin":[]}`)
				env = append(env, "OPENCODE_TEST_MANAGED_CONFIG_DIR="+dir)
			case "native-directory-empty":
				opencodeConfig(t, filepath.Join(fx.project, ".opencode", "opencode.json"), `{"plugin":[]}`)
			case "global-replaced-control":
				opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "config.json"), `{"plugin":["./hooks.js"]}`)
				opencodeConfig(t, global, `{"plugin":[]}`)
			case "all-empty-control":
				opencodeConfig(t, global, `{"plugin":[]}`)
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"plugin":[]}`)
			}
			state, permission, path, cache := "unknown", "unknown", declaring, "unknown"
			if strings.HasSuffix(route, "control") {
				state, permission, path, cache = "available", "allowed", "", "unavailable"
			}
			ale35InputJourney(t, fx, env, "safe", state, permission, path)
			result := opencodeEnvRun(t, fx, env, "list", "--agent", "opencode", "--project", fx.project, "--json")
			var list codexList
			decodeJSON(t, result.stdout, &list)
			if got := codexNamed(t, list, "cached").Availability.State; got != cache {
				t.Errorf("cache=%s want %s", got, cache)
			}
		})
	}
}

func TestALE35OrderedInputMarkdownBoundary(t *testing.T) {
	for _, route := range []string{"renamed", "recursive", "dot-recursive", "direct-mode", "custom-agent", "plan", "build", "broken", "metadata-denied", "directory-denied", "symlink-file", "symlink-directory", "cycle", "fifo", "depth-exhaustion", "global-directory", "home-directory", "custom-directory", "pure-markdown", "direct-mode-singular", "non-md-control", "nested-mode-control"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			dir := filepath.Join(fx.project, ".opencode")
			env := []string{}
			switch route {
			case "global-directory":
				dir = filepath.Join(fx.xdgConfig, "opencode")
			case "home-directory":
				dir = filepath.Join(fx.home, ".opencode")
			case "custom-directory":
				dir = filepath.Join(fx.base, "allowed", "custom")
				env = append(env, "OPENCODE_CONFIG_DIR="+dir)
			case "pure-markdown":
				env = append(env, "OPENCODE_PURE=true")
			}
			path := filepath.Join(dir, "agents", "override.md")
			content := "---\nname: build\npermission:\n  skill: deny\n---\nInert prompt\n"
			switch route {
			case "recursive":
				path = filepath.Join(dir, "agent", "nested", "override.md")
				content = "---\nname: build\ntools:\n  skill: false\n---\nInert prompt\n"
			case "dot-recursive":
				path = filepath.Join(dir, "agents", ".nested", "override.md")
			case "direct-mode":
				path = filepath.Join(dir, "modes", "override.md")
			case "direct-mode-singular":
				path = filepath.Join(dir, "mode", "override.md")
			case "custom-agent":
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"default_agent":"custom","agent":{"custom":{}}}`)
				content = "---\nname: custom\npermission:\n  skill: deny\n---\nInert prompt\n"
			case "plan":
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"default_agent":"plan"}`)
			case "build":
				path = filepath.Join(dir, "agent", "build.md")
			case "depth-exhaustion":
				path = filepath.Join(dir, "agents", strings.Repeat("nested/", 67), "override.md")
			case "non-md-control":
				path = filepath.Join(dir, "agents", "override.txt")
			case "nested-mode-control":
				path = filepath.Join(dir, "modes", "nested", "override.md")
			}
			writeFile(t, path, []byte(content), 0600)
			if strings.HasPrefix(route, "symlink") || route == "broken" || route == "metadata-denied" || route == "fifo" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(fx.base, "allowed", "target.md")
				if route == "metadata-denied" {
					target = filepath.Join(fx.base, "allowed", "private", "target.md")
				}
				if route == "broken" {
					target = filepath.Join(fx.base, "allowed", "absent.md")
				} else if route != "fifo" {
					writeFile(t, target, []byte(content), 0600)
				}
				if route == "symlink-directory" {
					path = filepath.Dir(path)
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					target = filepath.Dir(target) + "/target-dir"
					writeFile(t, filepath.Join(target, "override.md"), []byte(content), 0600)
				}
				if route == "fifo" {
					if err := syscall.Mkfifo(path, 0600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			if route == "cycle" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(filepath.Dir(path), "loop")
				if err := os.Symlink(filepath.Dir(path), path); err != nil {
					t.Fatal(err)
				}
			}
			if route == "directory-denied" || route == "metadata-denied" {
				denied := filepath.Dir(path)
				if route == "metadata-denied" {
					denied = filepath.Join(fx.base, "allowed", "private")
				}
				policy := string(mustReadFixturePolicy(t, fx)) + "\n(deny file-read* (subpath " + sandboxString(denied) + "))\n"
				writeFile(t, fx.policy, []byte(policy), 0600)
				writeEvidence(t, "markdown/"+route+"-injected-EPERM.sb", []byte(policy))
				if route == "directory-denied" {
					path = denied
				}
			}
			state, permission := "unknown", "unknown"
			if route == "symlink-directory" {
				path = filepath.Join(path, "override.md")
			}
			if route == "depth-exhaustion" {
				path = filepath.Join(dir, "agents", strings.Repeat("nested/", 65))
			}
			if strings.HasSuffix(route, "control") {
				state, permission, path = "available", "allowed", ""
			}
			ale35InputJourney(t, fx, env, "safe", state, permission, path)
			if route == "metadata-denied" || route == "directory-denied" {
				result := opencodeEnvRun(t, fx, env, "list", "--agent", "opencode", "--project", fx.project, "--json")
				if !strings.Contains(string(result.stdout), "operation not permitted") {
					t.Error("injected metadata EPERM not observed")
				}
			}
		})
	}
}
