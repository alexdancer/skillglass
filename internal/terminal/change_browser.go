package terminal

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
)

type changeFinished struct{ outcome collection.ChangeOutcome }

func (model browserModel) openChange(choice collection.ControlChoice) (tea.Model, tea.Cmd) {
	scope := model.snapshot.Scope()
	if scope.Agent == nil || !supportsSkillControls(scope.Agent.Application) {
		model.feedback = "Controls are unsupported for this agent scope."
		return model, nil
	}
	model.mutationDetails = false
	model.changeChoice = choice
	model.changePreview = nil
	model.changeScroll = 0
	model.mode = browserChangeScope
	model.feedback = ""
	return model, nil
}
func (model browserModel) updateChange(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.changing {
		return model, nil
	}
	if model.mode == browserChangeScope {
		switch key.String() {
		case "esc", "enter":
			model.mode = browserSourceDetail
			model.feedback = "Cancelled. No settings changed."
		case "j", "down", "k", "up", "pgdown", "pgup", "home", "end":
			layout := model.changePage().layout()
			switch key.String() {
			case "j", "down":
				model.changeScroll++
			case "k", "up":
				model.changeScroll--
			case "pgdown":
				model.changeScroll += layout.pageSize
			case "pgup":
				model.changeScroll -= layout.pageSize
			case "home":
				model.changeScroll = 0
			case "end":
				model.changeScroll = layout.maxScroll
			}
			model.changeScroll = min(max(model.changeScroll, 0), layout.maxScroll)
		case "g", "p", "i":
			scope := collection.ControlGlobal
			choice := model.changeChoice
			if key.String() == "p" || key.String() == "i" {
				scope = collection.ControlProject
			}
			if key.String() == "i" {
				choice = collection.ControlInherit
			}
			model.changePreview = model.snapshot.PreviewChange(context.Background(), collection.ChangeIntent{Source: model.selected, Application: model.snapshot.Scope().Agent.Application, Scope: scope, Choice: choice})
			model.mode = browserChangePreview
			model.changeScroll = 0
		}
		return model, nil
	}
	layout := model.changePage().layout()
	switch key.String() {
	case "esc", "enter":
		if model.mode == browserChangeResult {
			model.feedback = fmt.Sprintf("Result: %s | persistence: %s", model.changeOutcome.State, model.changeOutcome.Persistence)
		} else {
			model.feedback = "Cancelled. No settings changed."
		}
		model.mode = browserSourceDetail
		model.changePreview = nil
	case "d", "i":
		model.mutationDetails = key.String() == "d"
		model.changeScroll = 0
	case "y":
		if model.mode == browserChangePreview && model.changePreview != nil && model.changePreview.View().State == "ready" {
			held := model.changePreview
			token := held.View().Confirmation
			model.changing = true
			model.feedback = "Applying confirmed choice..."
			return model, func() tea.Msg { return changeFinished{held.Apply(context.Background(), token)} }
		}
	case "r":
		model.changePreview = nil
		model.mode = browserSourceDetail
		model.refresh()
	case "j", "down":
		model.changeScroll++
	case "k", "up":
		model.changeScroll--
	case "pgdown":
		model.changeScroll += layout.pageSize
	case "pgup":
		model.changeScroll -= layout.pageSize
	case "g", "home":
		model.changeScroll = 0
	case "G", "end":
		model.changeScroll = layout.maxScroll
	}
	model.changeScroll = min(max(model.changeScroll, 0), layout.maxScroll)
	return model, nil
}
func (model browserModel) changePage() pagedView {
	action := string(model.changeChoice)
	if model.changePreview != nil {
		intent := model.changePreview.View().Intent
		action = string(intent.Scope) + " " + string(intent.Choice)
	}
	if model.mode == browserChangeResult {
		action = string(model.changeOutcome.Intent.Scope) + " " + string(model.changeOutcome.Intent.Choice)
	}
	header := []string{fitLine("SKILLGLASS | "+controlApplication(model.snapshot.Scope().Agent.Application)+" control", model.width), fitLine("Action: "+action, model.width)}
	if source, ok := model.snapshot.Source(model.selected); ok {
		header = append(header, "Selected: "+model.fitSourceLabel(source, model.width-len("Selected: ")))
	}
	header = append(header, "")
	body := []string{"Select the setting scope explicitly.", "", "g Global user configuration", "p Project control: unavailable", "i Remove project override / inherit: unavailable", "", inspectionProjectLine(model.snapshot.Scope().Project)}
	if model.snapshot.Scope().Agent.Application == "hermes" {
		body[2] = "g Global active-profile configuration"
	}
	if model.snapshot.Scope().Agent.Application == "gemini" || model.snapshot.Scope().Agent.Application == "claude-code" || model.snapshot.Scope().Agent.Application == "pi" || model.snapshot.Scope().Agent.Application == "opencode" {
		body[3] = "p Project configuration"
		body[4] = "i Remove project override / inherit"
	}
	help := wrapGroups([]string{"g Global", "p Project", "i Inherit", "Esc cancel"}, model.width)
	if model.mode == browserChangePreview {
		header = append(header[:len(header)-1], mutationTabs(model.mutationDetails), "")
		body = browserChangeSummary(model.changePreview.View())
		if model.mutationDetails {
			body = browserDiagnostics(changePreviewLines(model.changePreview.View()))
		}
		help = wrapGroups([]string{"d details", "i summary", "↑↓ scroll", "y Apply " + action, "Enter/Esc Cancel", "? help", "q quit"}, model.width)
		if model.changePreview.View().State != "ready" {
			help = wrapGroups([]string{"d details", "i summary", "↑↓ scroll", "Enter/Esc Back"}, model.width)
		}
	} else if model.mode == browserChangeResult {
		body = append(changeOutcomeLines(model.changeOutcome), inspectionProjectLine(model.snapshot.Scope().Project))
		help = wrapGroups([]string{"j/k scroll", "PgUp/PgDn", "g/G top/end", "Enter/Esc Back", "r reinspect", "? help", "q quit"}, model.width)
	}
	lines := []string{}
	for _, line := range body {
		for _, paragraph := range strings.Split(ansi.Wordwrap(safeInline(line), max(model.width, 1), ""), "\n") {
			lines = append(lines, wrapLine(paragraph, model.width)...)
		}
	}
	return pagedView{palette: model.palette(), header: header, body: lines, help: help, statusLabel: "Control lines", feedback: model.feedback, scroll: model.changeScroll, width: model.width, height: model.height}
}
