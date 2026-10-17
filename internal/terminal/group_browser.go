package terminal

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/charmbracelet/x/ansi"
	"slices"
	"strings"
)

type groupFinished struct{ outcome collection.GroupOutcome }

func (model browserModel) openGroup(choice collection.ControlChoice) (tea.Model, tea.Cmd) {
	model.changeChoice = choice
	model.groupApplications = nil
	model.groupPreview = nil
	model.groupFocus = 0
	model.changeScroll = 0
	model.mode = browserGroupSelect
	model.feedback = ""
	return model, nil
}
func (model browserModel) updateGroup(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if model.mode == browserGroupSelect {
		catalog := collection.ApplicationCapabilities()
		switch k {
		case "esc":
			model.mode = browserSourceDetail
			model.groupPreview = nil
			model.feedback = "Cancelled. No settings changed."
		case "j", "down":
			model.groupFocus = min(model.groupFocus+1, len(catalog)-1)
		case "k", "up":
			model.groupFocus = max(model.groupFocus-1, 0)
		case "space", " ":
			app := catalog[model.groupFocus].Application
			if i := slices.Index(model.groupApplications, app); i >= 0 {
				model.groupApplications = slices.Delete(model.groupApplications, i, i+1)
			} else {
				model.groupApplications = append(model.groupApplications, app)
			}
			model.feedback = ""
		case "enter":
			if len(model.groupApplications) == 0 {
				model.feedback = "Select at least one application with Space."
			} else {
				model.mode = browserGroupScope
				model.changeScroll = 0
			}
		}
		return model, nil
	}
	if model.mode == browserGroupScope {
		switch k {
		case "esc":
			model.mode = browserGroupSelect
			model.groupPreview = nil
			model.changeScroll = 0
		case "g", "p", "i":
			scope := collection.ControlGlobal
			choice := model.changeChoice
			if k != "g" {
				scope = collection.ControlProject
			}
			if k == "i" {
				choice = collection.ControlInherit
			}
			model.groupPreview = model.snapshot.PreviewGroupChange(context.Background(), collection.GroupIntent{Source: model.selected, Applications: model.groupApplications, Scope: scope, Choice: choice})
			model.mode = browserGroupPreview
			model.changeScroll = 0
			model.feedback = ""
		}
		return model, nil
	}
	layout := model.groupPage().layout()
	switch k {
	case "esc", "enter":
		if model.mode == browserGroupResult {
			model.mode = browserSourceDetail
			model.feedback = "Group result: " + model.groupOutcome.State
		} else {
			model.mode = browserSourceDetail
			model.feedback = "Cancelled. No settings changed."
		}
		model.groupPreview = nil
	case "s":
		// Any reselection destroys the previous token, including a refused request.
		model.groupPreview = nil
		model.mode = browserGroupSelect
		model.changeScroll = 0
		model.feedback = "Edit selection, then request a fresh preview."
	case "y":
		if model.mode == browserGroupPreview && model.groupPreview != nil && model.groupPreview.View().State == "ready" {
			held := model.groupPreview
			token := held.View().Confirmation
			ctx, cancel := context.WithCancel(context.Background())
			model.groupCancel = cancel
			model.changing = true
			model.feedback = "Applying in order. Esc stops remaining applications."
			return model, func() tea.Msg { defer cancel(); return groupFinished{held.Apply(ctx, token)} }
		}
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
func (model browserModel) groupPage() pagedView {
	action := string(model.changeChoice)
	if model.mode == browserGroupPreview && model.groupPreview != nil {
		intent := model.groupPreview.View().Intent
		action = string(intent.Scope) + " " + string(intent.Choice)
	}
	if model.mode == browserGroupResult {
		intent := model.groupOutcome.View.Intent
		action = string(intent.Scope) + " " + string(intent.Choice)
	}
	header := []string{fitLine("SKILLGLASS | Group control", model.width), fitLine("Action: "+action, model.width)}
	if source, ok := model.snapshot.Source(model.selected); ok {
		header = append(header, fitLine("Selected: "+safeInline(source.Name), model.width), fitLine("Selector: "+string(source.ID), model.width))
	}
	body := []string{"Choose applications explicitly. All start unchecked.", "Space toggles; selection order is execution order."}
	help := []string{"↑↓ choose", "Space toggle", "Enter scope", "Esc cancel", "q quit"}
	if model.mode == browserGroupSelect {
		for i, c := range collection.ApplicationCapabilities() {
			mark := "[ ]"
			if n := slices.Index(model.groupApplications, c.Application); n >= 0 {
				mark = fmt.Sprintf("[%d]", n+1)
			}
			cursor := "  "
			if i == model.groupFocus {
				cursor = "› "
			}
			limit := ""
			if !c.Controls {
				limit = " (controls unsupported)"
			}
			body = append(body, cursor+mark+" "+c.Name+limit)
		}
		body = append(body, "Order: "+strings.Join(model.groupApplications, " -> "), "Exact source and bounded consumer evidence appear in the preview.", inspectionProjectLine(model.snapshot.Scope().Project))
	} else if model.mode == browserGroupScope {
		body = []string{"Select the group setting scope explicitly.", "Order: " + strings.Join(model.groupApplications, " -> "), "g Global configuration", "p Project configuration (unsupported members block)", "i Remove project override / inherit", inspectionProjectLine(model.snapshot.Scope().Project), "Esc edits application selection."}
		help = []string{"g Global", "p Project", "i Inherit", "Esc selection", "q quit"}
	} else {
		help = []string{"j/k scroll", "PgUp/PgDn", "g/G top/end", "s edit selection", "Enter/Esc Back", "q quit"}
		if model.mode == browserGroupPreview {
			view := model.groupPreview.View()
			body = groupPreviewLines(view)
			if view.State == "ready" {
				help = append([]string{"y Apply group", "Esc Cancel"}, help...)
			} else {
				body = append([]string{"Whole request unapplied. Press s to explicitly select a new subset."}, body...)
			}
		} else {
			body = groupOutcomeLines(model.groupOutcome)
		}
	}
	lines := []string{}
	for _, line := range body {
		if strings.Contains(line, "/") {
			lines = append(lines, wrapLine(safeInline(line), model.width)...)
			continue
		}
		for _, paragraph := range strings.Split(ansi.Wordwrap(safeInline(line), max(model.width, 1), ""), "\n") {
			lines = append(lines, wrapLine(paragraph, model.width)...)
		}
	}
	page := pagedView{palette: model.palette(), header: header, body: lines, help: wrapGroups(help, model.width), statusLabel: "Group lines", feedback: model.feedback, scroll: model.changeScroll, width: model.width, height: model.height}
	if model.mode == browserGroupSelect {
		for i, line := range lines {
			if strings.Contains(line, "› ") {
				page.scroll = max(0, i-page.layout().pageSize+1)
				break
			}
		}
	}
	return page
}
