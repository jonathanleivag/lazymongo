# lazymongo — Fuzzy-find search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fzf-style, live-as-you-type fuzzy search to the `[2]` Databases, `[3]` Collections, `[4]` Indexes, and `[5]` Conexiones panels, plus an independent local fuzzy-find over already-loaded rows in `[0]` Documents (bound to `Ctrl+f`, separate from the existing Mongo query filter on `/`).

**Architecture:** A single pure helper (`fuzzyMatchIndexes`) wraps `github.com/sahilm/fuzzy` and returns matched indexes in score order; `listModel` (Databases/Collections/Conexiones), `idxListModel` (Indexes), and `docListModel` (Documents) each grow their own `filtering`/`allX` fields and call the shared helper to narrow their own displayed set on every keystroke. `RootModel` wires the new active-search states into `inTextEntry()` and fixes two related keystroke-stealing bugs the new feature would otherwise make routine.

**Tech Stack:** Go, Bubbletea, `github.com/sahilm/fuzzy` (new dependency, verified: `go get github.com/sahilm/fuzzy@v0.1.3`, `go mod tidy` resolves cleanly with zero extra indirect deps for non-test use).

## Global Constraints

- `fuzzy.Find(pattern string, data []string) Matches` returns **zero matches for an empty pattern** (verified by direct testing, not assumed) — every filtering call site must special-case the empty query to mean "show everything, original order," never delegate straight to `fuzzy.Find`.
- The search indicator (`Buscar: <query>_`) is shown in the panel **title**, never as a line prepended to the item list. The existing Mongo-filter indicator in Documents uses the prepended-line convention and has a pre-existing, out-of-scope cursor-misalignment bug from doing so — do not copy that pattern into any of the 4 new panels or the new local fuzzy-find.
- The local fuzzy-find in Documents (`Ctrl+f`) matches only against each document's rendered `_id` (the same text `labelsFromDocs` already shows per row) — never against other fields or nested content. This is an explicit out-of-scope boundary from the spec, not an oversight.
- The existing Mongo query filter (`/` in Documents, `docListModel.filtering`/`filter` fields) is untouched and independent from the new local fuzzy-find (`docListModel.fuzzyFiltering`/`fuzzyQuery` fields) — the two must never both be active at once, and neither one's key handling reaches into the other's state.
- Global single-key shortcuts (`1`-`5` panel jump, `Tab`) must be gated behind `!m.inTextEntry()` (Task 6) — today only `?` has this gate, which means typing a search query containing a digit 1-5 (e.g. searching for `email_1` or `haddacloud-v2`) currently ejects the user to a different panel mid-search.
- Every new filtering mode supports: typed runes narrow results live; `Backspace` widens them; `Esc` clears the filter and restores the full set (cursor resets to 0); `Up`/`Down` (not `j`/`k`, which are literal query characters while filtering) move the cursor within the current results; `Enter` acts on the highlighted result exactly as it would outside filtering mode.

---

### Task 1: Shared fuzzy-matching helper + dependency

**Files:**
- Create: `internal/tui/fuzzy.go`
- Test: `internal/tui/fuzzy_test.go`
- Modify: `go.mod`, `go.sum` (via `go get`)

**Interfaces:**
- Produces: `func fuzzyMatchIndexes(query string, labels []string) []int` — returns indexes into `labels` ordered best-match-first; empty query returns all indexes in original order. Every later task's filtering logic calls this.

- [ ] **Step 1: Write the failing tests**

```go
package tui

import "testing"

func TestFuzzyMatchIndexes_EmptyQueryReturnsAllInOriginalOrder(t *testing.T) {
	idxs := fuzzyMatchIndexes("", []string{"admin", "haddacloud-v2", "test"})
	if len(idxs) != 3 || idxs[0] != 0 || idxs[1] != 1 || idxs[2] != 2 {
		t.Fatalf("expected [0 1 2], got %v", idxs)
	}
}

func TestFuzzyMatchIndexes_NarrowsToSubsequenceMatches(t *testing.T) {
	idxs := fuzzyMatchIndexes("hdc", []string{"admin", "haddacloud-v2", "test"})
	if len(idxs) != 1 || idxs[0] != 1 {
		t.Fatalf("expected [1] (only 'haddacloud-v2' contains h,d,c in order), got %v", idxs)
	}
}

func TestFuzzyMatchIndexes_NoMatchesReturnsEmpty(t *testing.T) {
	idxs := fuzzyMatchIndexes("zzz", []string{"admin", "test"})
	if len(idxs) != 0 {
		t.Fatalf("expected no matches, got %v", idxs)
	}
}

func TestFuzzyMatchIndexes_BetterMatchRanksFirst(t *testing.T) {
	idxs := fuzzyMatchIndexes("test", []string{"testing", "test"})
	if len(idxs) != 2 || idxs[0] != 1 {
		t.Fatalf("expected the exact match 'test' (index 1) ranked first, got %v", idxs)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/tui/... -v -run TestFuzzyMatchIndexes`
