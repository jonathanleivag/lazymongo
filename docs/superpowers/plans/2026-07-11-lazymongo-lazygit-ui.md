# lazymongo lazygit-style UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `lazymongo`'s drill-down navigation with a persistent lazygit-style layout: 5 numbered side panels (Status/Databases/Collections/Indexes/Conexiones) + a main Documents panel + a command log + a keybinding footer, with document detail/edit/confirm/delete/help rendered as centered popups.

**Architecture:** Two new standalone rendering files (`panel.go`, `layout.go`) built on `lipgloss.JoinHorizontal`/`JoinVertical`/`Place`, plus a rewrite of `RootModel`'s navigation model — a view-stack (`viewID` + `pushView`/`popView`) becomes a focus model (`panelID`, which of the 6 panels currently has the cursor) and a popup model (`popupID`, which floating dialog — if any — is open on top). Every existing MongoDB-facing message handler (connect, load, insert/edit/delete, confirm-before-write) is reused verbatim; only how state changes translate into `focus`/`popup`/rendering changes.

**Tech Stack:** Go, Bubbletea, Lipgloss (`JoinHorizontal`, `JoinVertical`, `Place`, `Border`) — same versions already in `go.mod`, no new dependencies.

## Global Constraints

- Module path: `github.com/jonathanleivag/lazymongo`
- `internal/config` and `internal/mongo` are NOT touched by this plan at all.
- Popups render as a centered box on a blank canvas (`lipgloss.Place`), NOT as a true character-level overlay compositing over the panels behind them. This is a deliberate, documented implementation choice: true ANSI-safe overlay compositing (splicing popup content into specific screen regions of the already-rendered panel grid) is fragile and hard to unit-test; a blank-canvas popup still satisfies "Esc closes the popup and returns to where you were with panel state intact" — it just doesn't show the panels dimmed behind it. If this looks wrong once running, it's a one-function change (`renderPopupOverlay` in `layout.go`), not a redesign.
- Panels `[2] Databases` and `[3] Collections` cascade **live on cursor movement** (`j`/`k`), no `Enter` required — highlighting a database loads its collections; highlighting a collection loads its indexes and documents. Panel `[5] Conexiones` is the exception: connecting is a heavier action (a real network connect), so it still requires an explicit `Enter`, matching the existing `connectionChosenMsg` flow.
- Every mutating action (insert, inline edit, full-document replace, delete, create index, drop index) still shows a confirmation before executing, on every connection, no exceptions — this rule is unchanged from v1; only its rendering (popup instead of full-screen) changes.
- The command log records every meaningful action (connect, filter applied, confirmed writes) and every error — not just failures.
- No aggregation pipeline builder, no schema viewer, no connection edit/delete UI, no read-only lockdown mode — still all out of scope, unchanged from v1.

---

### Task 1: Panel rendering primitive

**Files:**
- Create: `internal/tui/panel.go`
- Test: `internal/tui/panel_test.go`

**Interfaces:**
- Consumes: `colorStyle`, `titleStyle`, `cursorStyle` (`internal/tui/style.go`, existing)
- Produces: `func renderPanel(number int, title string, items []string, cursor int, focused bool, width, height int) string`

This is a pure function — no dependency on `RootModel` or any other TUI state — so it's testable with plain string slices.

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"strings"
	"testing"
)

func TestRenderPanel_ShowsNumberedTitle(t *testing.T) {
	out := renderPanel(2, "Databases", []string{"admin", "test"}, 0, false, 30, 5)
	if !strings.Contains(out, "[2] Databases") {
		t.Fatalf("expected title with panel number, got:\n%s", out)
	}
}

func TestRenderPanel_HighlightsCursorItem(t *testing.T) {
	plain := renderPanel(2, "Databases", []string{"admin", "test"}, 0, false, 30, 5)
	cursorOnSecond := renderPanel(2, "Databases", []string{"admin", "test"}, 1, false, 30, 5)
	if plain == cursorOnSecond {
		t.Fatal("expected rendering to differ when cursor moves to a different item")
	}
}

func TestRenderPanel_FocusedDiffersFromUnfocused(t *testing.T) {
	unfocused := renderPanel(1, "Status", []string{"a"}, 0, false, 30, 5)
	focused := renderPanel(1, "Status", []string{"a"}, 0, true, 30, 5)
	if unfocused == focused {
		t.Fatal("expected focused and unfocused rendering to differ")
	}
	if !strings.Contains(focused, "▶") {
		t.Fatalf("expected a focus marker in the focused panel's heading, got:\n%s", focused)
	}
}

func TestRenderPanel_TruncatesToHeightKeepingCursorVisible(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	// innerHeight for height=5 leaves room for border+title; cursor near the end
	// must still be visible in the rendered output.
	out := renderPanel(2, "Databases", items, 7, false, 30, 5)
	if !strings.Contains(out, "h") {
		t.Fatalf("expected the item at the cursor to remain visible after truncation, got:\n%s", out)
	}
}

