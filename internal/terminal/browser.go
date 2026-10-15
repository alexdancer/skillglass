package terminal

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/charmbracelet/x/term"
)

const wideBrowserMinimum = 96

type browserMode uint8

const (
	browserSources browserMode = iota
	browserProblems
	browserSearch
	browserSourceDetail
	browserProblemDetail
	browserScope
	browserComparison
	browserChangeScope
	browserChangePreview
	browserChangeResult
	browserHelp
)

type sourceTab uint8

const (
	sourceInstructions sourceTab = iota
	sourceDetails
)

type instructionView uint8

const (
	instructionsRendered instructionView = iota
	instructionsRaw
)

type browserSourceLabel struct {
	name       string
	hint       string
	globalHint string
}

type browserModel struct {
	changeChoice         collection.ControlChoice
	changePreview        *collection.ChangePreview
	changeOutcome        collection.ChangeOutcome
	changeScroll         int
	changing             bool
	mutationFailed       bool
	marked               collection.SourceID
	comparison           collection.Comparison
	comparisonGeneration uint64
	compareScroll        int
	comparing            bool
	snapshot             *collection.Snapshot
	filters              collection.Filters
	sources              []collection.Source
	problems             []collection.Problem
	mode                 browserMode
	searchReturn         browserMode
	helpReturn           browserMode
	previousMode         browserMode
	queryBefore          string
	sourceFocus          int
	problemFocus         int
	selected             collection.SourceID
	selectedProblem      collection.ProblemID
	sourceTab            sourceTab
	instructionView      instructionView
	sourceLabels         map[collection.SourceID]browserSourceLabel
	detailScroll         int
	problemScroll        int
	scopeScroll          int
	helpScroll           int
	feedback             string
	theme                browserTheme
	width                int
	height               int
}

func runBrowse(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("browse", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, true, false)
	themeName := flags.String("theme", "auto", "browser theme: auto, dark, light, or none")
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
	theme, err := resolveBrowserTheme(*themeName)
	if err != nil {
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return 2
	}
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	filters, err := optionFilters(snapshot, options, "")
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
	model := newBrowserModel(snapshot, filters, theme)
	program := tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(stdout))
	final, err := program.Run()
	if err != nil {
		writeError(stderr, fmt.Errorf("run browser: %w", err))
		return 1
	}
	if finalModel, ok := final.(browserModel); ok {
		snapshot = finalModel.snapshot
		if finalModel.mutationFailed {
			return 1
		}
	}
	if !snapshot.Complete() {
		return 1
	}
	return 0
}

func newBrowserModel(snapshot *collection.Snapshot, filters collection.Filters, theme browserTheme) browserModel {
	model := browserModel{
		snapshot:     snapshot,
		filters:      filters,
		mode:         browserSources,
		searchReturn: browserSources,
		helpReturn:   browserSources,
		previousMode: browserSources,
		sourceTab:    sourceInstructions,
		theme:        theme,
		width:        80,
		height:       24,
	}
	model.rebuildSourceLabels()
	model.applyView()
	return model
}

func (model browserModel) Init() tea.Cmd {
	return nil
}

