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

func runChange(operation string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, false, true)
	scope := flags.String("scope", "", "explicit control scope: global or project")
	var confirmation optionalString
	flags.Var(&confirmation, "confirm", "exact confirmation token from a current preview")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(stdout, usage)
			return 0
		}
		writeError(stderr, err)
		return 2
	}
	if flags.NArg() != 1 || options.agent != "codex" || *scope == "" {
		writeError(stderr, errors.New("enable/disable/inherit requires explicit --agent codex, --scope global or project and exactly one SOURCE_ID after the flags"))
		return 2
	}
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	preview := snapshot.PreviewChange(context.Background(), collection.ChangeIntent{Source: collection.SourceID(flags.Arg(0)), Application: options.agent, Scope: collection.ControlScope(*scope), Choice: collection.ControlChoice(operation)})
	view := preview.View()
	var data []byte
	var err error
	exit := 0
	if confirmation.set && view.State == "ready" {
		out := preview.Apply(context.Background(), confirmation.value)
		exit = out.ExitCode()
		if options.jsonOutput {
			data, err = encodeJSON(out)
		} else {
			data = []byte(strings.Join(changeOutcomeLines(out), "\n") + "\n")
		}
	} else {
		if view.State != "ready" {
			exit = 1
		}
		if options.jsonOutput {
			data, err = encodeJSON(view)
		} else {
			data = []byte(strings.Join(changePreviewLines(view), "\n") + "\n")
		}
	}
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if _, err = stdout.Write(data); err != nil {
		writeError(stderr, fmt.Errorf("write mutation result: %w", err))
		return 1
	}
	return exit
}
func changePreviewLines(view collection.ChangeView) []string {
	lines := []string{fmt.Sprintf("Codex %s %s | %s", view.Intent.Scope, view.Intent.Choice, view.State), "Source ID: " + string(view.Intent.Source), "Reason: " + view.Reason}
	if view.State != "ready" {
		if view.Intent.Scope == collection.ControlProject {
			lines = append(lines, inspectionProjectLine(view.InspectionProject))
		}
		return safeChangeLines(lines)
	}
	for _, change := range view.Changes {
		lines = append(lines, "Change: "+change.Before+" -> "+change.After)
	}
	lines = append(lines, "Action scope: global user configuration")
	lines = append(lines, availabilitySummaryLines(view.Before)...)
	lines = append(lines, "Source: "+view.SourcePath, "Target: "+view.ManifestPath, "Write: "+view.ConfigPath)
	if view.CreateDirectory != "" {
		lines = append(lines, "Create directory: "+view.CreateDirectory+" (0700)")
	}
	lines = append(lines, inspectionProjectLine(view.InspectionProject))
	for _, change := range view.Changes {
		lines = append(lines, "Setting: "+change.Setting)
	}
	lines = append(lines, fmt.Sprintf("Known references: %d", len(view.KnownReferences)))
	for _, reference := range view.KnownReferences {
		lines = append(lines, "Reference ("+string(reference.Context)+"): "+reference.DiscoveredPath)
	}
	lines = append(lines, view.Notes...)
	if view.NoWriteNeeded {
		lines = append(lines, "Already saved; confirmation will verify without rewriting.")
	}
	lines = append(lines, "Preview only. No settings changed.", "Confirm with --confirm "+view.Confirmation)
	return safeChangeLines(lines)
}
func inspectionProjectLine(project collection.ProjectContext) string {
	if !project.Selected {
		return "Inspection project: none selected"
	}
	return "Inspection project: " + project.Path
}

func changeOutcomeLines(out collection.ChangeOutcome) []string {
	lines := []string{fmt.Sprintf("Codex %s %s | %s", out.Intent.Scope, out.Intent.Choice, out.State), "Source ID: " + string(out.Intent.Source), "Write: " + out.WriteState, "Persistence: " + out.Persistence, "Effective result: " + out.Achievement, "Reason: " + out.Reason}
	if out.PersistedPermission != "" {
		lines = append(lines, "Persisted permission: "+string(out.PersistedPermission))
	}
	if out.Effective != nil {
		lines = append(lines, availabilitySummaryLines(*out.Effective)...)
		lines = append(lines, availabilityLines(*out.Effective)...)
	}
	lines = append(lines, "Session adoption: unknown", "Reload: "+out.Reload.State+" | "+out.Reload.Evidence.Reason)
	for _, path := range out.Artifacts {
		lines = append(lines, "Retained artifact: "+path)
	}
	return safeChangeLines(lines)
}

func safeChangeLines(lines []string) []string {
	for index := range lines {
		lines[index] = safeInline(lines[index])
	}
	return lines
}
