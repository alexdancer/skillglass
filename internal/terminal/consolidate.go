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
		lastApps, lastRoots, lastLibrary, found, err := collection.LastConsolidationSelection()
		if err != nil {
			return consolidateCLIError("consolidate-scan", fmt.Errorf("read last successful selection: %w", err), 2, *machine, stdout, stderr)
		}
		if !found {
			return consolidateCLIError("consolidate-scan", errors.New("no last successful application selection; pass --app explicitly"), 2, *machine, stdout, stderr)
		}
		selectionFrom = "remembered"
		rememberedID = collection.RememberedSelectionID(lastApps, lastRoots, lastLibrary)
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
	for _, effect := range preview.Effects {
		fmt.Fprintf(out, "%s %s | %s\n", effect.Action, effect.SourceID, safeInline(effect.SourcePath))
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
