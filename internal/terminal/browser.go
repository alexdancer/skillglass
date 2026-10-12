package terminal

import (
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
	browserList browserMode = iota
	browserSearch
	browserDetail
	browserScope
)

type browserModel struct {
	snapshot     *collection.Snapshot
	sources      []collection.Source
	mode         browserMode
	query        string
	queryBefore  string
	focus        int
	selected     collection.SourceID
	detailScroll int
	scopeScroll  int
	previousMode browserMode
	width        int
	height       int
}

func runBrowse(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("browse", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var roots stringList
	var project optionalString
	flags.Var(&roots, "root", "explicit discovery root; repeatable")
	flags.Var(&project, "project", "selected project context")
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
	snapshot, code := buildSnapshot(roots, project, stderr)
	if snapshot == nil {
		return code
	}

	inputFile, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(inputFile.Fd()) {
		writeError(stderr, errors.New("browse requires an interactive terminal"))
		return 1
	}
	model := newBrowserModel(snapshot)
	program := tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(stdout))
	if _, err := program.Run(); err != nil {
		writeError(stderr, fmt.Errorf("run browser: %w", err))
		return 1
	}
	if !snapshot.Complete() {
		return 1
	}
	return 0
}

func newBrowserModel(snapshot *collection.Snapshot) browserModel {
	return browserModel{
		snapshot: snapshot,
		sources:  snapshot.Query("").Sources,
		width:    80,
		height:   24,
	}
}

func (model browserModel) Init() tea.Cmd {
	return nil
}

func (model browserModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		detailAtEnd := model.mode == browserDetail && model.detailScroll == model.maxDetailScroll()
		scopeAtEnd := model.mode == browserScope && model.scopeScroll == model.maxScopeScroll()
		model.width = max(message.Width, 20)
		model.height = max(message.Height, 8)
		if detailAtEnd {
			model.detailScroll = model.maxDetailScroll()
		} else {
			model.detailScroll = min(model.detailScroll, model.maxDetailScroll())
		}
		if scopeAtEnd {
			model.scopeScroll = model.maxScopeScroll()
		} else {
			model.scopeScroll = min(model.scopeScroll, model.maxScopeScroll())
		}
		return model, nil
	case tea.PasteMsg:
		if model.mode == browserSearch {
			model.query += message.Content
			model.applyQuery()
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
		if model.mode == browserDetail {
			return model.updateDetail(message)
		}
		return model.updateList(message)
	}
	return model, nil
}

func (model browserModel) updateSearch(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "enter":
		model.mode = browserList
		return model, nil
	case "esc":
		model.query = model.queryBefore
		model.applyQuery()
		model.mode = browserList
		return model, nil
	case "backspace":
		if model.query != "" {
			_, size := utf8.DecodeLastRuneInString(model.query)
			model.query = model.query[:len(model.query)-size]
			model.applyQuery()
		}
		return model, nil
	case "ctrl+u":
		model.query = ""
		model.applyQuery()
		return model, nil
	}
	if text := key.Key().Text; text != "" {
		model.query += text
		model.applyQuery()
	}
	return model, nil
}

func (model browserModel) updateList(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "/":
		model.queryBefore = model.query
		model.mode = browserSearch
	case "s":
		model.previousMode = browserList
		model.scopeScroll = 0
		model.mode = browserScope
	case "up", "k":
		if model.focus > 0 {
			model.focus--
		}
	case "down", "j":
		if model.focus+1 < len(model.sources) {
			model.focus++
		}
	case "home", "g":
		model.focus = 0
	case "end", "G":
		if len(model.sources) > 0 {
			model.focus = len(model.sources) - 1
		}
	case "enter":
		if len(model.sources) > 0 {
			model.selected = model.sources[model.focus].ID
			model.detailScroll = 0
			model.mode = browserDetail
		}
	}
	return model, nil
}

func (model browserModel) updateDetail(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "left":
		model.mode = browserList
	case "s":
		model.previousMode = browserDetail
		model.scopeScroll = 0
		model.mode = browserScope
	case "up", "k":
		model.detailScroll = max(0, model.detailScroll-1)
	case "down", "j":
		model.detailScroll = min(model.maxDetailScroll(), model.detailScroll+1)
	case "pgup":
		model.detailScroll = max(0, model.detailScroll-model.detailPageSize())
	case "pgdown":
		model.detailScroll = min(
			model.maxDetailScroll(),
			model.detailScroll+model.detailPageSize(),
		)
	case "home", "g":
		model.detailScroll = 0
	case "end", "G":
		model.detailScroll = model.maxDetailScroll()
	}
	return model, nil
}

