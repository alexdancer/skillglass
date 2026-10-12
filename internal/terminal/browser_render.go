package terminal

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/charmbracelet/x/ansi"
)

type scrollPage struct {
	lines     []string
	pageSize  int
	maxScroll int
	start     int
	end       int
	total     int
}

func paginate(lines []string, height, scroll int) scrollPage {
	pageSize := max(1, height)
	maxScroll := max(0, len(lines)-pageSize)
	start := min(max(scroll, 0), maxScroll)
	end := min(len(lines), start+pageSize)
	return scrollPage{
		lines:     append([]string(nil), lines[start:end]...),
		pageSize:  pageSize,
		maxScroll: maxScroll,
		start:     start,
		end:       end,
		total:     len(lines),
	}
}

func (model browserModel) View() tea.View {
	var lines []string
	switch model.mode {
	case browserSourceDetail:
		lines = model.renderSourceReader()
	case browserProblemDetail:
		lines = model.renderProblemDetail()
	case browserScope:
		lines = model.renderScope()
	case browserHelp:
		lines = model.renderHelp()
	default:
		lines = model.renderCollection()
	}
	lines = normalizeFrame(lines, model.width, model.height)
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func (model browserModel) renderCollection() []string {
	target := model.activeCollectionMode()
	header := model.chromeLines(target)
	footer := model.collectionFooter(target)
	contentHeight := max(0, model.height-len(header)-1)
	var body []string
	if target == browserSources && model.width >= wideBrowserMinimum {
		listWidth := min(36, max(32, model.width/3))
		readerWidth := max(1, model.width-listWidth-1)
		left := model.renderSourceList(listWidth, contentHeight)
		source, ok := model.focusedSource()
		right := model.renderReaderPane(source, ok, readerWidth, contentHeight, sourceInstructions, instructionsRendered, 0).lines
		body = joinColumns(left, listWidth, right, readerWidth, model.palette().secondary("│"))
	} else if target == browserProblems {
		body = model.renderProblemList(model.width, contentHeight)
	} else {
		body = model.renderSourceList(model.width, contentHeight)
	}
	return append(append(header, body...), footer)
}

func (model browserModel) renderSourceList(width, height int) []string {
	if height <= 0 {
		return nil
	}
	lines := []string{model.searchLine(width)}
	if height == 1 {
		return lines
	}
	lines = append(lines, model.palette().secondary(strings.Repeat("─", max(1, width))))
	available := height - 2
	if available <= 0 {
		return lines
	}
	if len(model.sources) == 0 {
		message := "No matching sources."
		if model.filters.Search == "" && model.filters.SourceRoot == "" && model.filters.Context == collection.FilterAll && model.filters.Problem == "" {
			message = "No sources found in the selected roots."
		}
		lines = append(lines, fitLine(" "+message, width))
		return padLines(lines, height)
	}

	const rowHeight = 4
	capacity := max(1, available/rowHeight)
	first := min(max(0, model.sourceFocus-capacity+1), max(0, len(model.sources)-capacity))
	for index := first; index < len(model.sources) && len(lines) < height; index++ {
		source := model.sources[index]
		focused := index == model.sourceFocus
		marker := "  "
		if focused {
			marker = "› "
		}
		nameLine := marker + model.fitSourceLabel(source, max(1, width-ansi.StringWidth(marker)))
		descriptionLine := "  " + displayOptional(source.Description)
		locationLine := "  " + model.sourceLocation(source)
		row := []string{
			fitLine(nameLine, width),
			fitLine(descriptionLine, width),
			fitLine(locationLine, width),
			"",
		}
		if focused {
			for rowIndex := 0; rowIndex < 3; rowIndex++ {
				row[rowIndex] = model.palette().selected(padCells(row[rowIndex], width))
			}
		} else {
			row[1] = model.palette().secondary(row[1])
			row[2] = model.palette().secondary(row[2])
		}
		for _, line := range row {
			if len(lines) >= height {
				break
			}
			lines = append(lines, line)
		}
	}
	return padLines(lines, height)
}

func (model browserModel) renderProblemList(width, height int) []string {
	if height <= 0 {
		return nil
	}
	lines := []string{model.searchLine(width)}
	if height == 1 {
		return lines
	}
	lines = append(lines, model.palette().secondary(strings.Repeat("─", max(1, width))))
	available := height - 2
	if available <= 0 {
		return lines
	}
	if len(model.problems) == 0 {
		lines = append(lines, " No matching problems.")
		return padLines(lines, height)
	}

	const rowHeight = 4
	capacity := max(1, available/rowHeight)
	first := min(max(0, model.problemFocus-capacity+1), max(0, len(model.problems)-capacity))
	for index := first; index < len(model.problems) && len(lines) < height; index++ {
		problem := model.problems[index]
		focused := index == model.problemFocus
		marker := "  "
		if focused {
			marker = "› "
		}
		severity := "NOTICE"
		if problem.LimitsCoverage {
			severity = "ERROR"
		}
		row := []string{
			fitLine(marker+severity+"  "+string(problem.Code), width),
			fitLine("  "+safeInline(problem.Summary), width),
			"  " + rightmostCells(safeInline(problem.Path), max(1, width-2)),
			"",
		}
		if focused {
			for rowIndex := 0; rowIndex < 3; rowIndex++ {
				row[rowIndex] = model.palette().selected(padCells(row[rowIndex], width))
			}
		} else {
			row[1] = model.palette().secondary(row[1])
			row[2] = model.palette().secondary(row[2])
		}
		for _, line := range row {
			if len(lines) >= height {
				break
			}
			lines = append(lines, line)
		}
	}
	return padLines(lines, height)
}

func (model browserModel) renderSourceReader() []string {
	header := model.chromeLines(browserSourceDetail)
	contentHeight := max(0, model.height-len(header)-1)
	source, ok := model.selectedSource()
	pane := model.renderReaderPane(
		source,
		ok,
		model.width,
		contentHeight,
		model.sourceTab,
		model.instructionView,
		model.detailScroll,
	)
	return append(append(header, pane.lines...), model.readerFooter(pane))
}

func (model browserModel) renderReaderPane(
	source collection.Source,
	available bool,
	width, height int,
	tab sourceTab,
	presentation instructionView,
	scroll int,
) scrollPage {
	if height <= 0 {
		return scrollPage{}
	}
	innerWidth := max(1, width-2)
	if !available {
		lines := []string{" No source selected", "", " Change the search or return to Sources."}
		return scrollPage{lines: padLines(lines, height), pageSize: max(1, height)}
	}

	header := []string{" " + model.palette().active(model.fitSourceLabel(source, innerWidth))}
	description := wrapLine(safeInline(displayOptional(source.Description)), innerWidth)
	if len(description) > 2 {
		description = description[:2]
	}
	for _, line := range description {
		header = append(header, " "+fitLine(line, innerWidth))
	}
	tabs := model.sourceTabs(tab)
	header = append(header, " "+fitLine(tabs, innerWidth))
	header = append(header, " "+model.palette().secondary(strings.Repeat("─", innerWidth)))

	var body []string
	label := "Rendered instructions"
	if tab == sourceDetails {
		label = "Details"
		body = model.sourceDetailLines(source, innerWidth)
	} else if presentation == instructionsRaw {
		label = "Raw instructions"
		body = rawInstructionLines(source.Manifest.Instructions, innerWidth)
	} else {
		body = renderMarkdownLines(source.Manifest.Instructions, innerWidth, model.theme)
	}
	bodyHeight := max(1, height-len(header)-1)
	page := paginate(body, bodyHeight, scroll)
	status := fmt.Sprintf("%s · Read lines %d-%d of %d", label, visibleStart(page), page.end, page.total)
	lines := append(header, " "+model.palette().secondary(fitLine(status, innerWidth)))
	for _, line := range page.lines {
		lines = append(lines, " "+fitLine(line, innerWidth))
	}
	page.lines = padLines(lines, height)
	return page
}

func (model browserModel) sourceTabs(active sourceTab) string {
	instructions := "Instructions"
	details := "Details"
	if active == sourceInstructions {
		instructions = model.palette().active(instructions)
		details = model.palette().secondary(details)
	} else {
		instructions = model.palette().secondary(instructions)
		details = model.palette().active(details)
	}
	return instructions + "   " + details
}

func (model browserModel) sourceDetailLines(source collection.Source, width int) []string {
	lines := make([]string, 0, 32+len(source.Resources))
	appendField := func(label, value string) {
		lines = append(lines, wrapLine(label+": "+safeInline(value), width)...)
	}
	appendField("Source ID", string(source.ID))
	appendField("Source path", source.Directory)
	appendField("Identity", source.Identity.Method+" ("+source.Identity.Certainty+")")
	appendField("Ownership", source.Ownership.State+" ("+source.Ownership.Reason+")")
	appendField("Availability", source.Availability.State+" ("+source.Availability.Reason+")")
	appendField("Manifest", string(source.Manifest.Status))
	if source.Manifest.IsSymlink {
		appendField("Manifest link", source.Manifest.Path+" -> "+source.Manifest.RawSymlinkTarget)
		appendField("Manifest resolved path", source.Manifest.ResolvedPath)
	}

	lines = append(lines, "", "References")
	references := model.snapshot.ReferencesMatchingFilters(source.ID, model.filters)
	if len(references) == 0 {
		lines = append(lines, "  none")
	}
	for index, reference := range references {
		appendField(fmt.Sprintf("Reference %d ID", index+1), string(reference.ID))
		appendField("  Context", string(reference.Context))
		appendField("  Root ID", string(reference.Root))
		appendField("  Discovery path", reference.DiscoveredPath)
		appendField("  Resolved path", displayOptional(reference.ResolvedPath))
		appendField("  Status", reference.Status)
		appendField("  Matches filters", fmt.Sprintf("%t", reference.MatchesFilters))
	}

	if len(source.Problems) > 0 {
		lines = append(lines, "", "Diagnostics")
		for _, problem := range source.Problems {
			lines = append(lines, wrapLine(
				"  "+string(problem.Code)+": "+safeInline(problem.Summary)+" ("+safeInline(problem.Detail)+")",
				width,
			)...)
		}
	}

	lines = append(lines, "", "Raw metadata")
	if source.Manifest.RawFrontmatter == "" {
		lines = append(lines, "  none")
	} else {
		lines = append(lines, wrappedBlock(source.Manifest.RawFrontmatter, "  ", width)...)
	}
	if source.Manifest.Status != collection.ManifestParsed && source.Manifest.RawText != "" {
		lines = append(lines, "", "Raw manifest")
		lines = append(lines, wrappedBlock(source.Manifest.RawText, "  ", width)...)
	}

	lines = append(lines, "", "Resources")
	if len(source.Resources) == 0 {
		lines = append(lines, "  none")
	}
	for _, resource := range source.Resources {
		line := fmt.Sprintf("  %s · %s · mode %s", safeInline(resource.Path), resource.Kind, resource.Mode)
		if resource.Kind == "file" {
			line += fmt.Sprintf(" · %d bytes", resource.Size)
			if resource.Executable {
				line += " · executable"
			}
		}
		if resource.Kind == "symlink" {
			line += " · target " + safeInline(resource.LinkTarget) + " · scope " + resource.LinkScope
		}
		lines = append(lines, wrapLine(line, width)...)
	}
	return lines
}

func (model browserModel) renderProblemDetail() []string {
	header := model.chromeLines(browserProblemDetail)
	contentHeight := max(0, model.height-len(header)-1)
	problem, ok := model.problemByID(model.selectedProblem)
	if !ok {
		body := padLines([]string{" Selected problem is unavailable."}, contentHeight)
		return append(append(header, body...), model.problemDetailFooter(scrollPage{}))
	}
	pane := model.problemDetailPage(problem, model.width, contentHeight, model.problemScroll)
	return append(append(header, pane.lines...), model.problemDetailFooter(pane))
}

func (model browserModel) problemDetailPage(problem collection.Problem, width, height, scroll int) scrollPage {
	innerWidth := max(1, width-2)
	header := []string{
		" " + model.palette().active(fitLine(string(problem.Code), innerWidth)),
		" " + fitLine(safeInline(problem.Summary), innerWidth),
		" " + model.palette().secondary(strings.Repeat("─", innerWidth)),
	}
	body := model.problemDetailLines(problem, innerWidth)
	bodyHeight := max(1, height-len(header)-1)
	page := paginate(body, bodyHeight, scroll)
	status := fmt.Sprintf("Problem · Problem lines %d-%d of %d", visibleStart(page), page.end, page.total)
	lines := append(header, " "+model.palette().secondary(fitLine(status, innerWidth)))
	for _, line := range page.lines {
		lines = append(lines, " "+fitLine(line, innerWidth))
	}
	page.lines = padLines(lines, height)
	return page
}

func (model browserModel) problemDetailLines(problem collection.Problem, width int) []string {
	fields := []string{
		"Problem ID: " + safeInline(string(problem.ID)),
		"Code: " + string(problem.Code),
		"Path: " + safeInline(problem.Path),
		"Summary: " + safeInline(problem.Summary),
		"Detail: " + displayOptional(problem.Detail),
		fmt.Sprintf("Limits coverage: %t", problem.LimitsCoverage),
	}
	lines := make([]string, 0, len(fields)+len(problem.SourceIDs)+len(problem.ReferenceIDs)+len(problem.RootIDs)+6)
	for _, field := range fields {
		lines = append(lines, wrapLine(field, width)...)
	}
	lines = append(lines, "", "Related sources")
	lines = append(lines, identifiersOrNone(problem.SourceIDs, width)...)
	lines = append(lines, "Related references")
	lines = append(lines, identifiersOrNone(problem.ReferenceIDs, width)...)
	lines = append(lines, "Related roots")
	lines = append(lines, identifiersOrNone(problem.RootIDs, width)...)
	return lines
}

func identifiersOrNone[T ~string](values []T, width int) []string {
	if len(values) == 0 {
		return []string{"  none"}
	}
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, wrapLine("  "+safeInline(string(value)), width)...)
	}
	return lines
}

