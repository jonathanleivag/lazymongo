package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func newDatabaseListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Bases de datos", items, false)
}

func newCollectionListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Colecciones", items, false)
}

type dbCreateSubmittedMsg struct{ DBName, CollName string }
type dbDropConfirmedMsg struct{ Name string }

// dbListModel wraps a listModel to add create/delete support to the
// Databases panel — the same shape connectionPickerModel already uses to
// add create/edit/delete to Connections. The shared generic listModel
// itself is untouched.
type dbListModel struct {
	list           listModel
	creating       bool
	createDBName   string
	createCollName string
	createField    int // 0 = database name, 1 = initial collection name
	cursor         int // rune index within the active creation field
	confirmingDrop bool
	confirm        confirmModel
}

// newDbListModel builds a dbListModel from the given database names,
// reusing newDatabaseListModel for the underlying list construction.
func newDbListModel(names []string) dbListModel {
	return dbListModel{list: newDatabaseListModel(names)}
}

func (m dbListModel) activeFieldText() string {
	if m.createField == 0 {
		return m.createDBName
	}
	return m.createCollName
}

func (m *dbListModel) setActiveFieldText(newText string) {
	if m.createField == 0 {
		m.createDBName = newText
	} else {
		m.createCollName = newText
	}
}

func (m *dbListModel) insertAtCursor(text string) {
	runes := []rune(m.activeFieldText())
	inserted := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(inserted))
	newRunes = append(newRunes, runes[:m.cursor]...)
	newRunes = append(newRunes, inserted...)
	newRunes = append(newRunes, runes[m.cursor:]...)
	m.setActiveFieldText(string(newRunes))
	m.cursor += len(inserted)
}

func (m *dbListModel) deleteBeforeCursor() {
	if m.cursor == 0 {
		return
	}
	runes := []rune(m.activeFieldText())
	newRunes := make([]rune, 0, len(runes)-1)
	newRunes = append(newRunes, runes[:m.cursor-1]...)
	newRunes = append(newRunes, runes[m.cursor:]...)
	m.setActiveFieldText(string(newRunes))
	m.cursor--
}

func (m dbListModel) textBeforeCursor(field int) string {
	var text string
	if field == 0 {
		text = m.createDBName
	} else {
		text = m.createCollName
	}
	runes := []rune(text)
	if m.createField != field {
		return text
	}
	if m.cursor > len(runes) {
		return text
	}
	return string(runes[:m.cursor])
}

func (m dbListModel) textAfterCursor(field int) string {
	var text string
	if field == 0 {
		text = m.createDBName
	} else {
		text = m.createCollName
	}
	runes := []rune(text)
	if m.createField != field {
		return ""
	}
	if m.cursor > len(runes) {
		return ""
	}
	return string(runes[m.cursor:])
}

