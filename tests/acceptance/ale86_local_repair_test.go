package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexdancer/skillglass/internal/collection"
)

func TestALE86HermesOrdinaryAncestorAliasPreservesExplicitProfile(t *testing.T) {
	fx, canonicalProfile := hermesFixture(t)
	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
	profileSourcePath := filepath.Join(canonicalProfile, "skills", "profile-canary")
	profileManifest := []byte("---\nname: profile-canary\ndescription: direct home profile fixture\n---\nKeep this profile content.\n")
	writeFile(t, filepath.Join(profileSourcePath, "SKILL.md"), profileManifest, 0o644)
	piSourcePath := filepath.Join(fx.home, ".pi", "agent", "skills", "pi-canary")
	writeFile(t, filepath.Join(piSourcePath, "SKILL.md"), []byte("---\nname: pi-canary\ndescription: Hermes destination fixture\n---\nKeep this Pi source.\n"), 0o644)
	library := filepath.Join(fx.xdgData, "skillglass", "skills")

	aliasHome := strings.TrimPrefix(fx.home, "/private")
	if aliasHome == fx.home || !strings.HasPrefix(fx.home, "/private/var/") {
		t.Fatalf("fixture HOME does not exercise the macOS /var ancestor alias: canonical=%q alias=%q", fx.home, aliasHome)
	}
	aliasProfile := filepath.Join(aliasHome, ".hermes", "profiles", "research")
	resolvedCanonicalHome, err := filepath.EvalSymlinks(fx.home)
	if err != nil {
		t.Fatal(err)
	}
	resolvedAliasHome, err := filepath.EvalSymlinks(aliasHome)
	if err != nil {
		t.Fatal(err)
	}
	canonicalHomeInfo, err := os.Stat(fx.home)
	if err != nil {
		t.Fatal(err)
	}
	aliasHomeInfo, err := os.Stat(aliasHome)
	if err != nil {
		t.Fatal(err)
	}
	canonicalProfileInfo, err := os.Stat(canonicalProfile)
	if err != nil {
		t.Fatal(err)
	}
	aliasProfileInfo, err := os.Stat(aliasProfile)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedCanonicalHome != resolvedAliasHome || !os.SameFile(canonicalHomeInfo, aliasHomeInfo) || !os.SameFile(canonicalProfileInfo, aliasProfileInfo) {
		t.Fatalf("fixture does not prove an ordinary ancestor alias: canonical_home=%q alias_home=%q resolved_canonical=%q resolved_alias=%q same_home=%t same_profile=%t", fx.home, aliasHome, resolvedCanonicalHome, resolvedAliasHome, os.SameFile(canonicalHomeInfo, aliasHomeInfo), os.SameFile(canonicalProfileInfo, aliasProfileInfo))
	}

	apps := []string{"consolidate", "scan", "--app", "hermes", "--app", "pi", "--library", library, "--json"}
	beforeCanonicalScan := treeManifest(t, fx.base)
	canonicalScan := runALE86HermesAliasCLI(t, fx, []string{"HOME=" + fx.home, "HERMES_HOME=" + canonicalProfile}, apps...)
	writeEvidence(t, "hermes-ancestor-alias/canonical-scan.json", canonicalScan.stdout)
	afterCanonicalScan := treeManifest(t, fx.base)
	writeEvidence(t, "hermes-ancestor-alias/canonical-scan-tree-before.json", beforeCanonicalScan)
	writeEvidence(t, "hermes-ancestor-alias/canonical-scan-tree-after.json", afterCanonicalScan)
	assertTreeUnchanged(t, beforeCanonicalScan, afterCanonicalScan)
	assertExit(t, canonicalScan, 0)
	var canonicalInventory collection.ConsolidationInventory
	decodeJSON(t, canonicalScan.stdout, &canonicalInventory)
	canonicalSource := sourceAtPath(t, canonicalInventory, profileSourcePath)
	canonicalProof := hermesAliasProfile(canonicalInventory)
	if canonicalInventory.SelectedCoverage != collection.Complete || !canonicalSource.Eligible || canonicalProof == nil || canonicalProof.ID != "hermes-explicit-profile-index-alias-v1" {
		t.Fatalf("canonical control failed: selected_coverage=%s eligible=%t profile=%#v", canonicalInventory.SelectedCoverage, canonicalSource.Eligible, canonicalProof)
	}

	canonicalDraft := buildConsolidationDraft(canonicalInventory, map[collection.SourceID]string{canonicalSource.ID: "share"})
	canonicalChoice, ok := consolidationChoiceForSource(canonicalDraft, canonicalSource.ID)
	if !ok {
		t.Fatal("canonical source was not represented in a share draft")
	}
	canonicalDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-ancestor-canonical.json")
	writeConsolidationDraft(t, canonicalDraftPath, canonicalDraft)
	writeEvidence(t, "hermes-ancestor-alias/canonical-draft.json", mustMarshalJSON(t, canonicalDraft))
	beforeCanonicalPreview := treeManifest(t, fx.base)
	canonicalPreviewResult := runALE86HermesAliasCLI(t, fx, []string{"HOME=" + fx.home, "HERMES_HOME=" + canonicalProfile}, "consolidate", "preview", "--draft", canonicalDraftPath, "--json")
	writeEvidence(t, "hermes-ancestor-alias/canonical-preview.json", canonicalPreviewResult.stdout)
	var canonicalPreview collection.ConsolidationPreview
	decodeJSON(t, canonicalPreviewResult.stdout, &canonicalPreview)
	afterCanonicalPreview := treeManifest(t, fx.base)
	writeEvidence(t, "hermes-ancestor-alias/canonical-preview-tree-before.json", beforeCanonicalPreview)
	writeEvidence(t, "hermes-ancestor-alias/canonical-preview-tree-after.json", afterCanonicalPreview)
	assertTreeUnchanged(t, beforeCanonicalPreview, afterCanonicalPreview)
	if canonicalPreviewResult.code != 0 || canonicalPreview.State != "ready" || canonicalPreview.Confirmation == "" {
		t.Fatalf("canonical profile preview control was not ready: exit=%d state=%s failure=%#v", canonicalPreviewResult.code, canonicalPreview.State, canonicalPreview.Failure)
	}

	aliasEnv := []string{"HOME=" + aliasHome, "HERMES_HOME=" + aliasProfile}
	beforeAliasScan := treeManifest(t, fx.base)
	aliasScan := runALE86HermesAliasCLI(t, fx, aliasEnv, apps...)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-scan.json", aliasScan.stdout)
	afterAliasScan := treeManifest(t, fx.base)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-scan-tree-before.json", beforeAliasScan)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-scan-tree-after.json", afterAliasScan)
	assertTreeUnchanged(t, beforeAliasScan, afterAliasScan)
	assertExit(t, aliasScan, 0)
	assertALE86LaunchEnv(t, aliasScan, "HOME", aliasHome)
	assertALE86LaunchEnv(t, aliasScan, "HERMES_HOME", aliasProfile)
	var aliasInventory collection.ConsolidationInventory
	decodeJSON(t, aliasScan.stdout, &aliasInventory)
	aliasProof := hermesAliasProfile(aliasInventory)
	aliasSource := sourceAtPhysicalPath(t, aliasInventory, profileSourcePath)
	piSource := sourceAtPath(t, aliasInventory, piSourcePath)
	if aliasInventory.SelectedCoverage != collection.Complete {
		t.Fatalf("ancestor-alias scan coverage=%s, want complete", aliasInventory.SelectedCoverage)
	}
	if !piSource.Eligible {
		t.Fatalf("Pi destination fixture source is not eligible: %#v", piSource.BlockedReasons)
	}

	expectedDestinationRoots := map[string]string{
		"hermes": filepath.Join(resolvedCanonicalHome, ".hermes", "profiles", "research", "skills"),
		"pi":     filepath.Join(resolvedCanonicalHome, ".pi", "agent", "skills"),
	}
	for _, app := range aliasInventory.Applications {
		if expected, selected := expectedDestinationRoots[app.Application]; selected && filepath.Clean(app.DestinationRoot) != filepath.Clean(expected) {
			t.Fatalf("%s destination root was not normalized through the ordinary HOME alias: got=%q want=%q", app.Application, app.DestinationRoot, expected)
		}
	}
	for application, expected := range expectedDestinationRoots {
		found := false
		for _, audit := range aliasInventory.DestinationAudits {
			if audit.Application != application {
				continue
			}
			found = true
			if filepath.Clean(audit.DestinationRoot) != filepath.Clean(expected) || len(audit.Refusals) != 0 {
				t.Fatalf("%s destination audit did not use the canonical parent while preserving no-follow checks: root=%q refusals=%#v want=%q", application, audit.DestinationRoot, audit.Refusals, expected)
			}
		}
		if !found {
			t.Fatalf("%s destination audit was missing", application)
		}
	}

	aliasDraft := buildConsolidationDraft(aliasInventory, map[collection.SourceID]string{aliasSource.ID: "share"})
	if aliasChoice, found := consolidationChoiceForSource(aliasDraft, aliasSource.ID); found {
		aliasChoice.Action = "share"
		for index := range aliasDraft.Choices {
			if aliasDraft.Choices[index].SourceID == aliasSource.ID {
				aliasDraft.Choices[index] = aliasChoice
			}
		}
	} else {
		forcedChoice := canonicalChoice
		forcedChoice.SourceID = aliasSource.ID
		forcedChoice.SourcePath = aliasSource.SourcePath
		forcedChoice.BundleFingerprint = aliasSource.BundleFingerprint
		forcedChoice.CanonicalName = aliasSource.Name
		aliasDraft.Choices = append(aliasDraft.Choices, forcedChoice)
	}
	aliasDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-ancestor-var-alias.json")
	writeConsolidationDraft(t, aliasDraftPath, aliasDraft)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-draft.json", mustMarshalJSON(t, aliasDraft))
	beforeAliasPreview := treeManifest(t, fx.base)
	aliasPreviewResult := runALE86HermesAliasCLI(t, fx, aliasEnv, "consolidate", "preview", "--draft", aliasDraftPath, "--json")
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-preview.json", aliasPreviewResult.stdout)
	var aliasPreview collection.ConsolidationPreview
	decodeJSON(t, aliasPreviewResult.stdout, &aliasPreview)
	afterAliasPreview := treeManifest(t, fx.base)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-preview-tree-before.json", beforeAliasPreview)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-preview-tree-after.json", afterAliasPreview)
	assertTreeUnchanged(t, beforeAliasPreview, afterAliasPreview)
	if aliasPreviewResult.code != 0 || aliasPreview.State != "ready" || aliasPreview.Confirmation == "" {
		t.Fatalf("direct HOME Hermes profile reached through the ordinary /var ancestor alias should have a ready preview: profile=%q source_eligible=%t exit=%d state=%s failure=%#v", hermesAliasProfileID(aliasProof), aliasSource.Eligible, aliasPreviewResult.code, aliasPreview.State, aliasPreview.Failure)
	}

	var canonicalPath string
	for _, effect := range aliasPreview.Effects {
		if effect.Action == "share" && effect.SourceID == aliasSource.ID {
			canonicalPath = effect.CanonicalPath
			break
		}
	}
	if canonicalPath == "" {
		t.Fatal("alias preview omitted the shared canonical target")
	}
	aliasApplyResult := runALE86HermesAliasChangeCLI(t, fx, aliasEnv, "consolidate", "apply", "--draft", aliasDraftPath, "--confirm", aliasPreview.Confirmation, "--json")
	assertExit(t, aliasApplyResult, 0)
	var applied collection.ConsolidationApplyOutcome
	decodeJSON(t, aliasApplyResult.stdout, &applied)
	if applied.State != "applied" || !applied.Verified || applied.RecoveryID == "" {
		t.Fatalf("alias HOME apply did not verify the exact native links: %#v", applied)
	}
	piLink := filepath.Join(resolvedCanonicalHome, ".pi", "agent", "skills", "profile-canary")
	for _, path := range []string{profileSourcePath, piLink} {
		target, err := os.Readlink(path)
		if err != nil || filepath.Clean(target) != filepath.Clean(canonicalPath) {
			t.Fatalf("alias HOME writer did not create the canonical link at %s: target=%q err=%v want=%q", path, target, err, canonicalPath)
		}
	}

	rerunBefore := treeManifest(t, fx.base)
	rerunScanResult := runALE86HermesAliasCLI(t, fx, aliasEnv, apps...)
	assertExit(t, rerunScanResult, 0)
	assertTreeUnchanged(t, rerunBefore, treeManifest(t, fx.base))
	var rerunInventory collection.ConsolidationInventory
	decodeJSON(t, rerunScanResult.stdout, &rerunInventory)
	rerunSource := sourceAtPath(t, rerunInventory, canonicalPath)
	rerunDraft := buildConsolidationDraft(rerunInventory, map[collection.SourceID]string{rerunSource.ID: "share"})
	rerunDraftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-ancestor-alias-rerun.json")
	writeConsolidationDraft(t, rerunDraftPath, rerunDraft)
	rerunNoopBefore := treeManifest(t, fx.base)
	rerunPreviewResult := runALE86HermesAliasCLI(t, fx, aliasEnv, "consolidate", "preview", "--draft", rerunDraftPath, "--json")
	assertExit(t, rerunPreviewResult, 0)
	assertTreeUnchanged(t, rerunNoopBefore, treeManifest(t, fx.base))
	var rerunPreview collection.ConsolidationPreview
	decodeJSON(t, rerunPreviewResult.stdout, &rerunPreview)
	if rerunPreview.State != "ready" || rerunPreview.Confirmation == "" {
		t.Fatalf("unchanged alias HOME rerun preview was not ready: %#v", rerunPreview)
	}
	rerunApplyResult := runALE86HermesAliasChangeCLI(t, fx, aliasEnv, "consolidate", "apply", "--draft", rerunDraftPath, "--confirm", rerunPreview.Confirmation, "--json")
	assertExit(t, rerunApplyResult, 0)
	var rerunApply collection.ConsolidationApplyOutcome
	decodeJSON(t, rerunApplyResult.stdout, &rerunApply)
	if rerunApply.State != "unchanged" || !rerunApply.Verified || rerunApply.SelectionUpdated || rerunApply.RecoveryID != "" {
		t.Fatalf("unchanged alias HOME rerun made or reported filesystem changes: %#v", rerunApply)
	}
	assertTreeUnchanged(t, rerunNoopBefore, treeManifest(t, fx.base))
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-rerun-preview.json", rerunPreviewResult.stdout)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-rerun-apply.json", rerunApplyResult.stdout)

	recoveryPreviewResult := runALE86HermesAliasCLI(t, fx, aliasEnv, "consolidate", "recover", "--backup", applied.RecoveryID, "--json")
	assertExit(t, recoveryPreviewResult, 0)
	var recoveryPreview collection.ConsolidationPreview
	decodeJSON(t, recoveryPreviewResult.stdout, &recoveryPreview)
	if recoveryPreview.State != "ready" || recoveryPreview.Confirmation == "" {
		t.Fatalf("alias HOME recovery preview was not ready: %#v", recoveryPreview)
	}
	recoveryResult := runALE86HermesAliasChangeCLI(t, fx, aliasEnv, "consolidate", "recover", "--backup", applied.RecoveryID, "--confirm", recoveryPreview.Confirmation, "--json")
	assertExit(t, recoveryResult, 0)
	var recovered collection.ConsolidationApplyOutcome
	decodeJSON(t, recoveryResult.stdout, &recovered)
	if recovered.State != "recovered" || !recovered.Verified {
		t.Fatalf("alias HOME recovery did not restore the recorded source: %#v", recovered)
	}
	profileInfo, err := os.Lstat(profileSourcePath)
	if err != nil || !profileInfo.IsDir() || profileInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("recovery did not restore a real Hermes source directory: info=%v err=%v", profileInfo, err)
	}
	if got := readBytes(t, filepath.Join(profileSourcePath, "SKILL.md")); !bytes.Equal(got, profileManifest) {
		t.Fatalf("alias HOME recovery changed Hermes source bytes: %q", got)
	}
	if _, err := os.Lstat(piLink); !os.IsNotExist(err) {
		t.Fatalf("alias HOME recovery did not remove the Pi link: %v", err)
	}
	if _, err := os.Lstat(canonicalPath); !os.IsNotExist(err) {
		t.Fatalf("alias HOME recovery left the canonical target behind: %v", err)
	}
	t.Logf("ordinary ancestor alias public roundtrip verified: same_home=%t same_profile=%t HOME_override_reached_child=%t HERMES_HOME_override_reached_child=%t canonical_target=%s recovery_id=%s", os.SameFile(canonicalHomeInfo, aliasHomeInfo), os.SameFile(canonicalProfileInfo, aliasProfileInfo), hasALE86LaunchEnv(aliasApplyResult, "HOME", aliasHome), hasALE86LaunchEnv(aliasApplyResult, "HERMES_HOME", aliasProfile), canonicalPath, applied.RecoveryID)

	// The same helper has a separate default-root branch. Check its exact alias
	// coverage after removing the active-profile marker; this is diagnostic only.
	if err := os.Remove(filepath.Join(fx.home, ".hermes", "active_profile")); err != nil {
		t.Fatal(err)
	}
	defaultSourcePath := filepath.Join(fx.home, ".hermes", "skills", "default-canary")
	writeFile(t, filepath.Join(defaultSourcePath, "SKILL.md"), []byte("---\nname: default-canary\ndescription: observed default Hermes profile\n---\nKeep this default source.\n"), 0o644)
	beforeDefaultScan := treeManifest(t, fx.base)
	defaultScan := runALE86HermesAliasCLI(t, fx, []string{"HOME=" + aliasHome}, apps...)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-default-root-scan.json", defaultScan.stdout)
	assertExit(t, defaultScan, 0)
	assertALE86LaunchEnv(t, defaultScan, "HOME", aliasHome)
	if hasALE86LaunchEnvKey(defaultScan, "HERMES_HOME") {
		t.Fatal("default-root alias control unexpectedly received HERMES_HOME")
	}
	var defaultInventory collection.ConsolidationInventory
	decodeJSON(t, defaultScan.stdout, &defaultInventory)
	defaultSource := sourceAtPhysicalPath(t, defaultInventory, defaultSourcePath)
	defaultProof := hermesAliasProfile(defaultInventory)
	afterDefaultScan := treeManifest(t, fx.base)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-default-root-scan-tree-before.json", beforeDefaultScan)
	writeEvidence(t, "hermes-ancestor-alias/var-ancestor-alias-default-root-scan-tree-after.json", afterDefaultScan)
	assertTreeUnchanged(t, beforeDefaultScan, afterDefaultScan)
	t.Logf("default-root alias coverage: HOME_override_reached_child=%t HERMES_HOME_absent=%t selected_coverage=%s profile=%q source_path=%q source_eligible=%t", hasALE86LaunchEnv(defaultScan, "HOME", aliasHome), !hasALE86LaunchEnvKey(defaultScan, "HERMES_HOME"), defaultInventory.SelectedCoverage, hermesAliasProfileID(defaultProof), defaultSource.SourcePath, defaultSource.Eligible)

	if aliasProof == nil || aliasProof.ID != "hermes-explicit-profile-index-alias-v1" || !aliasSource.Eligible {
		t.Fatalf("direct HOME Hermes profile reached through the ordinary /var ancestor alias must retain its relocation proof: selected_coverage=%s profile=%q source_eligible=%t", aliasInventory.SelectedCoverage, hermesAliasProfileID(aliasProof), aliasSource.Eligible)
	}
	if aliasPreviewResult.code != 0 || aliasPreview.State != "ready" || aliasPreview.Confirmation == "" {
		t.Fatalf("direct HOME Hermes profile reached through the ordinary /var ancestor alias must retain relocation support: selected_coverage=%s profile=%q source_eligible=%t preview_exit=%d preview_state=%s preview_failure=%#v", aliasInventory.SelectedCoverage, hermesAliasProfileID(aliasProof), aliasSource.Eligible, aliasPreviewResult.code, aliasPreview.State, aliasPreview.Failure)
	}
}

