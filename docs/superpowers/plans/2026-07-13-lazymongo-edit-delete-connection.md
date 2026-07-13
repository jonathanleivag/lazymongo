# lazymongo — Edit and delete existing connections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `e` edit an existing connection's URI/color (name fixed) and `d`/`x` delete it, in the `[5]` Conexiones panel.

**Architecture:** Two new `internal/config` functions (`UpdateConnection`, `DeleteConnection`) mirror `AddConnection`'s exact safety model (validate name, write, verify zsh syntax, revert on failure) using new line-replace/line-remove helpers, kept separate from the existing `insertIntoArray` so `AddConnection`'s already-shipped behavior is untouched. `connectionPickerModel` reuses the existing create-connection form and `confirmModel` (already used for document/index deletion) for both new flows.

**Tech Stack:** Go, standard library only (no new dependencies).

## Global Constraints

- `insertIntoArray` (used by `AddConnection`) is not modified. The new helpers (`replaceOrInsertInArray`, `removeFromArray`) are separate functions.
- `e`/`d`/`x` in the Conexiones panel are gated behind `!m.list.Filtering()` — the same guard already applied to `a` when fuzzy search was added to this panel. Without it, typing those letters into a search query would trigger edit/delete instead of narrowing the search.
- Renaming (changing a connection's name/array-key) is out of scope — editing only ever changes URI and color; the Name field is read-only in the edit form.
- `internal/tui` tests never invoke the `tea.Cmd` closures that call `config.AddConnection`/`UpdateConnection`/`DeleteConnection` — those would hit the real `~/.config/mongo-connections.sh` on whatever machine runs the tests. The existing `TestConnectionPicker_CreateFormSubmitsNewConnection` test already follows this rule (checks `cmd != nil` and inspects form state, never calls `cmd()`); every new TUI-level test in this plan follows it too. Only `internal/config`'s own tests (which redirect to a temp fixture via `withTempConnectionsFile`) actually execute the file-writing code.

---

### Task 1: `config.UpdateConnection` and `config.DeleteConnection`

**Files:**
- Modify: `internal/config/writer.go`
- Test: `internal/config/writer_test.go`

**Interfaces:**
- Produces: `func UpdateConnection(conn Connection) error`, `func DeleteConnection(name string) error`. Task 2 (`connection_picker.go`) calls both.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/writer_test.go`:

```go
func TestUpdateConnection_ChangesURIAndColorKeepingName(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	err := UpdateConnection(Connection{Name: "qa", URI: "mongodb://newhost:27017/qa2", Color: "rojo"})
	if err != nil {
		t.Fatalf("UpdateConnection failed: %v", err)
	}

	conn, err := ResolveConnection("qa")
	if err != nil {
		t.Fatalf("resolving updated connection: %v", err)
	}
	want := Connection{Name: "qa", URI: "mongodb://newhost:27017/qa2", Color: "rojo"}
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

func TestUpdateConnection_WorksWhenConnectionNeverHadAColor(t *testing.T) {
	withTempConnectionsFile(t, "testdata/with_comments.sh")

	err := UpdateConnection(Connection{Name: "ejemplo-local", URI: "mongodb://localhost:27017/renamed", Color: "verde"})
	if err != nil {
		t.Fatalf("UpdateConnection failed: %v", err)
	}

	conn, err := ResolveConnection("ejemplo-local")
	if err != nil {
		t.Fatalf("resolving updated connection: %v", err)
	}
	want := Connection{Name: "ejemplo-local", URI: "mongodb://localhost:27017/renamed", Color: "verde"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestUpdateConnection_ResultIsValidZsh(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	if err := UpdateConnection(Connection{Name: "qa", URI: "mongodb://x", Color: "amarillo"}); err != nil {
		t.Fatalf("UpdateConnection failed: %v", err)
	}
	if err := validateZshSyntax(path); err != nil {
		t.Fatalf("resulting file is not valid zsh: %v", err)
	}
}

func TestUpdateConnection_RejectsUnsafeName(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file before UpdateConnection: %v", err)
	}

	malicious := `x]="y"; rm -rf ~ #`
	err = UpdateConnection(Connection{Name: malicious, URI: "mongodb://x", Color: "verde"})
	if err == nil {
		t.Fatal("expected an error for an unsafe connection name, got nil")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file after UpdateConnection: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file was modified despite rejected name")
	}
}

func TestDeleteConnection_RemovesConnectionFromBothArrays(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	if err := DeleteConnection("qa"); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	if _, err := ResolveConnection("qa"); err == nil {
		t.Fatal("expected 'qa' to no longer resolve after deletion")
	}

	other, err := ResolveConnection("prod")
	if err != nil {
		t.Fatalf("resolving untouched connection: %v", err)
	}
	if other.URI != "mongodb://localhost:27017/prod" || other.Color != "rojo" {
		t.Fatalf("untouched connection changed unexpectedly: %+v", other)
	}
}

func TestDeleteConnection_NoOpWhenConnectionNeverHadAColor(t *testing.T) {
	withTempConnectionsFile(t, "testdata/with_comments.sh")

	if err := DeleteConnection("ejemplo-local"); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	if _, err := ResolveConnection("ejemplo-local"); err == nil {
		t.Fatal("expected 'ejemplo-local' to no longer resolve after deletion")
	}
}

func TestDeleteConnection_ResultIsValidZsh(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	if err := DeleteConnection("qa"); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}
	if err := validateZshSyntax(path); err != nil {
		t.Fatalf("resulting file is not valid zsh: %v", err)
	}
}

func TestDeleteConnection_RejectsUnsafeName(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file before DeleteConnection: %v", err)
	}

	malicious := `x]="y"; rm -rf ~ #`
	err = DeleteConnection(malicious)
	if err == nil {
		t.Fatal("expected an error for an unsafe connection name, got nil")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file after DeleteConnection: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file was modified despite rejected name")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/config/... -v -run "TestUpdateConnection|TestDeleteConnection"`
Expected: FAIL — `UpdateConnection`/`DeleteConnection` undefined

- [ ] **Step 3: Implement UpdateConnection and DeleteConnection**

Append to `internal/config/writer.go` (the existing `import` block already has `"bytes"`, `"fmt"`, `"os"`, `"os/exec"`, `"strings"` — no new imports needed):

```go
// UpdateConnection replaces an existing connection's URI and color in the
// real connections file, keeping its name (the array key) unchanged.
// Mirrors AddConnection's safety model: validates the result is still
// valid zsh before keeping the change, restoring the original file on any
// failure.
func UpdateConnection(conn Connection) error {
	if !isValidConnectionName(conn.Name) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", conn.Name)
	}

	original, err := os.ReadFile(connectionsFile)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", connectionsFile, err)
	}

	updated, err := updateConnectionInFile(string(original), conn)
	if err != nil {
		return err
	}

	if err := os.WriteFile(connectionsFile, []byte(updated), 0600); err != nil {
		return fmt.Errorf("escribiendo %s: %w", connectionsFile, err)
	}

	if err := validateZshSyntax(connectionsFile); err != nil {
		_ = os.WriteFile(connectionsFile, original, 0600)
		return fmt.Errorf("el archivo resultante no era zsh válido, se revirtió: %w", err)
	}
	return nil
}

func updateConnectionInFile(content string, conn Connection) (string, error) {
	content, err := replaceOrInsertInArray(content, "MONGO_CONNECTIONS", conn.Name, fmt.Sprintf("  [%s]=%q", conn.Name, conn.URI))
	if err != nil {
		return "", err
	}
	content, err = replaceOrInsertInArray(content, "MONGO_CONNECTION_COLORS", conn.Name, fmt.Sprintf("  [%s]=%q", conn.Name, conn.Color))
	if err != nil {
		return "", err
	}
	return content, nil
}

// replaceOrInsertInArray replaces the existing "[name]=..." line within the
// named zsh associative array with newLine, or inserts it (same fallback as
// insertIntoArray) if the array exists but doesn't yet have an entry for
// name — creating the array block fresh if it doesn't exist at all. Used by
// UpdateConnection; insertIntoArray (used by AddConnection) is untouched —
// this is a separate function, not a refactor of already-shipped behavior.
func replaceOrInsertInArray(content, arrayName, name, newLine string) (string, error) {
	header := fmt.Sprintf("declare -A %s=(", arrayName)
	keyPrefix := fmt.Sprintf("[%s]=", name)

	if !strings.Contains(content, header) {
		block := fmt.Sprintf("\ndeclare -A %s=(\n%s\n)\n", arrayName, newLine)
		return content + block, nil
	}

	lines := strings.Split(content, "\n")
	headerLineIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			headerLineIdx = i
			break
		}
	}
	if headerLineIdx == -1 {
		return "", fmt.Errorf("se encontró %q pero no en su propia línea, no se pudo editar %s de forma segura", header, arrayName)
	}

	for i := headerLineIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == ")" {
			result := make([]string, 0, len(lines)+1)
			result = append(result, lines[:i]...)
			result = append(result, newLine)
			result = append(result, lines[i:]...)
			return strings.Join(result, "\n"), nil
		}
		if strings.HasPrefix(trimmed, keyPrefix) {
			result := make([]string, len(lines))
			copy(result, lines)
			result[i] = newLine
			return strings.Join(result, "\n"), nil
		}
	}
	return "", fmt.Errorf("no se encontró el cierre ')' del array %s", arrayName)
}

// DeleteConnection removes a connection from the real connections file.
// Mirrors AddConnection's safety model. Removing from
// MONGO_CONNECTION_COLORS is a no-op (not an error) when the connection
// never had a color entry.
func DeleteConnection(name string) error {
	if !isValidConnectionName(name) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", name)
	}

	original, err := os.ReadFile(connectionsFile)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", connectionsFile, err)
	}

	updated, err := removeFromArray(string(original), "MONGO_CONNECTIONS", name)
	if err != nil {
		return err
	}
	updated, err = removeFromArray(updated, "MONGO_CONNECTION_COLORS", name)
	if err != nil {
		return err
	}

	if err := os.WriteFile(connectionsFile, []byte(updated), 0600); err != nil {
		return fmt.Errorf("escribiendo %s: %w", connectionsFile, err)
	}

	if err := validateZshSyntax(connectionsFile); err != nil {
		_ = os.WriteFile(connectionsFile, original, 0600)
		return fmt.Errorf("el archivo resultante no era zsh válido, se revirtió: %w", err)
	}
	return nil
}

// removeFromArray removes the "[name]=..." line from the named zsh
// associative array, or does nothing (returns content unchanged) if the
// array doesn't exist or has no entry for name.
func removeFromArray(content, arrayName, name string) (string, error) {
	header := fmt.Sprintf("declare -A %s=(", arrayName)
	if !strings.Contains(content, header) {
		return content, nil
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
		return "", fmt.Errorf("se encontró %q pero no en su propia línea, no se pudo editar %s de forma segura", header, arrayName)
	}

	for i := headerLineIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == ")" {
			return content, nil
		}
		if strings.HasPrefix(trimmed, keyPrefix) {
			result := make([]string, 0, len(lines)-1)
			result = append(result, lines[:i]...)
			result = append(result, lines[i+1:]...)
			return strings.Join(result, "\n"), nil
		}
	}
	return content, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/... -v -run "TestUpdateConnection|TestDeleteConnection"`
Expected: PASS (all 8 tests)

- [ ] **Step 5: Run the full config package test suite to check for regressions**

Run: `go test ./internal/config/... -v`
Expected: PASS — every pre-existing test in `internal/config` still passes (in particular every `TestAddConnection_*` test, proving `insertIntoArray`/`AddConnection` are unaffected)

- [ ] **Step 6: Commit**

```bash
git add internal/config/writer.go internal/config/writer_test.go
git commit -m "feat: add UpdateConnection and DeleteConnection to internal/config"
```

---

### Task 2: Edit and delete UI in the connection picker

**Files:**
- Modify: `internal/tui/connection_picker.go`
- Test: `internal/tui/connection_picker_test.go`

**Interfaces:**
- Consumes: `config.UpdateConnection(conn Connection) error`, `config.DeleteConnection(name string) error` (Task 1)
- Produces: `type connectionUpdatedMsg struct{ Conn config.Connection }`, `type connectionUpdateErrMsg struct{ Err error }`, `type connectionDeletedMsg struct{ Name string }`, `type connectionDeleteErrMsg struct{ Err error }`. Task 3 (root.go) handles all four.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/connection_picker_test.go`:

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
	if m.form.field != 1 {
		t.Fatalf("expected edit form to start on the URI field (1), got %d", m.form.field)
	}
}

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

func TestConnectionPicker_EnterInEditModeReturnsACommand(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa:27017", Color: "verde"},
	}
	m := newConnectionPickerModel(conns)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run "TestConnectionPicker_E|TestConnectionForm_|TestConnectionPicker_D|TestConnectionPicker_Confirming|TestConnectionPicker_TypingED"`
Expected: FAIL — `m.editing`/`newEditConnectionForm`/`m.confirmingDelete` undefined

- [ ] **Step 3: Replace connection_picker.go**

Replace the full contents of `internal/tui/connection_picker.go`:

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
)

type connectionChosenMsg struct{ Conn config.Connection }
type connectionCreatedMsg struct{ Conn config.Connection }
type connectionCreateErrMsg struct{ Err error }
type connectionUpdatedMsg struct{ Conn config.Connection }
type connectionUpdateErrMsg struct{ Err error }
type connectionDeletedMsg struct{ Name string }
type connectionDeleteErrMsg struct{ Err error }

var colorChoices = []string{"amarillo", "rojo", "verde"}

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

func nextColor(current string, delta int) string {
	idx := 0
	for i, c := range colorChoices {
		if c == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(colorChoices)) % len(colorChoices)
	return colorChoices[idx]
}

type connectionPickerModel struct {
	list             listModel
	conns            []config.Connection
	creating         bool
	editing          bool
	confirmingDelete bool
	confirm          confirmModel
	form             connectionForm
}

func newConnectionPickerModel(conns []config.Connection) connectionPickerModel {
	items := make([]listItem, len(conns))
	for i, c := range conns {
		items[i] = listItem{ID: c.Name, Label: c.Name, Color: c.Color}
	}
	return connectionPickerModel{list: newListModel("Conexiones", items, true), conns: conns}
}

// selectedConnection returns the full Connection (including URI) behind
// the currently highlighted list item, looked up by name so it's correct
// even while the list is fuzzy-filtered/reordered.
func (m connectionPickerModel) selectedConnection() (config.Connection, bool) {
	if len(m.list.Items) == 0 {
		return config.Connection{}, false
	}
	name := m.list.Items[m.list.Cursor].ID
	for _, c := range m.conns {
		if c.Name == name {
			return c, true
		}
	}
	return config.Connection{}, false
}

func (m connectionPickerModel) Update(msg tea.Msg) (connectionPickerModel, tea.Cmd) {
	if m.confirmingDelete {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if cmd == nil {
			return m, nil
		}
		result, ok := cmd().(confirmResultMsg)
		if !ok {
			return m, cmd
		}
		m.confirmingDelete = false
		if !result.Confirmed {
			return m, nil
		}
		conn, ok := m.selectedConnection()
		if !ok {
			return m, nil
		}
		name := conn.Name
		return m, func() tea.Msg {
			if err := config.DeleteConnection(name); err != nil {
				return connectionDeleteErrMsg{Err: err}
			}
			return connectionDeletedMsg{Name: name}
		}
	}

	if m.creating || m.editing {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		if keyMsg.String() == "esc" {
			m.creating = false
			m.editing = false
			return m, nil
		}
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
		m.form = m.form.update(keyMsg)
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && !m.list.Filtering() {
		switch keyMsg.String() {
		case "a":
			m.creating = true
			m.form = newConnectionForm()
			return m, nil
		case "e":
			if conn, ok := m.selectedConnection(); ok {
				m.editing = true
				m.form = newEditConnectionForm(conn)
			}
			return m, nil
		case "d", "x":
			if conn, ok := m.selectedConnection(); ok {
				m.confirmingDelete = true
				m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar la conexión %q?", conn.Name)}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if cmd == nil {
		return m, nil
	}
	if selected, ok := cmd().(itemSelectedMsg); ok {
		for _, item := range m.list.Items {
			if item.ID == selected.Item.ID {
				return m, func() tea.Msg {
					return connectionChosenMsg{Conn: config.Connection{Name: item.ID, Color: item.Color}}
				}
			}
		}
	}
	return m, cmd
}

func (m connectionPickerModel) View() string {
	if m.confirmingDelete {
		return m.confirm.View()
	}
	if m.creating || m.editing {
		title := "Nueva conexión"
		if m.editing {
			title = "Editar conexión"
		}
		var b strings.Builder
		b.WriteString(titleStyle.Render(title) + "\n\n")
		b.WriteString("Nombre: " + m.form.name)
		if m.form.field == 0 {
			b.WriteString(" <")
		}
		b.WriteString("\nURI:    " + m.form.uri)
		if m.form.field == 1 {
			b.WriteString(" <")
		}
		b.WriteString("\nColor:  " + colorStyle(m.form.color).Render(m.form.color))
		if m.form.field == 2 {
			b.WriteString(" <")
		}
		b.WriteString("\n\n[Tab] siguiente campo  [h/l] cambiar color  [Enter] guardar  [Esc] cancelar")
		return b.String()
	}
	return m.list.View()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestConnectionPicker_E|TestConnectionForm_|TestConnectionPicker_D|TestConnectionPicker_Confirming|TestConnectionPicker_TypingED"`
Expected: PASS (all 6 new tests)

- [ ] **Step 5: Run the full connection_picker test file to check for regressions**

Run: `go test ./internal/tui/... -v -run TestConnectionPicker`
Expected: PASS — every pre-existing `TestConnectionPicker_*` test still passes, including `TestConnectionPicker_PressingAOpensCreateForm` and `TestConnectionPicker_CreateFormSubmitsNewConnection` (proving the "a" create-flow is unaffected by the new `editing`/`confirmingDelete` states and the refactored key-dispatch switch)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/connection_picker.go internal/tui/connection_picker_test.go
git commit -m "feat: add edit (e) and delete (d/x) to the Conexiones panel"
```

---

### Task 3: Wire edit/delete into RootModel, update docs

**Files:**
- Modify: `internal/tui/root.go`
- Modify: `internal/tui/help.go`
- Modify: `README.md`
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `connectionUpdatedMsg`, `connectionUpdateErrMsg`, `connectionDeletedMsg`, `connectionDeleteErrMsg` (Task 2), `connPicker.editing`/`connPicker.confirmingDelete` (Task 2)

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/root_test.go` (add `"os"` and `"path/filepath"` to the existing import block if not already present):

```go
func TestRootModel_ConnectionUpdatedMsgReloadsConnectionsList(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mongo-connections.sh")
	fixture := "declare -A MONGO_CONNECTIONS=(\n  [qa]=\"mongodb://localhost:27017/test\"\n)\n\ndeclare -A MONGO_CONNECTION_COLORS=(\n  [qa]=\"verde\"\n)\n"
	if err := os.WriteFile(tmp, []byte(fixture), 0600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	restore := config.SetConnectionsFileForTesting(tmp)
	defer restore()

	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionUpdatedMsg{Conn: config.Connection{Name: "qa", URI: "mongodb://localhost:27017/test", Color: "verde"}})
	root = model.(RootModel)
	if len(root.connPicker.list.Items) != 1 || root.connPicker.list.Items[0].ID != "qa" {
		t.Fatalf("expected connections list reloaded with 'qa', got %+v", root.connPicker.list.Items)
	}
}

func TestRootModel_ConnectionDeletedMsgReloadsConnectionsList(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mongo-connections.sh")
	fixture := "declare -A MONGO_CONNECTIONS=(\n  [staging]=\"mongodb://localhost:27017/staging\"\n)\n"
	if err := os.WriteFile(tmp, []byte(fixture), 0600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	restore := config.SetConnectionsFileForTesting(tmp)
	defer restore()

	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionDeletedMsg{Name: "qa"})
	root = model.(RootModel)
	if len(root.connPicker.list.Items) != 1 || root.connPicker.list.Items[0].ID != "staging" {
		t.Fatalf("expected connections list reloaded with only 'staging', got %+v", root.connPicker.list.Items)
	}
}

func TestRootModel_ConnectionUpdateErrMsgSetsErr(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionUpdateErrMsg{Err: fmt.Errorf("boom")})
	root = model.(RootModel)
	if root.err == nil {
		t.Fatal("expected root.err to be set after connectionUpdateErrMsg")
	}
}

func TestRootModel_ConnectionDeleteErrMsgSetsErr(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionDeleteErrMsg{Err: fmt.Errorf("boom")})
	root = model.(RootModel)
	if root.err == nil {
		t.Fatal("expected root.err to be set after connectionDeleteErrMsg")
	}
}

func TestRootModel_InTextEntry_TrueWhileEditingConnection(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.focus = panelConnections
	root.connPicker.editing = true

	if !root.inTextEntry() {
		t.Fatal("expected inTextEntry to be true while editing a connection")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v -run "TestRootModel_Connection|TestRootModel_InTextEntry_TrueWhileEditingConnection"`
Expected: FAIL — `connectionUpdatedMsg`/`connectionUpdateErrMsg`/`connectionDeletedMsg`/`connectionDeleteErrMsg` cases don't exist yet in `Update`, and `connPicker.editing` isn't read by `inTextEntry`

- [ ] **Step 3: Wire the new messages and inTextEntry**

In `internal/tui/root.go`, add `case m.focus == panelConnections && m.connPicker.editing: return true` to `inTextEntry()`, right after the existing `case m.focus == panelConnections && m.connPicker.creating:` case:

```go
	case m.focus == panelConnections && m.connPicker.creating:
		return true
	case m.focus == panelConnections && m.connPicker.editing:
		return true
```

Add these four cases to the top-level `switch msg := msg.(type)` block, right after the existing `case connectionCreateErrMsg:` case:

```go
	case connectionCreateErrMsg:
		m.err = msg.Err
		return m, nil

	case connectionUpdatedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		return m, nil

	case connectionUpdateErrMsg:
		m.err = msg.Err
		return m, nil

	case connectionDeletedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		return m, nil

	case connectionDeleteErrMsg:
		m.err = msg.Err
		return m, nil
```

(Only the four new cases are added — the existing `connectionCreateErrMsg` case shown above is an anchor, unchanged.)

Replace the popup-overlay condition:

```go
	if m.focus == panelConnections && m.connPicker.creating {
		return renderPopupOverlay(m.connPicker.View(), m.width, m.height)
	}
```

with:

```go
	if m.focus == panelConnections && (m.connPicker.creating || m.connPicker.editing || m.connPicker.confirmingDelete) {
		return renderPopupOverlay(m.connPicker.View(), m.width, m.height)
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestRootModel_Connection|TestRootModel_InTextEntry_TrueWhileEditingConnection"`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Update help.go**

In `internal/tui/help.go`, the existing `helpText` const has this line:

```go
e              editar campo (en detalle de documento)
```

Replace it with:

```go
e              editar campo (en detalle de documento) / editar conexión (URI y color, en Conexiones)
```

(only this one line changes — `i, a`, `E`, and `d, x` stay exactly as they are; `d, x`'s existing "borrar (siempre pide confirmación)" already generically covers the new Conexiones case with no wording change needed.)

- [ ] **Step 6: Update README.md**

In the "Keybindings" table, `README.md` currently has this row:

```markdown
| `e` | edit field inline |
```

Replace it with:

```markdown
| `e` | edit field inline (document detail popup) / edit a connection's URI and color (Conexiones panel — name stays fixed) |
```

In the "Manual smoke test against qa" checklist, `README.md` currently has this line:

```markdown
- [ ] `5` focuses Conexiones; `Enter` on a different saved connection reconnects
```

Add two new lines right after it:

```markdown
- [ ] `e` on a connection opens the edit form pre-filled with its current URI/color; `Enter` saves, `Esc` cancels; the name field cannot be typed into
- [ ] `d`/`x` on a connection opens a delete confirmation; confirming removes it from the list
```

- [ ] **Step 7: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: all packages build, vet is clean, all tests pass (pre-existing and new)

- [ ] **Step 8: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go internal/tui/help.go README.md
git commit -m "feat: wire connection edit/delete into RootModel, update docs"
```

---

## Final manual smoke test (after all three tasks)

Run: `go build -o lazymongo . && ./lazymongo`, then in `[5]` Conexiones:
- `e` on a connection opens the edit form pre-filled with its current URI/color; the Name field shows the name but rejects typed input; `Tab` only moves between URI and Color
- Editing the URI and/or color and pressing `Enter` saves it — reconnecting to that name later uses the new URI/color
- `d` or `x` on a connection opens "¿Borrar la conexión "name"?"; confirming with `y` removes it from the list; `n`/`Esc` cancels without changing anything
- Typing `/` then a query containing "e" or "d" narrows the search instead of opening the edit form or delete confirmation
- Creating a new connection with `a` still works exactly as before