func (model browserModel) renderScope() []string {
	header := model.chromeLines(browserScope)
	contentHeight := max(0, model.height-len(header)-1)
	innerWidth := max(1, model.width-2)
	body := model.scopeBodyLines(innerWidth)
	page := paginate(body, max(1, contentHeight-2), model.scopeScroll)
	status := fmt.Sprintf("Scope and filters · Scope lines %d-%d of %d", visibleStart(page), page.end, page.total)
	lines := []string{
		" " + model.palette().active("Scope and filters"),
		" " + model.palette().secondary(fitLine(status, innerWidth)),
	}
	for _, line := range page.lines {
		lines = append(lines, " "+fitLine(line, innerWidth))
	}
	lines = padLines(lines, contentHeight)
	return append(append(header, lines...), model.scopeFooter())
}

func (model browserModel) scopeBodyLines(width int) []string {
	scope := model.snapshot.Scope()
	lines := make([]string, 0, 10+len(scope.Roots)*6)
	if scope.Project.Selected {
		lines = append(lines, "Project")
		lines = append(lines, wrapLine("  "+safeInline(scope.Project.Path), width)...)
	} else {
		lines = append(lines, "Project: none selected")
	}
	lines = append(lines, "", "Active filters")
	lines = append(lines, wrapLine("  Search: "+displayOptional(model.filters.Search), width)...)
	lines = append(lines, wrapLine("  Source root: "+displayOptional(model.filters.SourceRoot), width)...)
	lines = append(lines, "  Context: "+string(model.filters.Context))
	lines = append(lines, "  Problem: "+displayOptional(string(model.filters.Problem)))
	for index, root := range scope.Roots {
		lines = append(lines, "")
		lines = append(lines, wrapLine(fmt.Sprintf(
			"Root %d [%s] · Context: %s · Sources: %d · Problems: %d",
			index+1,
			root.State,
			root.Context,
			root.SourceCount,
			root.ProblemCount,
		), width)...)
		lines = append(lines, wrapLine("  Root ID: "+safeInline(string(root.ID)), width)...)
		lines = append(lines, wrapLine("  Requested path: "+safeInline(root.Requested), width)...)
		if root.Resolved != "" {
			lines = append(lines, wrapLine("  Resolved path: "+safeInline(root.Resolved), width)...)
		}
	}
	return lines
}

