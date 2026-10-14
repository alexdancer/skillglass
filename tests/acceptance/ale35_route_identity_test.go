package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type ale35RouteInventory struct {
	Sources []struct {
		codexSource
		ReferenceIDs []string `json:"reference_ids"`
	}
	References []publicReference
	Scope      struct {
		Roots []struct {
			ID, Context, Origin string
			Requested           string `json:"requested_path"`
		}
	}
}

func ale35RouteList(t *testing.T, fx fixture, env []string, extra ...string) ale35RouteInventory {
	t.Helper()
	args := append([]string{"list", "--agent", "opencode", "--project", fx.project, "--json"}, extra...)
	result := opencodeEnvRun(t, fx, env, args...)
	assertExit(t, result, 0)
	var list ale35RouteInventory
	decodeJSON(t, result.stdout, &list)
	return list
}

func ale35OverlapFixture(t *testing.T) fixture {
	t.Helper()
	fx := newFixture(t)
	fx.project = fx.home
	for _, folder := range []string{".agents", ".claude"} {
		codexSkill(t, filepath.Join(fx.home, folder, "skills"), "source", folder[1:]+"-route")
	}
	opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"permission":{"skill":{"agents-route":"ask","claude-route":"deny"}}}`)
	return fx
}

func TestALE35RouteIdentityOverlap(t *testing.T) {
	fx := ale35OverlapFixture(t)
	before := treeManifest(t, fx.base)
	ale35PreserveInputs(t, fx, "overlap", nil, before)
	var original map[string]string
	for _, disabled := range []bool{false, true} {
		label, env := "enabled", []string{}
		if disabled {
			label, env = "disabled", []string{"OPENCODE_DISABLE_EXTERNAL_SKILLS=1"}
		}
		t.Run(label, func(t *testing.T) {
			ale35PreserveInputs(t, fx, label, env, before)
			all := ale35RouteList(t, fx, env)
			if len(all.Sources) != 2 || len(all.References) != 4 {
				t.Errorf("overlap inventory has %d sources/%d references; want 2/4", len(all.Sources), len(all.References))
			}
			ids := map[string]string{}
			for _, source := range all.Sources {
				ids[source.Name] = source.ID
				contexts, refs, roots := map[string]int{}, map[string]bool{}, map[string]bool{}
				for _, ref := range all.References {
					if ref.SourceID == source.ID {
						contexts[ref.Context]++
						refs[ref.ID], roots[ref.RootID] = true, true
					}
				}
				if contexts["global"] != 1 || contexts["project"] != 1 || len(refs) != 2 || len(roots) != 2 || len(source.ReferenceIDs) != 2 || source.ReferenceIDs[0] == source.ReferenceIDs[1] {
					t.Errorf("%s loses route contexts/identities: %+v refs=%v roots=%v", source.Name, contexts, source.ReferenceIDs, roots)
				}
				state, permission := "ask", "ask"
				if source.Name == "claude-route" {
					state, permission = "disabled", "denied"
				}
				if disabled {
					state = "unavailable"
				}
				if source.Availability.State != state || len(source.Availability.References) != 2 {
					t.Errorf("%s state/facts=%+v", source.Name, source.Availability)
				}
				facts := map[string]bool{}
				for _, fact := range source.Availability.References {
					participation := "eligible"
					if disabled {
						participation = "excluded"
					}
					if facts[fact.ReferenceID] || !refs[fact.ReferenceID] || fact.Participation != participation || fact.Permission.State != permission || fact.Permission.Evidence.InputPath != filepath.Join(fx.xdgConfig, "opencode", "opencode.json") {
						t.Errorf("%s availability fact corruption: %+v", source.Name, fact)
					}
					facts[fact.ReferenceID] = true
				}
				for _, operation := range []string{"read", "explain"} {
					result := opencodeEnvRun(t, fx, env, operation, "--agent", "opencode", "--project", fx.project, "--json", source.ID)
					assertExit(t, result, 0)
					var detail struct {
						Source     codexSource
						References []publicReference
					}
					decodeJSON(t, result.stdout, &detail)
					if !reflect.DeepEqual(detail.Source.Availability, source.Availability) || len(detail.References) != 2 || detail.References[0].ID == detail.References[1].ID || detail.References[0].Context == detail.References[1].Context {
						t.Errorf("%s detail loses route provenance", operation)
					}
				}
				human := opencodeEnvRun(t, fx, env, "explain", "--agent", "opencode", "--project", fx.project, source.ID)
				assertExit(t, human, 0)
				if strings.Count(string(human.stdout), "Discovered (global):") != 1 || strings.Count(string(human.stdout), "Discovered (project):") != 1 {
					t.Errorf("human detail repeats/omits route provenance: %s", human.stdout)
				}
			}
			if disabled && !reflect.DeepEqual(ids, original) {
				t.Error("disabled flags changed physical source identity")
			}
			if !disabled {
				original = ids
			}
			for _, context := range []string{"global", "project"} {
				filtered := ale35RouteList(t, fx, env, "--context", context)
				if len(filtered.Sources) != 2 {
					t.Errorf("%s inherited sources=%d want 2", context, len(filtered.Sources))
				}
				for _, source := range filtered.Sources {
					if source.ID != ids[source.Name] {
						t.Error("context filter changes physical identity")
					}
				}
				for _, folder := range []string{".agents", ".claude"} {
					root := filepath.Join(fx.home, folder, "skills")
					selected := ale35RouteList(t, fx, env, "--source-root", root, "--context", context)
					if len(selected.Sources) != 1 || selected.Sources[0].ID != ids[folder[1:]+"-route"] {
						t.Errorf("root/context conjunction loses %s/%s", root, context)
					}
				}
			}
		})
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}

func TestALE35RouteIdentityConfigured(t *testing.T) {
	for _, aliases := range []bool{false, true} {
		label := "repeated-configured-path"
		if aliases {
			label = "alias-and-independent-copy"
		}
		t.Run(label, func(t *testing.T) {
			fx := newFixture(t)
			one := filepath.Join(fx.base, "allowed", "configured")
			codexSkill(t, one, "one", "configured-route")
			paths := []string{one, one}
			if aliases {
				alias, other := filepath.Join(fx.base, "allowed", "alias"), filepath.Join(fx.base, "allowed", "other")
				if err := os.Symlink(one, alias); err != nil {
					t.Fatal(err)
				}
				codexSkill(t, other, "one", "configured-route")
				paths = append(paths, alias, other)
			}
			encoded, _ := json.Marshal(paths)
			opencodeConfig(t, filepath.Join(fx.xdgConfig, "opencode", "opencode.json"), `{"skills":{"paths":`+string(encoded)+`}}`)
			before := treeManifest(t, fx.base)
			ale35PreserveInputs(t, fx, "configured", nil, before)
			all := ale35RouteList(t, fx, nil)
			wantSources, wantRefs := 1, 1
			if aliases {
				wantSources, wantRefs = 2, 3
			}
			if len(all.Sources) != wantSources || len(all.References) != wantRefs {
				t.Errorf("configured source/reference count=%d/%d want %d/%d", len(all.Sources), len(all.References), wantSources, wantRefs)
			}
			var routes []string
			for _, root := range all.Scope.Roots {
				if root.Origin == "opencode-configured-path" {
					routes = append(routes, root.Requested)
				}
			}
			wantRoutes := append([]string{one}, paths[2:]...)
			if !reflect.DeepEqual(routes, wantRoutes) {
				t.Errorf("first-occurrence order/path provenance=%v want %v", routes, wantRoutes)
			}
			for _, source := range all.Sources {
				seen := map[string]bool{}
				for _, id := range source.ReferenceIDs {
					if seen[id] {
						t.Errorf("configured route repeats reference %s", id)
					}
					seen[id] = true
				}
				if len(source.ReferenceIDs) != len(source.Availability.References) {
					t.Error("availability/reference counts differ")
				}
				if aliases && (source.Availability.State != "unknown" || source.Availability.Precedence.State != "unknown" || source.Availability.Precedence.Winner != "" || len(source.Availability.Precedence.RelatedSources) != 1) {
					t.Error("independent-copy collision incorrectly resolved")
				}
				for _, operation := range []string{"read", "explain"} {
					result := opencodeEnvRun(t, fx, nil, operation, "--agent", "opencode", "--project", fx.project, "--json", source.ID)
					assertExit(t, result, 0)
					var detail struct{ Source codexSource }
					decodeJSON(t, result.stdout, &detail)
					if !reflect.DeepEqual(detail.Source.Availability, source.Availability) {
						t.Error("configured detail changes availability")
					}
				}
			}
			var selectedIDs []string
			for _, root := range wantRoutes {
				selected := ale35RouteList(t, fx, nil, "--source-root", root, "--context", "custom")
				if len(selected.Sources) != 1 {
					t.Fatal("distinct route is not inspectable")
				}
				selectedIDs = append(selectedIDs, selected.Sources[0].ID)
			}
			if aliases && (selectedIDs[0] != selectedIDs[1] || selectedIDs[0] == selectedIDs[2]) {
				t.Error("alias sharing or independent-copy identity broken")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}

func TestALE35RouteIdentityDistinctAndGenericControls(t *testing.T) {
	t.Run("distinct-home-project", func(t *testing.T) {
		fx := newFixture(t)
		for _, folder := range []string{".agents", ".claude"} {
			codexSkill(t, filepath.Join(fx.home, folder, "skills"), "global", folder[1:]+"-global")
			codexSkill(t, filepath.Join(fx.project, folder, "skills"), "project", folder[1:]+"-project")
		}
		before := treeManifest(t, fx.base)
		ale35PreserveInputs(t, fx, "distinct", nil, before)
		all := ale35RouteList(t, fx, nil)
		if len(all.Sources) != 4 || len(all.References) != 4 {
			t.Error("distinct roots were conflated")
		}
		for _, context := range []string{"global", "project"} {
			if len(ale35RouteList(t, fx, nil, "--context", context).Sources) != 2 {
				t.Error("distinct context filter changed")
			}
		}
		assertTreeUnchanged(t, before, treeManifest(t, fx.base))
	})
	for _, agent := range []string{"generic", "opencode"} {
		t.Run("repeated-explicit-"+agent, func(t *testing.T) {
			fx := newFixture(t)
			root := filepath.Join(fx.base, "allowed", "explicit")
			codexSkill(t, root, "one", "explicit-route")
			before := treeManifest(t, fx.base)
			ale35PreserveInputs(t, fx, "explicit", nil, before)
			args := []string{"list", "--json", "--project", fx.project, "--root", root, "--root", root}
			if agent == "opencode" {
				args = append(args, "--agent", agent)
			}
			result := fx.run(t, args...)
			assertExit(t, result, 0)
			var list ale35RouteInventory
			decodeJSON(t, result.stdout, &list)
			if len(list.Sources) != 1 || len(list.Sources[0].ReferenceIDs) != 2 || list.Sources[0].ReferenceIDs[0] == list.Sources[0].ReferenceIDs[1] {
				t.Error("generic explicit-root occurrence identity changed")
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
		})
	}
}
