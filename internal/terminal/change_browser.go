package terminal

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
)

type changeFinished struct{ outcome collection.ChangeOutcome }

func (model browserModel) openChange(choice collection.ControlChoice) (tea.Model, tea.Cmd) {
	scope := model.snapshot.Scope()
	if scope.Agent == nil || scope.Agent.Application != "codex" {
		model.feedback = "Controls are unsupported for this agent scope."
		return model, nil
	}
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
		case "g", "p":
			scope := collection.ControlGlobal
			if key.String() == "p" {
				scope = collection.ControlProject
			}
			model.changePreview = model.snapshot.PreviewChange(context.Background(), collection.ChangeIntent{Source: model.selected, Application: "codex", Scope: scope, Choice: model.changeChoice})
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
	case "y":
		if model.mode == browserChangePreview && model.changePreview != nil && model.changePreview.View().State == "ready" {
			held := model.changePreview
			token := held.View().Confirmation
			model.changing = true
			model.feedback = "Applying confirmed global choice..."
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
	header := []string{fitLine("SKILLGLASS | Codex control", model.width), fitLine(fmt.Sprintf("Action: global %s", model.changeChoice), model.width)}
	if source, ok := model.snapshot.Source(model.selected); ok {
		header = append(header, fitLine("Selected: "+safeInline(source.Name), model.width), fitLine("Selector: "+string(source.ID), model.width))
	}
	if model.snapshot.Scope().Project.Selected {
		header = append(header, fitLine("Inspection: selected project (details below)", model.width))
	}
	body := []string{"Select the setting scope explicitly.", "g Global user configuration", "p Project control: unavailable", "Enter or Esc cancel"}
	help := []string{"g Global | p Project unavailable", "Enter/Esc Cancel | q quit"}
	if model.mode == browserChangePreview {
		body = changePreviewLines(model.changePreview.View())
		help = wrapGroups([]string{"j/k scroll", "PgUp/PgDn", "g/G top/end", "y Apply global " + string(model.changeChoice), "Enter/Esc Cancel", "q quit"}, model.width)
		if model.changePreview.View().State != "ready" {
			help = wrapGroups([]string{"j/k scroll", "PgUp/PgDn", "Enter/Esc Back", "q quit"}, model.width)
		}
	} else if model.mode == browserChangeResult {
		body = changeOutcomeLines(model.changeOutcome)
		help = wrapGroups([]string{"j/k scroll", "PgUp/PgDn", "g/G top/end", "Enter/Esc Back", "r reinspect", "q quit"}, model.width)
	}
	lines := []string{}
	for _, line := range body {
		lines = append(lines, wrapLine(safeInline(line), model.width)...)
	}
	return pagedView{header: header, body: lines, help: help, statusLabel: "Control lines", feedback: model.feedback, scroll: model.changeScroll, width: model.width, height: model.height}
}
