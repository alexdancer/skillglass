package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALE35InheritedAgentProfiles(t *testing.T) {
	for _, tc := range []struct{ name, config, state, permission string }{
		{"explore-primary", `{"default_agent":"explore","agent":{"explore":{"mode":"primary"}}}`, "disabled", "denied"},
		{"explore-inherited-subagent", `{"default_agent":"explore","agent":{"explore":{}}}`, "unknown", "unknown"},
		{"general-inherited-subagent", `{"default_agent":"general","agent":{"general":{}}}`, "unknown", "unknown"},
		{"general-primary", `{"default_agent":"general","agent":{"general":{"mode":"primary"}}}`, "available", "allowed"},
		{"compaction-visible", `{"default_agent":"compaction","agent":{"compaction":{"hidden":false}}}`, "disabled", "denied"},
		{"title-visible", `{"default_agent":"title","agent":{"title":{"hidden":false}}}`, "disabled", "denied"},
		{"summary-visible", `{"default_agent":"summary","agent":{"summary":{"hidden":false}}}`, "disabled", "denied"},
		{"compaction-inherited-hidden", `{"default_agent":"compaction","agent":{"compaction":{}}}`, "unknown", "unknown"},
		{"title-inherited-hidden", `{"default_agent":"title"}`, "unknown", "unknown"},
		{"summary-inherited-hidden", `{"default_agent":"summary","agent":{"summary":{}}}`, "unknown", "unknown"},
		{"context-allow-explore", `{"default_agent":"explore","permission":{"skill":"allow"},"agent":{"explore":{"mode":"primary"}}}`, "available", "allowed"},
		{"context-ask-explore", `{"default_agent":"explore","permission":{"skill":"ask"},"agent":{"explore":{"mode":"primary"}}}`, "ask", "ask"},
		{"agent-ask-explore", `{"default_agent":"explore","permission":{"skill":"allow"},"agent":{"explore":{"mode":"all","permission":{"skill":"ask"}}}}`, "ask", "ask"},
		{"agent-allow-title", `{"default_agent":"title","permission":{"skill":"deny"},"agent":{"title":{"hidden":false,"permission":{"skill":"allow"}}}}`, "available", "allowed"},
		{"hidden-overrides-allow", `{"default_agent":"title","permission":{"skill":"allow"}}`, "unknown", "unknown"},
		{"subagent-overrides-allow", `{"default_agent":"explore","permission":{"skill":"allow"}}`, "unknown", "unknown"},
		{"disabled-explore", `{"default_agent":"explore","agent":{"explore":{"mode":"primary","disable":true}}}`, "unknown", "unknown"},
		{"disabled-implicit-build", `{"agent":{"build":{"disable":true}}}`, "unknown", "unknown"},
		{"hidden-implicit-build", `{"agent":{"build":{"hidden":true}}}`, "unknown", "unknown"},
		{"subagent-implicit-build", `{"agent":{"build":{"mode":"subagent"}}}`, "unknown", "unknown"},
		{"custom-all-default", `{"default_agent":"custom","agent":{"custom":{}}}`, "available", "allowed"},
		{"custom-missing", `{"default_agent":"custom"}`, "unknown", "unknown"},
		{"custom-explicit-allow", `{"default_agent":"custom","permission":{"skill":"deny"},"agent":{"custom":{"permission":{"skill":"allow"}}}}`, "available", "allowed"},
		{"plan-control", `{"default_agent":"plan"}`, "available", "allowed"},
		{"build-control", `{}`, "available", "allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
			opencodeConfig(t, path, tc.config)
			problem := ""
			if tc.state == "unknown" {
				problem = path
			}
			ale35InputJourney(t, fx, nil, "safe", tc.state, tc.permission, problem)
		})
	}
}