func TestALE86HermesProfileAndSkillsLeafAliasesStayUnvalidated(t *testing.T) {
	t.Run("profile root leaf alias", func(t *testing.T) {
		fx, profile := hermesFixture(t)
		writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
		profileTarget := filepath.Join(filepath.Dir(profile), "research-target")
		if err := os.Rename(profile, profileTarget); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(profileTarget, profile); err != nil {
			t.Fatal(err)
		}
		sourcePath := filepath.Join(profileTarget, "skills", "profile-leaf-canary")
		writeFile(t, filepath.Join(sourcePath, "SKILL.md"), []byte("---\nname: profile-leaf-canary\ndescription: profile leaf alias fixture\n---\nKeep this source.\n"), 0o644)
		library := filepath.Join(fx.xdgData, "skillglass", "skills")
		scan := runALE86HermesAliasCLI(t, fx, []string{"HERMES_HOME=" + profile}, "consolidate", "scan", "--app", "hermes", "--library", library, "--json")
		assertExit(t, scan, 0)
		var inventory collection.ConsolidationInventory
		decodeJSON(t, scan.stdout, &inventory)
		source := sourceAtPhysicalPath(t, inventory, sourcePath)
		if !source.Eligible {
			t.Fatalf("profile-leaf alias source should remain inspectable and keepable: %#v", source.BlockedReasons)
		}
		if hermesAliasProfile(inventory) != nil {
			t.Fatalf("symlinked explicit profile leaf received relocation evidence: %#v", hermesAliasProfile(inventory))
		}
		draft := buildConsolidationDraft(inventory, map[collection.SourceID]string{source.ID: "share"})
		draftPath := filepath.Join(fx.base, "allowed", "ownership-artifacts", "hermes-profile-leaf-alias.json")
		writeConsolidationDraft(t, draftPath, draft)
		preview := runALE86HermesAliasCLI(t, fx, []string{"HERMES_HOME=" + profile}, "consolidate", "preview", "--draft", draftPath, "--json")
		assertExit(t, preview, 1)
		var refused collection.ConsolidationPreview
		decodeJSON(t, preview.stdout, &refused)
		if refused.State != "refused" || refused.Failure == nil || refused.Failure.Code != "unsupported-native-relocation" {
			t.Fatalf("symlinked explicit profile leaf was not refused for relocation: %#v", refused)
		}
	})

	t.Run("skills root leaf alias", func(t *testing.T) {
		fx, profile := hermesFixture(t)
		writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("---\nname: acceptance-home-canary\ndescription: bounded fixture canary\n---\nFixture only.\n"), 0o644)
		skillsRoot := filepath.Join(profile, "skills")
		target := filepath.Join(fx.base, "allowed", "hermes-skills-target")
		sourcePath := filepath.Join(target, "skills-leaf-canary")
		writeFile(t, filepath.Join(sourcePath, "SKILL.md"), []byte("---\nname: skills-leaf-canary\ndescription: skills leaf alias fixture\n---\nKeep this source.\n"), 0o644)
		if err := os.Symlink(target, skillsRoot); err != nil {
			t.Fatal(err)
		}
		library := filepath.Join(fx.xdgData, "skillglass", "skills")
		scan := runALE86HermesAliasCLI(t, fx, []string{"HERMES_HOME=" + profile}, "consolidate", "scan", "--app", "hermes", "--library", library, "--json")
		assertExit(t, scan, 0)
		var inventory collection.ConsolidationInventory
		decodeJSON(t, scan.stdout, &inventory)
		source := sourceAtPhysicalPath(t, inventory, sourcePath)
		if !source.Eligible {
			t.Fatalf("skills-leaf alias source should remain inspectable and keepable: %#v", source.BlockedReasons)
		}
		if hermesAliasProfile(inventory) != nil {
			t.Fatalf("symlinked skills root leaf received relocation evidence: %#v", hermesAliasProfile(inventory))
		}
		foundRefusal := false
		for _, audit := range inventory.DestinationAudits {
			if audit.Application != "hermes" {
				continue
			}
			for _, refusal := range audit.Refusals {
				if strings.Contains(refusal, "destination path or one of its ancestors is an alias or unresolved") {
					foundRefusal = true
				}
			}
		}
		if !foundRefusal {
			t.Fatalf("symlinked skills-root leaf did not retain an explicit no-follow destination refusal: %#v", inventory.DestinationAudits)
		}
	})
}

