package terminal

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
)

type pagedView struct {
	palette     browserPalette
	header      []string
	body        []string
	help        []string
	statusLabel string
	feedback    string
	scroll      int
	width       int
	height      int
}

type pagedViewLayout struct {
	lines     []string
	pageSize  int
	maxScroll int
}

func (view pagedView) layout() pagedViewLayout {
	lastLine := max(1, len(view.body))
	footerCapacity := len(wrapLine(fmt.Sprintf(
		"%s %d-%d of %d",
		view.statusLabel,
		lastLine,
		lastLine,
		len(view.body),
	), view.width)) + len(view.help)
	if view.feedback != "" {
		footerCapacity++
	}
	pageSize := max(1, view.height-len(view.header)-footerCapacity)
	maxScroll := max(0, len(view.body)-pageSize)
	start := min(max(view.scroll, 0), maxScroll)
	end := min(len(view.body), start+pageSize)

	footer := make([]string, 0, footerCapacity)
	if view.feedback != "" {
		footer = append(footer, fitLine(view.feedback, view.width))
	}
	footer = append(footer, wrapLine(fmt.Sprintf(
		"%s %d-%d of %d",
		view.statusLabel,
		min(len(view.body), start+1),
		min(len(view.body), start+pageSize),
		len(view.body),
	), view.width)...)
	footer = append(footer, view.help...)

	lines := make([]string, 0, len(view.header)+len(view.body)+footerCapacity)
	for _, line := range view.header {
		lines = append(lines, view.palette.active(line))
	}
	lines = append(lines, view.body[start:end]...)
	for len(lines) < view.height-footerCapacity {
		lines = append(lines, "")
	}
	for len(footer) < footerCapacity {
		footer = append([]string{""}, footer...)
	}
	lines = append(lines, footer...)
	return pagedViewLayout{lines: lines, pageSize: pageSize, maxScroll: maxScroll}
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