Expected: FAIL — `fuzzyMatchIndexes` undefined

- [ ] **Step 3: Add the dependency and implement fuzzy.go**

Run: `go get github.com/sahilm/fuzzy@v0.1.3`

```go
package tui

import "github.com/sahilm/fuzzy"

// fuzzyMatchIndexes returns the indexes into labels whose text fuzzy-matches
// query, ordered best match first via github.com/sahilm/fuzzy's scoring. An
// empty query matches everything in original order — fuzzy.Find itself
// returns zero matches for an empty pattern, which would otherwise make an
// empty search box appear to filter out every item the moment it opens.
func fuzzyMatchIndexes(query string, labels []string) []int {
	if query == "" {
		idxs := make([]int, len(labels))
		for i := range labels {
			idxs[i] = i
		}
		return idxs
	}

	matches := fuzzy.Find(query, labels)
	idxs := make([]int, len(matches))
	for i, match := range matches {
		idxs[i] = match.Index
	}
	return idxs
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestFuzzyMatchIndexes`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Run go mod tidy and the full suite**

Run: `go mod tidy && go build ./... && go test ./...`
Expected: `go.mod` gains exactly one new direct requirement (`github.com/sahilm/fuzzy v0.1.3`), no new indirect deps beyond what `go mod tidy` adds for it; all tests still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/fuzzy.go internal/tui/fuzzy_test.go go.mod go.sum
git commit -m "feat: add shared fuzzy-matching helper over github.com/sahilm/fuzzy"
```

---

### Task 2: Fuzzy search in listModel (Databases/Collections/Conexiones)

**Files:**
- Modify: `internal/tui/list.go`
- Test: `internal/tui/list_test.go`

**Interfaces:**
- Consumes: `fuzzyMatchIndexes(query string, labels []string) []int` (Task 1)
- Produces: `func (m listModel) Filtering() bool`, `func (m listModel) FilterQuery() string` — Task 6 (root.go wiring) and Task 4 (connection_picker.go) both call these.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/list_test.go`:

```go
func TestListModel_SlashEntersFilteringAndNarrowsItems(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "haddacloud-v2", Label: "haddacloud-v2"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.Filtering() {
		t.Fatal("expected filtering mode to be active after '/'")
	}
	for _, r := range "hdc" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.Items) != 1 || m.Items[0].ID != "haddacloud-v2" {
		t.Fatalf("expected only 'haddacloud-v2' to match 'hdc', got %+v", m.Items)
	}
	if m.Cursor != 0 {
		t.Fatalf("expected cursor reset to 0 on the best match, got %d", m.Cursor)
	}
}

func TestListModel_EscDuringFilterRestoresFullList(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adm")})
	if len(m.Items) != 1 {
		t.Fatalf("expected filter to narrow to 1 item, got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.Filtering() {
		t.Fatal("expected filtering to be false after Esc")
	}
	if len(m.Items) != 2 {
		t.Fatalf("expected full list of 2 items restored after Esc, got %d", len(m.Items))
	}
}

func TestListModel_BackspaceDuringFilterWidensResults(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adminx")})
	if len(m.Items) != 0 {
		t.Fatalf("expected no matches for 'adminx', got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.Items) != 1 || m.Items[0].ID != "admin" {
		t.Fatalf("expected backspace to widen back to 'admin' matching, got %+v", m.Items)
	}
}

func TestListModel_EnterDuringFilterSelectsHighlightedItem(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}, {ID: "test", Label: "test"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter while filtering")
	}
	selected, ok := cmd().(itemSelectedMsg)
	if !ok || selected.Item.ID != "test" {
		t.Fatalf("expected itemSelectedMsg{Item.ID:'test'}, got %#v", cmd())
	}
}

func TestListModel_UpDownArrowsMoveCursorWithinFilterResults(t *testing.T) {
	items := []listItem{{ID: "test1", Label: "test1"}, {ID: "test2", Label: "test2"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
	if len(m.Items) != 2 {
		t.Fatalf("expected both items to match 'test', got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Cursor != 1 {
		t.Fatalf("expected Down to move cursor to 1, got %d", m.Cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.Cursor != 0 {
		t.Fatalf("expected Up to move cursor back to 0, got %d", m.Cursor)
	}
}

func TestListModel_NoMatchesThenBackspaceDoesNotPanic(t *testing.T) {
	items := []listItem{{ID: "admin", Label: "admin"}}
	m := newListModel("Test", items, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	if len(m.Items) != 0 {
		t.Fatalf("expected zero matches, got %d", len(m.Items))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.Items) != 1 {
		t.Fatalf("expected backspace to widen back to 1 match, got %d", len(m.Items))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run TestListModel_SlashEntersFiltering`
