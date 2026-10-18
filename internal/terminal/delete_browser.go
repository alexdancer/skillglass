package terminal

import (
	"context"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/charmbracelet/x/ansi"
)

type deleteFinished struct{ outcome collection.DeleteOutcome }

func (model browserModel) openDelete() (tea.Model, tea.Cmd) {
	model.deletePreview = model.snapshot.PreviewDelete(context.Background(), model.selected)
	model.deleteInput = ""
	model.changeScroll = 0
	model.mode = browserDeletePreview
	model.feedback = ""
	return model, nil
}
func (model browserModel) updateDelete(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if k == "esc" || k == "q" && model.mode != browserDeleteConfirm {
		model.deletePreview = nil
		model.deleteInput = ""
		model.feedback = "Cancelled. No further content changed."
		if model.mode == browserDeleteResult {
			model.feedback = "Delete result: " + model.deleteOutcome.State
			model.mode = browserSources
			if k == "q" {
				return model, tea.Quit
			}
			return model, nil
		}
		model.mode = browserSourceDetail
		if k == "q" {
			return model, tea.Quit
		}
		return model, nil
	}
	if k == "r" && model.mode != browserDeleteConfirm {
		model.deletePreview = nil
		model.deleteInput = ""
		model.mode = browserSources
		model.refresh()
		return model, nil
	}
	if model.mode == browserDeleteConfirm {
		switch k {
		case "enter":
			view := model.deletePreview.View()
			if model.deleteInput != view.Phrase {
				model.feedback = "Confirmation does not match. No content changed."
				return model, nil
			}
			held := model.deletePreview
			ctx, cancel := context.WithCancel(context.Background())
			model.deleteCancel = cancel
			model.changing = true
			model.feedback = "Removing selected content. Esc stops remaining operations."
			return model, func() tea.Msg { defer cancel(); return deleteFinished{held.Apply(ctx, view.Confirmation)} }
		case "backspace":
			if len(model.deleteInput) > 0 {
				_, size := utf8.DecodeLastRuneInString(model.deleteInput)
				model.deleteInput = model.deleteInput[:len(model.deleteInput)-size]
			}
		case "ctrl+u":
			model.deleteInput = ""
		case "up", "down", "pgup", "pgdown":
			return model.scrollDelete(k)
		default:
			if key.Text != "" && len(model.deleteInput) < 256 {
				model.deleteInput += key.Text
			}
		}
		return model, nil
	}
	if k == "enter" {
		if model.mode == browserDeletePreview && model.deletePreview.View().State == "ready" {
			model.mode = browserDeleteConfirm
			model.deleteInput = ""
			model.feedback = "Type the complete target-specific phrase, then Enter."
			return model, nil
		}
		model.deletePreview = nil
		model.mode = browserSources
		return model, nil
	}
	return model.scrollDelete(k)
}
func (model browserModel) scrollDelete(k string) (tea.Model, tea.Cmd) {
	layout := model.deletePage().layout()
	switch k {
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
func (model browserModel) deletePage() pagedView {
	body := []string{}
	help := []string{"↑↓ scroll", "PgUp/PgDn", "Enter confirmation", "Esc cancel", "q quit"}
	if model.mode == browserDeleteResult {
		body = deleteOutcomeLines(model.deleteOutcome)
		help = []string{"↑↓ scroll", "PgUp/PgDn", "Enter/Esc Back", "q quit"}
	} else {
		v := model.deletePreview.View()
		body = deletePreviewLines(v)
		if v.State != "ready" {
			help = []string{"↑↓ scroll", "PgUp/PgDn", "Enter back", "Esc cancel", "q quit"}
		}
		if model.mode == browserDeleteConfirm && (ansi.StringWidth("Type: "+v.Phrase) > model.width || ansi.StringWidth("Input: "+model.deleteInput) > model.width) {
			body = append([]string{"Type: " + v.Phrase, "Input: " + model.deleteInput}, body...)
		}
		// CLI digests are not browser input. The held preview is authorized only by its exact typed phrase.
		for i, line := range body {
			if strings.HasPrefix(line, "Confirm with --confirm ") {
				body[i] = "Type confirmation on the next screen: " + v.Phrase
			}
		}
		if model.mode == browserDeleteConfirm {
			help = []string{"↑↓ scroll", "PgUp/PgDn", "Enter delete", "Esc cancel"}
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
	header := []string{fitLine("SKILLGLASS | PERMANENT DELETE", model.width)}
	if model.mode == browserDeleteConfirm {
		header = append(header, fitLine("Type: "+model.deletePreview.View().Phrase, model.width))
		header = append(header, fitLine("Input: "+safeInline(model.deleteInput), model.width))
	}
	return pagedView{palette: model.palette(), header: header, body: lines, help: wrapGroups(help, model.width), statusLabel: "Delete lines", feedback: model.feedback, scroll: model.changeScroll, width: model.width, height: model.height}
}
