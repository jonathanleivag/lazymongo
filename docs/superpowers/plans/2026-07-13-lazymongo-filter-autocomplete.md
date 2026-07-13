# lazymongo — Filter field-name autocomplete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** While typing the Mongo query filter in `[0]` Documentos (`/`), inline-suggest the rest of a top-level field name (shell-style ghost text), sourced from the documents already loaded on the current page, acceptable with `Tab`.

**Architecture:** A pure function detects whether the filter text (append/backspace-only, so "cursor position" is always the end of the string) currently ends in an open JSON key position, and if so returns the missing suffix of the best-matching known field name. `docListModel` exposes it via a new accessor and a new `Tab` case; `RootModel`'s existing filter-line construction in `View()` renders the suggestion in a dim style right before the cursor blink.

**Tech Stack:** Go, standard `regexp`/`sort`/`strings` (no new dependencies).

## Global Constraints

- Only top-level field names are ever suggested — never nested `bson.M`/`bson.A` keys, and never values. This mirrors the same top-level-only boundary already used by the expanded-document view's `Array (N)`/`Object` placeholders.
- Field names come from the union of top-level keys across `m.docs` (the currently loaded page) — no new query is issued to fetch a broader sample.
- A tie between multiple matching field names resolves to the alphabetically-first one.
- `Tab` appends only the missing letters of the field name — never a closing quote.
- The existing `"Filtro: "` / `"Filtro activo: "` prepended-line construction and its pre-existing, out-of-scope cursor-alignment behavior are not touched beyond inserting the suggestion text — same rule already followed by the expanded-document-view and fuzzy-search plans.

---

### Task 1: Field-suggestion function + Tab handling in docListModel

**Files:**
- Modify: `internal/tui/document_list.go`
- Test: `internal/tui/document_list_test.go`

**Interfaces:**
- Produces: `func filterFieldSuggestion(filter string, docs []bson.M) string` (empty string means "no suggestion"), `func (m docListModel) FilterSuggestion() string`. Task 2 calls only `FilterSuggestion()`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/document_list_test.go`:

```go
func TestFilterFieldSuggestion_SuggestsRemainderAfterOpenBraceQuote(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}, {"_id": "2", "name": "Beto"}}
	suggestion := filterFieldSuggestion(`{"nam`, docs)
	if suggestion != "e" {
		t.Fatalf("expected suggestion 'e' completing 'name', got %q", suggestion)
	}
}

func TestFilterFieldSuggestion_SuggestsAfterComma(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana", "age": int32(30)}}
	suggestion := filterFieldSuggestion(`{"name": "Ana", "ag`, docs)
	if suggestion != "e" {
		t.Fatalf("expected suggestion 'e' completing 'age', got %q", suggestion)
	}
}

func TestFilterFieldSuggestion_TieResolvesToAlphabeticallyFirst(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana", "nationality": "AR"}}
	suggestion := filterFieldSuggestion(`{"na`, docs)
	if suggestion != "me" {
		t.Fatalf("expected 'name' (alphabetically before 'nationality') to win, got suggestion %q", suggestion)
	}
}

func TestFilterFieldSuggestion_EmptyPartialSuggestsFirstFieldAlphabetically(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana", "age": int32(30)}}
	suggestion := filterFieldSuggestion(`{"`, docs)
	if suggestion != "_id" {
		t.Fatalf("expected the alphabetically-first field '_id' fully suggested when nothing typed yet, got %q", suggestion)
	}
}

func TestFilterFieldSuggestion_NoMatchReturnsEmpty(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}}
	suggestion := filterFieldSuggestion(`{"zzz`, docs)
	if suggestion != "" {
		t.Fatalf("expected no suggestion for unmatched prefix, got %q", suggestion)
	}
}

func TestFilterFieldSuggestion_NotInKeyPositionInsideValueReturnsEmpty(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}}
	suggestion := filterFieldSuggestion(`{"name": "a`, docs)
	if suggestion != "" {
		t.Fatalf("expected no suggestion while typing a value, got %q", suggestion)
	}
}

func TestFilterFieldSuggestion_NotInKeyPositionAfterColonReturnsEmpty(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}}
	suggestion := filterFieldSuggestion(`{"name":`, docs)
	if suggestion != "" {
		t.Fatalf("expected no suggestion right after a completed key, got %q", suggestion)
	}
}

func TestFilterFieldSuggestion_IgnoresNestedFields(t *testing.T) {
	docs := []bson.M{{"_id": "1", "address": bson.M{"city": "BA", "country": "AR"}}}
	suggestion := filterFieldSuggestion(`{"cit`, docs)
	if suggestion != "" {
		t.Fatalf("expected no suggestion for a nested-only field name, got %q", suggestion)
	}
}

