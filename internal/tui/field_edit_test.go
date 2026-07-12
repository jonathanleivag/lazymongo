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
	if !ok {
		t.Fatalf("expected fieldUpdateConfirmedMsg, got %#v", cmd())
	}
	if confirmed.Field != "age" {
		t.Fatalf("expected Field 'age', got %q", confirmed.Field)
	}
	newValue, ok := confirmed.NewValue.(int32)
	if !ok || newValue != int32(30) {
		t.Fatalf("expected NewValue to be int32(30) (preserving original BSON type), got %#v (%T)", confirmed.NewValue, confirmed.NewValue)
	}
}

// TestFieldEditModel_EditingInt32FieldWithValidInputProducesTypedInt32Value
// proves the core data-integrity fix: editing an int32 field with a new
// numeric string must produce a fieldUpdateConfirmedMsg.NewValue that is a
// Go int32, not the raw string "31" — otherwise MongoDB silently coerces the
// field's BSON type on write.
func TestFieldEditModel_EditingInt32FieldWithValidInputProducesTypedInt32Value(t *testing.T) {
	m := newFieldEditModel("age", int32(30))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil}) // clear "30" prefill
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil})
	for _, r := range "31" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.confirming {
		t.Fatal("expected confirming state after Enter with valid numeric input")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming")
	}
	confirmed, ok := cmd().(fieldUpdateConfirmedMsg)
	if !ok {
		t.Fatalf("expected fieldUpdateConfirmedMsg, got %#v", cmd())
	}
	newValue, ok := confirmed.NewValue.(int32)
	if !ok || newValue != int32(31) {
		t.Fatalf("expected NewValue to be int32(31), got %#v (%T)", confirmed.NewValue, confirmed.NewValue)
	}
}

// TestFieldEditModel_EditingStringFieldStillWorks proves string fields keep
// working exactly as before: NewValue stays a string.
func TestFieldEditModel_EditingStringFieldStillWorks(t *testing.T) {
	m := newFieldEditModel("name", "Ana")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.confirming {
		t.Fatal("expected confirming state after Enter")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming")
	}
	confirmed, ok := cmd().(fieldUpdateConfirmedMsg)
	if !ok {
		t.Fatalf("expected fieldUpdateConfirmedMsg, got %#v", cmd())
	}
	newValue, ok := confirmed.NewValue.(string)
	if !ok || newValue != "Ana!" {
		t.Fatalf("expected NewValue to be string \"Ana!\", got %#v (%T)", confirmed.NewValue, confirmed.NewValue)
	}
}

// TestFieldEditModel_InvalidNumericInputDoesNotEnterConfirming proves that
// typing non-numeric text for a numeric (originally int32) field and
// pressing Enter is simply ignored — the user stays in the raw-input state
// so they can keep editing, instead of transitioning to a broken confirm
// step.
func TestFieldEditModel_InvalidNumericInputDoesNotEnterConfirming(t *testing.T) {
	m := newFieldEditModel("age", int32(30))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil}) // clear "30" prefill
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil})
	for _, r := range "abc" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.confirming {
		t.Fatal("expected invalid numeric input to NOT enter confirming state")
	}
	if m.input != "abc" {
		t.Fatalf("expected input to remain 'abc' so user can keep editing, got %q", m.input)
	}
}
