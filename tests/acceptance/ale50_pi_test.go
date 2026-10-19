package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/alexdancer/skillglass/internal/collection"
	"os"
	"path/filepath"
	"testing"
)

func ale50PiSource(t *testing.T, fx fixture, path string, exit int) codexSource {
	t.Helper()
	for _, source := range piList(t, fx, exit).Sources {
		if source.SourcePath == filepath.Dir(path) {
			return source
		}
	}
	t.Fatal("source absent", path)
	return codexSource{}
}

func TestALE50PiUnrelatedDiscoveryControls(t *testing.T) {
	for _, scenario := range []string{"ignore", "leaf-ignore", "broken", "missing-package", "inactive-cache"} {
		t.Run(scenario, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.home, ".pi/agent/skills")
			target := codexSkill(t, root, "target", "target")
			sibling := codexSkill(t, root, "sibling", "sibling")
			config := filepath.Join(fx.home, ".pi/agent/settings.json")
			switch scenario {
			case "ignore":
				writeFile(t, filepath.Join(root, ".gitignore"), []byte("unrelated/\n"), 0600)
			case "leaf-ignore":
				writeFile(t, filepath.Join(filepath.Dir(sibling), ".gitignore"), []byte("__pycache__/\n*.py[cod]\n.DS_Store\n"), 0600)
			case "broken":
				if err := os.Symlink(filepath.Join(fx.home, "absent"), filepath.Join(root, "broken")); err != nil {
					t.Fatal(err)
				}
			case "missing-package":
				pkg := filepath.Join(fx.home, ".pi/agent/npm/node_modules/pkg")
				writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"name":"fixture","version":"1.0.0","pi":{"skills":["skills"]}}`), 0600)
				raw, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "sentinel": 42, "packages": []string{pkg}})
				writeFile(t, config, raw, 0600)
			case "inactive-cache":
				dir := filepath.Join(fx.home, ".pi/agent/npm/node_modules/pi-subagents/src/runs")
				for i := 0; i < 10010; i++ {
					writeFile(t, filepath.Join(dir, fmt.Sprintf("run-%05d", i)), nil, 0600)
				}
			}
			exit := 0
			if scenario == "broken" || scenario == "missing-package" || scenario == "inactive-cache" {
				exit = 1
			}
			selected := ale50PiSource(t, fx, target, exit)
			if selected.Availability.State != "available" {
				t.Fatal(selected.Availability)
			}
			before := readBytes(t, sibling)
			preview := piChange(t, fx, "global", "disable", selected.ID, "", 0)
			if preview.State != "ready" {
				t.Fatal(preview)
			}
			out := piChange(t, fx, "global", "disable", selected.ID, preview.Confirmation, 0)
			assertSavedChange(t, out)
			if ale50PiSource(t, fx, target, exit).Availability.State != "disabled" {
				t.Fatal("target not disabled")
			}
			if !bytes.Equal(before, readBytes(t, sibling)) {
				t.Fatal("unrelated content changed")
			}
			piApply(t, fx, "global", "enable", selected.ID)
		})
	}
}

func TestALE50PiIgnoreFreshnessAndTargetUncertainty(t *testing.T) {
	fx := piFixture(t)
	root := filepath.Join(fx.home, ".pi/agent/skills")
	target := codexSkill(t, root, "target", "target")
	id := ale50PiSource(t, fx, target, 0).ID
	preview := piChange(t, fx, "global", "disable", id, "", 0)
	ignore := filepath.Join(root, ".gitignore")
	writeFile(t, ignore, []byte("unrelated/\n"), 0600)
	before := readBytes(t, filepath.Join(fx.home, ".pi/agent/settings.json"))
	out := piChange(t, fx, "global", "disable", id, preview.Confirmation, 1)
	if out.State == "applied" {
		t.Fatal("new ignore input accepted stale approval")
	}
	if !bytes.Equal(before, readBytes(t, filepath.Join(fx.home, ".pi/agent/settings.json"))) {
		t.Fatal("stale wrote settings")
	}
	writeFile(t, ignore, []byte("[!t]*\n"), 0600)
	if got := ale50PiSource(t, fx, target, 1).Availability.State; got != "unknown" {
		t.Fatal(got)
	}
	if got := piChange(t, fx, "global", "disable", id, "", 1).State; got == "ready" {
		t.Fatal("target uncertainty accepted")
	}
}

func TestALE50PiSymlinkedIgnoreContentFreshness(t *testing.T) {
	fx := piFixture(t)
	root := filepath.Join(fx.home, ".pi/agent/skills")
	target := codexSkill(t, root, "target", "target")
	evidence := filepath.Join(fx.home, "rules.ignore")
	writeFile(t, evidence, []byte("unrelated/\n"), 0600)
	if err := os.Symlink(evidence, filepath.Join(root, ".ignore")); err != nil {
		t.Fatal(err)
	}
	id := ale50PiSource(t, fx, target, 0).ID
	preview := piChange(t, fx, "global", "disable", id, "", 0)
	writeFile(t, evidence, []byte("different-unrelated/\n"), 0600)
	before := treeManifest(t, fx.base)
	out := piChange(t, fx, "global", "disable", id, preview.Confirmation, 1)
	if out.State == "applied" {
		t.Fatal("changed symlink target content accepted")
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE50PiOrderedReviewWithWarnings(t *testing.T) {
	for _, mutate := range []string{"none", "same-result-ignore", "broken-alias", "package-manifest"} {
		t.Run(mutate, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.home, ".pi/agent/skills")
			manifests := []string{codexSkill(t, root, "first", "first"), codexSkill(t, root, "second", "second")}
			broken := filepath.Join(root, "broken")
			if err := os.Symlink(filepath.Join(fx.home, "absent"), broken); err != nil {
				t.Fatal(err)
			}
			ignore := filepath.Join(root, ".gitignore")
			writeFile(t, ignore, []byte("unrelated/\n"), 0600)
			pkg := filepath.Join(fx.home, ".pi/agent/npm/node_modules/empty")
			if mutate == "package-manifest" {
				writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"version":"1.0.0","pi":{"skills":["skills"]}}`), 0600)
				writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), mustJSON(t, map[string]any{"defaultProjectTrust": "always", "packages": []string{pkg}}), 0600)
			}
			draft := ale46Draft([]string{"pi"}, collection.ControlGlobal, "")
			for i, path := range manifests {
				id := ale50PiSource(t, fx, path, 1).ID
				draft.Sources = append(draft.Sources, ale46Assess(id))
				draft.Changes = append(draft.Changes, ale46Change(fmt.Sprintf("disable-%d", i), id, "pi", collection.ControlDisable))
			}
			selection, path := ale47Approved(t, fx, draft, []string{"disable-0", "disable-1"})
			if selection.Proposal.Coverage.Discovery != collection.Incomplete {
				t.Fatal("warning lost")
			}
			config := filepath.Join(fx.home, ".pi/agent/settings.json")
			before := readBytes(t, config)
			want := 0
			switch mutate {
			case "package-manifest":
				writeFile(t, filepath.Join(pkg, "package.json"), []byte(`{"version":"1.0.1","pi":{"skills":["skills"]}}`), 0600)
				want = 1
			case "same-result-ignore":
				writeFile(t, ignore, []byte("different-unrelated/\n"), 0600)
				want = 1
			case "broken-alias":
				if err := os.Remove(broken); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(manifests[0]), broken); err != nil {
					t.Fatal(err)
				}
				want = 1
			}
			out := ale47Apply(t, fx, path, "", want)
			if want == 1 {
				if !bytes.Equal(before, readBytes(t, config)) {
					t.Fatal("stale selection wrote settings")
				}
				return
			}
			if out.State != "applied" || len(out.Results) != 2 {
				t.Fatal(out)
			}
			for _, manifest := range manifests {
				if got := ale50PiSource(t, fx, manifest, 1).Availability.State; got != "disabled" {
					t.Fatal(got)
				}
			}
		})
	}
}

