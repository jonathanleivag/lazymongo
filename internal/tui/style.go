package tui

import "github.com/charmbracelet/lipgloss"

func colorStyle(color string) lipgloss.Style {
	switch color {
	case "amarillo":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	case "rojo":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	case "verde":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	default:
		return lipgloss.NewStyle()
	}
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true)

	// cursorStyle marks the selected row in every panel (applied by
	// panel.go's visibleWindow) with a cyan background, matching lazygit's
	// own selection highlighting — cyan is the same tone already used for
	// focusedBorderColor in panel.go.
	//
	// This only renders correctly today because every line that reaches
	// cursorStyle.Render(...) either has no embedded lipgloss styling at
	// all, or has its embedded styled segment (e.g. a connection's own
	// colorStyle(...).Render(...), or a BSON value from styleBSONValue(...)
	// in the expanded-document view) sitting at the very end of the line
	// with nothing plain trailing after it. Verified directly: when an
	// inner styled segment is followed by more plain text within the same
	// outer-wrapped line, the inner segment's own closing ANSI reset code
	// kills the outer background for everything after it — so if a future
	// change adds trailing plain text after an embedded styled segment on
	// a cursor-highlighted line, the background will visibly cut off
	// partway through. This is why, not a bug to "fix" here.
	cursorStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("6"))

	helpHintStyle = lipgloss.NewStyle().Faint(true)
)
