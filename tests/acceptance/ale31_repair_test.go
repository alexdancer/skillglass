package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func repairHermesPlugin(t *testing.T, fx fixture, profile string, project bool) {
	t.Helper()
	plugin := filepath.Join(profile, "plugins", "portable")
	if project {
		plugin = filepath.Join(fx.project, ".hermes", "plugins", "portable")
	}
	codexSkill(t, filepath.Join(plugin, "skills"), "repair-plugin", "repair-plugin")
	writeFile(t, filepath.Join(plugin, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`), 0600)
	writeFile(t, filepath.Join(plugin, "__init__.py"), []byte("raise RuntimeError('inert plugin must never execute')\n"), 0600)
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("plugins:\n  enabled: [portable]\n"), 0600)
}

func repairHermesPublicParity(t *testing.T, fx fixture, selected codexSource, state, reason string, env []string, extra ...string) {
	t.Helper()
	if selected.Availability.State != state {
		t.Fatalf("literal state %q; want %q", selected.Availability.State, state)
	}
	args := append([]string{"explain", "--agent", "hermes", "--project", fx.project, "--json"}, extra...)
	explanation := hermesRun(t, fx, env, append(args, selected.ID)...)
	assertExit(t, explanation, 0)
	var detail struct{ Source codexSource }
	decodeJSON(t, explanation.stdout, &detail)
	if !reflect.DeepEqual(detail.Source.Availability, selected.Availability) {
		t.Fatal("explain and list facts differ")
	}
	args = append([]string{"explain", "--agent", "hermes", "--project", fx.project}, extra...)
	human := hermesRun(t, fx, env, append(args, selected.ID)...)
	assertExit(t, human, 0)
	if !strings.Contains(string(human.stdout), reason) {
		t.Fatalf("explanation lacks %q: %s", reason, human.stdout)
	}
	matching := hermesList(t, fx, 0, env, append(extra, "--availability", state)...)
	if len(matching.Sources) != 1 || matching.Sources[0].ID != selected.ID {
		t.Fatal("availability filter lost selected identity")
	}
	other := "available"
	if state == other {
		other = "unavailable"
	}
	absent := hermesList(t, fx, 0, env, append(extra, "--availability", other)...)
	if len(absent.Sources) != 0 {
		t.Fatalf("wrong availability filter retained source: %+v", absent.Sources)
	}
	for _, output := range [][]byte{explanation.stdout, explanation.stderr, human.stdout, human.stderr} {
		if strings.Contains(string(output), "INERT_CREDENTIAL_MUST_NOT_APPEAR") {
			t.Fatal("credential value exposed")
		}
	}
}

func TestALE31RepairProfileDotenvComments(t *testing.T) {
	whitespace := []struct{ name, value string }{
		{"space", " "}, {"tab", "\t"}, {"mixed-horizontal", " \t "}, {"formfeed", "\f"}, {"vertical-tab", "\v"}, {"nbsp", "\u00a0"}, {"unicode-em-space", "\u2003"}, {"python-control-space", "\x1c"},
	}
	for _, ws := range whitespace {
		for _, project := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/project=%t", ws.name, project), func(t *testing.T) {
				fx, profile := hermesFixture(t)
				repairHermesPlugin(t, fx, profile, project)
				key, state, reason := "HERMES_SAFE_MODE", "unavailable", "SAFE_MODE"
				if project {
					key, state, reason = "HERMES_ENABLE_PROJECT_PLUGINS", "explicit-only", "explicit"
				}
				writeFile(t, filepath.Join(profile, ".env"), []byte(key+"=1"+ws.value+"# inert\nOPENAI_API_KEY=INERT_CREDENTIAL_MUST_NOT_APPEAR\n"), 0600)
				before := treeManifest(t, fx.base)
				list := hermesList(t, fx, 0, nil)
				repairHermesPublicParity(t, fx, codexNamed(t, list, "repair-plugin"), state, reason, nil)
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}

func TestALE31RepairDotenvQuoteHashAndManagedControls(t *testing.T) {
	for _, test := range []struct {
		name, value, state string
		managed            bool
	}{
		{"single-quoted-comment", "'1'\t# inert", "unavailable", false},
		{"double-quoted-comment", "\"1\"\t# inert", "unavailable", false},
		{"attached-hash", "1#literal", "explicit-only", false},
		{"single-quoted-hash", "'1 # literal'", "explicit-only", false},
		{"double-quoted-hash", "\"1 # literal\"", "explicit-only", false},
		{"managed-keeps-tab-comment-literal", "1\t# inert", "explicit-only", true},
		{"managed-quoted-value", "'1'", "unavailable", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			repairHermesPlugin(t, fx, profile, false)
			path := filepath.Join(profile, ".env")
			var env []string
			if test.managed {
				managed := filepath.Join(fx.base, "allowed", "managed-policy")
				path = filepath.Join(managed, ".env")
				env = []string{"HERMES_MANAGED_DIR=" + managed}
			}
			writeFile(t, path, []byte("HERMES_SAFE_MODE="+test.value+"\n"), 0600)
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, 0, env)
			repairHermesPublicParity(t, fx, codexNamed(t, list, "repair-plugin"), test.state, path, env)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE31RepairFullReferenceExclusion(t *testing.T) {
	for _, test := range []struct {
		name, directory, state string
		configured             bool
	}{
		{"excluded-ancestor", "node_modules/skill-library", "unavailable", true},
		{"excluded-root-name", "node_modules", "unavailable", true},
		{"ordinary-root", "vendor-library/skill-library", "available", true},
		{"lookalike-component", "node_modules_backup/skill-library", "available", true},
		{"explicit-inspection", "node_modules/skill-library", "unavailable", false},
		{"org-ancestor-stays-root-relative", "_org/not-active/skill-library", "available", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx, profile := hermesFixture(t)
			external := filepath.Join(fx.project, filepath.FromSlash(test.directory))
			manifest := codexSkill(t, external, "bundle", "repair-external")
			var extra []string
			reason := "native skill index"
			if test.configured {
				writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  external_dirs: ["+external+"]\n"), 0600)
				if test.state == "unavailable" {
					reason = "native index excludes"
				}
			} else {
				extra = []string{"--root", external}
				reason = "inspection-only"
			}
			before := treeManifest(t, fx.base)
			list := hermesList(t, fx, 0, nil, extra...)
			selected := codexNamed(t, list, "repair-external")
			repairHermesPublicParity(t, fx, selected, test.state, reason, nil, extra...)
			if selected.SourcePath != filepath.Dir(manifest) {
				t.Fatal("source identity changed")
			}
			if test.configured && test.state == "unavailable" {
				if len(selected.Availability.References) != 1 || selected.Availability.References[0].Participation != "excluded" {
					t.Fatal("native exclusion fact missing")
				}
			}
			args := append([]string{"read", "--agent", "hermes", "--project", fx.project}, extra...)
			read := hermesRun(t, fx, nil, append(args, selected.ID)...)
			assertExit(t, read, 0)
			if !strings.Contains(string(read.stdout), "Inert") {
				t.Fatalf("excluded bundle unreadable: %s", read.stdout)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE31RepairExcludedAliasKeepsEligibleReference(t *testing.T) {
	fx, profile := hermesFixture(t)
	external := filepath.Join(fx.project, "node_modules", "skill-library")
	manifest := codexSkill(t, external, "bundle", "repair-shared")
	if err := os.MkdirAll(filepath.Join(profile, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(manifest), filepath.Join(profile, "skills", "eligible-alias")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  external_dirs: ["+external+"]\n"), 0600)
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, nil)
	selected := codexNamed(t, list, "repair-shared")
	if len(list.Sources) != 1 || selected.Availability.State != "available" || len(selected.Availability.References) != 2 {
		t.Fatalf("sharing/state changed: %+v", selected)
	}
	states := map[string]int{}
	for _, ref := range selected.Availability.References {
		states[ref.Participation]++
	}
	if states["eligible"] != 1 || states["excluded"] != 1 {
		t.Fatalf("reference-specific exclusions lost: %v", states)
	}
	for _, ref := range list.References {
		if ref.SourceID != selected.ID {
			t.Fatal("shared reference identity changed")
		}
	}
	read := hermesRun(t, fx, nil, "read", "--agent", "hermes", "--project", fx.project, selected.ID)
	assertExit(t, read, 0)
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE31RepairBrowserPolicyAndExclusionParity(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"policy", "exclusion"} {
		for _, width := range []int{48, 80} {
			t.Run(kind+"/"+strconv.Itoa(width), func(t *testing.T) {
				fx, profile := hermesFixture(t)
				name := "repair-external"
				if kind == "policy" {
					name = "repair-plugin"
					repairHermesPlugin(t, fx, profile, false)
					writeFile(t, filepath.Join(profile, ".env"), []byte("HERMES_SAFE_MODE=1\t# inert\n"), 0600)
				} else {
					external := filepath.Join(fx.project, "node_modules", "skill-library")
					codexSkill(t, external, "bundle", name)
					writeFile(t, filepath.Join(profile, "config.yaml"), []byte("skills:\n  external_dirs: ["+external+"]\n"), 0600)
				}
				before := treeManifest(t, fx.base)
				socket := fmt.Sprintf("ale31-repair-%s-%d-%d", kind, os.Getpid(), width)
				defer runTmuxIgnoringError(tmux, fx, socket, "kill-server")
				command := "exec " + shellQuote(skillglassBinary) + " browse --agent hermes --project " + shellQuote(fx.project) + " --availability unavailable"
				runTmux(t, tmux, fx, socket, "new-session", "-d", "-x", strconv.Itoa(width), "-y", "30", "-s", "browser", command)
				runTmux(t, tmux, fx, socket, "set-option", "-g", "status", "off")
				runTmux(t, tmux, fx, socket, "resize-window", "-t", "browser:0", "-x", strconv.Itoa(width), "-y", "30")
				record := func(label, frame string) {
					assertPaneCells(t, frame, width, 30)
					writeEvidence(t, fmt.Sprintf("tmux/ale31-repair/%s/%d/%s.txt", kind, width, label), []byte(frame))
				}
				record("01-sources", waitForPane(t, tmux, fx, socket, false, "SKILLGLASS | Sources", name, "Agent: hermes", "unavailable (inferred)"))
				send := func(key string, needles ...string) string {
					return actForPaneAt(t, tmux, fx, socket, "browser:0.0", false, func() { runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", key) }, needles...)
				}
				record("02-detail", send("Enter", "Read lines", "Availability: unavailable", "Configured permission: allowed", "Session: unknown"))
				record("03-refresh", send("r", "Refresh: selected directory object retained", "Availability: unavailable"))
				send("Escape", "SKILLGLASS | Sources")
				record("04-filter", send("a", "Availability filter: unknown", "0 matches"))
				record("05-all", send("a", "Availability filter: all", name, "unavailable (inferred)"))
				record("06-scope", send("s", "Scope and filters", "Agent: hermes", "Validated: Hermes 0.20.0"))
				runTmux(t, tmux, fx, socket, "send-keys", "-t", "browser:0.0", "q")
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			})
		}
	}
}
