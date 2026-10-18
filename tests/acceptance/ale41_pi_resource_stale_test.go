package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE41PiResourceRelationshipStalesConsent(t *testing.T) {
	for _, declaration := range []string{"skills", "skill*"} {
		t.Run(declaration, func(t *testing.T) {
			fx, _ := codexFixture(t)
			pkg := filepath.Join(fx.base, "allowed", "package")
			target := filepath.Dir(codexSkill(t, filepath.Join(fx.base, "allowed", "external"), "target", "relationship-target"))
			claudeSettings(t, filepath.Join(pkg, "package.json"), map[string]any{"pi": map[string]any{"skills": []string{declaration}}})
			claudeSettings(t, filepath.Join(fx.home, ".pi/agent/settings.json"), map[string]any{"packages": []string{pkg}})
			link := filepath.Join(pkg, "skills")
			if err := os.Symlink(filepath.Dir(target), link); err != nil {
				t.Fatal(err)
			}
			r := fx.run(t, "list", "--root", target, "--json")
			assertExit(t, r, 0)
			var view collection.View
			decodeJSON(t, r.stdout, &view)
			if len(view.Sources) != 1 {
				t.Fatal(view.Sources)
			}
			id := string(view.Sources[0].ID)
			args := []string{"delete", "--root", target, "--project", fx.project, "--json"}
			r = changeCommand(t, fx, "", append(args, id)...)
			assertExit(t, r, 1)
			var preview collection.DeleteView
			decodeJSON(t, r.stdout, &preview)
			if preview.State != "ready" {
				t.Fatal(preview.State, preview.Reason)
			}
			if err := os.Remove(link); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(pkg, link); err != nil {
				t.Fatal(err)
			}
			before := treeManifest(t, fx.base)
			r = changeCommand(t, fx, "", append(append(args, "--confirm", preview.Confirmation), id)...)
			assertExit(t, r, 1)
			var outcome collection.DeleteOutcome
			decodeJSON(t, r.stdout, &outcome)
			if outcome.State != "stale" || len(outcome.Removed) != 0 {
				t.Fatal(outcome.State, outcome.Reason, outcome.Removed)
			}
			assertTreeUnchanged(t, before, treeManifest(t, fx.base))
			writeEvidence(t, "pi-relationship-stale/"+declaration+".json", r.stdout)
		})
	}
}

func TestALE41PiDeleteScopeCopy(t *testing.T) {
	fx, _ := codexFixture(t)
	target := filepath.Join(fx.home, ".pi/agent/skills/standalone.md")
	writeFile(t, target, []byte("---\nname: scope-copy\ndescription: Inert fixture.\n---\nBody.\n"), 0600)
	listed := piList(t, fx, 0)
	id := codexNamed(t, listed, "scope-copy").ID
	args := []string{"delete", "--agent", "pi", "--project", fx.project}
	r := changeCommand(t, fx, "", append(append([]string{}, args...), id)...)
	assertExit(t, r, 1)
	assertPiDeleteScopeCopy(t, string(r.stdout))
	writeEvidence(t, "pi-scope-copy/cli.txt", r.stdout)
	r = changeCommand(t, fx, "", append(append(args, "--json"), id)...)
	assertExit(t, r, 1)
	var view collection.DeleteView
	decodeJSON(t, r.stdout, &view)
	if view.State != "ready" {
		t.Fatal(view.State, view.Reason)
	}
	found := false
	for _, consumer := range view.Consumers {
		if consumer.Application != "pi" {
			continue
		}
		if consumer.Scope.Agent == nil || consumer.Availability == nil {
			t.Fatal("missing Pi consumer context")
		}
		assertPiDeleteScopeCopy(t, strings.Join(consumer.Scope.Agent.Gaps, "\n"))
		for _, capability := range consumer.Availability.Capabilities {
			if capability.Operation == "delete" {
				found = true
				if capability.State != "unsupported" {
					t.Fatal("native application delete control changed", capability)
				}
			}
		}
	}
	if !found {
		t.Fatal("native application delete control missing")
	}
	writeEvidence(t, "pi-scope-copy/cli.json", r.stdout)
}

func assertPiDeleteScopeCopy(t *testing.T, text string) {
	t.Helper()
	for _, expected := range []string{"Standalone Markdown resources can be read; bundle comparison has incomplete coverage.", "Package writers are unavailable; exact resource controls require a confirmed preview."} {
		if !paneContains(text, expected) {
			t.Fatal("Pi consent omitted scope limit", expected)
		}
	}
	for _, obsolete := range []string{"bundle comparison has incomplete coverage and deletion is unsupported.", "Package writers and deletion are unavailable;"} {
		if paneContains(text, obsolete) {
			t.Fatal("Pi consent contains obsolete scope limit", obsolete)
		}
	}
}