func (model browserModel) renderHelp() []string {
	header := model.chromeLines(browserHelp)
	contentHeight := max(0, model.height-len(header)-1)
	page := paginate(model.helpBodyLines(), contentHeight, model.helpScroll)
	return append(append(header, padLines(page.lines, contentHeight)...), model.helpFooter())
}

func (model browserModel) helpBodyLines() []string {
	innerWidth := max(1, model.width-4)
	rows := []struct {
		keys   string
		action string
	}{
		{"↑ ↓ / j k", "Move selection or scroll"},
		{"Enter", "Open the focused source or problem; keep search"},
		{"Esc / ←", "Return from reader, scope, problem, or help"},
		{"Tab", "Switch Sources and Problems"},
		{"/", "Edit search in the active inventory"},
		{"Ctrl+U", "Clear search while editing"},
		{"d / i", "Show source Details or Instructions"},
		{"v", "Toggle rendered and raw instructions"},
		{"c", "Cycle all, project, and custom contexts"},
		{"o", "Cycle selected roots"},
		{"f", "Cycle documented problem codes"},
		{"r", "Refresh the collection explicitly"},
		{"s", "Inspect full scope and active filters"},
		{"PgUp / PgDn", "Move one page"},
		{"Home / End", "Move to the top or end"},
		{"g / G", "Move to the top or end"},
		{"?", "Toggle this help"},
		{"q", "Quit"},
	}
	lines := []string{"  " + model.palette().active("Keys"), "  " + model.palette().secondary(strings.Repeat("─", innerWidth))}
	for _, row := range rows {
		prefix := "  " + padCells(row.keys, 12) + "  "
		continuation := strings.Repeat(" ", ansi.StringWidth(prefix))
		actionWidth := max(1, model.width-ansi.StringWidth(prefix))
		wrapped := strings.Split(ansi.Wordwrap(row.action, actionWidth, ""), "\n")
		for index, line := range wrapped {
			if index == 0 {
				lines = append(lines, prefix+line)
			} else {
				lines = append(lines, continuation+line)
			}
		}
	}
	lines = append(lines, "")
	for _, paragraph := range []string{
		"Themes: browse --theme auto|dark|light|none",
		"NO_COLOR forces no-color output. No theme writes settings.",
	} {
		for _, line := range strings.Split(ansi.Wordwrap(paragraph, max(1, model.width-2), ""), "\n") {
			lines = append(lines, "  "+line)
		}
	}
	return lines
}