func TestALE50PiTargetRelevantSearchGapsRefuse(t *testing.T) {
	for _, scenario := range []string{"active-limit", "unknown-settings", "managed-alias"} {
		t.Run(scenario, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.home, ".pi/agent/skills")
			target := codexSkill(t, root, "target", "target")
			switch scenario {
			case "active-limit":
				dir := filepath.Join(root, "a-unbounded")
				for i := 0; i < 10010; i++ {
					writeFile(t, filepath.Join(dir, fmt.Sprintf("entry-%05d", i)), nil, 0600)
				}
				// An unseen alias could be behind this active traversal limit.
				if err := os.Symlink(filepath.Dir(target), filepath.Join(dir, "z-alias")); err != nil {
					t.Fatal(err)
				}
			case "unknown-settings":
				writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), []byte(`{"skills":["{unqualified,pattern}"]}`), 0600)
			case "managed-alias":
				managed := codexSkill(t, filepath.Join(fx.home, ".pi/agent/npm/node_modules/pkg/skills"), "managed", "managed")
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Dir(target)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(managed), filepath.Dir(target)); err != nil {
					t.Fatal(err)
				}
				target = managed
			}
			// The active-limit fixture can omit the target entirely depending on ordering;
			// use a separately configured exact target to keep the same selected identity.
			if scenario == "active-limit" {
				writeFile(t, filepath.Join(fx.home, ".pi/agent/settings.json"), mustJSON(t, map[string]any{"defaultProjectTrust": "always", "skills": []string{target}}), 0600)
			}
			exit := 1
			if scenario == "managed-alias" {
				exit = 0
			}
			id := ale50PiSource(t, fx, target, exit).ID
			before := treeManifest(t, fx.base)
			out := piChange(t, fx, "global", "disable", id, "", 1)
			if out.State == "ready" {
				t.Fatal("unproven target accepted")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE50PiIgnoredAliasRetainsNativeOccurrence(t *testing.T) {
	for _, ignored := range []string{"alias", "target"} {
		t.Run(ignored, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.home, ".pi/agent/skills")
			target := codexSkill(t, root, "target", "target")
			if err := os.Symlink(filepath.Dir(target), filepath.Join(root, "alias")); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, ".ignore"), []byte(ignored+"/\n"), 0600)
			source := ale50PiSource(t, fx, target, 0)
			if source.Availability.State != "available" {
				t.Fatal(source.Availability)
			}
			piApply(t, fx, "global", "disable", source.ID)
			if ale50PiSource(t, fx, target, 0).Availability.State != "disabled" {
				t.Fatal("remaining native alias not controlled")
			}
		})
	}
}

