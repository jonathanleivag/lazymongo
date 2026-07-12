package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderPopupOverlay centers content in a bordered box on a blank canvas of
// exactly width x height. See this plan's Global Constraints for why this is
// a blank-canvas popup rather than true compositing over the panels behind
// it.
func renderPopupOverlay(content string, width, height int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusedBorderColor).
		Padding(1, 2).
		Render(content)

	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// composeScreen joins the 5 side panels into a left column, places the main
// panel and command log to its right, and appends the keybinding footer.
// logWidth/logHeight size the command log box; the sidebar/main panels are
// expected to already be pre-rendered (via renderPanel) at their own sizes.
func composeScreen(sidebar []string, main string, log []string, footer string, logWidth, logHeight int) string {
	left := lipgloss.JoinVertical(lipgloss.Left, sidebar...)

	logBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(unfocusedBorderColor).
		Width(logWidth).
		Height(logHeight).
		Render(strings.Join(log, "\n"))

	right := lipgloss.JoinVertical(lipgloss.Left, main, logBox)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}
