package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type listItem struct {
	ID    string
	Label string
	Color string
}

type itemSelectedMsg struct{ Item listItem }
type listBackMsg struct{}

type listModel struct {
	Title     string
	Items     []listItem
	Cursor    int
	CanCreate bool
}

func newListModel(title string, items []listItem, canCreate bool) listModel {
	return listModel{Title: title, Items: items, CanCreate: canCreate}
}

func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.Cursor < len(m.Items)-1 {
			m.Cursor++
		}
	case "k", "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "enter":
		if len(m.Items) > 0 {
			item := m.Items[m.Cursor]
			return m, func() tea.Msg { return itemSelectedMsg{Item: item} }
		}
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

func (m listModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.Title) + "\n\n")
	if len(m.Items) == 0 {
		b.WriteString("(vacío)\n")
	}
	for i, item := range m.Items {
		prefix := "  "
		if i == m.Cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + colorStyle(item.Color).Render(item.Label) + "\n")
	}
	if m.CanCreate {
		b.WriteString("\n" + helpHintStyle.Render("[a] nueva conexión"))
	}
	return b.String()
}
