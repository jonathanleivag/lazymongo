package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmModel_YConfirms(t *testing.T) {
	m := confirmModel{Message: "¿Borrar documento?"}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command on 'y'")
	}
	result, ok := cmd().(confirmResultMsg)
	if !ok || !result.Confirmed {
		t.Fatalf("expected confirmResultMsg{Confirmed:true}, got %#v", cmd())
	}
}

func TestConfirmModel_NAndEscCancel(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("n")},
		{Type: tea.KeyEsc},
	} {
		m := confirmModel{Message: "¿Borrar documento?"}
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("expected a command on %v", key)
		}
		result, ok := cmd().(confirmResultMsg)
		if !ok || result.Confirmed {
			t.Fatalf("expected confirmResultMsg{Confirmed:false} for %v, got %#v", key, cmd())
		}
	}
}
