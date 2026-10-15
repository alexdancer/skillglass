package acceptance_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReviewHermesEffectivePluginFlags(t *testing.T) {
	for _, tc := range []struct {
		name, profileEnv, managedEnv, safe, projectFlag, want string
		project, disabled                                     bool
	}{
		{name: "quoted-safe", profileEnv: "'HERMES_SAFE_MODE'=0\n", safe: "1", want: "unknown"},
		{name: "quoted-project", profileEnv: "'HERMES_ENABLE_PROJECT_PLUGINS'=1\n", project: true, want: "unknown"},
		{name: "interpolated-safe", profileEnv: "HERMES_SAFE_MODE=${UNOBSERVED}\n", safe: "1", want: "unknown"},
		{name: "interpolated-project", profileEnv: "HERMES_ENABLE_PROJECT_PLUGINS=${UNOBSERVED}\n", project: true, want: "unknown"},
		{name: "safe-inherited", safe: "1", want: "unavailable"},
		{name: "project-inherited", project: true, projectFlag: "1", want: "explicit-only"},
		{name: "safe-assigned", profileEnv: "HERMES_SAFE_MODE=0\n", safe: "1", want: "explicit-only"},
		{name: "project-assigned", profileEnv: "HERMES_ENABLE_PROJECT_PLUGINS=1\n", project: true, want: "explicit-only"},
		{name: "safe-restored", profileEnv: "'HERMES_SAFE_MODE'=0\nHERMES_SAFE_MODE=0\n", safe: "1", want: "explicit-only"},
		{name: "managed-restores-safe", profileEnv: "'HERMES_SAFE_MODE'=0\n", managedEnv: "HERMES_SAFE_MODE=0\n", safe: "1", want: "explicit-only"},
		{name: "unrelated-safe-exclusion", profileEnv: "'HERMES_ENABLE_PROJECT_PLUGINS'=1\n", project: true, safe: "1", want: "unavailable"},
		{name: "unrelated-project-exclusion", profileEnv: "'HERMES_SAFE_MODE'=0\n", project: true, want: "unavailable"},
		{name: "disabled-exclusion", profileEnv: "'HERMES_SAFE_MODE'=0\n", safe: "1", disabled: true, want: "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			root := p
			if tc.project {
				root = filepath.Join(fx.project, ".hermes")
			}
			dir := d2Plugin(t, fx, root, "portable")
			cfg := "plugins:\n  enabled: [portable]\n"
			if tc.disabled {
				cfg += "  disabled: [portable]\n"
			}
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(cfg), 0600)
			writeFile(t, filepath.Join(p, ".env"), []byte(tc.profileEnv), 0600)
			env := []string{"HERMES_SAFE_MODE=" + tc.safe, "HERMES_ENABLE_PROJECT_PLUGINS=" + tc.projectFlag}
			if tc.managedEnv != "" {
				managed := filepath.Join(fx.home, "managed")
				writeFile(t, filepath.Join(managed, ".env"), []byte(tc.managedEnv), 0600)
				env = append(env, "HERMES_MANAGED_DIR="+managed)
			}
			defer g2Seal(t, fx)()
			list := d2List(t, fx, env)
			g2Inspect(t, fx, env, list, filepath.Join(dir, "SKILL.md"), tc.want)
		})
	}
}

