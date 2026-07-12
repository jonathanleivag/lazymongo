package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFieldEditModel_TypeThenEnterShowsConfirmation(t *testing.T) {
	m := newFieldEditModel("age", int32(30))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil}) // clear "30" prefill
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil})
	for _, r := range "31" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.confirming {
		t.Fatal("expected fieldEditModel to enter confirming state after Enter")
	}
	if m.input != "31" {
		t.Fatalf("expected input '31', got %q", m.input)
	}
}

func TestFieldEditModel_ConfirmingYesSendsFieldUpdateConfirmedMsg(t *testing.T) {
	m := newFieldEditModel("age", int32(30))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // confirm with prefilled value

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming")
	}
	confirmed, ok := cmd().(fieldUpdateConfirmedMsg)
	if !ok || confirmed.Field != "age" || confirmed.NewValue != "30" {
		t.Fatalf("expected fieldUpdateConfirmedMsg{Field:\"age\",NewValue:\"30\"}, got %#v", cmd())
	}
}
