package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
)

func TestConnectionPicker_SelectingItemSendsConnectionChosenMsg(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa", Color: "verde"},
		{Name: "prod", URI: "mongodb://prod", Color: "rojo"},
	}
	m := newConnectionPickerModel(conns)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter")
	}
	msg := cmd()
	chosen, ok := msg.(connectionChosenMsg)
	if !ok {
		t.Fatalf("expected connectionChosenMsg, got %T", msg)
	}
	if chosen.Conn.Name != "qa" {
		t.Fatalf("expected 'qa' chosen, got %q", chosen.Conn.Name)
	}
}

func TestConnectionPicker_PressingAOpensCreateForm(t *testing.T) {
	m := newConnectionPickerModel(nil)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected picker to enter 'creating' mode after pressing 'a'")
	}
}

func TestConnectionPicker_CreateFormSubmitsNewConnection(t *testing.T) {
	m := newConnectionPickerModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	// name field
	for _, r := range "movatec-dev" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// uri field
	for _, r := range "mongodb://x:27017/y" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// color field: cycle to "rojo" (starts at "amarillo")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create form")
	}
	if got := m.form.name; got != "movatec-dev" {
		t.Fatalf("expected name 'movatec-dev', got %q", got)
	}
	if got := m.form.color; got != "rojo" {
		t.Fatalf("expected color 'rojo' after one 'l', got %q", got)
	}
}