func TestRenderPanel_EmptyItemsDoesNotPanic(t *testing.T) {
	out := renderPanel(3, "Collections", nil, 0, false, 30, 5)
	if !strings.Contains(out, "[3] Collections") {
		t.Fatalf("expected title even with no items, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/tui/... -v -run TestRenderPanel`
Expected: FAIL — `renderPanel` undefined

- [ ] **Step 3: Implement panel.go**

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	focusedBorderColor   = lipgloss.Color("6") // cyan
	unfocusedBorderColor = lipgloss.Color("8") // gray
)

// renderPanel draws one bordered, numbered panel (e.g. "[2] Databases") with
// its items, highlighting the item at cursor. If there are more items than
// fit in height, the visible window scrolls to keep cursor in view. This is
// a pure function — no dependency on RootModel — so any of the 5 side
// panels or the main Documents panel can be rendered by calling this with
// their own title/items/cursor.
//
// Focus is indicated by BOTH border color AND a "▶ " text marker in the
// heading — not color alone. lipgloss only emits ANSI codes when it detects
// a real terminal; `go test` has no TTY, so `BorderForeground` (and any
// other styling) silently renders as plain text with zero visible
// difference in that environment (verified empirically while writing this
// plan). The text marker keeps focus state testable headlessly, and is a
// genuine accessibility win in real terminals too (works regardless of
// color support).
func renderPanel(number int, title string, items []string, cursor int, focused bool, width, height int) string {
	borderColor := unfocusedBorderColor
	if focused {
		borderColor = focusedBorderColor
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width).
		Height(height)

	marker := "  "
	if focused {
		marker = "▶ "
	}
	heading := titleStyle.Render(fmt.Sprintf("%s[%d] %s", marker, number, title))

	innerHeight := height - 1 // one line reserved for the heading
	if innerHeight < 1 {
		innerHeight = 1
	}

	visible := visibleWindow(items, cursor, innerHeight)

	var b strings.Builder
	b.WriteString(heading)
	for _, line := range visible {
		b.WriteString("\n")
		b.WriteString(line)
	}

	return box.Render(b.String())
}

// visibleWindow returns the slice of items that should be visible given a
// maximum of maxLines rows, scrolling so that the item at cursor is always
// included. Each returned line has the cursor marker/highlight applied.
func visibleWindow(items []string, cursor int, maxLines int) []string {
	if len(items) == 0 {
		return nil
	}

	start := 0
	if len(items) > maxLines {
		start = cursor - maxLines/2
		if start < 0 {
			start = 0
		}
		if start > len(items)-maxLines {
			start = len(items) - maxLines
		}
	}

	end := start + maxLines
	if end > len(items) {
		end = len(items)
	}

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		prefix := "  "
		line := items[i]
		if i == cursor {
			prefix = "> "
			line = cursorStyle.Render(line)
		}
		lines = append(lines, prefix+line)
	}
	return lines
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestRenderPanel`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/panel.go internal/tui/panel_test.go
git commit -m "feat: add bordered panel rendering primitive"
```

---

### Task 2: Popup overlay and screen composition

**Files:**
- Create: `internal/tui/layout.go`
- Test: `internal/tui/layout_test.go`

**Interfaces:**
- Consumes: nothing new (pure `lipgloss` usage)
- Produces:
  - `func renderPopupOverlay(content string, width, height int) string`
  - `func composeScreen(sidebar []string, main string, log []string, footer string, logWidth, logHeight int) string`

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"strings"
	"testing"
)

func TestRenderPopupOverlay_ContainsContent(t *testing.T) {
	out := renderPopupOverlay("¿Borrar documento? (y/n)", 80, 24)
	if !strings.Contains(out, "¿Borrar documento? (y/n)") {
		t.Fatalf("expected popup content to appear in overlay, got:\n%s", out)
	}
}

func TestRenderPopupOverlay_FillsRequestedHeight(t *testing.T) {
	out := renderPopupOverlay("hi", 80, 24)
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("expected overlay to be exactly 24 lines tall, got %d", len(lines))
	}
}