func (model browserModel) chromeLines(mode browserMode) []string {
	palette := model.palette()
	scope := model.snapshot.Scope()
	coverage := "coverage " + string(scope.Completeness)
	if scope.Completeness == collection.Incomplete {
		coverage = palette.caution(coverage)
	} else {
		coverage = palette.secondary(coverage)
	}
	first := palette.brand("Skillglass") + "  " + coverage
	target := model.chromeTarget(mode)
	sources := "Sources"
	problems := fmt.Sprintf("Problems %d", len(model.snapshot.Problems()))
	if target == browserProblems || target == browserProblemDetail {
		sources = palette.secondary(sources)
		problems = palette.active(problems)
	} else {
		sources = palette.active(sources)
		problems = palette.secondary(problems)
	}
	scopeLabel := "Scope: " + rootCountLabel(len(scope.Roots))
	selection := model.focusSummary(mode)
	var lines []string
	if model.width >= wideBrowserMinimum {
		prefix := sources + "   " + problems + "   " + palette.secondary(scopeLabel) + "   "
		second := prefix + palette.secondary(model.filterSummary(model.width-ansi.StringWidth(prefix)))
		lines = []string{fitLine(first, model.width), fitLine(second, model.width), fitLine(selection, model.width)}
	} else {
		second := sources + "   " + problems + "   " + palette.secondary(scopeLabel)
		lines = []string{
			fitLine(first, model.width),
			fitLine(second, model.width),
			palette.secondary(model.filterSummary(model.width)),
			fitLine(selection, model.width),
		}
	}
	if target == browserSources {
		if hidden := model.hiddenSelectionSummary(); hidden != "" {
			lines = append(lines, fitLine(hidden, model.width))
		}
	}
	return lines
}

