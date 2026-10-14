package acceptance_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewALE31SingletonHasNoRelatedIndependentSource(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "single", "single")
	before := treeManifest(t, fx.base)
	defer func() { assertTreeUnchanged(t, before, treeManifest(t, fx.base)) }()
	list := hermesList(t, fx, 0, nil)
	if len(list.Sources) != 1 {
		t.Fatalf("sources = %d; want 1", len(list.Sources))
	}
	selected := codexNamed(t, list, "single")
	result := hermesRun(t, fx, nil, "explain", "--agent", "hermes", "--project", fx.project, selected.ID)
	assertExit(t, result, 0)
	actual := strings.Count(string(result.stdout), "Related independent source:")
	if actual != 0 {
		t.Fatalf("singleton explanation has %d related independent sources; want 0\n%s", actual, result.stdout)
	}
}

func TestALE31RelatedIndependentCopiesPreserved(t *testing.T) {
	fx, profile := hermesFixture(t)
	codexSkill(t, filepath.Join(profile, "skills"), "first", "collision")
	codexSkill(t, filepath.Join(profile, "shared"), "second", "collision")
	before := treeManifest(t, fx.base)
	list := hermesList(t, fx, 0, nil)
	if len(list.Sources) != 2 {
		t.Fatalf("sources %d want 2", len(list.Sources))
	}
	for _, source := range list.Sources {
		related := source.Availability.Precedence.RelatedSources
		if len(related) != 1 || related[0] == source.ID {
			t.Fatalf("independent relation %+v for %s", related, source.ID)
		}
		if related[0] != list.Sources[0].ID && related[0] != list.Sources[1].ID {
			t.Fatal("relation lost independent copy")
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))
}
