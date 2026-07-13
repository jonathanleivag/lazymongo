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
	filtering      bool
	filterQuery    string
	allIndexes     []mongo.IndexInfo
}

func newIdxListModel(indexes []mongo.IndexInfo) idxListModel {
	return idxListModel{indexes: indexes}
}

// Filtering reports whether this panel is in an active fuzzy-search
// text-entry state, so RootModel.inTextEntry can keep global shortcuts
// (like "?", "1"-"5", "Tab") from stealing keystrokes meant for the query.
func (m idxListModel) Filtering() bool { return m.filtering }

// FilterQuery returns the text typed so far into the active fuzzy search.
func (m idxListModel) FilterQuery() string { return m.filterQuery }

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
					if r := []rune(m.createKeys); len(r) > 0 {
						m.createKeys = string(r[:len(r)-1])
					}
				case tea.KeyRunes:
					m.createKeys += string(keyMsg.Runes)
				}
			}
		}
		return m, nil
	}

	if m.filtering {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.filtering = false
			m.filterQuery = ""
			m.indexes = m.allIndexes
			m.cursor = 0
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.indexes)-1 {
				m.cursor++
			}
		case tea.KeyBackspace:
			if r := []rune(m.filterQuery); len(r) > 0 {
				m.filterQuery = string(r[:len(r)-1])
			}
			m.applyFilter()
		case tea.KeyRunes:
			m.filterQuery += string(keyMsg.Runes)
			m.applyFilter()
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
	case "/":
		m.filtering = true
		m.filterQuery = ""
		m.allIndexes = m.indexes
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

// applyFilter recomputes indexes from allIndexes using the current
// filterQuery, matched against each index's Name, ordered by fuzzy match
// quality (best first), and resets cursor to the top result.
func (m *idxListModel) applyFilter() {
	labels := make([]string, len(m.allIndexes))
	for i, idx := range m.allIndexes {
		labels[i] = idx.Name
	}
	idxs := fuzzyMatchIndexes(m.filterQuery, labels)
	indexes := make([]mongo.IndexInfo, len(idxs))
	for i, ix := range idxs {
		indexes[i] = m.allIndexes[ix]
	}
	m.indexes = indexes
	m.cursor = 0
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
