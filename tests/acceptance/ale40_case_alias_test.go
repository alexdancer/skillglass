package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE40CaseAliasAbsentDestinationRefusesBeforeWrites(t *testing.T) {
	for _, apps := range [][]string{{"claude-code", "pi"}, {"pi", "claude-code"}} {
		t.Run(strings.Join(apps, "-"), func(t *testing.T) {
			fx, id, source := ale40Fixture(t, false)
			claude := filepath.Join(fx.home, ".claude")
			variant := filepath.Join(fx.home, ".CLAUDE")
			actualInfo, err := os.Stat(claude)
			if err != nil {
				t.Fatal(err)
			}
			variantInfo, err := os.Stat(variant)
			if err != nil || !os.SameFile(actualInfo, variantInfo) {
				t.Fatalf("case-insensitive existing-parent proof required: %v", err)
			}
			pi := filepath.Join(fx.home, ".pi/agent")
			if err := os.Rename(pi, pi+"-saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(variant, pi); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(claude, "settings.json")
			if err := os.Remove(config); err != nil {
				t.Fatal(err)
			}
			piInfo, err := os.Stat(pi)
			if err != nil || !os.SameFile(actualInfo, piInfo) {
				t.Fatalf("Pi and Claude parents must be same existing object: %v", err)
			}
			for _, path := range []string{config, filepath.Join(pi, "settings.json")} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("config must be absent at %s: %v", path, err)
				}
			}
			base := "case-alias/" + strings.Join(apps, "-")
			facts, _ := json.MarshalIndent(map[string]any{
				"claude_parent": claude, "case_variant": variant, "pi_parent": pi,
				"same_parent_object": os.SameFile(actualInfo, variantInfo) && os.SameFile(actualInfo, piInfo),
				"parent_stat":        actualInfo.Sys(), "variant_stat": variantInfo.Sys(), "pi_parent_stat": piInfo.Sys(),
				"claude_destination": config, "pi_destination": filepath.Join(pi, "settings.json"),
				"destinations_absent": true, "selected_source": source,
			}, "", "  ")
			writeEvidence(t, base+"/fixture.json", facts)
			before := treeManifest(t, fx.base)
			sourceBefore := readBytes(t, source)
			args := []string{"disable", "--scope", "global", "--project", fx.project, "--json"}
			for _, app := range apps {
				args = append(args, "--agents", app)
			}
			args = append(args, id)
			result := changeCommand(t, fx, "", args...)
			writeEvidence(t, base+"/preview.json", result.stdout)
			writeEvidence(t, base+"/tree-before.json", before)
			after := treeManifest(t, fx.base)
			writeEvidence(t, base+"/tree-after.json", after)
			assertTreeUnchanged(t, before, after)
			if !bytes.Equal(sourceBefore, readBytes(t, source)) {
				t.Fatal("shared source changed")
			}
			var preview collection.GroupView
			decodeJSON(t, result.stdout, &preview)
			for _, member := range preview.Members {
				if member.Preview.State != "ready" || member.Preview.NoWriteNeeded {
					t.Fatalf("must reach aggregate alias check with actual native write: %+v", member)
				}
			}
			if preview.State != "refused" || preview.Confirmation != "" {
				t.Fatalf("same absent destination must refuse before writes: state=%q confirmation=%q reason=%q", preview.State, preview.Confirmation, preview.Reason)
			}
			assertExit(t, result, 1)
		})
	}
}

