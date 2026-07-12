package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

type fieldUpdateConfirmedMsg struct {
	Field    string
	NewValue any
}

type fieldEditModel struct {
	field         string
	input         string
	originalValue any
	confirming    bool
	confirm       confirmModel
}

func newFieldEditModel(field string, currentValue any) fieldEditModel {
	return fieldEditModel{field: field, input: fmt.Sprintf("%v", currentValue), originalValue: currentValue}
}

// convertToOriginalType parses input (the raw text the user typed) back
// into the same Go type as original, so editing a field never silently
// coerces its BSON type (e.g. int32 -> string) on write. Types we don't have
// a clear text-conversion story for (nested documents, arrays, ObjectID,
// dates, nil, etc.) fall back to returning input as a plain string, which
// preserves today's existing behavior for those cases.
func convertToOriginalType(input string, original any) (any, error) {
	switch original.(type) {
	case int32:
		v, err := strconv.ParseInt(input, 10, 32)
		if err != nil {
			return nil, err
		}
		return int32(v), nil
	case int64:
		v, err := strconv.ParseInt(input, 10, 64)
		if err != nil {
			return nil, err
		}
		return v, nil
	case int:
		v, err := strconv.Atoi(input)
		if err != nil {
			return nil, err
		}
		return v, nil
	case float64:
		v, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return nil, err
		}
		return v, nil
	case float32:
		v, err := strconv.ParseFloat(input, 32)
		if err != nil {
			return nil, err
		}
		return float32(v), nil
	case bool:
		v, err := strconv.ParseBool(input)
		if err != nil {
			return nil, err
		}
		return v, nil
	case string:
		return input, nil
	default:
		return input, nil
	}
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
			field, input, original := m.field, m.input, m.originalValue
			return m, func() tea.Msg {
				value, err := convertToOriginalType(input, original)
				if err != nil {
					// input was already validated before entering the
					// confirming state, so this should not happen; fall
					// back to the raw string rather than losing the write.
					value = input
				}
				return fieldUpdateConfirmedMsg{Field: field, NewValue: value}
			}
		}
		return m, cmd
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.Type {
	case tea.KeyEnter:
		if _, err := convertToOriginalType(m.input, m.originalValue); err != nil {
			// invalid input (e.g. "abc" for a numeric field): ignore this
			// Enter press and let the user keep editing.
			return m, nil
		}
		m.confirming = true
		m.confirm = confirmModel{Message: fmt.Sprintf("¿Actualizar %q a %q?", m.field, m.input)}
	case tea.KeyBackspace:
		if r := []rune(m.input); len(r) > 0 {
			m.input = string(r[:len(r)-1])
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
