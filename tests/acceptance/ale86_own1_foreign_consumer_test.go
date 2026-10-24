package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestOWN1SelectedPersonalSourcesAreNotOwnedByForeignCodeAFRoots(t *testing.T) {
	fx := newFixture(t)
	// Keep the fixture's home-fallback canary inside a complete native bundle so
	// it cannot make the selected inventory incomplete for unrelated reasons.
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n",
	), 0o644)
	// In a normal login, both native OpenCode and CodeAF derive this root from
	// HOME/.config. Keep the fixture spelling aligned so their effective root
	// paths physically coincide without launching either application.
	fx.xdgConfig = filepath.Join(fx.home, ".config")
	if err := os.MkdirAll(fx.xdgConfig, 0o755); err != nil {
		t.Fatal(err)
	}

	roots := map[string]string{
		"codex":    filepath.Join(fx.home, ".codex", "skills"),
		"pi":       filepath.Join(fx.home, ".pi", "agent", "skills"),
		"opencode": filepath.Join(fx.xdgConfig, "opencode", "skills"),
	}
	names := map[string]string{
		"codex":    "own1-codex-source",
		"pi":       "own1-pi-source",
		"opencode": "own1-opencode-source",
	}
	paths := make(map[string]string, len(roots))
	for app, root := range roots {
		bundle := filepath.Join(root, names[app])
		writeFile(t, filepath.Join(bundle, "SKILL.md"), []byte(
			"---\nname: "+names[app]+"\ndescription: ordinary portable fixture source\n---\nInstructions for ownership verification.\n",
		), 0o644)
		paths[app] = bundle
	}

	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	before := treeManifest(t, fx.base)
	result := fx.run(t, "consolidate", "scan", "--app", "codex", "--app", "pi", "--app", "opencode", "--library", library, "--json")
	writeEvidence(t, "ownership/own1-selected-app-scan.json", result.stdout)
	assertExit(t, result, 0)
	if len(result.stderr) != 0 {
		t.Fatalf("machine stderr: %s", result.stderr)
	}

	var inventory collection.ConsolidationInventory
	decodeJSON(t, result.stdout, &inventory)
	if got := strings.Join(inventory.Scope.Applications, ","); got != "codex,opencode,pi" {
		t.Fatalf("selected applications = %q, want codex,opencode,pi; full JSON: ownership/own1-selected-app-scan.json", got)
	}
	var codeAFSelected, codeAFPresent bool
	for _, app := range inventory.Applications {
		if app.Application == "codeaf" {
			codeAFPresent = true
			codeAFSelected = app.Selected
			if app.Selected {
				t.Fatalf("CodeAF entered direct selection; full JSON: ownership/own1-selected-app-scan.json")
			}
		}
	}
	if !codeAFPresent || codeAFSelected {
		t.Fatal("CodeAF must remain outside the direct source/destination selection")
	}

	selected := make(map[string]bool, len(inventory.Scope.Applications))
	for _, app := range inventory.Scope.Applications {
		selected[app] = true
	}
	var expectedFailures []string
	var target *collection.ConsolidationSource
	draft := collection.ConsolidationDraft{
		SchemaVersion: 1,
		Scope:         inventory.Scope,
		ScanID:        inventory.ScanID,
		Choices:       []collection.ConsolidationChoice{},
	}
	for app, wantPath := range paths {
		var found *collection.ConsolidationSource
		for index := range inventory.Sources {
			candidate := &inventory.Sources[index]
			if candidate.SourcePath == wantPath {
				found = candidate
				break
			}
		}
		if found == nil {
			t.Fatalf("selected %s source at %s missing from scan; full JSON: ownership/own1-selected-app-scan.json", app, wantPath)
		}
		if !found.Eligible {
			expectedFailures = append(expectedFailures, app+" source should be eligible, got "+found.RelocationScope+": "+strings.Join(found.BlockedReasons, "; "))
		} else if found.RelocationScope != "selected-personal-source" && found.RelocationScope != "selected-personal-root-alias" {
			expectedFailures = append(expectedFailures, app+" source has unexpected relocation scope "+found.RelocationScope)
		}
		if app == "codex" || app == "opencode" {
			var codeAFReference bool
			for _, reference := range found.KnownReferences {
				if reference.Application == "codeaf" {
					codeAFReference = true
				}
			}
			if !codeAFReference {
				expectedFailures = append(expectedFailures, "expected shared-root CodeAF consumer evidence for selected "+app+" source")
			}
		}
		if app == "codex" {
			target = found
		}
	}
	if target == nil {
		t.Fatal("Codex target source absent; full JSON: ownership/own1-selected-app-scan.json")
	}
	for _, source := range inventory.Sources {
		if source.ID == target.ID {
			choice := collection.ConsolidationChoice{
				SourceID:          source.ID,
				SourcePath:        source.SourcePath,
				BundleFingerprint: source.BundleFingerprint,
				Action:            "share",
				CanonicalName:     source.Name,
				Destinations:      []collection.ConsolidationDestinationChoice{},
			}
			for _, application := range inventory.Scope.Applications {
				assessment := collection.ConsolidationDestinationChoice{
					Application: application,
					Portability: "compatible",
					Reason:      "fixture instructions require no application-specific capability",
				}
				for _, audit := range inventory.DestinationAudits {
					if audit.Application != application {
						continue
					}
					for _, consumer := range audit.PotentialConsumers {
						if selected[consumer.Application] {
							continue
						}
						assessment.AcknowledgedConsumers = append(assessment.AcknowledgedConsumers, collection.ConsolidationConsumerAcknowledgment{
							Application:  consumer.Application,
							EvidenceIDs:  []string{consumer.EvidenceID},
							UserApproved: true,
						})
					}
				}
				choice.Destinations = append(choice.Destinations, assessment)
			}
			draft.Choices = append(draft.Choices, choice)
			continue
		}
		if source.Eligible {
			draft.Choices = append(draft.Choices, collection.ConsolidationChoice{
				SourceID: source.ID, SourcePath: source.SourcePath,
				BundleFingerprint: source.BundleFingerprint, Action: "keep",
			})
		}
	}
	assertTreeUnchanged(t, before, treeManifest(t, fx.base))

	draftBytes, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "own1-draft.json")
	writeFile(t, draftPath, draftBytes, 0o600)
	writeEvidence(t, "ownership/own1-acknowledged-draft.json", draftBytes)
	previewBefore := treeManifest(t, fx.base)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "ownership/own1-acknowledged-preview.json", previewResult.stdout)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" || preview.ExitCode != 0 || preview.Failure != nil {
		failure := "none"
		if preview.Failure != nil {
			failure = preview.Failure.Code + ": " + preview.Failure.Message
		}
		expectedFailures = append(expectedFailures, "exact-acknowledgment preview should be ready, got state="+preview.State+" failure="+failure)
	}
	assertTreeUnchanged(t, previewBefore, treeManifest(t, fx.base))
	if len(expectedFailures) != 0 {
		t.Fatalf("OWN-1 requirement failures: %s; full scan/draft/preview: ownership/own1-selected-app-scan.json, ownership/own1-acknowledged-draft.json, ownership/own1-acknowledged-preview.json", strings.Join(expectedFailures, " | "))
	}
}
