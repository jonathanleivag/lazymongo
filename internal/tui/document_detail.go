package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fieldSelectedMsg struct {
	Field string
	Value any
}
type editFullRequestedMsg struct{}
type deleteRequestedMsg struct{}

type docDetailModel struct {
	doc    bson.M
	fields []string
	cursor int
}

func newDocDetailModel(doc bson.M) docDetailModel {
	fields := make([]string, 0, len(doc))
	for k := range doc {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return docDetailModel{doc: doc, fields: fields}
}

func (m docDetailModel) Update(msg tea.Msg) (docDetailModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.fields)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "e":
		if len(m.fields) > 0 {
			field := m.fields[m.cursor]
			value := m.doc[field]
			return m, func() tea.Msg { return fieldSelectedMsg{Field: field, Value: value} }
		}
	case "E":
		return m, func() tea.Msg { return editFullRequestedMsg{} }
	case "d", "x":
		return m, func() tea.Msg { return deleteRequestedMsg{} }
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

func (m docDetailModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Documento") + "\n\n")
	for i, field := range m.fields {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + fmt.Sprintf("%s: %v\n", field, m.doc[field]))
	}
	b.WriteString("\n" + helpHintStyle.Render("[e] editar campo  [E] editar completo  [d] borrar  [Esc] volver"))
	return b.String()
}