func ale40CaseParent(t *testing.T, fx fixture) (string, string) {
	t.Helper()
	parent := filepath.Join(fx.home, ".claude")
	variant := filepath.Join(fx.home, ".CLAUDE")
	actual, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := os.Stat(variant)
	if err != nil || !os.SameFile(actual, alias) {
		t.Fatalf("case-variant existing-parent proof required: %v", err)
	}
	return parent, variant
}
func ale40RedirectParent(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, from+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(to, from); err != nil {
		t.Fatal(err)
	}
	actual, err := os.Stat(to)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := os.Stat(from)
	if err != nil || !os.SameFile(actual, alias) {
		t.Fatalf("redirected parent must be the same actual object: %v", err)
	}
}
func TestALE40CaseAliasDifferentFilenamesRemainIndependent(t *testing.T) {
	for _, apps := range [][]string{{"codex", "claude-code"}, {"claude-code", "codex"}} {
		t.Run(strings.Join(apps, "-"), func(t *testing.T) {
			fx, id, source := ale40Fixture(t, false)
			parent, variant := ale40CaseParent(t, fx)
			ale40RedirectParent(t, filepath.Join(fx.home, ".codex"), variant)
			if err := os.Remove(filepath.Join(parent, "settings.json")); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"settings.json", "config.toml"} {
				if _, err := os.Stat(filepath.Join(parent, name)); !os.IsNotExist(err) {
					t.Fatalf("destination %s must be absent: %v", name, err)
				}
			}
			facts, _ := json.MarshalIndent(map[string]any{"parent": parent, "case_variant": variant, "codex_parent": filepath.Join(fx.home, ".codex"), "different_filenames": []string{"config.toml", "settings.json"}, "same_parent_object": true}, "", "  ")
			base := "case-alias-disjoint/" + strings.Join(apps, "-")
			writeEvidence(t, base+"/fixture.json", facts)
			before := treeManifest(t, fx.base)
			sourceBefore := readBytes(t, source)
			preview := ale40Preview(t, fx, apps, "global", id, 0)
			if preview.State != "ready" || preview.Confirmation == "" {
				t.Fatal("different filenames under a shared parent must compose", preview.State, preview.Reason)
			}
			for _, member := range preview.Members {
				if member.Preview.State != "ready" || member.Preview.NoWriteNeeded {
					t.Fatal("expected native writes", member.Preview)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			outcome := ale40Apply(t, fx, apps, id, preview.Confirmation, "", 0)
			for _, result := range outcome.Results {
				if result.State != "succeeded" || result.Outcome == nil || result.Outcome.Persistence != "verified" {
					t.Fatal(result)
				}
			}
			if !strings.Contains(string(readBytes(t, filepath.Join(parent, "config.toml"))), "enabled = false") || !strings.Contains(string(readBytes(t, filepath.Join(parent, "settings.json"))), `"ale40-target":"off"`) {
				t.Fatal("distinct configuration choices were not retained")
			}
			if !bytes.Equal(sourceBefore, readBytes(t, source)) {
				t.Fatal("source changed")
			}
			preview = ale40Preview(t, fx, apps, "global", id, 0)
			before = treeManifest(t, fx.base)
			for _, member := range preview.Members {
				if !member.Preview.NoWriteNeeded {
					t.Fatal("expected verified no-op", member.Preview)
				}
			}
			ale40Apply(t, fx, apps, id, preview.Confirmation, "", 0)
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
func TestALE40CaseAliasReadDependencyRespectsOrder(t *testing.T) {
	for _, apps := range [][]string{{"pi", "claude-code"}, {"claude-code", "pi"}} {
		t.Run(strings.Join(apps, "-"), func(t *testing.T) {
			fx, id, _ := ale40Fixture(t, false)
			parent, variant := ale40CaseParent(t, fx)
			ale40RedirectParent(t, filepath.Join(fx.home, ".pi/agent"), variant)
			writeFile(t, filepath.Join(parent, "settings.json"), []byte(`{"keep":true,"skillOverrides":{"ale40-target":"off"}}`), 0600)
			before := treeManifest(t, fx.base)
			want := 0
			if apps[0] == "pi" {
				want = 1
			}
			preview := ale40Preview(t, fx, apps, "global", id, want)
			for _, member := range preview.Members {
				if member.Preview.State != "ready" || member.Preview.NoWriteNeeded != (member.Application == "claude-code") {
					t.Fatal("expected one write and one original no-op read", member.Preview)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			if want == 1 {
				if preview.Confirmation != "" || !strings.Contains(preview.Reason, "preview input") {
					t.Fatal("write before aliased no-op read must refuse", preview.State, preview.Reason)
				}
			} else {
				out := ale40Apply(t, fx, apps, id, preview.Confirmation, "", 0)
				if out.Results[0].State != "succeeded" || out.Results[1].State != "succeeded" {
					t.Fatal(out.Results)
				}
				if !strings.Contains(string(readBytes(t, filepath.Join(parent, "settings.json"))), `"ale40-target":"off"`) {
					t.Fatal("later independent Pi setting lost earlier Claude choice")
				}
			}
		})
	}
}
func TestALE40CaseAliasDiscoveryContainmentRefuses(t *testing.T) {
	fx, id, _ := ale40Fixture(t, false)
	parent, variant := ale40CaseParent(t, fx)
	// Pi inspects this additional bundle, whose resource inventory includes the
	// Claude settings file. The selected source still has its ordinary native
	// .agents reference. Replacing that resource changes later discovery facts.
	writeFile(t, filepath.Join(parent, "SKILL.md"), []byte("---\nname: configuration-container\ndescription: Inert additional inspected bundle.\n---\nNo execution.\n"), 0600)
	settings, _ := json.Marshal(map[string]any{"skills": []string{variant}, "keep": true})
	writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), settings, 0600)
	before := treeManifest(t, fx.base)
	preview := ale40Preview(t, fx, []string{"claude-code", "pi"}, "global", id, 1)
	for _, member := range preview.Members {
		if member.Preview.State != "ready" || member.Preview.NoWriteNeeded {
			t.Fatal("must reach discovery containment with native write previews", member.Preview)
		}
	}
	if preview.Confirmation != "" || !strings.Contains(preview.Reason, "intersects later native discovery") {
		t.Fatal("case-variant discovery containment must refuse", preview.State, preview.Reason)
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
