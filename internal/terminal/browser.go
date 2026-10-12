package terminal

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type browserMode uint8

const (
	browserSources browserMode = iota
	browserProblems
	browserSearch
	browserSourceDetail
	browserProblemDetail
	browserScope
)

type browserModel struct {
	snapshot        *collection.Snapshot
	filters         collection.Filters
	sources         []collection.Source
	problems        []collection.Problem
	mode            browserMode
	searchReturn    browserMode
	queryBefore     string
	sourceFocus     int
	problemFocus    int
	selected        collection.SourceID
	selectedProblem collection.ProblemID
	detailScroll    int
	problemScroll   int
	scopeScroll     int
	previousMode    browserMode
	feedback        string
	width           int
	height          int
}

func runBrowse(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("browse", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, true, false)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(stdout, usage)
			return 0
		}
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return 2
	}
	if flags.NArg() != 0 {
		writeError(stderr, errors.New("browse does not accept positional arguments"))
		io.WriteString(stderr, usage)
		return 2
	}
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	filters, err := snapshot.NewFilters("", options.sourceRoot, options.context, options.problem)
	if err != nil {
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return 2
	}

	inputFile, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(inputFile.Fd()) {
		writeError(stderr, errors.New("browse requires an interactive terminal"))
		return 1
	}
	model := newBrowserModel(snapshot, filters)
	program := tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(stdout))
	final, err := program.Run()
	if err != nil {
		writeError(stderr, fmt.Errorf("run browser: %w", err))
		return 1
	}
	if finalModel, ok := final.(browserModel); ok {
		snapshot = finalModel.snapshot
	}
	if !snapshot.Complete() {
		return 1
	}
	return 0
}

func newBrowserModel(snapshot *collection.Snapshot, filters collection.Filters) browserModel {
	model := browserModel{
		snapshot:     snapshot,
		filters:      filters,
		mode:         browserSources,
		searchReturn: browserSources,
		width:        80,
		height:       24,
	}
	model.applyView()
	return model
}

func (model browserModel) Init() tea.Cmd {
	return nil
}

func (model browserModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		detailAtEnd := model.mode == browserSourceDetail && model.detailScroll == model.maxDetailScroll()
		problemAtEnd := model.mode == browserProblemDetail && model.problemScroll == model.maxProblemScroll()
		scopeAtEnd := model.mode == browserScope && model.scopeScroll == model.maxScopeScroll()
		model.width = max(message.Width, 20)
		model.height = max(message.Height, 8)
		if detailAtEnd {
			model.detailScroll = model.maxDetailScroll()
		} else {
			model.detailScroll = min(model.detailScroll, model.maxDetailScroll())
		}
		if problemAtEnd {
			model.problemScroll = model.maxProblemScroll()
		} else {
			model.problemScroll = min(model.problemScroll, model.maxProblemScroll())
		}
		if scopeAtEnd {
			model.scopeScroll = model.maxScopeScroll()
		} else {
			model.scopeScroll = min(model.scopeScroll, model.maxScopeScroll())
		}
		return model, nil
	case tea.PasteMsg:
		if model.mode == browserSearch {
			model.filters.Search += message.Content
			model.applyView()
		}
		return model, nil
	case tea.KeyPressMsg:
		if message.String() == "ctrl+c" {
			return model, tea.Quit
		}
		if model.mode == browserSearch {
			return model.updateSearch(message)
		}
		if message.String() == "q" {
			return model, tea.Quit
		}
		if model.mode == browserScope {
			return model.updateScope(message)
		}
		if model.mode == browserSourceDetail {
			return model.updateSourceDetail(message)
		}
		if model.mode == browserProblemDetail {
			return model.updateProblemDetail(message)
		}
		return model.updateCollection(message)
	}
	return model, nil
}

func (model browserModel) updateSearch(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "enter":
		model.mode = model.searchReturn
		return model, nil
	case "esc":
		model.filters.Search = model.queryBefore
		model.applyView()
		model.mode = model.searchReturn
		return model, nil
	case "backspace":
		if model.filters.Search != "" {
			_, size := utf8.DecodeLastRuneInString(model.filters.Search)
			model.filters.Search = model.filters.Search[:len(model.filters.Search)-size]
			model.applyView()
		}
		return model, nil
	case "ctrl+u":
		model.filters.Search = ""
		model.applyView()
		return model, nil
	}
	if text := key.Key().Text; text != "" {
		model.filters.Search += text
		model.applyView()
	}
	return model, nil
}