Expected: FAIL — `m.Filtering` undefined (no `/` handling yet)

- [ ] **Step 3: Implement filtering in list.go**

Replace the full contents of `internal/tui/list.go`:

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type listItem struct {
	ID    string
	Label string
	Color string
}

type itemSelectedMsg struct{ Item listItem }
type listBackMsg struct{}

type listModel struct {
	Title     string
	Items     []listItem
	Cursor    int
	CanCreate bool

	filtering   bool
	filterQuery string
	allItems    []listItem
}

func newListModel(title string, items []listItem, canCreate bool) listModel {
	return listModel{Title: title, Items: items, CanCreate: canCreate}
}

// Filtering reports whether this list is in an active fuzzy-search
// text-entry state, so RootModel.inTextEntry can keep global shortcuts
// (like "?", "1"-"5", "Tab") from stealing keystrokes meant for the query.
func (m listModel) Filtering() bool { return m.filtering }

// FilterQuery returns the text typed so far into the active fuzzy search.
func (m listModel) FilterQuery() string { return m.filterQuery }

func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.filtering {
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.filtering = false
			m.filterQuery = ""
			m.Items = m.allItems
			m.Cursor = 0
		case tea.KeyUp:
			if m.Cursor > 0 {
				m.Cursor--
			}
		case tea.KeyDown:
			if m.Cursor < len(m.Items)-1 {
				m.Cursor++
			}
		case tea.KeyEnter:
			if len(m.Items) > 0 {
				item := m.Items[m.Cursor]
				return m, func() tea.Msg { return itemSelectedMsg{Item: item} }
			}
		case tea.KeyBackspace:
			if r := []rune(m.filterQuery); len(r) > 0 {
				m.filterQuery = string(r[:len(r)-1])
			}
			m.applyFilter()
		case tea.KeyRunes:
			m.filterQuery += string(keyMsg.Runes)
			m.applyFilter()
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "j", "down":
		if m.Cursor < len(m.Items)-1 {
			m.Cursor++
		}
	case "k", "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "enter":
		if len(m.Items) > 0 {
			item := m.Items[m.Cursor]
			return m, func() tea.Msg { return itemSelectedMsg{Item: item} }
		}
	case "/":
		m.filtering = true
		m.filterQuery = ""
		m.allItems = m.Items
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

// applyFilter recomputes Items from allItems using the current filterQuery,
// ordered by fuzzy match quality (best first), and resets Cursor to the top
// result.
func (m *listModel) applyFilter() {
	labels := make([]string, len(m.allItems))
	for i, item := range m.allItems {
		labels[i] = item.Label
	}
	idxs := fuzzyMatchIndexes(m.filterQuery, labels)
	items := make([]listItem, len(idxs))
	for i, idx := range idxs {
		items[i] = m.allItems[idx]
	}
	m.Items = items
	m.Cursor = 0
}

func (m listModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.Title) + "\n\n")
	if len(m.Items) == 0 {
		b.WriteString("(vacío)\n")
	}
	for i, item := range m.Items {
		prefix := "  "
		if i == m.Cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + colorStyle(item.Color).Render(item.Label) + "\n")
	}
	if m.CanCreate {
		b.WriteString("\n" + helpHintStyle.Render("[a] nueva conexión"))
	}
	return b.String()
}
```

(Only `listModel`, `newListModel`, and `Update` changed — `View()` is reproduced unchanged for context; it is not exercised by the panel-grid UI's rendering path, which reads `Items`/`Cursor` directly, so it needs no changes here.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestListModel`
Expected: PASS (all `TestListModel_*` tests, including the pre-existing `TestListModel_MoveCursorDownAndUp`, `TestListModel_EnterSelectsCurrentItem`, `TestListModel_EscSendsBackMsg`)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/list.go internal/tui/list_test.go
git commit -m "feat: add fuzzy search to listModel (Databases/Collections/Conexiones)"
```

---

### Task 3: Fuzzy search in idxListModel (Indexes)

**Files:**
- Modify: `internal/tui/index_list.go`
- Test: `internal/tui/index_list_test.go`

**Interfaces:**
- Consumes: `fuzzyMatchIndexes(query string, labels []string) []int` (Task 1)
- Produces: `func (m idxListModel) Filtering() bool`, `func (m idxListModel) FilterQuery() string` — Task 6 (root.go wiring) calls these.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/index_list_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run TestIdxListModel_SlashFiltersIndexesByName`
Expected: FAIL — `m.Filtering` undefined

