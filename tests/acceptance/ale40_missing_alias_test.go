package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexdancer/skillglass/internal/collection"
)

func ale40EnvironmentChange(t *testing.T, fx fixture, environment []string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	argv := fx.launchArgs(t, skillglassBinary, true, "", args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Env = append(fx.environment(), environment...)
	command.Dir = fx.project
	result, err := executeFinalizedCommand(t, command)
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	recordEvidenceCommand(t, "ale40-configured-mutation", result)
	assertRecordedContext(t, result.launch, command)
	return result
}

func ale40MissingParentFixture(t *testing.T) (fixture, string, string) {
	t.Helper()
	fx, id, source := ale40Fixture(t, false)
	root := filepath.Join(fx.project, ".claude/skills")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(source), filepath.Join(root, "ale40-target")); err != nil {
		t.Fatal(err)
	}
	return fx, id, source
}

func ale40ProveComponentAlias(t *testing.T, fx fixture, first, second, evidence string) {
	t.Helper()
	probe := filepath.Join(fx.home, "alias-prerequisite")
	if err := os.Mkdir(probe, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(probe, first), 0700); err != nil {
		t.Fatal(err)
	}
	actual, err := os.Stat(filepath.Join(probe, first))
	if err != nil {
		t.Fatal(err)
	}
	variant, aliasErr := os.Stat(filepath.Join(probe, second))
	available := aliasErr == nil && os.SameFile(actual, variant)
	if aliasErr != nil && !os.IsNotExist(aliasErr) {
		t.Fatal(aliasErr)
	}
	facts := map[string]any{"first": first, "second": second, "same_object": available, "first_stat": actual.Sys()}
	if variant != nil {
		facts["second_stat"] = variant.Sys()
	}
	raw, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, evidence+"/filesystem-prerequisite.json", raw)
	if err := os.RemoveAll(probe); err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Skipf("missing-component alias coverage unavailable on fixture volume for %q and %q", first, second)
	}
}

