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
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	// A box with no width constraint auto-sizes to its widest line, which
	// is fine for short content — but a long single-token value with
	// nowhere to break (e.g. a multi-host Mongo URI) grows the box past the
	// terminal's actual width, spilling content past its right edge
	// instead of wrapping. Only constraining Width when content is already
	// wider than what fits keeps every existing, already-short popup
	// (confirmations, help, etc.) rendering exactly as before — this only
	// kicks in for content that would otherwise overflow.
	maxContentWidth := width - 8
	if maxContentWidth > 80 {
		maxContentWidth = 80
	}
	if maxContentWidth < 20 {
		maxContentWidth = 20
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusedBorderColor).
		Padding(1, 2)
	if lipgloss.Width(content) > maxContentWidth {
		boxStyle = boxStyle.Width(maxContentWidth)
	}
	box := boxStyle.Render(content)

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
