# lazymongo — Expanded document view Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In the `[0]` Documentos panel, expand only the currently highlighted document to show every field (colored by BSON type), while every other document on the page stays collapsed to its existing single `_id` line.

**Architecture:** A new pure function builds the panel's flat display lines plus an "effective cursor" line index — the highlighted document contributes N lines (one per field) instead of 1, and `renderPanel` (unmodified) is told which physical line that block starts on. A second new function maps a single field value to a type-colored string, reusing `docDetailModel`'s existing alphabetical field ordering so the inline view and the popup agree.

**Tech Stack:** Go, `go.mongodb.org/mongo-driver/v2/bson`, `github.com/charmbracelet/lipgloss` (already a dependency — no new ones).

## Global Constraints

- `renderPanel`/`visibleWindow` (`internal/tui/panel.go`) are NOT modified — they keep assuming one line of input = one displayed item with one cursor marker. The multi-line expansion is handled entirely by computing lines + an effective cursor index before calling `renderPanel`, per the spec's chosen Approach A.
- Arrays (`bson.A`) and nested documents (`bson.M`) render as a collapsed placeholder (`Array (N)` / `Object`) — never their raw value, never recursively expanded. This is a deliberate, spec'd boundary, not a gap to fill in later.
- `docDetailModel`, the detail popup, editing (`e`/`E`), deletion (`d`/`x`), and search (`/`, `Ctrl+f`) are untouched by this plan — this is a rendering-only change to panel `[0]`'s collapsed/expanded row content.
- lipgloss emits zero ANSI styling without a real TTY (already discovered and documented in this codebase, see `panel.go`'s comment on `renderPanel`) — tests in this plan assert on the underlying text content (quoted strings, `Array (N)`, etc.), never on raw ANSI codes.

---

### Task 1: BSON type-colored value rendering + per-page line builder

**Files:**
- Create: `internal/tui/document_render.go`
- Test: `internal/tui/document_render_test.go`

**Interfaces:**
- Produces: `func styleBSONValue(v any) string`, `func expandedDocLines(doc bson.M) []string`, `func docPanelLines(docs []bson.M, cursor int) (lines []string, effectiveCursor int)`. Task 2 calls only `docPanelLines` — the other two are internal building blocks it composes, but keeping them as separate named functions makes each one independently testable.

Both functions are pure — no dependency on `RootModel`, `docListModel`, or any other TUI state — same testing approach as `panel.go`'s `renderPanel`/`visibleWindow`.

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestStyleBSONValue_ObjectID(t *testing.T) {
	id, err := bson.ObjectIDFromHex("640a53bfe6ef34dde9a6b4ba")
	if err != nil {
		t.Fatalf("unexpected error building ObjectID: %v", err)
	}
	out := styleBSONValue(id)
	if !strings.Contains(out, `ObjectID("640a53bfe6ef34dde9a6b4ba")`) {
		t.Fatalf("expected ObjectID hex representation, got %q", out)
	}
}

func TestStyleBSONValue_String(t *testing.T) {
	out := styleBSONValue("Ana")
	if !strings.Contains(out, `"Ana"`) {
		t.Fatalf("expected quoted string, got %q", out)
	}
}

func TestStyleBSONValue_Numbers(t *testing.T) {
	if !strings.Contains(styleBSONValue(int32(10)), "10") {
		t.Fatalf("expected '10' in rendering of int32(10), got %q", styleBSONValue(int32(10)))
	}
	if !strings.Contains(styleBSONValue(int64(20)), "20") {
		t.Fatalf("expected '20' in rendering of int64(20), got %q", styleBSONValue(int64(20)))
	}
	if !strings.Contains(styleBSONValue(3.5), "3.5") {
		t.Fatalf("expected '3.5' in rendering of float64(3.5), got %q", styleBSONValue(3.5))
	}
}

func TestStyleBSONValue_Bool(t *testing.T) {
	if !strings.Contains(styleBSONValue(true), "true") {
		t.Fatalf("expected 'true' in rendering, got %q", styleBSONValue(true))
	}
}

func TestStyleBSONValue_Nil(t *testing.T) {
	if !strings.Contains(styleBSONValue(nil), "null") {
		t.Fatalf("expected 'null' in rendering, got %q", styleBSONValue(nil))
	}
}

func TestStyleBSONValue_Time(t *testing.T) {
	ts := time.Date(2026, 7, 11, 18, 30, 43, 0, time.UTC)
	out := styleBSONValue(ts)
	if !strings.Contains(out, "2026-07-11") {
		t.Fatalf("expected date in rendering, got %q", out)
	}
}

func TestStyleBSONValue_Array(t *testing.T) {
	out := styleBSONValue(bson.A{"a", "b", "c"})
	if !strings.Contains(out, "Array (3)") {
		t.Fatalf("expected 'Array (3)' placeholder, got %q", out)
	}
}

func TestStyleBSONValue_NestedDocument(t *testing.T) {
	out := styleBSONValue(bson.M{"foo": "bar"})
	if !strings.Contains(out, "Object") {
		t.Fatalf("expected 'Object' placeholder, got %q", out)
	}
}

func TestExpandedDocLines_OneLinePerFieldSortedAlphabetically(t *testing.T) {
	doc := bson.M{"name": "Ana", "_id": "1", "age": int32(30)}
	lines := expandedDocLines(doc)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (one per field), got %d: %+v", len(lines), lines)
	}
	// "_id" sorts before "age" and "name" (ASCII '_' < 'a').
	if !strings.HasPrefix(lines[0], "_id:") {
		t.Fatalf("expected first line to be '_id:...', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "age:") {
		t.Fatalf("expected second line to be 'age:...', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "name:") {
		t.Fatalf("expected third line to be 'name:...', got %q", lines[2])
	}
}

func TestDocPanelLines_OnlyHighlightedDocumentExpands(t *testing.T) {
	docs := []bson.M{
		{"_id": "1", "name": "Ana"},
		{"_id": "2", "name": "Beto"},
		{"_id": "3", "name": "Caro"},
	}
	lines, effectiveCursor := docPanelLines(docs, 1)

	// doc 0: 1 collapsed line. doc 1 (highlighted): 2 field lines + 1 blank
	// separator. doc 2: 1 collapsed line. Total 5.
	if len(lines) != 5 {
		t.Fatalf("expected 5 total lines, got %d: %+v", len(lines), lines)
	}
	if effectiveCursor != 1 {
		t.Fatalf("expected effective cursor at line 1 (right after doc 0's single collapsed line), got %d", effectiveCursor)
	}
	if !strings.Contains(lines[1], "_id") {
		t.Fatalf("expected line 1 (start of the expanded block) to be a field line, got %q", lines[1])
	}
	if lines[3] != "" {
		t.Fatalf("expected a blank separator line after the expanded block, got %q", lines[3])
	}
}

func TestDocPanelLines_NonHighlightedDocsStayCollapsedToID(t *testing.T) {
	docs := []bson.M{
		{"_id": "1", "name": "Ana", "age": int32(30)},
		{"_id": "2", "name": "Beto"},
	}
	lines, _ := docPanelLines(docs, 1)

	if lines[0] != "1" {
		t.Fatalf("expected doc 0 collapsed to just its _id, got %q", lines[0])
	}
}

func TestDocPanelLines_EmptyDocsProducesNoLines(t *testing.T) {
	lines, effectiveCursor := docPanelLines(nil, 0)
	if len(lines) != 0 {
		t.Fatalf("expected no lines for an empty doc list, got %+v", lines)
	}
	if effectiveCursor != 0 {
		t.Fatalf("expected effective cursor 0 for an empty doc list, got %d", effectiveCursor)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/tui/... -v -run "TestStyleBSONValue|TestExpandedDocLines|TestDocPanelLines"`
Expected: FAIL — `styleBSONValue`/`expandedDocLines`/`docPanelLines` undefined

- [ ] **Step 3: Implement document_render.go**

```go
package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	bsonObjectIDStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // orange/yellow
	bsonStringStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	bsonNumberStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
	bsonBoolStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta
)

// styleBSONValue renders a document field's value as a single-line string,
// colored by its real Go/BSON type. Arrays and nested documents render as a
// collapsed placeholder ("Array (N)" / "Object") rather than their raw
// value — matching Mongo Compass's own collapsed display for nested data
// and avoiding new interactive state for drilling into sub-fields (the
// existing detail popup already covers that; see this plan's spec).
func styleBSONValue(v any) string {
	switch val := v.(type) {
	case nil:
		return helpHintStyle.Render("null")
	case bson.ObjectID:
		return bsonObjectIDStyle.Render(val.String())
	case string:
		return bsonStringStyle.Render(fmt.Sprintf("%q", val))
	case int32, int64, int, float64, float32:
		return bsonNumberStyle.Render(fmt.Sprintf("%v", val))
	case bool:
		return bsonBoolStyle.Render(fmt.Sprintf("%v", val))
	case time.Time:
		return bsonNumberStyle.Render(val.Format(time.RFC3339))
	case bson.A:
		return helpHintStyle.Render(fmt.Sprintf("Array (%d)", len(val)))
	case bson.M:
		return helpHintStyle.Render("Object")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// expandedDocLines renders one document's fields, one per line, colored by
// BSON type via styleBSONValue, in the same alphabetical field order
// docDetailModel already uses (see internal/tui/document_detail.go), so the
// inline expanded view and the detail popup agree on field order.
func expandedDocLines(doc bson.M) []string {
	fields := make([]string, 0, len(doc))
	for k := range doc {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	lines := make([]string, len(fields))
	for i, field := range fields {
		lines[i] = fmt.Sprintf("%s: %s", field, styleBSONValue(doc[field]))
	}
	return lines
}

// docPanelLines builds the flat display lines for panel [0], expanding only
// the document at cursor into all of its fields (via expandedDocLines)
// while every other document stays collapsed to its existing single _id
// line. It also returns the physical line index where the expanded block's
// first line lands: renderPanel (internal/tui/panel.go) takes a single
// line-index cursor and has no notion of multi-line items, so passing this
// computed index — not the raw docs-index cursor — is what makes the "> "
// marker land on the correct line once a document before the highlighted
// one may have expanded to more than one line.
func docPanelLines(docs []bson.M, cursor int) (lines []string, effectiveCursor int) {
	for i, doc := range docs {
		if i == cursor {
			effectiveCursor = len(lines)
			lines = append(lines, expandedDocLines(doc)...)
			lines = append(lines, "")
		} else {
			lines = append(lines, fmt.Sprintf("%v", doc["_id"]))
		}
	}
	return lines, effectiveCursor
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestStyleBSONValue|TestExpandedDocLines|TestDocPanelLines"`
Expected: PASS (all 12 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/document_render.go internal/tui/document_render_test.go
git commit -m "feat: add BSON type-colored, expand-on-highlight rendering for the Documents panel"
```

---

### Task 2: Wire the expanded view into RootModel's Documents panel

**Files:**
- Modify: `internal/tui/root.go:640-651`
- Modify: `internal/tui/document_list.go:164-167` (comment only)
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `docPanelLines(docs []bson.M, cursor int) (lines []string, effectiveCursor int)` (Task 1)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/root_test.go`:

```go
func TestRootModel_DocumentsPanelExpandsOnlyHighlightedDocument(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {
			{"_id": "o1", "total": int32(10)},
			{"_id": "o2", "total": int32(20)},
		},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	model, _ = root.Update(documentsLoadedMsg{Docs: fake.Databases["shop"]["orders"], Total: 2})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("precondition failed: expected focus=panelDocuments, got %v", root.focus)
	}

	view := root.View()
	if !strings.Contains(view, `total: `) {
		t.Fatalf("expected the highlighted document's fields to appear in the rendered view, got:\n%s", view)
	}
	if !strings.Contains(view, "o1") {
		t.Fatalf("expected the highlighted document's _id ('o1') to appear, got:\n%s", view)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/... -v -run TestRootModel_DocumentsPanelExpandsOnlyHighlightedDocument`
Expected: FAIL — the rendered view only shows `_id` values ("o1", "o2"), never `total: `, since panel `[0]` still calls the old `labelsFromDocs`

- [ ] **Step 3: Wire in docPanelLines**

In `internal/tui/root.go`, replace:

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
```

with:

```go
	docTitle := fmt.Sprintf("Documentos (%d total, pág %d)", m.docList.total, m.docList.page+1)
	if m.docList.FuzzyFiltering() {
		docTitle += " — Buscar: " + m.docList.FuzzyQuery() + "_"
	}
	docLines, docCursor := docPanelLines(m.docList.docs, m.docList.cursor)
	if m.docList.filtering {
		docLines = append([]string{"Filtro: " + m.docList.filter + "_"}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
	mainHeight := panelHeight*5 - 5
	main := renderPanel(0, docTitle, docLines, docCursor, m.focus == panelDocuments, mainWidth, mainHeight)
```

(The `"Filtro: "`/`"Filtro activo: "` prepended-line handling is left exactly as it was — same pre-existing, out-of-scope cursor-alignment behavior noted in this project's fuzzy-search plan, now just fed by `docCursor` instead of `m.docList.cursor` directly, with no other change to that block.)

Then remove the now-unused `labelsFromDocs` function (its only call site was the one just replaced):

```go
func labelsFromDocs(docs []bson.M) []string {
	labels := make([]string, len(docs))
	for i, doc := range docs {
		labels[i] = fmt.Sprintf("%v", doc["_id"])
	}
	return labels
}
```

Delete that whole function from `internal/tui/root.go`.

- [ ] **Step 4: Fix the now-stale comment in document_list.go**

In `internal/tui/document_list.go`, `applyFuzzyFilter`'s doc comment currently reads:

```go
// applyFuzzyFilter recomputes docs from allDocs using the current
// fuzzyQuery, matched against each document's rendered _id (the same text
// labelsFromDocs shows per row — not nested field content, per spec),
// ordered by fuzzy match quality, and resets cursor to the top result.
```

`labelsFromDocs` no longer exists after Step 3 — replace that comment with:

```go
// applyFuzzyFilter recomputes docs from allDocs using the current
// fuzzyQuery, matched against each document's rendered _id (the same text
// shown for every collapsed, non-highlighted row — not nested field
// content, per spec), ordered by fuzzy match quality, and resets cursor to
// the top result.
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestRootModel_DocumentsPanelExpandsOnlyHighlightedDocument`
Expected: PASS

- [ ] **Step 6: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all packages build, vet is clean, all tests pass (pre-existing and new) — this touches a call site every other Documents-panel test renders through, so the full suite is the real check here, not just the one new test.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/root.go internal/tui/document_list.go internal/tui/root_test.go
git commit -m "feat: expand only the highlighted document in the Documents panel"
```

---

## Final manual smoke test (after both tasks)

Run: `go build -o lazymongo . && ./lazymongo qa`, then confirm:
- The document under the cursor in `[0]` Documentos shows every field, colored by type (ObjectId, strings, numbers, booleans, dates distinguishable by color); every other row on the page still shows just its `_id`
- Moving the cursor (`j`/`k`) to a different document moves the expansion to that document
- A nested array/object field shows as `Array (N)`/`Object`, not its raw dump
- `Enter` on the highlighted document still opens the existing detail popup unchanged; editing/deleting from there still works
- `/` and `Ctrl+f` search still work exactly as before
