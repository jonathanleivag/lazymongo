package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewDatabaseListModel_ListsGivenNames(t *testing.T) {
	m := newDatabaseListModel([]string{"admin", "haddacloud-v2"})
	if len(m.Items) != 2 || m.Items[0].Label != "admin" || m.Items[1].Label != "haddacloud-v2" {
		t.Fatalf("unexpected items: %+v", m.Items)
	}
}

func TestNewDbListModel_WrapsUnderlyingDatabaseList(t *testing.T) {
	m := newDbListModel([]string{"admin", "shop"})
	if len(m.list.Items) != 2 || m.list.Items[0].ID != "admin" || m.list.Items[1].ID != "shop" {
		t.Fatalf("unexpected items: %+v", m.list.Items)
	}
}

func TestDbListModel_AOpensCreateFormWithTwoFields(t *testing.T) {
	m := newDbListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected dbListModel to enter 'creating' mode after 'a'")
	}
	if m.createField != 0 {
		t.Fatalf("expected create form to start on field 0 (database name), got %d", m.createField)
	}
}

func TestDbListModel_TabSwitchesBetweenCreateFieldsAndEnterSubmits(t *testing.T) {
	m := newDbListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	for _, r := range "shop" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "orders" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create-database form")
	}
	submitted, ok := cmd().(dbCreateSubmittedMsg)
	if !ok || submitted.DBName != "shop" || submitted.CollName != "orders" {
		t.Fatalf("expected dbCreateSubmittedMsg{DBName:\"shop\",CollName:\"orders\"}, got %#v", cmd())
	}
}

func TestDbListModel_EscCancelsCreateForm(t *testing.T) {
	m := newDbListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.creating {
		t.Fatal("expected Esc to cancel the create-database form")
	}
}

func TestDbListModel_DOpensDropConfirmationAndYEmitsDbDropConfirmedMsg(t *testing.T) {
	m := newDbListModel([]string{"shop"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.confirmingDrop {
		t.Fatal("expected dbListModel to enter confirmingDrop state after 'd'")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming drop")
	}
	dropped, ok := cmd().(dbDropConfirmedMsg)
	if !ok || dropped.Name != "shop" {
		t.Fatalf("expected dbDropConfirmedMsg{Name:\"shop\"}, got %#v", cmd())
	}
}

func TestDbListModel_TypingADuringFilterDoesNotOpenCreateOrDropForms(t *testing.T) {
	m := newDbListModel([]string{"shop", "admin"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.list.Filtering() {
		t.Fatal("expected filtering mode to be active after '/'")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.creating {
		t.Fatal("expected 'a' typed while filtering to NOT open the create-database form")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.confirmingDrop {
		t.Fatal("expected 'd' typed while filtering to NOT open the drop confirmation")
	}
	if m.list.FilterQuery() != "ad" {
		t.Fatalf("expected 'a' and 'd' added to the filter query, got %q", m.list.FilterQuery())
	}
}

func TestNewCollListModel_WrapsUnderlyingCollectionList(t *testing.T) {
	m := newCollListModel([]string{"orders", "users"})
	if len(m.list.Items) != 2 || m.list.Items[0].ID != "orders" || m.list.Items[1].ID != "users" {
		t.Fatalf("unexpected items: %+v", m.list.Items)
	}
}

func TestCollListModel_AOpensCreateFormAndEnterSubmits(t *testing.T) {
	m := newCollListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected collListModel to enter 'creating' mode after 'a'")
	}

	for _, r := range "logs" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create-collection form")
	}
	submitted, ok := cmd().(collCreateSubmittedMsg)
	if !ok || submitted.Name != "logs" {
		t.Fatalf("expected collCreateSubmittedMsg{Name:\"logs\"}, got %#v", cmd())
	}
}

func TestCollListModel_EOpensRenameFormPrefilledAndEnterSubmits(t *testing.T) {
	m := newCollListModel([]string{"orders"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !m.editing {
		t.Fatal("expected collListModel to enter 'editing' mode after 'e'")
	}
	if m.editName != "orders" {
		t.Fatalf("expected rename form pre-filled with 'orders', got %q", m.editName)
	}

	for _, r := range "2" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the rename form")
	}
	submitted, ok := cmd().(collRenameSubmittedMsg)
	if !ok || submitted.OldName != "orders" || submitted.NewName != "orders2" {
		t.Fatalf("expected collRenameSubmittedMsg{OldName:\"orders\",NewName:\"orders2\"}, got %#v", cmd())
	}
}

func TestCollListModel_EscCancelsCreateAndRenameForms(t *testing.T) {
	m := newCollListModel([]string{"orders"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.creating {
		t.Fatal("expected Esc to cancel the create-collection form")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.editing {
		t.Fatal("expected Esc to cancel the rename-collection form")
	}
}

func TestCollListModel_DOpensDropConfirmationAndYEmitsCollDropConfirmedMsg(t *testing.T) {
	m := newCollListModel([]string{"orders"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.confirmingDrop {
		t.Fatal("expected collListModel to enter confirmingDrop state after 'd'")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming drop")
	}
	dropped, ok := cmd().(collDropConfirmedMsg)
	if !ok || dropped.Name != "orders" {
		t.Fatalf("expected collDropConfirmedMsg{Name:\"orders\"}, got %#v", cmd())
	}
}

func TestCollListModel_TypingAEDDuringFilterDoesNotOpenAnyForm(t *testing.T) {
	m := newCollListModel([]string{"orders", "users"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.list.Filtering() {
		t.Fatal("expected filtering mode to be active after '/'")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.creating {
		t.Fatal("expected 'a' typed while filtering to NOT open the create-collection form")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.editing {
		t.Fatal("expected 'e' typed while filtering to NOT open the rename form")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.confirmingDrop {
		t.Fatal("expected 'd' typed while filtering to NOT open the drop confirmation")
	}
	if m.list.FilterQuery() != "aed" {
		t.Fatalf("expected 'a','e','d' added to the filter query, got %q", m.list.FilterQuery())
	}
}
