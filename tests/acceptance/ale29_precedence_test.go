package acceptance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func claudePrecedenceAlias(t *testing.T, bundle, alias string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bundle, alias); err != nil {
		t.Fatal(err)
	}
}

func claudePrecedenceInspect(t *testing.T, fx fixture, label string, want map[string]string, extra ...string) codexList {
	t.Helper()
	before := treeManifest(t, fx.base)
	args := append([]string{"list", "--agent", "claude-code", "--project", fx.project, "--json"}, extra...)
	r := fx.run(t, args...)
	assertExit(t, r, 0)
	writeEvidence(t, "precedence/"+label+"-list.json", r.stdout)
	var list codexList
	decodeJSON(t, r.stdout, &list)
	for directory, state := range want {
		s := claudeAt(t, list, directory)
		if s.Availability.State != state {
			t.Errorf("%s state=%s want %s", directory, s.Availability.State, state)
		}
		detailArgs := append([]string{"explain", "--agent", "claude-code", "--project", fx.project}, extra...)
		detail := fx.run(t, append(detailArgs, s.ID)...)
		assertExit(t, detail, 0)
		writeEvidence(t, "precedence/"+label+"-"+filepath.Base(directory)+"-explain.txt", detail.stdout)
		if !strings.Contains(string(detail.stdout), "Availability: "+state) {
			t.Errorf("explain %s lacks Availability: %s", s.ID, state)
		}
		for _, related := range s.Availability.Precedence.RelatedSources {
			if !strings.Contains(string(detail.stdout), "Related independent source: "+related) {
				t.Errorf("explain %s omits related source %s", s.ID, related)
			}
		}
		if state == "shadowed" && s.Availability.Precedence.Winner == "" {
			if s.Availability.Precedence.State != "unknown" || s.Availability.Precedence.Evidence.Kind != "unknown" || !strings.Contains(s.Availability.Precedence.Evidence.Reason, "higher-priority") || !strings.Contains(string(detail.stdout), "Precedence: unknown (unknown)") {
				t.Errorf("known lower shadowing confused with unresolved winner: %+v", s.Availability.Precedence)
			}
		}
		jsonArgs := append([]string{"explain", "--agent", "claude-code", "--project", fx.project, "--json"}, extra...)
		jsonDetail := fx.run(t, append(jsonArgs, s.ID)...)
		assertExit(t, jsonDetail, 0)
		var parsed struct{ Source codexSource }
		decodeJSON(t, jsonDetail.stdout, &parsed)
		if !reflect.DeepEqual(parsed.Source.Availability, s.Availability) {
			t.Errorf("list/explain parity lost for %s", s.ID)
		}
		t.Logf("%s source=%s state=%s name=%s name-kind=%s precedence=%s winner=%s related=%v", directory, s.ID, s.Availability.State, s.Availability.NativeName, s.Availability.NativeNameEvidence.Kind, s.Availability.Precedence.State, s.Availability.Precedence.Winner, s.Availability.Precedence.RelatedSources)
	}
	for _, state := range []string{"unknown", "available", "shadowed"} {
		fargs := append(append([]string{}, args...), "--availability", state)
		f := fx.run(t, fargs...)
		assertExit(t, f, 0)
		writeEvidence(t, "precedence/"+label+"-"+state+"-filter.json", f.stdout)
		var filtered codexList
		decodeJSON(t, f.stdout, &filtered)
		for directory, expected := range want {
			s := claudeAt(t, list, directory)
			found := false
			for _, v := range filtered.Sources {
				found = found || v.ID == s.ID
			}
			if found != (expected == state) {
				t.Errorf("%s filter %s membership=%t want %t", state, directory, found, expected == state)
			}
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	return list
}

func TestALE29PossibleEligibleNameCompetitor(t *testing.T) {
	for _, mode := range []string{"higher-possible-work", "equal-possible-work", "mixed-rank-higher-work", "mixed-rank-lower-work-control", "lower-possible-work-control", "unrelated-name-control", "explicit-name-control", "excluded-alias-control", "inspection-alias-control"} {
		t.Run(mode, func(t *testing.T) {
			fx, _ := codexFixture(t)
			personal := filepath.Join(fx.home, ".claude/skills")
			project := filepath.Join(fx.project, ".claude/skills")
			aliasRoot, independentRoot, independentName := personal, project, "work"
			manifestName := ""
			sharedState, independentState := "unknown", "unknown"
			extra := []string{}
			aliases := []string{"work", "other"}
			switch mode {
			case "equal-possible-work":
				independentRoot = personal
			case "mixed-rank-lower-work-control":
				independentRoot = personal
				independentState = "available"
			case "lower-possible-work-control":
				aliasRoot = project
				independentRoot = personal
				independentState = "available"
			case "unrelated-name-control":
				independentName = "unrelated"
				independentState = "available"
			case "explicit-name-control":
				manifestName = "work"
				sharedState = "available"
				independentState = "shadowed"
			case "excluded-alias-control":
				aliases = []string{"work", "synced"}
				sharedState = "available"
				independentState = "shadowed"
			case "inspection-alias-control":
				aliases = []string{"work"}
				sharedState = "available"
				independentState = "shadowed"
			}
			bundle := filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/shared"), "bundle", manifestName, ""))
			for _, name := range aliases {
				root := aliasRoot
				if (mode == "mixed-rank-higher-work" && name == "other") || (mode == "mixed-rank-lower-work-control" && name == "work") {
					root = project
				}
				claudePrecedenceAlias(t, bundle, filepath.Join(root, name))
			}
			if mode == "inspection-alias-control" {
				viewer := filepath.Join(fx.base, "allowed/viewer")
				claudePrecedenceAlias(t, bundle, filepath.Join(viewer, "other"))
				extra = []string{"--root", viewer}
			}
			independent := filepath.Dir(claudeSkill(t, independentRoot, "independent", independentName, ""))
			list := claudePrecedenceInspect(t, fx, mode, map[string]string{bundle: sharedState, independent: independentState}, extra...)
			shared, other := claudeAt(t, list, bundle), claudeAt(t, list, independent)
			if len(list.Sources) != 2 || shared.ID == other.ID || len(shared.Availability.References) != 2 {
				t.Errorf("source identity or sharing changed: %+v", list.Sources)
			}
			if independentState == "unknown" {
				if other.Availability.Evidence.Kind != "unknown" || other.Availability.Precedence.Evidence.Kind != "unknown" {
					t.Errorf("possible competitor gained positive confidence: %+v", other.Availability)
				}
				if other.Availability.Precedence.State != "unknown" || other.Availability.Precedence.Winner != "" {
					t.Errorf("possible same/higher contender must withhold definite winner: %+v", other.Availability.Precedence)
				}
				found := false
				for _, id := range other.Availability.Precedence.RelatedSources {
					found = found || id == shared.ID
				}
				if !found {
					t.Error("possible eligible-name competitor missing from related sources")
				}
			}
			if independentState == "available" && (other.Availability.Precedence.State != "unique" || other.Availability.Precedence.Winner != other.ID) {
				t.Errorf("unaffected/higher source lost definite winner: %+v", other.Availability.Precedence)
			}
			if independentState == "shadowed" && other.Availability.Precedence.Winner != shared.ID {
				t.Errorf("known shared source winner lost: %+v", other.Availability.Precedence)
			}
			if sharedState == "unknown" && (shared.Availability.Evidence.Rule != "shared-native-name" || shared.Availability.NativeNameEvidence.Kind != "unknown") {
				t.Errorf("shared-name uncertainty lost: %+v", shared.Availability)
			}
			for _, fact := range shared.Availability.References {
				want := "eligible"
				if fact.Evidence.Rule == "reserved-skill-name" {
					want = "excluded"
				}
				if fact.Evidence.Rule == "inspection-only" {
					want = "inspection-only"
				}
				if fact.Participation != want {
					t.Errorf("route status changed: %+v", fact)
				}
			}
		})
	}
}

func TestALE29TopTieDoesNotUnshadowLowerRank(t *testing.T) {
	for _, mode := range []string{"personal-top-tie", "project-top-tie", "same-physical-control", "lower-tie-control"} {
		t.Run(mode, func(t *testing.T) {
			fx, repo := codexFixture(t)
			personal := filepath.Join(fx.home, ".claude/skills")
			near := filepath.Join(fx.project, ".claude/skills")
			far := filepath.Join(repo, ".claude/skills")
			topRoot, lowRoot := personal, near
			if mode == "project-top-tie" {
				topRoot = near
				lowRoot = far
			}
			paths := []string{}
			want := map[string]string{}
			if mode == "same-physical-control" {
				shared := filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/shared"), "bundle", "action", ""))
				claudePrecedenceAlias(t, shared, filepath.Join(topRoot, "first"))
				claudePrecedenceAlias(t, shared, filepath.Join(topRoot, "second"))
				paths = append(paths, shared)
				want[shared] = "available"
			} else {
				first := filepath.Dir(claudeSkill(t, topRoot, "first", "action", ""))
				paths = append(paths, first)
				want[first] = "unknown"
				secondRoot := topRoot
				if mode == "lower-tie-control" {
					secondRoot = lowRoot
					want[first] = "available"
				}
				second := filepath.Dir(claudeSkill(t, secondRoot, "second", "action", ""))
				paths = append(paths, second)
				want[second] = "unknown"
				if mode == "lower-tie-control" {
					want[second] = "shadowed"
				}
			}
			low := filepath.Dir(claudeSkill(t, lowRoot, "lower", "action", ""))
			paths = append(paths, low)
			want[low] = "shadowed"
			list := claudePrecedenceInspect(t, fx, mode, want)
			lower := claudeAt(t, list, low)
			if mode == "personal-top-tie" || mode == "project-top-tie" {
				if lower.Availability.Precedence.Winner != "" {
					t.Error("unresolved top tie gained winner identity")
				}
				if len(lower.Availability.Precedence.RelatedSources) != 2 {
					t.Error("lower rank lost both independent top contenders")
				}
				for _, path := range paths[:2] {
					top := claudeAt(t, list, path)
					if top.Availability.Precedence.State != "unknown" || top.Availability.Precedence.Winner != "" {
						t.Errorf("top tie gained confidence: %+v", top.Availability.Precedence)
					}
					if !slices.Contains(lower.Availability.Precedence.RelatedSources, top.ID) {
						t.Errorf("lower rank lost contender identity %s", top.ID)
					}
				}
			} else {
				winner := claudeAt(t, list, paths[0])
				if lower.Availability.Precedence.Winner != winner.ID {
					t.Error("known winner changed")
				}
				if mode == "same-physical-control" && (len(list.Sources) != 2 || len(winner.Availability.References) != 2) {
					t.Error("shared explicit-name references became independent tie")
				}
			}
			if lower.Availability.Evidence.Kind != "inferred" {
				t.Errorf("certain shadowing evidence=%s want inferred", lower.Availability.Evidence.Kind)
			}
		})
	}
}

func TestALE29UnresolvedManagedAliasControl(t *testing.T) {
	fx, _ := codexFixture(t)
	plugin := filepath.Dir(claudeRootPluginFixture(t, fx, nil, false))
	claudeSettings(t, filepath.Join(fx.home, ".claude/settings.json"), map[string]any{"enabledPlugins": map[string]bool{"demo@market": false}})
	claudePrecedenceAlias(t, plugin, filepath.Join(fx.home, ".claude/skills", "alias"))
	claudePrecedenceAlias(t, plugin, filepath.Join(fx.home, ".claude/skills", "synced"))
	independent := filepath.Dir(claudeSkill(t, filepath.Join(fx.project, ".claude/skills"), "independent", "unrelated", ""))
	list := claudePrecedenceInspect(t, fx, "unresolved-managed-alias", map[string]string{plugin: "unknown", independent: "available"})
	s := claudeAt(t, list, plugin)
	got := map[string]int{}
	for _, f := range s.Availability.References {
		got[f.Participation+"/"+f.Evidence.Rule]++
	}
	want := map[string]int{"excluded/plugin-disabled": 1, "excluded/reserved-skill-name": 1, "unknown/managed-alias-unverified": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("route exclusion/uncertainty invariant: got %v want %v", got, want)
	}
}

func TestALE29PossibleNamesPreserveDefiniteShadowing(t *testing.T) {
	for _, mode := range []string{"possible-higher-rank", "possible-below-definite-winner"} {
		t.Run(mode, func(t *testing.T) {
			fx, repo := codexFixture(t)
			personal := filepath.Join(fx.home, ".claude/skills")
			near := filepath.Join(fx.project, ".claude/skills")
			far := filepath.Join(repo, ".claude/skills")
			bundle := filepath.Dir(claudeSkill(t, filepath.Join(fx.base, "allowed/shared"), "bundle", "", ""))
			workRoot, middleRoot, middleState := personal, near, "unknown"
			if mode == "possible-below-definite-winner" {
				workRoot, middleRoot, middleState = near, personal, "available"
			}
			claudePrecedenceAlias(t, bundle, filepath.Join(workRoot, "work"))
			claudePrecedenceAlias(t, bundle, filepath.Join(personal, "other"))
			middle := filepath.Dir(claudeSkill(t, middleRoot, "middle", "work", ""))
			lower := filepath.Dir(claudeSkill(t, far, "lower", "work", ""))
			list := claudePrecedenceInspect(t, fx, mode, map[string]string{bundle: "unknown", middle: middleState, lower: "shadowed"})
			shared, definite, low := claudeAt(t, list, bundle), claudeAt(t, list, middle), claudeAt(t, list, lower)
			if len(list.Sources) != 3 || shared.ID == definite.ID || shared.ID == low.ID || definite.ID == low.ID {
				t.Fatalf("independent source identities changed: %+v", list.Sources)
			}
			for _, source := range []codexSource{shared, definite, low} {
				if len(source.Availability.Precedence.RelatedSources) != 2 {
					t.Errorf("competitor identities missing: %+v", source.Availability.Precedence)
				}
				for _, id := range []string{shared.ID, definite.ID, low.ID} {
					if slices.Contains(source.Availability.Precedence.RelatedSources, id) != (id != source.ID) {
						t.Errorf("related identity %s for %s", id, source.ID)
					}
				}
			}
			winner, precedenceState := "", "unknown"
			if mode == "possible-below-definite-winner" {
				winner, precedenceState = definite.ID, "unique"
			}
			if low.Availability.Precedence.Winner != winner || low.Availability.Precedence.State != precedenceState || low.Availability.Evidence.Kind != "inferred" {
				t.Errorf("possible contender erased definite lower exclusion: %+v", low.Availability)
			}
			if definite.Availability.Precedence.Winner != winner || definite.Availability.Precedence.State != precedenceState {
				t.Errorf("matching-name winner confidence: %+v", definite.Availability.Precedence)
			}
			if shared.Availability.Evidence.Rule != "shared-native-name" || shared.Availability.NativeNameEvidence.Kind != "unknown" || shared.Availability.Precedence.Winner != "" {
				t.Errorf("possible names became verified: %+v", shared.Availability)
			}
			for _, source := range []codexSource{definite, low} {
				if source.Availability.Invocation.State != "automatic-and-user" || source.Availability.Session.State != "unknown" || source.Availability.Reload.State != "unknown" {
					t.Errorf("precedence changed invocation/session confidence: %+v", source.Availability)
				}
				for _, capability := range source.Availability.Capabilities {
					if capability.State != "unsupported" {
						t.Errorf("precedence enabled writer: %+v", capability)
					}
				}
			}
		})
	}
}
