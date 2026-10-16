package acceptance_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func piInheritedRegistration(t *testing.T, fx fixture, target, spelling string) string {
	t.Helper()
	path := target
	if strings.HasSuffix(spelling, "directory") {
		path = filepath.Dir(target)
	}
	switch {
	case strings.HasPrefix(spelling, "relative"):
		relative, err := filepath.Rel(filepath.Join(fx.project, ".pi"), path)
		if err != nil {
			t.Fatal(err)
		}
		return relative
	case strings.HasPrefix(spelling, "tilde"):
		relative, err := filepath.Rel(fx.home, path)
		if err != nil {
			t.Fatal(err)
		}
		return "~/" + relative
	default:
		return path
	}
}

func TestALE34InheritedUserRegistrationsRefuseNewOverrides(t *testing.T) {
	for _, spelling := range []string{"absolute-file", "relative-file", "tilde-file", "absolute-directory", "relative-directory", "tilde-directory"} {
		t.Run(spelling, func(t *testing.T) {
			fx := piFixture(t)
			root := filepath.Join(fx.home, ".pi/agent/skills")
			target := codexSkill(t, root, "target", "target")
			sibling := codexSkill(t, root, "sibling", "sibling")
			global := filepath.Join(fx.home, ".pi/agent/settings.json")
			data, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "skills": []string{"-" + target}})
			writeFile(t, global, data, 0600)
			registration := piInheritedRegistration(t, fx, target, spelling)
			project := filepath.Join(fx.project, ".pi/settings.json")
			data, _ = json.Marshal(map[string]any{"skills": []string{registration, "!unrelated*"}, "keep": map[string]any{"number": 42}})
			writeFile(t, project, data, 0600)
			source := piSourceAt(t, fx, target)
			if source.Availability.State != "available" {
				t.Fatal("user project resource should win global deny", source.Availability)
			}
			before := treeManifest(t, fx.base)
			projectBytes := readBytes(t, project)
			siblingBytes := readBytes(t, sibling)
			for _, choice := range []string{"enable", "disable"} {
				out := piChange(t, fx, "project", choice, source.ID, "", 1)
				if out.State != "refused" || !strings.Contains(out.Reason, "configured resource") || out.Confirmation != "" {
					t.Fatal(out)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			}
			// A lone registration remains user discovery, not a removable native pair.
			piApply(t, fx, "project", "inherit", source.ID)
			if !bytes.Equal(projectBytes, readBytes(t, project)) || !bytes.Equal(siblingBytes, readBytes(t, sibling)) {
				t.Fatal("user registration or independent content changed")
			}
		})
	}
}

func TestALE34MixedNativePairAndRetainedRegistrationRefusesInheritance(t *testing.T) {
	for _, globalChoice := range []string{"allowed", "denied"} {
		for _, spelling := range []string{"relative-file", "tilde-file", "absolute-directory", "relative-directory", "tilde-directory"} {
			t.Run(globalChoice+"/"+spelling, func(t *testing.T) {
				fx := piFixture(t)
				target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
				global := filepath.Join(fx.home, ".pi/agent/settings.json")
				globalEntries := []string{}
				if globalChoice == "denied" {
					globalEntries = append(globalEntries, "-"+target)
				}
				data, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "skills": globalEntries})
				writeFile(t, global, data, 0600)
				unrelated := filepath.Join(fx.base, "allowed/unrelated-resources")
				other := codexSkill(t, unrelated, "other", "other")
				project := filepath.Join(fx.project, ".pi/settings.json")
				registration := piInheritedRegistration(t, fx, target, spelling)
				// The pair comes first to exercise equivalent-root deduplication as well.
				data, _ = json.Marshal(map[string]any{"skills": []string{target, "-" + target, registration, unrelated, "!unrelated-exclusion"}, "keep": true})
				writeFile(t, project, data, 0600)
				id := piSourceAt(t, fx, target).ID
				before := treeManifest(t, fx.base)
				otherState := piSourceAt(t, fx, other).Availability.State
				out := piChange(t, fx, "project", "inherit", id, "", 1)
				if out.State != "refused" || !strings.Contains(out.Reason, "inheritance") || out.Confirmation != "" {
					t.Fatal(out)
				}
				assertTreeUnchanged(t, before, treeManifest(t, fx.base))
				if piSourceAt(t, fx, other).Availability.State != otherState {
					t.Fatal("independent project resource changed")
				}
			})
		}
	}
}

