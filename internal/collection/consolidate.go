package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const ConsolidationSchemaVersion = 1

var consolidationName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type ConsolidationRequest struct {
	Applications          []string `json:"applications"`
	Roots                 []string `json:"roots,omitempty"`
	Library               string   `json:"library"`
	SelectionFrom         string   `json:"selection_from"`
	RememberedSelectionID string   `json:"remembered_selection_id,omitempty"`
}

type ConsolidationReference struct {
	Application  string                  `json:"application"`
	Origin       string                  `json:"origin,omitempty"`
	Reference    Reference               `json:"reference"`
	Availability AvailabilityExplanation `json:"availability,omitempty"`
}

type ConsolidationSource struct {
	ID                  SourceID                           `json:"source_id"`
	SourcePath          string                             `json:"source_path"`
	Identity            Identity                           `json:"identity"`
	Name                string                             `json:"name,omitempty"`
	Description         string                             `json:"description,omitempty"`
	NativeName          string                             `json:"native_name,omitempty"`
	ManifestStatus      ManifestStatus                     `json:"manifest_status"`
	Ownership           EvidenceState                      `json:"ownership"`
	RelocationScope     string                             `json:"relocation_scope"`
	BundleFingerprint   string                             `json:"bundle_fingerprint,omitempty"`
	FingerprintCoverage Completeness                       `json:"fingerprint_coverage"`
	InspectionComplete  bool                               `json:"inspection_complete"`
	InspectionLimits    []string                           `json:"inspection_limits"`
	Resources           []Resource                         `json:"resources"`
	SelectedReferences  []ConsolidationReference           `json:"selected_references"`
	KnownReferences     []ConsolidationReference           `json:"known_references"`
	Availability        map[string]AvailabilityExplanation `json:"availability"`
	RelocationProofs    []ConsolidationRelocationProof     `json:"relocation_proofs,omitempty"`
	BlockedReasons      []string                           `json:"blocked_reasons"`
	Eligible            bool                               `json:"eligible"`
	ConflictGroup       string                             `json:"conflict_group,omitempty"`
}

// ConsolidationRelocationProof is a narrowly scoped recorded native profile,
// separate from the adapter parser baseline carried by Availability.Evidence.
type ConsolidationRelocationProof struct {
	ID            string   `json:"id"`
	Application   string   `json:"application"`
	Profile       string   `json:"profile"`
	BinarySHA256  string   `json:"binary_sha256"`
	Observed      string   `json:"observed"`
	Preconditions []string `json:"preconditions"`
	Limits        []string `json:"limits"`
}

const (
	codexConsolidationRelocationProfile = "Codex 0.162.1"
	codexConsolidationRelocationSHA256  = "74c6a6d263f40d65f99c27a815275f5b4ed0bcc2baecc396b4cf3255498bdfc0"
)

type ConsolidationApplication struct {
	Application     string                        `json:"application"`
	Selected        bool                          `json:"selected"`
	DestinationRoot string                        `json:"destination_root,omitempty"`
	Scope           Scope                         `json:"scope"`
	ExistingSources []ConsolidationExistingSource `json:"existing_sources"`
	Problems        []Problem                     `json:"problems"`
	ConfigEvidence  []PermissionLayer             `json:"config_evidence,omitempty"`
	Managers        []ManagerEvidence             `json:"manager_evidence,omitempty"`
	Gaps            []string                      `json:"gaps,omitempty"`
	Failure         string                        `json:"failure,omitempty"`
}

// ConsolidationDestinationAudit describes configured-loader routes that could
// observe a new link at one native destination. It does not prove installation,
// current use, or running-session adoption.
type ConsolidationDestinationAudit struct {
	Application        string                       `json:"application"`
	DestinationRoot    string                       `json:"destination_root"`
	PotentialConsumers []ConsolidationConsumerRoute `json:"potential_consumers"`
	Refusals           []string                     `json:"refusals"`
	Warnings           []string                     `json:"warnings"`
}

type ConsolidationConsumerRoute struct {
	Application       string   `json:"application"`
	EvidenceID        string   `json:"evidence_id"`
	RootIDs           []RootID `json:"root_ids,omitempty"`
	RootPaths         []string `json:"root_paths,omitempty"`
	InstallationState string   `json:"installation_state"`
	RuntimeState      string   `json:"runtime_state"`
	Reason            string   `json:"reason"`
}

type ConsolidationExistingSource struct {
	SourceID     SourceID                `json:"source_id"`
	SourcePath   string                  `json:"source_path"`
	Name         string                  `json:"name,omitempty"`
	NativeName   string                  `json:"native_name,omitempty"`
	Availability AvailabilityExplanation `json:"availability"`
	References   []Reference             `json:"references"`
}

type ConsolidationConflict struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	SourceIDs []SourceID `json:"source_ids"`
	Reason    string     `json:"reason"`
}

type ConsolidationExclusion struct {
	SourceID SourceID `json:"source_id,omitempty"`
	Path     string   `json:"path"`
	Reason   string   `json:"reason"`
}

type ConsolidationInventory struct {
	SchemaVersion     int                             `json:"schema_version"`
	Operation         string                          `json:"operation"`
	Outcome           string                          `json:"outcome"`
	ExitCode          int                             `json:"exit_code"`
	ScanID            string                          `json:"scan_id"`
	Scope             ConsolidationRequest            `json:"scope"`
	Coverage          Completeness                    `json:"coverage"`
	SelectedCoverage  Completeness                    `json:"selected_coverage"`
	Applications      []ConsolidationApplication      `json:"applications"`
	DestinationAudits []ConsolidationDestinationAudit `json:"destination_audits"`
	Sources           []ConsolidationSource           `json:"sources"`
	Conflicts         []ConsolidationConflict         `json:"conflicts"`
	Exclusions        []ConsolidationExclusion        `json:"exclusions"`
	Problems          []Problem                       `json:"problems"`
	LibrarySafe       bool                            `json:"library_safe"`
	LibraryReasons    []string                        `json:"library_reasons"`
	LibraryWarnings   []string                        `json:"library_warnings"`
	Failure           *ConsolidationFailure           `json:"failure,omitempty"`
}

type ConsolidationFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

type ConsolidationDestinationChoice struct {
	Application           string                                `json:"application"`
	Portability           string                                `json:"portability"`
	Reason                string                                `json:"reason"`
	UserApproved          bool                                  `json:"user_approved_unknown,omitempty"`
	AcknowledgedConsumers []ConsolidationConsumerAcknowledgment `json:"acknowledged_consumers,omitempty"`
}

type ConsolidationConsumerAcknowledgment struct {
	Application  string   `json:"application"`
	EvidenceIDs  []string `json:"evidence_ids"`
	UserApproved bool     `json:"user_approved"`
}

type ConsolidationChoice struct {
	SourceID          SourceID                         `json:"source_id"`
	SourcePath        string                           `json:"source_path"`
	BundleFingerprint string                           `json:"bundle_fingerprint"`
	Action            string                           `json:"action"`
	CanonicalName     string                           `json:"canonical_name,omitempty"`
	Destinations      []ConsolidationDestinationChoice `json:"destinations,omitempty"`
}

type ConsolidationDraft struct {
	SchemaVersion int                   `json:"schema_version"`
	Scope         ConsolidationRequest  `json:"scope"`
	ScanID        string                `json:"scan_id"`
	Choices       []ConsolidationChoice `json:"choices"`
}

type ConsolidationEffect struct {
	Action              string   `json:"action"`
	SourceID            SourceID `json:"source_id"`
	SourcePath          string   `json:"source_path"`
	BundleFingerprint   string   `json:"bundle_fingerprint,omitempty"`
	CanonicalPath       string   `json:"canonical_path,omitempty"`
	BackupPath          string   `json:"backup_path,omitempty"`
	Destinations        []string `json:"destinations,omitempty"`
	PreservedReferences []string `json:"preserved_references,omitempty"`
	State               string   `json:"state"`
	Reason              string   `json:"reason,omitempty"`
}

type ConsolidationPreview struct {
	SchemaVersion int                   `json:"schema_version"`
	Operation     string                `json:"operation"`
	Outcome       string                `json:"outcome"`
	ExitCode      int                   `json:"exit_code"`
	State         string                `json:"state"`
	Reason        string                `json:"reason"`
	ScanID        string                `json:"scan_id,omitempty"`
	Scope         ConsolidationRequest  `json:"scope"`
	Choices       []ConsolidationChoice `json:"choices"`
	Effects       []ConsolidationEffect `json:"effects"`
	Warnings      []string              `json:"warnings"`
	Confirmation  string                `json:"confirmation,omitempty"`
	RecoveryID    string                `json:"recovery_id,omitempty"`
	Failure       *ConsolidationFailure `json:"failure,omitempty"`
}

type consolidationSnapshot struct {
	app      string
	snapshot *Snapshot
	err      error
}

