# lazymongo — Connection form cursor editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `←`/`→` cursor movement to the Name and URI fields of the connection form (shared by "new connection" and "edit connection" in `[5]` Conexiones).

**Architecture:** `connectionForm` gains a `cursor int` (rune index) for whichever of Name/URI is active, with insert-at-cursor/backspace-at-cursor/arrow-clamp logic following the exact shape already shipped for the Mongo filter's `filterCursor` in `internal/tui/document_list.go`. The Color field's existing `h`/`l`/arrow cycling is untouched.

**Tech Stack:** Go, `github.com/charmbracelet/bubbletea` (already a dependency).

## Global Constraints

- No auto-close of `{`/`"` — a connection name or URI is not JSON. This plan is arrow-key movement only.
- The Color field (`field == 2`) is completely unaffected: its existing `h`/`l`/`left`/`right` color-cycling code path is untouched.
- Switching fields with `Tab`/`Shift+Tab` moves the cursor to the **end** of whichever field (Name or URI) becomes active — not to 0.
- `cursor` is a rune index — every edit must go through `[]rune(...)`, never byte-slice a string directly, to avoid splitting multi-byte UTF-8 characters.
- The edit form (`locked = true`) must keep only ever landing on field 1 (URI) or 2 (Color) when tabbing — field 0 (Name) stays unreachable, exactly as already shipped.

---

### Task 1: Cursor-based editing for Name/URI in connectionForm

**Files:**
- Modify: `internal/tui/connection_picker.go`
- Test: `internal/tui/connection_picker_test.go`

**Interfaces:**
- No new interfaces consumed by other files — `connectionForm.cursor` and its helper methods are used only within `connection_picker.go` itself (by `update` and `View`).

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/connection_picker_test.go`:

```go
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

