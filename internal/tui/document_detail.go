package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fieldSelectedMsg struct {
	Field string
	Value any
}
type editFullRequestedMsg struct{}
type deleteRequestedMsg struct{}
type valueCopiedMsg struct {
	Text string
}

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
	case "y", "c":
		if len(m.fields) > 0 {
			field := m.fields[m.cursor]
			value := m.doc[field]
			text := valueToCopyString(value)
			_ = copyToClipboard(text)
			return m, func() tea.Msg { return valueCopiedMsg{Text: text} }
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
	b.WriteString("\n" + helpHintStyle.Render("[e] editar campo  [y] copiar valor  [E] editar completo  [d] borrar  [Esc] volver"))
	return b.String()
}

func valueToCopyString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case bson.ObjectID:
		return val.Hex()
	case string:
		return val
	case int32, int64, int, float64, float32, bool:
		return fmt.Sprintf("%v", val)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		if bytes, err := bson.MarshalExtJSON(val, true, false); err == nil {
			return string(bytes)
		}
		return fmt.Sprintf("%v", val)
	}
}

func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default: // linux, freebsd, etc.
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip or xsel)")
		}
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := in.Write([]byte(text)); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}