func (m dbListModel) Update(msg tea.Msg) (dbListModel, tea.Cmd) {
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
		if !result.Confirmed {
			return m, nil
		}
		if len(m.list.Items) == 0 {
			return m, nil
		}
		name := m.list.Items[m.list.Cursor].ID
		return m, func() tea.Msg { return dbDropConfirmedMsg{Name: name} }
	}

	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.creating = false
			return m, nil
		case "tab", "shift+tab":
			m.createField = (m.createField + 1) % 2
			m.cursor = len([]rune(m.activeFieldText()))
			return m, nil
		case "enter":
			dbName, collName := m.createDBName, m.createCollName
			return m, func() tea.Msg { return dbCreateSubmittedMsg{DBName: dbName, CollName: collName} }
		}
		switch keyMsg.Type {
		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyRight:
			if m.cursor < len([]rune(m.activeFieldText())) {
				m.cursor++
			}
		case tea.KeyBackspace:
			m.deleteBeforeCursor()
		case tea.KeyRunes:
			m.insertAtCursor(string(keyMsg.Runes))
		}
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && !m.list.Filtering() {
		switch keyMsg.String() {
		case "a":
			m.creating = true
			m.createDBName = ""
			m.createCollName = ""
			m.createField = 0
			m.cursor = 0
			return m, nil
		case "d", "x":
			if len(m.list.Items) > 0 {
				name := m.list.Items[m.list.Cursor].ID
				m.confirmingDrop = true
				m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar la database %q? Se perderán todas sus collections y documentos.", name)}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m dbListModel) View() string {
	if m.confirmingDrop {
		return m.confirm.View()
	}
	if m.creating {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Nueva database") + "\n\n")
		dbNameText := m.createDBName
		if m.createField == 0 {
			dbNameText = m.textBeforeCursor(0) + "_" + m.textAfterCursor(0)
		}
		b.WriteString("Database:   " + dbNameText)
		if m.createField == 0 {
			b.WriteString(" <")
		}
		collNameText := m.createCollName
		if m.createField == 1 {
			collNameText = m.textBeforeCursor(1) + "_" + m.textAfterCursor(1)
		}
		b.WriteString("\nCollection: " + collNameText)
		if m.createField == 1 {
			b.WriteString(" <")
		}
		b.WriteString("\n\n[Tab] siguiente campo  [Enter] crear  [Esc] cancelar")
		return b.String()
	}
	// Mirrors idxListModel/connectionPickerModel's normal-mode View(): this
	// branch is only reached if something calls dbListModel.View() outside
	// root.go's popup-gated paths, which nothing does today (the sidebar
	// panel renders via renderPanel + labelsFromListModel(m.list) instead)
	// — kept for shape-consistency with those two models, not because it's
	// currently reachable.
	return m.list.View() + "\n" + helpHintStyle.Render("[a] crear database  [d] borrar")
}

type collCreateSubmittedMsg struct{ Name string }
type collRenameSubmittedMsg struct{ OldName, NewName string }
type collDropConfirmedMsg struct{ Name string }

// collListModel wraps a listModel the same way dbListModel does, adding
// create/rename/delete support to the Collections panel. Renaming a
// collection needs both the name at the moment editing started
// (editOriginalName) and whatever the user has typed into editName since,
// exactly like connectionPickerModel's editingOriginalName distinguishes a
// rename from a same-name resubmit.
type collListModel struct {
	list             listModel
	creating         bool
	createName       string
	editing          bool
	editOriginalName string
	editName         string
	cursor           int // rune index within the active input field
	confirmingDrop   bool
	confirm          confirmModel
}

// newCollListModel builds a collListModel from the given collection
// names, reusing newCollectionListModel for the underlying list
// construction.
func newCollListModel(names []string) collListModel {
	return collListModel{list: newCollectionListModel(names)}
}

func (m collListModel) activeFieldText() string {
	if m.creating {
		return m.createName
	}
	return m.editName
}

func (m *collListModel) setActiveFieldText(newText string) {
	if m.creating {
		m.createName = newText
	} else {
		m.editName = newText
	}
}

func (m *collListModel) insertAtCursor(text string) {
	runes := []rune(m.activeFieldText())
	inserted := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(inserted))
	newRunes = append(newRunes, runes[:m.cursor]...)
	newRunes = append(newRunes, inserted...)
	newRunes = append(newRunes, runes[m.cursor:]...)
	m.setActiveFieldText(string(newRunes))
	m.cursor += len(inserted)
}

func (m *collListModel) deleteBeforeCursor() {
	if m.cursor == 0 {
		return
	}
	runes := []rune(m.activeFieldText())
	newRunes := make([]rune, 0, len(runes)-1)
	newRunes = append(newRunes, runes[:m.cursor-1]...)
	newRunes = append(newRunes, runes[m.cursor:]...)
	m.setActiveFieldText(string(newRunes))
	m.cursor--
}

func (m collListModel) textBeforeCursor() string {
	runes := []rune(m.activeFieldText())
	if m.cursor > len(runes) {
		return m.activeFieldText()
	}
	return string(runes[:m.cursor])
}

func (m collListModel) textAfterCursor() string {
	runes := []rune(m.activeFieldText())
	if m.cursor > len(runes) {
		return ""
	}
	return string(runes[m.cursor:])
}

func (m collListModel) Update(msg tea.Msg) (collListModel, tea.Cmd) {
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
		if !result.Confirmed {
			return m, nil
		}
		if len(m.list.Items) == 0 {
			return m, nil
		}
		name := m.list.Items[m.list.Cursor].ID
		return m, func() tea.Msg { return collDropConfirmedMsg{Name: name} }
	}

	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.creating = false
			return m, nil
		case "enter":
			name := m.createName
			return m, func() tea.Msg { return collCreateSubmittedMsg{Name: name} }
		}
		switch keyMsg.Type {
		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyRight:
			if m.cursor < len([]rune(m.activeFieldText())) {
				m.cursor++
			}
		case tea.KeyBackspace:
			m.deleteBeforeCursor()
		case tea.KeyRunes:
			m.insertAtCursor(string(keyMsg.Runes))
		}
		return m, nil
	}

	if m.editing {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.editing = false
			return m, nil
		case "enter":
			oldName, newName := m.editOriginalName, m.editName
			return m, func() tea.Msg { return collRenameSubmittedMsg{OldName: oldName, NewName: newName} }
		}
		switch keyMsg.Type {
		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyRight:
			if m.cursor < len([]rune(m.activeFieldText())) {
				m.cursor++
			}
		case tea.KeyBackspace:
			m.deleteBeforeCursor()
		case tea.KeyRunes:
			m.insertAtCursor(string(keyMsg.Runes))
		}
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && !m.list.Filtering() {
		switch keyMsg.String() {
		case "a":
			m.creating = true
			m.createName = ""
			m.cursor = 0
			return m, nil
		case "e":
			if len(m.list.Items) > 0 {
				name := m.list.Items[m.list.Cursor].ID
				m.editing = true
				m.editOriginalName = name
				m.editName = name
				m.cursor = len([]rune(name))
			}
			return m, nil
		case "d", "x":
			if len(m.list.Items) > 0 {
				name := m.list.Items[m.list.Cursor].ID
				m.confirmingDrop = true
				m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar la collection %q? Se perderán todos sus documentos.", name)}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m collListModel) View() string {
	if m.confirmingDrop {
		return m.confirm.View()
	}
	if m.creating {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Nueva collection") + "\n\n")
		b.WriteString("Nombre: " + m.textBeforeCursor() + "_" + m.textAfterCursor() + " <")
		b.WriteString("\n\n[Enter] crear  [Esc] cancelar")
		return b.String()
	}
	if m.editing {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Renombrar collection") + "\n\n")
		b.WriteString("Nombre: " + m.textBeforeCursor() + "_" + m.textAfterCursor() + " <")
		b.WriteString("\n\n[Enter] guardar  [Esc] cancelar")
		return b.String()
	}
	// Same shape-consistency note as dbListModel.View() above — unreachable
	// via root.go's current popup-gated dispatch, kept for consistency.
	return m.list.View() + "\n" + helpHintStyle.Render("[a] crear  [e] renombrar  [d] borrar")
}
