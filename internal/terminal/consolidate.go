package terminal

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/alexdancer/skillglass/internal/collection"
)

func runConsolidate(args []string, stdout, stderr io.Writer) int {
	showHelp := len(args) == 0
	for _, arg := range args {
		if arg == "help" || arg == "-h" || arg == "--help" {
			showHelp = true
		}
	}
	if showHelp {
		if reviewJSONRequested(args) {
			return writeReviewJSON(map[string]any{"schema_version": 1, "operation": "consolidate-help", "help": usage}, stdout, stderr)
		}
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	switch args[0] {
	case "scan":
		return runConsolidateScan(args[1:], stdout, stderr)
	case "preview":
		return runConsolidatePreview(args[1:], stdout, stderr)
	case "apply":
		return runConsolidateApply(args[1:], stdout, stderr)
	case "recover":
		return runConsolidateRecover(args[1:], stdout, stderr)
	default:
		return consolidateCLIError("consolidate", fmt.Errorf("unknown consolidation operation %q", args[0]), 2, reviewJSONRequested(args[1:]), stdout, stderr)
	}
}

func runConsolidateScan(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("consolidate scan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var apps, roots stringList
	var library optionalString
	clearRoots := flags.Bool("clear-roots", false, "discard remembered custom roots")
	machine := flags.Bool("json", false, "emit one versioned JSON object")
	flags.Var(&apps, "app", "explicit application selection; repeatable")
	flags.Var(&roots, "root", "personal source root; repeatable")
	flags.Var(&library, "library", "canonical shared bundle library")
	if err := flags.Parse(args); err != nil {
		return consolidateCLIError("consolidate-scan", err, 2, reviewJSONRequested(args), stdout, stderr)
	}
	if flags.NArg() != 0 {
		return consolidateCLIError("consolidate-scan", errors.New("scan accepts flags only"), 2, *machine, stdout, stderr)
	}
	selectionFrom := "arguments"
	var rememberedID string
	selectedApps := append([]string(nil), apps...)
	selectedRoots := append([]string(nil), roots...)
	selectedLibrary := library.value
	if len(apps) == 0 {
		lastApps, lastRoots, lastLibrary, found, id, err := collection.LastConsolidationSelectionWithID()
		if err != nil {
			return consolidateCLIError("consolidate-scan", fmt.Errorf("read last successful selection: %w", err), 2, *machine, stdout, stderr)
		}
		if !found {
			return consolidateCLIError("consolidate-scan", errors.New("no last successful application selection; pass --app explicitly"), 2, *machine, stdout, stderr)
		}
		selectionFrom = "remembered"
		rememberedID = id
		selectedApps = lastApps
		if !*clearRoots {
			selectedRoots = append(append([]string(nil), lastRoots...), roots...)
		}
		if !library.set {
			selectedLibrary = lastLibrary
		}
	} else {
		var err error
		rememberedID, err = collection.CurrentConsolidationSelectionID()
		if err != nil {
			return consolidateCLIError("consolidate-scan", fmt.Errorf("read remembered-selection freshness evidence: %w", err), 2, *machine, stdout, stderr)
		}
	}
	if *clearRoots && len(apps) != 0 {
		selectedRoots = append([]string(nil), roots...)
	}
	selectedRoots = uniqueCLIStrings(selectedRoots)
	request, err := collection.NormalizeConsolidationRequest(selectedApps, selectedRoots, selectedLibrary, selectionFrom)
	if err != nil {
		return consolidateCLIError("consolidate-scan", err, 2, *machine, stdout, stderr)
	}
	request.RememberedSelectionID = rememberedID
	result := collection.ScanConsolidation(context.Background(), request)
	if *machine {
		if code := writeReviewJSON(result, stdout, stderr); code != 0 {
			return code
		}
		return result.ExitCode
	}
	renderConsolidationInventory(stdout, result)
	return result.ExitCode
}

func runConsolidatePreview(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("consolidate preview", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	draftPath := flags.String("draft", "", "agent-authored consolidation draft JSON")
	machine := flags.Bool("json", false, "emit one versioned JSON object")
	if err := flags.Parse(args); err != nil {
		return consolidateCLIError("consolidate-preview", err, 2, reviewJSONRequested(args), stdout, stderr)
	}
	if flags.NArg() != 0 || *draftPath == "" {
		return consolidateCLIError("consolidate-preview", errors.New("preview requires --draft FILE and accepts no positional arguments"), 2, *machine, stdout, stderr)
	}
	var draft collection.ConsolidationDraft
	if err := readReviewFile(*draftPath, &draft); err != nil {
		return consolidateCLIError("consolidate-preview", err, 2, *machine, stdout, stderr)
	}
	preview := collection.PreviewConsolidation(context.Background(), draft)
	if *machine {
		if code := writeReviewJSON(preview, stdout, stderr); code != 0 {
			return code
		}
		return preview.ExitCode
	}
	renderConsolidationPreview(stdout, preview)
	return preview.ExitCode
}

func runConsolidateApply(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("consolidate apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	draftPath := flags.String("draft", "", "exact consolidation draft JSON")
	confirmation := flags.String("confirm", "", "exact confirmation token from fresh preview")
	machine := flags.Bool("json", false, "emit one versioned JSON object")
	if err := flags.Parse(args); err != nil {
		return consolidateCLIError("consolidate-apply", err, 2, reviewJSONRequested(args), stdout, stderr)
	}
	if flags.NArg() != 0 || *draftPath == "" || *confirmation == "" {
		return consolidateCLIError("consolidate-apply", errors.New("apply requires --draft FILE and --confirm TOKEN"), 2, *machine, stdout, stderr)
	}
	var draft collection.ConsolidationDraft
	if err := readReviewFile(*draftPath, &draft); err != nil {
		return consolidateCLIError("consolidate-apply", err, 2, *machine, stdout, stderr)
	}
	outcome := collection.ApplyConsolidation(context.Background(), draft, *confirmation)
	if *machine {
		if code := writeReviewJSON(outcome, stdout, stderr); code != 0 {
			return code
		}
		return outcome.ExitCode
	}
	renderConsolidationOutcome(stdout, outcome)
	return outcome.ExitCode
}

func runConsolidateRecover(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("consolidate recover", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	backupID := flags.String("backup", "", "exact recovery ID printed by apply")
	confirmation := flags.String("confirm", "", "exact recovery confirmation token from preview")
	machine := flags.Bool("json", false, "emit one versioned JSON object")
	if err := flags.Parse(args); err != nil {
		return consolidateCLIError("consolidate-recover", err, 2, reviewJSONRequested(args), stdout, stderr)
	}
	if flags.NArg() != 0 || *backupID == "" {
		return consolidateCLIError("consolidate-recover", errors.New("recover requires --backup ID and accepts no positional arguments"), 2, *machine, stdout, stderr)
	}
	if *confirmation != "" {
		outcome := collection.ApplyConsolidationRecovery(context.Background(), *backupID, *confirmation)
		if *machine {
			if code := writeReviewJSON(outcome, stdout, stderr); code != 0 {
				return code
			}
			return outcome.ExitCode
		}
		renderConsolidationOutcome(stdout, outcome)
		return outcome.ExitCode
	}
	preview := collection.PreviewConsolidationRecovery(context.Background(), *backupID)
	if *machine {
		if code := writeReviewJSON(preview, stdout, stderr); code != 0 {
			return code
		}
		return preview.ExitCode
	}
	renderConsolidationPreview(stdout, preview)
	return preview.ExitCode
}

func consolidateCLIError(operation string, err error, exitCode int, machine bool, stdout, stderr io.Writer) int {
	if machine {
		failure := map[string]any{"schema_version": 1, "operation": operation, "outcome": "failed", "exit_code": exitCode, "state": "refused", "reason": err.Error(), "failure": map[string]string{"code": "invalid-request", "message": err.Error(), "action": "Correct the command or input and retry."}}
		if code := writeReviewJSON(failure, stdout, stderr); code != 0 {
			return code
		}
		return exitCode
	}
	writeError(stderr, err)
	_, _ = io.WriteString(stderr, usage)
	return exitCode
}

func renderConsolidationInventory(out io.Writer, inventory collection.ConsolidationInventory) {
	fmt.Fprintf(out, "Consolidation scan %s | %s | coverage: %s | selected coverage: %s\n", inventory.ScanID, inventory.Outcome, inventory.Coverage, inventory.SelectedCoverage)
	fmt.Fprintf(out, "Selected applications: %s\nLibrary: %s | safe: %t\n", strings.Join(inventory.Scope.Applications, ", "), inventory.Scope.Library, inventory.LibrarySafe)
	for _, app := range inventory.Applications {
		if !app.Selected {
			continue
		}
		if app.NativeRelocationProfile == nil {
			fmt.Fprintf(out, "Native relocation %s | %s | %s\n", app.Application, app.NativeRelocationStatus, safeInline(app.NativeRelocationReason))
			continue
		}
		fmt.Fprintf(out, "Native relocation %s | %s | %s | %s SHA-256 %s\n", app.Application, app.NativeRelocationStatus, safeInline(app.NativeRelocationProfile.Profile), safeInline(displayOptional(app.NativeRelocationProfile.ArtifactKind)), app.NativeRelocationProfile.BinarySHA256)
		for _, precondition := range app.NativeRelocationProfile.Preconditions {
			fmt.Fprintf(out, "  Profile precondition: %s\n", safeInline(precondition))
		}
		if app.NativeRelocationProfile.SourceRevision != "" {
			fmt.Fprintf(out, "  Source revision: %s | loader source SHA-256: %s\n", app.NativeRelocationProfile.SourceRevision, app.NativeRelocationProfile.LoaderSHA256)
		}
		fmt.Fprintf(out, "  Observed: %s\n", safeInline(app.NativeRelocationProfile.Observed))
		for _, limit := range app.NativeRelocationProfile.Limits {
			fmt.Fprintf(out, "  Profile limit: %s\n", safeInline(limit))
		}
	}
	for _, reason := range inventory.LibraryReasons {
		fmt.Fprintf(out, "Library restriction: %s\n", safeInline(reason))
	}
	for _, warning := range inventory.LibraryWarnings {
		fmt.Fprintf(out, "Library warning: %s\n", safeInline(warning))
	}
	for _, source := range inventory.Sources {
		fmt.Fprintf(out, "Source %s | %s | native name: %s | fingerprint: %s | inspection complete: %t\n", source.ID, safeInline(source.SourcePath), displayOptional(source.NativeName), displayOptional(source.BundleFingerprint), source.InspectionComplete)
		for _, limit := range source.InspectionLimits {
			fmt.Fprintf(out, "  Inspection limitation: %s\n", safeInline(limit))
		}
		for _, reference := range source.KnownReferences {
			fmt.Fprintf(out, "  %s %s %s | %s\n", reference.Application, reference.Reference.Context, reference.Reference.Status, safeInline(reference.Reference.DiscoveredPath))
		}
		for _, proof := range source.RelocationProofs {
			fmt.Fprintf(out, "  Native relocation profile %s | %s | binary SHA-256 %s\n", proof.ID, proof.Profile, proof.BinarySHA256)
			fmt.Fprintf(out, "    Observed: %s\n", safeInline(proof.Observed))
			for _, limit := range proof.Limits {
				fmt.Fprintf(out, "    Limit: %s\n", safeInline(limit))
			}
		}
		for _, reason := range source.BlockedReasons {
			fmt.Fprintf(out, "  Blocked: %s\n", safeInline(reason))
		}
	}
	for _, conflict := range inventory.Conflicts {
		fmt.Fprintf(out, "Conflict %s | %s | %s | sources %s\n", conflict.ID, conflict.Kind, safeInline(conflict.Reason), strings.Join(sourceIDsForTerminal(conflict.SourceIDs), ", "))
	}
	for _, destination := range inventory.DestinationAudits {
		fmt.Fprintf(out, "Destination %s | %s\n", destination.Application, safeInline(destination.DestinationRoot))
		for _, consumer := range destination.PotentialConsumers {
			fmt.Fprintf(out, "  Possible consumer %s | evidence %s | installation/runtime: unknown | %s\n", consumer.Application, consumer.EvidenceID, safeInline(consumer.Reason))
		}
		for _, refusal := range destination.Refusals {
			fmt.Fprintf(out, "  Route refusal: %s\n", safeInline(refusal))
		}
		for _, warning := range destination.Warnings {
			fmt.Fprintf(out, "  Route warning: %s\n", safeInline(warning))
		}
	}
	for _, excluded := range inventory.Exclusions {
		fmt.Fprintf(out, "Excluded %s | %s\n", safeInline(excluded.Path), safeInline(excluded.Reason))
	}
	if inventory.Failure != nil {
		fmt.Fprintf(out, "Failure: %s | %s\n", inventory.Failure.Code, safeInline(inventory.Failure.Message))
	}
}

func renderConsolidationPreview(out io.Writer, preview collection.ConsolidationPreview) {
	fmt.Fprintf(out, "Consolidation %s | %s | %s\n", preview.Operation, preview.State, safeInline(preview.Reason))
	if preview.ScanID != "" {
		fmt.Fprintf(out, "Scan: %s\n", preview.ScanID)
	}
	if len(preview.Scope.Applications) != 0 || preview.Scope.Library != "" {
		fmt.Fprintf(out, "Selected applications: %s\n", safeInline(displayOptional(strings.Join(preview.Scope.Applications, ", "))))
		fmt.Fprintf(out, "Roots: %s\n", safeInline(displayOptional(strings.Join(preview.Scope.Roots, ", "))))
		fmt.Fprintf(out, "Canonical library: %s\n", safeInline(preview.Scope.Library))
		fmt.Fprintf(out, "Selection: %s | remembered-selection evidence: %s\n", safeInline(displayOptional(preview.Scope.SelectionFrom)), safeInline(displayOptional(preview.Scope.RememberedSelectionID)))
	}
	for _, conflict := range preview.Conflicts {
		fmt.Fprintf(out, "Conflict %s | %s | %s | %s | sources %s\n", safeInline(conflict.ID), safeInline(conflict.Kind), safeInline(conflict.State), safeInline(conflict.Reason), strings.Join(sourceIDsForTerminal(conflict.SourceIDs), ", "))
	}
	for _, exclusion := range preview.Exclusions {
		fmt.Fprintf(out, "Excluded %s | %s\n", safeInline(exclusion.Path), safeInline(exclusion.Reason))
	}
	for _, source := range preview.SourceAvailability {
		fmt.Fprintf(out, "Source availability %s | %s | eligible: %t | resolution: %s | action: %s\n", safeInline(string(source.SourceID)), safeInline(source.SourcePath), source.Eligible, safeInline(source.Resolution), safeInline(displayOptional(source.Action)))
		for _, availability := range source.Availability {
			fmt.Fprintf(out, "  %s | %s | native name: %s\n", safeInline(availability.Application), safeInline(availability.State), safeInline(displayOptional(availability.NativeName)))
		}
		for _, reason := range source.BlockedReasons {
			fmt.Fprintf(out, "  Blocked: %s\n", safeInline(reason))
		}
	}
	for _, choice := range preview.Choices {
		fmt.Fprintf(out, "Choice %s | %s | %s\n", safeInline(string(choice.SourceID)), safeInline(choice.Action), safeInline(choice.SourcePath))
		fmt.Fprintf(out, "  Complete bundle fingerprint: %s\n", safeInline(displayOptional(choice.BundleFingerprint)))
		if choice.CanonicalName != "" {
			fmt.Fprintf(out, "  Canonical leaf: %s\n", safeInline(choice.CanonicalName))
		}
		for _, destination := range choice.Destinations {
			fmt.Fprintf(out, "  Destination %s | portability: %s | %s\n", safeInline(destination.Application), safeInline(destination.Portability), safeInline(destination.Reason))
			if destination.UserApproved {
				fmt.Fprintln(out, "    Unknown portability explicitly approved")
			}
			for _, consumer := range destination.AcknowledgedConsumers {
				fmt.Fprintf(out, "    Possible indirect consumer %s | explicitly acknowledged: %t | evidence IDs: %s\n", safeInline(consumer.Application), consumer.UserApproved, safeInline(displayOptional(strings.Join(consumer.EvidenceIDs, ", "))))
				fmt.Fprintln(out, "      This is exposure acknowledgement only; it does not import or directly link the consumer's skills.")
			}
		}
	}
	if preview.RecoveryID != "" {
		fmt.Fprintf(out, "Recovery ID: %s\n", safeInline(preview.RecoveryID))
	}
	fmt.Fprintln(out, "Execution uses a recovery journal; filesystem operations across directories are not atomic. A partial result may need explicit recovery.")
	for _, effect := range preview.Effects {
		fmt.Fprintf(out, "%s %s | %s\n", effect.Action, effect.SourceID, safeInline(effect.SourcePath))
		for _, profile := range effect.RelocationProfiles {
			fmt.Fprintf(out, "  Native relocation profile %s | %s | %s | %s SHA-256 %s\n", safeInline(profile.ID), safeInline(profile.Application), safeInline(profile.Profile), safeInline(displayOptional(profile.ArtifactKind)), safeInline(profile.BinarySHA256))
			if profile.SourceRevision != "" {
				fmt.Fprintf(out, "    Source revision: %s | loader source SHA-256: %s\n", safeInline(profile.SourceRevision), safeInline(profile.LoaderSHA256))
			}
			fmt.Fprintf(out, "    Observed: %s\n", safeInline(profile.Observed))
			for _, precondition := range profile.Preconditions {
				fmt.Fprintf(out, "    Precondition: %s\n", safeInline(precondition))
			}
			for _, limit := range profile.Limits {
				fmt.Fprintf(out, "    Profile limit: %s\n", safeInline(limit))
			}
		}
		if effect.CanonicalPath != "" {
			fmt.Fprintf(out, "  Canonical: %s\n", safeInline(effect.CanonicalPath))
		}
		if effect.BackupPath != "" {
			fmt.Fprintf(out, "  Recovery: %s\n", safeInline(effect.BackupPath))
		}
		for _, destination := range effect.Destinations {
			fmt.Fprintf(out, "  Link: %s\n", safeInline(destination))
		}
		for _, reference := range effect.PreservedReferences {
			fmt.Fprintf(out, "  Existing reference preserved: %s\n", safeInline(reference))
		}
		for _, operation := range effect.Operations {
			if operation.From != "" {
				fmt.Fprintf(out, "  Operation %s | %s -> %s | %s | %s\n", safeInline(operation.Action), safeInline(operation.From), safeInline(operation.Path), safeInline(operation.State), safeInline(operation.Reason))
			} else {
				fmt.Fprintf(out, "  Operation %s | %s | %s | %s\n", safeInline(operation.Action), safeInline(operation.Path), safeInline(operation.State), safeInline(operation.Reason))
			}
		}
	}
	for _, warning := range preview.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", safeInline(warning))
	}
	if preview.Confirmation != "" {
		fmt.Fprintf(out, "Exact confirmation: %s\n", preview.Confirmation)
	}
	if preview.Failure != nil {
		fmt.Fprintf(out, "Failure: %s | %s | %s\n", preview.Failure.Code, safeInline(preview.Failure.Message), safeInline(preview.Failure.Action))
	}
}

func renderConsolidationOutcome(out io.Writer, outcome collection.ConsolidationApplyOutcome) {
	fmt.Fprintf(out, "Consolidation %s | %s | verified: %t\n%s\n", outcome.Operation, outcome.State, outcome.Verified, safeInline(outcome.Reason))
	if outcome.RecoveryID != "" {
		fmt.Fprintf(out, "Recovery ID: %s\n", outcome.RecoveryID)
	}
	for _, effect := range outcome.Effects {
		fmt.Fprintf(out, "%s %s | %s\n", effect.Action, effect.SourceID, safeInline(effect.State))
		for _, operation := range effect.Operations {
			if operation.From != "" {
				fmt.Fprintf(out, "  Operation %s | %s -> %s | %s | %s\n", safeInline(operation.Action), safeInline(operation.From), safeInline(operation.Path), safeInline(operation.State), safeInline(operation.Reason))
			} else {
				fmt.Fprintf(out, "  Operation %s | %s | %s | %s\n", safeInline(operation.Action), safeInline(operation.Path), safeInline(operation.State), safeInline(operation.Reason))
			}
		}
	}
	if outcome.Failure != nil {
		fmt.Fprintf(out, "Failure: %s | %s | %s\n", outcome.Failure.Code, safeInline(outcome.Failure.Message), safeInline(outcome.Failure.Action))
	}
}

func sourceIDsForTerminal(values []collection.SourceID) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}

func uniqueCLIStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