func TestALE40MissingParentNamePrecision(t *testing.T) {
	for _, scenario := range []struct {
		name, second, secondApp string
		refuse                  bool
	}{
		{"case-uncertainty", "CONTROL", "codex", true},
		{"normalization-uncertainty", "contro\u0302le", "codex", true},
		{"ascii-disjoint", "control-other", "pi", false},
		{"prefix-disjoint", "control-other", "codex", false},
	} {
		for _, reverse := range []bool{false, true} {
			apps := []string{"claude-code", scenario.secondApp}
			if reverse {
				apps[0], apps[1] = apps[1], apps[0]
			}
			t.Run(scenario.name+"/"+strings.Join(apps, "-"), func(t *testing.T) {
				fx, id, source := ale40MissingParentFixture(t)
				first := "control"
				if scenario.name == "normalization-uncertainty" {
					first = "contr\u00f4le"
				}
				claude, second := filepath.Join(fx.home, first), filepath.Join(fx.home, scenario.second)
				key := "CODEX_HOME="
				if scenario.secondApp == "pi" {
					key = "PI_CODING_AGENT_DIR="
				}
				environment := []string{"CLAUDE_CONFIG_DIR=" + claude, key + second}
				for _, parent := range []string{claude, second} {
					if _, err := os.Stat(parent); !os.IsNotExist(err) {
						t.Fatalf("configuration parent must be absent: %s: %v", parent, err)
					}
				}
				before, sourceBefore := treeManifest(t, fx.base), readBytes(t, source)
				args := []string{"disable", "--scope", "global", "--project", fx.project, "--json"}
				for _, app := range apps {
					args = append(args, "--agents", app)
				}
				args = append(args, id)
				result := ale40EnvironmentChange(t, fx, environment, args...)
				base := "missing-parent-precision/" + scenario.name + "/" + strings.Join(apps, "-")
				writeEvidence(t, base+"/preview.json", result.stdout)
				var preview collection.GroupView
				decodeJSON(t, result.stdout, &preview)
				if len(preview.Members) != 2 {
					t.Fatal(preview)
				}
				for _, member := range preview.Members {
					if member.Preview.State != "ready" || member.Preview.NoWriteNeeded || member.Preview.CreateDirectory == "" {
						t.Fatal("must reach shared classifier with native writes", member)
					}
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if scenario.refuse {
					assertExit(t, result, 1)
					if preview.State != "refused" || preview.Confirmation != "" || !strings.Contains(preview.Reason, "preview input") {
						t.Fatal("uncertain parent creation with distinct filenames must refuse", preview.State, preview.Reason)
					}
				} else {
					assertExit(t, result, 0)
					if preview.State != "ready" || preview.Confirmation == "" {
						t.Fatal("ASCII-disjoint missing parents must remain independent", preview.State, preview.Reason)
					}
					args = append(args[:len(args)-1], "--confirm", preview.Confirmation, id)
					result = ale40EnvironmentChange(t, fx, environment, args...)
					writeEvidence(t, base+"/result.json", result.stdout)
					assertExit(t, result, 0)
					var outcome collection.GroupOutcome
					decodeJSON(t, result.stdout, &outcome)
					if outcome.State != "applied" || len(outcome.Results) != 2 {
						t.Fatal(outcome)
					}
					for i, applied := range outcome.Results {
						if applied.Application != apps[i] || applied.State != "succeeded" || applied.Outcome == nil || applied.Outcome.Persistence != "verified" || applied.Outcome.Achievement != "achieved" {
							t.Fatal(applied)
						}
					}
				}
				if !bytes.Equal(sourceBefore, readBytes(t, source)) {
					t.Fatal("source changed")
				}
			})
		}
	}
}

func TestALE40MissingParentAliasRefusesBeforeWrites(t *testing.T) {
	for _, names := range [][2]string{{"control", "CONTROL"}, {"contr\u00f4le", "contro\u0302le"}} {
		for _, apps := range [][]string{{"claude-code", "pi"}, {"pi", "claude-code"}} {
			t.Run(names[0]+"/"+strings.Join(apps, "-"), func(t *testing.T) {
				fx, id, source := ale40MissingParentFixture(t)
				base := "missing-parent-alias/" + names[0] + "/" + strings.Join(apps, "-")
				ale40ProveComponentAlias(t, fx, names[0], names[1], base)
				claude, pi := filepath.Join(fx.home, names[0]), filepath.Join(fx.home, names[1])
				environment := []string{"CLAUDE_CONFIG_DIR=" + claude, "PI_CODING_AGENT_DIR=" + pi}
				for _, parent := range []string{claude, pi} {
					if _, err := os.Stat(parent); !os.IsNotExist(err) {
						t.Fatalf("configuration parent must remain absent at preview: %s: %v", parent, err)
					}
				}
				before, sourceBefore := treeManifest(t, fx.base), readBytes(t, source)
				writeEvidence(t, base+"/tree-before.json", before)
				writeEvidence(t, base+"/source.md", sourceBefore)
				if os.Getenv("SKILLGLASS_EVIDENCE") != "" {
					raw, err := exec.Command("/usr/bin/tar", "-cf", "-", "-C", fx.base, ".").Output()
					if err != nil {
						t.Fatal(err)
					}
					writeEvidence(t, base+"/fixture.tar", raw)
				}
				for _, app := range apps {
					result := ale40EnvironmentChange(t, fx, environment, "disable", "--agent", app, "--scope", "global", "--project", fx.project, "--json", id)
					writeEvidence(t, base+"/singleton-"+app+".json", result.stdout)
					assertExit(t, result, 0)
					var native collection.ChangeView
					decodeJSON(t, result.stdout, &native)
					if native.State != "ready" || native.NoWriteNeeded || native.CreateDirectory == "" || native.SourcePath != filepath.Dir(source) {
						t.Fatal("independent native preview must support missing-parent creation", native)
					}
				}
				args := []string{"disable", "--scope", "global", "--project", fx.project, "--json"}
				for _, app := range apps {
					args = append(args, "--agents", app)
				}
				args = append(args, id)
				result := ale40EnvironmentChange(t, fx, environment, args...)
				writeEvidence(t, base+"/preview.json", result.stdout)
				var preview collection.GroupView
				decodeJSON(t, result.stdout, &preview)
				if len(preview.Members) != 2 || preview.Source == nil || string(preview.Source.ID) != id || preview.Source.Identity.Certainty != "proven" {
					t.Fatal("exact selected source or native members missing", preview)
				}
				for i, member := range preview.Members {
					parent := claude
					if member.Application == "pi" {
						parent = pi
					}
					if member.Application != apps[i] || member.Preview.State != "ready" || member.Preview.NoWriteNeeded || member.Preview.CreateDirectory != parent || member.Preview.ConfigPath != filepath.Join(parent, "settings.json") || member.Preview.SourcePath != filepath.Dir(source) {
						t.Fatalf("must reach aggregate check with independently writable missing parents for exact source: %+v", member)
					}
				}
				for _, app := range apps {
					found := false
					for _, consumer := range preview.Consumers {
						if consumer.Application != app {
							continue
						}
						for _, ref := range consumer.References {
							path := filepath.Dir(source)
							if app == "claude-code" {
								path = filepath.Join(fx.project, ".claude/skills/ale40-target")
							}
							if string(ref.Source) == id && ref.ResolvedPath == filepath.Dir(source) && ref.DiscoveredPath == path {
								found = true
							}
						}
					}
					if !found {
						t.Fatal("missing exact-source native consumer", app)
					}
				}
				after := treeManifest(t, fx.base)
				writeEvidence(t, base+"/tree-after-preview.json", after)
				assertTreeUnchanged(t, before, after)
				if !bytes.Equal(sourceBefore, readBytes(t, source)) {
					t.Fatal("source changed")
				}
				if preview.State != "refused" || preview.Confirmation != "" {
					t.Fatalf("missing configuration parent aliases must refuse before consent: state=%q token=%q reason=%q", preview.State, preview.Confirmation, preview.Reason)
				}
				assertExit(t, result, 1)
				args = append(args[:len(args)-1], "--confirm", "g1:forged", id)
				result = ale40EnvironmentChange(t, fx, environment, args...)
				writeEvidence(t, base+"/forged-result.json", result.stdout)
				assertExit(t, result, 1)
				var outcome collection.GroupOutcome
				decodeJSON(t, result.stdout, &outcome)
				if outcome.State != "refused" || outcome.View.Confirmation != "" || len(outcome.Results) != 2 {
					t.Fatal(outcome)
				}
				for i, result := range outcome.Results {
					if result.Application != apps[i] || result.State != "not-attempted" || result.Outcome != nil {
						t.Fatal(result)
					}
				}
				after = treeManifest(t, fx.base)
				writeEvidence(t, base+"/tree-after-forged.json", after)
				assertTreeUnchanged(t, before, after)
				if !bytes.Equal(sourceBefore, readBytes(t, source)) {
					t.Fatal("source changed")
				}
			})
		}
	}
}
