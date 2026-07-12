package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDeleteFlowModel_YSendsDeleteConfirmedMsg(t *testing.T) {
	m := newDeleteFlowModel("doc-1")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command on 'y'")
	}
	if _, ok := cmd().(deleteConfirmedMsg); !ok {
		t.Fatalf("expected deleteConfirmedMsg, got %T", cmd())
	}
}

func TestDeleteFlowModel_NSendsListBackMsg(t *testing.T) {
	m := newDeleteFlowModel("doc-1")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatal("expected a command on 'n'")
	}
	if _, ok := cmd().(listBackMsg); !ok {
		t.Fatalf("expected listBackMsg (cancel), got %T", cmd())
	}
}