func (model browserModel) updateScope(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "left":
		model.mode = model.previousMode
	case "up", "k":
		model.scopeScroll = max(0, model.scopeScroll-1)
	case "down", "j":
		model.scopeScroll = min(model.maxScopeScroll(), model.scopeScroll+1)
	case "pgup":
		model.scopeScroll = max(0, model.scopeScroll-model.scopePageSize())
	case "pgdown":
		model.scopeScroll = min(
			model.maxScopeScroll(),
			model.scopeScroll+model.scopePageSize(),
		)
	case "home", "g":
		model.scopeScroll = 0
	case "end", "G":
		model.scopeScroll = model.maxScopeScroll()
	}
	return model, nil
}

func (model *browserModel) applyQuery() {
	focused := collection.SourceID("")
	if model.focus >= 0 && model.focus < len(model.sources) {
		focused = model.sources[model.focus].ID
	}
	model.sources = model.snapshot.Query(model.query).Sources
	model.focus = 0
	for index, source := range model.sources {
		if source.ID == focused {
			model.focus = index
			break
		}
	}
}

func (model browserModel) View() tea.View {
	var lines []string
	switch model.mode {
	case browserDetail:
		lines = model.renderDetail()
	case browserScope:
		lines = model.renderScope()
	default:
		lines = model.renderInventory()
	}
	if len(lines) > model.height {
		lines = lines[:model.height]
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func (model browserModel) renderInventory() []string {
	header := model.headerLines()
	header = append(header, model.searchLine())

	body, focusLines := model.inventoryBodyLines()
	helpGroups := []string{"↑/↓ or j/k move", "/ search", "Enter read", "s scope", "q quit"}
	if model.mode == browserSearch {
		helpGroups = []string{"type to search", "Enter keep", "Esc cancel", "Ctrl+U clear"}
	}
	footer := append(wrapLine(model.inventoryStatus(), model.width), wrapGroups(helpGroups, model.width)...)
	available := max(1, model.height-len(header)-len(footer))
	start := 0
	if model.focus < len(focusLines) {
		start = max(0, focusLines[model.focus]-available/3)
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

func (model browserModel) searchLine() string {
	const prefix = "Search: "
	cursor := ""
	if model.mode == browserSearch {
		cursor = "█"
	}
	query := safeInline(model.query)
	queryWidth := max(
		0,
		model.width-ansi.StringWidth(prefix)-ansi.StringWidth(cursor),
	)
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

func (model browserModel) renderDetail() []string {
	header := model.headerLines()
	source, ok := model.snapshot.Source(model.selected)
	if !ok {
		header = append(header, "Selected source is unavailable")
		return append(header, "Esc back | q quit")
	}
	body := model.detailBodyLines(source)
	pageSize := model.detailPageSizeFor(len(body))
	footer := append(
		wrapLine(fmt.Sprintf(
			"Read lines %d-%d of %d",
			min(len(body), model.detailScroll+1),
			min(len(body), model.detailScroll+pageSize),
			len(body),
		), model.width),
		model.detailHelpLines()...,
	)
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

func (model browserModel) headerLines() []string {
	scope := model.snapshot.Scope()
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
	selected := "Selected: none"
	if model.selected != "" {
		if source, ok := model.snapshot.Source(model.selected); ok {
			name := source.Name
			if name == "" {
				name = "<metadata unavailable>"
			}
			selected = fmt.Sprintf("Selected: %s [%s]", safeInline(name), safeInline(string(source.ID)))
		} else {
			selected = "Selected: unavailable"
		}
	}
	return []string{
		fitLine("SKILLGLASS | Browse", model.width),
		fitLine(project, model.width),
		fitLine(scopeSummary, model.width),
		fitLine(selected, model.width),
	}
}

func rootCountLabel(count int) string {
	unit := "roots"
	if count == 1 {
		unit = "root"
	}
	return fmt.Sprintf("%d %s", count, unit)
}

func (model browserModel) renderScope() []string {
	header := []string{fitLine("SKILLGLASS | Scope", model.width)}
	body := model.scopeBodyLines()
	pageSize := model.scopePageSizeFor(len(body))
	footer := append(
		wrapLine(fmt.Sprintf(
			"Scope lines %d-%d of %d",
			min(len(body), model.scopeScroll+1),
			min(len(body), model.scopeScroll+pageSize),
			len(body),
		), model.width),
		model.scopeHelpLines()...,
	)
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
	lines := make([]string, 0, 2+len(scope.Roots)*4)
	if scope.Project.Selected {
		lines = append(lines, "Project:")
		lines = append(lines, wrapLine("  "+safeInline(scope.Project.Path), model.width)...)
	} else {
		lines = append(lines, "Project: none selected")
	}
	for index, root := range scope.Roots {
		lines = append(lines, "")
		lines = append(lines, wrapLine(fmt.Sprintf(
			"Root %d [%s] | Sources: %d | Problems: %d",
			index+1,
			root.State,
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

func (model browserModel) inventoryBodyLines() ([]string, []int) {
	if len(model.sources) == 0 {
		lines := []string{"No matching sources."}
		if model.query == "" {
			lines[0] = "No sources found in the selected roots."
		}
		for _, problem := range model.snapshot.Problems() {
			lines = append(lines, wrapLine(
				fmt.Sprintf(
					"%s at %s: %s",
					problem.Code,
					safeInline(problem.Path),
					safeInline(problem.Summary),
				),
				model.width,
			)...)
		}
		return lines, nil
	}

	lines := make([]string, 0, len(model.sources)*3)
	focusLines := make([]int, 0, len(model.sources))
	for index, source := range model.sources {
		focusLines = append(focusLines, len(lines))
		marker := "  "
		if index == model.focus {
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

func (model browserModel) inventoryStatus() string {
	if len(model.sources) == 0 {
		if model.query == "" {
			return "0 sources"
		}
		return "0 matches | selected source unchanged"
	}
	return fmt.Sprintf("Focus: %d/%d", model.focus+1, len(model.sources))
}

func (model browserModel) detailBodyLines(source collection.Source) []string {
	lines := make([]string, 0)
	appendField := func(label, value string) {
		lines = append(lines, wrapLine(label+": "+safeInline(value), model.width)...)
	}
	appendField("Description", optionalValue(source.Description))
	appendField("Source", source.Directory)
	for _, reference := range model.snapshot.References(source.ID) {
		appendField("Discovered", reference.DiscoveredPath)
	}
	appendField("Identity", source.Identity.Method+" ("+source.Identity.Certainty+")")
	appendField("Ownership", source.Ownership.State+" ("+source.Ownership.Reason+")")
	appendField("Availability", source.Availability.State+" ("+source.Availability.Reason+")")
	appendField("Manifest", string(source.Manifest.Status))
	if source.Manifest.IsSymlink {
		appendField("Manifest link", source.Manifest.Path+" -> "+source.Manifest.RawSymlinkTarget)
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

func (model browserModel) detailPageSize() int {
	source, ok := model.snapshot.Source(model.selected)
	if !ok {
		return 1
	}
	return model.detailPageSizeFor(len(model.detailBodyLines(source)))
}

func (model browserModel) detailPageSizeFor(bodyLength int) int {
	return max(1, model.height-len(model.headerLines())-model.detailFooterCapacity(bodyLength))
}

func (model browserModel) detailFooterCapacity(bodyLength int) int {
	lastLine := max(1, bodyLength)
	return len(wrapLine(
		fmt.Sprintf("Read lines %d-%d of %d", lastLine, lastLine, bodyLength),
		model.width,
	)) + len(model.detailHelpLines())
}

func (model browserModel) detailHelpLines() []string {
	return wrapGroups(
		[]string{"↑/↓ or j/k scroll", "PgUp/PgDn", "g/G top/end", "s scope", "Esc back", "q quit"},
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

func (model browserModel) scopePageSize() int {
	return model.scopePageSizeFor(len(model.scopeBodyLines()))
}

func (model browserModel) scopePageSizeFor(bodyLength int) int {
	return max(1, model.height-1-model.scopeFooterCapacity(bodyLength))
}

func (model browserModel) scopeFooterCapacity(bodyLength int) int {
	lastLine := max(1, bodyLength)
	return len(wrapLine(
		fmt.Sprintf("Scope lines %d-%d of %d", lastLine, lastLine, bodyLength),
		model.width,
	)) + len(model.scopeHelpLines())
}

func (model browserModel) scopeHelpLines() []string {
	return wrapGroups(
		[]string{"↑/↓ or j/k scroll", "PgUp/PgDn", "g/G top/end", "Esc back", "q quit"},
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
