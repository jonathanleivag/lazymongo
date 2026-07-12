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