- [ ] **Step 3: Implement filtering in index_list.go**

In `internal/tui/index_list.go`, add these fields to the `idxListModel` struct (after `createField int`):

```go
	filtering   bool
	filterQuery string
	allIndexes  []mongo.IndexInfo
```

Add these two methods after `newIdxListModel`:

```go
// Filtering reports whether this panel is in an active fuzzy-search
// text-entry state, so RootModel.inTextEntry can keep global shortcuts
// (like "?", "1"-"5", "Tab") from stealing keystrokes meant for the query.
func (m idxListModel) Filtering() bool { return m.filtering }

// FilterQuery returns the text typed so far into the active fuzzy search.
func (m idxListModel) FilterQuery() string { return m.filterQuery }
```

In `Update`, insert a new `if m.filtering { ... }` block right after the existing `if m.creating { ... }` block (before the final `keyMsg, ok := msg.(tea.KeyMsg)` / `switch keyMsg.String()` block), and add a `"/"` case plus a `applyFilter` method. The full updated `Update` method and new helper:

```go
func (m idxListModel) Update(msg tea.Msg) (idxListModel, tea.Cmd) {
	if m.confirmingDrop {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if cmd == nil {
			return m, nil
		}
		result, ok := cmd().(confirmResultMsg)
		if !ok {
			return m, cmd
		}
		m.confirmingDrop = false
		if result.Confirmed {
			name := m.indexes[m.cursor].Name
			return m, func() tea.Msg { return indexDropConfirmedMsg{Name: name} }
		}
		return m, nil
	}

	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.creating = false
		case "tab":
			m.createField = (m.createField + 1) % 2
		case "enter":
			keys, unique := m.createKeys, m.createUnique
			return m, func() tea.Msg { return indexCreateSubmittedMsg{KeysJSON: keys, Unique: unique} }
		default:
			if m.createField == 1 && keyMsg.String() == " " {
				m.createUnique = !m.createUnique
			} else if m.createField == 0 {
				switch keyMsg.Type {
				case tea.KeyBackspace:
					if r := []rune(m.createKeys); len(r) > 0 {
						m.createKeys = string(r[:len(r)-1])
					}
				case tea.KeyRunes:
					m.createKeys += string(keyMsg.Runes)
				}
			}
		}
		return m, nil
	}

	if m.filtering {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.filtering = false
			m.filterQuery = ""
			m.indexes = m.allIndexes
			m.cursor = 0
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.indexes)-1 {
				m.cursor++
			}
		case tea.KeyBackspace:
			if r := []rune(m.filterQuery); len(r) > 0 {
				m.filterQuery = string(r[:len(r)-1])
			}
			m.applyFilter()
		case tea.KeyRunes:
			m.filterQuery += string(keyMsg.Runes)
			m.applyFilter()
		}
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.indexes)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "a":
		m.creating = true
		m.createKeys = ""
		m.createUnique = false
		m.createField = 0
	case "d":
		if len(m.indexes) > 0 {
			m.confirmingDrop = true
			m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar el índice %q?", m.indexes[m.cursor].Name)}
		}
	case "/":
		m.filtering = true
		m.filterQuery = ""
		m.allIndexes = m.indexes
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

// applyFilter recomputes indexes from allIndexes using the current
// filterQuery, matched against each index's Name, ordered by fuzzy match
// quality (best first), and resets cursor to the top result.
func (m *idxListModel) applyFilter() {
	labels := make([]string, len(m.allIndexes))
	for i, idx := range m.allIndexes {
		labels[i] = idx.Name
	}
	idxs := fuzzyMatchIndexes(m.filterQuery, labels)
	indexes := make([]mongo.IndexInfo, len(idxs))
	for i, ix := range idxs {
		indexes[i] = m.allIndexes[ix]
	}
	m.indexes = indexes
	m.cursor = 0
}
```

`View()` is unchanged (not reached by the panel-grid rendering path either — same reasoning as Task 2).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestIdxListModel`
Expected: PASS (all `TestIdxListModel_*`, including the pre-existing `TestIdxListModel_DSendsIndexDropConfirmedFlow` and `TestIdxListModel_AOpensCreateFormAndEnterSubmits`)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/index_list.go internal/tui/index_list_test.go
git commit -m "feat: add fuzzy search to idxListModel, gate a/d behind !filtering"
```

---

### Task 4: Gate Conexiones' "a" create-connection shortcut behind filtering