func (model browserModel) chromeTarget(mode browserMode) browserMode {
	switch mode {
	case browserSearch:
		return model.searchReturn
	case browserScope:
		return model.previousMode
	case browserHelp:
		return model.helpReturn
	default:
		return mode
	}
}

func (model browserModel) focusSummary(mode browserMode) string {
	mode = model.chromeTarget(mode)
	if mode == browserProblems || mode == browserProblemDetail {
		if mode == browserProblemDetail {
			if problem, ok := model.problemByID(model.selectedProblem); ok {
				return "Selected problem: " + string(problem.Code) + " @ " + safeInline(problem.Path)
			}
			return "Selected problem: unavailable"
		}
		if model.problemFocus >= 0 && model.problemFocus < len(model.problems) {
			problem := model.problems[model.problemFocus]
			return "Focused: " + string(problem.Code) + " @ " + safeInline(problem.Path)
		}
		return "Focused: no matching problems"
	}
	if mode == browserSourceDetail {
		if source, ok := model.selectedSource(); ok {
			if !model.sourceVisible(model.selected) {
				return model.hiddenSelectionSummary()
			}
			return model.sourceIdentitySummary("Selected: ", source)
		}
		return "Selected: unavailable"
	}
	if source, ok := model.focusedSource(); ok {
		return model.sourceIdentitySummary("Focused: ", source)
	}
	return "Focused: no matching sources"
}

func (model browserModel) hiddenSelectionSummary() string {
	if model.selected == "" || model.sourceVisible(model.selected) {
		return ""
	}
	source, ok := model.selectedSource()
	if !ok {
		return ""
	}
	label := model.sourceDisplayLabel(source)
	label.hint = label.globalHint
	marker := " (hidden by filters)"
	spacedLine := "Selected: " + label.name + marker + " " + label.suffix()
	if ansi.StringWidth(spacedLine) <= model.width {
		return spacedLine
	}
	compactLine := "Selected: " + label.name + marker + "[" + label.hint + "]"
	if ansi.StringWidth(compactLine) <= model.width {
		return compactLine
	}
	prefix := "Selected: (hidden by filters) "
	return prefix + label.fit(max(1, model.width-ansi.StringWidth(prefix)))
}

