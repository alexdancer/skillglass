package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func decision5RetainTree(t *testing.T, fx fixture, label string) {
	t.Helper()
	err := filepath.Walk(fx.base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fx.base, path)
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			writeEvidence(t, "fixtures/"+label+"/"+rel, raw)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			writeEvidence(t, "fixtures/"+label+"/"+rel+".symlink-target", []byte(target))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDecision5Precedence(t *testing.T) {
	for _, scenario := range []string{"higher-pattern", "higher-ignore", "higher-settings", "higher-ask", "later-pattern", "disabled", "invalid", "denied", "inactive", "independent", "same-source-alias", "alias-competitor", "same-rank-earlier", "same-rank-later", "known-first", "explicit-first", "lower-explicit", "ask-disabled", "known-first-later-uncertain"} {
		t.Run(scenario, func(t *testing.T) {
			fx := piFixture(t)
			global := filepath.Join(fx.home, ".pi", "agent", "skills")
			lowManifest := codexSkill(t, global, "global", "collision")
			highRoot := filepath.Join(fx.base, "allowed", "configured")
			highManifest := codexSkill(t, highRoot, "🔍", "collision")
			projectSettings := map[string]any{"skills": []string{filepath.Dir(highManifest), "!?"}}
			lowState, highState, code := "unknown", "unknown", 1
			switch scenario {
			case "higher-ignore":
				projectSettings["skills"] = []string{filepath.Dir(highManifest)}
				writeFile(t, filepath.Join(filepath.Dir(highManifest), ".ignore"), []byte("*\n"), 0600)
			case "higher-settings":
				projectSettings["skills"] = []string{filepath.Dir(highManifest)}
				projectSettings["enableSkillCommands"] = "invalid"
			case "higher-ask":
				projectSettings["skills"] = []string{filepath.Dir(highManifest)}
				piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), map[string]any{"defaultProjectTrust": "ask"})
				highState, code = "ask", 0
			case "later-pattern", "known-first-later-uncertain":
				highState, lowState = "available", "unknown"
				projectSettings["skills"] = []string{filepath.Dir(highManifest)}
				replacement := filepath.Join(global, "🔍")
				if err := os.Rename(filepath.Dir(lowManifest), replacement); err != nil {
					t.Fatal(err)
				}
				lowManifest = filepath.Join(replacement, "SKILL.md")
				piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), map[string]any{"defaultProjectTrust": "always", "skills": []string{"!?"}})
				if scenario == "known-first-later-uncertain" {
					codexSkill(t, global, "zz-known", "collision")
				}
			case "known-first", "explicit-first":
				projectSettings["skills"] = []string{filepath.Dir(highManifest)}
				lowState, highState, code = "shadowed", "available", 0
				if scenario == "explicit-first" {
					writeFile(t, highManifest, []byte("---\nname: collision\ndescription: explicit first\ndisable-model-invocation: true\n---\nEXPLICIT_FIRST\n"), 0600)
					highState = "explicit-only"
				}
			case "lower-explicit":
				writeFile(t, lowManifest, []byte("---\nname: collision\ndescription: explicit lower\ndisable-model-invocation: true\n---\nEXPLICIT_LOWER\n"), 0600)
			case "ask-disabled":
				projectSettings["skills"] = []string{filepath.Dir(highManifest), "-" + filepath.Dir(highManifest)}
				piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "settings.json"), map[string]any{"defaultProjectTrust": "ask"})
				lowState, highState, code = "available", "ask", 0
			case "disabled":
				projectSettings["skills"] = []string{filepath.Dir(highManifest), "-" + filepath.Dir(highManifest)}
				lowState, highState, code = "available", "disabled", 0
			case "invalid":
				writeFile(t, highManifest, []byte("---\nname: collision\n---\nINVALID_DESCRIPTION\n"), 0600)
				lowState, highState = "available", "unavailable"
			case "denied":
				piSettingsFile(t, filepath.Join(fx.home, ".pi", "agent", "trust.json"), map[string]bool{fx.project: false})
				lowState, highState = "available", "unavailable"
			case "inactive":
				cache := filepath.Join(fx.home, ".pi", "agent", "git", "unregistered", "skills")
				if err := os.MkdirAll(cache, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Dir(highManifest), filepath.Join(cache, "🔍")); err != nil {
					t.Fatal(err)
				}
				highManifest = filepath.Join(cache, "🔍", "SKILL.md")
				projectSettings = map[string]any{}
				lowState, highState, code = "available", "unavailable", 0
			case "independent":
				writeFile(t, highManifest, []byte("---\nname: different\ndescription: independent\n---\nDIFFERENT\n"), 0600)
				lowState = "available"
			case "same-source-alias", "alias-competitor":
				if err := os.RemoveAll(filepath.Dir(highManifest)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(lowManifest), filepath.Dir(highManifest)); err != nil {
					t.Fatal(err)
				}
				if scenario == "same-source-alias" {
					lowState = "unknown"
				} else {
					lowManifest = codexSkill(t, global, "other", "collision")
				}
			case "same-rank-earlier", "same-rank-later":
				if err := os.RemoveAll(filepath.Dir(highManifest)); err != nil {
					t.Fatal(err)
				}
				first, second := "a🔍", "zz"
				if scenario == "same-rank-later" {
					first, second = "z🔍", "aa"
					lowState = "available"
				}
				highManifest = codexSkill(t, highRoot, first, "collision")
				if err := os.RemoveAll(filepath.Dir(lowManifest)); err != nil {
					t.Fatal(err)
				}
				lowManifest = codexSkill(t, highRoot, second, "collision")
				projectSettings["skills"] = []string{highRoot, "!?"}
			}
			codexSkill(t, global, "healthy", "healthy")
			piSettingsFile(t, filepath.Join(fx.project, ".pi", "settings.json"), projectSettings)
			before := treeManifest(t, fx.base)
			writeEvidence(t, "trees/"+scenario+"-before.json", before)
			decision5RetainTree(t, fx, scenario)
			list := piRelatedInventory(t, fx, code)
			low := piRelatedSource(t, list, canonicalPath(t, filepath.Dir(lowManifest)))
			high := piRelatedSource(t, list, canonicalPath(t, filepath.Dir(highManifest)))
			if string(low.Availability.State) != lowState {
				t.Errorf("lower state=%s want=%s", low.Availability.State, lowState)
			}
			if string(high.Availability.State) != highState {
				t.Errorf("higher state=%s want=%s", high.Availability.State, highState)
			}
			if lowState == "unknown" && scenario != "same-source-alias" && scenario != "later-pattern" && scenario != "known-first-later-uncertain" {
				p := low.Availability.Precedence
				if p == nil || p.State != collection.PrecedenceUnknown || p.Winner != "" || p.Evidence.Kind != collection.EvidenceUnknown || low.Availability.Evidence.Kind != collection.EvidenceUnknown || !slices.Contains(p.RelatedSources, high.ID) {
					t.Errorf("lower precedence/evidence overconfident: %+v evidence=%+v", p, low.Availability.Evidence)
				}
				for _, ref := range low.Availability.References {
					if ref.Reference == low.Availability.ReferenceID && (ref.Permission.State != collection.PermissionAllowed || ref.Permission.Evidence.Kind != collection.EvidenceInferred) {
						t.Errorf("precedence rewrote local permission: %+v", ref.Permission)
					}
				}
				if scenario != "higher-settings" && slices.ContainsFunc(low.Problems, func(problem collection.Problem) bool { return problem.Code == collection.ProblemNativeInput }) {
					t.Error("higher occurrence diagnostic spread to lower source")
				}
			}
			if lowState == "shadowed" && low.Availability.Precedence.Winner != high.ID {
				t.Error("known winner was not retained")
			}
			if scenario == "known-first-later-uncertain" {
				third := piRelatedSource(t, list, canonicalPath(t, filepath.Join(global, "zz-known")))
				if third.Availability.State != collection.AvailabilityShadowed || third.Availability.Precedence.Winner != high.ID || slices.ContainsFunc(third.Problems, func(problem collection.Problem) bool { return problem.Code == collection.ProblemNativeInput }) {
					t.Error("later uncertainty displaced known first winner or spread its diagnostic")
				}
			}
			if low.ID == high.ID && scenario == "same-source-alias" && len(low.Availability.Precedence.RelatedSources) != 0 {
				t.Error("one shared source competes with itself")
			}
			if scenario == "later-pattern" && high.Availability.Precedence.Winner != "" && high.Availability.Precedence.Winner != high.ID {
				t.Error("later uncertainty replaced known earlier winner")
			}
			if scenario != "same-rank-earlier" && scenario != "same-rank-later" {
				healthy := piRelatedSource(t, list, canonicalPath(t, filepath.Join(global, "healthy")))
				if healthy.Availability.State != collection.AvailabilityAvailable || (scenario != "higher-settings" && len(healthy.Problems) != 0) {
					t.Errorf("independent healthy source contaminated: %+v", healthy.Availability)
				}
			}
			for _, op := range []string{"read", "explain"} {
				r := fx.run(t, op, "--agent", "pi", "--project", fx.project, "--json", string(low.ID))
				assertExit(t, r, code)
				var detail struct{ Source collection.Source }
				decodeJSON(t, r.stdout, &detail)
				if string(detail.Source.Availability.State) != lowState {
					t.Errorf("%s lower=%s want=%s", op, detail.Source.Availability.State, lowState)
				}
			}
			human := fx.run(t, "explain", "--agent", "pi", "--project", fx.project, string(low.ID))
			assertExit(t, human, code)
			if !strings.Contains(string(human.stdout), "Availability: "+lowState) {
				t.Errorf("human explanation lacks expected %s", lowState)
			}
			filtered := piRelatedInventory(t, fx, code, "--availability", lowState)
			found := false
			for _, s := range filtered.Sources {
				found = found || s.ID == low.ID
			}
			if !found {
				t.Errorf("%s filter omits lower source", lowState)
			}
			if scenario == "higher-pattern" {
				selected := piRelatedInventory(t, fx, code, "--source-root", global)
				view := piRelatedSource(t, selected, canonicalPath(t, filepath.Dir(lowManifest)))
				if view.Availability.State != collection.AvailabilityUnknown {
					t.Error("source-root filtering erases higher uncertainty")
				}
				selected = piRelatedInventory(t, fx, code, "--context", "global")
				view = piRelatedSource(t, selected, canonicalPath(t, filepath.Dir(lowManifest)))
				if view.Availability.State != collection.AvailabilityUnknown {
					t.Error("context filtering erases higher uncertainty")
				}
			}
			after := treeManifest(t, fx.base)
			writeEvidence(t, "trees/"+scenario+"-after.json", after)
			assertTreeUnchanged(t, before, after)
			raw, err := json.MarshalIndent(map[string]any{"scenario": scenario, "low": low, "high": high, "expected_low": lowState, "expected_high": highState}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			writeEvidence(t, "observations/"+scenario+".json", raw)
		})
	}
}
