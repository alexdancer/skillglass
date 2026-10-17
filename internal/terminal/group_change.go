package terminal

import (
	"fmt"
	"github.com/alexdancer/skillglass/internal/collection"
	"strings"
)

func groupContextLines(view collection.GroupView) []string {
	lines := []string{"Group " + string(view.Intent.Scope) + " " + string(view.Intent.Choice) + " | " + view.State, "Order: " + strings.Join(view.Intent.Applications, " -> "), "Source ID: " + string(view.Intent.Source), inspectionProjectLine(view.InspectionProject)}
	if view.Source != nil {
		lines = append(lines, "Exact source: "+view.Source.Directory, "Identity: "+view.Source.Identity.Method+" | "+view.Source.Identity.Certainty)
	}
	lines = append(lines, "Bounded native discovery (not machine-wide):")
	for _, c := range view.Consumers {
		lines = append(lines, fmt.Sprintf("%s: coverage %s | %d exact-source references | controls %t", controlApplication(c.Application), c.Scope.Completeness, len(c.References), c.Controls))
		if c.Reason != "" {
			lines = append(lines, c.Reason)
		}
		for _, root := range c.Scope.Roots {
			lines = append(lines, "Root ("+string(root.State)+"): "+root.Requested)
		}
		for _, ref := range c.References {
			lines = append(lines, "Reference ("+string(ref.Context)+"): "+ref.DiscoveredPath)
		}
	}
	return lines
}
func groupPreviewLines(view collection.GroupView) []string {
	lines := []string{"Group " + string(view.Intent.Scope) + " " + string(view.Intent.Choice) + " | " + view.State, "Order: " + strings.Join(view.Intent.Applications, " -> "), "Source ID: " + string(view.Intent.Source), "Reason: " + view.Reason}
	for i, m := range view.Members {
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s", i+1, controlApplication(m.Application), m.Preview.State, m.Preview.Reason))
		child := m.Preview
		// Child confirmation is private. Only the aggregate token authorizes execution.
		child.Confirmation = ""
		detail := changePreviewLines(child)
		for _, line := range detail {
			if !strings.HasPrefix(line, "Confirm with --confirm") {
				lines = append(lines, line)
			}
		}
	}
	lines = append(lines, groupContextLines(view)...)
	lines = append(lines, view.Notes...)
	if view.State == "ready" {
		lines = append(lines, "Preview only. No settings changed.", "Confirm group with --confirm "+view.Confirmation)
	}
	return safeChangeLines(lines)
}
func groupOutcomeLines(out collection.GroupOutcome) []string {
	lines := []string{"Group result: " + out.State, "Order: " + strings.Join(out.View.Intent.Applications, " -> "), "Reason: " + out.Reason}
	// Compact ordered results stay at the top; complete native evidence follows.
	for i, r := range out.Results {
		lines = append(lines, fmt.Sprintf("%d. %s: %s", i+1, controlApplication(r.Application), r.State))
	}
	for _, r := range out.Results {
		lines = append(lines, controlApplication(r.Application)+": "+r.Reason)
		if r.Outcome != nil {
			lines = append(lines, changeOutcomeLines(*r.Outcome)...)
		} else {
			lines = append(lines, "Reload: unknown (application not attempted)")
		}
	}
	lines = append(lines, groupContextLines(out.View)...)
	return safeChangeLines(lines)
}