// NormalizeConsolidationRequest resolves defaults and validates an explicit app
// selection. The scan does not persist selection or write any filesystem state.
func NormalizeConsolidationRequest(apps, roots []string, library string, selectionFrom string) (ConsolidationRequest, error) {
	if strings.TrimSpace(library) == "" {
		if !filepath.IsAbs(os.Getenv("XDG_DATA_HOME")) && os.Getenv("HOME") == "" {
			return ConsolidationRequest{}, errors.New("default consolidation library requires HOME or an absolute XDG_DATA_HOME")
		}
		library = defaultConsolidationLibrary()
	}
	if !filepath.IsAbs(library) {
		var err error
		library, err = filepath.Abs(library)
		if err != nil {
			return ConsolidationRequest{}, fmt.Errorf("normalize library path: %w", err)
		}
	}
	library = filepath.Clean(library)
	if len(apps) == 0 {
		return ConsolidationRequest{}, errors.New("select at least one application explicitly or use the last successfully applied selection")
	}
	selected := map[string]bool{}
	for _, app := range apps {
		if _, ok := applicationCapability(app); !ok || app == "codeaf" {
			return ConsolidationRequest{}, fmt.Errorf("unsupported consolidation application %q; selectable applications are codex, claude-code, pi, opencode, hermes, and gemini", app)
		}
		if selected[app] {
			return ConsolidationRequest{}, fmt.Errorf("duplicate consolidation application %q", app)
		}
		selected[app] = true
	}
	normalizedApps := make([]string, 0, len(apps))
	for app := range selected {
		normalizedApps = append(normalizedApps, app)
	}
	sort.Strings(normalizedApps)
	normalizedRoots := make([]string, 0, len(roots))
	for i, raw := range roots {
		if strings.TrimSpace(raw) == "" {
			return ConsolidationRequest{}, fmt.Errorf("custom root %d is empty", i+1)
		}
		path, err := filepath.Abs(raw)
		if err != nil {
			return ConsolidationRequest{}, fmt.Errorf("normalize custom root %d: %w", i+1, err)
		}
		normalizedRoots = append(normalizedRoots, filepath.Clean(path))
	}
	sort.Strings(normalizedRoots)
	for i := 1; i < len(normalizedRoots); i++ {
		if normalizedRoots[i] == normalizedRoots[i-1] {
			return ConsolidationRequest{}, fmt.Errorf("duplicate custom root %q", normalizedRoots[i])
		}
	}
	return ConsolidationRequest{Applications: normalizedApps, Roots: normalizedRoots, Library: library, SelectionFrom: selectionFrom}, nil
}

func defaultConsolidationLibrary() string {
	home := os.Getenv("HOME")
	if data := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(data) {
		return filepath.Join(data, "skillglass", "skills")
	}
	if home == "" {
		return filepath.Join(string(filepath.Separator), "skillglass", "skills")
	}
	return filepath.Join(home, ".local", "share", "skillglass", "skills")
}

// ScanConsolidation combines existing native inventories without reimplementing
// source identity or discovery. Unselected adapters are inspected read-only to
// disclose current consumers and to guard the library/destination paths.
func ScanConsolidation(ctx context.Context, request ConsolidationRequest) ConsolidationInventory {
	result := ConsolidationInventory{SchemaVersion: ConsolidationSchemaVersion, Operation: "consolidate-scan", Outcome: "success", ExitCode: 0, Scope: request, Coverage: Complete, SelectedCoverage: Complete, Applications: []ConsolidationApplication{}, DestinationAudits: []ConsolidationDestinationAudit{}, Sources: []ConsolidationSource{}, Conflicts: []ConsolidationConflict{}, Exclusions: []ConsolidationExclusion{}, Problems: []Problem{}, LibraryReasons: []string{}, LibraryWarnings: []string{}}
	selected := map[string]bool{}
	for _, app := range request.Applications {
		selected[app] = true
	}
	if len(selected) == 0 || request.Library == "" {
		return consolidationFailedScan(result, "invalid-scope", "consolidation scope is missing selected applications or library", "Select at least one supported application and a library outside application skill roots.")
	}
	customRoots := make([]string, len(request.Roots))
	copy(customRoots, request.Roots)
	all := make([]consolidationSnapshot, 0, len(applicationCatalog))
	for _, capability := range applicationCatalog {
		roots := []string(nil)
		if selected[capability.Application] {
			roots = customRoots
		}
		req, err := NewRequestWithAgent(roots, nil, nil, capability.Application)
		entry := consolidationSnapshot{app: capability.Application}
		if err == nil {
			entry.snapshot, err = Build(ctx, req)
		}
		entry.err = err
		all = append(all, entry)
		appView := ConsolidationApplication{Application: capability.Application, Selected: selected[capability.Application], ExistingSources: []ConsolidationExistingSource{}, Problems: []Problem{}, Managers: []ManagerEvidence{}}
		if err != nil {
			appView.Failure = err.Error()
			appView.Scope = Scope{Completeness: Incomplete}
			result.Coverage = Incomplete
			if selected[capability.Application] {
				result.SelectedCoverage = Incomplete
			}
			result.Problems = append(result.Problems, Problem{Code: ProblemNativeInput, Path: capability.Application, Summary: "native application inventory failed", Detail: err.Error(), LimitsCoverage: true})
		} else {
			snapshot := entry.snapshot
			appView.Scope = snapshot.Scope()
			appView.Problems = snapshot.Problems()
			appView.Managers = append([]ManagerEvidence(nil), snapshot.managers...)
			if appView.Scope.Agent != nil {
				appView.ConfigEvidence = cloneLayers(appView.Scope.Agent.ConfigLayers)
				appView.Gaps = append([]string(nil), appView.Scope.Agent.Gaps...)
			}
			appView.DestinationRoot = consolidationDestinationRoot(capability.Application, snapshot.request)
			view := snapshot.Query(Filters{Context: FilterAll}, QuerySources)
			for _, source := range view.Sources {
				appView.ExistingSources = append(appView.ExistingSources, ConsolidationExistingSource{SourceID: source.ID, SourcePath: source.Directory, Name: source.Name, NativeName: source.Availability.NativeName, Availability: cloneAvailability(source.Availability), References: snapshot.References(source.ID)})
			}
			result.Problems = append(result.Problems, snapshot.Problems()...)
			if !snapshot.Complete() {
				result.Coverage = Incomplete
				if selected[capability.Application] {
					result.SelectedCoverage = Incomplete
				}
			}
		}
		result.Applications = append(result.Applications, appView)
	}
	if err := ctx.Err(); err != nil {
		result.Coverage = Incomplete
		return consolidationFailedScan(result, "scan-interrupted", err.Error(), "Retry the complete scan before preparing a consolidation.")
	}
	selectedSnapshots := map[SourceID]Source{}
	for _, entry := range all {
		if entry.err != nil || !selected[entry.app] {
			continue
		}
		view := entry.snapshot.Query(Filters{Context: FilterAll}, QuerySources)
		for _, source := range view.Sources {
			personal := false
			for _, rid := range source.References {
				ref, ok := entry.snapshot.references[rid]
				if ok && (ref.Context == ContextGlobal || ref.Context == ContextCustom) && consolidationReferenceIsSelectedSource(entry.app, ref, rootOrigin(entry.snapshot, ref.Root)) {
					personal = true
				}
			}
			if !personal {
				contexts := map[ReferenceContext]bool{}
				for _, rid := range source.References {
					if ref, ok := entry.snapshot.references[rid]; ok {
						contexts[ref.Context] = true
					}
				}
				reason := "source has no selected personal global or custom reference"
				if contexts[ContextManaged] {
					reason = "managed sources are disclosed but are not migrated"
				} else if contexts[ContextProject] {
					reason = "project sources are disclosed but are not migrated"
				} else if contexts[ContextSystem] {
					reason = "system sources are disclosed but are not migrated"
				}
				result.Exclusions = append(result.Exclusions, ConsolidationExclusion{SourceID: source.ID, Path: source.Directory, Reason: reason})
				continue
			}
			selectedSnapshots[source.ID] = source
		}
	}
	ids := make([]SourceID, 0, len(selectedSnapshots))
	for id := range selectedSnapshots {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		base := selectedSnapshots[id]
		item := ConsolidationSource{ID: id, SourcePath: base.Directory, Identity: base.Identity, Name: base.Name, Description: base.Description, NativeName: base.Availability.NativeName, ManifestStatus: base.Manifest.Status, Ownership: base.Ownership, FingerprintCoverage: Incomplete, Resources: append([]Resource(nil), base.Resources...), SelectedReferences: []ConsolidationReference{}, KnownReferences: []ConsolidationReference{}, Availability: map[string]AvailabilityExplanation{}, RelocationProofs: []ConsolidationRelocationProof{}, InspectionLimits: []string{}, BlockedReasons: []string{}, Eligible: false}
		for _, entry := range all {
			if entry.err != nil {
				continue
			}
			if source, ok := entry.snapshot.Source(id); ok {
				item.Availability[entry.app] = cloneAvailability(source.Availability)
				for _, ref := range entry.snapshot.References(id) {
					rootOrigin := ""
					for _, root := range entry.snapshot.scope.Roots {
						if root.ID == ref.Root {
							rootOrigin = root.Origin
							break
						}
					}
					reference := ConsolidationReference{Application: entry.app, Origin: rootOrigin, Reference: ref, Availability: cloneAvailability(source.Availability)}
					item.KnownReferences = append(item.KnownReferences, reference)
					if selected[entry.app] {
						item.SelectedReferences = append(item.SelectedReferences, reference)
					}
				}
			}
		}
		for _, app := range result.Applications {
			availability, ok := item.Availability[app.Application]
			if app.Application == "codex" && ok && consolidationCodexDisableProof(item, app.Application, app, availability) {
				item.RelocationProofs = append(item.RelocationProofs, codexRelocationProof())
			}
		}
		var relocationBlock string
		item.RelocationScope, relocationBlock = consolidationRelocationAuthority(base.Directory, item.SelectedReferences, selected, all)
		if base.Identity.Certainty != "proven" || base.Identity.Method != "darwin-directory-object" {
			item.BlockedReasons = append(item.BlockedReasons, "source is not a proven bundle directory")
		}
		if base.Ownership.State == "managed" {
			item.BlockedReasons = append(item.BlockedReasons, "source has a known managed owner")
		}
		for _, reference := range item.KnownReferences {
			switch reference.Reference.Context {
			case ContextManaged:
				item.BlockedReasons = append(item.BlockedReasons, "source has a managed reference through "+reference.Application+" and will not be migrated")
			case ContextSystem:
				item.BlockedReasons = append(item.BlockedReasons, "source has a system reference through "+reference.Application+" and will not be migrated")
			case ContextProject:
				item.BlockedReasons = append(item.BlockedReasons, "source has a project reference through "+reference.Application+" and will not be migrated")
			}
		}
		if base.Manifest.Status != ManifestParsed {
			item.BlockedReasons = append(item.BlockedReasons, "SKILL.md is not completely parsed")
		}
		fingerprint, coverage, inspectable, reasons, limits := consolidationFingerprint(ctx, base)
		item.BundleFingerprint, item.FingerprintCoverage, item.InspectionComplete = fingerprint, coverage, inspectable
		item.InspectionLimits = limits
		item.BlockedReasons = append(item.BlockedReasons, reasons...)
		// A previously consolidated bundle is an authorized source on an
		// unchanged rerun only when its complete digest names its exact library
		// path. Arbitrary bundles merely placed under the library remain blocked.
		if relocationBlock != "" && fingerprint != "" && filepath.Clean(base.Directory) == filepath.Clean(filepath.Join(request.Library, fingerprint)) {
			item.RelocationScope = "consolidation-library"
			relocationBlock = ""
		}
		if relocationBlock != "" {
			item.BlockedReasons = append(item.BlockedReasons, relocationBlock)
		}
		sort.Strings(item.BlockedReasons)
		item.BlockedReasons = uniqueStrings(item.BlockedReasons)
		item.Eligible = len(item.BlockedReasons) == 0
		if !item.Eligible {
			result.Exclusions = append(result.Exclusions, ConsolidationExclusion{SourceID: id, Path: base.Directory, Reason: strings.Join(item.BlockedReasons, "; ")})
		}
		result.Sources = append(result.Sources, item)
	}
	// Broken references and non-selected context are retained as explicit scope exclusions.
	for _, entry := range all {
		if entry.err != nil {
			continue
		}
		for _, ref := range entry.snapshot.references {
			if ref.Source == "" {
				result.Exclusions = append(result.Exclusions, ConsolidationExclusion{Path: ref.DiscoveredPath, Reason: "unresolved discovery reference; no source was fabricated"})
			}
		}
	}
	result.DestinationAudits = consolidationDestinationAudits(ctx, all)
	result.Conflicts = consolidationConflicts(result.Sources)
	for _, app := range result.Applications {
		for _, root := range app.Scope.Roots {
			if relation := setupPathsOverlap(request.Library, root.Requested); relation != deleteUnrelated {
				result.LibrarySafe = false
				result.LibraryReasons = append(result.LibraryReasons, fmt.Sprintf("library overlaps %s %s root %s", app.Application, root.Context, root.Requested))
			}
		}
	}
	if len(result.LibraryReasons) == 0 {
		result.LibrarySafe = true
	}
	backupRoot := consolidationBackupRoot()
	if resolved, err := setupProspectivePath(request.Library); err != nil || filepath.Clean(resolved) != filepath.Clean(request.Library) {
		result.LibraryReasons = append(result.LibraryReasons, "library path or one of its ancestors is an alias or unresolved; use its canonical no-follow path")
	}
	if resolved, err := setupProspectivePath(backupRoot); err != nil || filepath.Clean(resolved) != filepath.Clean(backupRoot) {
		result.LibraryReasons = append(result.LibraryReasons, "private recovery store path or one of its ancestors is an alias or unresolved")
	}
	if relation := setupPathsOverlap(request.Library, backupRoot); relation != deleteUnrelated {
		result.LibrarySafe = false
		result.LibraryReasons = append(result.LibraryReasons, "library overlaps its private recovery store")
	}
	for _, app := range result.Applications {
		for _, root := range app.Scope.Roots {
			if setupPathsOverlap(backupRoot, root.Requested) != deleteUnrelated {
				result.LibrarySafe = false
				result.LibraryReasons = append(result.LibraryReasons, fmt.Sprintf("private recovery store overlaps %s discovery root %s", app.Application, root.Requested))
			}
		}
	}
	for _, root := range request.Roots {
		if setupPathsOverlap(request.Library, root) != deleteUnrelated {
			result.LibrarySafe = false
			result.LibraryReasons = append(result.LibraryReasons, "library overlaps an explicit custom discovery root: "+root)
		}
	}
	location := AuditSetupLocation(ctx, request.Library)
	location.Refusals, location.Warnings = consolidationLibraryRouteNotes(request.Library, result.Sources, location.Refusals, location.Warnings)
	for _, refusal := range location.Refusals {
		result.LibraryReasons = append(result.LibraryReasons, "consumer audit: "+refusal)
	}
	for _, warning := range location.Warnings {
		result.LibraryWarnings = append(result.LibraryWarnings, warning)
	}
	result.LibraryReasons = append(result.LibraryReasons, consolidationStateOutsideRoots(request, result.Applications)...)
	sort.Strings(result.LibraryReasons)
	result.LibraryReasons = uniqueStrings(result.LibraryReasons)
	result.LibraryWarnings = uniqueStrings(result.LibraryWarnings)
	result.LibrarySafe = len(result.LibraryReasons) == 0
	result.Problems = uniqueProblems(result.Problems)
	result.ScanID = consolidationScanID(result, all)
	if result.Coverage == Incomplete {
		result.Outcome, result.ExitCode = "partial", 1
	}
	return result
}

