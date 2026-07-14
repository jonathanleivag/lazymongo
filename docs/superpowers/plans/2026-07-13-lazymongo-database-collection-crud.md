# Database/Collection Create, Rename, Delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add create + delete support to the Databases panel and create + rename + delete support to the Collections panel, mirroring the existing Indexes/Connections panels' patterns.

**Architecture:** Four new `mongo.Client` methods (`CreateCollection`, `DropCollection`, `DropDatabase`, `RenameCollection`) back two new TUI wrapper types, `dbListModel` and `collListModel`, each wrapping a `listModel` field exactly like `connectionPickerModel` already does. `RootModel` in `root.go` is rewired to use these wrapper types instead of plain `listModel` for `dbList`/`collList`, gains new message types for create/rename/drop submission and completion, and gets cascade logic that mirrors the app's existing "cursor movement cascades to the next panel" behavior.

**Tech Stack:** Go, Bubbletea, official MongoDB Go driver v2 (`go.mongodb.org/mongo-driver/v2`).

## Global Constraints

- Databases get create + delete only (no rename — MongoDB has no native database-rename operation). Collections get create + rename (edit) + delete.
- Creating a database requires both a database name and an initial collection name in one form, submitted together — MongoDB has no standalone "create database" command; a database starts existing the moment it has its first collection.
- `RenameCollection` runs the admin command `{renameCollection: "<db>.<old>", to: "<db>.<new>"}` via `client.Database("admin").RunCommand(...)` — the Go driver has no convenience method for this. It may fail with a permissions error on a restricted Mongo user; that error surfaces through the existing `m.err` display like any other error, no special handling needed.
- No client-side duplication of MongoDB's naming-rule validation (no `$`, no leading `system.`, length limits, etc.) anywhere in this plan. Every create/rename attempt is sent to the server as-is; a rejection surfaces via the existing `m.err` mechanism.
- Delete confirmations use the existing simple y/n `confirmModel` popup for both databases and collections — no "type the exact name" confirmation.
- Deleting the currently-active database clears `db`/`coll` and resets Collections, Documents, and Indexes to empty. Deleting the currently-active collection (but not its database) clears `coll` and resets Documents and Indexes, leaving the database selection untouched. Deleting a database/collection that is NOT the active one only reloads the relevant list.
- After a successful create or rename, the list cursor is positioned on the resulting name (same "find by ID in the refreshed list" mechanism the rename-connection feature already uses in `root.go`'s `connectionUpdatedMsg` handler).
- Text entry in every new form (database name, initial collection name, new collection name, rename field) is append/backspace-at-the-end only — no arrow-key cursor movement. This matches how the Indexes create form and the Connections form both started before cursor editing was added to Connections as a separate, later feature.
- `AddConnection`, `UpdateConnection`, `DeleteConnection`, `RenameConnection` (`internal/config`), `insertIntoArray`, `replaceOrInsertInArray`, `removeFromArray`, and `internal/mongo/client_integration_test.go` are not touched by this plan.
- `internal/tui` tests must never invoke a `tea.Cmd` closure that would call a real `internal/config` function — not applicable to this plan (it doesn't touch `internal/config`), but `internal/mongo.FakeClient`-backed closures ARE safe to invoke directly in tests (in-memory only, no real network or filesystem I/O) — this is the established precedent in `root_test.go` already (e.g. `TestRootModel_ConnectionUpdatedMsgReloadsConnectionsList`'s sibling tests around Mongo operations use `mongo.NewFakeClient()` and freely call the returned `tea.Cmd`).

---

### Task 1: `mongo.Client` methods — CreateCollection, DropCollection, DropDatabase, RenameCollection

**Files:**
- Modify: `internal/mongo/client.go` (interface + `RealClient`)
- Modify: `internal/mongo/fake.go` (`FakeClient`)
- Create: `internal/mongo/fake_test.go`

**Interfaces:**
- Produces (used starting in Task 4, via `RootModel`'s `m.client`):
  - `CreateCollection(ctx context.Context, db, coll string) error`
  - `DropCollection(ctx context.Context, db, coll string) error`
  - `DropDatabase(ctx context.Context, db string) error`
  - `RenameCollection(ctx context.Context, db, oldName, newName string) error`

- [ ] **Step 1: Write the failing tests**

Create `internal/mongo/fake_test.go`:

```go
package mongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFakeClient_CreateCollectionCreatesDatabaseAndCollection(t *testing.T) {
	f := NewFakeClient()
	if err := f.CreateCollection(context.Background(), "shop", "orders"); err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	if _, ok := f.Databases["shop"]["orders"]; !ok {
		t.Fatalf("expected shop.orders to exist, got %+v", f.Databases)
	}
}

func TestFakeClient_CreateCollectionRejectsExistingCollection(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {}}

	if err := f.CreateCollection(context.Background(), "shop", "orders"); err == nil {
		t.Fatal("expected an error creating a collection that already exists")
	}
}

func TestFakeClient_DropCollectionRemovesItAndItsIndexes(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {{"_id": "1"}}}
	f.Indexes["shop"] = map[string][]IndexInfo{"orders": {{Name: "email_1"}}}

	if err := f.DropCollection(context.Background(), "shop", "orders"); err != nil {
		t.Fatalf("DropCollection failed: %v", err)
	}
	if _, ok := f.Databases["shop"]["orders"]; ok {
		t.Fatal("expected shop.orders to be gone")
	}
	if _, ok := f.Indexes["shop"]["orders"]; ok {
		t.Fatal("expected shop.orders' indexes to be gone too")
	}
}

func TestFakeClient_DropCollectionRejectsMissingCollection(t *testing.T) {
	f := NewFakeClient()
	if err := f.DropCollection(context.Background(), "shop", "ghost"); err == nil {
		t.Fatal("expected an error dropping a collection that doesn't exist")
	}
}

func TestFakeClient_DropDatabaseRemovesAllItsCollectionsAndIndexes(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {{"_id": "1"}}}
	f.Indexes["shop"] = map[string][]IndexInfo{"orders": {{Name: "email_1"}}}

	if err := f.DropDatabase(context.Background(), "shop"); err != nil {
		t.Fatalf("DropDatabase failed: %v", err)
	}
	if _, ok := f.Databases["shop"]; ok {
		t.Fatal("expected 'shop' to be gone from Databases")
	}
	if _, ok := f.Indexes["shop"]; ok {
		t.Fatal("expected 'shop' to be gone from Indexes")
	}
}

func TestFakeClient_RenameCollectionMovesDocsAndIndexesToNewName(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {{"_id": "1"}}}
	f.Indexes["shop"] = map[string][]IndexInfo{"orders": {{Name: "email_1"}}}

	if err := f.RenameCollection(context.Background(), "shop", "orders", "orders_v2"); err != nil {
		t.Fatalf("RenameCollection failed: %v", err)
	}
	if _, ok := f.Databases["shop"]["orders"]; ok {
		t.Fatal("expected old name 'orders' to be gone")
	}
	docs, ok := f.Databases["shop"]["orders_v2"]
	if !ok || len(docs) != 1 {
		t.Fatalf("expected docs moved to 'orders_v2', got %+v", f.Databases["shop"])
	}
	idxs, ok := f.Indexes["shop"]["orders_v2"]
	if !ok || len(idxs) != 1 {
		t.Fatalf("expected indexes moved to 'orders_v2', got %+v", f.Indexes["shop"])
	}
}

func TestFakeClient_RenameCollectionRejectsMissingOldName(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{}

	if err := f.RenameCollection(context.Background(), "shop", "ghost", "new"); err == nil {
		t.Fatal("expected an error renaming a collection that doesn't exist")
	}
}

func TestFakeClient_RenameCollectionRejectsCollisionWithDifferentExistingCollection(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "1"}},
		"users":  {{"_id": "2"}},
	}

	if err := f.RenameCollection(context.Background(), "shop", "orders", "users"); err == nil {
		t.Fatal("expected an error renaming 'orders' to the already-existing 'users'")
	}
	if _, ok := f.Databases["shop"]["orders"]; !ok {
		t.Fatal("expected 'orders' to still exist after the rejected rename")
	}
	if len(f.Databases["shop"]["users"]) != 1 {
		t.Fatalf("expected 'users' to be untouched after the rejected rename, got %+v", f.Databases["shop"]["users"])
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/mongo/... -run TestFakeClient_ -v`
Expected: FAIL — compile error, since `CreateCollection`/`DropCollection`/`DropDatabase`/`RenameCollection` don't exist on `FakeClient` yet.

- [ ] **Step 3: Add the methods to the `Client` interface and implement them**

In `internal/mongo/client.go`, add these four lines to the `Client` interface (right after the existing `DropIndex` line):

```go
	DropIndex(ctx context.Context, db, coll, name string) error

	CreateCollection(ctx context.Context, db, coll string) error
	DropCollection(ctx context.Context, db, coll string) error
	DropDatabase(ctx context.Context, db string) error
	RenameCollection(ctx context.Context, db, oldName, newName string) error
}
```

Then append these `RealClient` methods at the end of `internal/mongo/client.go`, after the existing `DropIndex` implementation:

```go
func (c *RealClient) CreateCollection(ctx context.Context, db, coll string) error {
	if err := c.client.Database(db).CreateCollection(ctx, coll); err != nil {
		return fmt.Errorf("creando collection %q en %q: %w", coll, db, err)
	}
	return nil
}

func (c *RealClient) DropCollection(ctx context.Context, db, coll string) error {
	if err := c.client.Database(db).Collection(coll).Drop(ctx); err != nil {
		return fmt.Errorf("borrando collection %q en %q: %w", coll, db, err)
	}
	return nil
}

func (c *RealClient) DropDatabase(ctx context.Context, db string) error {
	if err := c.client.Database(db).Drop(ctx); err != nil {
		return fmt.Errorf("borrando database %q: %w", db, err)
	}
	return nil
}

// RenameCollection has no dedicated driver method — the official Go driver
// exposes renaming only via the admin "renameCollection" command, which
// requires admin privileges on the connected Mongo user. A permissions
// error here surfaces through the returned error like any other failure.
func (c *RealClient) RenameCollection(ctx context.Context, db, oldName, newName string) error {
	fromNS := fmt.Sprintf("%s.%s", db, oldName)
	toNS := fmt.Sprintf("%s.%s", db, newName)
	cmd := bson.D{{Key: "renameCollection", Value: fromNS}, {Key: "to", Value: toNS}}
	if err := c.client.Database("admin").RunCommand(ctx, cmd).Err(); err != nil {
		return fmt.Errorf("renombrando collection %q a %q en %q: %w", oldName, newName, db, err)
	}
	return nil
}
```

In `internal/mongo/fake.go`, append these `FakeClient` methods at the end of the file, after the existing `DropIndex` implementation:

```go
func (f *FakeClient) CreateCollection(ctx context.Context, db, coll string) error {
	if f.Databases[db] == nil {
		f.Databases[db] = map[string][]bson.M{}
	}
	if _, exists := f.Databases[db][coll]; exists {
		return fmt.Errorf("la collection %q ya existe en %q", coll, db)
	}
	f.Databases[db][coll] = []bson.M{}
	return nil
}

func (f *FakeClient) DropCollection(ctx context.Context, db, coll string) error {
	if _, exists := f.Databases[db][coll]; !exists {
		return fmt.Errorf("la collection %q no existe en %q", coll, db)
	}
	delete(f.Databases[db], coll)
	delete(f.Indexes[db], coll)
	return nil
}

func (f *FakeClient) DropDatabase(ctx context.Context, db string) error {
	delete(f.Databases, db)
	delete(f.Indexes, db)
	return nil
}

func (f *FakeClient) RenameCollection(ctx context.Context, db, oldName, newName string) error {
	docs, exists := f.Databases[db][oldName]
	if !exists {
		return fmt.Errorf("la collection %q no existe en %q", oldName, db)
	}
	if _, collides := f.Databases[db][newName]; collides {
		return fmt.Errorf("ya existe una collection llamada %q en %q", newName, db)
	}
	delete(f.Databases[db], oldName)
	f.Databases[db][newName] = docs
	if idxs, ok := f.Indexes[db][oldName]; ok {
		delete(f.Indexes[db], oldName)
		f.Indexes[db][newName] = idxs
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/mongo/... -v`
Expected: PASS for every test in the package, including all 8 new `TestFakeClient_*` tests. (`go build ./...` must also succeed, since `RealClient` now implements the expanded `Client` interface too.)

- [ ] **Step 5: Commit**

```bash
git add internal/mongo/client.go internal/mongo/fake.go internal/mongo/fake_test.go
git commit -m "feat: add CreateCollection/DropCollection/DropDatabase/RenameCollection to mongo.Client"
```

---

### Task 2: `dbListModel` — create + delete for the Databases panel

**Files:**
- Modify: `internal/tui/db_collection.go`
- Modify: `internal/tui/db_collection_test.go`

**Interfaces:**
- Consumes: `listModel`, `newListModel` (`internal/tui/list.go`), `confirmModel` (`internal/tui/confirm.go`), `titleStyle`/`helpHintStyle` (styling helpers already used by `idxListModel`/`connectionPickerModel`).
- Produces (used starting in Task 4):
  - `type dbListModel struct { list listModel; creating bool; createDBName string; createCollName string; createField int; confirmingDrop bool; confirm confirmModel }`
  - `func newDbListModel(names []string) dbListModel`
  - `func (m dbListModel) Update(msg tea.Msg) (dbListModel, tea.Cmd)`
  - `func (m dbListModel) View() string`
  - `type dbCreateSubmittedMsg struct{ DBName, CollName string }`
  - `type dbDropConfirmedMsg struct{ Name string }`
- This task does NOT touch `root.go` — `newDatabaseListModel` (existing, returns plain `listModel`) is left completely unchanged and is still what `root.go` calls today. `newDbListModel` is a new, additional constructor that wraps it. Task 4 is what switches `root.go` over to `dbListModel`/`newDbListModel`.

- [ ] **Step 1: Write the failing tests**

`internal/tui/db_collection_test.go` currently starts with `import "testing"` only. The new tests below use `tea.KeyMsg`, so first replace that single-line import with:

```go
import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

Then add the following (the existing `TestNewDatabaseListModel_ListsGivenNames` test stays exactly as-is — `newDatabaseListModel` is unchanged):

```go
func TestNewDbListModel_WrapsUnderlyingDatabaseList(t *testing.T) {
	m := newDbListModel([]string{"admin", "shop"})
	if len(m.list.Items) != 2 || m.list.Items[0].ID != "admin" || m.list.Items[1].ID != "shop" {
		t.Fatalf("unexpected items: %+v", m.list.Items)
	}
}

func TestDbListModel_AOpensCreateFormWithTwoFields(t *testing.T) {
	m := newDbListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected dbListModel to enter 'creating' mode after 'a'")
	}
	if m.createField != 0 {
		t.Fatalf("expected create form to start on field 0 (database name), got %d", m.createField)
	}
}

func TestDbListModel_TabSwitchesBetweenCreateFieldsAndEnterSubmits(t *testing.T) {
	m := newDbListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	for _, r := range "shop" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "orders" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create-database form")
	}
	submitted, ok := cmd().(dbCreateSubmittedMsg)
	if !ok || submitted.DBName != "shop" || submitted.CollName != "orders" {
		t.Fatalf("expected dbCreateSubmittedMsg{DBName:\"shop\",CollName:\"orders\"}, got %#v", cmd())
	}
}