func TestConnectionForm_LockedModeTabToURIStartsCursorAtEnd(t *testing.T) {
	f := newEditConnectionForm(config.Connection{Name: "qa", URI: "mongodb://qa", Color: "verde"})
	if f.cursor != len([]rune("mongodb://qa")) {
		t.Fatalf("expected edit form to start with cursor at end of URI, got %d", f.cursor)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	if f.cursor != len([]rune("mongodb://qa")) {
		t.Fatalf("expected cursor back at end of URI after cycling through Color, got %d", f.cursor)
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/tui/... -v -run "TestConnectionForm_TypingInsertsAtCursor|TestConnectionForm_BackspaceRemovesRuneBeforeCursor|TestConnectionForm_ArrowsMoveAndClampCursorInNameField|TestConnectionForm_TabMovesCursorToEndOfNewlyActiveField|TestConnectionForm_LockedModeTabToURIStartsCursorAtEnd|TestConnectionForm_ColorFieldCyclingUnaffectedByCursorChanges|TestConnectionForm_InsertAndBackspaceHandleMultiByteRunesSafely|TestConnectionPicker_ViewShowsCursorMarkerAtRealPositionInActiveField"`
Expected: FAIL — `f.cursor` undefined, `tea.KeyLeft`/`tea.KeyRight` unhandled, no cursor marker rendered

- [ ] **Step 3: Implement cursor-based editing**

In `internal/tui/connection_picker.go`, replace:

```go
type connectionForm struct {
	name   string
	uri    string
	color  string
	field  int  // 0=name, 1=uri, 2=color
	locked bool // when true, Name is read-only and Tab only cycles URI/Color
}

func newConnectionForm() connectionForm {
	return connectionForm{color: colorChoices[0]}
}

// newEditConnectionForm pre-fills the form with an existing connection's
// current values, locking the Name field (the array key can't be renamed
// through this form) and starting focus on URI.
func newEditConnectionForm(conn config.Connection) connectionForm {
	return connectionForm{name: conn.Name, uri: conn.URI, color: conn.Color, field: 1, locked: true}
}

func (f connectionForm) update(msg tea.KeyMsg) connectionForm {
	switch msg.String() {
	case "tab":
		if f.locked {
			if f.field == 1 {
				f.field = 2
			} else {
				f.field = 1
			}
		} else {
			f.field = (f.field + 1) % 3
		}
		return f
	case "shift+tab":
		if f.locked {
			if f.field == 1 {
				f.field = 2
			} else {
				f.field = 1
			}
		} else {
			f.field = (f.field + 2) % 3
		}
		return f
	}

	if f.field == 2 {
		switch msg.String() {
		case "l", "right":
			f.color = nextColor(f.color, 1)
		case "h", "left":
			f.color = nextColor(f.color, -1)
		}
		return f
	}

	switch msg.Type {
	case tea.KeyBackspace:
		if f.field == 0 {
			if f.locked {
				return f
			}
			if r := []rune(f.name); len(r) > 0 {
				f.name = string(r[:len(r)-1])
			}
		} else if f.field == 1 {
			if r := []rune(f.uri); len(r) > 0 {
				f.uri = string(r[:len(r)-1])
			}
		}
	case tea.KeyRunes:
		text := string(msg.Runes)
		if f.field == 0 {
			if f.locked {
				return f
			}
			f.name += text
		} else if f.field == 1 {
			f.uri += text
		}
	}
	return f
}
```

with:

```go
type connectionForm struct {
	name   string
	uri    string
	color  string
	field  int  // 0=name, 1=uri, 2=color
	locked bool // when true, Name is read-only and Tab only cycles URI/Color
	cursor int  // rune index within whichever of Name/URI (field 0 or 1) is active
}

func newConnectionForm() connectionForm {
	return connectionForm{color: colorChoices[0]}
}

// newEditConnectionForm pre-fills the form with an existing connection's
// current values, locking the Name field (the array key can't be renamed
// through this form) and starting focus on URI, with the cursor at the end
// of the pre-filled URI (ready to keep typing, like tabbing into a normal
// form input).
func newEditConnectionForm(conn config.Connection) connectionForm {
	return connectionForm{
		name: conn.Name, uri: conn.URI, color: conn.Color,
		field: 1, locked: true, cursor: len([]rune(conn.URI)),
	}
}

func (f connectionForm) update(msg tea.KeyMsg) connectionForm {
	switch msg.String() {
	case "tab":
		if f.locked {
			if f.field == 1 {
				f.field = 2
			} else {
				f.field = 1
			}
		} else {
			f.field = (f.field + 1) % 3
		}
		f.cursor = len([]rune(f.activeFieldText()))
		return f
	case "shift+tab":
		if f.locked {
			if f.field == 1 {
				f.field = 2
			} else {
				f.field = 1
			}
		} else {
			f.field = (f.field + 2) % 3
		}
		f.cursor = len([]rune(f.activeFieldText()))
		return f
	}

	if f.field == 2 {
		switch msg.String() {
		case "l", "right":
			f.color = nextColor(f.color, 1)
		case "h", "left":
			f.color = nextColor(f.color, -1)
		}
		return f
	}

	switch msg.Type {
	case tea.KeyLeft:
		if f.cursor > 0 {
			f.cursor--
		}
	case tea.KeyRight:
		if f.cursor < len([]rune(f.activeFieldText())) {
			f.cursor++
		}
	case tea.KeyBackspace:
		if f.field == 0 && f.locked {
			return f
		}
		f.deleteBeforeCursor()
	case tea.KeyRunes:
		if f.field == 0 && f.locked {
			return f
		}
		f.insertAtCursor(string(msg.Runes))
	}
	return f
}

// activeFieldText returns the text of whichever field (Name or URI) is
// currently focused. Only meaningful when field is 0 or 1 — the Color
// field (2) has no text/cursor concept and never calls this.
func (f connectionForm) activeFieldText() string {
	if f.field == 0 {
		return f.name
	}
	return f.uri
}

// setActiveFieldText writes newText back to whichever field (Name or URI)
// is currently focused.
func (f *connectionForm) setActiveFieldText(newText string) {
	if f.field == 0 {
		f.name = newText
	} else {
		f.uri = newText
	}
}

// insertAtCursor inserts text into the active field at f.cursor (a rune
// index, never a byte index — this always goes through []rune to avoid
// splitting multi-byte UTF-8 characters), then advances the cursor past
// the inserted text.
func (f *connectionForm) insertAtCursor(text string) {
	runes := []rune(f.activeFieldText())
	inserted := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(inserted))
	newRunes = append(newRunes, runes[:f.cursor]...)
	newRunes = append(newRunes, inserted...)
	newRunes = append(newRunes, runes[f.cursor:]...)
	f.setActiveFieldText(string(newRunes))
	f.cursor += len(inserted)
}

// deleteBeforeCursor removes the rune immediately before f.cursor in the
// active field, if any.
func (f *connectionForm) deleteBeforeCursor() {
	if f.cursor == 0 {
		return
	}
	runes := []rune(f.activeFieldText())
	newRunes := make([]rune, 0, len(runes)-1)
	newRunes = append(newRunes, runes[:f.cursor-1]...)
	newRunes = append(newRunes, runes[f.cursor:]...)
	f.setActiveFieldText(string(newRunes))
	f.cursor--
}

// textBeforeCursor and textAfterCursor split the active field's text at
// the cursor, for rendering the cursor-blink marker at its real position
// (View() uses these only when field is 0 or 1).
func (f connectionForm) textBeforeCursor() string {
	runes := []rune(f.activeFieldText())
	if f.cursor > len(runes) {
		return f.activeFieldText()
	}
	return string(runes[:f.cursor])
}

func (f connectionForm) textAfterCursor() string {
	runes := []rune(f.activeFieldText())
	if f.cursor > len(runes) {
		return ""
	}
	return string(runes[f.cursor:])
}
```

Then in `connectionPickerModel.View()`, replace:

```go
		b.WriteString("Nombre: " + m.form.name)
		if m.form.field == 0 {
			b.WriteString(" <")
		}
		b.WriteString("\nURI:    " + m.form.uri)
		if m.form.field == 1 {
			b.WriteString(" <")
		}
```

with:

```go
		nameText := m.form.name
		if m.form.field == 0 {
			nameText = m.form.textBeforeCursor() + "_" + m.form.textAfterCursor()
		}
		b.WriteString("Nombre: " + nameText)
		if m.form.field == 0 {
			b.WriteString(" <")
		}
		uriText := m.form.uri
		if m.form.field == 1 {
			uriText = m.form.textBeforeCursor() + "_" + m.form.textAfterCursor()
		}
		b.WriteString("\nURI:    " + uriText)
		if m.form.field == 1 {
			b.WriteString(" <")
		}
```

(The `Color:` line right after is unchanged.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestConnectionForm_TypingInsertsAtCursor|TestConnectionForm_BackspaceRemovesRuneBeforeCursor|TestConnectionForm_ArrowsMoveAndClampCursorInNameField|TestConnectionForm_TabMovesCursorToEndOfNewlyActiveField|TestConnectionForm_LockedModeTabToURIStartsCursorAtEnd|TestConnectionForm_ColorFieldCyclingUnaffectedByCursorChanges|TestConnectionForm_InsertAndBackspaceHandleMultiByteRunesSafely|TestConnectionPicker_ViewShowsCursorMarkerAtRealPositionInActiveField"`
Expected: PASS (all 8 new tests)

- [ ] **Step 5: Run the full connection_picker test file to check for regressions**

Run: `go test ./internal/tui/... -v -run "TestConnectionPicker|TestConnectionForm"`
Expected: PASS — every pre-existing test still passes, in particular `TestConnectionPicker_CreateFormSubmitsNewConnection` (types into Name/URI, cycles Color, submits — must still work with cursor-based typing appending at the end when the cursor is already there) and `TestConnectionPicker_EnterInEditModeReturnsACommand` (types one character into a pre-filled URI, expecting it appended at the end — the edit form's initial cursor position, now at the end of the pre-filled URI, keeps this passing unmodified)

- [ ] **Step 6: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all packages build, vet is clean, all tests pass (pre-existing and new)

- [ ] **Step 7: Commit**

```bash
git add internal/tui/connection_picker.go internal/tui/connection_picker_test.go
git commit -m "feat: add cursor-based editing (arrow keys) to the connection form's Name/URI fields"
```

---

## Final manual smoke test

Run: `go build -o lazymongo . && ./lazymongo`, then in `[5]` Conexiones:
- `a` (new connection): type a name, use `←`/`→` to move within it and insert/delete in the middle; `Tab` to URI, same behavior; `Tab` to Color, `h`/`l` still cycles colors normally
- `e` (edit an existing connection): the URI field starts with the cursor at the end of the current value, ready to keep typing or move around with `←`/`→`; the Name field still cannot be typed into
- Accented characters (e.g. "ó") insert and delete correctly without corrupting the field
