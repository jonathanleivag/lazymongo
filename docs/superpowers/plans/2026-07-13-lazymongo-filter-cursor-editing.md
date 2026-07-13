# lazymongo — Cursor-based filter editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real cursor movement (`←`/`→`), smart auto-close/skip-over for `{`/`"`, and Esc-clears-an-applied-filter to the Mongo query filter field in `[0]` Documentos — scoped to that one field only.

**Architecture:** `docListModel` gains a `filterCursor int` (rune index) alongside the existing `filter string`. All edits (insert, backspace, arrows) operate through small helper methods on `docListModel` rather than the old append-only string concatenation. The already-shipped field-name autocomplete is adjusted to work relative to the cursor instead of the string's end. A new message, `filterClearedMsg`, lets `Esc` clear an already-applied filter. `RootModel`'s rendering moves the cursor-blink marker to the real cursor position.

**Tech Stack:** Go, standard library only (no new dependencies).

## Global Constraints

- Scoped to the Mongo filter field only — no other text-entry field in the app (connection form, index create form, fuzzy search boxes) changes.
- `filterCursor` is a **rune** index, not a byte index — every edit operation must go through `[]rune(m.filter)`, never byte-slice the string directly, to avoid splitting multi-byte UTF-8 characters.
- Auto-close/skip-over/empty-pair-backspace are purely positional — they never parse the JSON and never track which characters were auto-inserted vs. typed by hand. Skip-over: typing `}`/`"` when that exact character is already immediately to the right of the cursor moves the cursor past it instead of inserting a duplicate. Empty-pair backspace: Backspace when the character before the cursor is `{`/`"` and the character after it is the matching closer removes both at once.
- `docListModel.View()` (the method, not `RootModel.View()`) is dead code — not reached by the panel-grid rendering path (confirmed in the prior expanded-document-view and fuzzy-search plans). It is intentionally left exactly as-is throughout this plan; do not "fix" it to match the new cursor behavior.
- `Esc`/`h` with no applied filter keep their current, unchanged behavior (`listBackMsg{}`). Only `Esc` (not `h`) gains the new "clear an applied filter" behavior, and only when `m.docList.filter != ""` and filtering is not currently active.

---

### Task 1: Cursor-based text model with auto-close and skip-over

**Files:**
- Modify: `internal/tui/document_list.go` (full-file replacement — many small, non-contiguous changes)
- Test: `internal/tui/document_list_test.go`

**Interfaces:**
- Produces: `func (m docListModel) FilterCursor() int`. Later tasks and their tests use this to assert cursor position.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/document_list_test.go`:

```go
func TestDocListModel_TypingInsertsAtCursorNotAlwaysAtEnd(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "ac" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if m.FilterText() != "abc" {
		t.Fatalf("expected insertion at cursor to produce 'abc', got %q", m.FilterText())
	}
}

func TestDocListModel_BackspaceRemovesRuneBeforeCursor(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "abc" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.FilterText() != "ac" {
		t.Fatalf("expected backspace to remove 'b' (before cursor), got %q", m.FilterText())
	}
}

func TestDocListModel_ArrowsMoveAndClampCursor(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "ab" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.FilterCursor() != 2 {
		t.Fatalf("expected cursor at end (2) after typing 'ab', got %d", m.FilterCursor())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.FilterCursor() != 0 {
		t.Fatalf("expected cursor clamped at 0, got %d", m.FilterCursor())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.FilterCursor() != 2 {
		t.Fatalf("expected cursor clamped at 2 (end), got %d", m.FilterCursor())
	}
}

func TestDocListModel_TypingOpenBraceAutoClosesWithCursorBetween(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	if m.FilterText() != "{}" {
		t.Fatalf("expected '{' to auto-close as '{}', got %q", m.FilterText())
	}
	if m.FilterCursor() != 1 {
		t.Fatalf("expected cursor between the braces (1), got %d", m.FilterCursor())
	}
}

func TestDocListModel_TypingQuoteAutoClosesWithCursorBetween(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(`"`)})
	if m.FilterText() != `""` {
		t.Fatalf(`expected '"' to auto-close as '""', got %q`, m.FilterText())
	}
	if m.FilterCursor() != 1 {
		t.Fatalf("expected cursor between the quotes (1), got %d", m.FilterCursor())
	}
}

