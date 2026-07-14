# lazymongo — Create/Edit/Delete for Databases and Collections

Date: 2026-07-13
Status: Approved

## Overview

The Databases and Collections panels are currently read-only navigation
(cursor movement + fuzzy filter only). This adds create, rename, and delete
support to both, mirroring the patterns already established for the
Indexes panel (`idxListModel`'s inline create/drop forms) and the
Connections panel (`connectionPickerModel`'s wrapped-`listModel` shape,
`renderPopupOverlay` rendering, and the "cursor follows the item you just
created/renamed" behavior from the recent rename-connection feature).

MongoDB has no explicit "create database" or "rename database" command — a
database only exists once it has at least one collection, and there is no
native database-rename operation. Collections, by contrast, support a real
rename via the admin `renameCollection` command. This shapes the scope
below: **Databases get create + delete. Collections get create + rename
(edit) + delete.**

## Client layer

New methods on the `Client` interface in `internal/mongo/client.go`,
alongside the existing `ListDatabases`/`ListCollections`/index methods,
implemented in both `RealClient` and `FakeClient`:

```go
CreateCollection(ctx context.Context, db, coll string) error
DropCollection(ctx context.Context, db, coll string) error
DropDatabase(ctx context.Context, db string) error
RenameCollection(ctx context.Context, db, oldName, newName string) error
```

- `CreateCollection` → `client.Database(db).CreateCollection(ctx, coll)`.
  This is how a database starts existing in Mongo — there is no separate
  "create database" call, which is why creating a database in the UI
  requires both a database name and an initial collection name (see
  below).
- `DropCollection` → `client.Database(db).Collection(coll).Drop(ctx)`.
- `DropDatabase` → `client.Database(db).Drop(ctx)`.
- `RenameCollection` — the Go driver has no convenience method for this;
  implemented by running the admin command
  `{renameCollection: "<db>.<oldName>", to: "<db>.<newName>"}` via
  `client.Database("admin").RunCommand(ctx, ...)`. This requires admin
  privileges on the Mongo server/user; on a restricted user (e.g. some
  managed-Mongo tiers) this can fail with a permissions error — that
  error surfaces through the existing `m.err` display mechanism like any
  other Mongo error. Not blocking for this feature, just a real
  operational caveat worth documenting.
- No client-side duplication of Mongo's naming rules (no `$`, no leading
  `system.`, length limits, etc.). The operation is attempted directly;
  if the server rejects it (invalid name, already exists, insufficient
  permissions), the error surfaces via `m.err`, exactly like existing
  index/filter error handling.
- `FakeClient` implements all four against its in-memory `Databases
  map[string]map[string][]bson.M` (and whatever index-tracking structure
  it already uses for `CreateIndex`/`DropIndex`), so tests never touch a
  real Mongo server.

## Databases panel: `dbListModel`

New type in `internal/tui`, wrapping a `list listModel` field — the same
shape as `connectionPickerModel`, not a modification of the shared generic
`listModel` (which `dbList`/`collList` currently use directly, and which
`connectionPickerModel` and other consumers also rely on unmodified).

```go
type dbListModel struct {
    list           listModel
    creating       bool
    createDBName   string
    createCollName string
    createField    int // 0 = database name, 1 = initial collection name
    confirmingDrop bool
    confirm        confirmModel
}
```

- `a` opens the create form: two fields (new database name, its first
  collection's name). `Tab`/`Shift+Tab` switches focus between the two
  fields. `Enter` submits (emits `dbCreateSubmittedMsg`). `Esc` cancels
  without creating anything.
- `d`/`x` — if a database is selected, opens a confirmation
  (`confirmModel`, same y/n popup already used throughout the app) with
  the message `"¿Borrar la database %q? Se perderán todas sus collections
  y documentos."`. Confirming emits `dbDropConfirmedMsg`.
- Text entry in the create form is append/backspace-at-the-end only (no
  arrow-key cursor movement) — matching how the index-create and
  connection forms started before cursor editing was added to connections
  as a separate, later feature. Cursor-based editing for these new fields
  is out of scope here (see below).
- Rendering: `root.go` checks
  `m.focus == panelDatabases && (m.dbList.creating || m.dbList.confirmingDrop)`
  and renders `m.dbList.View()` via the existing `renderPopupOverlay`,
  exactly like the Indexes and Connections panels already do — no new
  rendering mechanism needed.

## Collections panel: `collListModel`

Same approach as `dbListModel` — wraps `list listModel`, doesn't touch the
shared generic type.

```go
type collListModel struct {
    list           listModel
    creating       bool
    createName     string
    editing        bool
    editName       string
    confirmingDrop bool
    confirm        confirmModel
}
```

- `a` opens the create form: a single field (name of the new collection,
  created inside the currently-selected database). `Enter` submits (emits
  `collCreateSubmittedMsg`). `Esc` cancels.
- `e` opens the rename form: a single field, pre-filled with the currently
  selected collection's name (same convention as the connection edit
  form — the field starts with the current value, ready to keep typing).
  `Enter` submits (emits `collRenameSubmittedMsg`). `Esc` cancels without
  renaming.
- `d`/`x` — confirmation popup with `"¿Borrar la collection %q? Se
  perderán todos sus documentos."`. Confirming emits
  `collDropConfirmedMsg`.
- Same rendering mechanism:
  `m.focus == panelCollections && (m.collList.creating || m.collList.editing || m.collList.confirmingDrop)`
  → `renderPopupOverlay`.
- Same text-entry scope as Databases: append/backspace at the end only,
  no cursor movement.

## Wiring in `root.go`

New message types, following the existing pattern used for indexes and
connections:

```go
type dbCreateSubmittedMsg struct{ DBName, CollName string }
type dbDropConfirmedMsg struct{ Name string }
type dbWriteCompletedMsg struct{ Err error; CreatedName string }

type collCreateSubmittedMsg struct{ Name string }
type collRenameSubmittedMsg struct{ OldName, NewName string }
type collDropConfirmedMsg struct{ Name string }
type collWriteCompletedMsg struct{ Err error; ResultName string }
```

`CreatedName`/`ResultName` carry the final name so the completion handler
can reposition the cursor onto it, mirroring how `connectionUpdatedMsg.Conn.Name`
is used today for the rename-connection cursor-follow behavior.

Cascades:

- **Create database**: on a successful `dbWriteCompletedMsg`, reload the
  databases list and position the cursor on `CreatedName` — the same
  "find by ID in the refreshed list" mechanism already used for
  `connectionUpdatedMsg`.
- **Create or rename collection**: on success, reload Collections and
  position the cursor on `ResultName`. If it was a rename, also reload
  Indexes (still the same underlying collection, just renamed).
- **Delete the currently-active database** (the one currently
  connected/selected): clear Collections, Documents, and Indexes back to
  the "nothing selected" state.
- **Delete the currently-active collection**: clear Documents and Indexes;
  the database selection is untouched.
- **Delete a database/collection that is NOT the active one**: only reload
  the relevant list; Documents/Indexes/Collections of the still-active
  selection are untouched.

## Help text and panel footers

- Update the `i, a` and `d, x` rows in `help.go` (currently "insertar
  documento / crear conexión o índice" and "borrar (siempre pide
  confirmación)") to mention databases and collections.
- Extend the existing `e` row to mention collection renaming, alongside
  its current document-field-edit and connection-edit meanings.
- Add panel-specific footers analogous to the existing Indexes footer
  (`"[a] crear índice  [d] borrar  [Esc] volver"`): Databases gets
  `"[a] crear database  [d] borrar"`, Collections gets `"[a] crear  [e]
  renombrar  [d] borrar"`.

## Testing

Same approach as the rest of the project: tests run against `FakeClient`,
never a real Mongo server. `Update()`/`update()` are exercised by feeding
`tea.Msg` sequences directly (no terminal mocking). Coverage includes:
create/delete/rename each emit the right message and transition
`creating`/`editing`/`confirmingDrop` correctly; the cursor-follow behavior
after create/rename; and the three delete-cascade cases above (active
database, active collection, and a non-active item) each leave the
dependent panels in the state described.

## Out of scope

- Arrow-key cursor editing in the new create/rename forms (append/backspace
  at the end only, for now — could be a follow-up feature later, as it was
  for connections).
- Renaming a database (not supported by MongoDB itself).
- "Type the exact name to confirm" for deletes — the existing simple y/n
  confirmation popup is used for both databases and collections, per
  owner's choice.
- Client-side validation of MongoDB's naming rules — the server's
  rejection surfaces via the existing `m.err` display.
