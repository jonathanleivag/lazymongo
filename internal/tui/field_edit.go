package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type fieldUpdateConfirmedMsg struct {
	Field    string
	NewValue string
}

type fieldEditModel struct {
	field      string
	input      string
	confirming bool
	confirm    confirmModel
}

func newFieldEditModel(field string, currentValue any) fieldEditModel {
	return fieldEditModel{field: field, input: fmt.Sprintf("%v", currentValue)}
}

func (m fieldEditModel) Update(msg tea.Msg) (fieldEditModel, tea.Cmd) {
	if m.confirming {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if cmd == nil {
			return m, nil
		}
		if result, ok := cmd().(confirmResultMsg); ok {
			if !result.Confirmed {
				m.confirming = false
				return m, nil
			}
			field, value := m.field, m.input
			return m, func() tea.Msg { return fieldUpdateConfirmedMsg{Field: field, NewValue: value} }
		}
		return m, cmd
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.Type {
	case tea.KeyEnter:
		m.confirming = true
		m.confirm = confirmModel{Message: fmt.Sprintf("¿Actualizar %q a %q?", m.field, m.input)}
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	case tea.KeyEsc:
		return m, func() tea.Msg { return listBackMsg{} }
	case tea.KeyRunes:
		m.input += string(keyMsg.Runes)
	}
	return m, nil
}

func (m fieldEditModel) View() string {
	if m.confirming {
		return m.confirm.View()
	}
	return fmt.Sprintf("Editar %s: %s_\n\n[Enter] confirmar  [Esc] cancelar", m.field, m.input)
}