func TestDocListModel_TypingClosingBraceSkipsOverExistingOne(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("}")})
	if m.FilterText() != "{}" {
		t.Fatalf("expected typing '}' to skip over the existing one, not duplicate, got %q", m.FilterText())
	}
	if m.FilterCursor() != 2 {
		t.Fatalf("expected cursor to land after the closing brace (2), got %d", m.FilterCursor())
	}
}

func TestDocListModel_TypingClosingQuoteSkipsOverExistingOne(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(`"`)})
	for _, r := range "Ana" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(`"`)})
	if m.FilterText() != `"Ana"` {
		t.Fatalf(`expected typing '"' to skip over the existing closing quote, got %q`, m.FilterText())
	}
	if m.FilterCursor() != 5 {
		t.Fatalf("expected cursor to land after the closing quote (5), got %d", m.FilterCursor())
	}
}

func TestDocListModel_BackspaceOnEmptyPairRemovesBoth(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.FilterText() != "" {
		t.Fatalf("expected backspace on an empty pair to remove both characters, got %q", m.FilterText())
	}
	if m.FilterCursor() != 0 {
		t.Fatalf("expected cursor back at 0, got %d", m.FilterCursor())
	}
}

func TestDocListModel_BackspaceOnNonEmptyPairRemovesOnlyOneChar(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.FilterText() != "{}" {
		t.Fatalf("expected backspace to remove only 'a' (pair is not empty), got %q", m.FilterText())
	}
}

