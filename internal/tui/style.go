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
	titleStyle    = lipgloss.NewStyle().Bold(true)
	cursorStyle   = lipgloss.NewStyle().Bold(true)
	helpHintStyle = lipgloss.NewStyle().Faint(true)
)