func TestDocListModel_TabAcceptsFilterSuggestion(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}}
	m := newDocListModel(docs, 1, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range `{"nam` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.FilterSuggestion() != "e" {
		t.Fatalf("expected suggestion 'e' before accepting, got %q", m.FilterSuggestion())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.FilterText() != `{"name` {
		t.Fatalf("expected Tab to accept the suggestion, got filter %q", m.FilterText())
	}
}

func TestDocListModel_TabWithNoSuggestionDoesNotCorruptFilter(t *testing.T) {
	docs := []bson.M{{"_id": "1", "name": "Ana"}}
	m := newDocListModel(docs, 1, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range `{"zzz` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.FilterText() != `{"zzz` {
		t.Fatalf("expected Tab with no suggestion to leave the filter unchanged, got %q", m.FilterText())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/tui/... -v -run "TestFilterFieldSuggestion|TestDocListModel_TabAccepts|TestDocListModel_TabWithNoSuggestion"`
Expected: FAIL — `filterFieldSuggestion`/`FilterSuggestion` undefined, and `Tab` while filtering isn't handled

- [ ] **Step 3: Implement the suggestion function and wire Tab**

In `internal/tui/document_list.go`, add `"regexp"` and `"sort"` to the import block (alongside the existing `"fmt"` and `"strings"`):

```go
import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)
```

Add this accessor right after the existing `FuzzyQuery` accessor:

```go
// FilterSuggestion returns the missing suffix of the best-matching
// top-level field name for whatever partial key is currently being typed
// into the Mongo filter, or "" if none applies. See filterFieldSuggestion.
func (m docListModel) FilterSuggestion() string {
	return filterFieldSuggestion(m.filter, m.docs)
}
```

In the `if m.filtering { switch keyMsg.Type { ... } }` block, add a `tea.KeyTab` case (between the existing `tea.KeyEsc` and `tea.KeyBackspace` cases):

```go
		case tea.KeyTab:
			m.filter += filterFieldSuggestion(m.filter, m.docs)
```

At the end of the file, add:

```go
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

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestFilterFieldSuggestion|TestDocListModel_TabAccepts|TestDocListModel_TabWithNoSuggestion"`
Expected: PASS (all 10 tests)

- [ ] **Step 5: Run the full docListModel test file to check for regressions**

Run: `go test ./internal/tui/... -v -run TestDocListModel`
Expected: PASS — including every pre-existing `TestDocListModel_*` test (the new `tea.KeyTab` case must not change behavior for any other filtering-mode key)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/document_list.go internal/tui/document_list_test.go
git commit -m "feat: add field-name autocomplete to the Documents Mongo filter"
```

---

### Task 2: Render the suggestion inline in RootModel's filter line

**Files:**
- Modify: `internal/tui/root.go:644-649`
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `docListModel.FilterSuggestion() string` (Task 1)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/root_test.go`:

```go
func TestRootModel_DocumentsFilterShowsInlineSuggestion(t *testing.T) {
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
	for _, r := range `{"nam` {
		model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		root = model.(RootModel)
	}

	view := root.View()
	if !strings.Contains(view, `Filtro: {"nam`) {
		t.Fatalf("expected the typed filter text to appear, got:\n%s", view)
	}
	if !strings.Contains(view, `{"name`) {
		t.Fatalf("expected the suggestion 'e' to appear appended right after the typed text, got:\n%s", view)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/... -v -run TestRootModel_DocumentsFilterShowsInlineSuggestion`
Expected: FAIL — the rendered line only shows `Filtro: {"nam_`, with no suggestion inserted

- [ ] **Step 3: Insert the suggestion into the filter line**

In `internal/tui/root.go`, replace:

```go
	docLines, docCursor := docPanelLines(m.docList.docs, m.docList.cursor)
	if m.docList.filtering {
		docLines = append([]string{"Filtro: " + m.docList.filter + "_"}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
```

with:

```go
	docLines, docCursor := docPanelLines(m.docList.docs, m.docList.cursor)
	if m.docList.filtering {
		suggestion := helpHintStyle.Render(m.docList.FilterSuggestion())
		docLines = append([]string{"Filtro: " + m.docList.filter + suggestion + "_"}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
```

(Only the `if m.docList.filtering` branch changes — the `else if` branch for an already-applied, no-longer-being-typed filter is untouched, since autocomplete only applies while actively typing.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestRootModel_DocumentsFilterShowsInlineSuggestion`
Expected: PASS

- [ ] **Step 5: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all packages build, vet is clean, all tests pass (pre-existing and new)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat: render the filter field-name suggestion inline in the Documents panel"
```

---

## Final manual smoke test (after both tasks)

Run: `go build -o lazymongo . && ./lazymongo qa`, then confirm:
- Press `/` in Documentos, type `{"` — the alphabetically-first field name of the loaded documents appears dimmed after the cursor
- Typing more letters narrows/changes the suggestion as expected; typing a prefix matching no field clears it
- `Tab` accepts the suggestion (appends the missing letters, no closing quote); typing normally after accepting continues to work
- Typing a value (after a `:` and an opening quote) shows no suggestion
- The existing Mongo filter behavior (`Enter` submits, `Esc` cancels) and the local fuzzy-find (`Ctrl+f`) both still work exactly as before
