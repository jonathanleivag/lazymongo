package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	focusedBorderColor   = lipgloss.Color("6") // cyan
	unfocusedBorderColor = lipgloss.Color("8") // gray
)

// renderPanel draws one bordered, numbered panel (e.g. "[2] Databases") with
// its items, highlighting the item at cursor. If there are more items than
// fit in height, the visible window scrolls to keep cursor in view. This is
// a pure function — no dependency on RootModel — so any of the 5 side
// panels or the main Documents panel can be rendered by calling this with
// their own title/items/cursor.
//
// Focus is indicated by BOTH border color AND a "▶ " text marker in the
// heading — not color alone. lipgloss only emits ANSI codes when it detects
// a real terminal; `go test` has no TTY, so `BorderForeground` (and any
// other styling) silently renders as plain text with zero visible
// difference in that environment (verified empirically while writing this
// plan). The text marker keeps focus state testable headlessly, and is a
// genuine accessibility win in real terminals too (works regardless of
// color support).
func renderPanel(number int, title string, items []string, cursor int, focused bool, width, height int) string {
	borderColor := unfocusedBorderColor
	if focused {
		borderColor = focusedBorderColor
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width).
		Height(height)

	marker := "  "
	if focused {
		marker = "▶ "
	}
	heading := titleStyle.Render(fmt.Sprintf("%s[%d] %s", marker, number, title))

	innerHeight := height - 1 // one line reserved for the heading
	if innerHeight < 1 {
		innerHeight = 1
	}

	visible := visibleWindow(items, cursor, innerHeight)

	var b strings.Builder
	b.WriteString(heading)
	for _, line := range visible {
		b.WriteString("\n")
		b.WriteString(line)
	}

	return box.Render(b.String())
}

// visibleWindow returns the slice of items that should be visible given a
// maximum of maxLines rows, scrolling so that the item at cursor is always
// included. Each returned line has the cursor marker/highlight applied.
func visibleWindow(items []string, cursor int, maxLines int) []string {
	if len(items) == 0 {
		return nil
	}

	start := 0
	if len(items) > maxLines {
		start = cursor - maxLines/2
		if start < 0 {
			start = 0
		}
		if start > len(items)-maxLines {
			start = len(items) - maxLines
		}
	}

	end := start + maxLines
	if end > len(items) {
		end = len(items)
	}

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		prefix := "  "
		line := items[i]
		if i == cursor {
			prefix = "> "
			line = cursorStyle.Render(line)
		}
		lines = append(lines, prefix+line)
	}
	return lines
}