func TestALE50PiNegatedParentAndWhitespaceProof(t *testing.T) {
	for _, tc := range []struct {
		name, rules, state string
		exit               int
	}{
		{"parent-name", "*\n!target\n!group\n", "unavailable", 0},
		{"parent-directory", "*\n!target/\n!group/\n", "unavailable", 0},
		{"trailing-directory-space", "target/ \n!target/SKILL.md\n", "unknown", 1},
		{"trailing-directory-tab", "target/\t\n!target/SKILL.md\n", "unknown", 1},
		{"restored-parent", "target\n!target/\n", "available", 0},
		{"restored-parent-and-child", "*\n!target/\n!target/SKILL.md\n", "available", 0},
		{"retained-child-exclusion", "target/*\n!target/\n", "unavailable", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.home, ".pi/agent/skills")
			target := codexSkill(t, root, "target", "target")
			codexSkill(t, root, "group/target", "nested")
			codexSkill(t, root, "sibling", "sibling")
			writeFile(t, filepath.Join(root, ".ignore"), []byte(tc.rules), 0600)
			before := treeManifest(t, fx.base)
			selected := ale50PiSource(t, fx, target, tc.exit)
			if selected.Availability.State != tc.state {
				t.Errorf("target = %s, want native-qualified state %s", selected.Availability.State, tc.state)
			}
			for _, choice := range []string{"enable", "disable"} {
				want := 1
				if tc.state == "available" {
					want = 0
				}
				preview := piChange(t, fx, "global", choice, selected.ID, "", want)
				if (preview.State == "ready") != (want == 0) {
					t.Fatal("native participation and preview proof disagree", choice, preview)
				}
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