func TestALE34OrdinaryPairsRestoreInheritanceWithUnrelatedProjectResources(t *testing.T) {
	for _, globalChoice := range []string{"allowed", "denied"} {
		for _, choice := range []string{"enable", "disable"} {
			t.Run(globalChoice+"/"+choice, func(t *testing.T) {
				fx := piFixture(t)
				target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
				global := filepath.Join(fx.home, ".pi/agent/settings.json")
				globalEntries := []string{}
				want := "available"
				if globalChoice == "denied" {
					globalEntries = append(globalEntries, "-"+target)
					want = "disabled"
				}
				data, _ := json.Marshal(map[string]any{"defaultProjectTrust": "always", "skills": globalEntries})
				writeFile(t, global, data, 0600)
				globalBytes := readBytes(t, global)
				unrelated := filepath.Join(fx.base, "allowed/unrelated-resources")
				other := codexSkill(t, unrelated, "other", "other")
				project := filepath.Join(fx.project, ".pi/settings.json")
				data, _ = json.Marshal(map[string]any{"skills": []string{unrelated, "!irrelevant"}, "keep": true})
				writeFile(t, project, data, 0600)
				id := piSourceAt(t, fx, target).ID
				otherBytes := readBytes(t, other)
				piApply(t, fx, "project", choice, id)
				piApply(t, fx, "project", "inherit", id)
				if piSourceAt(t, fx, target).Availability.State != want {
					t.Fatal("ordinary native pair did not restore inherited choice")
				}
				if !bytes.Equal(globalBytes, readBytes(t, global)) || !bytes.Equal(otherBytes, readBytes(t, other)) || piSourceAt(t, fx, other).Availability.State != "available" {
					t.Fatal("unrelated state changed")
				}
				doc := piSettingsValues(t, project)
				var entries []string
				if err := json.Unmarshal(doc["skills"], &entries); err != nil {
					t.Fatal(err)
				}
				if len(entries) != 2 || entries[0] != unrelated || entries[1] != "!irrelevant" || string(doc["keep"]) != "true" {
					t.Fatal("independent resource or setting lost", string(readBytes(t, project)))
				}
			})
		}
	}
}

func TestALE34ProjectOnlyConfiguredResourceKeepsLocalControls(t *testing.T) {
	fx := piFixture(t)
	target := codexSkill(t, filepath.Join(fx.base, "allowed/project-only"), "target", "project-only")
	project := filepath.Join(fx.project, ".pi/settings.json")
	data, _ := json.Marshal(map[string]any{"skills": []string{target}, "keep": true})
	writeFile(t, project, data, 0600)
	id := piSourceAt(t, fx, target).ID
	global := filepath.Join(fx.home, ".pi/agent/settings.json")
	globalBytes := readBytes(t, global)
	content := readBytes(t, target)
	piApply(t, fx, "project", "disable", id)
	piApply(t, fx, "project", "inherit", id)
	if piSourceAt(t, fx, target).Availability.State != "available" {
		t.Fatal("project-local default was not restored")
	}
	doc := piSettingsValues(t, project)
	var entries []string
	if err := json.Unmarshal(doc["skills"], &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0] != target || string(doc["keep"]) != "true" || !bytes.Equal(globalBytes, readBytes(t, global)) || !bytes.Equal(content, readBytes(t, target)) {
		t.Fatal("legitimate project discovery or unrelated state changed")
	}
}
