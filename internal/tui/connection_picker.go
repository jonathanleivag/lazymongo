package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
)

type connectionChosenMsg struct{ Conn config.Connection }
type connectionCreatedMsg struct{ Conn config.Connection }
type connectionCreateErrMsg struct{ Err error }
type connectionUpdatedMsg struct{ Conn config.Connection }
type connectionUpdateErrMsg struct{ Err error }
type connectionDeletedMsg struct{ Name string }
type connectionDeleteErrMsg struct{ Err error }

var colorChoices = []string{"amarillo", "rojo", "verde"}

type connectionForm struct {
	name   string
	uri    string
	color  string
	field  int  // 0=name, 1=uri, 2=color
	locked bool // when true, Name is read-only and Tab only cycles URI/Color
}

func newConnectionForm() connectionForm {
	return connectionForm{color: colorChoices[0]}
}

// newEditConnectionForm pre-fills the form with an existing connection's
// current values, locking the Name field (the array key can't be renamed
// through this form) and starting focus on URI.
func newEditConnectionForm(conn config.Connection) connectionForm {
	return connectionForm{name: conn.Name, uri: conn.URI, color: conn.Color, field: 1, locked: true}
}

func (f connectionForm) update(msg tea.KeyMsg) connectionForm {
	switch msg.String() {
	case "tab":
		if f.locked {
			if f.field == 1 {
				f.field = 2
			} else {
				f.field = 1
			}
		} else {
			f.field = (f.field + 1) % 3
		}
		return f
	case "shift+tab":
		if f.locked {
			if f.field == 1 {
				f.field = 2
			} else {
				f.field = 1
			}
		} else {
			f.field = (f.field + 2) % 3
		}
		return f
	}

	if f.field == 2 {
		switch msg.String() {
		case "l", "right":
			f.color = nextColor(f.color, 1)
		case "h", "left":
			f.color = nextColor(f.color, -1)
		}
		return f
	}

	switch msg.Type {
	case tea.KeyBackspace:
		if f.field == 0 {
			if f.locked {
				return f
			}
			if r := []rune(f.name); len(r) > 0 {
				f.name = string(r[:len(r)-1])
			}
		} else if f.field == 1 {
			if r := []rune(f.uri); len(r) > 0 {
				f.uri = string(r[:len(r)-1])
			}
		}
	case tea.KeyRunes:
		text := string(msg.Runes)
		if f.field == 0 {
			if f.locked {
				return f
			}
			f.name += text
		} else if f.field == 1 {
			f.uri += text
		}
	}
	return f
}

func nextColor(current string, delta int) string {
	idx := 0
	for i, c := range colorChoices {
		if c == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(colorChoices)) % len(colorChoices)
	return colorChoices[idx]
}

type connectionPickerModel struct {
	list             listModel
	conns            []config.Connection
	creating         bool
	editing          bool
	confirmingDelete bool
	confirm          confirmModel
	form             connectionForm
}

func newConnectionPickerModel(conns []config.Connection) connectionPickerModel {
	items := make([]listItem, len(conns))
	for i, c := range conns {
		items[i] = listItem{ID: c.Name, Label: c.Name, Color: c.Color}
	}
	return connectionPickerModel{list: newListModel("Conexiones", items, true), conns: conns}
}

// selectedConnection returns the full Connection (including URI) behind
// the currently highlighted list item, looked up by name so it's correct
// even while the list is fuzzy-filtered/reordered.
func (m connectionPickerModel) selectedConnection() (config.Connection, bool) {
	if len(m.list.Items) == 0 {
		return config.Connection{}, false
	}
	name := m.list.Items[m.list.Cursor].ID
	for _, c := range m.conns {
		if c.Name == name {
			return c, true
		}
	}
	return config.Connection{}, false
}

func (m connectionPickerModel) Update(msg tea.Msg) (connectionPickerModel, tea.Cmd) {
	if m.confirmingDelete {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if cmd == nil {
			return m, nil
		}
		result, ok := cmd().(confirmResultMsg)
		if !ok {
			return m, cmd
		}
		m.confirmingDelete = false
		if !result.Confirmed {
			return m, nil
		}
		conn, ok := m.selectedConnection()
		if !ok {
			return m, nil
		}
		name := conn.Name
		return m, func() tea.Msg {
			if err := config.DeleteConnection(name); err != nil {
				return connectionDeleteErrMsg{Err: err}
			}
			return connectionDeletedMsg{Name: name}
		}
	}

	if m.creating || m.editing {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		if keyMsg.String() == "esc" {
			m.creating = false
			m.editing = false
			return m, nil
		}
		if keyMsg.String() == "enter" {
			conn := config.Connection{Name: m.form.name, URI: m.form.uri, Color: m.form.color}
			if m.editing {
				return m, func() tea.Msg {
					if err := config.UpdateConnection(conn); err != nil {
						return connectionUpdateErrMsg{Err: err}
					}
					return connectionUpdatedMsg{Conn: conn}
				}
			}
			return m, func() tea.Msg {
				if err := config.AddConnection(conn); err != nil {
					return connectionCreateErrMsg{Err: err}
				}
				return connectionCreatedMsg{Conn: conn}
			}
		}
		m.form = m.form.update(keyMsg)
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && !m.list.Filtering() {
		switch keyMsg.String() {
		case "a":
			m.creating = true
			m.form = newConnectionForm()
			return m, nil
		case "e":
			if conn, ok := m.selectedConnection(); ok {
				m.editing = true
				m.form = newEditConnectionForm(conn)
			}
			return m, nil
		case "d", "x":
			if conn, ok := m.selectedConnection(); ok {
				m.confirmingDelete = true
				m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar la conexión %q?", conn.Name)}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if cmd == nil {
		return m, nil
	}
	if selected, ok := cmd().(itemSelectedMsg); ok {
		for _, item := range m.list.Items {
			if item.ID == selected.Item.ID {
				return m, func() tea.Msg {
					return connectionChosenMsg{Conn: config.Connection{Name: item.ID, Color: item.Color}}
				}
			}
		}
	}
	return m, cmd
}

func (m connectionPickerModel) View() string {
	if m.confirmingDelete {
		return m.confirm.View()
	}
	if m.creating || m.editing {
		title := "Nueva conexión"
		if m.editing {
			title = "Editar conexión"
		}
		var b strings.Builder
		b.WriteString(titleStyle.Render(title) + "\n\n")
		b.WriteString("Nombre: " + m.form.name)
		if m.form.field == 0 {
			b.WriteString(" <")
		}
		b.WriteString("\nURI:    " + m.form.uri)
		if m.form.field == 1 {
			b.WriteString(" <")
		}
		b.WriteString("\nColor:  " + colorStyle(m.form.color).Render(m.form.color))
		if m.form.field == 2 {
			b.WriteString(" <")
		}
		b.WriteString("\n\n[Tab] siguiente campo  [h/l] cambiar color  [Enter] guardar  [Esc] cancelar")
		return b.String()
	}
	return m.list.View()
}
