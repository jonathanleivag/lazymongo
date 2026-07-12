package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestListModel_MoveCursorDownAndUp(t *testing.T) {
	m := newListModel("Test", []listItem{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"}}, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.Cursor != 1 {
		t.Fatalf("expected cursor 1 after one 'j', got %d", m.Cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // should clamp at last item
	if m.Cursor != 2 {
		t.Fatalf("expected cursor to clamp at 2, got %d", m.Cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.Cursor != 1 {
		t.Fatalf("expected cursor 1 after one 'k', got %d", m.Cursor)
	}
}

func TestListModel_EnterSelectsCurrentItem(t *testing.T) {
	m := newListModel("Test", []listItem{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, false)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command to be returned on enter")
	}
	msg := cmd()
	selected, ok := msg.(itemSelectedMsg)
	if !ok {
		t.Fatalf("expected itemSelectedMsg, got %T", msg)
	}
	if selected.Item.ID != "b" {
		t.Fatalf("expected item 'b' selected, got %q", selected.Item.ID)
	}
}

func TestListModel_EscSendsBackMsg(t *testing.T) {
	m := newListModel("Test", []listItem{{ID: "a", Label: "A"}}, false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command to be returned on esc")
	}
	if _, ok := cmd().(listBackMsg); !ok {
		t.Fatalf("expected listBackMsg, got %T", cmd())
	}
}
