# Editable Connection Name (Rename Support) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Name field editable when editing an existing connection (not just when creating one), with real rename support that moves the connection's entries to a new array key in `~/.config/mongo-connections.sh`.

**Architecture:** A new `RenameConnection(oldName string, conn Connection) error` in `internal/config/writer.go` handles moving a connection's entries to a new key across both zsh associative arrays, rejecting collisions with a different existing connection. `connectionForm` in `internal/tui/connection_picker.go` drops its `locked` field entirely, so Name is editable in both create and edit modes. `connectionPickerModel` remembers the name a connection had when editing started (`editingOriginalName`) and picks `RenameConnection` vs the existing `UpdateConnection` at submit time based on whether Name changed. `root.go`'s `connectionUpdatedMsg` handler repositions the list cursor onto the edited/renamed connection after refreshing the list.

**Tech Stack:** Go, Bubbletea, zsh (via `exec.Command("zsh", ...)`), Go's standard `testing` package.

## Global Constraints

- Renaming to a name that already belongs to a **different** existing connection is rejected with an error, and the connections file is left byte-for-byte unchanged (no partial write). Renaming to the same name (no-op rename) is not an error.
- `connectionForm` loses its `locked` field entirely. Name is editable via the same insert-at-cursor/backspace-before-cursor mechanics already used for URI, in both create and edit forms. `Tab`/`Shift+Tab` always cycle `0 → 1 → 2` (Name → URI → Color) and back, unconditionally — no more locked-mode two-state cycle.
- The edit form (`newEditConnectionForm`) starts focus on field 0 (Name), with the cursor at the end of the pre-filled Name — matching `newConnectionForm`'s default start point.
- `RenameConnection` must wrap URI/Color values with the existing `zshSingleQuote` helper, never `fmt.Sprintf("%q", ...)` — `%q` does not neutralize zsh's `$(...)`/backtick command substitution (see `zshSingleQuote`'s doc comment in `internal/config/writer.go` for the exploit history).
- `AddConnection`, `UpdateConnection`, `DeleteConnection`, `insertIntoArray`, `replaceOrInsertInArray`, and `removeFromArray` in `internal/config/writer.go` are not modified by this plan — only a new `RenameConnection` and a new `arrayHasKey` helper are added.
- The list-cursor-follows-the-connection fix applies only to the `connectionUpdatedMsg` handler in `internal/tui/root.go`. `connectionCreatedMsg` and `connectionDeletedMsg` keep resetting the cursor to 0 — that is out of scope for this plan.
- The three existing tests in `internal/tui/connection_picker_test.go` that assert the removed `locked` behavior (`TestConnectionForm_NameFieldIsReadOnlyWhenLocked`, `TestConnectionForm_TabInLockedModeOnlyCyclesURIAndColor`, `TestConnectionForm_LockedModeTabToURIStartsCursorAtEnd`) must be deleted, not adapted — they test behavior being deliberately removed.
- `internal/tui` tests must never invoke the `tea.Cmd` closures that call `config.AddConnection`/`UpdateConnection`/`RenameConnection`/`DeleteConnection` directly in `connection_picker_test.go` (only assert `cmd != nil` and inspect model/form state) — the established project convention. `internal/tui/root_test.go` tests that exercise `connectionUpdatedMsg` handling directly (not via the form) may use `config.SetConnectionsFileForTesting` to redirect to a temp fixture, exactly as existing tests in that file already do.

---

### Task 1: `RenameConnection` in the config layer

**Files:**
- Modify: `internal/config/writer.go` (append new functions at the end of the file)
- Test: `internal/config/writer_test.go` (append new tests at the end of the file)

