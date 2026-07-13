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

func TestListModel_SlashEntersFilteringAndNarrowsItems(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "haddacloud-v2", Label: "haddacloud-v2"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.Filtering() {
		t.Fatal("expected filtering mode to be active after '/'")
	}
	for _, r := range "hdc" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.Items) != 1 || m.Items[0].ID != "haddacloud-v2" {
		t.Fatalf("expected only 'haddacloud-v2' to match 'hdc', got %+v", m.Items)
	}
	if m.Cursor != 0 {
		t.Fatalf("expected cursor reset to 0 on the best match, got %d", m.Cursor)
	}
}

func TestListModel_EscDuringFilterRestoresFullList(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adm")})
	if len(m.Items) != 1 {
		t.Fatalf("expected filter to narrow to 1 item, got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.Filtering() {
		t.Fatal("expected filtering to be false after Esc")
	}
	if len(m.Items) != 2 {
		t.Fatalf("expected full list of 2 items restored after Esc, got %d", len(m.Items))
	}
}

func TestListModel_BackspaceDuringFilterWidensResults(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adminx")})
	if len(m.Items) != 0 {
		t.Fatalf("expected no matches for 'adminx', got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.Items) != 1 || m.Items[0].ID != "admin" {
		t.Fatalf("expected backspace to widen back to 'admin' matching, got %+v", m.Items)
	}
}

func TestListModel_EnterDuringFilterSelectsHighlightedItem(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter while filtering")
	}
	selected, ok := cmd().(itemSelectedMsg)
	if !ok || selected.Item.ID != "test" {
		t.Fatalf("expected itemSelectedMsg{Item.ID:'test'}, got %#v", cmd())
	}
}

// TestListModel_EnterDuringFilterExitsFilteringMode is a regression test:
// pressing Enter used to leave filtering active, so the very next keystroke
// (e.g. a digit meant for a panel-jump shortcut) kept being swallowed as
// literal query text instead of reaching global shortcut handling.
func TestListModel_EnterDuringFilterExitsFilteringMode(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "haddacloud-v2", Label: "haddacloud-v2"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ha")})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Filtering() {
		t.Fatal("expected filtering to be false after Enter")
	}
	if len(m.Items) != 3 {
		t.Fatalf("expected the full 3-item list restored after Enter, got %+v", m.Items)
	}
	if m.Items[m.Cursor].ID != "haddacloud-v2" {
		t.Fatalf("expected cursor to point at the selected item 'haddacloud-v2' in the restored list, got %+v at cursor %d", m.Items, m.Cursor)
	}

	// The bug this guards against: with filtering still stuck active, "3"
	// would be swallowed into filterQuery instead of being free for
	// RootModel to treat as a panel-jump shortcut.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if m.FilterQuery() != "" {
		t.Fatalf("expected '3' typed after Enter to NOT be swallowed into the filter query, got %q", m.FilterQuery())
	}
}

func TestListModel_UpDownArrowsMoveCursorWithinFilterResults(t *testing.T) {
	items := []listItem{{ID: "test1", Label: "test1"}, {ID: "test2", Label: "test2"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
	if len(m.Items) != 2 {
		t.Fatalf("expected both items to match 'test', got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Cursor != 1 {
		t.Fatalf("expected Down to move cursor to 1, got %d", m.Cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.Cursor != 0 {
		t.Fatalf("expected Up to move cursor back to 0, got %d", m.Cursor)
	}
}

func TestListModel_NoMatchesThenBackspaceDoesNotPanic(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adminx")})
	if len(m.Items) != 0 {
		t.Fatalf("expected zero matches, got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.Items) != 1 {
		t.Fatalf("expected backspace to widen back to 1 match, got %d", len(m.Items))
	}
}
