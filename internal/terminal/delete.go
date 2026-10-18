package terminal

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alexdancer/skillglass/internal/collection"
)

func runDelete(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("delete", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, false, true)
	var confirmation optionalString
	flags.Var(&confirmation, "confirm", "target-specific token from the unchanged deletion preview")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(stdout, usage)
			return 0
		}
		writeError(stderr, err)
		return 2
	}
	if flags.NArg() != 1 {
		writeError(stderr, errors.New("delete requires exactly one SOURCE_ID after the flags"))
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	preview := snapshot.PreviewDelete(ctx, collection.SourceID(flags.Arg(0)))
	var value any = preview.View()
	lines := deletePreviewLines(preview.View())
	exit := 1
	if confirmation.set {
		out := preview.Apply(ctx, confirmation.value)
		value = out
		lines = deleteOutcomeLines(out)
		exit = out.ExitCode()
	}
	var data []byte
	var err error
	if options.jsonOutput {
		data, err = encodeJSON(value)
	} else {
		data = []byte(strings.Join(lines, "\n") + "\n")
	}
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if _, err = stdout.Write(data); err != nil {
		writeError(stderr, err)
		return 1
	}
	return exit
}
func deletePreviewLines(v collection.DeleteView) []string {
	lines := []string{"PERMANENT DELETE | " + v.State, "Reason: " + v.Reason, "Exact target: " + v.Target.Path, "Source ID: " + string(v.Target.Source), "Kind: " + v.Target.Kind, "Ownership: " + v.Ownership.State + " | " + v.Ownership.Reason, inspectionProjectLine(v.Project)}
	for _, limit := range v.Limits {
		lines = append(lines, limit)
	}
	lines = append(lines, "Known selected-scope references (left untouched):")
	for _, ref := range v.References {
		lines = append(lines, "Reference: "+ref.DiscoveredPath+" -> "+ref.ResolvedPath)
	}
	lines = append(lines, "Bounded native consumers and provenance:")
	for _, c := range v.Consumers {
		lines = append(lines, fmt.Sprintf("%s: coverage %s | %d exact-source references | %s", controlApplication(c.Application), c.Scope.Completeness, len(c.References), c.Reason))
		for _, root := range c.Scope.Roots {
			lines = append(lines, "Root ("+string(root.State)+", "+root.Origin+"): "+root.Requested)
		}
		if c.Scope.Agent != nil {
			for _, gap := range c.Scope.Agent.Gaps {
				lines = append(lines, "Unverified scope: "+gap)
			}
		}
		for _, ref := range c.References {
			lines = append(lines, "Reference ("+string(ref.Context)+"): "+ref.DiscoveredPath+" -> "+ref.ResolvedPath)
		}
		if c.Availability != nil {
			for _, ref := range c.Availability.References {
				lines = append(lines, "Provenance: "+ref.Provenance.Kind+" | "+ref.Provenance.Evidence.Reason+" | "+ref.Provenance.Evidence.InputPath)
			}
		}
	}
	lines = append(lines, "Exact no-follow content inventory:")
	for _, entry := range v.Contents {
		line := fmt.Sprintf("%s | %s | %d bytes | %d links", entry.Path, entry.Kind, entry.Size, entry.Links)
		if entry.LinkTarget != "" {
			line += " -> " + entry.LinkTarget
		}
		if entry.SHA256 != "" {
			line += " | SHA256 " + entry.SHA256
		}
		lines = append(lines, line)
	}
	if v.State == "ready" {
		lines = append(lines, "Preview only. No content or settings changed. Confirmation is required.", "Confirm with --confirm "+v.Confirmation)
	}
	return safeChangeLines(lines)
}
func deleteOutcomeLines(out collection.DeleteOutcome) []string {
	lines := []string{"Permanent delete result: " + out.State, "Reason: " + out.Reason, "Exact target: " + out.View.Target.Path, fmt.Sprintf("Verified removal: %t", out.Verified)}
	for _, path := range out.Removed {
		lines = append(lines, "Removed: "+path)
	}
	for _, path := range out.Remaining {
		lines = append(lines, "Remaining: "+path)
	}
	for _, path := range out.Unverified {
		lines = append(lines, "Unverified: "+path)
	}
	for _, path := range out.Replacements {
		lines = append(lines, "Replacement observed: "+path)
	}
	lines = append(lines, "No undo or built-in recovery. Remaining references and settings were not repaired.")
	return safeChangeLines(lines)
}