**Interfaces:**
- Consumes: `connectionsFile` (package-level var), `isValidConnectionName`, `zshSingleQuote`, `insertIntoArray`, `removeFromArray`, `validateZshSyntax` — all already defined in `internal/config/writer.go`.
- Produces: `func RenameConnection(oldName string, conn Connection) error` — used by Task 3. `func arrayHasKey(content, arrayName, name string) bool` — internal helper, not consumed outside this file.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/writer_test.go`:

```go
func TestRenameConnection_MovesEntriesToNewKeyInBothArrays(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	err := RenameConnection("qa", Connection{Name: "staging", URI: "mongodb://localhost:27017/test", Color: "verde"})
	if err != nil {
		t.Fatalf("RenameConnection failed: %v", err)
	}

	if _, err := ResolveConnection("qa"); err == nil {
		t.Fatal("expected 'qa' to no longer resolve after rename")
	}

	conn, err := ResolveConnection("staging")
	if err != nil {
		t.Fatalf("resolving renamed connection: %v", err)
	}
	want := Connection{Name: "staging", URI: "mongodb://localhost:27017/test", Color: "verde"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}

	other, err := ResolveConnection("prod")
	if err != nil {
		t.Fatalf("resolving untouched connection: %v", err)
	}
	if other.URI != "mongodb://localhost:27017/prod" || other.Color != "rojo" {
		t.Fatalf("untouched connection changed unexpectedly: %+v", other)
	}
}

func TestRenameConnection_RejectsCollisionWithDifferentExistingConnection(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file before RenameConnection: %v", err)
	}

	err = RenameConnection("qa", Connection{Name: "prod", URI: "mongodb://x", Color: "verde"})
	if err == nil {
		t.Fatal("expected an error renaming 'qa' to the already-existing 'prod', got nil")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file after RenameConnection: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file was modified despite rejected rename")
	}

	if _, err := ResolveConnection("qa"); err != nil {
		t.Fatalf("'qa' broke after rejected rename: %v", err)
	}
	if _, err := ResolveConnection("prod"); err != nil {
		t.Fatalf("'prod' broke after rejected rename: %v", err)
	}
}

