package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func sampleDocs() []bson.M {
	return []bson.M{
		{"_id": "1", "name": "Ana"},
		{"_id": "2", "name": "Beto"},
	}
}

func TestDocListModel_EnterOnRowSendsDocumentChosenMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter")
	}
	chosen, ok := cmd().(documentChosenMsg)
	if !ok {
		t.Fatalf("expected documentChosenMsg, got %T", cmd())
	}
	if chosen.Doc["_id"] != "2" {
		t.Fatalf("expected doc '2' chosen, got %+v", chosen.Doc)
	}
}

func TestDocListModel_SlashOpensFilterAndTypingUpdatesFilterText(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.filtering {
		t.Fatal("expected filtering mode to be active after '/'")
	}
	for _, r := range `{"name":"Ana"}` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.FilterText() != `{"name":"Ana"}` {
		t.Fatalf("expected filter text to accumulate, got %q", m.FilterText())
	}
}

func TestDocListModel_NextPageSendsPageChangedMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 50, 0, 20)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatal("expected a command on 'n'")
	}
	changed, ok := cmd().(pageChangedMsg)
	if !ok || changed.Page != 1 {
		t.Fatalf("expected pageChangedMsg{Page:1}, got %#v", cmd())
	}
}

func TestDocListModel_ISendsInsertRequestedMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if cmd == nil {
		t.Fatal("expected a command on 'i'")
	}
	if _, ok := cmd().(insertRequestedMsg); !ok {
		t.Fatalf("expected insertRequestedMsg, got %T", cmd())
	}
}

func TestDocListModel_TabSendsSwitchToIndexesMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("expected a command on Tab")
	}
	if _, ok := cmd().(switchToIndexesMsg); !ok {
		t.Fatalf("expected switchToIndexesMsg, got %T", cmd())
	}
}

func TestDocListModel_CtrlFOpensLocalFuzzyAndNarrowsRowsByID(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20) // _id "1"/"Ana", _id "2"/"Beto"

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if !m.FuzzyFiltering() {
		t.Fatal("expected local fuzzy-find to be active after Ctrl+f")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if len(m.docs) != 1 || fmt.Sprintf("%v", m.docs[0]["_id"]) != "2" {
		t.Fatalf("expected only doc '2' to match, got %+v", m.docs)
	}
}

func TestDocListModel_EscDuringLocalFuzzyRestoresFullDocs(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if len(m.docs) != 1 {
		t.Fatalf("expected narrowed to 1 doc, got %d", len(m.docs))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.FuzzyFiltering() {
		t.Fatal("expected local fuzzy-find inactive after Esc")
	}
	if len(m.docs) != 2 {
		t.Fatalf("expected full 2-doc set restored after Esc, got %d", len(m.docs))
	}
}

func TestDocListModel_LocalFuzzyDoesNotTouchMongoFilterState(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if m.filtering || m.filter != "" {
		t.Fatalf("expected the Mongo query filter state untouched by local fuzzy-find, got filtering=%v filter=%q", m.filtering, m.filter)
	}
}

func TestDocListModel_EnterDuringLocalFuzzySendsDocumentChosenMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter during local fuzzy-find")
	}
	chosen, ok := cmd().(documentChosenMsg)
	if !ok || chosen.Doc["_id"] != "2" {
		t.Fatalf("expected documentChosenMsg for doc '2', got %#v", cmd())
	}
}

// TestDocListModel_EnterDuringLocalFuzzyExitsFuzzyFilteringMode is a
// regression test: pressing Enter used to leave fuzzy-find active, so once
// the opened document's popup closed, the next keystroke (e.g. a panel-jump
// digit) kept being swallowed as literal query text instead of reaching
// global shortcut handling.
func TestDocListModel_EnterDuringLocalFuzzyExitsFuzzyFilteringMode(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.FuzzyFiltering() {
		t.Fatal("expected fuzzy-filtering to be false after Enter")
	}
	if len(m.docs) != 2 {
		t.Fatalf("expected the full 2-doc set restored after Enter, got %+v", m.docs)
	}
	if m.docs[m.cursor]["_id"] != "2" {
		t.Fatalf("expected cursor to point at the selected doc '2' in the restored set, got %+v at cursor %d", m.docs, m.cursor)
	}
}
