package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type confirmResultMsg struct{ Confirmed bool }

type confirmModel struct {
	Message string
}

func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y":
		return m, func() tea.Msg { return confirmResultMsg{Confirmed: true} }
	case "n", "esc":
		return m, func() tea.Msg { return confirmResultMsg{Confirmed: false} }
	}
	return m, nil
}

func (m confirmModel) View() string {
	return fmt.Sprintf("%s (y/n)", m.Message)
}