func consolidationLibraryRouteNotes(library string, sources []ConsolidationSource, refusals, warnings []string) ([]string, []string) {
	const marker = ": prospective alias isolation is unverified: related or unresolved alias "
	var remaining []string
	for _, refusal := range refusals {
		index := strings.Index(refusal, marker)
		if index < 0 {
			remaining = append(remaining, refusal)
			continue
		}
		relation := refusal[index+len(marker):]
		parts := strings.SplitN(relation, " targets ", 2)
		if len(parts) != 2 {
			remaining = append(remaining, refusal)
			continue
		}
		aliasPath := parts[0]
		target := strings.TrimSuffix(parts[1], "; unsupported isolation")
		approved := false
		for _, source := range sources {
			if !source.Eligible || len(source.BundleFingerprint) != sha256.Size*2 || filepath.Clean(source.SourcePath) != filepath.Clean(target) || filepath.Clean(filepath.Join(library, source.BundleFingerprint)) != filepath.Clean(target) {
				continue
			}
			if _, err := hex.DecodeString(source.BundleFingerprint); err != nil {
				continue
			}
			for _, reference := range source.KnownReferences {
				if filepath.Clean(reference.Reference.DiscoveredPath) != filepath.Clean(aliasPath) || filepath.Clean(reference.Reference.ResolvedPath) != filepath.Clean(target) || reference.Reference.Status != "resolved" {
					continue
				}
				info, err := os.Lstat(aliasPath)
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					continue
				}
				resolved, err := filepath.EvalSymlinks(aliasPath)
				if err == nil && filepath.Clean(resolved) == filepath.Clean(target) {
					approved = true
					warnings = append(warnings, "existing application reference remains linked to its exact complete canonical bundle and is not changed: "+aliasPath)
					break
				}
			}
			if approved {
				break
			}
		}
		if !approved {
			remaining = append(remaining, refusal)
		}
	}
	return uniqueStrings(remaining), uniqueStrings(warnings)
}

func consolidationFailedScan(result ConsolidationInventory, code, message, action string) ConsolidationInventory {
	result.Outcome, result.ExitCode = "failed", 2
	result.Failure = &ConsolidationFailure{Code: code, Message: message, Action: action}
	return result
}

func consolidationDestinationAudits(ctx context.Context, snapshots []consolidationSnapshot) []ConsolidationDestinationAudit {
	byApp := make(map[string]consolidationSnapshot, len(snapshots))
	for _, entry := range snapshots {
		byApp[entry.app] = entry
	}
	var audits []ConsolidationDestinationAudit
	for _, capability := range applicationCatalog {
		destinationEntry := byApp[capability.Application]
		if destinationEntry.err != nil || destinationEntry.snapshot == nil {
			continue
		}
		destinationRoot := consolidationDestinationRoot(capability.Application, destinationEntry.snapshot.request)
		if destinationRoot == "" {
			continue
		}
		location := AuditSetupLocation(ctx, destinationRoot)
		if resolved, err := setupProspectivePath(destinationRoot); err != nil || filepath.Clean(resolved) != filepath.Clean(destinationRoot) {
			location.Refusals = append(location.Refusals, "destination path or one of its ancestors is an alias or unresolved; no-follow creation is unsupported")
		}
		location.Refusals, location.Warnings = consolidationDestinationRouteNotes(destinationRoot, location.Refusals, location.Warnings)
		audit := ConsolidationDestinationAudit{
			Application: capability.Application, DestinationRoot: destinationRoot,
			PotentialConsumers: []ConsolidationConsumerRoute{},
			Refusals:           uniqueStrings(location.Refusals), Warnings: uniqueStrings(location.Warnings),
		}
		for _, consumer := range location.Consumers {
			entry := byApp[consumer]
			if entry.err != nil || entry.snapshot == nil {
				audit.Refusals = append(audit.Refusals, consumer+": possible destination consumer inventory is unavailable: "+fmt.Sprint(entry.err))
				continue
			}
			route := consolidationConsumerRoute(ctx, consumer, destinationRoot, entry.snapshot)
			audit.PotentialConsumers = append(audit.PotentialConsumers, route)
		}
		sort.Slice(audit.PotentialConsumers, func(i, j int) bool {
			return audit.PotentialConsumers[i].Application < audit.PotentialConsumers[j].Application
		})
		audits = append(audits, audit)
	}
	return audits
}