func (model browserModel) sourceIdentitySummary(prefix string, source collection.Source) string {
	label := model.sourceDisplayLabel(source)
	provenance := " @ " + model.sourceLocation(source)
	line := prefix + label.text() + provenance
	if ansi.StringWidth(line) <= model.width {
		return fitLine(line, model.width)
	}

	hint := label.suffix()
	available := model.width - ansi.StringWidth(prefix) - ansi.StringWidth(hint)
	if available <= 0 {
		return fitLine(prefix+hint, model.width)
	}
	nameWidth := ansi.StringWidth(label.name)
	fullProvenanceWidth := ansi.StringWidth(provenance)
	minimumProvenanceWidth := min(12, fullProvenanceWidth)
	provenanceWidth := 0
	if nameWidth <= available-minimumProvenanceWidth {
		provenanceWidth = min(fullProvenanceWidth, available-nameWidth)
	} else {
		minimumNameWidth := min(12, max(1, nameWidth))
		provenanceWidth = min(fullProvenanceWidth, max(0, available-minimumNameWidth))
		nameWidth = max(1, available-provenanceWidth)
	}
	name := ansi.Truncate(label.name, nameWidth, "…")
	compactProvenance := ""
	if provenanceWidth > 0 {
		compactProvenance = ansi.Truncate(provenance, provenanceWidth, "…")
	}
	return prefix + name + hint + compactProvenance
}

func (model browserModel) filterSummary(width int) string {
	type field struct {
		name, compact, value string
	}
	fields := make([]field, 0, 4)
	if model.filters.Search != "" {
		fields = append(fields, field{"search ", "s:", safeInline(model.filters.Search)})
	}
	if model.filters.Context != collection.FilterAll {
		fields = append(fields, field{"context ", "c:", string(model.filters.Context)})
	}
	if model.filters.SourceRoot != "" {
		fields = append(fields, field{"root ", "r:", model.rootFilterName()})
	}
	if model.filters.Problem != "" {
		fields = append(fields, field{"problem ", "p:", string(model.filters.Problem)})
	}
	if len(fields) == 0 {
		return "Filters: all"
	}
	parts := make([]string, len(fields))
	for index, field := range fields {
		parts[index] = field.name + field.value
	}
	full := "Filters: " + strings.Join(parts, " · ")
	if ansi.StringWidth(full) <= width {
		return full
	}

	prefix := "Filters: "
	overhead := ansi.StringWidth(prefix) + len(fields) - 1
	for _, field := range fields {
		overhead += ansi.StringWidth(field.compact)
	}
	if overhead+len(fields) > width {
		overhead -= ansi.StringWidth(prefix)
		prefix = ""
	}
	budgets := make([]int, len(fields))
	remaining := max(0, width-overhead)
	for remaining > 0 {
		allocated := false
		for index, field := range fields {
			if remaining > 0 && budgets[index] < ansi.StringWidth(field.value) {
				budgets[index]++
				remaining--
				allocated = true
			}
		}
		if !allocated {
			break
		}
	}
	for index, field := range fields {
		parts[index] = field.compact + ansi.Truncate(field.value, budgets[index], "…")
	}
	return prefix + strings.Join(parts, " ")
}

func (model browserModel) rootFilterName() string {
	selected := filepath.Base(model.filters.SourceRoot)
	if selected == "." || selected == string(filepath.Separator) || selected == "" {
		selected = model.filters.SourceRoot
	}
	count := 0
	for _, root := range model.uniqueRoots() {
		if filepath.Base(root) == selected {
			count++
		}
	}
	if count <= 1 {
		return safeInline(selected)
	}
	for index, root := range model.uniqueRoots() {
		if root == model.filters.SourceRoot {
			return fmt.Sprintf("#%d/%d %s", index+1, len(model.uniqueRoots()), safeInline(selected))
		}
	}
	return safeInline(selected)
}

func (model browserModel) sourceLocation(source collection.Source) string {
	references := model.snapshot.References(source.ID)
	if len(references) == 0 {
		return "unknown source"
	}
	contexts := make([]string, 0, len(references))
	roots := make([]string, 0, len(references))
	rootLabels := make(map[collection.RootID]string)
	scopeRoots := model.snapshot.Scope().Roots
	for index, root := range scopeRoots {
		label := rootBaseName(root.Requested, root.ID)
		for otherIndex, other := range scopeRoots {
			if otherIndex != index && rootBaseName(other.Requested, other.ID) == label {
				label = fmt.Sprintf("#%d %s", index+1, label)
				break
			}
		}
		rootLabels[root.ID] = label
	}
	for _, reference := range references {
		context := string(reference.Context)
		if !slices.Contains(contexts, context) {
			contexts = append(contexts, context)
		}
		root := rootLabels[reference.Root]
		if root == "" {
			root = string(reference.Root)
		}
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	location := strings.Join(contexts, "+") + " · " + strings.Join(roots, "+")
	if len(references) > 1 {
		location += fmt.Sprintf(" · %d refs", len(references))
	}
	return safeInline(location)
}

func rootBaseName(path string, fallback collection.RootID) string {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return string(fallback)
	}
	return name
}