func TestDocListModel_InsertAndBackspaceHandleMultiByteRunesSafely(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "ó" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.FilterText() != "ó" {
		t.Fatalf("expected 'ó' inserted intact, got %q", m.FilterText())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.FilterText() != "" {
		t.Fatalf("expected backspace to remove the whole rune 'ó', got %q", m.FilterText())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/tui/... -v -run "TestDocListModel_TypingInsertsAtCursor|TestDocListModel_BackspaceRemovesRuneBeforeCursor|TestDocListModel_ArrowsMoveAndClampCursor|TestDocListModel_TypingOpenBrace|TestDocListModel_TypingQuote|TestDocListModel_TypingClosing|TestDocListModel_BackspaceOnEmptyPair|TestDocListModel_BackspaceOnNonEmptyPair|TestDocListModel_InsertAndBackspaceHandleMultiByteRunesSafely"`
Expected: FAIL — `m.FilterCursor` undefined, `tea.KeyLeft`/`tea.KeyRight` unhandled, auto-close not implemented

- [ ] **Step 3: Replace document_list.go**

Replace the full contents of `internal/tui/document_list.go`:

```go
package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type documentChosenMsg struct{ Doc bson.M }
type pageChangedMsg struct{ Page int64 }
type filterSubmittedMsg struct{ Filter string }
type insertRequestedMsg struct{}
type switchToIndexesMsg struct{}

type docListModel struct {
	docs         []bson.M
	total        int64
	page         int64
	pageSize     int64
	cursor       int
	filtering    bool
	filter       string
	filterCursor int

	fuzzyFiltering bool
	fuzzyQuery     string
	allDocs        []bson.M
}

func newDocListModel(docs []bson.M, total, page, pageSize int64) docListModel {
	return docListModel{docs: docs, total: total, page: page, pageSize: pageSize}
}

func (m docListModel) FilterText() string { return m.filter }

// FilterCursor returns the current rune-index cursor position within the
// Mongo filter text.
func (m docListModel) FilterCursor() int { return m.filterCursor }

// FuzzyFiltering reports whether the local (non-Mongo) fuzzy-find over
// already-loaded rows is active, so RootModel.inTextEntry can keep global
// shortcuts (like "?", "1"-"5", "Tab") from stealing keystrokes meant for
// the search query.
func (m docListModel) FuzzyFiltering() bool { return m.fuzzyFiltering }

// FuzzyQuery returns the text typed so far into the active local fuzzy-find.
func (m docListModel) FuzzyQuery() string { return m.fuzzyQuery }

// FilterSuggestion returns the missing suffix of the best-matching
// top-level field name for whatever partial key is currently being typed
// into the Mongo filter, or "" if none applies. See filterFieldSuggestion.
func (m docListModel) FilterSuggestion() string {
	return filterFieldSuggestion(m.filter, m.docs)
}

func (m docListModel) Update(msg tea.Msg) (docListModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.filtering {
		switch keyMsg.Type {
		case tea.KeyEnter:
			m.filtering = false
			filter := m.filter
			m.filterCursor = 0
			return m, func() tea.Msg { return filterSubmittedMsg{Filter: filter} }
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
			m.filterCursor = 0
		case tea.KeyTab:
			m.filter += filterFieldSuggestion(m.filter, m.docs)
		case tea.KeyLeft:
			if m.filterCursor > 0 {
				m.filterCursor--
			}
		case tea.KeyRight:
			if m.filterCursor < len([]rune(m.filter)) {
				m.filterCursor++
			}
		case tea.KeyBackspace:
			m.backspaceFilter()
		case tea.KeyRunes:
			m.insertFilterRunes(keyMsg.Runes)
		}
		return m, nil
	}

	if m.fuzzyFiltering {
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.fuzzyFiltering = false
			m.fuzzyQuery = ""
			m.docs = m.allDocs
			m.cursor = 0
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.docs)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			if len(m.docs) > 0 {
				doc := m.docs[m.cursor]
				m.exitFuzzyFiltering(doc["_id"])
				return m, func() tea.Msg { return documentChosenMsg{Doc: doc} }
			}
		case tea.KeyBackspace:
			if r := []rune(m.fuzzyQuery); len(r) > 0 {
				m.fuzzyQuery = string(r[:len(r)-1])
			}
			m.applyFuzzyFilter()
		case tea.KeyRunes:
			m.fuzzyQuery += string(keyMsg.Runes)
			m.applyFuzzyFilter()
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.docs)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.docs) > 0 {
			doc := m.docs[m.cursor]
			return m, func() tea.Msg { return documentChosenMsg{Doc: doc} }
		}
	case "/":
		m.filtering = true
		m.filter = ""
		m.filterCursor = 0
	case "ctrl+f":
		m.fuzzyFiltering = true
		m.fuzzyQuery = ""
		m.allDocs = m.docs
	case "n":
		if (m.page+1)*m.pageSize < m.total {
			page := m.page + 1
			return m, func() tea.Msg { return pageChangedMsg{Page: page} }
		}
	case "p":
		if m.page > 0 {
			page := m.page - 1
			return m, func() tea.Msg { return pageChangedMsg{Page: page} }
		}
	case "i", "a":
		return m, func() tea.Msg { return insertRequestedMsg{} }
	case "tab":
		return m, func() tea.Msg { return switchToIndexesMsg{} }
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

// insertFilterRunes inserts each typed rune at filterCursor in order,
// applying auto-close and skip-over for "{" and "\"" (see
// insertFilterRune).
func (m *docListModel) insertFilterRunes(runes []rune) {
	for _, r := range runes {
		m.insertFilterRune(r)
	}
}

// insertFilterRune inserts one rune into filter at filterCursor. Typing "{"
// or "\"" auto-inserts the matching closer immediately after it, leaving
// the cursor between the two. Typing a closer ("}" or "\"") that is already
// sitting immediately to the right of the cursor moves the cursor past it
// instead of inserting a duplicate ("skip-over") — this is purely
// positional (it doesn't parse the JSON or track which characters were
// auto-inserted), which is what makes it work identically whether you're
// closing an auto-inserted pair or finishing a value string by hand.
func (m *docListModel) insertFilterRune(r rune) {
	text := []rune(m.filter)

	if (r == '}' || r == '"') && m.filterCursor < len(text) && text[m.filterCursor] == r {
		m.filterCursor++
		return
	}

	var closer rune
	switch r {
	case '{':
		closer = '}'
	case '"':
		closer = '"'
	}

	newText := make([]rune, 0, len(text)+2)
	newText = append(newText, text[:m.filterCursor]...)
	newText = append(newText, r)
	if closer != 0 {
		newText = append(newText, closer)
	}
	newText = append(newText, text[m.filterCursor:]...)

	m.filter = string(newText)
	m.filterCursor++
}

// backspaceFilter removes the rune immediately before filterCursor. If that
// rune is "{" or "\"" and the rune immediately after the cursor is its
// matching closer with nothing typed in between (an empty auto-closed
// pair), both characters are removed at once instead of leaving an orphan
// closer behind.
func (m *docListModel) backspaceFilter() {
	if m.filterCursor == 0 {
		return
	}
	text := []rune(m.filter)
	before := text[m.filterCursor-1]

	emptyPair := false
	if m.filterCursor < len(text) {
		after := text[m.filterCursor]
		emptyPair = (before == '{' && after == '}') || (before == '"' && after == '"')
	}

	if emptyPair {
		newText := make([]rune, 0, len(text)-2)
		newText = append(newText, text[:m.filterCursor-1]...)
		newText = append(newText, text[m.filterCursor+1:]...)
		m.filter = string(newText)
		m.filterCursor--
		return
	}

	newText := make([]rune, 0, len(text)-1)
	newText = append(newText, text[:m.filterCursor-1]...)
	newText = append(newText, text[m.filterCursor:]...)
	m.filter = string(newText)
	m.filterCursor--
}

// exitFuzzyFiltering leaves local fuzzy-find mode and restores the full
// loaded row set, with cursor moved to selectedID's position in that full
// set. Used on Enter: without this, fuzzy-find stayed active after opening
// a document, so the next keystroke (e.g. a panel-jump digit, once the
// detail popup closes) kept being swallowed as literal query text instead
// of reaching RootModel's global shortcut handling.
func (m *docListModel) exitFuzzyFiltering(selectedID any) {
	m.fuzzyFiltering = false
	m.fuzzyQuery = ""
	m.docs = m.allDocs
	m.cursor = 0
	for i, doc := range m.docs {
		if doc["_id"] == selectedID {
			m.cursor = i
			break
		}
	}
}

// applyFuzzyFilter recomputes docs from allDocs using the current
// fuzzyQuery, matched against each document's rendered _id (the same text
// shown for every collapsed, non-highlighted row — not nested field
// content, per spec), ordered by fuzzy match quality, and resets cursor to
// the top result.
func (m *docListModel) applyFuzzyFilter() {
	labels := make([]string, len(m.allDocs))
	for i, doc := range m.allDocs {
		labels[i] = fmt.Sprintf("%v", doc["_id"])
	}
	idxs := fuzzyMatchIndexes(m.fuzzyQuery, labels)
	docs := make([]bson.M, len(idxs))
	for i, idx := range idxs {
		docs[i] = m.allDocs[idx]
	}
	m.docs = docs
	m.cursor = 0
}

func (m docListModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Documentos (%d total, página %d)", m.total, m.page+1)) + "\n\n")

	if m.filtering {
		b.WriteString("Filtro: " + m.filter + "_\n\n")
	} else if m.filter != "" {
		b.WriteString(helpHintStyle.Render("Filtro activo: "+m.filter) + "\n\n")
	}

	if len(m.docs) == 0 {
		b.WriteString("(sin documentos)\n")
	}
	for i, doc := range m.docs {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + fmt.Sprintf("%v\n", doc["_id"]))
	}
	b.WriteString("\n" + helpHintStyle.Render("[/] filtrar  [n/p] página  [Enter] ver  [i] insertar  [Tab] índices"))
	return b.String()
}

// filterKeyPositionPattern matches when the filter text ends in an open
// JSON key position: a "{" or "," (a value's closing quote and any
// whitespace already consumed), followed by an opening quote and the
// partial key text typed so far, with no further quotes in between (an
// interior quote would mean we're actually past the key, e.g. typing a
// value or already at a colon).
var filterKeyPositionPattern = regexp.MustCompile(`[{,]\s*"([^"]*)$`)

// filterFieldSuggestion returns the missing suffix of the best-matching
// top-level field name for the partial key currently being typed in
// filter, or "" if filter isn't in a JSON key position (see
// filterKeyPositionPattern) or no known field name starts with the partial
// text typed so far. Field names are the union of top-level keys across
// docs — nested fields inside a bson.M/bson.A value are never considered,
// matching the same top-level-only boundary used by docPanelLines'
// "Array (N)"/"Object" placeholders (see document_render.go). Ties between
// multiple matching field names resolve to the alphabetically-first one.
func filterFieldSuggestion(filter string, docs []bson.M) string {
	match := filterKeyPositionPattern.FindStringSubmatch(filter)
	if match == nil {
		return ""
	}
	partial := match[1]

	fieldSet := map[string]bool{}
	for _, doc := range docs {
		for field := range doc {
			fieldSet[field] = true
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		if strings.HasPrefix(field, partial) {
			return field[len(partial):]
		}
	}
	return ""
}
```

Note: `FilterSuggestion()` and the `tea.KeyTab` case above are UNCHANGED from before this task (still end-of-string based) — Task 2 fixes both to be cursor-relative. This task is scoped to the cursor/insert/backspace/arrows/auto-close mechanics only.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestDocListModel_TypingInsertsAtCursor|TestDocListModel_BackspaceRemovesRuneBeforeCursor|TestDocListModel_ArrowsMoveAndClampCursor|TestDocListModel_TypingOpenBrace|TestDocListModel_TypingQuote|TestDocListModel_TypingClosing|TestDocListModel_BackspaceOnEmptyPair|TestDocListModel_BackspaceOnNonEmptyPair|TestDocListModel_InsertAndBackspaceHandleMultiByteRunesSafely"`
Expected: PASS (all 9 new tests)

- [ ] **Step 5: Run the full docListModel test file to check for regressions**

Run: `go test ./internal/tui/... -v -run TestDocListModel`
Expected: PASS — every pre-existing `TestDocListModel_*` test still passes (existing filter/fuzzy/pagination behavior is untouched by this task)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/document_list.go internal/tui/document_list_test.go
git commit -m "feat: add cursor-based editing with auto-close/skip-over to the Documents filter"
```

---

### Task 2: Cursor-relative autocomplete

**Files:**
- Modify: `internal/tui/document_list.go`
- Test: `internal/tui/document_list_test.go`

**Interfaces:**
- Consumes: `filterFieldSuggestion(filter string, docs []bson.M) string` (existing, unchanged)
- Produces: `func (m docListModel) FilterBeforeCursor() string`, `func (m docListModel) FilterAfterCursor() string`. Task 3 (root.go rendering) calls both.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/document_list_test.go`:

```go
func TestFilterFieldSuggestion_ViaAccessorIgnoresTextAfterCursor(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}}
	m := newDocListModel(docs, 1, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	for _, r := range `"nam` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.FilterText() != `{"nam"}` {
		t.Fatalf(`precondition failed: expected filter '{"nam"}' with cursor before the closing quote, got %q`, m.FilterText())
	}
	if m.FilterSuggestion() != "e" {
		t.Fatalf(`expected suggestion 'e' for the key typed before the cursor, ignoring the trailing '"}', got %q`, m.FilterSuggestion())
	}
}

