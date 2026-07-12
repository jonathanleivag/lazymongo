package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
)

type connectionChosenMsg struct{ Conn config.Connection }
type connectionCreatedMsg struct{ Conn config.Connection }
type connectionCreateErrMsg struct{ Err error }

var colorChoices = []string{"amarillo", "rojo", "verde"}

type connectionForm struct {
	name  string
	uri   string
	color string
	field int // 0=name, 1=uri, 2=color
}

func newConnectionForm() connectionForm {
	return connectionForm{color: colorChoices[0]}
}

func (f connectionForm) update(msg tea.KeyMsg) connectionForm {
	switch msg.String() {
	case "tab":
		f.field = (f.field + 1) % 3
		return f
	case "shift+tab":
		f.field = (f.field + 2) % 3
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
	list     listModel
	creating bool
	form     connectionForm
}

func newConnectionPickerModel(conns []config.Connection) connectionPickerModel {
	items := make([]listItem, len(conns))
	for i, c := range conns {
		items[i] = listItem{ID: c.Name, Label: c.Name, Color: c.Color}
	}
	return connectionPickerModel{list: newListModel("Conexiones", items, true)}
}

func (m connectionPickerModel) Update(msg tea.Msg) (connectionPickerModel, tea.Cmd) {
	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		if keyMsg.String() == "esc" {
			m.creating = false
			return m, nil
		}
		if keyMsg.String() == "enter" {
			conn := config.Connection{Name: m.form.name, URI: m.form.uri, Color: m.form.color}
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

	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "a" {
		m.creating = true
		m.form = newConnectionForm()
		return m, nil
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
	if m.creating {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Nueva conexión") + "\n\n")
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