func (model browserModel) searchLine(width int) string {
	prefix := "Search / "
	cursor := ""
	if model.mode == browserSearch {
		cursor = "█"
	}
	query := safeInline(model.filters.Search)
	if query == "" && model.mode != browserSearch {
		query = model.palette().secondary("type to filter")
	}
	queryWidth := max(0, width-ansi.StringWidth(prefix)-ansi.StringWidth(cursor)-1)
	return fitLine(" "+model.palette().secondary(prefix)+rightmostCells(query, queryWidth)+cursor, width)
}

func (model browserModel) collectionFooter(target browserMode) string {
	if model.mode == browserSearch {
		value := "Type to filter  Enter keep  Esc cancel  Ctrl+U clear"
		if model.width < 60 {
			value = "Type  Enter keep  Esc cancel  Ctrl+U clear"
		}
		return model.footer(value)
	}
	if model.feedback != "" {
		return model.footerWithHint(model.feedback, "? help  q quit")
	}
	if target == browserProblems {
		status := fmt.Sprintf("Problem focus: %d/%d", min(model.problemFocus+1, len(model.problems)), len(model.problems))
		if len(model.problems) == 0 {
			status = "0 matching problems"
		}
		if model.width < 60 && len(model.problems) > 0 {
			status = fmt.Sprintf("%d/%d  ↑↓ move Enter problem", model.problemFocus+1, len(model.problems))
			return model.footerWithHint(status, "? help  q quit")
		}
		return model.footerWithHint(status+"  ↑↓ move Enter problem Tab sources / search", "? help  q quit")
	}
	status := fmt.Sprintf("Focus: %d/%d", min(model.sourceFocus+1, len(model.sources)), len(model.sources))
	if len(model.sources) == 0 {
		status = "0 matches | selected source unchanged"
	}
	action := "↑↓ move Enter open / search ? help q quit"
	if model.width >= wideBrowserMinimum {
		action = "↑↓ move Enter focus / search Tab problems ? help q quit"
	}
	return model.footerWithHint(status+"  "+strings.TrimSuffix(action, " ? help q quit"), "? help  q quit")
}

func (model browserModel) readerFooter(page scrollPage) string {
	if model.feedback != "" {
		return model.footerWithHint(model.feedback, "? help  q quit")
	}
	if model.sourceTab == sourceDetails {
		value := "↑↓ scroll  i instructions  ← back"
		if model.width < 60 {
			value = "↑↓ scroll  i read  Esc back"
		}
		return model.footerWithHint(value, "? help  q quit")
	}
	mode := "v raw"
	if model.instructionView == instructionsRaw {
		mode = "v rendered"
	}
	value := "↑↓ scroll  d details  " + mode + "  ← back"
	if model.width < 60 {
		value = "↑↓ scroll  d details  Esc back"
	}
	return model.footerWithHint(value, "? help  q quit")
}

func (model browserModel) problemDetailFooter(_ scrollPage) string {
	if model.feedback != "" {
		return model.footerWithHint(model.feedback, "? help  q quit")
	}
	value := "↑↓ scroll  ← back"
	if model.width < 60 {
		value = "↑↓ scroll  Esc back"
	}
	return model.footerWithHint(value, "? help  q quit")
}

func (model browserModel) scopeFooter() string {
	if model.feedback != "" {
		return model.footerWithHint(model.feedback, "? help  q quit")
	}
	value := "↑↓ scroll  ← back"
	if model.width < 60 {
		value = "↑↓ scroll  Esc back"
	}
	return model.footerWithHint(value, "? help  q quit")
}

func (model browserModel) helpFooter() string {
	return model.footer("Esc/←/? close  q quit")
}

func (model browserModel) footer(value string) string {
	return model.palette().secondary(fitLine(" "+value, model.width))
}

func (model browserModel) footerWithHint(value, hint string) string {
	hint = safeInline(hint)
	hintWidth := ansi.StringWidth(hint)
	if model.width <= hintWidth+1 {
		return model.footer(hint)
	}
	leftWidth := max(0, model.width-hintWidth-2)
	left := padCells(fitLine(" "+safeInline(value), leftWidth), leftWidth)
	return model.palette().secondary(left + "  " + hint)
}

func (model browserModel) palette() browserPalette {
	return paletteFor(model.theme)
}

func (model browserModel) detailPageSize() int {
	source, ok := model.selectedSource()
	if !ok {
		return 1
	}
	height := max(0, model.height-len(model.chromeLines(browserSourceDetail))-1)
	return model.renderReaderPane(source, true, model.width, height, model.sourceTab, model.instructionView, model.detailScroll).pageSize
}

func (model browserModel) maxDetailScroll() int {
	source, ok := model.selectedSource()
	if !ok {
		return 0
	}
	height := max(0, model.height-len(model.chromeLines(browserSourceDetail))-1)
	return model.renderReaderPane(source, true, model.width, height, model.sourceTab, model.instructionView, model.detailScroll).maxScroll
}

