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

func TestALE86RecoveryJournalEncodedLimit(t *testing.T) {
	t.Run("oversized-plan-refuses-before-effects", func(t *testing.T) {
		fx, sourcePath, _, draftPath, preview := journalLimitFixture(t, strings.Repeat("R", 5<<20), "journal-size/large")
		if preview.State != "ready" {
			t.Fatalf("large reason did not reach the apply boundary: state=%s failure=%+v", preview.State, preview.Failure)
		}
		before := treeManifest(t, fx.base)
		apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
		writeEvidence(t, "journal-size/large-apply.json", apply.stdout)
		var outcome collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &outcome)
		if apply.code != 1 || outcome.State != "refused" || outcome.Failure == nil || outcome.Failure.Code != "journal-preflight" || !strings.Contains(outcome.Failure.Message, "4 MiB encoded limit") || outcome.RecoveryID != "" {
			t.Fatalf("oversized encoded journal did not refuse before apply: exit=%d state=%s failure=%+v recovery_id=%q", apply.code, outcome.State, outcome.Failure, outcome.RecoveryID)
		}
		if !bytes.Equal(before, treeManifest(t, fx.base)) {
			t.Fatal("oversized recovery journal changed fixture files before refusal")
		}
		planDir := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", preview.RecoveryID)
		if _, err := os.Stat(planDir); !os.IsNotExist(err) {
			t.Fatalf("oversized recovery plan directory remained after refusal: %v", err)
		}
		if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
			t.Fatalf("source did not remain at its original path: %v", err)
		}
	})

	t.Run("small-plan-applies-and-recovers", func(t *testing.T) {
		fx, _, library, draftPath, preview := journalLimitFixture(t, "A bounded fixture reason for Codex.", "journal-size/small")
		piRoot := filepath.Join(fx.home, ".pi", "agent", "skills")
		codexRoot := filepath.Join(fx.home, ".codex", "skills")
		beforePi := treeManifest(t, piRoot)
		beforeCodex := treeManifest(t, codexRoot)
		beforeLibrary := treeManifest(t, library)
		apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
		writeEvidence(t, "journal-size/small-apply.json", apply.stdout)
		assertExit(t, apply, 0)
		var applied collection.ConsolidationApplyOutcome
		decodeJSON(t, apply.stdout, &applied)
		if applied.State != "applied" || !applied.Verified || applied.RecoveryID == "" {
			t.Fatalf("small recovery journal did not apply: state=%s verified=%t recovery_id=%q", applied.State, applied.Verified, applied.RecoveryID)
		}
		journalPath := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", applied.RecoveryID, "record.json")
		record, err := os.ReadFile(journalPath)
		if err != nil || len(record) > 4<<20 {
			t.Fatalf("applied journal is not within its reader bound: bytes=%d err=%v", len(record), err)
		}
		var journalState struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(record, &journalState); err != nil || journalState.State != "applied" {
			t.Fatalf("persisted journal state is unreadable: state=%q err=%v", journalState.State, err)
		}
		writeEvidence(t, "journal-size/small-record.json", record)

		recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", applied.RecoveryID, "--json")
		writeEvidence(t, "journal-size/small-recovery-preview.json", recoveryPreviewResult.stdout)
		assertExit(t, recoveryPreviewResult, 0)
		var recoveryPreview collection.ConsolidationPreview
		decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
		if recoveryPreview.State != "ready" || recoveryPreview.Confirmation == "" {
			t.Fatalf("bounded applied journal was not readable for recovery: state=%s failure=%+v", recoveryPreview.State, recoveryPreview.Failure)
		}
		recover := changeCommand(t, fx, "", "consolidate", "recover", "--backup", applied.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
		writeEvidence(t, "journal-size/small-recovery-apply.json", recover.stdout)
		assertExit(t, recover, 0)
		var recovered collection.ConsolidationApplyOutcome
		decodeJSON(t, recover.stdout, &recovered)
		if recovered.State != "recovered" || !recovered.Verified {
			t.Fatalf("public recovery did not finish: state=%s verified=%t failure=%+v", recovered.State, recovered.Verified, recovered.Failure)
		}
		if !bytes.Equal(beforePi, treeManifest(t, piRoot)) || !bytes.Equal(beforeCodex, treeManifest(t, codexRoot)) || !bytes.Equal(beforeLibrary, treeManifest(t, library)) {
			t.Fatal("public recovery did not restore the original Pi, Codex, and library trees")
		}
	})
}

func journalLimitFixture(t *testing.T, reason, evidence string) (fixture, string, string, string, collection.ConsolidationPreview) {
	t.Helper()
	fx, sourcePath, library := recoveryFixture(t)
	if err := os.MkdirAll(library, 0o700); err != nil {
		t.Fatal(err)
	}
	inventory := recoveryScan(t, fx, library, evidence+"-scan.json")
	draft := recoveryDraft(t, inventory, sourcePath)
	found := false
	for i := range draft.Choices {
		for j := range draft.Choices[i].Destinations {
			if draft.Choices[i].Destinations[j].Application == "codex" && draft.Choices[i].Action == "share" {
				draft.Choices[i].Destinations[j].Reason = reason
				found = true
			}
		}
	}
	if !found {
		t.Fatal("fixture draft has no Codex destination reason")
	}
	draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "journal-size-draft.json")
	writeConsolidationDraft(t, draftPath, draft)
	previewResult := fx.run(t, "consolidate", "preview", "--draft", draftPath, "--json")
	writeEvidence(t, evidence+"-preview.json", previewResult.stdout)
	assertExit(t, previewResult, 0)
	var preview collection.ConsolidationPreview
	decodeJSON(t, previewResult.stdout, &preview)
	if preview.State != "ready" || preview.Confirmation == "" || preview.RecoveryID == "" {
		t.Fatalf("fixture preview was not ready: state=%s failure=%+v", preview.State, preview.Failure)
	}
	return fx, sourcePath, library, draftPath, preview
}
