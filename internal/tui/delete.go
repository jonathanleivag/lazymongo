package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type deleteConfirmedMsg struct{}

type deleteFlowModel struct {
	docID   any
	confirm confirmModel
}

func newDeleteFlowModel(docID any) deleteFlowModel {
	return deleteFlowModel{
		docID:   docID,
		confirm: confirmModel{Message: fmt.Sprintf("¿Borrar el documento %v? Esta acción no se puede deshacer", docID)},
	}
}

func (m deleteFlowModel) Update(msg tea.Msg) (deleteFlowModel, tea.Cmd) {
	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	if cmd == nil {
		return m, nil
	}
	result, ok := cmd().(confirmResultMsg)
	if !ok {
		return m, cmd
	}
	if result.Confirmed {
		return m, func() tea.Msg { return deleteConfirmedMsg{} }
	}
	return m, func() tea.Msg { return listBackMsg{} }
}

func (m deleteFlowModel) View() string {
	return m.confirm.View()
}