func (model browserModel) problemPageSize() int {
	problem, ok := model.problemByID(model.selectedProblem)
	if !ok {
		return 1
	}
	height := max(0, model.height-len(model.chromeLines(browserProblemDetail))-1)
	return model.problemDetailPage(problem, model.width, height, model.problemScroll).pageSize
}

func (model browserModel) maxProblemScroll() int {
	problem, ok := model.problemByID(model.selectedProblem)
	if !ok {
		return 0
	}
	height := max(0, model.height-len(model.chromeLines(browserProblemDetail))-1)
	return model.problemDetailPage(problem, model.width, height, model.problemScroll).maxScroll
}

func (model browserModel) scopePageSize() int {
	height := max(0, model.height-len(model.chromeLines(browserScope))-1)
	return max(1, height-2)
}

func (model browserModel) maxScopeScroll() int {
	width := max(1, model.width-2)
	return paginate(model.scopeBodyLines(width), model.scopePageSize(), model.scopeScroll).maxScroll
}

func (model browserModel) helpPageSize() int {
	return max(1, model.height-len(model.chromeLines(browserHelp))-1)
}

func (model browserModel) maxHelpScroll() int {
	return paginate(model.helpBodyLines(), model.helpPageSize(), model.helpScroll).maxScroll
}

func visibleStart(page scrollPage) int {
	if page.total == 0 {
		return 0
	}
	return page.start + 1
}

func sourceBaseName(source collection.Source) string {
	if source.Name == "" {
		return "<metadata unavailable>"
	}
	return safeInline(source.Name)
}

func (model browserModel) sourceLabel(source collection.Source) string {
	return model.sourceDisplayLabel(source).text()
}

func (model browserModel) fitSourceLabel(source collection.Source, width int) string {
	return model.sourceDisplayLabel(source).fit(width)
}

func (model browserModel) sourceDisplayLabel(source collection.Source) browserSourceLabel {
	if label, ok := model.sourceLabels[source.ID]; ok {
		return label
	}
	return browserSourceLabel{name: sourceBaseName(source)}
}

func (label browserSourceLabel) text() string {
	return label.name + label.suffix()
}

func (label browserSourceLabel) suffix() string {
	if label.hint == "" {
		return ""
	}
	return " [" + label.hint + "]"
}

func (label browserSourceLabel) fit(width int) string {
	width = max(1, width)
	full := label.text()
	if ansi.StringWidth(full) <= width || label.hint == "" {
		return fitLine(full, width)
	}
	suffix := label.suffix()
	nameWidth := width - ansi.StringWidth(suffix)
	if nameWidth <= 0 {
		return rightmostCells(suffix, width)
	}
	return ansi.Truncate(label.name, nameWidth, "…") + suffix
}

func rootCountLabel(count int) string {
	unit := "roots"
	if count == 1 {
		unit = "root"
	}
	return fmt.Sprintf("%d %s", count, unit)
}

func fitLine(line string, width int) string {
	return ansi.Truncate(line, max(width, 1), "…")
}

func wrapLine(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	return strings.Split(ansi.Hardwrap(line, max(width, 1), true), "\n")
}

func wrappedBlock(value, prefix string, width int) []string {
	inputLines := strings.Split(safeBlock(value), "\n")
	output := make([]string, 0, len(inputLines))
	for _, line := range inputLines {
		output = append(output, wrapLine(prefix+line, width)...)
	}
	return output
}

func rightmostCells(value string, width int) string {
	valueWidth := ansi.StringWidth(value)
	if valueWidth <= width {
		return value
	}
	if width <= 0 {
		return ""
	}
	const overflow = "…"
	if width == 1 {
		return overflow
	}
	removeWidth := max(0, valueWidth-width+1)
	for removeWidth <= valueWidth {
		visible := ansi.TruncateLeft(value, removeWidth, overflow)
		if ansi.StringWidth(visible) <= width {
			return visible
		}
		removeWidth++
	}
	return overflow
}

func padCells(value string, width int) string {
	value = fitLine(value, width)
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}

func padLines(lines []string, height int) []string {
	if len(lines) > height {
		return lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func normalizeFrame(lines []string, width, height int) []string {
	lines = padLines(lines, height)
	for index := range lines {
		lines[index] = fitLine(lines[index], width)
	}
	return lines
}

func joinColumns(left []string, leftWidth int, right []string, rightWidth int, separator string) []string {
	height := max(len(left), len(right))
	left = padLines(left, height)
	right = padLines(right, height)
	lines := make([]string, 0, height)
	for index := 0; index < height; index++ {
		lines = append(lines, padCells(left[index], leftWidth)+separator+fitLine(right[index], rightWidth))
	}
	return lines
}
