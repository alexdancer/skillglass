package terminal

import (
	"fmt"
	"strings"

	"github.com/alexdancer/skillglass/internal/collection"
)

const browserDeletePhrase = "confirm delete"

// Browser previews explain the decision; CLI diagnostics remain available in Details.
// Authorization tokens belong to the held preview and are never user input here.
func browserDiagnostics(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "Confirm with --confirm") && !strings.HasPrefix(line, "Confirm group with --confirm") {
			result = append(result, line)
		}
	}
	return result
}

func browserDeleteSummary(v collection.DeleteView) []string {
	lines := []string{"Permanent delete | " + v.State, "Permanently removes this content. No trash or undo.", inspectionProjectLine(v.Project), "Exact target: " + v.Target.Path, ""}
	if v.State != "ready" {
		lines = append(lines, "Cannot delete: "+v.Reason)
	}
	lines = append(lines, fmt.Sprintf("Known references: %d in the selected scope.", len(v.References)))
	consumers := []string{}
	gaps := []string{}
	dependencies := map[string]bool{}
	for _, d := range v.Dependencies {
		dependencies[string(d.Source)] = true
	}
	for _, c := range v.Consumers {
		if len(c.References) > 0 {
			consumers = append(consumers, fmt.Sprintf("%s (%d)", controlApplication(c.Application), len(c.References)))
		}
		limited := string(c.Scope.Completeness) != "complete"
		if c.Scope.Agent != nil && len(c.Scope.Agent.Gaps) > 0 {
			limited = true
		}
		for _, p := range c.Problems {
			if p.LimitsCoverage {
				limited = true
			}
		}
		if limited {
			gaps = append(gaps, controlApplication(c.Application))
		}
		for _, d := range c.Dependencies {
			dependencies[string(d.Source)] = true
		}
	}
	if len(consumers) == 0 {
		lines = append(lines, "No native consumers found in the checked scope.")
	} else {
		lines = append(lines, "Known native consumers: "+strings.Join(consumers, ", ")+".")
	}
	if len(dependencies) > 0 {
		lines = append(lines, fmt.Sprintf("%d independent dependent sources may become unavailable.", len(dependencies)))
	}
	lines = append(lines, "References and settings stay in place and may stop working. Running sessions may retain loaded content.", "")
	if v.Project.Selected {
		lines = append(lines, "Discovery checks seven native integrations and the selected project. Other projects and machine-wide consumers are unverified.")
	} else {
		lines = append(lines, "Discovery checks seven native integrations without a selected project. Project and machine-wide consumers are unverified.")
	}
	if len(gaps) > 0 {
		lines = append(lines, "Discovery gaps: "+strings.Join(gaps, ", ")+". See Details.")
	}
	lines = append(lines, "Links inside this bundle are removed; their external targets stay intact.")
	return lines
}

func browserChangeSummary(v collection.ChangeView) []string {
	lines := []string{string(v.Intent.Scope) + " " + string(v.Intent.Choice) + " | " + v.State}
	if v.State != "ready" {
		return append(lines, "Reason: "+v.Reason, inspectionProjectLine(v.InspectionProject))
	}
	lines = append(lines, "Skill content stays in place. Only the selected application setting changes.")
	lines = append(lines, browserChangeIntent(v.Intent.Choice), inspectionProjectLine(v.InspectionProject))
	lines = append(lines, "Source: "+v.SourcePath, "Write: "+v.ConfigPath, "")
	lines = append(lines, fmt.Sprintf("Known references: %d. Other applications' settings stay in place.", len(v.KnownReferences)))
	if v.CreateDirectory != "" {
		lines = append(lines, "Creates directory: "+v.CreateDirectory)
	}
	if v.NoWriteNeeded {
		lines = append(lines, "No write needed; confirmation verifies the current choice.")
	}
	lines = append(lines, browserChangeCaveats(v.Intent.Application)...)
	lines = append(lines, "Saved choice and effective availability can differ. Running-session adoption is unverified; reload or restart may be required.", "", "Preview only. No settings changed.")
	return lines
}

func browserChangeCaveats(application string) []string {
	lines := []string{}
	switch application {
	case "hermes":
		lines = append(lines, "Applies only to the active profile. Platform rules, other profiles and context overrides may still prevent availability.")
	case "pi":
		lines = append(lines, "Disabling a name winner can expose an independent same-name copy. Inherit removes the exact project override pair.")
	case "gemini":
		lines = append(lines, "Native name controls apply. Activation consent remains required; other layers may still disable this skill.")
	case "opencode":
		lines = append(lines, "Native name permission controls apply. Inherit leaves other rules active; enable preserves an existing ask permission.")
	case "claude-code":
		lines = append(lines, "Enable restores native defaults. Frontmatter and Skill permission restrictions still apply.")
	}
	return lines
}

func browserGroupSummary(v collection.GroupView) []string {
	lines := []string{"Group " + string(v.Intent.Scope) + " " + string(v.Intent.Choice) + " | " + v.State}
	if v.State != "ready" {
		for _, m := range v.Members {
			if m.Preview.State != "ready" {
				lines = append(lines, controlApplication(m.Application)+": "+m.Preview.State+" | "+m.Preview.Reason)
			}
		}
	}
	lines = append(lines, inspectionProjectLine(v.InspectionProject))
	if v.Source != nil {
		lines = append(lines, "Exact source: "+v.Source.Directory)
	}
	consumers := []string{}
	gaps := []string{}
	for _, c := range v.Consumers {
		if len(c.References) > 0 {
			consumers = append(consumers, fmt.Sprintf("%s (%d)", controlApplication(c.Application), len(c.References)))
		}
		if string(c.Scope.Completeness) != "complete" || c.Scope.Agent != nil && len(c.Scope.Agent.Gaps) > 0 {
			gaps = append(gaps, controlApplication(c.Application))
		}
	}
	if len(consumers) > 0 {
		lines = append(lines, "Known native consumers: "+strings.Join(consumers, ", ")+".")
	}
	if len(gaps) > 0 {
		lines = append(lines, "Discovery gaps: "+strings.Join(gaps, ", ")+". See Details.")
	}
	lines = append(lines, "Order: "+strings.Join(v.Intent.Applications, " -> "), "")
	if v.State != "ready" {
		lines = append(lines, "Whole request unapplied. "+v.Reason)
	}
	for _, m := range v.Members {
		lines = append(lines, controlApplication(m.Application)+": "+m.Preview.State)
		if m.Preview.State != "ready" {
			lines = append(lines, m.Preview.Reason)
		} else {
			lines = append(lines, "Write: "+m.Preview.ConfigPath)
			lines = append(lines, browserChangeIntent(m.Preview.Intent.Choice))
			lines = append(lines, browserChangeCaveats(m.Application)...)
		}
	}
	lines = append(lines, "Skill content and other applications' settings stay in place.", "Saved choice and effective availability can differ. Session adoption is unknown; reload or restart may be required.")
	lines = append(lines, "", "Applies in order; stops on first failure. Earlier changes remain, later applications are skipped.", "Discovery covers listed native roots and the selected project, not the whole machine. Unsupported members block the whole request.")
	return lines
}

func mutationTabs(details bool) string {
	if details {
		return "i Summary | Details"
	}
	return "Summary | d Details"
}

func browserChangeIntent(choice collection.ControlChoice) string {
	switch choice {
	case collection.ControlDisable:
		return "Change: disable this skill."
	case collection.ControlEnable:
		return "Change: enable this skill under native rules."
	case collection.ControlInherit:
		return "Change: remove this skill's project override."
	default:
		return "Change: " + string(choice)
	}
}