func (model browserModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case changeFinished:
		model.changing = false
		model.changeOutcome = message.outcome
		model.mutationFailed = message.outcome.ExitCode() != 0
		if message.outcome.Snapshot != nil {
			model.snapshot = message.outcome.Snapshot
			model.rebuildSourceLabels()
			model.applyView()
		}
		model.mode = browserChangeResult
		model.changeScroll = 0
		model.feedback = ""
		return model, nil
	case comparisonReady:
		comparisonVisible := model.mode == browserComparison || model.mode == browserHelp && model.helpReturn == browserComparison
		if message.generation != model.comparisonGeneration || !comparisonVisible {
			return model, nil
		}
		model.comparing = false
		model.comparison = message.report
		model.feedback = ""
		if message.err != nil {
			model.feedback = safeInline(message.err.Error())
		}
		return model, nil
	case tea.WindowSizeMsg:
		detailAtEnd := model.mode == browserSourceDetail && model.detailScroll == model.maxDetailScroll()
		problemAtEnd := model.mode == browserProblemDetail && model.problemScroll == model.maxProblemScroll()
		scopeAtEnd := model.mode == browserScope && model.scopeScroll == model.maxScopeScroll()
		helpAtEnd := model.mode == browserHelp && model.helpScroll == model.maxHelpScroll()
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
		if helpAtEnd {
			model.helpScroll = model.maxHelpScroll()
		} else {
			model.helpScroll = min(model.helpScroll, model.maxHelpScroll())
		}
		return model, nil
	case tea.PasteMsg:
		if model.mode == browserSearch {
			model.filters.Search += message.Content
			model.applyView()
		}
		return model, nil
	case tea.KeyPressMsg:
		if model.changing {
			return model, nil
		}
		if message.String() == "ctrl+c" {
			return model, tea.Quit
		}
		if model.mode == browserSearch {
			return model.updateSearch(message)
		}
		if message.String() == "q" {
			return model, tea.Quit
		}
		if model.mode == browserHelp {
			return model.updateHelp(message)
		}
		if message.String() == "?" {
			model.helpReturn = model.mode
			model.helpScroll = 0
			model.mode = browserHelp
			return model, nil
		}
		if model.mode == browserChangeScope || model.mode == browserChangePreview || model.mode == browserChangeResult {
			return model.updateChange(message)
		}
		if model.mode == browserComparison {
			return model.updateComparison(message)
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
	case "a":
		if model.snapshot.Scope().Agent != nil {
			model.cycleAvailability()
		}
	case "m":
		if model.mode == browserSources && len(model.sources) > 0 {
			model.marked = model.sources[model.sourceFocus].ID
			model.feedback = "Left marked; select a partner and press v"
		}
	case "v":
		if model.mode == browserSources {
			return model.startComparison()
		}
	case "esc":
		model.marked = ""
		model.feedback = "Comparison mark cleared"
	case "/":
		model.queryBefore = model.filters.Search
		model.searchReturn = model.mode
		model.feedback = ""
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
			model.sourceTab = sourceInstructions
			model.instructionView = instructionsRendered
			model.detailScroll = 0
			model.mode = browserSourceDetail
		}
	}
	return model, nil
}