func (model browserModel) updateCollection(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "/":
		model.queryBefore = model.filters.Search
		model.searchReturn = model.mode
		model.mode = browserSearch
	case "tab":
		if model.mode == browserProblems {
			model.mode = browserSources
		} else {
			model.mode = browserProblems
		}
		model.feedback = ""
	case "r":
		model.refresh()
	case "c":
		model.cycleContext()
	case "o":
		model.cycleSourceRoot()
	case "f":
		model.cycleProblem()
	case "s":
		model.previousMode = model.mode
		model.scopeScroll = 0
		model.mode = browserScope
	case "up", "k":
		model.moveFocus(-1)
	case "down", "j":
		model.moveFocus(1)
	case "home", "g":
		model.setFocusStart()
	case "end", "G":
		model.setFocusEnd()
	case "enter":
		if model.mode == browserProblems {
			if len(model.problems) > 0 {
				model.selectedProblem = model.problems[model.problemFocus].ID
				model.problemScroll = 0
				model.mode = browserProblemDetail
			}
		} else if len(model.sources) > 0 {
			model.selected = model.sources[model.sourceFocus].ID
			model.detailScroll = 0
			model.mode = browserSourceDetail
		}
	}
	return model, nil
}

func (model browserModel) updateSourceDetail(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	switch key.String() {
	case "esc", "left":
		model.mode = browserSources
	case "tab":
		model.mode = browserProblems
	case "r":
		model.refresh()
	case "c":
		model.cycleContext()
	case "o":
		model.cycleSourceRoot()
	case "f":
		model.cycleProblem()
	case "s":
		model.previousMode = browserSourceDetail
		model.scopeScroll = 0
		model.mode = browserScope
	case "up", "k":
		model.detailScroll = max(0, model.detailScroll-1)
	case "down", "j":
		model.detailScroll = min(model.maxDetailScroll(), model.detailScroll+1)
	case "pgup":
		model.detailScroll = max(0, model.detailScroll-model.detailPageSize())
	case "pgdown":
		model.detailScroll = min(model.maxDetailScroll(), model.detailScroll+model.detailPageSize())
	case "home", "g":
		model.detailScroll = 0
	case "end", "G":
		model.detailScroll = model.maxDetailScroll()
		command = tea.ClearScreen
	}
	return model, command
}

func (model browserModel) updateProblemDetail(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "left":
		model.mode = browserProblems
	case "tab":
		model.mode = browserSources
	case "r":
		model.refresh()
	case "c":
		model.cycleContext()
	case "o":
		model.cycleSourceRoot()
	case "f":
		model.cycleProblem()
	case "s":
		model.previousMode = browserProblemDetail
		model.scopeScroll = 0
		model.mode = browserScope
	case "up", "k":
		model.problemScroll = max(0, model.problemScroll-1)
	case "down", "j":
		model.problemScroll = min(model.maxProblemScroll(), model.problemScroll+1)
	case "pgup":
		model.problemScroll = max(0, model.problemScroll-model.problemPageSize())
	case "pgdown":
		model.problemScroll = min(model.maxProblemScroll(), model.problemScroll+model.problemPageSize())
	case "home", "g":
		model.problemScroll = 0
	case "end", "G":
		model.problemScroll = model.maxProblemScroll()
	}
	return model, nil
}

func (model browserModel) updateScope(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "left":
		model.mode = model.previousMode
	case "r":
		model.refresh()
	case "up", "k":
		model.scopeScroll = max(0, model.scopeScroll-1)
	case "down", "j":
		model.scopeScroll = min(model.maxScopeScroll(), model.scopeScroll+1)
	case "pgup":
		model.scopeScroll = max(0, model.scopeScroll-model.scopePageSize())
	case "pgdown":
		model.scopeScroll = min(model.maxScopeScroll(), model.scopeScroll+model.scopePageSize())
	case "home", "g":
		model.scopeScroll = 0
	case "end", "G":
		model.scopeScroll = model.maxScopeScroll()
	}
	return model, nil
}

func (model *browserModel) applyView() {
	focusedSource := collection.SourceID("")
	if model.sourceFocus >= 0 && model.sourceFocus < len(model.sources) {
		focusedSource = model.sources[model.sourceFocus].ID
	}
	focusedProblem := collection.ProblemID("")
	if model.problemFocus >= 0 && model.problemFocus < len(model.problems) {
		focusedProblem = model.problems[model.problemFocus].ID
	}
	sourceView := model.snapshot.Query(model.filters, collection.QuerySources)
	problemView := model.snapshot.Query(model.filters, collection.QueryProblems)
	model.sources = sourceView.Sources
	model.problems = problemView.Problems
	model.sourceFocus = focusedSourceIndex(model.sources, focusedSource)
	model.problemFocus = focusedProblemIndex(model.problems, focusedProblem)
}

