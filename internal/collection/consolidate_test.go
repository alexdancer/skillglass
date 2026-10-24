package collection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	root := t.TempDir()
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
		SourcePath: target, BundleFingerprint: fingerprint, Eligible: true,
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
	directory := t.TempDir()
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
	if err != nil || initial != "selection1:none" {
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
