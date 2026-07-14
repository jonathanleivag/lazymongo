package tui

import (
	"strings"
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

func TestConnectionPicker_TypingADuringFilterDoesNotOpenCreateForm(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa", Color: "verde"},
		{Name: "staging", URI: "mongodb://staging", Color: "amarillo"},
	}
	m := newConnectionPickerModel(conns)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.list.Filtering() {
		t.Fatal("expected the underlying list to be filtering after '/'")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.creating {
		t.Fatal("expected 'a' typed while filtering to NOT open the create-connection form")
	}
	if m.list.FilterQuery() != "a" {
		t.Fatalf("expected 'a' to be added to the filter query, got %q", m.list.FilterQuery())
	}
}

func TestConnectionPicker_EPreFillsEditFormWithCurrentValues(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
	}
	m := newConnectionPickerModel(conns)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !m.editing {
		t.Fatal("expected picker to enter 'editing' mode after pressing 'e'")
	}
	if m.form.name != "qa" || m.form.uri != "mongodb://qa:27017" || m.form.color != "verde" {
		t.Fatalf("expected form pre-filled with current values, got %+v", m.form)
	}
	if m.form.field != 0 {
		t.Fatalf("expected edit form to start on the Name field (0), got %d", m.form.field)
	}
}

func TestConnectionPicker_EnterInEditModeReturnsACommand(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
	}
	m := newConnectionPickerModel(conns)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // move focus from Name to URI
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.form.uri != "mongodb://qa:270172" {
		t.Fatalf("expected the edited URI to accumulate, got %q", m.form.uri)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the edit form")
	}
}

func TestConnectionPicker_DOpensDeleteConfirmation(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
	}
	m := newConnectionPickerModel(conns)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.confirmingDelete {
		t.Fatal("expected picker to enter 'confirmingDelete' mode after pressing 'd'")
	}
}

func TestConnectionPicker_ConfirmingDeleteReturnsACommand(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
	}
	m := newConnectionPickerModel(conns)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming delete")
	}
}

func TestConnectionPicker_TypingEDDuringFilterDoesNotTriggerEditOrDelete(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
		{Name: "staging", URI: "mongodb://staging:27017", Color: "amarillo"},
	}
	m := newConnectionPickerModel(conns)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.list.Filtering() {
		t.Fatal("expected the underlying list to be filtering after '/'")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.editing {
		t.Fatal("expected 'e' typed while filtering to NOT open the edit form")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.confirmingDelete {
		t.Fatal("expected 'd' typed while filtering to NOT open the delete confirmation")
	}
	if m.list.FilterQuery() != "ed" {
		t.Fatalf("expected 'e' and 'd' to be added to the filter query, got %q", m.list.FilterQuery())
	}
}

func TestConnectionForm_TypingInsertsAtCursorNotAlwaysAtEnd(t *testing.T) {
	f := newConnectionForm()
	for _, r := range "ac" {
		f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyLeft})
	f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if f.name != "abc" {
		t.Fatalf("expected insertion at cursor to produce 'abc', got %q", f.name)
	}
}

func TestConnectionForm_BackspaceRemovesRuneBeforeCursor(t *testing.T) {
	f := newConnectionForm()
	for _, r := range "abc" {
		f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyLeft})
	f = f.update(tea.KeyMsg{Type: tea.KeyBackspace})
	if f.name != "ac" {
		t.Fatalf("expected backspace to remove 'b' (before cursor), got %q", f.name)
	}
}

func TestConnectionForm_ArrowsMoveAndClampCursorInNameField(t *testing.T) {
	f := newConnectionForm()
	for _, r := range "ab" {
		f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if f.cursor != 2 {
		t.Fatalf("expected cursor at end (2) after typing 'ab', got %d", f.cursor)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyLeft})
	f = f.update(tea.KeyMsg{Type: tea.KeyLeft})
	f = f.update(tea.KeyMsg{Type: tea.KeyLeft})
	if f.cursor != 0 {
		t.Fatalf("expected cursor clamped at 0, got %d", f.cursor)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyRight})
	f = f.update(tea.KeyMsg{Type: tea.KeyRight})
	f = f.update(tea.KeyMsg{Type: tea.KeyRight})
	if f.cursor != 2 {
		t.Fatalf("expected cursor clamped at 2 (end), got %d", f.cursor)
	}
}

func TestConnectionForm_TabMovesCursorToEndOfNewlyActiveField(t *testing.T) {
	f := newConnectionForm()
	for _, r := range "myname" {
		f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "mongodb://x" {
		f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if f.cursor != len([]rune("mongodb://x")) {
		t.Fatalf("expected cursor at end of URI (%d), got %d", len([]rune("mongodb://x")), f.cursor)
	}

	f = f.update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if f.cursor != len([]rune("myname")) {
		t.Fatalf("expected cursor at end of Name (%d) after tabbing back, got %d", len([]rune("myname")), f.cursor)
	}
}

func TestConnectionForm_NameFieldIsEditableInEditForm(t *testing.T) {
	f := newEditConnectionForm(config.Connection{Name: "qa", URI: "mongodb://qa", Color: "verde"})
	if f.field != 0 {
		t.Fatalf("expected edit form to start on field 0 (Name), got %d", f.field)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if f.name != "qax" {
		t.Fatalf("expected Name editable in edit form, got %q", f.name)
	}
}

func TestConnectionForm_TabInEditFormCyclesThroughAllThreeFields(t *testing.T) {
	f := newEditConnectionForm(config.Connection{Name: "qa", URI: "mongodb://qa", Color: "verde"})
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	if f.field != 1 {
		t.Fatalf("expected Tab to move from Name to URI (field 1), got %d", f.field)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	if f.field != 2 {
		t.Fatalf("expected Tab to move from URI to Color (field 2), got %d", f.field)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	if f.field != 0 {
		t.Fatalf("expected Tab to wrap from Color back to Name (field 0), got %d", f.field)
	}
}

func TestConnectionForm_ColorFieldCyclingUnaffectedByCursorChanges(t *testing.T) {
	f := newConnectionForm()
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if f.color != "rojo" {
		t.Fatalf("expected color to cycle to 'rojo' after one 'l', got %q", f.color)
	}
}

func TestConnectionForm_InsertAndBackspaceHandleMultiByteRunesSafely(t *testing.T) {
	f := newConnectionForm()
	for _, r := range "ó" {
		f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if f.name != "ó" {
		t.Fatalf("expected 'ó' inserted intact, got %q", f.name)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyBackspace})
	if f.name != "" {
		t.Fatalf("expected backspace to remove the whole rune 'ó', got %q", f.name)
	}
}

func TestConnectionPicker_ViewShowsCursorMarkerAtRealPositionInActiveField(t *testing.T) {
	m := newConnectionPickerModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	for _, r := range "ab" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})

	view := m.View()
	if !strings.Contains(view, "Nombre: a_b") {
		t.Fatalf("expected the cursor marker between 'a' and 'b', got:\n%s", view)
	}
}

func TestConnectionPicker_EStoresOriginalNameForRenameDetection(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
	}
	m := newConnectionPickerModel(conns)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.editingOriginalName != "qa" {
		t.Fatalf("expected editingOriginalName 'qa', got %q", m.editingOriginalName)
	}
}

func TestConnectionPicker_ChangingNameThenEnterStillReturnsACommand(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
	}
	m := newConnectionPickerModel(conns)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	// edit form now starts on the Name field; append to it directly
	for _, r := range "2" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.form.name != "qa2" {
		t.Fatalf("expected Name editable and accumulating, got %q", m.form.name)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the edit form after renaming")
	}
}