func runALE86HermesAliasCLI(t *testing.T, fx fixture, env []string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := fx.command(t, ctx, args...)
	command.Dir = fx.project
	command.Env = mergeALE86HermesEnv(fx.environment(), env)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.Stdin = bytes.NewReader(nil)
	started := time.Now()
	launch := recordLaunch(t, command)
	err := command.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	if ctx.Err() != nil {
		t.Fatal("Hermes alias public command timed out")
	}
	result := commandResult{code: code, stdout: stdout.Bytes(), stderr: stderr.Bytes(), duration: time.Since(started), launch: launch}
	recordEvidenceCommand(t, "hermes-ancestor-alias-public-cli", result)
	assertRecordedContext(t, launch, command)
	return result
}

func runALE86HermesAliasChangeCLI(t *testing.T, fx fixture, env []string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	argv := fx.launchArgs(t, skillglassBinary, true, "", args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir = fx.project
	command.Env = mergeALE86HermesEnv(fx.environment(), env)
	command.Stdin = bytes.NewReader(nil)
	result, err := executeEvidenceCommand(t, command, "offline-public-mutation")
	if result.executionCanceled || ctx.Err() != nil {
		t.Fatalf("Hermes alias public mutation command timed out: %s", strings.Join(args, " "))
	}
	if result.recordingErr != nil {
		t.Fatalf("Hermes alias public mutation launch recording failed: %v", result.recordingErr)
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("Hermes alias public mutation command failed to execute: %v", err)
		}
	}
	return result
}