func focusedSourceIndex(sources []collection.Source, focused collection.SourceID) int {
	for index, source := range sources {
		if source.ID == focused {
			return index
		}
	}
	return 0
}

func focusedProblemIndex(problems []collection.Problem, focused collection.ProblemID) int {
	for index, problem := range problems {
		if problem.ID == focused {
			return index
		}
	}
	return 0
}

func (model *browserModel) refresh() {
	detailAtEnd := model.mode == browserSourceDetail && model.detailScroll == model.maxDetailScroll()
	problemAtEnd := model.mode == browserProblemDetail && model.problemScroll == model.maxProblemScroll()
	refreshed, change, err := model.snapshot.Refresh(context.Background(), model.selected)
	if err != nil {
		model.feedback = "Refresh failed: " + safeInline(err.Error())
		return
	}
	model.snapshot = refreshed
	if change.State == "cleared" {
		model.selected = ""
		if model.mode == browserSourceDetail {
			model.mode = browserSources
		}
		model.feedback = "Refresh: selection cleared because its directory object is no longer reachable"
	} else if change.State == "retained" {
		model.feedback = "Refresh: selected directory object retained"
	} else {
		model.feedback = "Refresh: collection updated"
	}
	model.applyView()
	if detailAtEnd {
		model.detailScroll = model.maxDetailScroll()
	} else {
		model.detailScroll = min(model.detailScroll, model.maxDetailScroll())
	}
	if problemAtEnd {
		model.problemScroll = model.maxProblemScroll()
	} else {
		model.problemScroll = min(model.problemScroll, model.maxProblemScroll())
	}
	if model.selectedProblem != "" {
		if _, ok := model.problemByID(model.selectedProblem); !ok && model.mode == browserProblemDetail {
			model.mode = browserProblems
			model.feedback = "Refresh: selected problem is no longer reported"
		}
	}
}

func (model *browserModel) cycleContext() {
	switch model.filters.Context {
	case collection.FilterAll:
		model.filters.Context = collection.FilterProject
	case collection.FilterProject:
		model.filters.Context = collection.FilterCustom
	default:
		model.filters.Context = collection.FilterAll
	}
	model.feedback = "Context filter: " + string(model.filters.Context)
	model.applyView()
}

func (model *browserModel) cycleSourceRoot() {
	roots := model.uniqueRoots()
	if len(roots) == 0 {
		return
	}
	next := roots[0]
	if model.filters.SourceRoot != "" {
		next = ""
		for index, root := range roots {
			if root == model.filters.SourceRoot && index+1 < len(roots) {
				next = roots[index+1]
				break
			}
		}
	}
	model.filters.SourceRoot = next
	if next == "" {
		model.feedback = "Source-root filter: all selected roots"
	} else {
		model.feedback = "Source-root filter: " + safeInline(next)
	}
	model.applyView()
}

func (model *browserModel) uniqueRoots() []string {
	seen := make(map[string]struct{})
	var roots []string
	for _, root := range model.snapshot.Scope().Roots {
		if _, exists := seen[root.Requested]; exists {
			continue
		}
		seen[root.Requested] = struct{}{}
		roots = append(roots, root.Requested)
	}
	return roots
}

func (model *browserModel) cycleProblem() {
	codes := collection.ProblemCodes()
	next := collection.ProblemCode("")
	if model.filters.Problem == "" && len(codes) > 0 {
		next = codes[0]
	} else {
		for index, code := range codes {
			if code == model.filters.Problem && index+1 < len(codes) {
				next = codes[index+1]
				break
			}
		}
	}
	model.filters.Problem = next
	if next == "" {
		model.feedback = "Problem filter: all problem codes"
	} else {
		model.feedback = "Problem filter: " + string(next)
	}
	model.applyView()
}

func (model *browserModel) moveFocus(delta int) {
	if model.mode == browserProblems {
		model.problemFocus = min(max(0, model.problemFocus+delta), max(0, len(model.problems)-1))
		return
	}
	model.sourceFocus = min(max(0, model.sourceFocus+delta), max(0, len(model.sources)-1))
}

func (model *browserModel) setFocusStart() {
	if model.mode == browserProblems {
		model.problemFocus = 0
	} else {
		model.sourceFocus = 0
	}
}

func (model *browserModel) setFocusEnd() {
	if model.mode == browserProblems {
		model.problemFocus = max(0, len(model.problems)-1)
	} else {
		model.sourceFocus = max(0, len(model.sources)-1)
	}
}