func TestDocListModel_TabInsertsSuggestionAtCursorNotAtEnd(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}}
	m := newDocListModel(docs, 1, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	for _, r := range `"nam` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.FilterText() != `{"name"}` {
		t.Fatalf(`expected Tab to insert "e" at the cursor (before the closing quote), got %q`, m.FilterText())
	}
}

func TestDocListModel_FilterBeforeAndAfterCursorSplitCorrectly(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	if m.FilterBeforeCursor() != "{" {
		t.Fatalf("expected text before cursor to be '{', got %q", m.FilterBeforeCursor())
	}
	if m.FilterAfterCursor() != "}" {
		t.Fatalf("expected text after cursor to be '}', got %q", m.FilterAfterCursor())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run "TestFilterFieldSuggestion_ViaAccessorIgnoresTextAfterCursor|TestDocListModel_TabInsertsSuggestionAtCursorNotAtEnd|TestDocListModel_FilterBeforeAndAfterCursorSplitCorrectly"`
Expected: FAIL — `FilterBeforeCursor`/`FilterAfterCursor` undefined; `TestDocListModel_TabInsertsSuggestionAtCursorNotAtEnd` fails because Tab still appends to the end (`{"nam"}Tab` would wrongly produce `{"nam"}e` today, not `{"name"}`)

- [ ] **Step 3: Make the autocomplete cursor-relative**

In `internal/tui/document_list.go`, replace:

```go
// FilterSuggestion returns the missing suffix of the best-matching
// top-level field name for whatever partial key is currently being typed
// into the Mongo filter, or "" if none applies. See filterFieldSuggestion.
func (m docListModel) FilterSuggestion() string {
	return filterFieldSuggestion(m.filter, m.docs)
}
```

with:

```go
// textBeforeCursor returns the filter text up to (not including) the
// character at filterCursor.
func (m docListModel) textBeforeCursor() string {
	text := []rune(m.filter)
	if m.filterCursor > len(text) {
		return m.filter
	}
	return string(text[:m.filterCursor])
}

// textAfterCursor returns the filter text from filterCursor onward.
func (m docListModel) textAfterCursor() string {
	text := []rune(m.filter)
	if m.filterCursor > len(text) {
		return ""
	}
	return string(text[m.filterCursor:])
}

// FilterSuggestion returns the missing suffix of the best-matching
// top-level field name for whatever partial key is currently being typed
// into the Mongo filter, considering only the text before the cursor (text
// after it — e.g. an auto-closed "}" — is irrelevant to what key is being
// typed), or "" if none applies. See filterFieldSuggestion.
func (m docListModel) FilterSuggestion() string {
	return filterFieldSuggestion(m.textBeforeCursor(), m.docs)
}

// FilterBeforeCursor and FilterAfterCursor split the filter text at the
// cursor for rendering: RootModel draws the cursor-blink marker at the real
// cursor position instead of always at the end of the string.
func (m docListModel) FilterBeforeCursor() string { return m.textBeforeCursor() }
func (m docListModel) FilterAfterCursor() string  { return m.textAfterCursor() }
```

Then replace:

```go
		case tea.KeyTab:
			m.filter += filterFieldSuggestion(m.filter, m.docs)
```

with:

```go
		case tea.KeyTab:
			suggestion := []rune(filterFieldSuggestion(m.textBeforeCursor(), m.docs))
			if len(suggestion) > 0 {
				text := []rune(m.filter)
				newText := make([]rune, 0, len(text)+len(suggestion))
				newText = append(newText, text[:m.filterCursor]...)
				newText = append(newText, suggestion...)
				newText = append(newText, text[m.filterCursor:]...)
				m.filter = string(newText)
				m.filterCursor += len(suggestion)
			}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestFilterFieldSuggestion_ViaAccessorIgnoresTextAfterCursor|TestDocListModel_TabInsertsSuggestionAtCursorNotAtEnd|TestDocListModel_FilterBeforeAndAfterCursorSplitCorrectly"`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Run the full docListModel test file to check for regressions**

Run: `go test ./internal/tui/... -v -run TestDocListModel`
Expected: PASS — every pre-existing test plus Task 1's new tests still pass (in particular the pre-existing `TestDocListModel_TabAcceptsFilterSuggestion` and `TestDocListModel_TabWithNoSuggestionDoesNotCorruptFilter`, both written when the filter was end-of-string-only — they still exercise a cursor that happens to be at the true end, so they must keep passing unmodified)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/document_list.go internal/tui/document_list_test.go
git commit -m "feat: make filter autocomplete cursor-relative instead of end-of-string-relative"
```

---

### Task 3: Esc clears an applied filter, and render the cursor at its real position

**Files:**
- Modify: `internal/tui/document_list.go`
- Modify: `internal/tui/root.go`
- Test: `internal/tui/document_list_test.go`, `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `docListModel.FilterBeforeCursor()`, `docListModel.FilterAfterCursor()` (Task 2)
- Produces: `type filterClearedMsg struct{}` — `RootModel.Update`'s `panelDocuments` case handles it.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/document_list_test.go`:

```go
func TestDocListModel_EscWithAppliedFilterClearsItAndEmitsFilterClearedMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m.filter = `{"name":"Ana"}`

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command on Esc with an applied filter")
	}
	if _, ok := cmd().(filterClearedMsg); !ok {
		t.Fatalf("expected filterClearedMsg, got %T", cmd())
	}
}

func TestDocListModel_EscWithAppliedFilterClearsFilterText(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m.filter = `{"name":"Ana"}`

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.FilterText() != "" {
		t.Fatalf("expected filter text cleared after Esc, got %q", m.FilterText())
	}
}

func TestDocListModel_EscWithNoAppliedFilterSendsListBackMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command on Esc with no applied filter")
	}
	if _, ok := cmd().(listBackMsg); !ok {
		t.Fatalf("expected listBackMsg (unchanged behavior), got %T", cmd())
	}
}

func TestDocListModel_HKeyStillSendsListBackMsgRegardlessOfFilter(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)
	m.filter = `{"name":"Ana"}`

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if cmd == nil {
		t.Fatal("expected a command on 'h'")
	}
	if _, ok := cmd().(listBackMsg); !ok {
		t.Fatalf("expected 'h' to keep sending listBackMsg even with an applied filter, got %T", cmd())
	}
}
```

Append to `internal/tui/root_test.go`:

```go
func TestRootModel_EscWithAppliedFilterClearsAndReloadsDocuments(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "name": "Ana"}, {"_id": "o2", "name": "Beto"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	root.filter = bson.M{"name": "Ana"}
	model, _ = root.Update(documentsLoadedMsg{Docs: []bson.M{{"_id": "o1", "name": "Ana"}}, Total: 1})
	root = model.(RootModel)
	root.docList.filter = `{"name":"Ana"}`
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("precondition failed: expected focus=panelDocuments, got %v", root.focus)
	}

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEsc})
	root = model.(RootModel)
	if root.docList.FilterText() != "" {
		t.Fatalf("expected the applied filter text cleared, got %q", root.docList.FilterText())
	}
	if cmd == nil {
		t.Fatal("expected a command to reload documents")
	}
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if len(root.docList.docs) != 2 {
		t.Fatalf("expected all 2 documents reloaded without the filter, got %+v", root.docList.docs)
	}
}

func TestRootModel_DocumentsFilterCursorMarkerAtRealPosition(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "name": "Ana"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	model, _ = root.Update(documentsLoadedMsg{Docs: fake.Databases["shop"]["orders"], Total: 1})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	root = model.(RootModel)

	view := root.View()
	if !strings.Contains(view, "Filtro: {_}") {
		t.Fatalf("expected the cursor marker between the auto-closed braces, got:\n%s", view)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run "TestDocListModel_EscWith|TestDocListModel_HKeyStillSendsListBackMsg|TestRootModel_EscWithAppliedFilter|TestRootModel_DocumentsFilterCursorMarker"`
Expected: FAIL — `filterClearedMsg` undefined, `Esc` on an applied filter currently falls through to `listBackMsg{}` unconditionally, and the cursor marker still always renders at the end of the string

- [ ] **Step 3: Add filterClearedMsg and split the esc/h case**

In `internal/tui/document_list.go`, add this new message type next to the others near the top of the file:

```go
type filterClearedMsg struct{}
```

Then replace:

```go
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}
```

with:

```go
	case "esc":
		if m.filter != "" {
			m.filter = ""
			return m, func() tea.Msg { return filterClearedMsg{} }
		}
		return m, func() tea.Msg { return listBackMsg{} }
	case "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}
```

- [ ] **Step 4: Handle filterClearedMsg and render the cursor at its real position in root.go**

In `internal/tui/root.go`, in the `panelDocuments` case's `switch out := cmd().(type)` block, add a new case right after `filterSubmittedMsg`:

```go
			case filterSubmittedMsg:
				m.page = 0
				var filter bson.M
				if err := bson.UnmarshalExtJSON([]byte(out.Filter), false, &filter); err != nil {
					m.err = fmt.Errorf("filtro inválido: %w", err)
					return m, nil
				}
				m.filter = filter
				m.logf("Filtro aplicado: %s", out.Filter)
				return m, m.loadDocuments(filter)
			case filterClearedMsg:
				m.filter = nil
				m.page = 0
				m.logf("Filtro removido")
				return m, m.loadDocuments(bson.M{})
			case documentChosenMsg:
```

(Only the new `case filterClearedMsg:` block is added — `filterSubmittedMsg` and `documentChosenMsg` are shown as anchors and are otherwise unchanged.)

Then replace the filter-line construction:

```go
	docLines, docCursor := docPanelLines(m.docList.docs, m.docList.cursor)
	if m.docList.filtering {
		suggestion := helpHintStyle.Render(m.docList.FilterSuggestion())
		docLines = append([]string{"Filtro: " + m.docList.filter + suggestion + "_"}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
```

with:

```go
	docLines, docCursor := docPanelLines(m.docList.docs, m.docList.cursor)
	if m.docList.filtering {
		suggestion := helpHintStyle.Render(m.docList.FilterSuggestion())
		line := "Filtro: " + m.docList.FilterBeforeCursor() + "_" + suggestion + m.docList.FilterAfterCursor()
		docLines = append([]string{line}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestDocListModel_EscWith|TestDocListModel_HKeyStillSendsListBackMsg|TestRootModel_EscWithAppliedFilter|TestRootModel_DocumentsFilterCursorMarker"`
Expected: PASS (all 6 tests)

- [ ] **Step 6: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all packages build, vet is clean, all tests pass (pre-existing and new) — this touches `root.go`'s `View()` and `Update()`, which most other Documents-panel tests render/route through, so the full suite is the real check here

- [ ] **Step 7: Commit**

```bash
git add internal/tui/document_list.go internal/tui/document_list_test.go internal/tui/root.go internal/tui/root_test.go
git commit -m "feat: Esc clears an applied filter; render the filter cursor at its real position"
```

---

## Final manual smoke test (after all three tasks)

Run: `go build -o lazymongo . && ./lazymongo qa`, then confirm:
- In Documentos, `/` then typing `{` produces `{}` with the cursor between; typing `"` similarly produces `""`
- Typing a field name inside the braces still shows the autocomplete ghost suggestion correctly, ignoring the auto-closed `}` after the cursor; `Tab` accepts it at the cursor, not at the end
- Typing the closing `}`/`"` when one is already there skips over it instead of duplicating
- `←`/`→` move the cursor within the filter text; Backspace on an empty pair (e.g. right after typing `{` with nothing inside) removes both characters
- After applying a filter (`Enter`) and seeing "Filtro activo: ...", pressing `Esc` clears it and reloads all documents
- The local fuzzy-find (`Ctrl+f`) and the expanded-document view are both unaffected