func TestALE35ActionOnlyPermissionFields(t *testing.T) {
	for _, field := range []string{"todowrite", "question", "webfetch", "websearch", "doom_loop"} {
		for _, route := range []string{"context", "selected-agent", "unselected-agent", "later-context", "later-selected-agent", "later-unselected-agent"} {
			t.Run(field+"/"+route, func(t *testing.T) {
				fx := newFixture(t)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
				permission := `{"skill":"allow","` + field + `":{"*":"allow"}}`
				config := `{"permission":` + permission + `}`
				if route == "selected-agent" || route == "later-selected-agent" {
					config = `{"agent":{"build":{"permission":` + permission + `}}}`
				}
				if route == "unselected-agent" || route == "later-unselected-agent" {
					config = `{"agent":{"unused":{"permission":` + permission + `}}}`
				}
				path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
				opencodeConfig(t, path, config)
				switch route {
				case "later-context":
					opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"`+field+`":"allow"}}`)
				case "later-selected-agent":
					opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"agent":{"build":{"permission":{"`+field+`":"allow"}}}}`)
				case "later-unselected-agent":
					opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"agent":{"unused":{"permission":{"`+field+`":"allow"}}}}`)
				}
				ale35InputJourney(t, fx, nil, "safe", "unknown", "unknown", path)
			})
		}
		for _, action := range []string{"allow", "ask", "deny"} {
			t.Run(field+"/scalar-"+action, func(t *testing.T) {
				fx := newFixture(t)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
				permission := `{"skill":"allow","` + field + `":"` + action + `"}`
				opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"permission":`+permission+`,"agent":{"build":{"permission":`+permission+`},"unused":{"permission":`+permission+`}}}`)
				ale35InputJourney(t, fx, nil, "safe", "available", "allowed", "")
			})
		}
	}
	for _, field := range []string{"read", "edit", "glob", "grep", "list", "bash", "task", "external_directory", "lsp", "skill", "arbitrary_tool", "qu?stion"} {
		t.Run("rule-control/"+field, func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
			permission := `{"` + field + `":{"question":"allow","*":"allow"}}`
			opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"permission":`+permission+`,"agent":{"build":{"permission":`+permission+`},"unused":{"permission":`+permission+`}}}`)
			ale35InputJourney(t, fx, nil, "safe", "available", "allowed", "")
		})
	}
}

func TestALE35FlatPermissionPatterns(t *testing.T) {
	for _, permission := range []string{`{"question":{}}`, `{"arbitrary_tool":{"*":{}}}`, `{"skill":{"safe":{"*":"allow"}}}`} {
		for _, agent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", permission, agent), func(t *testing.T) {
				fx := newFixture(t)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "safe", "safe")
				config := `{"permission":` + permission + `}`
				if agent {
					config = `{"agent":{"build":{"permission":` + permission + `}}}`
				}
				path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
				opencodeConfig(t, path, config)
				ale35InputJourney(t, fx, nil, "safe", "unknown", "unknown", path)
			})
		}
	}
}

func TestALE35UTF16WildcardOracle(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot, "tests", "acceptance", "testdata", "opencode-wildcard.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Value         string
		PatternJSON   string `json:"pattern_json"`
		ScalarPattern bool   `json:"scalar_pattern"`
		Matched       bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("empty wildcard oracle corpus")
	}
	for index, tc := range cases {
		for _, route := range []string{"context", "agent", "tool"} {
			if route == "tool" && tc.Value != "skill" {
				continue
			}
			t.Run(fmt.Sprintf("%02d/%s", index, route), func(t *testing.T) {
				fx := newFixture(t)
				name, _ := json.Marshal(tc.Value)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "source", string(name))
				permission := `{"*":"deny","skill":{` + tc.PatternJSON + `:"allow"}}`
				if route == "tool" {
					permission = `{"*":"deny",` + tc.PatternJSON + `:"allow"}`
				}
				config := `{"permission":` + permission + `}`
				if route == "agent" {
					config = `{"agent":{"build":{"permission":` + permission + `}}}`
				}
				path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
				opencodeConfig(t, path, config)
				state, permissionState, problem := "disabled", "denied", ""
				if tc.Matched {
					state, permissionState = "available", "allowed"
				}
				if !tc.ScalarPattern {
					state, permissionState, problem = "unknown", "unknown", path
				}
				ale35InputJourney(t, fx, nil, tc.Value, state, permissionState, problem)
			})
		}
	}
}

func TestALE35WildcardSurrogateInputs(t *testing.T) {
	for _, tc := range []struct{ name, value, pattern, state, permission string }{
		{"paired-escape", "🔍", `"\ud83d\udd0d"`, "available", "allowed"},
		{"literal-replacement", "�x", `"�?"`, "available", "allowed"},
		{"escaped-replacement", "�x", `"\ufffd?"`, "available", "allowed"},
		{"escaped-backslash", "literal/ud83dx", `"literal\\ud83d?"`, "available", "allowed"},
		{"unpaired-high", "🔍", `"\ud83d?"`, "unknown", "unknown"},
		{"unpaired-low", "🔍", `"?\udd0d"`, "unknown", "unknown"},
		{"high-before-bmp", "🔍", `"\ud83d\u0061"`, "unknown", "unknown"},
	} {
		for _, agent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", tc.name, agent), func(t *testing.T) {
				fx := newFixture(t)
				name, _ := json.Marshal(tc.value)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "source", string(name))
				permission := `{"*":"deny","skill":{` + tc.pattern + `:"allow"}}`
				config := `{"permission":` + permission + `}`
				if agent {
					config = `{"agent":{"build":{"permission":` + permission + `}}}`
				}
				path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
				opencodeConfig(t, path, config)
				problem := ""
				if tc.state == "unknown" {
					problem = path
				}
				ale35InputJourney(t, fx, nil, tc.value, tc.state, tc.permission, problem)
				if tc.state == "unknown" {
					result := fx.run(t, "list", "--agent", "opencode", "--project", fx.project, "--json")
					if !strings.Contains(string(result.stdout), "unpaired UTF-16 surrogate") {
						t.Error("unsupported Unicode reason missing")
					}
				}
			})
		}
	}
}

func TestALE35UnsupportedUnicodeLayers(t *testing.T) {
	for _, route := range []string{"value", "comment-control", "later-project", "custom", "inline", "managed", "runtime"} {
		t.Run(route, func(t *testing.T) {
			fx := newFixture(t)
			codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "source", `"🔍"`)
			path := filepath.Join(fx.xdgConfig, "opencode", "opencode.json")
			permission := `{"skill":{"*":"deny","\ud83d?":"allow"}}`
			config := `{"permission":` + permission + `}`
			env := []string{}
			switch route {
			case "value":
				config = `{"agent":{"build":{"description":"\ud83d"}}}`
			case "comment-control":
				config = "{ // \\ud83d\n\"permission\":{\"skill\":\"allow\"}}"
			case "later-project":
				opencodeConfig(t, filepath.Join(fx.project, "opencode.json"), `{"permission":{"skill":"allow"}}`)
			case "custom":
				path = filepath.Join(fx.base, "allowed", "custom.json")
				env = append(env, "OPENCODE_CONFIG="+path)
			case "inline":
				path = "OPENCODE_CONFIG_CONTENT"
				env = append(env, path+"="+config)
			case "managed":
				dir := filepath.Join(fx.base, "allowed", "managed")
				path = filepath.Join(dir, "opencode.json")
				env = append(env, "OPENCODE_TEST_MANAGED_CONFIG_DIR="+dir)
			case "runtime":
				path = "OPENCODE_PERMISSION"
				env = append(env, path+"="+permission)
			}
			if route != "inline" && route != "runtime" {
				opencodeConfig(t, path, config)
			}
			state, permissionState, problem := "unknown", "unknown", path
			if route == "comment-control" {
				state, permissionState, problem = "available", "allowed", ""
			}
			ale35InputJourney(t, fx, env, "🔍", state, permissionState, problem)
		})
	}
}

func TestALE35WildcardHomeExpansion(t *testing.T) {
	for _, pattern := range []string{"~", "~/🔍", "$HOME/🔍"} {
		for _, agent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", pattern, agent), func(t *testing.T) {
				fx := newFixture(t)
				value := fx.home
				if pattern != "~" {
					value += "/🔍"
				}
				name, _ := json.Marshal(value)
				codexSkill(t, filepath.Join(fx.xdgConfig, "opencode", "skills"), "source", string(name))
				key, _ := json.Marshal(pattern)
				permission := `{"*":"deny","skill":{` + string(key) + `:"allow"}}`
				config := `{"permission":` + permission + `}`
				if agent {
					config = `{"agent":{"build":{"permission":` + permission + `}}}`
				}
				opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), config)
				ale35InputJourney(t, fx, nil, value, "available", "allowed", "")
			})
		}
	}
}
