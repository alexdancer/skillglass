package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86CodexDisabledManifestChoiceStaysDisabledAndSettingsStayUntouched(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte(
		"---\nname: home-canary\ndescription: complete fixture control\n---\nFixture only.\n",
	), 0o644)
	personalRoot := filepath.Join(fx.home, ".codex", "skills")
	manifest := filepath.Join(personalRoot, "disabled-fixture", "SKILL.md")
	writeFile(t, manifest, []byte("---\nname: disabled-fixture\ndescription: inert disabled relocation fixture\n---\nThis source stays disabled.\n"), 0o644)
	config := []byte("# preserve unrelated configuration\n\n[[skills.config]]\npath = " + strconv.Quote(manifest) + "\nenabled = false\n")
	configPath := filepath.Join(fx.home, ".codex", "config.toml")
	writeFile(t, configPath, config, 0o600)
	configBefore := readBytes(t, configPath)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")
	scan := fx.run(t, "consolidate", "scan", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "consolidation/disabled-choice-scan-before.json", scan.stdout)
	assertExit(t, scan, 0)
	var inventory collection.ConsolidationInventory
	decodeJSON(t, scan.stdout, &inventory)
	source := sourceAtPath(t, inventory, filepath.Dir(manifest))
	if !source.Eligible || source.Availability["codex"].State != collection.AvailabilityDisabled {
		t.Fatalf("exact disabled Codex selector should retain an explicitly reviewable source: %#v", source)
	}
	if source.Availability["codex"].Evidence == nil || source.Availability["codex"].Evidence.Rule != "configured-availability" || source.Availability["codex"].Evidence.AgentVersion != "codex-cli 0.155.1" || len(source.RelocationProofs) != 1 || source.RelocationProofs[0].Profile != "Codex 0.162.1" {
		t.Fatalf("disabled move must be bound to separate adapter and native relocation evidence: availability=%#v proofs=%#v", source.Availability["codex"], source.RelocationProofs)
	}
	draft := edit1Draft(inventory, map[collection.SourceID]string{source.ID: "share"})
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "disabled-choice.json")
	edit1WriteDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, "consolidation/disabled-choice-preview.json", previewResult.stdout)
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" {
		t.Fatalf("exactly evidenced disabled source did not produce a ready plan: %#v", preview)
	}
	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	writeEvidence(t, "consolidation/disabled-choice-apply.json", apply.stdout)
	assertExit(t, apply, 0)
	var outcome collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &outcome)
	if outcome.State != "applied" || !outcome.Verified {
		t.Fatalf("approved disabled relocation did not verify: %#v", outcome)
	}
	if !bytes.Equal(configBefore, readBytes(t, configPath)) {
		t.Fatal("consolidation changed Codex configuration while relocating a disabled source")
	}
	canonical := filepath.Join(library, source.BundleFingerprint)
	if got, err := os.ReadFile(filepath.Join(canonical, "SKILL.md")); err != nil || !bytes.Equal(got, readBytes(t, manifest)) {
		t.Fatalf("relocated source bytes changed: %v", err)
	}
	post := fx.run(t, "consolidate", "scan", "--app", "codex", "--library", library, "--json")
	writeEvidence(t, "consolidation/disabled-choice-scan-after.json", post.stdout)
	assertExit(t, post, 0)
	var postInventory collection.ConsolidationInventory
	decodeJSON(t, post.stdout, &postInventory)
	canonicalSource := sourceAtPath(t, postInventory, canonical)
	if canonicalSource.Availability["codex"].State != collection.AvailabilityDisabled || !bytes.Equal(configBefore, readBytes(t, configPath)) {
		t.Fatalf("adapter-evaluated disabled selector or config changed after relocation: state=%s", canonicalSource.Availability["codex"].State)
	}
}
