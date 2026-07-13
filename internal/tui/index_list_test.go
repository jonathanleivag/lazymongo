package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func sampleIndexes() []mongo.IndexInfo {
	return []mongo.IndexInfo{
		{Name: "_id_", Key: bson.M{"_id": 1}, Unique: true},
		{Name: "email_1", Key: bson.M{"email": 1}, Unique: true},
	}
}

func TestIdxListModel_DSendsIndexDropConfirmedFlow(t *testing.T) {
	m := newIdxListModel(sampleIndexes())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // -> email_1

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.confirmingDrop {
		t.Fatal("expected idxListModel to enter confirmingDrop state after 'd'")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming drop")
	}
	dropped, ok := cmd().(indexDropConfirmedMsg)
	if !ok || dropped.Name != "email_1" {
		t.Fatalf("expected indexDropConfirmedMsg{Name:\"email_1\"}, got %#v", cmd())
	}
}

func TestIdxListModel_AOpensCreateFormAndEnterSubmits(t *testing.T) {
	m := newIdxListModel(nil)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected idxListModel to enter creating state after 'a'")
	}

	for _, r := range `{"email":1}` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})                       // move to unique toggle
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // toggle unique on

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create-index form")
	}
	submitted, ok := cmd().(indexCreateSubmittedMsg)
	if !ok || submitted.KeysJSON != `{"email":1}` || !submitted.Unique {
		t.Fatalf("expected indexCreateSubmittedMsg{KeysJSON:'{\"email\":1}',Unique:true}, got %#v", cmd())
	}
}

func TestIdxListModel_SlashFiltersIndexesByName(t *testing.T) {
	m := newIdxListModel(sampleIndexes())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.Filtering() {
		t.Fatal("expected filtering mode to be active after '/'")
	}
	for _, r := range "email" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.indexes) != 1 || m.indexes[0].Name != "email_1" {
		t.Fatalf("expected only 'email_1' to match, got %+v", m.indexes)
	}
}

func TestIdxListModel_TypingADuringFilterDoesNotOpenCreateForm(t *testing.T) {
	m := newIdxListModel(sampleIndexes())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.creating {
		t.Fatal("expected 'a' typed while filtering to NOT open the create-index form")
	}
	if m.FilterQuery() != "a" {
		t.Fatalf("expected 'a' to be added to the filter query, got %q", m.FilterQuery())
	}
}

func TestIdxListModel_TypingDDuringFilterDoesNotOpenDropConfirm(t *testing.T) {
	m := newIdxListModel(sampleIndexes())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.confirmingDrop {
		t.Fatal("expected 'd' typed while filtering to NOT open the drop confirmation")
	}
}

func TestIdxListModel_EscDuringFilterRestoresFullIndexList(t *testing.T) {
	m := newIdxListModel(sampleIndexes())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("email")})
	if len(m.indexes) != 1 {
		t.Fatalf("expected filter to narrow to 1 index, got %d", len(m.indexes))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.Filtering() {
		t.Fatal("expected filtering false after Esc")
	}
	if len(m.indexes) != 2 {
		t.Fatalf("expected full index list of 2 restored after Esc, got %d", len(m.indexes))
	}
}

// TestIdxListModel_EnterDuringFilterExitsFilteringMode is a regression test:
// pressing Enter used to do nothing at all (no KeyEnter case existed in the
// filtering branch), leaving filtering stuck active so the next keystroke
// (e.g. a panel-jump digit) kept being swallowed as literal query text.
func TestIdxListModel_EnterDuringFilterExitsFilteringMode(t *testing.T) {
	m := newIdxListModel(sampleIndexes())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("email")})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Filtering() {
		t.Fatal("expected filtering to be false after Enter")
	}
	if len(m.indexes) != 2 {
		t.Fatalf("expected the full 2-index list restored after Enter, got %+v", m.indexes)
	}
	if m.indexes[m.cursor].Name != "email_1" {
		t.Fatalf("expected cursor to point at the selected index 'email_1' in the restored list, got %+v at cursor %d", m.indexes, m.cursor)
	}
}