func (model browserModel) View() tea.View {
	var lines []string
	switch model.mode {
	case browserSourceDetail:
		lines = model.renderSourceDetail()
	case browserProblemDetail:
		lines = model.renderProblemDetail()
	case browserScope:
		lines = model.renderScope()
	default:
		lines = model.renderCollection()
	}
	if len(lines) > model.height {
		lines = lines[:model.height]
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func (model browserModel) renderCollection() []string {
	target := model.activeCollectionMode()
	header := model.headerLines(target)
	header = append(header, model.searchLine())
	var body []string
	var focusLines []int
	var status string
	if target == browserProblems {
		body, focusLines = model.problemBodyLines()
		status = model.problemStatus()
	} else {
		body, focusLines = model.inventoryBodyLines()
		status = model.inventoryStatus()
	}
	detailHelp := "Enter read"
	if target == browserProblems {
		detailHelp = "Enter problem"
	}
	helpGroups := []string{
		"↑/↓ or j/k move", "/ search", detailHelp, "Tab sources/problems",
		"r refresh", "c context", "o root", "f problem", "s scope", "q quit",
	}
	if model.mode == browserSearch {
		helpGroups = []string{"type to search", "Enter keep", "Esc cancel", "Ctrl+U clear"}
	}
	footer := make([]string, 0)
	if model.feedback != "" {
		footer = append(footer, fitLine(model.feedback, model.width))
	}
	footer = append(footer, wrapLine(status, model.width)...)
	footer = append(footer, wrapGroups(helpGroups, model.width)...)
	available := max(1, model.height-len(header)-len(footer))
	focus := model.sourceFocus
	if target == browserProblems {
		focus = model.problemFocus
	}
	start := 0
	if focus >= 0 && focus < len(focusLines) {
		start = max(0, focusLines[focus]-available/3)
	}
	if start+available > len(body) {
		start = max(0, len(body)-available)
	}
	end := min(len(body), start+available)
	lines := append([]string(nil), header...)
	lines = append(lines, body[start:end]...)
	for len(lines) < model.height-len(footer) {
		lines = append(lines, "")
	}
	return append(lines, footer...)
}

func (model browserModel) activeCollectionMode() browserMode {
	if model.mode == browserSearch {
		return model.searchReturn
	}
	return model.mode
}

func (model browserModel) searchLine() string {
	const prefix = "Search: "
	cursor := ""
	if model.mode == browserSearch {
		cursor = "█"
	}
	query := safeInline(model.filters.Search)
	queryWidth := max(0, model.width-ansi.StringWidth(prefix)-ansi.StringWidth(cursor))
	return prefix + rightmostCells(query, queryWidth) + cursor
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
	overflowWidth := ansi.StringWidth(overflow)
	if width <= overflowWidth {
		return ansi.Truncate(overflow, width, "")
	}
	suffixWidth := width - overflowWidth
	removeWidth := max(0, valueWidth-suffixWidth)
	for removeWidth <= valueWidth {
		visible := ansi.TruncateLeft(value, removeWidth, overflow)
		if ansi.StringWidth(visible) <= width {
			return visible
		}
		removeWidth++
	}
	return ansi.Truncate(overflow, width, "")
}

func (model browserModel) headerLines(mode browserMode) []string {
	scope := model.snapshot.Scope()
	title := "SKILLGLASS | Sources"
	if mode == browserProblems {
		title = "SKILLGLASS | Problems"
	} else if mode == browserSourceDetail {
		title = "SKILLGLASS | Source"
	} else if mode == browserProblemDetail {
		title = "SKILLGLASS | Problem"
	}
	project := "Project: none selected"
	if scope.Project.Selected {
		project = "Project: " + safeInline(scope.Project.Path)
	}
	scopeSummary := fmt.Sprintf(
		"Scope: %s | Coverage: %s | Problems: %d | s view",
		rootCountLabel(len(scope.Roots)),
		scope.Completeness,
		len(model.snapshot.Problems()),
	)
	filterSummary := fmt.Sprintf(
		"Filters: root=%s | context=%s | problem=%s",
		optionalValue(model.filters.SourceRoot),
		model.filters.Context,
		optionalValue(string(model.filters.Problem)),
	)
	selected := model.selectedSummary(mode)
	return []string{
		fitLine(title, model.width),
		fitLine(project, model.width),
		fitLine(scopeSummary, model.width),
		fitLine(filterSummary, model.width),
		fitLine(selected, model.width),
	}
}

func (model browserModel) selectedSummary(mode browserMode) string {
	if mode == browserProblems || mode == browserProblemDetail {
		if model.selectedProblem == "" {
			return "Selected problem: none"
		}
		if problem, ok := model.problemByID(model.selectedProblem); ok {
			return fmt.Sprintf("Selected problem: %s [%s]", problem.Code, problem.ID)
		}
		return "Selected problem: unavailable"
	}
	if model.selected == "" {
		return "Selected: none"
	}
	source, ok := model.snapshot.Source(model.selected)
	if !ok {
		return "Selected: unavailable"
	}
	name := source.Name
	if name == "" {
		name = "<metadata unavailable>"
	}
	hidden := ""
	if !model.sourceVisible(model.selected) {
		hidden = " (hidden by filters)"
	}
	return fmt.Sprintf("Selected: %s [%s]%s", safeInline(name), safeInline(string(source.ID)), hidden)
}

func (model browserModel) sourceVisible(id collection.SourceID) bool {
	for _, source := range model.sources {
		if source.ID == id {
			return true
		}
	}
	return false
}

func rootCountLabel(count int) string {
	unit := "roots"
	if count == 1 {
		unit = "root"
	}
	return fmt.Sprintf("%d %s", count, unit)
}

func (model browserModel) inventoryBodyLines() ([]string, []int) {
	if len(model.sources) == 0 {
		lines := []string{"No matching sources."}
		if model.filters.Search == "" && model.filters.SourceRoot == "" && model.filters.Context == collection.FilterAll && model.filters.Problem == "" {
			lines[0] = "No sources found in the selected roots."
		}
		return lines, nil
	}
	lines := make([]string, 0, len(model.sources)*3)
	focusLines := make([]int, 0, len(model.sources))
	for index, source := range model.sources {
		focusLines = append(focusLines, len(lines))
		marker := "  "
		if index == model.sourceFocus {
			marker = "> "
		}
		name := source.Name
		if name == "" {
			name = "<metadata unavailable>"
		}
		lines = append(lines, wrapLine(
			fmt.Sprintf("%s%s [%s]", marker, safeInline(name), safeInline(string(source.ID))),
			model.width,
		)...)
		lines = append(lines, wrapLine("  "+displayOptional(source.Description), model.width)...)
	}
	return lines, focusLines
}

func (model browserModel) problemBodyLines() ([]string, []int) {
	if len(model.problems) == 0 {
		return []string{"No matching problems."}, nil
	}
	lines := make([]string, 0, len(model.problems)*3)
	focusLines := make([]int, 0, len(model.problems))
	for index, problem := range model.problems {
		focusLines = append(focusLines, len(lines))
		marker := "  "
		if index == model.problemFocus {
			marker = "> "
		}
		lines = append(lines, wrapLine(
			fmt.Sprintf("%s%s [%s]", marker, problem.Code, problem.ID),
			model.width,
		)...)
		lines = append(lines, wrapLine("  "+safeInline(problem.Path), model.width)...)
		lines = append(lines, wrapLine("  "+safeInline(problem.Summary), model.width)...)
	}
	return lines, focusLines
}

func (model browserModel) inventoryStatus() string {
	if len(model.sources) == 0 {
		if model.filters.Search == "" && model.filters.SourceRoot == "" && model.filters.Context == collection.FilterAll && model.filters.Problem == "" {
			return "0 sources"
		}
		return "0 matches | selected source unchanged"
	}
	return fmt.Sprintf("Focus: %d/%d", model.sourceFocus+1, len(model.sources))
}

func (model browserModel) problemStatus() string {
	if len(model.problems) == 0 {
		return "0 matching problems"
	}
	return fmt.Sprintf("Problem focus: %d/%d", model.problemFocus+1, len(model.problems))
}

func (model browserModel) renderSourceDetail() []string {
	header := model.headerLines(browserSourceDetail)
	source, ok := model.snapshot.Source(model.selected)
	if !ok {
		header = append(header, "Selected source is unavailable")
		return append(header, "Esc back | q quit")
	}
	body := model.detailBodyLines(source)
	pageSize := model.detailPageSizeFor(len(body))
	footer := make([]string, 0)
	if model.feedback != "" {
		footer = append(footer, fitLine(model.feedback, model.width))
	}
	footer = append(footer, wrapLine(fmt.Sprintf(
		"Read lines %d-%d of %d",
		min(len(body), model.detailScroll+1),
		min(len(body), model.detailScroll+pageSize),
		len(body),
	), model.width)...)
	footer = append(footer, model.detailHelpLines()...)
	footerCapacity := model.detailFooterCapacity(len(body))
	available := max(1, model.height-len(header)-footerCapacity)
	start := min(model.detailScroll, max(0, len(body)-available))
	end := min(len(body), start+available)
	lines := append([]string(nil), header...)
	lines = append(lines, body[start:end]...)
	for len(lines) < model.height-footerCapacity {
		lines = append(lines, "")
	}
	for len(footer) < footerCapacity {
		footer = append([]string{""}, footer...)
	}
	return append(lines, footer...)
}

func (model browserModel) detailBodyLines(source collection.Source) []string {
	lines := make([]string, 0)
	appendField := func(label, value string) {
		lines = append(lines, wrapLine(label+": "+safeInline(value), model.width)...)
	}
	appendField("Description", optionalValue(source.Description))
	appendField("Source", source.Directory)
	for _, reference := range model.snapshot.References(source.ID) {
		appendField("Reference "+string(reference.Context), reference.DiscoveredPath)
		if reference.ResolvedPath != "" && reference.ResolvedPath != reference.DiscoveredPath {
			appendField("Resolved reference", reference.ResolvedPath)
		}
	}
	appendField("Identity", source.Identity.Method+" ("+source.Identity.Certainty+")")
	appendField("Ownership", source.Ownership.State+" ("+source.Ownership.Reason+")")
	appendField("Availability", source.Availability.State+" ("+source.Availability.Reason+")")
	appendField("Manifest", string(source.Manifest.Status))
	if source.Manifest.IsSymlink {
		appendField("Manifest link", source.Manifest.Path+" -> "+source.Manifest.RawSymlinkTarget)
		appendField("Manifest resolved", source.Manifest.ResolvedPath)
	}
	if len(source.Problems) > 0 {
		lines = append(lines, "", "Diagnostics:")
		for _, problem := range source.Problems {
			lines = append(lines, wrapLine(
				"  "+string(problem.Code)+": "+safeInline(problem.Summary)+" ("+safeInline(problem.Detail)+")",
				model.width,
			)...)
		}
	}
	if source.Manifest.Status == collection.ManifestParsed {
		lines = append(lines, "", "Metadata:")
		lines = append(lines, wrappedBlock(source.Manifest.RawFrontmatter, "  ", model.width)...)
		lines = append(lines, "", "Instructions:")
		lines = append(lines, wrappedBlock(source.Manifest.Instructions, "  ", model.width)...)
	} else if source.Manifest.RawText != "" {
		lines = append(lines, "", "Raw manifest:")
		lines = append(lines, wrappedBlock(source.Manifest.RawText, "  ", model.width)...)
	}
	lines = append(lines, "", "Resources:")
	if len(source.Resources) == 0 {
		lines = append(lines, "  none")
	}
	for _, resource := range source.Resources {
		line := fmt.Sprintf("  %s | %s | mode=%s", safeInline(resource.Path), resource.Kind, resource.Mode)
		if resource.Kind == "file" {
			line += fmt.Sprintf(" | size=%d", resource.Size)
			if resource.Executable {
				line += " | executable"
			}
		}
		if resource.Kind == "symlink" {
			line += " | target=" + safeInline(resource.LinkTarget) + " | scope=" + resource.LinkScope
		}
		lines = append(lines, wrapLine(line, model.width)...)
	}
	return lines
}

func (model browserModel) renderProblemDetail() []string {
	header := model.headerLines(browserProblemDetail)
	problem, ok := model.problemByID(model.selectedProblem)
	if !ok {
		header = append(header, "Selected problem is unavailable")
		return append(header, "Esc back | q quit")
	}
	body := model.problemDetailLines(problem)
	pageSize := model.problemPageSizeFor(len(body))
	footer := make([]string, 0)
	if model.feedback != "" {
		footer = append(footer, fitLine(model.feedback, model.width))
	}
	footer = append(footer, wrapLine(fmt.Sprintf(
		"Problem lines %d-%d of %d",
		min(len(body), model.problemScroll+1),
		min(len(body), model.problemScroll+pageSize),
		len(body),
	), model.width)...)
	footer = append(footer, model.problemHelpLines()...)
	footerCapacity := model.problemFooterCapacity(len(body))
	available := max(1, model.height-len(header)-footerCapacity)
	start := min(model.problemScroll, max(0, len(body)-available))
	end := min(len(body), start+available)
	lines := append([]string(nil), header...)
	lines = append(lines, body[start:end]...)
	for len(lines) < model.height-footerCapacity {
		lines = append(lines, "")
	}
	for len(footer) < footerCapacity {
		footer = append([]string{""}, footer...)
	}
	return append(lines, footer...)
}

func (model browserModel) problemDetailLines(problem collection.Problem) []string {
	fields := []string{
		"Code: " + string(problem.Code),
		"Path: " + safeInline(problem.Path),
		"Summary: " + safeInline(problem.Summary),
		"Detail: " + displayOptional(problem.Detail),
		fmt.Sprintf("Limits coverage: %t", problem.LimitsCoverage),
	}
	lines := make([]string, 0, len(fields)+len(problem.SourceIDs)+len(problem.ReferenceIDs)+len(problem.RootIDs)+6)
	for _, field := range fields {
		lines = append(lines, wrapLine(field, model.width)...)
	}
	lines = append(lines, "", "Related sources:")
	if len(problem.SourceIDs) == 0 {
		lines = append(lines, "  none")
	}
	for _, id := range problem.SourceIDs {
		lines = append(lines, wrapLine("  "+safeInline(string(id)), model.width)...)
	}
	lines = append(lines, "Related references:")
	if len(problem.ReferenceIDs) == 0 {
		lines = append(lines, "  none")
	}
	for _, id := range problem.ReferenceIDs {
		lines = append(lines, wrapLine("  "+safeInline(string(id)), model.width)...)
	}
	lines = append(lines, "Related roots:")
	if len(problem.RootIDs) == 0 {
		lines = append(lines, "  none")
	}
	for _, id := range problem.RootIDs {
		lines = append(lines, wrapLine("  "+safeInline(string(id)), model.width)...)
	}
	return lines
}

func (model browserModel) problemByID(id collection.ProblemID) (collection.Problem, bool) {
	for _, problem := range model.snapshot.Problems() {
		if problem.ID == id {
			return problem, true
		}
	}
	return collection.Problem{}, false
}

func (model browserModel) renderScope() []string {
	header := []string{fitLine("SKILLGLASS | Scope and filters", model.width)}
	body := model.scopeBodyLines()
	pageSize := model.scopePageSizeFor(len(body))
	footer := make([]string, 0)
	if model.feedback != "" {
		footer = append(footer, fitLine(model.feedback, model.width))
	}
	footer = append(footer, wrapLine(fmt.Sprintf(
		"Scope lines %d-%d of %d",
		min(len(body), model.scopeScroll+1),
		min(len(body), model.scopeScroll+pageSize),
		len(body),
	), model.width)...)
	footer = append(footer, model.scopeHelpLines()...)
	footerCapacity := model.scopeFooterCapacity(len(body))
	available := max(1, model.height-len(header)-footerCapacity)
	start := min(model.scopeScroll, max(0, len(body)-available))
	end := min(len(body), start+available)
	lines := append([]string(nil), header...)
	lines = append(lines, body[start:end]...)
	for len(lines) < model.height-footerCapacity {
		lines = append(lines, "")
	}
	for len(footer) < footerCapacity {
		footer = append([]string{""}, footer...)
	}
	return append(lines, footer...)
}

func (model browserModel) scopeBodyLines() []string {
	scope := model.snapshot.Scope()
	lines := make([]string, 0, 8+len(scope.Roots)*4)
	if scope.Project.Selected {
		lines = append(lines, "Project:")
		lines = append(lines, wrapLine("  "+safeInline(scope.Project.Path), model.width)...)
	} else {
		lines = append(lines, "Project: none selected")
	}
	lines = append(lines, "", "Active filters:")
	lines = append(lines, wrapLine("  Search: "+displayOptional(model.filters.Search), model.width)...)
	lines = append(lines, wrapLine("  Source root: "+displayOptional(model.filters.SourceRoot), model.width)...)
	lines = append(lines, "  Context: "+string(model.filters.Context))
	lines = append(lines, "  Problem: "+optionalValue(string(model.filters.Problem)))
	for index, root := range scope.Roots {
		lines = append(lines, "")
		lines = append(lines, wrapLine(fmt.Sprintf(
			"Root %d [%s] | Context: %s | Sources: %d | Problems: %d",
			index+1,
			root.State,
			root.Context,
			root.SourceCount,
			root.ProblemCount,
		), model.width)...)
		lines = append(lines, wrapLine("  Requested: "+safeInline(root.Requested), model.width)...)
		if root.Resolved != "" && root.Resolved != root.Requested {
			lines = append(lines, wrapLine("  Resolved: "+safeInline(root.Resolved), model.width)...)
		}
	}
	return lines
}

func (model browserModel) detailPageSize() int {
	source, ok := model.snapshot.Source(model.selected)
	if !ok {
		return 1
	}
	return model.detailPageSizeFor(len(model.detailBodyLines(source)))
}

func (model browserModel) detailPageSizeFor(bodyLength int) int {
	return max(1, model.height-len(model.headerLines(browserSourceDetail))-model.detailFooterCapacity(bodyLength))
}

func (model browserModel) detailFooterCapacity(bodyLength int) int {
	lastLine := max(1, bodyLength)
	capacity := len(wrapLine(fmt.Sprintf("Read lines %d-%d of %d", lastLine, lastLine, bodyLength), model.width)) + len(model.detailHelpLines())
	if model.feedback != "" {
		capacity++
	}
	return capacity
}

func (model browserModel) detailHelpLines() []string {
	return wrapGroups(
		[]string{"↑/↓ or j/k scroll", "PgUp/PgDn", "g/G top/end", "r refresh", "s scope", "Esc back", "q quit"},
		model.width,
	)
}

func (model browserModel) maxDetailScroll() int {
	source, ok := model.snapshot.Source(model.selected)
	if !ok {
		return 0
	}
	return max(0, len(model.detailBodyLines(source))-model.detailPageSize())
}

func (model browserModel) problemPageSize() int {
	problem, ok := model.problemByID(model.selectedProblem)
	if !ok {
		return 1
	}
	return model.problemPageSizeFor(len(model.problemDetailLines(problem)))
}

func (model browserModel) problemPageSizeFor(bodyLength int) int {
	return max(1, model.height-len(model.headerLines(browserProblemDetail))-model.problemFooterCapacity(bodyLength))
}

func (model browserModel) problemFooterCapacity(bodyLength int) int {
	lastLine := max(1, bodyLength)
	capacity := len(wrapLine(fmt.Sprintf("Problem lines %d-%d of %d", lastLine, lastLine, bodyLength), model.width)) + len(model.problemHelpLines())
	if model.feedback != "" {
		capacity++
	}
	return capacity
}

func (model browserModel) problemHelpLines() []string {
	return wrapGroups(
		[]string{"↑/↓ or j/k scroll", "PgUp/PgDn", "g/G top/end", "r refresh", "s scope", "Esc back", "q quit"},
		model.width,
	)
}

func (model browserModel) maxProblemScroll() int {
	problem, ok := model.problemByID(model.selectedProblem)
	if !ok {
		return 0
	}
	return max(0, len(model.problemDetailLines(problem))-model.problemPageSize())
}

func (model browserModel) scopePageSize() int {
	return model.scopePageSizeFor(len(model.scopeBodyLines()))
}

func (model browserModel) scopePageSizeFor(bodyLength int) int {
	return max(1, model.height-1-model.scopeFooterCapacity(bodyLength))
}

func (model browserModel) scopeFooterCapacity(bodyLength int) int {
	lastLine := max(1, bodyLength)
	capacity := len(wrapLine(fmt.Sprintf("Scope lines %d-%d of %d", lastLine, lastLine, bodyLength), model.width)) + len(model.scopeHelpLines())
	if model.feedback != "" {
		capacity++
	}
	return capacity
}

func (model browserModel) scopeHelpLines() []string {
	return wrapGroups(
		[]string{"↑/↓ or j/k scroll", "PgUp/PgDn", "g/G top/end", "r refresh", "Esc back", "q quit"},
		model.width,
	)
}

func (model browserModel) maxScopeScroll() int {
	bodyLength := len(model.scopeBodyLines())
	return max(0, bodyLength-model.scopePageSizeFor(bodyLength))
}

func fitLine(line string, width int) string {
	return ansi.Truncate(line, max(width, 1), "…")
}

func wrapGroups(groups []string, width int) []string {
	lines := make([]string, 0, len(groups))
	current := ""
	for _, group := range groups {
		candidate := group
		if current != "" {
			candidate = current + " | " + group
		}
		if current == "" || ansi.StringWidth(candidate) <= max(width, 1) {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = group
	}
	if current != "" {
		lines = append(lines, wrapLine(current, width)...)
	}
	return lines
}

func wrapLine(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	wrapped := ansi.Hardwrap(line, max(width, 1), true)
	return strings.Split(wrapped, "\n")
}

func wrappedBlock(value, prefix string, width int) []string {
	safe := safeBlock(value)
	inputLines := strings.Split(safe, "\n")
	output := make([]string, 0, len(inputLines))
	for _, line := range inputLines {
		output = append(output, wrapLine(prefix+line, width)...)
	}
	return output
}

func optionalValue(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