func consolidationConsumerRoute(ctx context.Context, app, destination string, snapshot *Snapshot) ConsolidationConsumerRoute {
	route := ConsolidationConsumerRoute{
		Application: app, InstallationState: "unknown", RuntimeState: "unknown",
		RootIDs: []RootID{}, RootPaths: []string{},
		Reason: "native loader evidence identifies a possible discovery route; installation, actual use, and running-session adoption are unknown",
	}
	if app == "codeaf" {
		route.Reason = "recorded CodeAF loader evidence identifies possible indirect exposure; installed discovery and runtime use are unknown"
	}
	type fact struct {
		Application string
		Destination string
		Scope       Scope
		References  []Reference
		Problems    []Problem
		Managers    []ManagerEvidence
	}
	evidence := fact{Application: app, Destination: destination, Scope: snapshot.Scope(), Problems: snapshot.Problems(), Managers: append([]ManagerEvidence(nil), snapshot.managers...)}
	rootSeen, pathSeen := map[RootID]bool{}, map[string]bool{}
	for _, root := range snapshot.scope.Roots {
		if !setupRootParticipates(root) {
			continue
		}
		path := root.Requested
		if root.Resolved != "" {
			path = root.Resolved
		}
		if setupPathsOverlap(destination, path) != deleteUnrelated {
			if !rootSeen[root.ID] {
				route.RootIDs = append(route.RootIDs, root.ID)
				rootSeen[root.ID] = true
			}
			if !pathSeen[root.Requested] {
				route.RootPaths = append(route.RootPaths, root.Requested)
				pathSeen[root.Requested] = true
			}
		}
		var warnings []string
		aliases, err := setupAliasEvidence(ctx, root.Requested, destination, &warnings)
		if err == nil {
			for _, alias := range aliases {
				aliasPath := alias.Requested
				if alias.Resolved != "" {
					aliasPath += " -> " + alias.Resolved
				}
				if !pathSeen[aliasPath] {
					route.RootPaths = append(route.RootPaths, aliasPath)
					pathSeen[aliasPath] = true
				}
			}
		} else {
			route.RootPaths = append(route.RootPaths, "unresolved route under "+root.Requested)
		}
	}
	view := snapshot.Query(Filters{Context: FilterAll}, QuerySources)
	for _, reference := range view.References {
		if reference.ResolvedPath == "" || setupPathsOverlap(destination, filepath.Dir(reference.ResolvedPath)) != deleteRelated {
			continue
		}
		evidence.References = append(evidence.References, reference)
		if !rootSeen[reference.Root] {
			route.RootIDs = append(route.RootIDs, reference.Root)
			rootSeen[reference.Root] = true
		}
		if !pathSeen[reference.ResolvedPath] {
			route.RootPaths = append(route.RootPaths, reference.ResolvedPath)
			pathSeen[reference.ResolvedPath] = true
		}
	}
	sort.Slice(route.RootIDs, func(i, j int) bool { return route.RootIDs[i] < route.RootIDs[j] })
	sort.Strings(route.RootPaths)
	encoded, err := json.Marshal(evidence)
	if err == nil {
		digest := sha256.Sum256(encoded)
		route.EvidenceID = "route1:" + hex.EncodeToString(digest[:])
	}
	return route
}

func consolidationRelocationAuthority(sourcePath string, selectedReferences []ConsolidationReference, selected map[string]bool, snapshots []consolidationSnapshot) (string, string) {
	selectedCustom := false
	selectedPersonalRoot := false
	selectedDirectReference := false
	for _, reference := range selectedReferences {
		if (reference.Reference.Context == ContextGlobal || reference.Reference.Context == ContextCustom) && consolidationReferenceIsSelectedSource(reference.Application, reference.Reference, reference.Origin) && filepath.Clean(reference.Reference.DiscoveredPath) == filepath.Clean(sourcePath) {
			selectedDirectReference = true
		}
	}
	var excludedRoots []string
	for _, entry := range snapshots {
		if entry.err != nil || entry.snapshot == nil {
			continue
		}
		for _, root := range entry.snapshot.scope.Roots {
			if !setupRootParticipates(root) || root.Context != ContextGlobal && root.Context != ContextCustom {
				continue
			}
			selectedCLIOverride := selected[entry.app] && root.Context == ContextCustom && root.Origin == "" && strings.HasPrefix(string(root.ID), "root-custom-")
			if !selectedCLIOverride && !consolidationRootIsPhysicalPersonalRoot(entry.app, root) {
				continue
			}
			physicalRoot := root.Resolved
			if physicalRoot == "" {
				physicalRoot = root.Requested
			}
			if !pathWithin(physicalRoot, sourcePath) {
				continue
			}
			if selected[entry.app] {
				selectedPersonalRoot = true
				if root.Context == ContextCustom && (root.Origin == "custom" || root.Origin == "" && strings.HasPrefix(string(root.ID), "root-custom-")) {
					selectedCustom = true
				}
			} else {
				excludedRoots = append(excludedRoots, entry.app+" "+physicalRoot)
			}
		}
	}
	if len(excludedRoots) > 0 {
		sort.Strings(excludedRoots)
		return "unselected-application-root", "source bundle is physically inside an unselected application's personal root; moving it would change that excluded application's source location: " + strings.Join(uniqueStrings(excludedRoots), "; ")
	}
	if selectedCustom {
		return "explicit-selected-custom-root", ""
	}
	if selectedDirectReference {
		return "selected-personal-source", ""
	}
	if selectedPersonalRoot {
		return "selected-personal-root-alias", ""
	}
	return "outside-selected-personal-roots", "source bundle is outside every selected personal source root; a selected alias alone does not authorize relocating another application's or external source"
}

func rootOrigin(snapshot *Snapshot, id RootID) string {
	if snapshot == nil {
		return ""
	}
	for _, root := range snapshot.scope.Roots {
		if root.ID == id {
			return root.Origin
		}
	}
	return ""
}

// consolidationRootIsPhysicalPersonalRoot limits excluded-app ownership to
// that application's own personal directory. Several loaders intentionally
// scan another application's folder; those routes are consumers, not owners.
func consolidationRootIsPhysicalPersonalRoot(app string, root RootReport) bool {
	if root.Context != ContextGlobal && root.Context != ContextCustom {
		return false
	}
	switch app {
	case "codex":
		return root.Origin == "codex-compatibility-user"
	case "claude-code":
		return root.Origin == "claude-personal"
	case "hermes":
		return root.Origin == "hermes-profile"
	case "pi":
		return root.Origin == "pi-auto-global-pi"
	case "opencode":
		return root.Origin == "opencode-native-directory"
	case "gemini":
		return root.Origin == "gemini-user.gemini"
	case "codeaf":
		// CodeAF enumerates many compatible roots. Only its own profile folder
		// is an owned source location; e.g. .codex/.claude/.agents are consumers.
		path := filepath.ToSlash(root.Requested)
		return strings.HasSuffix(path, "/.codeaf/skills")
	default:
		return false
	}
}

func consolidationOriginIsNativeForApp(app, origin string) bool {
	if origin == "" {
		return false
	}
	if app == "opencode" && strings.HasPrefix(origin, "opencode-external") {
		return false
	}
	return true
}

func consolidationReferenceIsSelectedSource(app string, reference Reference, origin string) bool {
	if reference.Context == ContextCustom && origin == "" && strings.HasPrefix(string(reference.Root), "root-custom-") {
		return true // an explicitly supplied CLI custom root
	}
	return consolidationOriginIsNativeForApp(app, origin)
}

func consolidationDestinationRoot(app string, request Request) string {
	switch app {
	case "codex":
		return filepath.Join(request.codexHome, "skills")
	case "claude-code":
		return filepath.Join(request.claudeHome, "skills")
	case "pi":
		if path, err := piAgentDir(request.home); err == nil {
			return filepath.Join(path, "skills")
		}
	case "opencode":
		return filepath.Join(opencodeConfigHome(request.home), "opencode", "skills")
	case "hermes":
		return filepath.Join(request.hermesHome, "skills")
	case "gemini":
		return filepath.Join(request.home, ".gemini", "skills")
	}
	return ""
}

func consolidationFingerprint(ctx context.Context, source Source) (string, Completeness, bool, []string, []string) {
	var reasons []string
	var inspectionLimits []string
	if source.Identity.Certainty != "proven" || source.Identity.Method != "darwin-directory-object" {
		return "", Incomplete, false, []string{"full bundle identity is not proven"}, nil
	}
	if source.Manifest.IsSymlink {
		return "", Incomplete, false, []string{"linked SKILL.md relocation is unsupported; content target remains unchanged"}, nil
	}
	if source.Manifest.Status != ManifestParsed {
		return "", Incomplete, false, []string{"SKILL.md is not fully readable and parsed"}, nil
	}
	for _, resource := range source.Resources {
		if resource.Kind == "symlink" {
			if reason := relocatableBundleLink(source.Directory, resource.Path, resource.LinkTarget); reason != "" {
				reasons = append(reasons, "resource link "+resource.Path+": "+reason)
			}
		}
		if resource.Kind != "file" && resource.Kind != "directory" && resource.Kind != "symlink" {
			reasons = append(reasons, "unsupported special resource "+resource.Path+" ("+resource.Kind+")")
		}
		if resource.Kind == "file" && resource.Size > ResourceReadLimit {
			inspectionLimits = append(inspectionLimits, fmt.Sprintf("resource %s is %d bytes and exceeds read-resource's %d-byte content limit; bundle identity remains fully fingerprinted", resource.Path, resource.Size, ResourceReadLimit))
		}
	}
	if len(reasons) > 0 {
		return "", Incomplete, false, uniqueStrings(reasons), uniqueStrings(inspectionLimits)
	}
	observed := observeBundle(ctx, source, "source", nil)
	if !observed.proven || len(observed.issues) > 0 {
		for _, issue := range observed.issues {
			reasons = append(reasons, fmt.Sprintf("bundle observation %s: %s", issue.Path, issue.Reason))
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "bundle observation is incomplete")
		}
		return "", Incomplete, false, uniqueStrings(reasons), uniqueStrings(inspectionLimits)
	}
	type entryFact struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
		Mode uint16 `json:"mode"`
		Size int64  `json:"size"`
		Hash string `json:"sha256,omitempty"`
		Link string `json:"link,omitempty"`
	}
	paths := make([]string, 0, len(observed.entries))
	for path := range observed.entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	facts := make([]entryFact, 0, len(paths))
	for _, path := range paths {
		entry := observed.entries[path]
		fact := entryFact{Path: path, Kind: entryKind(entry.Stamp.Mode), Mode: entry.Stamp.Mode, Size: entry.Stamp.Size, Link: entry.Link}
		if entry.Stamp.Mode&unix.S_IFMT == unix.S_IFREG {
			hash := sha256.Sum256(entry.Data)
			fact.Hash = hex.EncodeToString(hash[:])
		}
		facts = append(facts, fact)
	}
	encoded, err := json.Marshal(facts)
	if err != nil {
		return "", Incomplete, false, []string{"bundle fingerprint cannot be encoded: " + err.Error()}, uniqueStrings(inspectionLimits)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), Complete, len(inspectionLimits) == 0, nil, uniqueStrings(inspectionLimits)
}

