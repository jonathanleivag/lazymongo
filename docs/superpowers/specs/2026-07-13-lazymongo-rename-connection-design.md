# lazymongo — Editable connection name (rename support)

Date: 2026-07-13
Status: Approved

## Overview

The earlier edit/delete-connection design
(`docs/superpowers/specs/2026-07-13-lazymongo-edit-delete-connection-design.md`)
explicitly locked the Name field when editing an existing connection
("URI y color, el nombre queda fijo"), because renaming means moving the
array key across two zsh associative arrays (`MONGO_CONNECTIONS` and
`MONGO_CONNECTION_COLORS`) rather than just replacing a value under an
unchanging key. The owner now wants Name editable too. This spec reverses
that earlier decision and adds real rename support.

## Config layer: `RenameConnection`

New function in `internal/config/writer.go`, alongside (not replacing)
`AddConnection`/`UpdateConnection`/`DeleteConnection`:

```go
func RenameConnection(oldName string, conn Connection) error
```

Behavior:
1. Validates both `oldName` and `conn.Name` with the existing
   `isValidConnectionName` (same regex, same security rationale already
   documented on `AddConnection` — the name is interpolated raw into
   `[%s]=` array-subscript syntax).
2. If `conn.Name != oldName`, checks for a collision: does
   `MONGO_CONNECTIONS` already contain an entry for `conn.Name`? Uses a new
   helper, `arrayHasKey(content, arrayName, name string) bool`, which scans
   the named array's block for a `[name]=` line the same way
   `replaceOrInsertInArray`/`removeFromArray` already do (returns `false` if
   the array doesn't exist at all). If found, returns an error —
   `fmt.Errorf("ya existe una conexión llamada %q", conn.Name)` — **before**
   reading further or writing anything.
3. Removes `oldName`'s entries from both `MONGO_CONNECTIONS` and
   `MONGO_CONNECTION_COLORS` via the existing `removeFromArray`.
4. Inserts `conn.Name`'s entries into both arrays via the existing
   `insertIntoArray`, wrapping URI/Color with the existing `zshSingleQuote`
   (never `%q` — see `zshSingleQuote`'s doc comment for why).
5. Writes the result, validates with `validateZshSyntax`, and reverts to
   the original bytes on failure — the same safety pattern already shared
   by `AddConnection`/`UpdateConnection`/`DeleteConnection`.

If `conn.Name == oldName` is passed in, step 2's collision check is
trivially skipped (a name can't collide with itself) and the function
degenerates into a remove+reinsert under the same key — functionally
equivalent to `UpdateConnection` but not the path the UI takes for a
no-name-change edit (see below). `UpdateConnection` is not modified.

## UI layer: removing `locked` from `connectionForm`

`connectionForm` (`internal/tui/connection_picker.go`) currently has a
`locked bool` field that makes Name read-only and skips it during
`Tab`/`Shift+Tab` cycling when editing. With Name now editable in both
create and edit flows, create and edit become functionally identical forms
(same field navigation, same editability) except for pre-filled values and
which config function gets called on submit. `locked` is removed entirely:

- `connectionForm` loses the `locked` field.
- `newEditConnectionForm(conn config.Connection) connectionForm` pre-fills
  Name/URI/Color from `conn`, starts on field 0 (Name — matching
  `newConnectionForm`'s default start point), with `cursor` at the end of
  the pre-filled Name.
- `Tab`/`Shift+Tab` always cycle `0 → 1 → 2` (and reverse), unconditionally
  — the locked-mode two-state cycle is deleted.
- The `if f.field == 0 && f.locked { return f }` guards in the
  `tea.KeyBackspace` and `tea.KeyRunes` cases are deleted. Name is freely
  editable via the same insert-at-cursor/backspace-before-cursor mechanics
  already used for URI.
- `View()` in `connection_picker.go` is unaffected — it already renders the
  cursor-blink marker for field 0 the same way it does for field 1; it
  never checked `locked`.

## `connectionPickerModel`: detecting a rename at submit time

`connectionPickerModel` gains one field: `editingOriginalName string`.

- When `e` is pressed, alongside setting `m.editing = true` and
  `m.form = newEditConnectionForm(conn)`, also set
  `m.editingOriginalName = conn.Name` — the name as it was *before* any
  edits, captured once at the start of the edit session.
- On `Enter` while `m.editing`: build `conn` from the form as today, then
  branch:
  - If `conn.Name != m.editingOriginalName`: call
    `config.RenameConnection(m.editingOriginalName, conn)`.
  - Otherwise: call `config.UpdateConnection(conn)` (unchanged).
  - Both branches report through the existing `connectionUpdatedMsg{Conn: conn}`
    / `connectionUpdateErrMsg{Err: err}` — no new message types.
- Creating (`m.creating`) is unaffected — always `config.AddConnection`.

An empty or invalid new Name (e.g. cleared to `""`, or containing a
character outside `isValidConnectionName`'s allowed set) is caught by the
same validation already inside `RenameConnection`/`UpdateConnection` and
surfaces as `connectionUpdateErrMsg`, exactly like today's invalid-name
handling on create — no new validation is added at the UI layer.

## Cursor follows the connection after a rename

`root.go`'s `connectionUpdatedMsg` handler currently rebuilds
`m.connPicker` via `newConnectionPickerModel(conns)` after re-listing
connections, which always resets the list cursor to 0 (top of the
alphabetically-sorted list). Since a rename can move the connection to a
different alphabetical position, this handler is extended: after rebuilding
`m.connPicker`, find the item whose `ID == msg.Conn.Name` in
`m.connPicker.list.Items` and set `m.connPicker.list.Cursor` to its index —
the same lookup-by-ID approach `listModel.exitFiltering` already uses
elsewhere in the codebase. Because `msg.Conn.Name` is already the *new*
name for both the rename and the plain-update path, this one change
correctly repositions the cursor for both cases with no extra branching.

This is scoped to `connectionUpdatedMsg` only. `connectionCreatedMsg` and
`connectionDeletedMsg` keep resetting the cursor to 0 — that behavior isn't
part of this request and isn't touched.

## Testing

- `internal/config/writer_test.go`: renaming an existing connection moves
  its entries in both arrays under the new key and removes the old key
  entries; renaming to a name that already belongs to a *different*
  existing connection is rejected with an error and leaves the file byte-
  for-byte unchanged; renaming with an invalid new name is rejected before
  any write; the URI/Color values survive the rename unchanged (still
  wrapped via `zshSingleQuote`, not `%q`).
- `internal/tui/connection_picker_test.go`:
  - Delete the tests that assert the removed `locked` behavior (as of this
    writing: the tests covering "Name field is read-only when editing" and
    "Tab in edit mode only cycles URI/Color" and "edit mode initial cursor
    lands on URI") — do not adapt them, since they test behavior being
    deliberately removed.
  - New/updated tests: `newEditConnectionForm` starts on field 0 (Name)
    with the cursor at the end of the pre-filled name; typing/backspacing
    on Name works identically in create and edit forms; `Tab` from Color
    wraps back to Name in both create and edit forms.
  - `connectionPickerModel`: editing a connection and changing only the
    Name (leaving URI/Color the same) results in a call path that would
    invoke `RenameConnection`, not `UpdateConnection` — verified the same
    way existing tests already avoid touching the real connections file
    (assert `cmd != nil` and inspect `m.editingOriginalName` vs the
    submitted `conn.Name`, never invoke the `tea.Cmd` closure for real).
- `internal/tui/root_test.go` (or wherever `connectionUpdatedMsg` handling
  is already tested, if anywhere): after a `connectionUpdatedMsg` with a
  `Conn.Name` that differs from any pre-existing cursor position, the
  resulting `m.connPicker.list.Cursor` points at the list item whose
  `ID == Conn.Name`.

## Out of scope

- Any change to `connectionCreatedMsg`/`connectionDeletedMsg` cursor
  behavior (both keep resetting to 0).
- Bulk rename, undo, or rename history.
- Any change to `AddConnection`, `UpdateConnection`, `DeleteConnection`,
  `insertIntoArray`, `replaceOrInsertInArray`, or `removeFromArray` — all
  stay exactly as they are; only a new `RenameConnection` and a new
  `arrayHasKey` helper are added.
- Any change to the Color field's cycling behavior or to cursor-based
  editing mechanics themselves (insert-at-cursor, backspace-before-cursor) —
  already shipped, untouched by this spec.