func TestComposeScreen_ContainsAllPanelsAndFooter(t *testing.T) {
	sidebar := []string{"PANEL-ONE", "PANEL-TWO"}
	out := composeScreen(sidebar, "MAIN-CONTENT", []string{"log line 1"}, "FOOTER-HINTS", 40, 5)
	for _, want := range []string{"PANEL-ONE", "PANEL-TWO", "MAIN-CONTENT", "log line 1", "FOOTER-HINTS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected composed screen to contain %q, got:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run "TestRenderPopupOverlay|TestComposeScreen"`
Expected: FAIL — `renderPopupOverlay`/`composeScreen` undefined

- [ ] **Step 3: Implement layout.go**

```go
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderPopupOverlay centers content in a bordered box on a blank canvas of
// exactly width x height. See this plan's Global Constraints for why this is
// a blank-canvas popup rather than true compositing over the panels behind
// it.
func renderPopupOverlay(content string, width, height int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusedBorderColor).
		Padding(1, 2).
		Render(content)

	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// composeScreen joins the 5 side panels into a left column, places the main
// panel and command log to its right, and appends the keybinding footer.
// logWidth/logHeight size the command log box; the sidebar/main panels are
// expected to already be pre-rendered (via renderPanel) at their own sizes.
func composeScreen(sidebar []string, main string, log []string, footer string, logWidth, logHeight int) string {
	left := lipgloss.JoinVertical(lipgloss.Left, sidebar...)

	logBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(unfocusedBorderColor).
		Width(logWidth).
		Height(logHeight).
		Render(strings.Join(log, "\n"))

	right := lipgloss.JoinVertical(lipgloss.Left, main, logBox)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestRenderPopupOverlay|TestComposeScreen"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/layout.go internal/tui/layout_test.go
git commit -m "feat: add popup overlay and panel-grid screen composition"
```

---

### Task 3: RootModel focus/popup model — struct additions (no behavior change yet)

**Files:**
- Modify: `internal/tui/root.go`
- Modify: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: nothing new
- Produces:
  - `type panelID int` with consts `panelStatus, panelDatabases, panelCollections, panelIndexes, panelConnections, panelDocuments`
  - `type popupID int` with consts `popupNone, popupDocDetail, popupFieldEdit, popupConfirmWrite, popupDelete, popupHelp`
  - New `RootModel` fields: `focus panelID`, `popup popupID`, `width int`, `height int`, `log []string`
  - `func (m *RootModel) logf(format string, args ...any)` — appends a line to `m.log`, capped at the last 50 entries

This task is purely additive — it does NOT change `Update()`'s routing or `View()`'s rendering yet (that's Task 4). Every existing test must still pass unchanged after this task; it only proves the new fields/types compile and behave correctly in isolation.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/root_test.go`:

```go
func TestRootModel_WindowSizeMsgUpdatesDimensions(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	root := model.(RootModel)
	if root.width != 120 || root.height != 40 {
		t.Fatalf("expected width=120 height=40, got width=%d height=%d", root.width, root.height)
	}
}

func TestRootModel_LogfAppendsAndCapsAt50(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	for i := 0; i < 60; i++ {
		root.logf("line %d", i)
	}
	if len(root.log) != 50 {
		t.Fatalf("expected log capped at 50 entries, got %d", len(root.log))
	}
	if root.log[len(root.log)-1] != "line 59" {
		t.Fatalf("expected most recent entry to be 'line 59', got %q", root.log[len(root.log)-1])
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run "TestRootModel_WindowSizeMsgUpdatesDimensions|TestRootModel_LogfAppendsAndCapsAt50"`
Expected: FAIL — `width`/`height`/`logf` undefined, and `tea.WindowSizeMsg` unhandled (test compiles but first assertion fails with 0/0)

- [ ] **Step 3: Add the new types and fields to root.go**

Find the existing `viewID`/`const` block near the top of `internal/tui/root.go` and the `RootModel` struct. Add, just below the existing `pageSize` constant:

```go
type panelID int

const (
	panelStatus panelID = iota
	panelDatabases
	panelCollections
	panelIndexes
	panelConnections
	panelDocuments
)

type popupID int

const (
	popupNone popupID = iota
	popupDocDetail
	popupFieldEdit
	popupConfirmWrite
	popupDelete
	popupHelp
)
```

Add these fields to the `RootModel` struct (alongside the existing `filter bson.M` field added in the prior fix round):

```go
	focus panelID
	popup popupID

	width  int
	height int
	log    []string
```

Add this method near `pushView`/`popView` (those two functions and the `prevViews` field will be REMOVED in Task 4 — leave them in place for this task, since Task 4 is what actually rewires `Update`/`View`):

```go
// logf appends a formatted line to the command log, keeping only the most
// recent 50 entries.
func (m *RootModel) logf(format string, args ...any) {
	m.log = append(m.log, fmt.Sprintf(format, args...))
	if len(m.log) > 50 {
		m.log = m.log[len(m.log)-50:]
	}
}
```

- [ ] **Step 4: Handle tea.WindowSizeMsg in Update**

In `RootModel.Update`, find the very first lines of the function (the `if keyMsg, ok := msg.(tea.KeyMsg); ok {` block). Add a new case immediately before it:

```go
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height
		return m, nil
	}

```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestRootModel_WindowSizeMsgUpdatesDimensions|TestRootModel_LogfAppendsAndCapsAt50"`
Expected: PASS

- [ ] **Step 6: Run the full suite to confirm nothing broke**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all pre-existing tests still pass unchanged — this task only added inert fields/types.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat: add panel/popup focus model fields to RootModel (no behavior change)"
```

---

### Task 4: Rewrite RootModel Update() and View() for the panel/focus/popup model

**Files:**
- Modify: `internal/tui/root.go` (full rewrite of `Update`, `View`, and related helpers — everything else in the file stays)
- Modify: `internal/tui/root_test.go` (many existing tests assert on the removed `view viewID` field or the removed `viewDatabaseList` etc. consts — these must be rewritten to assert on `focus`/`popup` instead)

**Interfaces:**
- Consumes: `panelID`/`popupID` (Task 3), `renderPanel` (Task 1), `renderPopupOverlay`/`composeScreen` (Task 2), every existing message type and helper from `root.go` (`connectedMsg`, `collectionsLoadedMsg`, `documentsLoadedMsg`, `indexesLoadedMsg`, `docWriteCompletedMsg`, `indexWriteCompletedMsg`, `executePendingWrite`, `loadCollections`, `loadDocuments`, `loadIndexes`, `connectAndListDatabases`, `currentFilter` — all unchanged, all reused)
- Produces: the new `RootModel.Update`/`RootModel.View` — everything downstream (main.go) is unaffected, since `NewRootModel`/`Run`'s signatures don't change

This is the largest task in this plan — it replaces the view-stack navigation
with focus-based routing and popup-overlay rendering. Nothing about *what*
happens on a given message changes (every Mongo call, every confirm-before-write
step, is identical); only *how the screen looks* and *which panel currently
owns the keyboard* changes. Read the CURRENT `internal/tui/root.go` in full
before starting — this task replaces `Update`, `View`, `pushView`, `popView`,
and removes the `viewID`/`view`/`prevViews` machinery entirely, but keeps
every other function (`connectAndListDatabases`, `loadCollections`,
`loadDocuments`, `loadIndexes`, `executePendingWrite`, `statusBar`,
`inTextEntry`, `dispatch`, `Run`) as-is or lightly adapted (noted below).

- [ ] **Step 1: Write the failing tests (rewriting the view-based assertions to focus/popup-based ones)**

Replace `internal/tui/root_test.go` in full with:

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func newTestRootModel() (*RootModel, *mongo.FakeClient) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "total": int32(10)}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)
	return &m, fake
}

func TestRootModel_InitConnectsAndLoadsDatabases(t *testing.T) {
	m, _ := newTestRootModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a command that connects and lists databases")
	}
	msg := cmd()
	newModel, _ := m.Update(msg)
	root := newModel.(RootModel)
	if len(root.dbList.Items) != 1 || root.dbList.Items[0].ID != "shop" {
		t.Fatalf("expected dbList populated with 'shop', got %+v", root.dbList.Items)
	}
}

func TestRootModel_WindowSizeMsgUpdatesDimensions(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	root := model.(RootModel)
	if root.width != 120 || root.height != 40 {
		t.Fatalf("expected width=120 height=40, got width=%d height=%d", root.width, root.height)
	}
}

func TestRootModel_LogfAppendsAndCapsAt50(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	for i := 0; i < 60; i++ {
		root.logf("line %d", i)
	}
	if len(root.log) != 50 {
		t.Fatalf("expected log capped at 50 entries, got %d", len(root.log))
	}
	if root.log[len(root.log)-1] != "line 59" {
		t.Fatalf("expected most recent entry to be 'line 59', got %q", root.log[len(root.log)-1])
	}
}

func TestRootModel_DefaultFocus_WithResolvedConnection_IsDatabases(t *testing.T) {
	m, _ := newTestRootModel()
	if m.focus != panelDatabases {
		t.Fatalf("expected focus=panelDatabases when launched with a resolved connection, got %v", m.focus)
	}
}

func TestRootModel_DefaultFocus_NoArgLaunch_IsConnections(t *testing.T) {
	fake := mongo.NewFakeClient()
	m := NewRootModel(fake, nil)
	if m.focus != panelConnections {
		t.Fatalf("expected focus=panelConnections on no-argument launch, got %v", m.focus)
	}
}

func TestRootModel_NumberKeysSwitchFocus(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	cases := []struct {
		key   string
		panel panelID
	}{
		{"1", panelStatus},
		{"2", panelDatabases},
		{"3", panelCollections},
		{"4", panelIndexes},
		{"5", panelConnections},
	}
	for _, c := range cases {
		model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(c.key)})
		root = model.(RootModel)
		if root.focus != c.panel {
			t.Fatalf("pressing %q: expected focus=%v, got %v", c.key, c.panel, root.focus)
		}
	}
}

func TestRootModel_TabSwitchesFocusToDocuments(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	root := model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("expected focus=panelDocuments after Tab, got %v", root.focus)
	}
}

// rootModelAtCollectionsCursor drives a RootModel to focus=panelDatabases with
// "shop" already loaded via Init(), landing on cursor 0 (the only database).
func rootModelAtDatabasesFocus(t *testing.T) (RootModel, *mongo.FakeClient) {
	t.Helper()
	m, fake := newTestRootModel()
	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	root = model.(RootModel)
	return root, fake
}

func TestRootModel_CursorMoveInDatabasesCascadesToCollections(t *testing.T) {
	root, fake := rootModelAtDatabasesFocus(t)
	fake.Databases["shop"]["users"] = []bson.M{{"_id": "u1"}}

	// cursor is already on "shop" (the only database) after Init; moving down
	// and back up forces a cursor-change event even with a single item by
	// instead asserting the cascade already happened as part of landing here
	// — simulate a real cascade by checking a fresh cursor move triggers reload
	// via a second database.
	fake.Databases["admin"] = map[string][]bson.M{}
	model, _ := root.Update(m0Init(t))
	root = model.(RootModel)

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command (collections load) after moving cursor to a new database")
	}
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if len(root.collList.Items) != 2 {
		t.Fatalf("expected 2 collections loaded for 'shop', got %+v", root.collList.Items)
	}
}

// m0Init re-runs Init's connect+list-databases cycle so a second database
// ("admin", added after the initial connect) is reflected in dbList.
func m0Init(t *testing.T) tea.Msg {
	t.Helper()
	return connectedMsg{Databases: []string{"admin", "shop"}}
}

func TestRootModel_PopupHelpTogglesOnQuestionMark(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	root := model.(RootModel)
	if root.popup != popupHelp {
		t.Fatalf("expected popup=popupHelp after '?', got %v", root.popup)
	}
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	root = model.(RootModel)
	if root.popup != popupNone {
		t.Fatalf("expected any key to close help popup, got popup=%v", root.popup)
	}
}

func TestRootModel_QuitsOnCtrlC(t *testing.T) {
	m, _ := newTestRootModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a quit command on Ctrl+C")
	}
}

func TestRootModel_ViewShowsConnectionNameInStatusBar(t *testing.T) {
	m, _ := newTestRootModel()
	view := m.View()
	if !strings.Contains(view, "qa") {
		t.Fatalf("expected status panel to show connection name 'qa', got view:\n%s", view)
	}
}

func TestRootModel_ErrorClearsOnAnyKeypress(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.err = fmt.Errorf("boom")
	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	root = model.(RootModel)
	if root.err != nil {
		t.Fatalf("expected err to be cleared after a keypress, got %v", root.err)
	}
}

func TestRootModel_DocumentEnterOpensDocDetailPopup(t *testing.T) {
	root, _ := rootModelAtDatabasesFocus(t)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	root = model.(RootModel)
	if cmd != nil {
		model, _ = root.Update(cmd())
		root = model.(RootModel)
	}
	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	root = model.(RootModel)
	// land on collections, cascade to documents, then focus documents and open detail
	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	_ = cmd
	_ = root
}
```

**Note on the last test (`TestRootModel_DocumentEnterOpensDocDetailPopup`):** this scaffold is intentionally loose because the exact cascade sequence depends on decisions you'll make while implementing Step 3 below (e.g. exactly when collections/documents load). Rewrite this test's body once your implementation is working, so it actually drives: focus databases (cursor already on "shop") → focus collections (cursor already on "orders", which should have cascaded documents already since Task's cascade fires on cursor movement OR initial population — decide and document which, then assert `root.popup == popupDocDetail` after pressing `Enter` while `root.focus == panelDocuments`. Do not leave the loose scaffold in the final commit — it must assert a real, specific outcome.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run TestRootModel`
Expected: FAIL — `panelStatus` etc. used in ways the current `Update`/`View` don't yet support (focus never changes on number keys, popup never opens, etc.)

- [ ] **Step 3: Rewrite Update() and View()**

Read the current `internal/tui/root.go` fully first — you are replacing `Update`, `View`, `pushView`, `popView`, and the `view`/`prevViews` fields (removing them), while keeping every message-type case's *business logic* (the actual Mongo calls, confirm-before-write flow) unchanged. Below is the target shape for the parts that change; **the `connectAndListDatabases`, `loadCollections`, `loadDocuments`, `loadIndexes`, `executePendingWrite`, `statusBar`, `dispatch`, `Run`, `currentFilter`, `connectedMsg`, `collectionsLoadedMsg`, `documentsLoadedMsg`, `indexesLoadedMsg`, `docWriteCompletedMsg`, `indexWriteCompletedMsg` types/functions all stay exactly as they are today — do not rewrite them, just call them from the new `Update`.**

Remove the `view viewID` and `prevViews []viewID` fields from `RootModel` (replaced by `focus panelID` and `popup popupID`, already added in Task 3). Remove `pushView`/`popView` entirely — popups no longer change `focus`, so there's nothing to push/pop.

Update `inTextEntry` to check the new fields:

```go
// inTextEntry reports whether the current focus/popup is in an active
// text-entry sub-state, where printable keys like "?" must be typed
// literally rather than treated as a global shortcut.
func (m RootModel) inTextEntry() bool {
	switch {
	case m.focus == panelConnections && m.connPicker.creating:
		return true
	case m.focus == panelDocuments && m.docList.filtering:
		return true
	case m.popup == popupFieldEdit && !m.fieldEdit.confirming:
		return true
	case m.focus == panelIndexes && m.idxList.creating:
		return true
	}
	return false
}
```

Replace `NewRootModel`:

```go
// NewRootModel builds the root model. If resolved is nil, focus starts on
// the Conexiones panel so the user can pick a saved connection; otherwise
// focus starts on Databases since the connection is already known.
func NewRootModel(client mongo.Client, resolved *config.Connection) RootModel {
	m := RootModel{client: client}
	if resolved != nil {
		m.conn = *resolved
		m.focus = panelDatabases
	} else {
		m.focus = panelConnections
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
		} else {
			m.connPicker = newConnectionPickerModel(conns)
		}
	}
	return m
}
```

`Init()` stays as-is (still calls `m.connectAndListDatabases()` only when a connection was already resolved — but it now checks `m.focus` instead of `m.view`):

```go
func (m RootModel) Init() tea.Cmd {
	if m.focus != panelDatabases {
		return nil
	}
	return m.connectAndListDatabases()
}
```

Replace `Update`. The overall shape: (1) handle `tea.WindowSizeMsg` first (from Task 3, keep as-is); (2) handle global keys (Ctrl+C, error-dismiss, help toggle, number/Tab focus switching) — only when no popup is blocking them; (3) the EXISTING top-level `switch msg := msg.(type)` for async results and cross-cutting messages stays, with every `m.view = viewX` / `m.pushView(viewX)` / `m.popView()` line replaced by the equivalent `m.popup = popupX` / `m.popup = popupNone` (no `focus` changes here — loading data into an already-visible panel doesn't move focus); (4) if a popup is open, route to it; (5) otherwise route to whichever panel is focused, including the new cascade-on-cursor-move logic for Databases/Collections.

```go
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.err != nil {
			m.err = nil
			return m, nil
		}
		if keyMsg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.popup == popupHelp {
			m.popup = popupNone
			return m, nil
		}
		if keyMsg.String() == "?" && !m.inTextEntry() {
			m.popup = popupHelp
			return m, nil
		}
		if m.popup == popupNone {
			switch keyMsg.String() {
			case "1":
				m.focus = panelStatus
				return m, nil
			case "2":
				m.focus = panelDatabases
				return m, nil
			case "3":
				m.focus = panelCollections
				return m, nil
			case "4":
				m.focus = panelIndexes
				return m, nil
			case "5":
				m.focus = panelConnections
				return m, nil
			case "tab":
				m.focus = panelDocuments
				return m, nil
			}
		}
	}

	switch msg := msg.(type) {
	case connectedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.dbList = newDatabaseListModel(msg.Databases)
		m.logf("Conectado a %s", m.conn.Name)
		return m, nil

	case connectionChosenMsg:
		conn, err := config.ResolveConnection(msg.Conn.Name)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.conn = conn
		m.focus = panelDatabases
		return m, m.connectAndListDatabases()

	case connectionCreatedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		return m, nil

	case connectionCreateErrMsg:
		m.err = msg.Err
		return m, nil

	case collectionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.collList = newCollectionListModel(msg.Collections)
		return m, nil

	case documentsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.docList = newDocListModel(msg.Docs, msg.Total, m.page, pageSize)
		return m, nil

	case indexesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.idxList = newIdxListModel(msg.Indexes)
		return m, nil

	case fieldSelectedMsg:
		m.fieldEdit = newFieldEditModel(msg.Field, msg.Value)
		m.popup = popupFieldEdit
		return m, nil

	case deleteRequestedMsg:
		id := m.docDetail.doc["_id"]
		m.delete = newDeleteFlowModel(id)
		m.popup = popupDelete
		return m, nil

	case deleteConfirmedMsg:
		id := m.docDetail.doc["_id"]
		client, db, coll := m.client, m.db, m.coll
		m.popup = popupNone
		m.logf("Borrando documento %v", id)
		return m, func() tea.Msg {
			err := client.DeleteOne(context.Background(), db, coll, id)
			return docWriteCompletedMsg{Err: err}
		}

	case fieldUpdateConfirmedMsg:
		client, db, coll, id := m.client, m.db, m.coll, m.docDetail.doc["_id"]
		field, value := msg.Field, msg.NewValue
		m.popup = popupNone
		m.logf("Actualizando campo %q del documento %v", field, id)
		return m, func() tea.Msg {
			err := client.UpdateField(context.Background(), db, coll, id, field, value)
			return docWriteCompletedMsg{Err: err}
		}

	case insertRequestedMsg:
		m.editMode = "insert"
		return m, startEditFullFlow(bson.M{})

	case editFullRequestedMsg:
		m.editMode = "edit"
		return m, startEditFullFlow(m.docDetail.doc)

	case editFullDoneMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.pendingDoc = msg.Doc
		m.confirmWrite = confirmModel{Message: "¿Guardar estos cambios en Mongo?"}
		m.popup = popupConfirmWrite
		return m, nil

	case switchToIndexesMsg:
		return m, m.loadIndexes()

	case indexCreateSubmittedMsg:
		m.pendingIndexKeysJSON = msg.KeysJSON
		m.pendingIndexUnique = msg.Unique
		m.confirmWrite = confirmModel{Message: "¿Crear este índice?"}
		m.popup = popupConfirmWrite
		return m, nil

	case indexDropConfirmedMsg:
		client, db, coll, name := m.client, m.db, m.coll, msg.Name
		m.logf("Borrando índice %q", name)
		return m, func() tea.Msg {
			err := client.DropIndex(context.Background(), db, coll, name)
			return indexWriteCompletedMsg{Err: err}
		}

	case docWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.pendingDoc = nil
		return m, m.loadDocuments(m.currentFilter())

	case indexWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadIndexes()

	case listBackMsg:
		// with no view stack, "back" just closes whatever popup is open.
		m.popup = popupNone
		return m, nil
	}

	if m.popup != popupNone {
		var cmd tea.Cmd
		switch m.popup {
		case popupDocDetail:
			var docCmd tea.Cmd
			m.docDetail, docCmd = m.docDetail.Update(msg)
			if docCmd != nil {
				return m.dispatch(docCmd())
			}
		case popupFieldEdit:
			var feCmd tea.Cmd
			m.fieldEdit, feCmd = m.fieldEdit.Update(msg)
			if feCmd != nil {
				return m.dispatch(feCmd())
			}
		case popupDelete:
			var delCmd tea.Cmd
			m.delete, delCmd = m.delete.Update(msg)
			if delCmd != nil {
				return m.dispatch(delCmd())
			}
		case popupConfirmWrite:
			var cwCmd tea.Cmd
			m.confirmWrite, cwCmd = m.confirmWrite.Update(msg)
			if cwCmd != nil {
				result, ok := cwCmd().(confirmResultMsg)
				if !ok {
					return m, cwCmd
				}
				m.popup = popupNone
				if !result.Confirmed {
					m.pendingDoc = nil
					m.pendingIndexKeysJSON = ""
					m.pendingIndexUnique = false
					return m, nil
				}
				return m.executePendingWrite()
			}
		}
		return m, cmd
	}

	switch m.focus {
	case panelDatabases:
		before := m.dbList.Cursor
		var listCmd tea.Cmd
		m.dbList, listCmd = m.dbList.Update(msg)
		if m.dbList.Cursor != before && len(m.dbList.Items) > 0 {
			m.db = m.dbList.Items[m.dbList.Cursor].ID
			return m, m.loadCollections()
		}
		return m, listCmd

	case panelCollections:
		before := m.collList.Cursor
		var listCmd tea.Cmd
		m.collList, listCmd = m.collList.Update(msg)
		if m.collList.Cursor != before && len(m.collList.Items) > 0 {
			m.coll = m.collList.Items[m.collList.Cursor].ID
			m.page = 0
			m.filter = nil
			return m, tea.Batch(m.loadIndexes(), m.loadDocuments(bson.M{}))
		}
		return m, listCmd

	case panelIndexes:
		var idxCmd tea.Cmd
		m.idxList, idxCmd = m.idxList.Update(msg)
		if idxCmd != nil {
			return m.dispatch(idxCmd())
		}
		return m, nil

	case panelConnections:
		var cmd tea.Cmd
		m.connPicker, cmd = m.connPicker.Update(msg)
		return m, cmd

	case panelDocuments:
		var cmd tea.Cmd
		m.docList, cmd = m.docList.Update(msg)
		if cmd != nil {
			switch out := cmd().(type) {
			case pageChangedMsg:
				m.page = out.Page
				return m, m.loadDocuments(m.currentFilter())
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
			case documentChosenMsg:
				m.docDetail = newDocDetailModel(out.Doc)
				m.popup = popupDocDetail
				return m, nil
			case insertRequestedMsg:
				m.editMode = "insert"
				return m, startEditFullFlow(bson.M{})
			case switchToIndexesMsg:
				m.focus = panelIndexes
				return m, m.loadIndexes()
			}
			return m, cmd
		}
	}
	return m, nil
}
```

Note the `panelDocuments` branch handles `insertRequestedMsg`/`switchToIndexesMsg` INLINE (unwrapping `cmd()` directly) rather than relying on those message types being re-dispatched through the top-level `switch msg := msg.(type)` — this is because, unlike v1, the top-level switch no longer has cases for `insertRequestedMsg`/`switchToIndexesMsg` (they were folded into the `panelDocuments` branch here since that's their only source). If you find it cleaner to keep them as top-level cases instead (matching how `editFullRequestedMsg` etc. remain top-level), that's an acceptable equivalent restructuring — just don't handle the same message in two places.

Now replace `View()`:

```go
func (m RootModel) View() string {
	if m.err != nil {
		return renderPopupOverlay(fmt.Sprintf("Error: %v\n\n[cualquier tecla] continuar", m.err), m.width, m.height)
	}

	switch m.popup {
	case popupDocDetail:
		return renderPopupOverlay(m.docDetail.View(), m.width, m.height)
	case popupFieldEdit:
		return renderPopupOverlay(m.fieldEdit.View(), m.width, m.height)
	case popupConfirmWrite:
		return renderPopupOverlay(m.confirmWrite.View(), m.width, m.height)
	case popupDelete:
		return renderPopupOverlay(m.delete.View(), m.width, m.height)
	case popupHelp:
		return renderPopupOverlay(helpModel{}.View(), m.width, m.height)
	}

	if m.focus == panelConnections && m.connPicker.creating {
		return renderPopupOverlay(m.connPicker.View(), m.width, m.height)
	}
	if m.focus == panelIndexes && (m.idxList.creating || m.idxList.confirmingDrop) {
		return renderPopupOverlay(m.idxList.View(), m.width, m.height)
	}

	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 30
	}

	sidebarWidth := width / 3
	if sidebarWidth < 24 {
		sidebarWidth = 24
	}
	mainWidth := width - sidebarWidth - 2
	if mainWidth < 30 {
		mainWidth = 30
	}
	panelHeight := 5

	statusLines := []string{fmt.Sprintf("%s.%s", m.db, m.coll)}
	p1 := renderPanel(1, "Status", statusLines, 0, m.focus == panelStatus, sidebarWidth, panelHeight)
	p2 := renderPanel(2, "Databases", labelsFromListModel(m.dbList), m.dbList.Cursor, m.focus == panelDatabases, sidebarWidth, panelHeight)
	p3 := renderPanel(3, "Collections", labelsFromListModel(m.collList), m.collList.Cursor, m.focus == panelCollections, sidebarWidth, panelHeight)
	p4 := renderPanel(4, "Indexes", labelsFromIndexes(m.idxList.indexes), m.idxList.cursor, m.focus == panelIndexes, sidebarWidth, panelHeight)
	p5 := renderPanel(5, "Conexiones", labelsFromListModel(m.connPicker.list), m.connPicker.list.Cursor, m.focus == panelConnections, sidebarWidth, panelHeight)

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

	return composeScreen([]string{p1, p2, p3, p4, p5}, main, lastLogLines(m.log, 4), footer, mainWidth, 4)
}

// labelsFromListModel renders each item's colored label as a plain line for
// panel display (no cursor/border chrome — renderPanel adds that).
func labelsFromListModel(m listModel) []string {
	labels := make([]string, len(m.Items))
	for i, item := range m.Items {
		labels[i] = colorStyle(item.Color).Render(item.Label)
	}
	return labels
}

func labelsFromIndexes(indexes []mongo.IndexInfo) []string {
	labels := make([]string, len(indexes))
	for i, idx := range indexes {
		unique := ""
		if idx.Unique {
			unique = " (unique)"
		}
		labels[i] = fmt.Sprintf("%s %v%s", idx.Name, idx.Key, unique)
	}
	return labels
}

func labelsFromDocs(docs []bson.M) []string {
	labels := make([]string, len(docs))
	for i, doc := range docs {
		labels[i] = fmt.Sprintf("%v", doc["_id"])
	}
	return labels
}

// lastLogLines returns at most n of the most recent log entries.
func lastLogLines(log []string, n int) []string {
	if len(log) <= n {
		return log
	}
	return log[len(log)-n:]
}
```

This reads private fields of `docListModel` (`docList.total`, `.page`, `.filtering`, `.filter`, `.docs`, `.cursor`) and `idxListModel` (`.indexes`, `.cursor`, `.creating`, `.confirmingDrop`) directly — safe and intentional, since `panel.go`/`layout.go`/`root.go` are all in the same `tui` package. Do not add exported getters for these; that would be unnecessary surface area for something only ever called from within the package.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestRootModel`
Expected: PASS — fix the loose scaffold test from Step 1 now that the real cascade/popup behavior exists, asserting a specific, real outcome (not a placeholder).

- [ ] **Step 5: Run the full suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all tests pass across `internal/config`, `internal/mongo`, `internal/tui`.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat: replace view-stack navigation with panel focus + popup overlay model"
```

---

### Task 5: Update help text and README for the new keybindings

**Files:**
- Modify: `internal/tui/help.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: nothing
- Produces: nothing new — text-only changes

- [ ] **Step 1: Update the help text**

In `internal/tui/help.go`, replace `helpText`'s content with:

```go
const helpText = `lazymongo — atajos

1-5            saltar a un panel (Status/Databases/Collections/Indexes/Conexiones)
Tab            saltar al panel de Documentos
j/k, flechas   moverse dentro del panel enfocado
Enter          ver documento / conectar (en Conexiones) / entrar
Esc            cerrar el popup activo
/              filtrar (en Documentos)
n/p            página siguiente/anterior (en Documentos)
i, a           insertar documento / crear conexión o índice
e              editar campo (en detalle de documento)
E              editar documento completo en $EDITOR
d, x           borrar (siempre pide confirmación)
?              esta ayuda
Ctrl+c         salir

Presiona cualquier tecla para cerrar esta ayuda.`
```

- [ ] **Step 2: Update README's keybindings table**

In `README.md`, replace the "Keybindings" table with:

```markdown
## Keybindings

| Key | Action |
|---|---|
| `1`-`5` | jump to a panel (Status/Databases/Collections/Indexes/Conexiones) |
| `Tab` | jump to the Documents panel |
| `j`/`k`, arrows | move within the focused panel |
| `Enter` | view document / connect (in Conexiones) / enter |
| `Esc` | close the active popup |
| `/` | filter documents |
| `n`/`p` | next/previous page |
| `i`, `a` | insert document / create connection or index |
| `e` | edit field inline |
| `E` | edit full document in `$EDITOR` |
| `d`, `x` | delete (always confirms) |
| `?` | help |
| `Ctrl+c` | quit |

Moving the cursor in the Databases/Collections panels immediately loads the
next panel's contents (live preview) — no `Enter` needed. Connecting from the
Conexiones panel still requires `Enter`, since it's a real network connect.
```

- [ ] **Step 3: Verify the build**

Run: `go build -o lazymongo . && go test ./... -count=1`
Expected: builds cleanly, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/help.go README.md
git commit -m "docs: update help text and README for the panel-based UI"
```

---

### Task 6: Manual smoke test against `qa` (redo, since the whole UI changed)

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: everything built in Tasks 1-5
- Produces: a documented, manually-verified working binary with the new UI

- [ ] **Step 1: Add a note to the manual smoke test section**

In `README.md`'s existing "Manual smoke test against qa" section, add this line at the top:

```markdown
**Note:** the UI changed from a full-screen drill-down to a persistent
5-panel + Documents-panel layout (see the design spec
`docs/superpowers/specs/2026-07-11-lazymongo-lazygit-ui-design.md`). Redo this
checklist even if you validated the drill-down version before — panel focus,
popups, and live-preview cascading are all new interaction paths.
```

- [ ] **Step 2: Manual smoke test against `qa` (do this by hand, not automated)**

Run: `go build -o lazymongo . && ./lazymongo qa`

Walk through, confirming each works as expected:
- [ ] All 5 side panels + Documents panel render on launch, Status panel shows `qa` in its assigned color
- [ ] `2` focuses Databases; `j`/`k` moves the cursor; Collections panel live-updates as you move
- [ ] `3` focuses Collections; moving the cursor live-updates Indexes and Documents
- [ ] `Tab` focuses Documents; `/` filters, `n`/`p` paginate
- [ ] `Enter` on a document opens the detail popup; `Esc` closes it, panels underneath are unchanged
- [ ] `e` on a field in the detail popup opens the inline editor; confirming actually updates the field (verify with `mongosh`/`mgo qa` afterward)
- [ ] `E` opens the full document in `nvim`; saving+exiting shows the confirm popup, then replaces it
- [ ] `i` inserts a new document after confirmation
- [ ] `d` on a document opens the delete confirmation popup
- [ ] `4` focuses Indexes; `a` opens the create-index popup, `d` drops one (both confirm first)
- [ ] `5` focuses Conexiones; `Enter` on a different saved connection reconnects
- [ ] `?` opens/closes help; `Ctrl+c` quits cleanly
- [ ] The command log shows connect/filter/write history

Only after this passes should `prod` be used with `lazymongo`.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: note UI redesign in manual smoke test checklist"
```