// relocatableBundleLink rejects any chain that leaves the source bundle or
// depends on an absolute path. Relative links wholly inside the bundle retain
// their meaning when the whole directory is moved as one unit.
func relocatableBundleLink(root, relative, rawTarget string) string {
	if filepath.IsAbs(rawTarget) {
		return "absolute resource links change meaning when the bundle is relocated"
	}
	start := filepath.Clean(filepath.Join(root, filepath.Dir(filepath.FromSlash(relative)), rawTarget))
	rel, err := filepath.Rel(root, start)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "relative target escapes the bundle"
	}
	pending := strings.Split(filepath.ToSlash(rel), "/")
	resolved := []string{}
	links := 0
	seen := map[string]bool{}
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(resolved) == 0 {
				return "link chain escapes the bundle"
			}
			resolved = resolved[:len(resolved)-1]
			continue
		}
		resolved = append(resolved, part)
		path := filepath.Join(append([]string{root}, resolved...)...)
		info, err := os.Lstat(path)
		if err != nil {
			return "link target is broken or unreadable: " + err.Error()
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if len(pending) > 0 && !info.IsDir() {
				return "link chain traverses a non-directory resource"
			}
			continue
		}
		links++
		if links > 40 || seen[path] {
			return "link chain is cyclic or exceeds 40 links"
		}
		seen[path] = true
		target, err := os.Readlink(path)
		if err != nil {
			return "link target could not be read: " + err.Error()
		}
		if filepath.IsAbs(target) {
			return "link chain contains an absolute target"
		}
		resolved = resolved[:len(resolved)-1]
		targetParts := append([]string{root}, resolved...)
		targetPath := filepath.Clean(filepath.Join(filepath.Join(targetParts...), target))
		targetRelative, err := filepath.Rel(root, targetPath)
		if err != nil || targetRelative == ".." || strings.HasPrefix(targetRelative, ".."+string(filepath.Separator)) {
			return "link chain escapes the bundle"
		}
		pending = append(strings.Split(filepath.ToSlash(targetRelative), "/"), pending...)
		resolved = nil
	}
	return ""
}