func TestRenameConnection_SameNameBehavesLikePlainUpdate(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	err := RenameConnection("qa", Connection{Name: "qa", URI: "mongodb://newhost:27017/qa2", Color: "rojo"})
	if err != nil {
		t.Fatalf("RenameConnection failed: %v", err)
	}

	conn, err := ResolveConnection("qa")
	if err != nil {
		t.Fatalf("resolving connection: %v", err)
	}
	want := Connection{Name: "qa", URI: "mongodb://newhost:27017/qa2", Color: "rojo"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestRenameConnection_RejectsUnsafeNewName(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file before RenameConnection: %v", err)
	}

	malicious := `x]="y"; rm -rf ~ #`
	err = RenameConnection("qa", Connection{Name: malicious, URI: "mongodb://x", Color: "verde"})
	if err == nil {
		t.Fatal("expected an error for an unsafe new connection name, got nil")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file after RenameConnection: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file was modified despite rejected name")
	}
}

func TestRenameConnection_ResultIsValidZsh(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	if err := RenameConnection("qa", Connection{Name: "staging", URI: "mongodb://x", Color: "amarillo"}); err != nil {
		t.Fatalf("RenameConnection failed: %v", err)
	}
	if err := validateZshSyntax(path); err != nil {
		t.Fatalf("resulting file is not valid zsh: %v", err)
	}
}

func TestRenameConnection_NeutralizesShellMetacharactersInURI(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	marker := filepath.Join(t.TempDir(), "pwned")
	malicious := fmt.Sprintf("mongodb://x$(touch %s)y", marker)

	if err := RenameConnection("qa", Connection{Name: "staging", URI: malicious, Color: "verde"}); err != nil {
		t.Fatalf("RenameConnection failed: %v", err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command substitution in URI was executed — shell injection not neutralized")
	}

	conn, err := ResolveConnection("staging")
	if err != nil {
		t.Fatalf("resolving connection: %v", err)
	}
	if conn.URI != malicious {
		t.Fatalf("expected URI preserved literally as %q, got %q", malicious, conn.URI)
	}
}
```

The fixture `internal/config/testdata/basic.sh` already contains:
```sh
declare -A MONGO_CONNECTIONS=(
  [qa]="mongodb://localhost:27017/test"
  [prod]="mongodb://localhost:27017/prod"
)

declare -A MONGO_CONNECTION_COLORS=(
  [qa]="verde"
  [prod]="rojo"
)
```
No new fixture file is needed — these tests only use `qa` and `prod`, both already present.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/... -run TestRenameConnection -v`
Expected: FAIL — `RenameConnection` is undefined (compile error), since it doesn't exist yet.

- [ ] **Step 3: Implement `arrayHasKey` and `RenameConnection`**

Append to `internal/config/writer.go` (after the existing `removeFromArray` function, at the end of the file):

```go
// arrayHasKey reports whether the named zsh associative array already
// contains an entry for name. Used by RenameConnection to detect a
// collision before writing anything — renaming into an existing different
// connection's name would otherwise silently overwrite it.
func arrayHasKey(content, arrayName, name string) bool {
	header := fmt.Sprintf("declare -A %s=(", arrayName)
	if !strings.Contains(content, header) {
		return false
	}

	keyPrefix := fmt.Sprintf("[%s]=", name)
	lines := strings.Split(content, "\n")
	headerLineIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			headerLineIdx = i
			break
		}
	}
	if headerLineIdx == -1 {
		return false
	}

	for i := headerLineIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == ")" {
			return false
		}
		if strings.HasPrefix(trimmed, keyPrefix) {
			return true
		}
	}
	return false
}

// RenameConnection moves an existing connection from oldName to conn.Name,
// updating both MONGO_CONNECTIONS and MONGO_CONNECTION_COLORS. If conn.Name
// differs from oldName and already names a different existing connection,
// the rename is rejected (no file is written) to avoid silently
// overwriting that connection. Mirrors AddConnection/UpdateConnection's
// safety model: validates the result is still valid zsh before keeping the
// change, restoring the original file on any failure.
func RenameConnection(oldName string, conn Connection) error {
	if !isValidConnectionName(oldName) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", oldName)
	}
	if !isValidConnectionName(conn.Name) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", conn.Name)
	}

	original, err := os.ReadFile(connectionsFile)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", connectionsFile, err)
	}
	content := string(original)

	if conn.Name != oldName && arrayHasKey(content, "MONGO_CONNECTIONS", conn.Name) {
		return fmt.Errorf("ya existe una conexión llamada %q", conn.Name)
	}

	content, err = removeFromArray(content, "MONGO_CONNECTIONS", oldName)
	if err != nil {
		return err
	}
	content, err = removeFromArray(content, "MONGO_CONNECTION_COLORS", oldName)
	if err != nil {
		return err
	}
	content, err = insertIntoArray(content, "MONGO_CONNECTIONS", fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.URI)))
	if err != nil {
		return err
	}
	content, err = insertIntoArray(content, "MONGO_CONNECTION_COLORS", fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.Color)))
	if err != nil {
		return err
	}

	if err := os.WriteFile(connectionsFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("escribiendo %s: %w", connectionsFile, err)
	}

	if err := validateZshSyntax(connectionsFile); err != nil {
		_ = os.WriteFile(connectionsFile, original, 0600)
		return fmt.Errorf("el archivo resultante no era zsh válido, se revirtió: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS — all tests in the package, including the 6 new `TestRenameConnection_*` tests and every pre-existing test (`TestAddConnection_*`, `TestUpdateConnection_*`, `TestDeleteConnection_*`) still passing unmodified.

- [ ] **Step 5: Commit**

```bash
git add internal/config/writer.go internal/config/writer_test.go
git commit -m "feat: add RenameConnection for moving a connection to a new array key"
```

---

### Task 2: Remove `locked` from `connectionForm`

**Files:**
- Modify: `internal/tui/connection_picker.go:21-105` (the `connectionForm` struct and its `update`/`newEditConnectionForm` methods)
- Test: `internal/tui/connection_picker_test.go`

**Interfaces:**
- Consumes: nothing new from other tasks.
- Produces: `connectionForm` with no `locked` field; `newEditConnectionForm` now returns a form starting on field 0 (Name) instead of field 1 (URI). Task 3 relies on Name being freely editable in edit mode.

- [ ] **Step 1: Update the test file**

In `internal/tui/connection_picker_test.go`:

Delete these three tests entirely (they assert behavior being removed):
```go
func TestConnectionForm_NameFieldIsReadOnlyWhenLocked(t *testing.T) {
	f := newEditConnectionForm(config.Connection{Name: "qa", URI: "mongodb://qa", Color: "verde"})
	f.field = 0 // force onto the Name field to prove typing is still rejected
	f = f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if f.name != "qa" {
		t.Fatalf("expected Name to stay unchanged when locked, got %q", f.name)
	}
}

func TestConnectionForm_TabInLockedModeOnlyCyclesURIAndColor(t *testing.T) {
	f := newEditConnectionForm(config.Connection{Name: "qa", URI: "mongodb://qa", Color: "verde"})
	if f.field != 1 {
		t.Fatalf("expected edit form to start on field 1 (URI), got %d", f.field)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	if f.field != 2 {
		t.Fatalf("expected Tab to move to field 2 (Color), got %d", f.field)
	}
	f = f.update(tea.KeyMsg{Type: tea.KeyTab})
	if f.field != 1 {
		t.Fatalf("expected Tab to cycle back to field 1 (URI), skipping Name, got %d", f.field)
	}
}
```
and
```go
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
```

Replace `TestConnectionPicker_EPreFillsEditFormWithCurrentValues` (currently asserts `field != 1`) with:
```go
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
```

Replace `TestConnectionPicker_EnterInEditModeReturnsACommand` (currently types directly into what used to be the URI field) with:
```go
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
```

Add two new tests, anywhere after `TestConnectionForm_TabMovesCursorToEndOfNewlyActiveField`:
```go
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
```

- [ ] **Step 2: Run the tests to verify the updated/new ones fail**

Run: `go test ./internal/tui/... -run TestConnectionForm -v` and `go test ./internal/tui/... -run TestConnectionPicker_EPreFillsEditFormWithCurrentValues -v`
Expected: FAIL — `TestConnectionForm_NameFieldIsEditableInEditForm` and `TestConnectionForm_TabInEditFormCyclesThroughAllThreeFields` fail because the current `newEditConnectionForm` still starts on field 1 and still sets `locked: true`; `TestConnectionPicker_EPreFillsEditFormWithCurrentValues` fails because `m.form.field` is `1`, not `0`.

- [ ] **Step 3: Remove `locked` from the implementation**

In `internal/tui/connection_picker.go`, replace the `connectionForm` struct (lines 21-28):
```go
type connectionForm struct {
	name   string
	uri    string
	color  string
	field  int  // 0=name, 1=uri, 2=color
	locked bool // when true, Name is read-only and Tab only cycles URI/Color
	cursor int  // rune index within whichever of Name/URI (field 0 or 1) is active
}
```
with:
```go
type connectionForm struct {
	name   string
	uri    string
	color  string
	field  int // 0=name, 1=uri, 2=color
	cursor int // rune index within whichever of Name/URI (field 0 or 1) is active
}
```

Replace `newEditConnectionForm` (lines 34-44):
```go
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
```
with:
```go
// newEditConnectionForm pre-fills the form with an existing connection's
// current values, starting focus on Name with the cursor at its end —
// matching newConnectionForm's default start point. Name is fully editable
// here too; connectionPickerModel decides at submit time whether a changed
// Name means a rename (see editingOriginalName).
func newEditConnectionForm(conn config.Connection) connectionForm {
	return connectionForm{
		name: conn.Name, uri: conn.URI, color: conn.Color,
		field: 0, cursor: len([]rune(conn.Name)),
	}
}
```

Replace the `update` method (lines 46-105):
```go
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
```
with:
```go
func (f connectionForm) update(msg tea.KeyMsg) connectionForm {
	switch msg.String() {
	case "tab":
		f.field = (f.field + 1) % 3
		f.cursor = len([]rune(f.activeFieldText()))
		return f
	case "shift+tab":
		f.field = (f.field + 2) % 3
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
		f.deleteBeforeCursor()
	case tea.KeyRunes:
		f.insertAtCursor(string(msg.Runes))
	}
	return f
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS for every test in the package. In particular: the two new tests, the two rewritten tests, and every other pre-existing `TestConnectionForm_*`/`TestConnectionPicker_*` test (e.g. `TestConnectionForm_TypingInsertsAtCursorNotAlwaysAtEnd`, `TestConnectionForm_TabMovesCursorToEndOfNewlyActiveField`, `TestConnectionPicker_ViewShowsCursorMarkerAtRealPositionInActiveField`) still passing unmodified — none of them touch `newEditConnectionForm` or `locked`.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/connection_picker.go internal/tui/connection_picker_test.go
git commit -m "feat: make the connection form's Name field editable in edit mode too"
```

---

### Task 3: `connectionPickerModel` picks rename vs. plain update at submit time

**Files:**
- Modify: `internal/tui/connection_picker.go:187-297` (the `connectionPickerModel` struct and its `Update` method)
- Test: `internal/tui/connection_picker_test.go`

**Interfaces:**
- Consumes: `config.RenameConnection(oldName string, conn Connection) error` (Task 1), `connectionForm` with Name editable in edit mode (Task 2).
- Produces: `connectionPickerModel.editingOriginalName string` — the name captured when `e` was pressed. Task 4 does not depend on this field directly (it reads `msg.Conn.Name` from the message instead), but relies on this task correctly setting `Conn.Name` to the *new* name in both the rename and plain-update paths.

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/connection_picker_test.go` (anywhere after `TestConnectionPicker_EPreFillsEditFormWithCurrentValues`):

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -run TestConnectionPicker_EStoresOriginalNameForRenameDetection -v`
Expected: FAIL — compile error, `editingOriginalName` is not a field on `connectionPickerModel` yet.

- [ ] **Step 3: Implement the field and the submit branching**

In `internal/tui/connection_picker.go`, replace the `connectionPickerModel` struct:
```go
type connectionPickerModel struct {
	list             listModel
	conns            []config.Connection
	creating         bool
	editing          bool
	confirmingDelete bool
	confirm          confirmModel
	form             connectionForm
}
```
with:
```go
type connectionPickerModel struct {
	list                listModel
	conns               []config.Connection
	creating            bool
	editing             bool
	confirmingDelete    bool
	confirm             confirmModel
	form                connectionForm
	editingOriginalName string
}
```

Replace the `"e"` case inside `Update`:
```go
		case "e":
			if conn, ok := m.selectedConnection(); ok {
				m.editing = true
				m.form = newEditConnectionForm(conn)
			}
			return m, nil
```
with:
```go
		case "e":
			if conn, ok := m.selectedConnection(); ok {
				m.editing = true
				m.form = newEditConnectionForm(conn)
				m.editingOriginalName = conn.Name
			}
			return m, nil
```

Replace the `enter`-handling block inside `Update`:
```go
		if keyMsg.String() == "enter" {
			conn := config.Connection{Name: m.form.name, URI: m.form.uri, Color: m.form.color}
			if m.editing {
				return m, func() tea.Msg {
					if err := config.UpdateConnection(conn); err != nil {
						return connectionUpdateErrMsg{Err: err}
					}
					return connectionUpdatedMsg{Conn: conn}
				}
			}
			return m, func() tea.Msg {
				if err := config.AddConnection(conn); err != nil {
					return connectionCreateErrMsg{Err: err}
				}
				return connectionCreatedMsg{Conn: conn}
			}
		}
```
with:
```go
		if keyMsg.String() == "enter" {
			conn := config.Connection{Name: m.form.name, URI: m.form.uri, Color: m.form.color}
			if m.editing {
				oldName := m.editingOriginalName
				if conn.Name != oldName {
					return m, func() tea.Msg {
						if err := config.RenameConnection(oldName, conn); err != nil {
							return connectionUpdateErrMsg{Err: err}
						}
						return connectionUpdatedMsg{Conn: conn}
					}
				}
				return m, func() tea.Msg {
					if err := config.UpdateConnection(conn); err != nil {
						return connectionUpdateErrMsg{Err: err}
					}
					return connectionUpdatedMsg{Conn: conn}
				}
			}
			return m, func() tea.Msg {
				if err := config.AddConnection(conn); err != nil {
					return connectionCreateErrMsg{Err: err}
				}
				return connectionCreatedMsg{Conn: conn}
			}
		}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS for every test in the package, including the 2 new tests and every test from Task 2.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/connection_picker.go internal/tui/connection_picker_test.go
git commit -m "feat: route a changed connection name through RenameConnection on submit"
```

---

### Task 4: List cursor follows the connection after an edit or rename

**Files:**
- Modify: `internal/tui/root.go:302-309` (the `connectionUpdatedMsg` case inside `RootModel.Update`)
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `connectionUpdatedMsg{Conn config.Connection}` (existing message type), `m.connPicker.list.Items []listItem` and `m.connPicker.list.Cursor int` (existing `listModel` fields, each `listItem` has an `ID string`).
- Produces: nothing consumed by other tasks — this is the last task in the plan.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/root_test.go`, anywhere after `TestRootModel_ConnectionUpdatedMsgReloadsConnectionsList`:

```go
func TestRootModel_ConnectionUpdatedMsgMovesCursorToTheEditedConnection(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mongo-connections.sh")
	fixture := "declare -A MONGO_CONNECTIONS=(\n  [aaa]=\"mongodb://localhost:27017/aaa\"\n  [zzz]=\"mongodb://localhost:27017/zzz\"\n)\n\ndeclare -A MONGO_CONNECTION_COLORS=(\n  [aaa]=\"verde\"\n  [zzz]=\"verde\"\n)\n"
	if err := os.WriteFile(tmp, []byte(fixture), 0600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	restore := config.SetConnectionsFileForTesting(tmp)
	defer restore()

	m, _ := newTestRootModel()
	root := *m

	// connectionUpdatedMsg.Conn.Name is always the *new* name, regardless of
	// whether it came from a plain update or a rename — this message names
	// "zzz", the connection that sorts alphabetically last in the fixture,
	// to prove the handler positions the cursor by name rather than
	// leaving it at whatever index newConnectionPickerModel defaults to.
	model, _ := root.Update(connectionUpdatedMsg{Conn: config.Connection{Name: "zzz", URI: "mongodb://localhost:27017/zzz", Color: "verde"}})
	root = model.(RootModel)

	if got := root.connPicker.list.Cursor; got != 1 {
		t.Fatalf("expected cursor at index 1 ('zzz', alphabetically last), got %d (items: %+v)", got, root.connPicker.list.Items)
	}
	if got := root.connPicker.list.Items[root.connPicker.list.Cursor].ID; got != "zzz" {
		t.Fatalf("expected cursor to point at 'zzz', got %q", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/... -run TestRootModel_ConnectionUpdatedMsgMovesCursorToTheEditedConnection -v`
Expected: FAIL — the cursor is `0` (pointing at `aaa`), not `1`, because `newConnectionPickerModel` always resets the cursor to 0 and nothing repositions it yet.

- [ ] **Step 3: Implement the cursor repositioning**

In `internal/tui/root.go`, replace the `connectionUpdatedMsg` case:
```go
	case connectionUpdatedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		return m, nil
```
with:
```go
	case connectionUpdatedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		for i, item := range m.connPicker.list.Items {
			if item.ID == msg.Conn.Name {
				m.connPicker.list.Cursor = i
				break
			}
		}
		return m, nil
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS for every test in the package, including the new test and the pre-existing `TestRootModel_ConnectionUpdatedMsgReloadsConnectionsList` (which only has one connection in its fixture, so the cursor lands at index 0 either way — unaffected by this change).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat: keep the connections list cursor on the connection just edited or renamed"
```

---

## Final check

Run the full suite once more from the project root to confirm nothing elsewhere regressed:

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.