func (model browserModel) updateSourceDetail(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	switch key.String() {
	case "D":
		return model.openChange(collection.ControlDisable)
	case "e":
		return model.openChange(collection.ControlEnable)
	case "esc", "left":
		model.mode = browserSources
	case "tab":
		model.mode = browserProblems
	case "d":
		model.sourceTab = sourceDetails
		model.detailScroll = 0
	case "i":
		model.sourceTab = sourceInstructions
		model.detailScroll = 0
	case "v":
		if model.sourceTab == sourceInstructions {
			if model.instructionView == instructionsRendered {
				model.instructionView = instructionsRaw
			} else {
				model.instructionView = instructionsRendered
			}
			model.detailScroll = 0
		}
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

func (model browserModel) updateHelp(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "?", "esc", "left":
		model.mode = model.helpReturn
	case "up", "k":
		model.helpScroll = max(0, model.helpScroll-1)
	case "down", "j":
		model.helpScroll = min(model.maxHelpScroll(), model.helpScroll+1)
	case "pgup":
		model.helpScroll = max(0, model.helpScroll-model.helpPageSize())
	case "pgdown":
		model.helpScroll = min(model.maxHelpScroll(), model.helpScroll+model.helpPageSize())
	case "home", "g":
		model.helpScroll = 0
	case "end", "G":
		model.helpScroll = model.maxHelpScroll()
	}
	return model, nil
}

func (model *browserModel) rebuildSourceLabels() {
	allSources := model.snapshot.Query(collection.Filters{Context: collection.FilterAll}, collection.QuerySources).Sources
	globalHints := uniqueSourceIDPrefixes(allSources)
	groups := make(map[string][]collection.Source)
	for _, source := range allSources {
		key := sourceBaseName(source)
		groups[key] = append(groups[key], source)
	}

	model.sourceLabels = make(map[collection.SourceID]browserSourceLabel, len(allSources))
	for _, group := range groups {
		for _, source := range group {
			label := browserSourceLabel{
				name:       sourceBaseName(source),
				globalHint: globalHints[source.ID],
			}
			if len(group) > 1 {
				label.hint = globalHints[source.ID]
			}
			model.sourceLabels[source.ID] = label
		}
	}
}

func uniqueSourceIDPrefixes(sources []collection.Source) map[collection.SourceID]string {
	type entry struct {
		id    collection.SourceID
		value string
	}
	entries := make([]entry, 0, len(sources))
	for _, source := range sources {
		entries = append(entries, entry{
			id:    source.ID,
			value: strings.TrimPrefix(string(source.ID), "src_"),
		})
	}
	slices.SortFunc(entries, func(left, right entry) int {
		return strings.Compare(left.value, right.value)
	})

	prefixes := make(map[collection.SourceID]string, len(entries))
	for index, current := range entries {
		shared := 0
		if index > 0 {
			shared = max(shared, commonPrefixLength(current.value, entries[index-1].value))
		}
		if index+1 < len(entries) {
			shared = max(shared, commonPrefixLength(current.value, entries[index+1].value))
		}
		length := min(len(current.value), max(4, shared+1))
		prefixes[current.id] = current.value[:length]
	}
	return prefixes
}

func commonPrefixLength(left, right string) int {
	length := min(len(left), len(right))
	for index := 0; index < length; index++ {
		if left[index] != right[index] {
			return index
		}
	}
	return length
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
	if model.marked != "" {
		previous, _ := model.snapshot.Source(model.marked)
		current, ok := refreshed.Source(model.marked)
		if !ok || previous.Identity.Certainty != "proven" || current.Identity.Certainty != "proven" {
			model.marked = ""
		}
	}
	objectKind := "directory"
	if previous, ok := model.snapshot.Source(model.selected); ok && previous.Identity.Method == "darwin-file-object" {
		objectKind = "file"
	}
	model.comparisonGeneration++
	model.snapshot = refreshed
	model.rebuildSourceLabels()
	switch change.State {
	case collection.SelectionChangeCleared:
		model.clearSelectedSource()
		model.feedback = "Refresh: selection cleared because its " + objectKind + " object is no longer reachable"
	case collection.SelectionChangeRetained:
		model.feedback = "Refresh: selected " + objectKind + " object retained"
	case collection.SelectionChangeNone:
		model.feedback = "Refresh: collection updated"
	default:
		model.clearSelectedSource()
		model.feedback = "Refresh: selection cleared because reconciliation returned an unknown state"
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
		if _, ok := model.problemByID(model.selectedProblem); !ok {
			model.selectedProblem = ""
			if model.mode == browserProblemDetail {
				model.mode = browserProblems
			}
			if model.mode == browserScope && model.previousMode == browserProblemDetail {
				model.previousMode = browserProblems
			}
			if model.chromeTarget(model.mode) == browserProblems {
				model.feedback = "Refresh: selected problem is no longer reported"
			}
		}
	}
}

func (model *browserModel) clearSelectedSource() {
	model.selected = ""
	if model.mode == browserSourceDetail {
		model.mode = browserSources
	}
	if model.mode == browserScope && model.previousMode == browserSourceDetail {
		model.previousMode = browserSources
	}
}

func (model *browserModel) cycleContext() {
	states := []collection.ContextFilter{collection.FilterAll, collection.FilterProject, collection.FilterCustom}
	if model.snapshot.Scope().Agent != nil {
		states = append(states, collection.FilterGlobal, collection.FilterSystem, collection.FilterManaged)
	}
	current := model.filters.Context
	model.filters.Context = collection.FilterAll
	for index, state := range states {
		if current == state {
			model.filters.Context = states[(index+1)%len(states)]
			break
		}
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

func (model browserModel) activeCollectionMode() browserMode {
	if model.mode == browserSearch {
		return model.searchReturn
	}
	return model.mode
}

func (model browserModel) focusedSource() (collection.Source, bool) {
	if model.sourceFocus < 0 || model.sourceFocus >= len(model.sources) {
		return collection.Source{}, false
	}
	return model.sources[model.sourceFocus], true
}

func (model browserModel) selectedSource() (collection.Source, bool) {
	return model.snapshot.Source(model.selected)
}

func (model browserModel) sourceVisible(id collection.SourceID) bool {
	for _, source := range model.sources {
		if source.ID == id {
			return true
		}
	}
	return false
}

func (model browserModel) problemByID(id collection.ProblemID) (collection.Problem, bool) {
	for _, problem := range model.snapshot.Problems() {
		if problem.ID == id {
			return problem, true
		}
	}
	return collection.Problem{}, false
}

func (model *browserModel) cycleAvailability() {
	states := append([]collection.AvailabilityState{""}, collection.AvailabilityStates()...)
	for index, state := range states {
		if model.filters.Availability == state {
			model.filters.Availability = states[(index+1)%len(states)]
			break
		}
	}
	model.applyView()
	model.feedback = "Availability filter: " + availabilityFilterValue(model.filters.Availability)
}