func mergeALE86HermesEnv(base, overrides []string) []string {
	merged := append([]string(nil), base...)
	for _, override := range overrides {
		separator := strings.IndexByte(override, '=')
		if separator <= 0 {
			continue
		}
		key := override[:separator]
		prefix := key + "="
		replaced := false
		for index, entry := range merged {
			if strings.HasPrefix(entry, prefix) {
				merged[index] = override
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, override)
		}
	}
	return merged
}

func hermesAliasProfile(inventory collection.ConsolidationInventory) *collection.ConsolidationRelocationProof {
	for _, app := range inventory.Applications {
		if app.Application == "hermes" {
			return app.NativeRelocationProfile
		}
	}
	return nil
}

func hermesAliasProfileID(proof *collection.ConsolidationRelocationProof) string {
	if proof == nil {
		return ""
	}
	return proof.ID
}

func sourceAtPhysicalPath(t *testing.T, inventory collection.ConsolidationInventory, path string) collection.ConsolidationSource {
	t.Helper()
	want, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range inventory.Sources {
		got, err := os.Stat(source.SourcePath)
		if err == nil && os.SameFile(want, got) {
			return source
		}
	}
	t.Fatalf("no scanned source shares the physical identity of %s", path)
	return collection.ConsolidationSource{}
}

func consolidationChoiceForSource(draft collection.ConsolidationDraft, id collection.SourceID) (collection.ConsolidationChoice, bool) {
	for _, choice := range draft.Choices {
		if choice.SourceID == id {
			return choice, true
		}
	}
	return collection.ConsolidationChoice{}, false
}

func hasALE86LaunchEnv(result commandResult, key, value string) bool {
	for _, entry := range result.launch.Environment {
		if entry == key+"="+value {
			return true
		}
	}
	return false
}

func hasALE86LaunchEnvKey(result commandResult, key string) bool {
	prefix := key + "="
	for _, entry := range result.launch.Environment {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func assertALE86LaunchEnv(t *testing.T, result commandResult, key, value string) {
	t.Helper()
	matches := 0
	for _, entry := range result.launch.Environment {
		if strings.HasPrefix(entry, key+"=") {
			matches++
			if entry != key+"="+value {
				t.Fatalf("public child environment %s override=%q, want %q", key, entry, key+"="+value)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("public child environment contains %d entries for %s, want exactly one", matches, key)
	}
}