func TestDbListModel_EscCancelsCreateForm(t *testing.T) {
	m := newDbListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.creating {
		t.Fatal("expected Esc to cancel the create-database form")
	}
}

func TestDbListModel_DOpensDropConfirmationAndYEmitsDbDropConfirmedMsg(t *testing.T) {
	m := newDbListModel([]string{"shop"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.confirmingDrop {
		t.Fatal("expected dbListModel to enter confirmingDrop state after 'd'")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming drop")
	}
	dropped, ok := cmd().(dbDropConfirmedMsg)
	if !ok || dropped.Name != "shop" {
		t.Fatalf("expected dbDropConfirmedMsg{Name:\"shop\"}, got %#v", cmd())
	}
}

func TestDbListModel_TypingADuringFilterDoesNotOpenCreateOrDropForms(t *testing.T) {
	m := newDbListModel([]string{"shop", "admin"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.list.Filtering() {
		t.Fatal("expected filtering mode to be active after '/'")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.creating {
		t.Fatal("expected 'a' typed while filtering to NOT open the create-database form")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.confirmingDrop {
		t.Fatal("expected 'd' typed while filtering to NOT open the drop confirmation")
	}
	if m.list.FilterQuery() != "ad" {
		t.Fatalf("expected 'a' and 'd' added to the filter query, got %q", m.list.FilterQuery())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -run TestDbListModel -v` and `go test ./internal/tui/... -run TestNewDbListModel -v`
Expected: FAIL — compile error, `dbListModel`/`newDbListModel` don't exist yet.

- [ ] **Step 3: Implement `dbListModel`**

Add imports and the new code to `internal/tui/db_collection.go`. The full file becomes:

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func newDatabaseListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Bases de datos", items, false)
}

func newCollectionListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Colecciones", items, false)
}

type dbCreateSubmittedMsg struct{ DBName, CollName string }
type dbDropConfirmedMsg struct{ Name string }

// dbListModel wraps a listModel to add create/delete support to the
// Databases panel — the same shape connectionPickerModel already uses to
// add create/edit/delete to Connections. The shared generic listModel
// itself is untouched.
type dbListModel struct {
	list           listModel
	creating       bool
	createDBName   string
	createCollName string
	createField    int // 0 = database name, 1 = initial collection name
	confirmingDrop bool
	confirm        confirmModel
}

// newDbListModel builds a dbListModel from the given database names,
// reusing newDatabaseListModel for the underlying list construction.
func newDbListModel(names []string) dbListModel {
	return dbListModel{list: newDatabaseListModel(names)}
}

func (m dbListModel) Update(msg tea.Msg) (dbListModel, tea.Cmd) {
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
		if !result.Confirmed {
			return m, nil
		}
		if len(m.list.Items) == 0 {
			return m, nil
		}
		name := m.list.Items[m.list.Cursor].ID
		return m, func() tea.Msg { return dbDropConfirmedMsg{Name: name} }
	}

	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.creating = false
			return m, nil
		case "tab", "shift+tab":
			m.createField = (m.createField + 1) % 2
			return m, nil
		case "enter":
			dbName, collName := m.createDBName, m.createCollName
			return m, func() tea.Msg { return dbCreateSubmittedMsg{DBName: dbName, CollName: collName} }
		}
		switch keyMsg.Type {
		case tea.KeyBackspace:
			if m.createField == 0 {
				if r := []rune(m.createDBName); len(r) > 0 {
					m.createDBName = string(r[:len(r)-1])
				}
			} else {
				if r := []rune(m.createCollName); len(r) > 0 {
					m.createCollName = string(r[:len(r)-1])
				}
			}
		case tea.KeyRunes:
			if m.createField == 0 {
				m.createDBName += string(keyMsg.Runes)
			} else {
				m.createCollName += string(keyMsg.Runes)
			}
		}
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && !m.list.Filtering() {
		switch keyMsg.String() {
		case "a":
			m.creating = true
			m.createDBName = ""
			m.createCollName = ""
			m.createField = 0
			return m, nil
		case "d", "x":
			if len(m.list.Items) > 0 {
				name := m.list.Items[m.list.Cursor].ID
				m.confirmingDrop = true
				m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar la database %q? Se perderán todas sus collections y documentos.", name)}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m dbListModel) View() string {
	if m.confirmingDrop {
		return m.confirm.View()
	}
	if m.creating {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Nueva database") + "\n\n")
		dbNameText := m.createDBName
		if m.createField == 0 {
			dbNameText += "_"
		}
		b.WriteString("Database:   " + dbNameText)
		if m.createField == 0 {
			b.WriteString(" <")
		}
		collNameText := m.createCollName
		if m.createField == 1 {
			collNameText += "_"
		}
		b.WriteString("\nCollection: " + collNameText)
		if m.createField == 1 {
			b.WriteString(" <")
		}
		b.WriteString("\n\n[Tab] siguiente campo  [Enter] crear  [Esc] cancelar")
		return b.String()
	}
	// Mirrors idxListModel/connectionPickerModel's normal-mode View(): this
	// branch is only reached if something calls dbListModel.View() outside
	// root.go's popup-gated paths, which nothing does today (the sidebar
	// panel renders via renderPanel + labelsFromListModel(m.list) instead)
	// — kept for shape-consistency with those two models, not because it's
	// currently reachable.
	return m.list.View() + "\n" + helpHintStyle.Render("[a] crear database  [d] borrar")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS for every test in the package, including the 5 new tests and the pre-existing `TestNewDatabaseListModel_ListsGivenNames`.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/db_collection.go internal/tui/db_collection_test.go
git commit -m "feat: add dbListModel with create/delete for the Databases panel"
```

---

### Task 3: `collListModel` — create + rename + delete for the Collections panel

**Files:**
- Modify: `internal/tui/db_collection.go`
- Modify: `internal/tui/db_collection_test.go`

**Interfaces:**
- Consumes: same as Task 2 (`listModel`, `confirmModel`, styling helpers).
- Produces (used starting in Task 4):
  - `type collListModel struct { list listModel; creating bool; createName string; editing bool; editOriginalName string; editName string; confirmingDrop bool; confirm confirmModel }`
  - `func newCollListModel(names []string) collListModel`
  - `func (m collListModel) Update(msg tea.Msg) (collListModel, tea.Cmd)`
  - `func (m collListModel) View() string`
  - `type collCreateSubmittedMsg struct{ Name string }`
  - `type collRenameSubmittedMsg struct{ OldName, NewName string }`
  - `type collDropConfirmedMsg struct{ Name string }`
- Same non-goal as Task 2: `newCollectionListModel` (existing, plain `listModel`) is untouched; `root.go` still calls it directly until Task 4.

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/db_collection_test.go`:

```go
func TestNewCollListModel_WrapsUnderlyingCollectionList(t *testing.T) {
	m := newCollListModel([]string{"orders", "users"})
	if len(m.list.Items) != 2 || m.list.Items[0].ID != "orders" || m.list.Items[1].ID != "users" {
		t.Fatalf("unexpected items: %+v", m.list.Items)
	}
}

func TestCollListModel_AOpensCreateFormAndEnterSubmits(t *testing.T) {
	m := newCollListModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected collListModel to enter 'creating' mode after 'a'")
	}

	for _, r := range "logs" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create-collection form")
	}
	submitted, ok := cmd().(collCreateSubmittedMsg)
	if !ok || submitted.Name != "logs" {
		t.Fatalf("expected collCreateSubmittedMsg{Name:\"logs\"}, got %#v", cmd())
	}
}

func TestCollListModel_EOpensRenameFormPrefilledAndEnterSubmits(t *testing.T) {
	m := newCollListModel([]string{"orders"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !m.editing {
		t.Fatal("expected collListModel to enter 'editing' mode after 'e'")
	}
	if m.editName != "orders" {
		t.Fatalf("expected rename form pre-filled with 'orders', got %q", m.editName)
	}

	for _, r := range "2" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the rename form")
	}
	submitted, ok := cmd().(collRenameSubmittedMsg)
	if !ok || submitted.OldName != "orders" || submitted.NewName != "orders2" {
		t.Fatalf("expected collRenameSubmittedMsg{OldName:\"orders\",NewName:\"orders2\"}, got %#v", cmd())
	}
}

func TestCollListModel_EscCancelsCreateAndRenameForms(t *testing.T) {
	m := newCollListModel([]string{"orders"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.creating {
		t.Fatal("expected Esc to cancel the create-collection form")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.editing {
		t.Fatal("expected Esc to cancel the rename-collection form")
	}
}

func TestCollListModel_DOpensDropConfirmationAndYEmitsCollDropConfirmedMsg(t *testing.T) {
	m := newCollListModel([]string{"orders"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.confirmingDrop {
		t.Fatal("expected collListModel to enter confirmingDrop state after 'd'")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming drop")
	}
	dropped, ok := cmd().(collDropConfirmedMsg)
	if !ok || dropped.Name != "orders" {
		t.Fatalf("expected collDropConfirmedMsg{Name:\"orders\"}, got %#v", cmd())
	}
}

func TestCollListModel_TypingAEDDuringFilterDoesNotOpenAnyForm(t *testing.T) {
	m := newCollListModel([]string{"orders", "users"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.list.Filtering() {
		t.Fatal("expected filtering mode to be active after '/'")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.creating {
		t.Fatal("expected 'a' typed while filtering to NOT open the create-collection form")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.editing {
		t.Fatal("expected 'e' typed while filtering to NOT open the rename form")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.confirmingDrop {
		t.Fatal("expected 'd' typed while filtering to NOT open the drop confirmation")
	}
	if m.list.FilterQuery() != "aed" {
		t.Fatalf("expected 'a','e','d' added to the filter query, got %q", m.list.FilterQuery())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -run TestCollListModel -v` and `go test ./internal/tui/... -run TestNewCollListModel -v`
Expected: FAIL — compile error, `collListModel`/`newCollListModel` don't exist yet.

- [ ] **Step 3: Implement `collListModel`**

Append to `internal/tui/db_collection.go` (after the `dbListModel` code from Task 2):

```go
type collCreateSubmittedMsg struct{ Name string }
type collRenameSubmittedMsg struct{ OldName, NewName string }
type collDropConfirmedMsg struct{ Name string }

// collListModel wraps a listModel the same way dbListModel does, adding
// create/rename/delete support to the Collections panel. Renaming a
// collection needs both the name at the moment editing started
// (editOriginalName) and whatever the user has typed into editName since,
// exactly like connectionPickerModel's editingOriginalName distinguishes a
// rename from a same-name resubmit.
type collListModel struct {
	list             listModel
	creating         bool
	createName       string
	editing          bool
	editOriginalName string
	editName         string
	confirmingDrop   bool
	confirm          confirmModel
}

// newCollListModel builds a collListModel from the given collection
// names, reusing newCollectionListModel for the underlying list
// construction.
func newCollListModel(names []string) collListModel {
	return collListModel{list: newCollectionListModel(names)}
}

func (m collListModel) Update(msg tea.Msg) (collListModel, tea.Cmd) {
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
		if !result.Confirmed {
			return m, nil
		}
		if len(m.list.Items) == 0 {
			return m, nil
		}
		name := m.list.Items[m.list.Cursor].ID
		return m, func() tea.Msg { return collDropConfirmedMsg{Name: name} }
	}

	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.creating = false
			return m, nil
		case "enter":
			name := m.createName
			return m, func() tea.Msg { return collCreateSubmittedMsg{Name: name} }
		}
		switch keyMsg.Type {
		case tea.KeyBackspace:
			if r := []rune(m.createName); len(r) > 0 {
				m.createName = string(r[:len(r)-1])
			}
		case tea.KeyRunes:
			m.createName += string(keyMsg.Runes)
		}
		return m, nil
	}

	if m.editing {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.editing = false
			return m, nil
		case "enter":
			oldName, newName := m.editOriginalName, m.editName
			return m, func() tea.Msg { return collRenameSubmittedMsg{OldName: oldName, NewName: newName} }
		}
		switch keyMsg.Type {
		case tea.KeyBackspace:
			if r := []rune(m.editName); len(r) > 0 {
				m.editName = string(r[:len(r)-1])
			}
		case tea.KeyRunes:
			m.editName += string(keyMsg.Runes)
		}
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && !m.list.Filtering() {
		switch keyMsg.String() {
		case "a":
			m.creating = true
			m.createName = ""
			return m, nil
		case "e":
			if len(m.list.Items) > 0 {
				name := m.list.Items[m.list.Cursor].ID
				m.editing = true
				m.editOriginalName = name
				m.editName = name
			}
			return m, nil
		case "d", "x":
			if len(m.list.Items) > 0 {
				name := m.list.Items[m.list.Cursor].ID
				m.confirmingDrop = true
				m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar la collection %q? Se perderán todos sus documentos.", name)}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m collListModel) View() string {
	if m.confirmingDrop {
		return m.confirm.View()
	}
	if m.creating {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Nueva collection") + "\n\n")
		b.WriteString("Nombre: " + m.createName + "_ <")
		b.WriteString("\n\n[Enter] crear  [Esc] cancelar")
		return b.String()
	}
	if m.editing {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Renombrar collection") + "\n\n")
		b.WriteString("Nombre: " + m.editName + "_ <")
		b.WriteString("\n\n[Enter] guardar  [Esc] cancelar")
		return b.String()
	}
	// Same shape-consistency note as dbListModel.View() above — unreachable
	// via root.go's current popup-gated dispatch, kept for consistency.
	return m.list.View() + "\n" + helpHintStyle.Render("[a] crear  [e] renombrar  [d] borrar")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS for every test in the package, including the 6 new tests from this task plus everything from Task 2.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/db_collection.go internal/tui/db_collection_test.go
git commit -m "feat: add collListModel with create/rename/delete for the Collections panel"
```

---

### Task 4: Wire `dbListModel`/`collListModel` into `RootModel`

**Files:**
- Modify: `internal/tui/root.go`
- Modify: `internal/tui/root_test.go`
- Modify: `internal/tui/help.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `dbListModel`/`newDbListModel` (Task 2), `collListModel`/`newCollListModel` (Task 3), `CreateCollection`/`DropCollection`/`DropDatabase`/`RenameCollection` (Task 1), `dbCreateSubmittedMsg`/`dbDropConfirmedMsg`/`collCreateSubmittedMsg`/`collRenameSubmittedMsg`/`collDropConfirmedMsg` (Tasks 2/3).
- Produces: nothing consumed elsewhere — this is the last task in the plan.

This task has several distinct edits to the same two files (`root.go`/`root_test.go`). Apply them in the order below; the file won't compile until all of them are in place, so there's a single Step 3 covering the whole set, followed by one test run.

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/root_test.go` (anywhere after `newTestRootModel`'s definition):

```go
func TestRootModel_DbCreateSubmittedMsgCreatesCollectionAndSelectsNewDatabase(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.dbList = newDbListModel([]string{"shop"})

	model, cmd := r.Update(dbCreateSubmittedMsg{DBName: "reports", CollName: "events"})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after dbCreateSubmittedMsg")
	}
	model, cmd = r.Update(cmd())
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after dbCreateCompletedMsg to reload databases")
	}
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["reports"]["events"]; !ok {
		t.Fatalf("expected FakeClient to have created reports.events, got %+v", fake.Databases)
	}
	found := false
	for _, item := range r.dbList.list.Items {
		if item.ID == "reports" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'reports' in the reloaded databases list, got %+v", r.dbList.list.Items)
	}
	if r.db != "reports" {
		t.Fatalf("expected active database to become 'reports', got %q", r.db)
	}
}

func TestRootModel_DbDropCompletedMsgOnActiveDatabaseClearsDependentPanels(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.dbList = newDbListModel([]string{"shop"})
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(dbDropConfirmedMsg{Name: "shop"})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after dbDropConfirmedMsg")
	}
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["shop"]; ok {
		t.Fatalf("expected FakeClient to have dropped 'shop', got %+v", fake.Databases)
	}
	if r.db != "" || r.coll != "" {
		t.Fatalf("expected db/coll cleared after dropping the active database, got db=%q coll=%q", r.db, r.coll)
	}
	if len(r.collList.list.Items) != 0 {
		t.Fatalf("expected collections list cleared, got %+v", r.collList.list.Items)
	}
}

func TestRootModel_DbDropCompletedMsgOnNonActiveDatabaseOnlyReloadsList(t *testing.T) {
	root, fake := newTestRootModel()
	fake.Databases["archive"] = map[string][]bson.M{"old": {}}
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.dbList = newDbListModel([]string{"shop", "archive"})
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(dbDropConfirmedMsg{Name: "archive"})
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if r.db != "shop" || r.coll != "orders" {
		t.Fatalf("expected db/coll untouched when dropping a non-active database, got db=%q coll=%q", r.db, r.coll)
	}
	if len(r.collList.list.Items) != 1 {
		t.Fatalf("expected collections list untouched, got %+v", r.collList.list.Items)
	}
}

func TestRootModel_CollCreateSubmittedMsgCreatesAndSelectsNewCollection(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.db = "shop"
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(collCreateSubmittedMsg{Name: "logs"})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after collCreateSubmittedMsg")
	}
	model, cmd = r.Update(cmd())
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after collCreateCompletedMsg to reload collections")
	}
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["shop"]["logs"]; !ok {
		t.Fatalf("expected FakeClient to have created shop.logs, got %+v", fake.Databases["shop"])
	}
	if r.coll != "logs" {
		t.Fatalf("expected active collection to become 'logs', got %q", r.coll)
	}
}

func TestRootModel_CollRenameSubmittedMsgRenamesAndFollowsCursor(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(collRenameSubmittedMsg{OldName: "orders", NewName: "orders_v2"})
	r = model.(RootModel)
	model, cmd = r.Update(cmd())
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["shop"]["orders_v2"]; !ok {
		t.Fatalf("expected FakeClient to have renamed to shop.orders_v2, got %+v", fake.Databases["shop"])
	}
	if r.coll != "orders_v2" {
		t.Fatalf("expected active collection to follow the rename to 'orders_v2', got %q", r.coll)
	}
}

func TestRootModel_CollDropCompletedMsgOnActiveCollectionClearsDocumentsAndIndexes(t *testing.T) {
	root, _ := newTestRootModel()
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.collList = newCollListModel([]string{"orders"})
	r.docList = newDocListModel([]bson.M{{"_id": "o1"}}, 1, 0, pageSize)

	model, cmd := r.Update(collDropConfirmedMsg{Name: "orders"})
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if r.coll != "" {
		t.Fatalf("expected coll cleared after dropping the active collection, got %q", r.coll)
	}
	if len(r.docList.docs) != 0 {
		t.Fatalf("expected documents cleared, got %+v", r.docList.docs)
	}
}

func TestRootModel_CollDropCompletedMsgOnNonActiveCollectionOnlyReloadsList(t *testing.T) {
	root, fake := newTestRootModel()
	fake.Databases["shop"]["logs"] = []bson.M{}
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.collList = newCollListModel([]string{"orders", "logs"})
	r.docList = newDocListModel([]bson.M{{"_id": "o1"}}, 1, 0, pageSize)

	model, cmd := r.Update(collDropConfirmedMsg{Name: "logs"})
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if r.coll != "orders" {
		t.Fatalf("expected coll untouched when dropping a non-active collection, got %q", r.coll)
	}
	if len(r.docList.docs) != 1 {
		t.Fatalf("expected documents untouched, got %+v", r.docList.docs)
	}
}
```

Now migrate every existing `root_test.go` reference to `dbList`/`collList` fields to go through the new `.list` field, EXCEPT bare assignments (those already work unchanged, since `newDatabaseListModel`/`newCollectionListModel` are untouched and `root.dbList`/`root.collList` will become `dbListModel`/`collListModel` — Step 3 below is what changes the field types).

Apply this rule across `internal/tui/root_test.go`:
- Every `.dbList.Items` → `.dbList.list.Items`
- Every `.dbList.Cursor` → `.dbList.list.Cursor`
- Every `.dbList.Filtering()` → `.dbList.list.Filtering()`
- Every `.dbList.FilterQuery()` → `.dbList.list.FilterQuery()`
- Every `.collList.Items` → `.collList.list.Items`
- Every `.collList.Cursor` → `.collList.list.Cursor`
- The two `driveCursorToItem(t, root, root.dbList, ...)` / `driveCursorToItem(t, root, root.collList, ...)` calls → `driveCursorToItem(t, root, root.dbList.list, ...)` / `driveCursorToItem(t, root, root.collList.list, ...)` (the helper itself, `func driveCursorToItem(t *testing.T, root RootModel, list listModel, id string) RootModel`, is unchanged — it already takes a plain `listModel`)
- Do NOT change bare assignments like `root.dbList = newDatabaseListModel(...)` or `root.collList = newCollectionListModel(...)` — Step 3 changes these two call sites to `newDbListModel(...)`/`newCollListModel(...)` instead, at which point the assignment is still correct as-is (assigning the whole wrapper, no `.list` needed on the left side).

After applying the rule, verify completeness with:

```bash
grep -n '\.dbList\.\|\.collList\.\|root\.dbList\b\|root\.collList\b\|r\.dbList\b\|r\.collList\b' internal/tui/root_test.go
```

Every remaining match must be one of: `.dbList.list.` / `.collList.list.` (a correctly-migrated field access), or `root.dbList = ` / `r.dbList = ` / `root.collList = ` / `r.collList = ` (a constructor assignment, left as-is per the rule above). If any other shape remains, it was missed — fix it.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -v 2>&1 | head -50`
Expected: FAIL — compile errors (`dbCreateSubmittedMsg`/`dbDropConfirmedMsg`/`collCreateSubmittedMsg`/etc. are unresolved in the new tests; `root.dbList` is still `listModel` so `.list.Items` doesn't exist yet on it in the migrated existing tests).

- [ ] **Step 3: Wire everything into `root.go`**

Apply these changes to `internal/tui/root.go`, in order:

**3a. Struct field types** — in the `RootModel` struct, replace:
```go
	dbList     listModel
	collList   listModel
```
with:
```go
	dbList     dbListModel
	collList   collListModel
```

**3b. `connectedMsg` handler** — replace:
```go
		m.dbList = newDatabaseListModel(msg.Databases)
```
with:
```go
		m.dbList = newDbListModel(msg.Databases)
```

**3c. `inTextEntry()`** — replace:
```go
	case m.focus == panelDatabases && m.dbList.Filtering():
		return true
	case m.focus == panelCollections && m.collList.Filtering():
		return true
```
with:
```go
	case m.focus == panelDatabases && m.dbList.list.Filtering():
		return true
	case m.focus == panelDatabases && m.dbList.creating:
		return true
	case m.focus == panelCollections && m.collList.list.Filtering():
		return true
	case m.focus == panelCollections && m.collList.creating:
		return true
	case m.focus == panelCollections && m.collList.editing:
		return true
```

**3d. `collectionsLoadedMsg` type and `loadCollections`** — replace:
```go
type collectionsLoadedMsg struct {
	Collections []string
	Err         error
}

func (m RootModel) loadCollections() tea.Cmd {
	client, db := m.client, m.db
	return func() tea.Msg {
		names, err := client.ListCollections(context.Background(), db)
		return collectionsLoadedMsg{Collections: names, Err: err}
	}
}
```
with:
```go
type collectionsLoadedMsg struct {
	Collections []string
	Err         error
	SelectName  string
}

// loadCollections refreshes the Collections panel. When selectName is
// non-empty, the resulting handler positions the cursor on it and
// cascades into loadIndexes/loadDocuments, mirroring what moving the
// Collections cursor there by hand would do. Manual cursor movement
// (case panelCollections below) always passes "" — it already does its
// own cascade inline.
func (m RootModel) loadCollections(selectName string) tea.Cmd {
	client, db := m.client, m.db
	return func() tea.Msg {
		names, err := client.ListCollections(context.Background(), db)
		return collectionsLoadedMsg{Collections: names, Err: err, SelectName: selectName}
	}
}
```

**3e. Add `loadDatabases` and `databasesLoadedMsg`** — insert right after `connectAndListDatabases`'s closing brace (before `type collectionsLoadedMsg`):
```go
type databasesLoadedMsg struct {
	Databases []string
	Err       error
	SelectDB  string
}

// loadDatabases refreshes the Databases panel without reconnecting
// (unlike connectAndListDatabases, used only for the initial connect).
// When selectDB is non-empty, the resulting handler positions the cursor
// on it and cascades into loadCollections, mirroring what moving the
// Databases cursor there by hand would do.
func (m RootModel) loadDatabases(selectDB string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		names, err := client.ListDatabases(context.Background())
		return databasesLoadedMsg{Databases: names, Err: err, SelectDB: selectDB}
	}
}
```

**3f. `collectionsLoadedMsg` case handler** — replace:
```go
	case collectionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.collList = newCollectionListModel(msg.Collections)
		return m, nil
```
with:
```go
	case databasesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.dbList = newDbListModel(msg.Databases)
		if msg.SelectDB == "" {
			return m, nil
		}
		for i, item := range m.dbList.list.Items {
			if item.ID == msg.SelectDB {
				m.dbList.list.Cursor = i
				break
			}
		}
		m.db = msg.SelectDB
		return m, m.loadCollections("")

	case dbCreateSubmittedMsg:
		client, dbName, collName := m.client, msg.DBName, msg.CollName
		m.logf("Creando database %q (collection inicial %q)", dbName, collName)
		return m, func() tea.Msg {
			err := client.CreateCollection(context.Background(), dbName, collName)
			return dbCreateCompletedMsg{Err: err, DBName: dbName}
		}

	case dbCreateCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadDatabases(msg.DBName)

	case dbDropConfirmedMsg:
		client, name := m.client, msg.Name
		m.logf("Borrando database %q", name)
		return m, func() tea.Msg {
			err := client.DropDatabase(context.Background(), name)
			return dbDropCompletedMsg{Err: err, Name: name}
		}

	case dbDropCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		if msg.Name == m.db {
			m.db = ""
			m.coll = ""
			m.page = 0
			m.filter = nil
			m.collList = newCollListModel(nil)
			m.docList = newDocListModel(nil, 0, 0, pageSize)
			m.idxList = newIdxListModel(nil)
		}
		return m, m.loadDatabases("")

	case collectionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.collList = newCollListModel(msg.Collections)
		if msg.SelectName == "" {
			return m, nil
		}
		for i, item := range m.collList.list.Items {
			if item.ID == msg.SelectName {
				m.collList.list.Cursor = i
				break
			}
		}
		m.coll = msg.SelectName
		m.page = 0
		m.filter = nil
		m.docList.filter = ""
		m.docList.filterCursor = 0
		return m, tea.Batch(m.loadIndexes(), m.loadDocuments(bson.M{}))

	case collCreateSubmittedMsg:
		client, db, name := m.client, m.db, msg.Name
		m.logf("Creando collection %q en %q", name, db)
		return m, func() tea.Msg {
			err := client.CreateCollection(context.Background(), db, name)
			return collCreateCompletedMsg{Err: err, Name: name}
		}

	case collCreateCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadCollections(msg.Name)

	case collRenameSubmittedMsg:
		client, db, oldName, newName := m.client, m.db, msg.OldName, msg.NewName
		m.logf("Renombrando collection %q a %q en %q", oldName, newName, db)
		return m, func() tea.Msg {
			err := client.RenameCollection(context.Background(), db, oldName, newName)
			return collRenameCompletedMsg{Err: err, NewName: newName}
		}

	case collRenameCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadCollections(msg.NewName)

	case collDropConfirmedMsg:
		client, db, name := m.client, m.db, msg.Name
		m.logf("Borrando collection %q en %q", name, db)
		return m, func() tea.Msg {
			err := client.DropCollection(context.Background(), db, name)
			return collDropCompletedMsg{Err: err, Name: name}
		}

	case collDropCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		if msg.Name == m.coll {
			m.coll = ""
			m.page = 0
			m.filter = nil
			m.docList = newDocListModel(nil, 0, 0, pageSize)
			m.idxList = newIdxListModel(nil)
		}
		return m, m.loadCollections("")
```

Also add the five completion message types right after the existing
`docWriteCompletedMsg`/`indexWriteCompletedMsg` declarations
(`type docWriteCompletedMsg struct{ Err error }` /
`type indexWriteCompletedMsg struct{ Err error }`, found just above the
`inTextEntry` method):
```go
type dbCreateCompletedMsg struct {
	Err    error
	DBName string
}
type dbDropCompletedMsg struct {
	Err  error
	Name string
}
type collCreateCompletedMsg struct {
	Err  error
	Name string
}
type collRenameCompletedMsg struct {
	Err     error
	NewName string
}
type collDropCompletedMsg struct {
	Err  error
	Name string
}
```

**3g. `case panelDatabases:`/`case panelCollections:`** — replace:
```go
	case panelDatabases:
		// listModel.Update still emits itemSelectedMsg on Enter, but we intentionally
		// don't handle it here: cursor movement above already cascades into loading
		// collections for the highlighted database, so Enter has nothing left to do.
		// Do not "fix" this into an itemSelectedMsg handler.
		beforeID := highlightedItemID(m.dbList.Items, m.dbList.Cursor)
		var listCmd tea.Cmd
		m.dbList, listCmd = m.dbList.Update(msg)
		afterID := highlightedItemID(m.dbList.Items, m.dbList.Cursor)
		if afterID != "" && afterID != beforeID {
			m.db = afterID
			return m, m.loadCollections()
		}
		return m, listCmd

	case panelCollections:
		// Same as panelDatabases above: Enter is a silent no-op by design, since
		// cursor movement already cascades into loading indexes/documents for the
		// highlighted collection.
		beforeID := highlightedItemID(m.collList.Items, m.collList.Cursor)
		var listCmd tea.Cmd
		m.collList, listCmd = m.collList.Update(msg)
		afterID := highlightedItemID(m.collList.Items, m.collList.Cursor)
		if afterID != "" && afterID != beforeID {
			m.coll = afterID
			m.page = 0
			m.filter = nil
			// documentsLoadedMsg now carries docList.filter across a reload
			// (see its handler) so a stale filter from the previous
			// collection doesn't linger — clear it explicitly here too.
			m.docList.filter = ""
			m.docList.filterCursor = 0
			return m, tea.Batch(m.loadIndexes(), m.loadDocuments(bson.M{}))
		}
		return m, listCmd
```
with:
```go
	case panelDatabases:
		// listModel.Update still emits itemSelectedMsg on Enter, but we intentionally
		// don't handle it here: cursor movement above already cascades into loading
		// collections for the highlighted database, so Enter has nothing left to do.
		// Do not "fix" this into an itemSelectedMsg handler.
		beforeID := highlightedItemID(m.dbList.list.Items, m.dbList.list.Cursor)
		var listCmd tea.Cmd
		m.dbList, listCmd = m.dbList.Update(msg)
		afterID := highlightedItemID(m.dbList.list.Items, m.dbList.list.Cursor)
		if afterID != "" && afterID != beforeID {
			m.db = afterID
			return m, m.loadCollections("")
		}
		return m, listCmd

	case panelCollections:
		// Same as panelDatabases above: Enter is a silent no-op by design, since
		// cursor movement already cascades into loading indexes/documents for the
		// highlighted collection.
		beforeID := highlightedItemID(m.collList.list.Items, m.collList.list.Cursor)
		var listCmd tea.Cmd
		m.collList, listCmd = m.collList.Update(msg)
		afterID := highlightedItemID(m.collList.list.Items, m.collList.list.Cursor)
		if afterID != "" && afterID != beforeID {
			m.coll = afterID
			m.page = 0
			m.filter = nil
			// documentsLoadedMsg now carries docList.filter across a reload
			// (see its handler) so a stale filter from the previous
			// collection doesn't linger — clear it explicitly here too.
			m.docList.filter = ""
			m.docList.filterCursor = 0
			return m, tea.Batch(m.loadIndexes(), m.loadDocuments(bson.M{}))
		}
		return m, listCmd
```

**3h. `View()` popup gating** — replace:
```go
	if m.focus == panelConnections && (m.connPicker.creating || m.connPicker.editing || m.connPicker.confirmingDelete) {
		return renderPopupOverlay(m.connPicker.View(), m.width, m.height)
	}
	if m.focus == panelIndexes && (m.idxList.creating || m.idxList.confirmingDrop) {
		return renderPopupOverlay(m.idxList.View(), m.width, m.height)
	}
```
with:
```go
	if m.focus == panelConnections && (m.connPicker.creating || m.connPicker.editing || m.connPicker.confirmingDelete) {
		return renderPopupOverlay(m.connPicker.View(), m.width, m.height)
	}
	if m.focus == panelIndexes && (m.idxList.creating || m.idxList.confirmingDrop) {
		return renderPopupOverlay(m.idxList.View(), m.width, m.height)
	}
	if m.focus == panelDatabases && (m.dbList.creating || m.dbList.confirmingDrop) {
		return renderPopupOverlay(m.dbList.View(), m.width, m.height)
	}
	if m.focus == panelCollections && (m.collList.creating || m.collList.editing || m.collList.confirmingDrop) {
		return renderPopupOverlay(m.collList.View(), m.width, m.height)
	}
```

**3i. `View()` title/filter and panel rendering** — replace:
```go
	dbTitle := "Databases"
	if m.dbList.Filtering() {
		dbTitle = "Databases — Buscar: " + m.dbList.FilterQuery() + "_"
	}
	collTitle := "Collections"
	if m.collList.Filtering() {
		collTitle = "Collections — Buscar: " + m.collList.FilterQuery() + "_"
	}
```
with:
```go
	dbTitle := "Databases"
	if m.dbList.list.Filtering() {
		dbTitle = "Databases — Buscar: " + m.dbList.list.FilterQuery() + "_"
	}
	collTitle := "Collections"
	if m.collList.list.Filtering() {
		collTitle = "Collections — Buscar: " + m.collList.list.FilterQuery() + "_"
	}
```

and replace:
```go
	p2 := renderPanel(2, dbTitle, labelsFromListModel(m.dbList), m.dbList.Cursor, m.focus == panelDatabases, sidebarWidth, panelHeight)
	p3 := renderPanel(3, collTitle, labelsFromListModel(m.collList), m.collList.Cursor, m.focus == panelCollections, sidebarWidth, panelHeight)
```
with:
```go
	p2 := renderPanel(2, dbTitle, labelsFromListModel(m.dbList.list), m.dbList.list.Cursor, m.focus == panelDatabases, sidebarWidth, panelHeight)
	p3 := renderPanel(3, collTitle, labelsFromListModel(m.collList.list), m.collList.list.Cursor, m.focus == panelCollections, sidebarWidth, panelHeight)
```

**3j. `help.go`** — replace:
```
i, a           insertar documento / crear conexión o índice
e              editar campo (en detalle de documento) / editar conexión (nombre, URI y color, en Conexiones)
d, x           borrar (siempre pide confirmación)
```
with:
```
i, a           insertar documento / crear conexión, índice, database o collection
e              editar campo (en detalle de documento) / editar conexión (nombre, URI y color) / renombrar collection
d, x           borrar (siempre pide confirmación)
```

**3k. `README.md`** — replace:
```
| `i`, `a` | insert document / create connection or index |
| `e` | edit field inline (document detail popup) / edit a connection's URI and color (Conexiones panel — name stays fixed) |
| `d`, `x` | delete (always confirms) |
```
with:
```
| `i`, `a` | insert document / create connection, index, database, or collection |
| `e` | edit field inline (document detail popup) / edit a connection's name, URI, and color / rename a collection |
| `d`, `x` | delete (always confirms) |
```
(The connection's Name field became editable in an earlier feature; this README line was never updated then — fixing it now since it's the same line being touched for this feature anyway.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go build ./... && go test ./... -v 2>&1 | tail -80`
Expected: build succeeds; every test in every package passes, including the 7 new `TestRootModel_*` tests from Step 1 and every pre-existing test in `root_test.go` (migrated field accesses) and the rest of the suite.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go internal/tui/help.go README.md
git commit -m "feat: wire database/collection create, rename, delete into RootModel"
```

---

## Final check

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.
