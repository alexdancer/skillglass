package collection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRememberConsolidationSelectionRejectsOversizedEncodingBeforeCreatingState(t *testing.T) {
	base := t.TempDir()
	config := filepath.Join(base, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("XDG_CONFIG_HOME", config)
	roots := make([]string, 12_000)
	for i := range roots {
		roots[i] = "x"
	}
	request := ConsolidationRequest{Applications: []string{"pi"}, Roots: roots, Library: filepath.Join(base, "library")}
	encoded, err := json.MarshalIndent(consolidationSelection{SchemaVersion: ConsolidationSchemaVersion, Applications: request.Applications, Roots: request.Roots, Library: request.Library}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if len(strings.Join(roots, "")) >= maxConsolidationSelectionBytes || len(encoded) <= maxConsolidationSelectionBytes {
		t.Fatalf("fixture must exceed the encoded limit by JSON structure alone: raw_root_bytes=%d encoded=%d", len(strings.Join(roots, "")), len(encoded))
	}
	if err := RememberConsolidationSelection(request); err == nil || !strings.Contains(err.Error(), "64 KiB encoded limit") {
		t.Fatalf("oversized selection writer did not refuse before writing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config, "skillglass")); !os.IsNotExist(err) {
		t.Fatalf("oversized selection created its state directory: %v", err)
	}
}

func TestConsolidationMoveDevicePreflightIsSkippedOnlyForNoop(t *testing.T) {
	_, _, library := consolidationLibraryAuditFixture(t)
	sourcePath := canonicalFixturePath(t, filepath.Join(t.TempDir(), "move-device-source"))
	if err := os.MkdirAll(sourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "SKILL.md"), []byte("---\nname: move-device\ndescription: test\n---\nmove device test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, fingerprint, complete, reason := consolidationFingerprintPath(context.Background(), sourcePath)
	if !complete || source.ID == "" {
		t.Fatalf("move source lacks complete identity: complete=%t source=%q reason=%s", complete, source.ID, reason)
	}
	effect := ConsolidationEffect{Action: "share", SourceID: source.ID, SourcePath: sourcePath, CanonicalPath: filepath.Join(library, "move-device"), BackupPath: filepath.Join(library, ".backups", "move-device"), BundleFingerprint: fingerprint}
	deviceCalls := 0
	deviceForPath := func(string) (int32, error) {
		deviceCalls++
		return 0, nil
	}
	if err := consolidationMovesPreflightWithDevice(context.Background(), []ConsolidationEffect{effect}, false, deviceForPath); err != nil {
		t.Fatalf("fresh no-op preflight should retain source/ownership checks without requiring move devices: %v", err)
	}
	if deviceCalls != 0 {
		t.Fatalf("no-op called move-device lookup %d times", deviceCalls)
	}
	var sourceStat unix.Stat_t
	if err := unix.Lstat(sourcePath, &sourceStat); err != nil {
		t.Fatal(err)
	}
	deviceForPath = func(string) (int32, error) {
		deviceCalls++
		return sourceStat.Dev + 1, nil
	}
	err := consolidationMovesPreflightWithDevice(context.Background(), []ConsolidationEffect{effect}, true, deviceForPath)
	if err == nil || !strings.Contains(err.Error(), "cross-device relocation is unsupported") {
		t.Fatalf("real move preflight did not preserve its cross-device refusal: calls=%d err=%v", deviceCalls, err)
	}
}

func TestConsolidationMoveDevicePreflightChecksOnlyEffectsThatMoveBundles(t *testing.T) {
	_, _, library := consolidationLibraryAuditFixture(t)
	sourcePath := canonicalFixturePath(t, filepath.Join(t.TempDir(), "link-only-source"))
	if err := os.MkdirAll(sourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "SKILL.md"), []byte("---\nname: link-only\ndescription: test\n---\nlink-only test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, fingerprint, complete, reason := consolidationFingerprintPath(context.Background(), sourcePath)
	if !complete || source.ID == "" {
		t.Fatalf("link-only source lacks complete identity: complete=%t source=%q reason=%s", complete, source.ID, reason)
	}

	deviceCalls := 0
	deviceForPath := func(string) (int32, error) {
		deviceCalls++
		return 0, errors.New("link-only effect must not inspect a move destination")
	}
	linkOnly := ConsolidationEffect{
		Action: "share", SourceID: source.ID, SourcePath: sourcePath,
		CanonicalPath: sourcePath, BackupPath: filepath.Join(library, ".backups", "link-only"),
		BundleFingerprint: fingerprint, Destinations: []string{filepath.Join(t.TempDir(), "codex", "link-only")},
	}
	if err := consolidationMovesPreflightWithDevice(context.Background(), []ConsolidationEffect{linkOnly}, true, deviceForPath); err != nil {
		t.Fatalf("link-only preflight required an unused move-device lookup: %v", err)
	}
	if deviceCalls != 0 {
		t.Fatalf("link-only preflight called move-device lookup %d times", deviceCalls)
	}

	var sourceStat unix.Stat_t
	if err := unix.Lstat(sourcePath, &sourceStat); err != nil {
		t.Fatal(err)
	}
	move := linkOnly
	move.SourcePath = canonicalFixturePath(t, filepath.Join(t.TempDir(), "move-source"))
	move.CanonicalPath = filepath.Join(library, "move-destination")
	if err := os.MkdirAll(move.SourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(move.SourcePath, "SKILL.md"), []byte("---\nname: actual-move\ndescription: test\n---\nactual move test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moveSource, moveFingerprint, moveComplete, moveReason := consolidationFingerprintPath(context.Background(), move.SourcePath)
	if !moveComplete || moveSource.ID == "" {
		t.Fatalf("move source lacks complete identity: complete=%t source=%q reason=%s", moveComplete, moveSource.ID, moveReason)
	}
	move.SourceID = moveSource.ID
	move.BundleFingerprint = moveFingerprint
	deviceCalls = 0
	deviceForPath = func(string) (int32, error) {
		deviceCalls++
		return sourceStat.Dev + 1, nil
	}
	err := consolidationMovesPreflightWithDevice(context.Background(), []ConsolidationEffect{linkOnly, move}, true, deviceForPath)
	if err == nil || !strings.Contains(err.Error(), "cross-device relocation is unsupported") {
		t.Fatalf("mixed plan did not retain cross-device refusal for its actual move: calls=%d err=%v", deviceCalls, err)
	}
	if deviceCalls == 0 {
		t.Fatal("actual move in mixed plan skipped move-device lookup")
	}
	if err := os.MkdirAll(move.CanonicalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	deviceCalls = 0
	deviceForPath = func(path string) (int32, error) {
		deviceCalls++
		if filepath.Clean(path) == filepath.Clean(move.CanonicalPath) {
			return sourceStat.Dev, nil
		}
		return sourceStat.Dev + 1, nil
	}
	duplicateErr := consolidationMovesPreflightWithDevice(context.Background(), []ConsolidationEffect{move}, true, deviceForPath)
	if duplicateErr == nil || !strings.Contains(duplicateErr.Error(), "duplicate recovery path must be on the source filesystem") {
		t.Fatalf("duplicate-to-backup move skipped its same-filesystem check: calls=%d err=%v", deviceCalls, duplicateErr)
	}
	if deviceCalls != 2 {
		t.Fatalf("duplicate-to-backup preflight performed %d device lookups, want canonical and backup checks", deviceCalls)
	}

	archive := move
	archive.Action = "archive"
	deviceCalls = 0
	archiveErr := consolidationMovesPreflightWithDevice(context.Background(), []ConsolidationEffect{archive}, true, deviceForPath)
	if archiveErr == nil || !strings.Contains(archiveErr.Error(), "cross-device relocation is unsupported") {
		t.Fatalf("archive did not retain cross-device refusal: calls=%d err=%v", deviceCalls, archiveErr)
	}
	if deviceCalls == 0 {
		t.Fatal("archive skipped move-device lookup")
	}
}

func TestConsolidationJournalLifecycleProjectionCoversMultiLinkStates(t *testing.T) {
	paths := []string{"/native/codex/skill", "/native/opencode/skill", "/native/pi/skill"}
	journal := consolidationJournal{
		SchemaVersion: ConsolidationSchemaVersion,
		ID:            "c-0123456789abcdef01234567",
		State:         "prepared",
		Entries: []consolidationJournalEntry{{
			SourceID:             SourceID("src_aaaaaaaaaaaaaaaaaaaaaaaaaa"),
			Action:               "share",
			SourcePath:           paths[2],
			CanonicalPath:        "/library/skill",
			ExpectedLinks:        paths[:2],
			CreatedLinks:         []string{},
			PendingLinks:         []string{},
			FailedLinks:          []string{},
			RemovedLinks:         []string{},
			PendingRecoveryLinks: []string{},
		}},
	}
	projected := consolidationJournalLifecycleProjection(journal)
	entry := projected.Entries[0]
	for label, actual := range map[string][]string{"created": entry.CreatedLinks, "removed": entry.RemovedLinks} {
		if len(actual) != len(paths) {
			t.Errorf("%s projection has %d candidate paths, want %d", label, len(actual), len(paths))
			continue
		}
		for index := range paths {
			if actual[index] != paths[index] {
				t.Errorf("%s path[%d]=%q, want %q", label, index, actual[index], paths[index])
			}
		}
	}
	for label, actual := range map[string][]string{"pending apply": entry.PendingLinks, "failed": entry.FailedLinks, "pending recovery": entry.PendingRecoveryLinks} {
		if len(actual) != 1 || actual[0] != paths[1] {
			t.Errorf("%s projection did not retain the longest JSON-encoded candidate: %#v", label, actual)
		}
	}
	if projected.State != "recovering" || !projected.PendingSelectionApply || !projected.SelectionApplied || !projected.SelectionRestored || !projected.PendingSelectionRestore || entry.PendingMove != "duplicate" || entry.PendingRecoveryMove != "canonical" || !entry.MovedToCanonical || !entry.MovedToBackup || !entry.CanonicalWasPresent || !entry.RestoredToSource {
		t.Fatalf("projection omitted a reachable state marker: journal=%#v entry=%#v", projected, entry)
	}
	original := journal.Entries[0]
	if journal.State != "prepared" || len(original.CreatedLinks) != 0 || len(original.PendingLinks) != 0 || len(original.FailedLinks) != 0 || len(original.RemovedLinks) != 0 || len(original.PendingRecoveryLinks) != 0 || original.PendingMove != "" || original.PendingRecoveryMove != "" {
		t.Fatalf("building the size projection mutated the prepared journal: %#v", original)
	}
}

func TestConsolidationDestinationNamesDeduplicateOnlyCompleteIdenticalBundles(t *testing.T) {
	var names consolidationDestinationNames
	if err := names.add("codex", "review", "review", "sha256-a"); err != nil {
		t.Fatalf("first bundle: %v", err)
	}
	if err := names.add("codex", "review", "review", "sha256-a"); err != nil {
		t.Fatalf("identical complete bundle: %v", err)
	}
	if err := names.add("codex", "review", "review-copy", "sha256-a"); err == nil || !strings.Contains(err.Error(), "one canonical link leaf") {
		t.Fatalf("identical bundle with a second link leaf should refuse, got %v", err)
	}
	if err := names.add("codex", "review", "review-variant", "sha256-b"); err == nil || !strings.Contains(err.Error(), "native name review remains duplicated") {
		t.Fatalf("divergent same-name bundle should refuse, got %v", err)
	}
}

func TestCodexDisabledRelocationRequiresExactExistingManifestAlias(t *testing.T) {
	const directory = "/fixture/.codex/skills/review"
	ref := ConsolidationReference{
		Application: "codex",
		Origin:      "codex-user",
		Reference:   Reference{Context: ContextGlobal, DiscoveredPath: directory, ResolvedPath: directory, Status: "resolved"},
	}
	source := ConsolidationSource{KnownReferences: []ConsolidationReference{ref}}
	destination := ConsolidationApplication{Scope: Scope{Agent: &AgentScope{Application: "codex", ValidatedVersion: codexValidatedVersion}}}
	availability := AvailabilityExplanation{
		State:  AvailabilityDisabled,
		Reason: "effective manifest selector disables this source",
		Evidence: &Evidence{
			Rule: "configured-availability", Setting: "skills.config.path=" + directory + "/SKILL.md", AgentVersion: codexValidatedVersion,
		},
	}
	if !consolidationCodexDisableProof(source, "codex", destination, availability) {
		t.Fatal("exact disabled selector for a retained original reference should match the recorded profile")
	}
	for name, mutate := range map[string]func(*ConsolidationSource, *AvailabilityExplanation){
		"folder selector": func(_ *ConsolidationSource, value *AvailabilityExplanation) {
			value.Evidence.Setting = "skills.config.path=" + directory
		},
		"unrelated manifest path": func(_ *ConsolidationSource, value *AvailabilityExplanation) {
			value.Evidence.Setting = "skills.config.path=/elsewhere/SKILL.md"
		},
		"wrong evidence rule": func(_ *ConsolidationSource, value *AvailabilityExplanation) { value.Evidence.Rule = "unknown" },
		"unknown state":       func(_ *ConsolidationSource, value *AvailabilityExplanation) { value.State = AvailabilityUnknown },
		"non-Codex app": func(value *ConsolidationSource, _ *AvailabilityExplanation) {
			value.KnownReferences[0].Application = "pi"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := source
			candidate.KnownReferences = append([]ConsolidationReference(nil), source.KnownReferences...)
			observed := availability
			evidence := *availability.Evidence
			observed.Evidence = &evidence
			mutate(&candidate, &observed)
			if consolidationCodexDisableProof(candidate, "codex", destination, observed) {
				t.Fatal("unsupported selector evidence was accepted")
			}
		})
	}
}

func TestRelocationAuthorityRefusesAnExcludedApplicationsPhysicalSource(t *testing.T) {
	root := "/fixture/.hermes/skills"
	sourcePath := root + "/personal"
	snapshots := []consolidationSnapshot{{
		app:      "hermes",
		snapshot: &Snapshot{scope: Scope{Roots: []RootReport{{Requested: root, Resolved: root, Context: ContextGlobal, Origin: "hermes-profile"}}}},
	}}
	selectedRefs := []ConsolidationReference{{
		Application: "codex", Origin: "codex-user",
		Reference: Reference{Context: ContextGlobal, DiscoveredPath: sourcePath, ResolvedPath: sourcePath, Status: "resolved"},
	}}
	scope, reason := consolidationRelocationAuthority(sourcePath, selectedRefs, map[string]bool{"codex": true}, snapshots)
	if scope != "unselected-application-root" || !strings.Contains(reason, "hermes") {
		t.Fatalf("excluded physical ownership must refuse relocation, got scope=%q reason=%q", scope, reason)
	}
}

func TestRelocationAuthorityTreatsForeignLoaderRoutesAsConsumers(t *testing.T) {
	const sourcePath = "/fixture/.codex/skills/review"
	snapshots := []consolidationSnapshot{
		{app: "codeaf", snapshot: &Snapshot{scope: Scope{Roots: []RootReport{{Requested: "/fixture/.codex/skills", Resolved: "/fixture/.codex/skills", Context: ContextGlobal, Origin: "codeaf-global"}}}}},
		{app: "opencode", snapshot: &Snapshot{scope: Scope{Roots: []RootReport{{Requested: "/fixture/.claude/skills", Resolved: "/fixture/.claude/skills", Context: ContextGlobal, Origin: "opencode-external"}}}}},
	}
	selectedRefs := []ConsolidationReference{{
		Application: "codex", Origin: "codex-compatibility-user",
		Reference: Reference{Root: "codex-root", Context: ContextGlobal, DiscoveredPath: sourcePath, ResolvedPath: sourcePath, Status: "resolved"},
	}}
	scope, reason := consolidationRelocationAuthority(sourcePath, selectedRefs, map[string]bool{"codex": true}, snapshots)
	if scope != "selected-personal-source" || reason != "" {
		t.Fatalf("foreign discovery routes must not claim physical source ownership, got scope=%q reason=%q", scope, reason)
	}
	if consolidationRootIsPhysicalPersonalRoot("codeaf", RootReport{Context: ContextGlobal, Origin: "codeaf-global", Requested: "/fixture/.codex/skills"}) {
		t.Fatal("CodeAF compatibility route into Codex must not be classified as CodeAF-owned")
	}
	if consolidationRootIsPhysicalPersonalRoot("opencode", RootReport{Context: ContextGlobal, Origin: "opencode-external", Requested: "/fixture/.claude/skills"}) {
		t.Fatal("OpenCode external Claude route must not be classified as OpenCode-owned")
	}
}

func TestConsolidationEffectsRecognizeOnlyUnchangedRerunsAsNoops(t *testing.T) {
	tests := map[string]struct {
		effects []ConsolidationEffect
		want    bool
	}{
		"empty plan":                            {effects: nil, want: true},
		"keep":                                  {effects: []ConsolidationEffect{{Action: "keep"}}, want: true},
		"already canonical with existing links": {effects: []ConsolidationEffect{{Action: "share", SourcePath: "/library/hash", CanonicalPath: "/library/hash"}}, want: true},
		"new destination link":                  {effects: []ConsolidationEffect{{Action: "share", SourcePath: "/library/hash", CanonicalPath: "/library/hash", Destinations: []string{"/codex/skills/name"}}}, want: false},
		"relocation":                            {effects: []ConsolidationEffect{{Action: "share", SourcePath: "/source/name", CanonicalPath: "/library/hash"}}, want: false},
		"archive":                               {effects: []ConsolidationEffect{{Action: "archive"}}, want: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := consolidationEffectsAreNoop(test.effects); got != test.want {
				t.Fatalf("consolidationEffectsAreNoop() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIndirectConsumerAcknowledgmentBindsExactRouteAndDoesNotExpandSelection(t *testing.T) {
	const destination = "/fixture/.claude/skills"
	const routeID = "route1:current"
	audits := []ConsolidationDestinationAudit{{
		Application:     "claude-code",
		DestinationRoot: destination,
		PotentialConsumers: []ConsolidationConsumerRoute{{
			Application: "claude-code", EvidenceID: "route1:claude",
		}, {
			Application: "opencode", EvidenceID: routeID,
		}},
	}}
	assessments := []ConsolidationDestinationChoice{{
		Application: "claude-code", Portability: "compatible", Reason: "instructions and resources were reviewed",
		AcknowledgedConsumers: []ConsolidationConsumerAcknowledgment{{
			Application: "opencode", EvidenceIDs: []string{routeID}, UserApproved: true,
		}},
	}}
	if err := validateDestinationConsumerSet(nil, destination, filepath.Join(destination, "skill"), "claude-code", []string{"claude-code"}, assessments, audits); err != nil {
		t.Fatalf("exact user-approved unselected route should be accepted: %v", err)
	}
	if err := validateDestinationConsumerSet(nil, destination, filepath.Join(destination, "skill"), "claude-code", []string{"claude-code"}, []ConsolidationDestinationChoice{{
		Application: "claude-code", Portability: "compatible", Reason: "reviewed",
	}}, audits); err == nil || !strings.Contains(err.Error(), "explicit acknowledgement is required") {
		t.Fatalf("unacknowledged possible consumer must refuse, got %v", err)
	}
	stale := assessments
	stale[0].AcknowledgedConsumers = append([]ConsolidationConsumerAcknowledgment(nil), assessments[0].AcknowledgedConsumers...)
	stale[0].AcknowledgedConsumers[0].EvidenceIDs = []string{"route1:stale"}
	if err := validateDestinationConsumerSet(nil, destination, filepath.Join(destination, "skill"), "claude-code", []string{"claude-code"}, stale, audits); err == nil || !strings.Contains(err.Error(), "exactly current scan evidence") {
		t.Fatalf("stale route evidence must refuse, got %v", err)
	}
}

func TestDestinationPreflightIgnoresNestedManagedRootsOnlyForSiblingLinks(t *testing.T) {
	refusal := "codex: managed/system root: /fixture/.codex/skills/.system"
	remaining, warnings := consolidationDestinationRouteNotes("/fixture/.codex/skills", []string{refusal}, nil)
	if len(remaining) != 0 || len(warnings) != 1 || !strings.Contains(warnings[0], ".system") {
		t.Fatalf("scan should disclose the untouched reserved root without treating sibling skill leaves as a conflict: refusals=%v warnings=%v", remaining, warnings)
	}
	if got := consolidationRelevantDestinationRefusals([]string{refusal}, "/fixture/.codex/skills/new-skill"); len(got) != 0 {
		t.Fatalf("sibling skill link should not collide with nested system root: %v", got)
	}
	remaining, warnings = consolidationDestinationRouteNotes("/fixture/.codex/skills", []string{"codex: managed/system root: /fixture/.codex/skills/skills"}, nil)
	if len(remaining) != 1 || len(warnings) != 0 {
		t.Fatalf("a valid native name can collide with a managed child, so it must remain refused: refusals=%v warnings=%v", remaining, warnings)
	}
	if got := consolidationRelevantDestinationRefusals([]string{refusal}, "/fixture/.codex/skills/.system/new-skill"); len(got) != 1 {
		t.Fatalf("target inside nested system root must remain refused: %v", got)
	}
	other := "codex: incomplete discovery: /fixture/settings.toml"
	if got := consolidationRelevantDestinationRefusals([]string{refusal, other}, "/fixture/.codex/skills/new-skill"); len(got) != 1 || got[0] != other {
		t.Fatalf("unrelated route-evidence refusals must remain blocking: %v", got)
	}
}

func TestLibraryAuditAcceptsOnlyKnownExactLinksIntoCompleteCanonicalBundles(t *testing.T) {
	root := canonicalFixturePath(t, t.TempDir())
	library := filepath.Join(root, "library")
	fingerprint := strings.Repeat("a", 64)
	target := filepath.Join(library, fingerprint)
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "app", "skills", "review")
	if err := os.MkdirAll(filepath.Dir(alias), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	source := ConsolidationSource{
		SourcePath: target, BundleFingerprint: fingerprint, Eligible: true, RelocationScope: "consolidation-library",
		KnownReferences: []ConsolidationReference{{Reference: Reference{DiscoveredPath: alias, ResolvedPath: target, Status: "resolved"}}},
	}
	refusal := "codex: prospective alias isolation is unverified: related or unresolved alias " + alias + " targets " + target + "; unsupported isolation"
	remaining, warnings := consolidationLibraryRouteNotes(library, []ConsolidationSource{source}, []string{refusal}, nil)
	if len(remaining) != 0 || len(warnings) != 1 || !strings.Contains(warnings[0], alias) {
		t.Fatalf("exact known symlink into a proven canonical bundle should be disclosed and preserved: refusals=%v warnings=%v", remaining, warnings)
	}
	for name, mutate := range map[string]func(*ConsolidationSource){
		"unknown alias":          func(value *ConsolidationSource) { value.KnownReferences = nil },
		"incomplete source":      func(value *ConsolidationSource) { value.Eligible = false },
		"wrong fingerprint path": func(value *ConsolidationSource) { value.BundleFingerprint = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := source
			candidate.KnownReferences = append([]ConsolidationReference(nil), source.KnownReferences...)
			mutate(&candidate)
			remaining, _ := consolidationLibraryRouteNotes(library, []ConsolidationSource{candidate}, []string{refusal}, nil)
			if len(remaining) != 1 {
				t.Fatalf("unverified library alias must remain a refusal, got %v", remaining)
			}
		})
	}
}

func TestConsolidationFingerprintSeparatesLargeAssetInspectionLimit(t *testing.T) {
	directory := canonicalFixturePath(t, t.TempDir())
	manifest := []byte("---\nname: asset-check\ndescription: Check a bundled binary resource.\n---\nRead the binary resource metadata.\n")
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	asset := make([]byte, int(ResourceReadLimit)+1)
	for i := range asset {
		asset[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(directory, "asset.bin"), asset, 0o755); err != nil {
		t.Fatal(err)
	}
	parsed, manifestProblems := inspectManifest(directory, false)
	resources, resourceProblems := inventoryResources(context.Background(), directory)
	if len(manifestProblems)+len(resourceProblems) != 0 {
		t.Fatalf("fixture inventory problems: manifest=%v resources=%v", manifestProblems, resourceProblems)
	}
	token, err := directoryObjectToken(directory)
	if err != nil {
		t.Fatal(err)
	}
	source := Source{
		ID: sourceID(token), Directory: directory,
		Identity: Identity{Method: "darwin-directory-object", Certainty: "proven"},
		Manifest: parsed, Resources: resources,
	}
	fingerprint, coverage, inspectionComplete, reasons, limits := consolidationFingerprint(context.Background(), source)
	if coverage != Complete || fingerprint == "" || inspectionComplete || len(reasons) != 0 || len(limits) != 1 {
		t.Fatalf("large binary should remain fully fingerprintable but disclose its inspection limit: fingerprint=%q coverage=%q complete=%t reasons=%v limits=%v", fingerprint, coverage, inspectionComplete, reasons, limits)
	}
	if !strings.Contains(limits[0], "bundle identity remains fully fingerprinted") {
		t.Fatalf("inspection limit must distinguish content display from identity: %q", limits[0])
	}
}

func TestCurrentConsolidationSelectionIDTracksOnlyPersistedSelections(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	initial, err := CurrentConsolidationSelectionID()
	if err != nil || !strings.HasPrefix(initial, "selection1:none:") {
		t.Fatalf("missing selection should have a stable absent marker, got %q, %v", initial, err)
	}
	first, err := NormalizeConsolidationRequest([]string{"codex", "pi"}, []string{filepath.Join(base, "custom")}, filepath.Join(base, "library"), "arguments")
	if err != nil {
		t.Fatal(err)
	}
	if err := RememberConsolidationSelection(first); err != nil {
		t.Fatal(err)
	}
	firstID, err := CurrentConsolidationSelectionID()
	if err != nil || firstID != RememberedSelectionID(first.Applications, first.Roots, first.Library) {
		t.Fatalf("stored selection ID mismatch: got %q, %v", firstID, err)
	}
	second, err := NormalizeConsolidationRequest([]string{"pi"}, nil, first.Library, "arguments")
	if err != nil {
		t.Fatal(err)
	}
	if err := RememberConsolidationSelection(second); err != nil {
		t.Fatal(err)
	}
	secondID, err := CurrentConsolidationSelectionID()
	if err != nil || secondID == firstID {
		t.Fatalf("changed persisted selection must invalidate an old scan, got %q after %q: %v", secondID, firstID, err)
	}
}

func TestConsolidationJournalDoesNotAdoptSelectionChangedAfterScan(t *testing.T) {
	request, _, library := consolidationLibraryAuditFixture(t)
	if err := RememberConsolidationSelection(request); err != nil {
		t.Fatal(err)
	}
	approvedID, err := CurrentConsolidationSelectionID()
	if err != nil {
		t.Fatal(err)
	}
	request.RememberedSelectionID = approvedID
	inventory := ScanConsolidation(context.Background(), request)
	if inventory.Failure != nil || inventory.SelectedCoverage != Complete || !inventory.LibrarySafe {
		t.Fatalf("initial isolated library scan is not safe: failure=%+v coverage=%q safe=%t", inventory.Failure, inventory.SelectedCoverage, inventory.LibrarySafe)
	}

	later, err := NormalizeConsolidationRequest([]string{"pi"}, nil, library, "arguments")
	if err != nil {
		t.Fatal(err)
	}
	if err := RememberConsolidationSelection(later); err != nil {
		t.Fatal(err)
	}
	selectionPath, err := consolidationSelectionPath()
	if err != nil {
		t.Fatal(err)
	}
	laterBytes, laterExists, err := readPrivateSelection(selectionPath)
	if err != nil || !laterExists {
		t.Fatalf("later selection is not readable: exists=%t err=%v", laterExists, err)
	}

	draft := ConsolidationDraft{SchemaVersion: ConsolidationSchemaVersion, Scope: inventory.Scope}
	preview := ConsolidationPreview{ScanID: inventory.ScanID, RecoveryID: "c-0123456789abcdef01234567", Effects: []ConsolidationEffect{}}
	if _, err := newConsolidationJournal(context.Background(), draft, preview); err == nil {
		t.Fatal("journal accepted a selection that changed after the proposal was scanned")
	}
	got, exists, err := readPrivateSelection(selectionPath)
	if err != nil || !exists || !bytes.Equal(got, laterBytes) {
		t.Fatalf("stale proposal changed the later selection: exists=%t err=%v got=%q want=%q", exists, err, got, laterBytes)
	}
	if _, err := readConsolidationJournal(preview.RecoveryID); err == nil {
		t.Fatal("stale proposal left a recovery record for an unapproved selection")
	}
	if currentID, err := CurrentConsolidationSelectionID(); err != nil || currentID == approvedID {
		t.Fatalf("later selection did not remain the current selection: got=%q approved=%q err=%v", currentID, approvedID, err)
	}
}

func TestConsolidationSelectionResolvesParentAliasesAndPinsCanonicalLocation(t *testing.T) {
	base := t.TempDir()
	configA := filepath.Join(base, "config-a")
	configB := filepath.Join(base, "config-b")
	alias := filepath.Join(base, "config-alias")
	if err := os.MkdirAll(configA, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configB, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(configA, alias); err != nil {
		t.Fatal(err)
	}
	canonicalA := canonicalFixturePath(t, configA)
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", alias)
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	path, err := consolidationSelectionPath()
	if err != nil || filepath.Dir(filepath.Dir(path)) != canonicalA {
		t.Fatalf("selection path should resolve a safe parent alias: %q, %v", path, err)
	}
	first, err := CurrentConsolidationSelectionID()
	if err != nil || !strings.HasPrefix(first, "selection1:none:") {
		t.Fatalf("missing selection through a parent alias should be readable: %q, %v", first, err)
	}
	request, err := NormalizeConsolidationRequest([]string{"codex"}, nil, filepath.Join(base, "library"), "arguments")
	if err != nil {
		t.Fatal(err)
	}
	if err := RememberConsolidationSelection(request); err != nil {
		t.Fatalf("remember through an aliased XDG parent: %v", err)
	}
	stored, err := CurrentConsolidationSelectionID()
	if err != nil || stored == first {
		t.Fatalf("persisted selection should change the selection ID: %q after %q, %v", stored, first, err)
	}
	t.Setenv("XDG_CONFIG_HOME", configA)
	canonical, err := CurrentConsolidationSelectionID()
	if err != nil || canonical != stored {
		t.Fatalf("alias and canonical spelling of the same state location should have one stable ID: %q vs %q, %v", canonical, stored, err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(configB, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", alias)
	redirected, err := CurrentConsolidationSelectionID()
	if err != nil || redirected == stored || !strings.HasPrefix(redirected, "selection1:none:") {
		t.Fatalf("redirecting XDG_CONFIG_HOME must invalidate the prior state evidence: %q after %q, %v", redirected, stored, err)
	}
}

func TestDefaultConsolidationLibraryResolvesXDGDataParentAlias(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data-real")
	alias := filepath.Join(base, "data-alias")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(data, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", alias)
	request, err := NormalizeConsolidationRequest([]string{"codex"}, nil, "", "arguments")
	if err != nil {
		t.Fatal(err)
	}
	canonicalData := canonicalFixturePath(t, data)
	want := filepath.Join(canonicalData, "skillglass", "skills")
	if request.Library != want {
		t.Fatalf("default library should use the canonical XDG data parent: got %q, want %q", request.Library, want)
	}
}

func canonicalFixturePath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := setupProspectivePath(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func consolidationLibraryAuditFixture(t *testing.T) (ConsolidationRequest, string, string) {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	config := filepath.Join(base, "config")
	data := filepath.Join(base, "data")
	library := filepath.Join(data, "skillglass", "skills")
	for _, path := range []string{home, config, data, filepath.Join(home, ".codex", "skills"), library} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": config, "XDG_DATA_HOME": data,
		"XDG_CACHE_HOME": filepath.Join(base, "cache"), "XDG_STATE_HOME": filepath.Join(base, "state"),
		"CODEX_HOME": "", "CLAUDE_CONFIG_DIR": "", "PI_CODING_AGENT_DIR": "",
		"OPENCODE_CONFIG_DIR": "", "OPENCODE_CONFIG_CONTENT": "", "OPENCODE_DISABLE_EXTERNAL_SKILLS": "",
		"OPENCODE_DISABLE_CLAUDE_CODE": "", "OPENCODE_DISABLE_CLAUDE_CODE_SKILLS": "",
		"OPENCODE_DISABLE_PROJECT_CONFIG": "", "OPENCODE_PURE": "", "HERMES_HOME": "",
		"HERMES_SAFE_MODE": "", "HERMES_ENABLE_PROJECT_PLUGINS": "", "GEMINI_CLI_HOME": "",
		"GEMINI_CLI_SYSTEM_SETTINGS_PATH": "", "GEMINI_CLI_TRUSTED_FOLDERS_PATH": "",
		"CODEAF_HOME": "", "CODEAF_PROFILE_DIR": "",
	} {
		t.Setenv(key, value)
	}
	request, err := NormalizeConsolidationRequest([]string{"codex"}, nil, library, "arguments")
	if err != nil {
		t.Fatal(err)
	}
	return request, filepath.Join(home, ".codex", "skills", ".shared"), library
}

func TestConsolidationScanIDBindsLibraryAudit(t *testing.T) {
	request, hiddenLink, library := consolidationLibraryAuditFixture(t)
	before := ScanConsolidation(context.Background(), request)
	if before.Failure != nil || !before.LibrarySafe {
		t.Fatalf("initial isolated library scan is not safe: failure=%+v reasons=%v", before.Failure, before.LibraryReasons)
	}
	if err := os.Symlink(library, hiddenLink); err != nil {
		t.Fatal(err)
	}
	after := ScanConsolidation(context.Background(), request)
	if after.Failure != nil || after.LibrarySafe {
		t.Fatalf("hidden discovery route was not rejected: failure=%+v safe=%t reasons=%v", after.Failure, after.LibrarySafe, after.LibraryReasons)
	}
	if after.ScanID == before.ScanID {
		t.Fatal("library safety audit changed without invalidating scan identity")
	}
}

func TestConsolidationPrewriteRescanRefusesNewHiddenLibraryRoute(t *testing.T) {
	request, hiddenLink, library := consolidationLibraryAuditFixture(t)
	before := ScanConsolidation(context.Background(), request)
	if before.Failure != nil || !before.LibrarySafe {
		t.Fatalf("initial isolated library scan is not safe: failure=%+v reasons=%v", before.Failure, before.LibraryReasons)
	}
	if err := os.Symlink(library, hiddenLink); err != nil {
		t.Fatal(err)
	}
	draft := ConsolidationDraft{SchemaVersion: ConsolidationSchemaVersion, Scope: request}
	preview := ConsolidationPreview{ScanID: before.ScanID, RecoveryID: "c-0123456789abcdef01234567", Effects: []ConsolidationEffect{}}
	if _, err := newConsolidationJournal(context.Background(), draft, preview); err == nil {
		t.Fatal("apply pre-write rescan accepted a library made unsafe after preview")
	}
}

func TestConsolidationRecoveryRefusesUnsafeLibrary(t *testing.T) {
	request, hiddenLink, library := consolidationLibraryAuditFixture(t)
	if err := os.Symlink(library, hiddenLink); err != nil {
		t.Fatal(err)
	}
	journal := consolidationJournal{SchemaVersion: ConsolidationSchemaVersion, ID: "c-0123456789abcdef01234567", State: "applied", Scope: request, Entries: []consolidationJournalEntry{}}
	if err := writeConsolidationJournal(journal); err != nil {
		t.Fatal(err)
	}
	journalPath := consolidationJournalPath(journal.ID)
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	preview := PreviewConsolidationRecovery(context.Background(), journal.ID)
	if preview.State != "refused" || preview.Failure == nil || preview.Failure.Code != "unsafe-library" {
		t.Fatalf("unsafe library recovery was not explicitly refused: %#v", preview)
	}
	out := ApplyConsolidationRecovery(context.Background(), journal.ID, preview.Confirmation)
	if out.State != "refused" || out.Failure == nil || out.Failure.Code != "unsafe-library" {
		t.Fatalf("unsafe library recovery application was not refused: %#v", out)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("refused recovery changed its journal")
	}
}
