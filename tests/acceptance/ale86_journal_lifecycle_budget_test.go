package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86RecoveryJournalLifecycleBudget(t *testing.T) {
	// This encoded reason leaves room for the prepared/apply record on c81 but
	// not the larger link-removal plus source-restore intent states.
	const nearLimitReasonBytes = 4_188_800
	fx, sourcePath, _, draftPath, preview := journalLimitFixture(t, strings.Repeat("R", nearLimitReasonBytes), "journal-lifecycle/near")
	before := treeManifest(t, fx.base)

	apply := changeCommand(t, fx, "", "consolidate", "apply", "--draft", draftPath, "--confirm", preview.Confirmation, "--json")
	writeEvidence(t, "journal-lifecycle/near-apply.json", apply.stdout)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, apply.stdout, &applied)
	failureCode := ""
	if applied.Failure != nil {
		failureCode = applied.Failure.Code
	}
	t.Logf("public near-limit apply: exit=%d state=%s recovery_id_present=%t failure_code=%s", apply.code, applied.State, applied.RecoveryID != "", failureCode)
	if apply.code == 1 && applied.State == "refused" && applied.Failure != nil && applied.Failure.Code == "journal-preflight" && applied.RecoveryID == "" {
		if !bytes.Equal(before, treeManifest(t, fx.base)) {
			t.Fatal("lifecycle-budget refusal changed fixture files")
		}
		planDir := filepath.Join(fx.xdgData, "skillglass", "consolidation", "backups", preview.RecoveryID)
		if _, err := os.Stat(planDir); !os.IsNotExist(err) {
			t.Fatalf("lifecycle-budget refusal retained a recovery plan: %v", err)
		}
		return
	}
	if applied.RecoveryID == "" || (applied.State != "applied" && applied.State != "partial") {
		t.Fatalf("large lifecycle case neither refused before effects nor retained recoverable state: exit=%d state=%s id=%q failure=%+v", apply.code, applied.State, applied.RecoveryID, applied.Failure)
	}

	recoveryPreviewResult := fx.run(t, "consolidate", "recover", "--backup", applied.RecoveryID, "--json")
	writeEvidence(t, "journal-lifecycle/near-recovery-preview.json", recoveryPreviewResult.stdout)
	assertExit(t, recoveryPreviewResult, 0)
	var recoveryPreview collection.ConsolidationPreview
	decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
	if recoveryPreview.State != "ready" || recoveryPreview.Confirmation == "" {
		t.Fatalf("accepted near-limit apply has no ready recovery: state=%s failure=%+v", recoveryPreview.State, recoveryPreview.Failure)
	}

	recover := changeCommand(t, fx, "", "consolidate", "recover", "--backup", applied.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
	writeEvidence(t, "journal-lifecycle/near-recovery-apply.json", recover.stdout)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, recover.stdout, &recovered)
	if recover.code != 0 || recovered.State != "recovered" || !recovered.Verified {
		t.Fatalf("accepted near-limit apply could not recover: exit=%d state=%s reason=%s failure=%+v", recover.code, recovered.State, recovered.Reason, recovered.Failure)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("public recovery did not restore the exact original fixture tree")
	}
	if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
		t.Fatalf("recovery did not restore the original skill source: %v", err)
	}
}
