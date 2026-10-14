package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
)

func runCompare(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, false, true)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(stdout, usage)
			return 0
		}
		writeError(stderr, err)
		return 2
	}
	if flags.NArg() != 2 {
		writeError(stderr, errors.New("compare requires LEFT_SOURCE_ID RIGHT_SOURCE_ID after flags"))
		return 2
	}
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	report, err := snapshot.Compare(context.Background(), collection.SourceID(flags.Arg(0)), collection.SourceID(flags.Arg(1)))
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if options.jsonOutput {
		err = json.NewEncoder(stdout).Encode(struct {
			Operation       string                 `json:"operation"`
			Scope           collection.Scope       `json:"scope"`
			LeftReferences  []collection.Reference `json:"left_references"`
			RightReferences []collection.Reference `json:"right_references"`
			Report          collection.Comparison  `json:"report"`
		}{"compare", snapshot.Scope(), snapshot.References(report.Left), snapshot.References(report.Right), report})
	} else {
		var output strings.Builder
		output.WriteString(strings.Join(comparisonLines(report), "\n") + "\n\nDiscovery scope (separate from comparison coverage):\n")
		renderScope(&output, snapshot.Scope())
		for _, side := range []struct {
			label string
			id    collection.SourceID
		}{{"Left", report.Left}, {"Right", report.Right}} {
			for _, ref := range snapshot.References(side.id) {
				fmt.Fprintf(&output, "%s reference (%s): %s\n", side.label, ref.Context, safeInline(ref.DiscoveredPath))
			}
		}
		_, err = io.WriteString(stdout, output.String())
	}
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if report.Coverage != collection.Complete {
		return 1
	}
	return 0
}
func comparisonLines(r collection.Comparison) []string {
	lines := []string{"Left: " + safeInline(string(r.Left)) + " | " + safeInline(r.LeftPath), "Right: " + safeInline(string(r.Right)) + " | " + safeInline(r.RightPath), "Relation: " + string(r.Relation), "Comparison coverage: " + string(r.Coverage) + " | Outcome: " + string(r.Outcome)}
	if r.Relation == "same-source" {
		lines = append(lines, "Selections are shared references to one source, not duplicate copies.")
	}
	for _, issue := range r.Issues {
		lines = append(lines, "Coverage: "+issue.Side+" "+safeInline(issue.Path)+": "+safeInline(issue.Reason))
	}
	for _, c := range r.Changes {
		lines = append(lines, "", fmt.Sprintf("%s: %s | %s | kind %s -> %s | mode %s -> %s | bytes %d -> %d", safeInline(c.Path), c.Status, c.Kind, c.LeftKind, c.RightKind, c.LeftMode, c.RightMode, c.LeftSize, c.RightSize))
		if c.LeftLink != "" || c.RightLink != "" {
			lines = append(lines, "Link target: "+safeInline(c.LeftLink)+" -> "+safeInline(c.RightLink))
		}
		for _, line := range c.Diff {
			lines = append(lines, safeInline(line))
		}
		if c.DiffTruncated {
			lines = append(lines, "Diff presentation truncated; byte comparison completed.")
		}
	}
	return lines
}

type comparisonReady struct {
	generation uint64
	report     collection.Comparison
	err        error
}

func (model browserModel) startComparison() (tea.Model, tea.Cmd) {
	if model.marked == "" || len(model.sources) == 0 {
		model.feedback = "Press m to mark the left source first"
		return model, nil
	}
	model.comparisonGeneration++
	generation := model.comparisonGeneration
	snapshot := model.snapshot
	left, right := model.marked, model.sources[model.sourceFocus].ID
	model.mode = browserComparison
	model.compareScroll = 0
	model.comparing = true
	model.feedback = "Comparing current bundle bytes..."
	a, _ := snapshot.Source(left)
	b, _ := snapshot.Source(right)
	model.comparison = collection.Comparison{Left: left, Right: right, LeftPath: a.Directory, RightPath: b.Directory, Relation: "unknown", Outcome: "indeterminate", Coverage: collection.Incomplete}
	return model, func() tea.Msg {
		report, err := snapshot.Compare(context.Background(), left, right)
		return comparisonReady{generation, report, err}
	}
}
func (model browserModel) comparisonPage() pagedView {
	header := []string{fitLine("SKILLGLASS | Comparison", model.width)}
	scope := model.snapshot.Scope()
	project := "Project: none selected"
	if scope.Project.Selected {
		project = "Project: " + safeInline(scope.Project.Path)
	}
	header = append(header, fitLine(project, model.width))
	header = append(header, fitLine("Left: "+string(model.comparison.Left)+" | "+safeInline(filepath.Base(model.comparison.LeftPath)), model.width), fitLine("Right: "+string(model.comparison.Right)+" | "+safeInline(filepath.Base(model.comparison.RightPath)), model.width))
	body := []string{"Reading current bundle contents..."}
	if !model.comparing {
		body = nil
		for _, line := range comparisonLines(model.comparison) {
			body = append(body, wrapLine(line, model.width)...)
		}
		header = append(header, fitLine("Coverage: "+string(model.comparison.Coverage)+" | "+string(model.comparison.Outcome), model.width))
	}
	return pagedView{palette: model.palette(), header: header, body: body, help: wrapGroups([]string{"j/k scroll", "PgUp/PgDn", "g/G top/end", "Esc back", "r refresh", "? help", "q quit"}, model.width), statusLabel: "Compare lines", feedback: model.feedback, scroll: model.compareScroll, width: model.width, height: model.height}
}
func (model browserModel) updateComparison(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	layout := model.comparisonPage().layout()
	switch key.String() {
	case "esc":
		model.comparisonGeneration++
		model.mode = browserSources
		model.feedback = ""
	case "r":
		model.comparisonGeneration++
		model.refresh()
		model.mode = browserSources
	case "j", "down":
		model.compareScroll++
	case "k", "up":
		model.compareScroll--
	case "pgdown":
		model.compareScroll += layout.pageSize
	case "pgup":
		model.compareScroll -= layout.pageSize
	case "g", "home":
		model.compareScroll = 0
	case "G", "end":
		model.compareScroll = layout.maxScroll
	}
	model.compareScroll = min(max(model.compareScroll, 0), layout.maxScroll)
	return model, nil
}