func consolidationConflicts(sources []ConsolidationSource) []ConsolidationConflict {
	byName := map[string][]ConsolidationSource{}
	byFingerprint := map[string][]ConsolidationSource{}
	for _, source := range sources {
		if !source.Eligible {
			continue
		}
		name := source.NativeName
		if name == "" {
			name = source.Name
		}
		if name != "" {
			byName[strings.ToLower(name)] = append(byName[strings.ToLower(name)], source)
		}
		byFingerprint[source.BundleFingerprint] = append(byFingerprint[source.BundleFingerprint], source)
	}
	var out []ConsolidationConflict
	for name, group := range byName {
		if len(group) < 2 {
			continue
		}
		ids := make([]SourceID, 0, len(group))
		fingerprints := map[string]bool{}
		for _, source := range group {
			ids = append(ids, source.ID)
			fingerprints[source.BundleFingerprint] = true
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		kind := "divergent-name"
		reason := "same native name has multiple distinct complete bundle contents"
		if len(fingerprints) == 1 {
			kind = "identical-bundle-copies"
			reason = "same native name has multiple complete byte-and-mode-identical bundle copies"
		}
		out = append(out, ConsolidationConflict{ID: "conflict_" + stableID("consolidation-name-v1", name, strings.Join(sourceIDs(ids), ",")), Name: name, Kind: kind, SourceIDs: ids, Reason: reason})
	}
	for fingerprint, group := range byFingerprint {
		if len(group) < 2 {
			continue
		}
		ids := make([]SourceID, 0, len(group))
		for _, source := range group {
			ids = append(ids, source.ID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if fingerprint == "" {
			continue
		}
		id := "conflict_" + stableID("consolidation-identical-v1", fingerprint)
		out = append(out, ConsolidationConflict{ID: id, Name: group[0].Name, Kind: "identical-bundle-copies", SourceIDs: ids, Reason: "complete bundle contents, resource paths, modes and link targets are identical"})
		for _, source := range group {
			for i := range sources {
				if sources[i].ID == source.ID {
					sources[i].ConflictGroup = id
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sourceIDs(ids []SourceID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func uniqueProblems(values []Problem) []Problem {
	seen := map[ProblemID]bool{}
	out := make([]Problem, 0, len(values))
	for _, value := range values {
		if !seen[value.ID] {
			seen[value.ID] = true
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func consolidationScanID(inventory ConsolidationInventory, snapshots []consolidationSnapshot) string {
	type appFact struct {
		App           string
		Scope         Scope
		Problems      []Problem
		ConfigDigests []string
		Environment   map[string]string
		Sources       []struct {
			ID       SourceID
			Path     string
			Identity Identity
			Name     string
			Owner    EvidenceState
			Refs     []Reference
		}
		Managers []ManagerEvidence
	}
	var facts []appFact
	for _, entry := range snapshots {
		if entry.err != nil {
			facts = append(facts, appFact{App: entry.app, Problems: []Problem{{Path: entry.app, Detail: entry.err.Error()}}})
			continue
		}
		view := entry.snapshot.Query(Filters{Context: FilterAll}, QuerySources)
		fact := appFact{App: entry.app, Scope: entry.snapshot.Scope(), Problems: entry.snapshot.Problems(), Managers: append([]ManagerEvidence(nil), entry.snapshot.managers...), Environment: map[string]string{}}
		if entry.snapshot.scope.Agent != nil {
			for _, layer := range entry.snapshot.scope.Agent.ConfigLayers {
				if layer.Path == "" {
					continue
				}
				observations, observationErr := observeDeleteInput(layer.Path)
				encodedObservation, encodeErr := json.Marshal(observations)
				if observationErr != nil || encodeErr != nil {
					fact.ConfigDigests = append(fact.ConfigDigests, layer.Path+":unknown:"+fmt.Sprint(observationErr, encodeErr))
					continue
				}
				digest := sha256.Sum256(encodedObservation)
				fact.ConfigDigests = append(fact.ConfigDigests, layer.Path+":"+hex.EncodeToString(digest[:]))
			}
		}
		for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT", "OPENCODE_DISABLE_EXTERNAL_SKILLS", "OPENCODE_DISABLE_CLAUDE_CODE", "OPENCODE_DISABLE_CLAUDE_CODE_SKILLS", "OPENCODE_DISABLE_PROJECT_CONFIG", "OPENCODE_PURE", "HERMES_HOME", "HERMES_SAFE_MODE", "HERMES_ENABLE_PROJECT_PLUGINS", "GEMINI_CLI_HOME", "GEMINI_CLI_SYSTEM_SETTINGS_PATH", "GEMINI_CLI_TRUSTED_FOLDERS_PATH", "CODEAF_HOME", "CODEAF_PROFILE_DIR"} {
			fact.Environment[key] = os.Getenv(key)
		}
		for _, source := range view.Sources {
			item := struct {
				ID       SourceID
				Path     string
				Identity Identity
				Name     string
				Owner    EvidenceState
				Refs     []Reference
			}{ID: source.ID, Path: source.Directory, Identity: source.Identity, Name: source.Name, Owner: source.Ownership, Refs: entry.snapshot.References(source.ID)}
			fact.Sources = append(fact.Sources, item)
		}
		facts = append(facts, fact)
	}
	encoded, err := json.Marshal(struct {
		Version           int
		Scope             ConsolidationRequest
		Apps              []appFact
		Sources           []ConsolidationSource
		DestinationAudits []ConsolidationDestinationAudit
	}{ConsolidationSchemaVersion, inventory.Scope, facts, inventory.Sources, inventory.DestinationAudits})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return "scan1:" + hex.EncodeToString(sum[:])
}

// PreviewConsolidation rebuilds every selected and known-consumer inventory.
// The draft is usable only when its scan ID still matches the complete current
// scope. It writes no files and returns the exact confirmation token for apply.
func PreviewConsolidation(ctx context.Context, draft ConsolidationDraft) ConsolidationPreview {
	preview := ConsolidationPreview{SchemaVersion: ConsolidationSchemaVersion, Operation: "consolidate-preview", Outcome: "failed", ExitCode: 2, State: "refused", Reason: "proposal is not ready", Choices: []ConsolidationChoice{}, Effects: []ConsolidationEffect{}, Warnings: []string{}}
	if draft.SchemaVersion != ConsolidationSchemaVersion {
		return consolidationPreviewFailure(preview, "unsupported-schema", "draft schema_version must be 1", "Create a fresh draft from the current consolidate scan output.")
	}
	request, err := NormalizeConsolidationRequest(draft.Scope.Applications, draft.Scope.Roots, draft.Scope.Library, draft.Scope.SelectionFrom)
	if err != nil {
		return consolidationPreviewFailure(preview, "invalid-scope", err.Error(), "Use the exact application, root and library scope from a fresh scan.")
	}
	request.RememberedSelectionID = draft.Scope.RememberedSelectionID
	if request.RememberedSelectionID != "" {
		currentID, selectionErr := CurrentConsolidationSelectionID()
		if selectionErr != nil || currentID != request.RememberedSelectionID {
			return consolidationPreviewFailure(preview, "stale-selection", "last-successful application selection changed or is unavailable", "Repeat scan with explicit applications, roots and library, then rewrite the draft.")
		}
	} else if strings.EqualFold(request.SelectionFrom, "remembered") {
		return consolidationPreviewFailure(preview, "stale-selection", "remembered-selection scan has no selection evidence ID", "Repeat scan with the current CLI and rewrite the complete draft.")
	}
	preview.Scope = request
	preview.Choices = cloneConsolidationChoices(draft.Choices)
	inventory := ScanConsolidation(ctx, request)
	preview.ScanID = inventory.ScanID
	if inventory.Failure != nil {
		return consolidationPreviewFailure(preview, inventory.Failure.Code, inventory.Failure.Message, inventory.Failure.Action)
	}
	if inventory.SelectedCoverage != Complete {
		return consolidationPreviewFailure(preview, "incomplete-selected-scan", "one or more selected application inventories are incomplete", "Review selected app failures, roots and problems, then resolve the evidence gap and rescan.")
	}
	if !inventory.LibrarySafe {
		return consolidationPreviewFailure(preview, "unsafe-library", strings.Join(inventory.LibraryReasons, "; "), "Choose a dedicated path outside every discovered consumer root and custom source root.")
	}
	if draft.ScanID == "" || draft.ScanID != inventory.ScanID {
		return consolidationPreviewFailure(preview, "stale-scan", "full selected-app and consumer evidence changed since the draft was authored", "Run a fresh scan, reinspect the complete inventory and rewrite the draft.")
	}
	if inventory.Scope.Library == "" {
		return consolidationPreviewFailure(preview, "unsafe-library", "canonical library path is unavailable", "Select a dedicated path outside every discovered consumer root.")
	}
	byID := make(map[SourceID]ConsolidationSource, len(inventory.Sources))
	for _, source := range inventory.Sources {
		byID[source.ID] = source
	}
	choices := make(map[SourceID]ConsolidationChoice, len(draft.Choices))
	for _, choice := range draft.Choices {
		if _, duplicate := choices[choice.SourceID]; duplicate {
			return consolidationPreviewFailure(preview, "duplicate-choice", "draft repeats source_id "+string(choice.SourceID), "Keep one explicit action for each eligible source.")
		}
		choices[choice.SourceID] = choice
	}
	for _, source := range inventory.Sources {
		if !source.Eligible {
			if _, supplied := choices[source.ID]; supplied {
				return consolidationPreviewFailure(preview, "ineligible-source", "draft selects source "+string(source.ID)+" with incomplete or unsupported evidence", "Resolve its listed blocked_reasons and rescan, or omit it from the proposal.")
			}
			continue
		}
		if _, supplied := choices[source.ID]; !supplied {
			return consolidationPreviewFailure(preview, "unresolved-choice", "draft omits eligible source "+string(source.ID), "Choose share, archive, or keep explicitly for every eligible source.")
		}
		choice := choices[source.ID]
		if choice.SourcePath != source.SourcePath || choice.BundleFingerprint != source.BundleFingerprint {
			return consolidationPreviewFailure(preview, "source-binding-mismatch", "every choice must bind the exact current source_path and complete bundle_fingerprint", "Copy both fields from the current scan, including keep/archive choices.")
		}
	}
	for id := range choices {
		if _, exists := byID[id]; !exists {
			return consolidationPreviewFailure(preview, "unknown-source", "draft source_id is absent from the full current inventory: "+string(id), "Use source IDs from the exact current scan; do not select by name.")
		}
	}
	appByName := map[string]ConsolidationApplication{}
	for _, app := range inventory.Applications {
		appByName[app.Application] = app
	}
	var effects []ConsolidationEffect
	planID := consolidationPlanID(draft)
	nativeNames := consolidationDestinationNames{}
	for _, source := range inventory.Sources {
		choice := choices[source.ID]
		switch choice.Action {
		case "keep":
			if len(choice.Destinations) != 0 {
				return consolidationPreviewFailure(preview, "invalid-choice", "keep choices cannot specify destinations", "Remove destinations from the keep choice.")
			}
			effects = append(effects, ConsolidationEffect{Action: "keep", SourceID: source.ID, SourcePath: source.SourcePath, State: "unchanged", Reason: "source and its current application references remain untouched"})
		case "archive":
			if !source.Eligible {
				return consolidationPreviewFailure(preview, "ineligible-source", "cannot archive a source with incomplete or unsupported relocation evidence", "Keep it in place until the listed blocked evidence is resolved.")
			}
			for _, reference := range source.KnownReferences {
				if !containsString(request.Applications, reference.Application) {
					return consolidationPreviewFailure(preview, "external-reference", "archive would break an existing reference from unselected application "+reference.Application+" at "+reference.Reference.DiscoveredPath, "Keep the source or explicitly resolve that application's existing reference before preparing a new proposal.")
				}
			}
			if len(choice.Destinations) != 0 || choice.CanonicalName != "" {
				return consolidationPreviewFailure(preview, "invalid-choice", "archive choices cannot specify a canonical name or destinations", "Use archive only for a divergent version the user chose to retain for recovery.")
			}
			archivePath := filepath.Join(consolidationBackupRoot(), planID, "archived", string(source.ID))
			if pathExists(archivePath) {
				return consolidationPreviewFailure(preview, "backup-collision", "planned recovery path already exists: "+archivePath, "Start a fresh proposal after inspecting the existing recovery record.")
			}
			effects = append(effects, ConsolidationEffect{Action: "archive", SourceID: source.ID, SourcePath: source.SourcePath, BundleFingerprint: source.BundleFingerprint, BackupPath: archivePath, State: "move-to-recovery"})
		case "share":
			if !source.Eligible || choice.SourcePath != source.SourcePath || choice.BundleFingerprint != source.BundleFingerprint {
				return consolidationPreviewFailure(preview, "source-binding-mismatch", "share choice does not bind the current source path and complete bundle fingerprint", "Use the exact source_id, source_path and bundle_fingerprint from scan.")
			}
			if !consolidationName.MatchString(choice.CanonicalName) {
				return consolidationPreviewFailure(preview, "invalid-canonical-name", "canonical_name must be a lowercase native-safe leaf of 1 to 64 letters, digits or hyphens", "Choose a valid folder name without changing the original bundle metadata.")
			}
			assessments, err := validateDestinationChoices(source, choice, request, appByName)
			if err != nil {
				return consolidationPreviewFailure(preview, "invalid-portability", err.Error(), "Assess every selected destination explicitly; unknown compatibility needs the user's explicit approval.")
			}
			canonicalPath := filepath.Join(request.Library, source.BundleFingerprint)
			if pathExists(canonicalPath) {
				existing, fingerprint, complete, reason := consolidationFingerprintPath(ctx, canonicalPath)
				if !complete || fingerprint != source.BundleFingerprint {
					return consolidationPreviewFailure(preview, "library-collision", "existing canonical library path does not contain the exact complete bundle: "+canonicalPath+": "+reason, "Inspect or move the existing object before preparing a fresh proposal.")
				}
				_ = existing
			}
			destinations := []string{}
			preservedReferences := []string{}
			suitableDestinations := 0
			for _, assessment := range assessments {
				if assessment.Portability == "incompatible" {
					if consolidationAlreadyConnected(source, assessment.Application) {
						preservedReferences = append(preservedReferences, consolidationReferencesForApp(source, assessment.Application)...)
					}
					continue
				}
				suitableDestinations++
				if consolidationAlreadyConnected(source, assessment.Application) {
					preservedReferences = append(preservedReferences, consolidationReferencesForApp(source, assessment.Application)...)
					continue
				}
				app := appByName[assessment.Application]
				if app.DestinationRoot == "" {
					return consolidationPreviewFailure(preview, "unsupported-destination", "no recorded per-skill destination root exists for "+assessment.Application, "Keep the source or use a supported application destination.")
				}
				destinationRoot := app.DestinationRoot
				destination := filepath.Join(destinationRoot, choice.CanonicalName)
				if err := validateDestinationConsumerSet(ctx, destinationRoot, destination, assessment.Application, request.Applications, assessments, inventory.DestinationAudits); err != nil {
					return consolidationPreviewFailure(preview, "unapproved-consumer", err.Error(), "Acknowledge every exact possible indirect consumer shown in the scan, or keep the source unchanged.")
				}
				if err := validateDestinationCollision(destination, source, choice.CanonicalName, inventory, choices); err != nil {
					return consolidationPreviewFailure(preview, "destination-collision", err.Error(), "Resolve the existing native skill conflict explicitly, then rescan.")
				}
				name := source.Availability[assessment.Application].NativeName
				if name == "" {
					name = source.NativeName
				}
				if name == "" {
					name = source.Name
				}
				if name == "" {
					name = choice.CanonicalName
				}
				if err := nativeNames.add(assessment.Application, name, choice.CanonicalName, source.BundleFingerprint); err != nil {
					return consolidationPreviewFailure(preview, "native-name-conflict", err.Error(), "Choose one winner, or prepare separately reviewed content edits and rescan before retaining variants.")
				}
				destinations = append(destinations, destination)
			}
			if suitableDestinations == 0 {
				return consolidationPreviewFailure(preview, "no-compatible-destination", "share choice has no compatible approved destination", "Choose keep/archive or explicitly assess a compatible selected destination.")
			}
			backupPath := filepath.Join(consolidationBackupRoot(), planID, "originals", string(source.ID))
			if pathExists(backupPath) {
				return consolidationPreviewFailure(preview, "backup-collision", "planned recovery path already exists: "+backupPath, "Start a fresh proposal after inspecting the existing recovery record.")
			}
			state := "relocate-and-link"
			if len(destinations) == 0 {
				state = "relocate-and-preserve-existing-references"
			}
			effects = append(effects, ConsolidationEffect{Action: "share", SourceID: source.ID, SourcePath: source.SourcePath, BundleFingerprint: source.BundleFingerprint, CanonicalPath: canonicalPath, BackupPath: backupPath, Destinations: destinations, PreservedReferences: uniqueStrings(preservedReferences), State: state})
		default:
			return consolidationPreviewFailure(preview, "invalid-choice", "action must be share, archive, or keep", "Use one explicit action for every eligible source.")
		}
	}
	if err := validateConflictChoices(inventory.Sources, choices); err != nil {
		return consolidationPreviewFailure(preview, "unresolved-conflict", err.Error(), "Choose a winner, archive discarded versions, or retain variants only when native names are distinct.")
	}
	for _, app := range request.Applications {
		if !appByName[app].Selected {
			return consolidationPreviewFailure(preview, "scope-mismatch", "application selection changed during scan", "Run a fresh scan with the exact selected applications.")
		}
	}
	preview.Effects = effects
	preview.Warnings = consolidationWarnings(inventory, choices)
	preview.State, preview.Reason, preview.Outcome, preview.ExitCode = "ready", "complete proposal is fresh and all exact filesystem effects are inspectable", "success", 0
	seal, err := consolidationConfirmation(draft, inventory, effects)
	if err != nil {
		return consolidationPreviewFailure(preview, "confirmation-error", err.Error(), "Create a new complete draft and preview.")
	}
	preview.Confirmation = seal
	preview.RecoveryID = planID
	return preview
}

func consolidationPreviewFailure(preview ConsolidationPreview, code, message, action string) ConsolidationPreview {
	preview.State, preview.Reason, preview.Outcome, preview.ExitCode = "refused", message, "failed", 1
	preview.Failure = &ConsolidationFailure{Code: code, Message: message, Action: action}
	return preview
}

func cloneConsolidationChoices(values []ConsolidationChoice) []ConsolidationChoice {
	out := append([]ConsolidationChoice(nil), values...)
	for i := range out {
		out[i].Destinations = append([]ConsolidationDestinationChoice(nil), values[i].Destinations...)
		for j := range out[i].Destinations {
			out[i].Destinations[j].AcknowledgedConsumers = append([]ConsolidationConsumerAcknowledgment(nil), values[i].Destinations[j].AcknowledgedConsumers...)
			for k := range out[i].Destinations[j].AcknowledgedConsumers {
				out[i].Destinations[j].AcknowledgedConsumers[k].EvidenceIDs = append([]string(nil), values[i].Destinations[j].AcknowledgedConsumers[k].EvidenceIDs...)
			}
		}
	}
	return out
}

func consolidationFingerprintPath(ctx context.Context, path string) (Source, string, bool, string) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(path) {
		if err == nil {
			err = errors.New("canonical path is an alias")
		}
		return Source{}, "", false, err.Error()
	}
	token, err := directoryObjectToken(path)
	if err != nil {
		return Source{}, "", false, err.Error()
	}
	manifest, manifestProblems := inspectManifest(path, false)
	resources, resourceProblems := inventoryResources(ctx, path)
	source := Source{ID: sourceID(token), Identity: Identity{Method: "darwin-directory-object", Certainty: "proven"}, Directory: path, Name: manifest.Name, Manifest: manifest, Resources: resources}
	if len(manifestProblems)+len(resourceProblems) != 0 {
		return source, "", false, "canonical bundle inspection is incomplete"
	}
	fingerprint, coverage, _, reasons, _ := consolidationFingerprint(ctx, source)
	if coverage != Complete {
		return source, fingerprint, false, strings.Join(reasons, "; ")
	}
	return source, fingerprint, true, ""
}

func validateDestinationChoices(source ConsolidationSource, choice ConsolidationChoice, request ConsolidationRequest, apps map[string]ConsolidationApplication) ([]ConsolidationDestinationChoice, error) {
	if len(choice.Destinations) != len(request.Applications) {
		return nil, errors.New("destinations must contain exactly one assessment for every selected application")
	}
	assessments := append([]ConsolidationDestinationChoice(nil), choice.Destinations...)
	seen := map[string]bool{}
	for _, app := range assessments {
		if !containsString(request.Applications, app.Application) || seen[app.Application] {
			return nil, fmt.Errorf("destination app %q is not selected or is repeated", app.Application)
		}
		seen[app.Application] = true
		if strings.TrimSpace(app.Reason) == "" {
			return nil, fmt.Errorf("destination %s requires a written portability reason", app.Application)
		}
		switch app.Portability {
		case "compatible", "incompatible":
			if app.UserApproved {
				return nil, fmt.Errorf("destination %s may set user_approved_unknown only when portability is unknown", app.Application)
			}
		case "unknown":
			if !app.UserApproved {
				return nil, fmt.Errorf("destination %s portability is unknown and has not been explicitly approved by the user", app.Application)
			}
		default:
			return nil, fmt.Errorf("destination %s portability must be compatible, incompatible, or unknown", app.Application)
		}
		availability := source.Availability[app.Application]
		if hasNativeReference(source, app.Application) {
			switch availability.State {
			case AvailabilityDisabled:
				if !consolidationCodexDisableProof(source, app.Application, apps[app.Application], availability) {
					return nil, fmt.Errorf("%s currently reports this source disabled without matching the recorded Codex path-selector preservation proof", app.Application)
				}
			case AvailabilityExplicitOnly, AvailabilityAsk, AvailabilityShadowed, AvailabilityUnknown:
				return nil, fmt.Errorf("%s currently reports this source %s; moving it could change native availability and is refused", app.Application, availability.State)
			}
		}
		if apps[app.Application].DestinationRoot == "" {
			return nil, fmt.Errorf("destination root is unsupported for %s", app.Application)
		}
	}
	return assessments, nil
}

func consolidationCodexDisableProof(source ConsolidationSource, app string, destination ConsolidationApplication, availability AvailabilityExplanation) bool {
	if app != "codex" || destination.Scope.Agent == nil || availability.State != AvailabilityDisabled || availability.Reason != "effective manifest selector disables this source" || availability.Evidence == nil {
		return false
	}
	if destination.Scope.Agent.Application != "codex" || destination.Scope.Agent.ValidatedVersion != codexValidatedVersion || availability.Evidence.AgentVersion != codexValidatedVersion || availability.Evidence.Rule != "configured-availability" {
		return false
	}
	const prefix = "skills.config.path="
	if !strings.HasPrefix(availability.Evidence.Setting, prefix) {
		return false
	}
	selector := strings.TrimPrefix(availability.Evidence.Setting, prefix)
	for _, reference := range source.KnownReferences {
		if reference.Application != "codex" || reference.Origin == "" || (reference.Reference.Context != ContextGlobal && reference.Reference.Context != ContextCustom) || reference.Reference.Status != "resolved" {
			continue
		}
		for _, directory := range []string{reference.Reference.DiscoveredPath, reference.Reference.ResolvedPath} {
			if directory != "" && filepath.Clean(selector) == filepath.Clean(filepath.Join(directory, "SKILL.md")) {
				return true
			}
		}
	}
	return false
}

type consolidationDestinationNames struct {
	byNativeName map[string]map[string]string
	byBundle     map[string]map[string]string
}

func (names *consolidationDestinationNames) add(application, nativeName, canonicalLeaf, fingerprint string) error {
	if names.byNativeName == nil {
		names.byNativeName = map[string]map[string]string{}
		names.byBundle = map[string]map[string]string{}
	}
	if names.byNativeName[application] == nil {
		names.byNativeName[application] = map[string]string{}
		names.byBundle[application] = map[string]string{}
	}
	nativeKey := strings.ToLower(nativeName)
	if previous, exists := names.byNativeName[application][nativeKey]; exists && previous != fingerprint {
		return fmt.Errorf("native name %s remains duplicated in %s even though link leaves may differ", nativeName, application)
	}
	if previous, exists := names.byBundle[application][fingerprint]; exists && previous != canonicalLeaf {
		return fmt.Errorf("identical bundles must use one canonical link leaf in %s; %s and %s would create duplicate references", application, previous, canonicalLeaf)
	}
	names.byNativeName[application][nativeKey] = fingerprint
	names.byBundle[application][fingerprint] = canonicalLeaf
	return nil
}

func codexRelocationProof() ConsolidationRelocationProof {
	return ConsolidationRelocationProof{
		ID: "codex-manifest-path-selector-alias-v1", Application: "codex", Profile: codexConsolidationRelocationProfile, BinarySHA256: codexConsolidationRelocationSHA256,
		Observed:      "the configured disabled state remained effective after moving the complete bundle and replacing its original path with a symlink; existing aliases still resolved and native config bytes were unchanged",
		Preconditions: []string{"Codex AvailabilityDisabled is derived from one exact skills.config.path selector naming an existing native bundle reference's SKILL.md", "the original bundle path is retained as a symlink to the canonical library bundle", "post-apply selected-app inventory rechecks the effective disabled state"},
		Limits:        []string{"this recorded disposable relocation profile does not observe the installed Codex binary or a running session", "the adapter parser's earlier codex-cli 0.155.1 validation is separate and is not validated by this 0.162.1 relocation profile", "unknown, ask, shadowed, explicit-only, non-Codex, and other selector forms remain unsupported"},
	}
}

func hasNativeReference(source ConsolidationSource, app string) bool {
	for _, ref := range source.KnownReferences {
		if ref.Application == app && ref.Origin != "" && (ref.Reference.Context == ContextGlobal || ref.Reference.Context == ContextCustom) {
			return true
		}
	}
	return false
}

func consolidationAlreadyConnected(source ConsolidationSource, app string) bool {
	for _, reference := range source.KnownReferences {
		if reference.Application != app || reference.Origin == "" || reference.Reference.Context != ContextGlobal && reference.Reference.Context != ContextCustom {
			continue
		}
		if reference.Reference.Status == "resolved" {
			return true
		}
	}
	return false
}

func consolidationReferencesForApp(source ConsolidationSource, app string) []string {
	var refs []string
	for _, reference := range source.KnownReferences {
		if reference.Application == app && reference.Origin != "" && (reference.Reference.Context == ContextGlobal || reference.Reference.Context == ContextCustom) && reference.Reference.Status == "resolved" {
			refs = append(refs, reference.Reference.DiscoveredPath)
		}
	}
	return uniqueStrings(refs)
}

func validateDestinationConsumerSet(_ context.Context, root, destination, destinationApp string, selected []string, assessments []ConsolidationDestinationChoice, audits []ConsolidationDestinationAudit) error {
	var audit *ConsolidationDestinationAudit
	for i := range audits {
		if audits[i].Application == destinationApp && filepath.Clean(audits[i].DestinationRoot) == filepath.Clean(root) {
			audit = &audits[i]
			break
		}
	}
	if audit == nil {
		return fmt.Errorf("no scan-pinned consumer audit exists for destination %s", root)
	}
	refusals := consolidationRelevantDestinationRefusals(audit.Refusals, destination)
	if len(refusals) > 0 {
		return fmt.Errorf("destination route evidence is refused: %s", strings.Join(refusals, "; "))
	}
	assessmentByApp := map[string]ConsolidationDestinationChoice{}
	for _, assessment := range assessments {
		assessmentByApp[assessment.Application] = assessment
	}
	routeByApp := map[string]ConsolidationConsumerRoute{}
	for _, route := range audit.PotentialConsumers {
		routeByApp[route.Application] = route
	}
	for _, app := range selected {
		assessment, hasAssessment := assessmentByApp[app]
		_, isConsumer := routeByApp[app]
		if isConsumer && (!hasAssessment || assessment.Portability == "incompatible") {
			return fmt.Errorf("new link under %s would expose selected application %s despite its incompatible or missing assessment", root, app)
		}
	}
	destinationAssessment, ok := assessmentByApp[destinationApp]
	if !ok || destinationAssessment.Portability == "incompatible" {
		return fmt.Errorf("destination application %s is not assessed as compatible", destinationApp)
	}
	acks := map[string]ConsolidationConsumerAcknowledgment{}
	for _, ack := range destinationAssessment.AcknowledgedConsumers {
		if ack.Application == "" || acks[ack.Application].Application != "" {
			return fmt.Errorf("destination %s repeats or omits an acknowledged consumer application", destinationApp)
		}
		acks[ack.Application] = ack
	}
	for app, ack := range acks {
		route, exists := routeByApp[app]
		if !exists || containsString(selected, app) {
			return fmt.Errorf("acknowledgement for %s is not a current unselected consumer route", app)
		}
		if !ack.UserApproved || len(ack.EvidenceIDs) != 1 || ack.EvidenceIDs[0] != route.EvidenceID {
			return fmt.Errorf("acknowledgement for %s must approve exactly current scan evidence %s", app, route.EvidenceID)
		}
	}
	for _, route := range audit.PotentialConsumers {
		if containsString(selected, route.Application) {
			continue
		}
		ack, ok := acks[route.Application]
		if !ok || !ack.UserApproved || len(ack.EvidenceIDs) != 1 || ack.EvidenceIDs[0] != route.EvidenceID {
			return fmt.Errorf("new link under %s may indirectly expose unselected %s through evidence %s; explicit acknowledgement is required, installation/runtime are unknown", root, route.Application, route.EvidenceID)
		}
	}
	return nil
}

func consolidationRelevantDestinationRefusals(refusals []string, destination string) []string {
	const managedRootMarker = ": managed/system root: "
	var relevant []string
	for _, refusal := range refusals {
		if index := strings.Index(refusal, managedRootMarker); index >= 0 {
			root := refusal[index+len(managedRootMarker):]
			if setupPathsOverlap(destination, root) == deleteUnrelated {
				continue // a sibling skill link does not modify this nested managed/system root
			}
		}
		relevant = append(relevant, refusal)
	}
	return relevant
}

func consolidationDestinationRouteNotes(destinationRoot string, refusals, warnings []string) ([]string, []string) {
	const managedRootMarker = ": managed/system root: "
	var remaining []string
	for _, refusal := range refusals {
		index := strings.Index(refusal, managedRootMarker)
		if index < 0 {
			remaining = append(remaining, refusal)
			continue
		}
		path := refusal[index+len(managedRootMarker):]
		relative, err := filepath.Rel(filepath.Clean(destinationRoot), filepath.Clean(path))
		first := strings.Split(relative, string(filepath.Separator))[0]
		if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && strings.HasPrefix(first, ".") {
			warnings = append(warnings, "managed/system root remains unchanged and is outside every supported non-dot skill-link leaf: "+path)
			continue
		}
		remaining = append(remaining, refusal)
	}
	return uniqueStrings(remaining), uniqueStrings(warnings)
}

func validateDestinationCollision(destination string, source ConsolidationSource, canonicalName string, inventory ConsolidationInventory, choices map[SourceID]ConsolidationChoice) error {
	info, err := os.Lstat(destination)
	if err == nil {
		exactCanonicalLink := false
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := filepath.EvalSymlinks(destination)
			if readErr == nil && filepath.Clean(target) == filepath.Clean(filepath.Join(inventory.Scope.Library, source.BundleFingerprint)) {
				exactCanonicalLink = true
			}
		}
		if filepath.Clean(destination) != filepath.Clean(source.SourcePath) && !exactCanonicalLink {
			return fmt.Errorf("destination already exists and is not the exact requested canonical link: %s", destination)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("destination cannot be inspected: %s: %w", destination, err)
	}
	sourceByID := map[SourceID]ConsolidationSource{}
	for _, candidate := range inventory.Sources {
		sourceByID[candidate.ID] = candidate
	}
	for _, app := range inventory.Applications {
		if !app.Selected || app.DestinationRoot != filepath.Dir(destination) && !pathWithin(app.DestinationRoot, destination) {
			continue
		}
		for _, existing := range app.ExistingSources {
			if existing.SourceID == source.ID {
				continue
			}
			name := existing.NativeName
			if name == "" {
				name = existing.Name
			}
			if selected, chosen := choices[existing.SourceID]; chosen {
				if selected.Action == "archive" {
					continue
				}
				if selected.Action == "share" && sourceByID[existing.SourceID].BundleFingerprint == source.BundleFingerprint {
					continue
				}
			}
			sourceName := source.Availability[app.Application].NativeName
			if sourceName == "" {
				sourceName = source.NativeName
			}
			if sourceName == "" {
				sourceName = source.Name
			}
			if name != "" && strings.EqualFold(name, sourceName) {
				return fmt.Errorf("existing independent source %s keeps native name %s in %s", existing.SourceID, name, app.Application)
			}
		}
	}
	return nil
}

func validateConflictChoices(sources []ConsolidationSource, choices map[SourceID]ConsolidationChoice) error {
	byNameApp := map[string]map[SourceID]string{}
	for _, source := range sources {
		choice, ok := choices[source.ID]
		if !ok || choice.Action != "share" {
			continue
		}
		for _, destination := range choice.Destinations {
			if destination.Portability == "incompatible" {
				continue
			}
			name := source.Availability[destination.Application].NativeName
			if name == "" {
				name = source.NativeName
			}
			if name == "" {
				name = source.Name
			}
			if name == "" {
				name = choice.CanonicalName
			}
			key := destination.Application + "\x00" + strings.ToLower(name)
			if byNameApp[key] == nil {
				byNameApp[key] = map[SourceID]string{}
			}
			for previous, fingerprint := range byNameApp[key] {
				if previous != source.ID && fingerprint != source.BundleFingerprint {
					return fmt.Errorf("native name %s remains duplicated in %s even though link leaves may differ", name, destination.Application)
				}
			}
			byNameApp[key][source.ID] = source.BundleFingerprint
		}
	}
	return nil
}

func consolidationWarnings(inventory ConsolidationInventory, choices map[SourceID]ConsolidationChoice) []string {
	var out []string
	for _, warning := range inventory.LibraryWarnings {
		out = append(out, "library consumer audit warning: "+warning)
	}
	for _, app := range inventory.Applications {
		if app.Scope.Agent != nil {
			out = append(out, app.Application+" validated baseline: "+app.Scope.Agent.ValidatedVersion+"; running-session adoption remains unknown")
			for _, gap := range app.Gaps {
				out = append(out, app.Application+" evidence gap: "+gap)
			}
		}
	}
	for _, source := range inventory.Sources {
		choice := choices[source.ID]
		for _, limit := range source.InspectionLimits {
			out = append(out, "bundle inspection limitation: "+source.SourcePath+": "+limit)
		}
		for _, proof := range source.RelocationProofs {
			out = append(out, fmt.Sprintf("native relocation proof %s (%s, binary SHA-256 %s) applies only to its listed preconditions; installed version and running-session adoption are not observed", proof.ID, proof.Profile, proof.BinarySHA256))
		}
		if choice.Action == "share" {
			for _, destination := range choice.Destinations {
				if destination.Portability == "incompatible" {
					continue
				}
				for _, audit := range inventory.DestinationAudits {
					if audit.Application == destination.Application {
						for _, warning := range audit.Warnings {
							out = append(out, destination.Application+" destination route warning: "+warning)
						}
						for _, route := range audit.PotentialConsumers {
							if containsString(inventory.Scope.Applications, route.Application) {
								continue
							}
							out = append(out, fmt.Sprintf("new link at %s may indirectly expose unselected %s through %s; acknowledged exposure does not import it or create a direct link", audit.DestinationRoot, route.Application, route.EvidenceID))
						}
					}
				}
			}
		}
		for _, reference := range source.KnownReferences {
			if containsString(inventory.Scope.Applications, reference.Application) {
				continue
			}
			switch choice.Action {
			case "share", "keep":
				out = append(out, "existing external reference remains at "+reference.Reference.DiscoveredPath+" for "+reference.Application)
			case "archive":
				out = append(out, "archive will move the target of existing external reference "+reference.Reference.DiscoveredPath+" for "+reference.Application)
			}
		}
	}
	return uniqueStrings(out)
}

func consolidationPlanID(draft ConsolidationDraft) string {
	value := draft
	value.Choices = cloneConsolidationChoices(draft.Choices)
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return "c-" + hex.EncodeToString(sum[:12])
}

func consolidationConfirmation(draft ConsolidationDraft, inventory ConsolidationInventory, effects []ConsolidationEffect) (string, error) {
	encoded, err := json.Marshal(struct {
		Version int
		Draft   ConsolidationDraft
		ScanID  string
		Effects []ConsolidationEffect
	}{ConsolidationSchemaVersion, draft, inventory.ScanID, effects})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "c1:" + hex.EncodeToString(sum[:]), nil
}

func consolidationBackupRoot() string {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(data, "skillglass", "consolidation", "backups")
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func consolidationEqualStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func RememberedSelectionID(apps, roots []string, library string) string {
	return rememberedSelectionID(apps, roots, library)
}

func rememberedSelectionID(apps, roots []string, library string) string {
	value := struct {
		Applications []string
		Roots        []string
		Library      string
	}{append([]string(nil), apps...), append([]string(nil), roots...), filepath.Clean(library)}
	sort.Strings(value.Applications)
	sort.Strings(value.Roots)
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "selection1:" + hex.EncodeToString(digest[:])
}