**Files:**
- Modify: `internal/tui/connection_picker.go:118`
- Test: `internal/tui/connection_picker_test.go`

**Interfaces:**
- Consumes: `listModel.Filtering() bool` (Task 2) via the embedded `m.list`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/connection_picker_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/... -v -run TestConnectionPicker_TypingADuringFilterDoesNotOpenCreateForm`
Expected: FAIL — `m.creating` is true (the "a" is intercepted before reaching the list)

- [ ] **Step 3: Add the guard**

In `internal/tui/connection_picker.go`, change:

```go
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "a" {
		m.creating = true
		m.form = newConnectionForm()
		return m, nil
	}
```

to:

```go
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "a" && !m.list.Filtering() {
		m.creating = true
		m.form = newConnectionForm()
		return m, nil
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestConnectionPicker`
Expected: PASS (all `TestConnectionPicker_*`, including the pre-existing `TestConnectionPicker_PressingAOpensCreateForm`, which still passes since it doesn't filter first)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/connection_picker.go internal/tui/connection_picker_test.go
git commit -m "fix: don't open create-connection form while typing a fuzzy search query"
```

---

### Task 5: Local fuzzy-find over loaded rows in Documents (Ctrl+f)

**Files:**
- Modify: `internal/tui/document_list.go`
- Test: `internal/tui/document_list_test.go`

**Interfaces:**
- Consumes: `fuzzyMatchIndexes(query string, labels []string) []int` (Task 1)
- Produces: `func (m docListModel) FuzzyFiltering() bool`, `func (m docListModel) FuzzyQuery() string` — Task 6 (root.go wiring) calls these.

This is independent from the existing Mongo query filter (`filtering`/`filter` fields, bound to `/`) — the two must never interact. `Ctrl+f`'s key value is `tea.KeyCtrlF`, whose `.String()` is `"ctrl+f"` (verified against the installed `github.com/charmbracelet/bubbletea@v1.3.10` — `key.go` maps `keyACK` to `"ctrl+f"`).

Note on testing "no new query is issued": `docListModel` holds no reference to `mongo.Client` at all (it only ever receives already-loaded `[]bson.M`), so there is no `FakeClient` call-count assertion to write here — it is structurally impossible for this local fuzzy-find to reach the database. The tests below assert the actual observable behavior instead: narrowing/restoring `m.docs` in place, with no `tea.Cmd` other than the pre-existing `documentChosenMsg` on `Enter`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/document_list_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run TestDocListModel_CtrlFOpensLocalFuzzy`
Expected: FAIL — `m.FuzzyFiltering` undefined

- [ ] **Step 3: Implement local fuzzy-find in document_list.go**

Replace the full contents of `internal/tui/document_list.go`:

```go
package tui

import (
	"fmt"
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
	docs      []bson.M
	total     int64
	page      int64
	pageSize  int64
	cursor    int
	filtering bool
	filter    string

	fuzzyFiltering bool
	fuzzyQuery     string
	allDocs        []bson.M
}

func newDocListModel(docs []bson.M, total, page, pageSize int64) docListModel {
	return docListModel{docs: docs, total: total, page: page, pageSize: pageSize}
}

func (m docListModel) FilterText() string { return m.filter }

// FuzzyFiltering reports whether the local (non-Mongo) fuzzy-find over
// already-loaded rows is active, so RootModel.inTextEntry can keep global
// shortcuts (like "?", "1"-"5", "Tab") from stealing keystrokes meant for
// the search query.
func (m docListModel) FuzzyFiltering() bool { return m.fuzzyFiltering }

// FuzzyQuery returns the text typed so far into the active local fuzzy-find.
func (m docListModel) FuzzyQuery() string { return m.fuzzyQuery }

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
			return m, func() tea.Msg { return filterSubmittedMsg{Filter: filter} }
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
		case tea.KeyBackspace:
			if r := []rune(m.filter); len(r) > 0 {
				m.filter = string(r[:len(r)-1])
			}
		case tea.KeyRunes:
			m.filter += string(keyMsg.Runes)
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

// applyFuzzyFilter recomputes docs from allDocs using the current
// fuzzyQuery, matched against each document's rendered _id (the same text
// labelsFromDocs shows per row — not nested field content, per spec),
// ordered by fuzzy match quality, and resets cursor to the top result.
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
```

(`View()` is reproduced unchanged for context — same reasoning as Task 2: it's not reached by the panel-grid rendering path.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestDocListModel`
Expected: PASS (all `TestDocListModel_*`, including every pre-existing test in the file)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/document_list.go internal/tui/document_list_test.go
git commit -m "feat: add local fuzzy-find over loaded document rows (Ctrl+f)"
```

---

### Task 6: Wire fuzzy search into RootModel, help text, README

**Files:**
- Modify: `internal/tui/root.go`
- Modify: `internal/tui/help.go`
- Modify: `README.md`
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `listModel.Filtering()`/`FilterQuery()` (Task 2), `idxListModel.Filtering()`/`FilterQuery()` (Task 3), `docListModel.FuzzyFiltering()`/`FuzzyQuery()` (Task 5)

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/root_test.go`:

```go
func TestRootModel_InTextEntry_TrueWhileFilteringDatabasesPanel(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDatabaseListModel([]string{"shop", "admin"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	r := model.(RootModel)
	if !r.inTextEntry() {
		t.Fatal("expected inTextEntry to be true while filtering the Databases panel")
	}
}

func TestRootModel_DigitDuringFilterAddsToQueryInsteadOfSwitchingFocus(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDatabaseListModel([]string{"shop-v1", "shop-v2"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v2")})
	r := model.(RootModel)
	if r.focus != panelDatabases {
		t.Fatalf("expected focus to remain on Databases while its filter query contains a digit, got %v", r.focus)
	}
	if len(r.dbList.Items) != 1 || r.dbList.Items[0].ID != "shop-v2" {
		t.Fatalf("expected filter to narrow to 'shop-v2', got %+v", r.dbList.Items)
	}
}

func TestRootModel_QuestionMarkDuringFilterDoesNotOpenHelp(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDatabaseListModel([]string{"shop"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	r := model.(RootModel)
	if r.popup == popupHelp {
		t.Fatal("expected '?' typed into an active filter query to NOT open help")
	}
	if r.dbList.FilterQuery() != "?" {
		t.Fatalf("expected '?' to be added to the filter query, got %q", r.dbList.FilterQuery())
	}
}

func TestRootModel_EscDuringDatabaseFilterRestoresFullListWithoutLeavingPanel(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDatabaseListModel([]string{"shop", "admin"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adm")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyEsc})
	r := model.(RootModel)
	if r.focus != panelDatabases {
		t.Fatalf("expected Esc to only clear the filter, not change focus, got focus=%v", r.focus)
	}
	if len(r.dbList.Items) != 2 {
		t.Fatalf("expected full database list restored, got %d items", len(r.dbList.Items))
	}
}

func TestRootModel_CtrlFOnDocumentsPanelIsGuardedByInTextEntry(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.focus = panelDocuments
	root.docList = newDocListModel([]bson.M{{"_id": "o1"}, {"_id": "o2"}}, 2, 0, 20)

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	r := model.(RootModel)
	if !r.inTextEntry() {
		t.Fatal("expected inTextEntry true after Ctrl+f on Documents")
	}

	model, _ = r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	r2 := model.(RootModel)
	if r2.popup == popupHelp {
		t.Fatal("expected '?' typed during local fuzzy-find to NOT open help")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run TestRootModel_InTextEntry_TrueWhileFilteringDatabasesPanel`
Expected: FAIL — `inTextEntry` doesn't know about `dbList.Filtering()` yet, and pressing "/" on the Databases panel currently does nothing (no `/` handling reaches `listModel.Update` yet as of Task 2 alone — Task 2 added the capability, but nothing routes the keystroke differently at the RootModel level; this test now exercises the full path through `RootModel.Update`)

- [ ] **Step 3: Update inTextEntry and the global key gate in root.go**

Replace `inTextEntry`:

```go
// inTextEntry reports whether the current focus/popup is in an active
// text-entry sub-state, where printable keys like "?" must be typed
// literally rather than treated as a global shortcut.
func (m RootModel) inTextEntry() bool {
	switch {
	case m.focus == panelConnections && m.connPicker.creating:
		return true
	case m.focus == panelConnections && m.connPicker.list.Filtering():
		return true
	case m.focus == panelDatabases && m.dbList.Filtering():
		return true
	case m.focus == panelCollections && m.collList.Filtering():
		return true
	case m.focus == panelIndexes && m.idxList.creating:
		return true
	case m.focus == panelIndexes && m.idxList.Filtering():
		return true
	case m.focus == panelDocuments && m.docList.filtering:
		return true
	case m.focus == panelDocuments && m.docList.FuzzyFiltering():
		return true
	case m.popup == popupFieldEdit && !m.fieldEdit.confirming:
		return true
	}
	return false
}
```

In `Update`, change the global panel-jump gate from:

```go
		if m.popup == popupNone {
			switch keyMsg.String() {
			case "1":
```

to:

```go
		if m.popup == popupNone && !m.inTextEntry() {
			switch keyMsg.String() {
			case "1":
```

(the rest of that `switch` — cases `"2"` through `"tab"` — is unchanged, only the guarding `if` condition changes).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestRootModel_InTextEntry_TrueWhileFilteringDatabasesPanel|TestRootModel_DigitDuringFilter|TestRootModel_QuestionMarkDuringFilter|TestRootModel_EscDuringDatabaseFilter|TestRootModel_CtrlFOnDocumentsPanel`
Expected: PASS (all 5 new tests)

- [ ] **Step 5: Update rendering in root.go's View()**

Change `labelsFromListModel`:

```go
// labelsFromListModel renders each item's colored label as a plain line for
// panel display (no cursor/border chrome — renderPanel adds that). While
// filtering with no matches, a single placeholder line is shown instead of
// an empty list — safe because there's no real selection to misalign
// against an empty item set.
func labelsFromListModel(m listModel) []string {
	if m.Filtering() && len(m.Items) == 0 {
		return []string{helpHintStyle.Render("(sin coincidencias)")}
	}
	labels := make([]string, len(m.Items))
	for i, item := range m.Items {
		labels[i] = colorStyle(item.Color).Render(item.Label)
	}
	return labels
}
```

Change `labelsFromIndexes` (signature changes from `[]mongo.IndexInfo` to `idxListModel`, so it can see filtering state):

```go
func labelsFromIndexes(m idxListModel) []string {
	if m.Filtering() && len(m.indexes) == 0 {
		return []string{helpHintStyle.Render("(sin coincidencias)")}
	}
	labels := make([]string, len(m.indexes))
	for i, idx := range m.indexes {
		unique := ""
		if idx.Unique {
			unique = " (unique)"
		}
		labels[i] = fmt.Sprintf("%s %v%s", idx.Name, idx.Key, unique)
	}
	return labels
}
```

In `View()`, replace the panel-construction block:

```go
	statusLines := []string{colorStyle(m.conn.Color).Render(m.conn.Name), fmt.Sprintf("%s.%s", m.db, m.coll)}
	p1 := renderPanel(1, "Status", statusLines, 0, m.focus == panelStatus, sidebarWidth, panelHeight)
	p2 := renderPanel(2, "Databases", labelsFromListModel(m.dbList), m.dbList.Cursor, m.focus == panelDatabases, sidebarWidth, panelHeight)
	p3 := renderPanel(3, "Collections", labelsFromListModel(m.collList), m.collList.Cursor, m.focus == panelCollections, sidebarWidth, panelHeight)
	p4 := renderPanel(4, "Indexes", labelsFromIndexes(m.idxList.indexes), m.idxList.cursor, m.focus == panelIndexes, sidebarWidth, panelHeight)
	p5 := renderPanel(5, "Conexiones", labelsFromListModel(m.connPicker.list), m.connPicker.list.Cursor, m.focus == panelConnections, sidebarWidth, panelHeight)
```

with:

```go
	statusLines := []string{colorStyle(m.conn.Color).Render(m.conn.Name), fmt.Sprintf("%s.%s", m.db, m.coll)}

	dbTitle := "Databases"
	if m.dbList.Filtering() {
		dbTitle = "Databases — Buscar: " + m.dbList.FilterQuery() + "_"
	}
	collTitle := "Collections"
	if m.collList.Filtering() {
		collTitle = "Collections — Buscar: " + m.collList.FilterQuery() + "_"
	}
	idxTitle := "Indexes"
	if m.idxList.Filtering() {
		idxTitle = "Indexes — Buscar: " + m.idxList.FilterQuery() + "_"
	}
	connTitle := "Conexiones"
	if m.connPicker.list.Filtering() {
		connTitle = "Conexiones — Buscar: " + m.connPicker.list.FilterQuery() + "_"
	}

	p1 := renderPanel(1, "Status", statusLines, 0, m.focus == panelStatus, sidebarWidth, panelHeight)
	p2 := renderPanel(2, dbTitle, labelsFromListModel(m.dbList), m.dbList.Cursor, m.focus == panelDatabases, sidebarWidth, panelHeight)
	p3 := renderPanel(3, collTitle, labelsFromListModel(m.collList), m.collList.Cursor, m.focus == panelCollections, sidebarWidth, panelHeight)
	p4 := renderPanel(4, idxTitle, labelsFromIndexes(m.idxList), m.idxList.cursor, m.focus == panelIndexes, sidebarWidth, panelHeight)
	p5 := renderPanel(5, connTitle, labelsFromListModel(m.connPicker.list), m.connPicker.list.Cursor, m.focus == panelConnections, sidebarWidth, panelHeight)
```

Replace the `docTitle`/footer construction:

```go
	docTitle := fmt.Sprintf("Documentos (%d total, pág %d)", m.docList.total, m.docList.page+1)
	docLines := labelsFromDocs(m.docList.docs)
	if m.docList.filtering {
		docLines = append([]string{"Filtro: " + m.docList.filter + "_"}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
	mainHeight := panelHeight*5 - 5
	main := renderPanel(0, docTitle, docLines, m.docList.cursor, m.focus == panelDocuments, mainWidth, mainHeight)

	footer := "[1-5] panel  [j/k] mover  [Tab] documentos  [Enter] ver  [e] editar  [d] borrar  [/] filtro  [?] ayuda  [Ctrl+c] salir"
```

with:

```go
	docTitle := fmt.Sprintf("Documentos (%d total, pág %d)", m.docList.total, m.docList.page+1)
	if m.docList.FuzzyFiltering() {
		docTitle += " — Buscar: " + m.docList.FuzzyQuery() + "_"
	}
	docLines := labelsFromDocs(m.docList.docs)
	if m.docList.filtering {
		docLines = append([]string{"Filtro: " + m.docList.filter + "_"}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
	mainHeight := panelHeight*5 - 5
	main := renderPanel(0, docTitle, docLines, m.docList.cursor, m.focus == panelDocuments, mainWidth, mainHeight)

	footer := "[1-5] panel  [j/k] mover  [Tab] documentos  [/] buscar/filtro  [Ctrl+f] buscar en docs  [Enter] ver  [e] editar  [d] borrar  [?] ayuda  [Ctrl+c] salir"
```

(The pre-existing `"Filtro: "`/`"Filtro activo: "` prepended-line handling for the Mongo query filter is left untouched, per Global Constraints — its cursor-alignment bug is out of scope for this plan.)

- [ ] **Step 6: Update help.go**

Replace the full contents of `internal/tui/help.go`:

```go
package tui

const helpText = `lazymongo — atajos

1-5            saltar a un panel (Status/Databases/Collections/Indexes/Conexiones)
Tab            saltar a Documentos (o a Indexes si ya estás en Documentos)
j/k, flechas   moverse dentro del panel enfocado
Enter          ver documento / conectar (en Conexiones) / entrar
Esc            cerrar el popup activo / salir de un buscador activo
/              buscar (fuzzy en Databases/Collections/Indexes/Conexiones; filtro de query Mongo en Documentos)
Ctrl+f         buscar (fuzzy) entre los documentos ya cargados en pantalla
n/p            página siguiente/anterior (en Documentos)
i, a           insertar documento / crear conexión o índice
e              editar campo (en detalle de documento)
E              editar documento completo en $EDITOR
d, x           borrar (siempre pide confirmación)
?              esta ayuda
Ctrl+c         salir

Presiona cualquier tecla para cerrar esta ayuda.`

type helpModel struct{}

func (m helpModel) View() string { return helpText }
```

- [ ] **Step 7: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all packages build, vet is clean, all tests pass (pre-existing and new)

- [ ] **Step 8: Update README.md**

In the "Keybindings" table, change the `/` row and add a `Ctrl+f` row right after it:

```markdown
| `/` | buscar/filtrar: fuzzy-search por nombre en Databases/Collections/Indexes/Conexiones; filtro de query Mongo en Documentos |
| `Ctrl+f` | fuzzy-search entre los documentos ya cargados en pantalla (no dispara una nueva query) |
```

In the "Manual smoke test against qa" checklist, add two bullets right after the existing `2`/`3` bullets:

```markdown
- [ ] `2` focuses Databases; `/` opens fuzzy search, typing narrows the list live, `Esc` restores the full list, `Enter` selects the highlighted database
- [ ] `5` focuses Conexiones; `/` fuzzy-searches connection names without triggering `a` (create connection) if the query contains the letter "a"
```

- [ ] **Step 9: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go internal/tui/help.go README.md
git commit -m "feat: wire fuzzy search into RootModel, gate global shortcuts behind inTextEntry"
```

---

## Final manual smoke test (after all tasks)

Run: `go build -o lazymongo . && ./lazymongo qa`, then confirm:
- `/` on Databases/Collections/Indexes/Conexiones narrows the list live as you type, `Esc` restores it, `Up`/`Down` move within results, `Enter` acts as it normally would
- Typing a query containing "v2" or "1" while filtering keeps focus on the same panel (does not jump to another panel)
- `Ctrl+f` on Documents narrows the currently-loaded page's rows by `_id` without issuing a new query (verify no new command-log "Filtro aplicado" line appears)
- The existing Mongo query filter (`/` in Documents) still works exactly as before, unaffected by any of the above