func TestReviewHermesImplicitScalarFields(t *testing.T) {
	for _, token := range []string{"0o0", "+0o0", "0O0", "08", "1e3", "1.0e3", ".0e0"} {
		for _, field := range []string{"name", "description", "platforms", "environments", "requires_tools", "requires_toolsets", "fallback_for_tools", "fallback_for_toolsets"} {
			t.Run(token+"/"+field, func(t *testing.T) {
				fx, p := hermesFixture(t)
				file := codexSkill(t, filepath.Join(p, "skills"), "probe", "probe")
				text := g2Text("probe", "")
				if field == "name" || field == "description" {
					old := "name: probe"
					if field == "description" {
						old = "description: Inert."
					}
					text = strings.Replace(text, old, field+": "+token, 1)
				} else {
					extra := field + ": " + token + "\n"
					if strings.Contains(field, "tools") {
						extra = "metadata:\n  hermes:\n    " + field + ": " + token + "\n"
					}
					text = g2Text("probe", extra)
				}
				writeFile(t, file, []byte(text), 0600)
				defer g2Seal(t, fx)()
				g2Inspect(t, fx, nil, d2List(t, fx, nil), file, "unknown")
			})
		}
	}
	for _, tc := range []struct{ token, want string }{{"!!int 0o0", "available"}, {"'0o0'", "unavailable"}, {"!!str 0o0", "unavailable"}, {"0", "available"}, {"0.0", "available"}, {"0.0e+0", "available"}} {
		t.Run("platform-control/"+tc.token, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := codexSkill(t, filepath.Join(p, "skills"), "probe", "probe")
			writeFile(t, file, []byte(g2Text("probe", "platforms: "+tc.token+"\n")), 0600)
			defer g2Seal(t, fx)()
			g2Inspect(t, fx, nil, d2List(t, fx, nil), file, tc.want)
		})
	}
	for _, field := range []string{"platforms", "environments", "requires_tools", "requires_toolsets", "fallback_for_tools", "fallback_for_toolsets"} {
		t.Run("full-content/"+field, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := codexSkill(t, filepath.Join(p, "skills"), "probe", "probe")
			extra := field + ": 0o0\n"
			if strings.Contains(field, "tools") {
				extra = "metadata:\n  hermes:\n    " + field + ": 0o0\n"
			}
			text := g2Text("probe", "padding: "+strings.Repeat("x", 4100)+"\n"+extra)
			writeFile(t, file, []byte(text), 0600)
			defer g2Seal(t, fx)()
			g2Inspect(t, fx, nil, d2List(t, fx, nil), file, "unknown")
		})
	}
	for _, field := range []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"} {
		t.Run("portable/"+field, func(t *testing.T) {
			fx, p := hermesFixture(t)
			dir := d2Plugin(t, fx, p, "portable")
			writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
			text := g2Text("explicit", "")
			if field == "name" {
				text = strings.Replace(text, "name: explicit", "name: 0o0", 1)
			} else if field == "description" {
				text = strings.Replace(text, "description: Inert.", "description: 0o0", 1)
			} else {
				value := "0o0"
				if field == "metadata" {
					value = "{key: 0o0}"
				}
				text = g2Text("explicit", field+": "+value+"\n")
			}
			file := filepath.Join(dir, "SKILL.md")
			writeFile(t, file, []byte(text), 0600)
			defer g2Seal(t, fx)()
			g2Inspect(t, fx, nil, d2List(t, fx, nil), file, "unknown")
		})
	}
}

func TestReviewHermesImplicitPolicyKeys(t *testing.T) {
	for _, tc := range []struct{ name, config, want string }{
		{"implicit-platform-key", "skills:\n  platform_disabled: {0o0: [probe]}\n", "unknown"},
		{"quoted-platform-key", "skills:\n  platform_disabled: {'0o0': [probe]}\n", "disabled"},
		{"implicit-overrides-merge", "skills:\n  platform_disabled:\n    <<: {'0o0': [probe]}\n    0o0: []\n", "unknown"},
		{"mixed-key-merge-unknown", "skills:\n  platform_disabled:\n    <<: {0o0: [probe]}\n    '0o0': []\n", "unknown"},
		{"disabled-value", "skills:\n  disabled: [0o0]\n", "unknown"},
		{"platform-value", "skills:\n  platform_disabled: {'0o0': [0o0]}\n", "unknown"},
		{"external-dirs", "skills:\n  external_dirs: [0o0]\n", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, p := hermesFixture(t)
			file := codexSkill(t, filepath.Join(p, "skills"), "probe", "probe")
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(tc.config), 0600)
			env := []string{"HERMES_PLATFORM=0o0"}
			defer g2Seal(t, fx)()
			g2Inspect(t, fx, env, d2List(t, fx, env), file, tc.want)
		})
	}
	for _, field := range []string{"enabled", "disabled"} {
		t.Run("plugins-"+field, func(t *testing.T) {
			fx, p := hermesFixture(t)
			dir := d2Plugin(t, fx, p, "portable")
			cfg := "plugins:\n  " + field + ": [0o0]\n"
			if field == "disabled" {
				cfg += "  enabled: [portable]\n"
			}
			writeFile(t, filepath.Join(p, "config.yaml"), []byte(cfg), 0600)
			defer g2Seal(t, fx)()
			g2Inspect(t, fx, nil, d2List(t, fx, nil), filepath.Join(dir, "SKILL.md"), "unknown")
		})
	}
}

