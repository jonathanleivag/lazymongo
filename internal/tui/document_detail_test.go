package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDocDetailModel_EnterOnFieldSendsFieldSelectedMsg(t *testing.T) {
	m := newDocDetailModel(bson.M{"_id": "1", "age": int32(30), "name": "Ana"})

	// fields are sorted alphabetically: _id, age, name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // -> age

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd == nil {
		t.Fatal("expected a command on 'e'")
	}
	selected, ok := cmd().(fieldSelectedMsg)
	if !ok || selected.Field != "age" {
		t.Fatalf("expected fieldSelectedMsg{Field:\"age\"}, got %#v", cmd())
	}
}

func TestDocDetailModel_UppercaseESendsEditFullMsg(t *testing.T) {
	m := newDocDetailModel(bson.M{"_id": "1"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	if cmd == nil {
		t.Fatal("expected a command on 'E'")
	}
	if _, ok := cmd().(editFullRequestedMsg); !ok {
		t.Fatalf("expected editFullRequestedMsg, got %T", cmd())
	}
}

func TestDocDetailModel_CopySelectedFieldValue(t *testing.T) {
	id := bson.NewObjectID()
	m := newDocDetailModel(bson.M{"_id": id, "name": "Ana"})

	// Cursor is on _id (alphabetically sorted: _id, name)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command on 'y'")
	}
	copied, ok := cmd().(valueCopiedMsg)
	if !ok {
		t.Fatalf("expected valueCopiedMsg, got %T", cmd())
	}
	if copied.Text != id.Hex() {
		t.Fatalf("expected copied text to be id hex %q, got %q", id.Hex(), copied.Text)
	}

	// Move cursor to "name"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Press 'c' to copy
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("expected a command on 'c'")
	}
	copied, ok = cmd().(valueCopiedMsg)
	if !ok {
		t.Fatalf("expected valueCopiedMsg, got %T", cmd())
	}
	if copied.Text != "Ana" {
		t.Fatalf("expected copied text to be %q, got %q", "Ana", copied.Text)
	}
}

func TestDocDetailModel_CollapsibleTree(t *testing.T) {
	doc := bson.M{
		"_id": "1",
		"additionals": bson.A{
			bson.M{"product": "A"},
		},
	}
	m := newDocDetailModel(doc)

	// In initial state, nested arrays/objects are collapsed
	// Visible nodes: _id, additionals (collapsed)
	if len(m.visibleNodes) != 2 {
		t.Fatalf("expected 2 visible nodes initially, got %d", len(m.visibleNodes))
	}
	if m.visibleNodes[1].key != "additionals" || m.visibleNodes[1].isExpanded {
		t.Fatalf("expected additionals to be collapsed initially")
	}

	// Move cursor to additionals
	m.cursor = 1

	// Press Enter to expand additionals
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected no cmd on enter toggle")
	}

	// Now it should be expanded, showing the array item "0" (which is an Object, collapsed by default)
	// Visible nodes: _id, additionals (expanded), 0 (collapsed)
	if len(m.visibleNodes) != 3 {
		t.Fatalf("expected 3 visible nodes after expand, got %d: %+v", len(m.visibleNodes), m.visibleNodes)
	}
	if m.visibleNodes[2].key != "0" || m.visibleNodes[2].isExpanded {
		t.Fatalf("expected array item '0' to be collapsed initially")
	}

	// Move cursor to "0"
	m.cursor = 2

	// Press Enter to expand "0" (the nested object)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Now it should show "product" under "0"
	// Visible nodes: _id, additionals (expanded), 0 (expanded), product
	if len(m.visibleNodes) != 4 {
		t.Fatalf("expected 4 visible nodes, got %d", len(m.visibleNodes))
	}
	if m.visibleNodes[3].key != "product" || m.visibleNodes[3].fullPath() != "additionals.0.product" {
		t.Fatalf("expected path to be additionals.0.product, got %q", m.visibleNodes[3].fullPath())
	}
}
