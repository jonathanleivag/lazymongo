package tui

import (
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