func TestReviewHermesNativePluginScalarConfidence(t *testing.T) {
	for _, field := range []string{"name", "version"} {
		for _, quoted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", field, quoted), func(t *testing.T) {
				fx, p := hermesFixture(t)
				root := filepath.Join(p, "plugins", "category", "entry")
				token := "0o0"
				if quoted {
					token = "'0o0'"
				}
				manifest := "name: native\nversion: stable\n"
				original := field + ": native"
				if field == "version" {
					original = field + ": stable"
				}
				manifest = strings.Replace(manifest, original, field+": "+token, 1)
				writeFile(t, filepath.Join(root, "plugin.yaml"), []byte(manifest), 0600)
				file := codexSkill(t, filepath.Join(root, "skills"), "probe", "probe")
				writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: [category/entry]\n"), 0600)
				defer g2Seal(t, fx)()
				list := d2List(t, fx, nil)
				g2Inspect(t, fx, nil, list, file, "unknown")
				source := gateSourceAt(t, list, filepath.Dir(file))
				if field == "name" && (source.Availability.NativeNameEvidence.Kind == "unknown") == quoted {
					t.Errorf("native name confidence: %+v", source.Availability.NativeNameEvidence)
				}
				if field == "version" {
					found := false
					for _, ref := range source.Availability.References {
						if ref.Provenance.Kind == "plugin" {
							found = true
							want := ""
							if quoted {
								want = "0o0"
							}
							if ref.Provenance.Version != want || (ref.Provenance.Evidence.Kind == "unknown") == quoted {
								t.Errorf("native version confidence: %+v", ref.Provenance)
							}
						}
					}
					if !found {
						t.Fatal("plugin provenance absent")
					}
				}
			})
		}
	}
}

func TestReviewHermesUncertainProjectAdmission(t *testing.T) {
	fx, p := hermesFixture(t)
	user := d2Plugin(t, fx, p, "portable")
	project := d2Plugin(t, fx, filepath.Join(fx.project, ".hermes"), "portable")
	writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
	writeFile(t, filepath.Join(p, ".env"), []byte("'HERMES_ENABLE_PROJECT_PLUGINS'=1\n"), 0600)
	defer g2Seal(t, fx)()
	list := d2List(t, fx, nil)
	for _, dir := range []string{user, project} {
		g2Inspect(t, fx, nil, list, filepath.Join(dir, "SKILL.md"), "unknown")
	}
}

func TestReviewHermesConfidenceBrowser(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{48, 80} {
		for _, mode := range []string{"safe", "project", "scalar"} {
			t.Run(fmt.Sprintf("%s/%d", mode, width), func(t *testing.T) {
				fx, p := hermesFixture(t)
				env := []string{}
				name := "explicit"
				if mode == "scalar" {
					name = "probe"
					file := codexSkill(t, filepath.Join(p, "skills"), name, name)
					writeFile(t, file, []byte(g2Text(name, "platforms: 0o0\n")), 0600)
				} else {
					root := p
					key, value := "HERMES_SAFE_MODE", "0"
					if mode == "project" {
						root = filepath.Join(fx.project, ".hermes")
						key, value = "HERMES_ENABLE_PROJECT_PLUGINS", "1"
					} else {
						env = append(env, "HERMES_SAFE_MODE=1")
					}
					d2Plugin(t, fx, root, "portable")
					writeFile(t, filepath.Join(p, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
					writeFile(t, filepath.Join(p, ".env"), []byte("'"+key+"'="+value+"\n"), 0600)
				}
				defer g2Seal(t, fx)()
				socket := "review-hermes"
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := fx.productCommand(t, "browse", "--agent", "hermes", "--project", fx.project)
				command.Env = append(fx.environment(), env...)
				runTmuxProduct(t, tmux, fx, socket, command, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser")
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				waitForPane(t, tmux, fx, socket, false, "Focused: "+name)
				frame := actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "Enter", "d") }, "Availability: unknown", "Read lines")
				assertPaneCells(t, frame, width, 30)
				writeEvidence(t, fmt.Sprintf("review-hermes/%s-%d.txt", mode, width), []byte(frame))
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
			})
		}
	}
}
