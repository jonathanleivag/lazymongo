package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
)

type indexCreateSubmittedMsg struct {
	KeysJSON string
	Unique   bool
}
type indexDropConfirmedMsg struct{ Name string }

type idxListModel struct {
	indexes        []mongo.IndexInfo
	cursor         int
	confirmingDrop bool
	confirm        confirmModel
	creating       bool
	createKeys     string
	createUnique   bool
	createField    int // 0=keys, 1=unique toggle
}

func newIdxListModel(indexes []mongo.IndexInfo) idxListModel {
	return idxListModel{indexes: indexes}
}

func (m idxListModel) Update(msg tea.Msg) (idxListModel, tea.Cmd) {
	if m.confirmingDrop {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if cmd == nil {
			return m, nil
		}
		result, ok := cmd().(confirmResultMsg)
		if !ok {
			return m, cmd
		}
		m.confirmingDrop = false
		if result.Confirmed {
			name := m.indexes[m.cursor].Name
			return m, func() tea.Msg { return indexDropConfirmedMsg{Name: name} }
		}
		return m, nil
	}

	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.creating = false
		case "tab":
			m.createField = (m.createField + 1) % 2
		case "enter":
			keys, unique := m.createKeys, m.createUnique
			return m, func() tea.Msg { return indexCreateSubmittedMsg{KeysJSON: keys, Unique: unique} }
		default:
			if m.createField == 1 && keyMsg.String() == " " {
				m.createUnique = !m.createUnique
			} else if m.createField == 0 {
				switch keyMsg.Type {
				case tea.KeyBackspace:
					if len(m.createKeys) > 0 {
						m.createKeys = m.createKeys[:len(m.createKeys)-1]
					}
				case tea.KeyRunes:
					m.createKeys += string(keyMsg.Runes)
				}
			}
		}
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.indexes)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "a":
		m.creating = true
		m.createKeys = ""
		m.createUnique = false
		m.createField = 0
	case "d":
		if len(m.indexes) > 0 {
			m.confirmingDrop = true
			m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar el índice %q?", m.indexes[m.cursor].Name)}
		}
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

func (m idxListModel) View() string {
	if m.confirmingDrop {
		return m.confirm.View()
	}
	if m.creating {
		unique := "no"
		if m.createUnique {
			unique = "sí"
		}
		return fmt.Sprintf(
			"Nuevo índice\n\nCampos (JSON, ej. {\"email\":1}): %s_\nUnique: %s\n\n[Tab] siguiente campo  [Espacio] alternar unique  [Enter] crear  [Esc] cancelar",
			m.createKeys, unique,
		)
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Índices") + "\n\n")
	if len(m.indexes) == 0 {
		b.WriteString("(sin índices además de _id_)\n")
	}
	for i, idx := range m.indexes {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		unique := ""
		if idx.Unique {
			unique = " (unique)"
		}
		b.WriteString(fmt.Sprintf("%s%s %v%s\n", prefix, idx.Name, idx.Key, unique))
	}
	b.WriteString("\n" + helpHintStyle.Render("[a] crear índice  [d] borrar  [Esc] volver"))
	return b.String()
}
