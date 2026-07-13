# lazymongo — Edit and delete existing connections

Date: 2026-07-13
Status: Approved

## Overview

Today, `[5]` Conexiones only supports creating a new connection (`a`) —
editing or deleting an existing one has been explicitly out of scope since
the v1 spec. The owner now wants both. This only touches the Conexiones
panel and `internal/config`'s connection-writing code; nothing else in the
app changes.

## Interaction

On the highlighted connection in `[5]` Conexiones — only when not actively
fuzzy-filtering (same guard already applied to `a` when fuzzy search was
added to this panel):

- `e` opens the same form used for "new connection," but with **Name
  fixed** (shown as plain text, not an editable field) and URI/Color
  pre-filled with the connection's current values. `Tab` only alternates
  between URI and Color.
- `d`/`x` opens a confirmation popup ("¿Borrar la conexión "name"?"), the
  same pattern already used for deleting documents/indexes.

Renaming (changing the name/array-key) is not supported — editing only
changes URI and color. Deleting or editing the connection currently in use
does not affect the already-open session; it only changes the saved file.

## Where this lives

- **`internal/config/writer.go`**: two new functions, `UpdateConnection(conn Connection) error` and `DeleteConnection(name string) error`, following `AddConnection`'s exact safety model (validate the name, write, verify the result is still valid zsh via `zsh -n`, restore the original file on any failure). Two new private helpers:
  - `replaceOrInsertInArray(content, arrayName, name, newLine string) (string, error)`: replaces the existing `[name]=...` line if one exists within the named array; inserts it (same fallback as `insertIntoArray`) if the array exists but the key doesn't; creates the array block fresh if it doesn't exist at all. Used by `UpdateConnection` for both `MONGO_CONNECTIONS` and `MONGO_CONNECTION_COLORS` — the "insert if missing" fallback for colors specifically handles editing a connection that never had a color set.
  - `removeFromArray(content, arrayName, name string) (string, error)`: removes the `[name]=...` line if present; a no-op (returns content unchanged) if the array doesn't exist or the key isn't in it. Used by `DeleteConnection` for both arrays — the no-op case handles deleting a connection that never had a color entry.
  - `insertIntoArray` (used by `AddConnection`) is left completely untouched — these are new, separate functions, not a refactor of existing, already-shipped behavior.
- **`internal/tui/connection_picker.go`**: `connectionPickerModel` gains a `conns []config.Connection` field (the full connection list, URI included — today only `Name`/`Color` survive into the `listItem`s used for display) so the edit form can be pre-filled without a fresh file read. New state: `editing bool`, `confirmingDelete bool`, `confirm confirmModel` (the existing type, reused). `connectionForm` gains a `locked bool` field: when true, the Name field (index 0) ignores typed input and `Tab`/`Shift+Tab` cycle only between URI (1) and Color (2) instead of all three fields.
- **`internal/tui/root.go`**: four new message types mirroring the existing creation ones — `connectionUpdatedMsg{Conn config.Connection}`, `connectionUpdateErrMsg{Err error}`, `connectionDeletedMsg{Name string}`, `connectionDeleteErrMsg{Err error}` — each success case reloads the connections list (same refresh already done for `connectionCreatedMsg`), each error case sets `m.err`. `inTextEntry()` gains a case for `connPicker.editing` (mirroring the existing `connPicker.creating` case) so global shortcuts don't steal keystrokes while editing. The popup-overlay condition in `View()` extends to cover `editing`/`confirmingDelete`, not just `creating`.

## Testing

Same approach as the rest of the project: `Update()` fed with `tea.Msg`
sequences for the TUI, and file-fixture-based tests (mirroring
`writer_test.go`'s existing pattern) for `internal/config`.
- `UpdateConnection` changes URI/color while leaving the name and every
  other connection untouched; works even when the connection never had a
  color entry (falls back to insert instead of erroring).
- `DeleteConnection` removes the connection from both arrays; is a no-op
  (not an error) when the connection never had a color entry; every other
  connection survives.
- `e` pre-fills the form with the connection's current URI/color and the
  Name field rejects typed input; `Tab` in edit mode alternates only
  between URI and Color.
- `d`/`x` open the confirmation; confirming triggers the delete flow.
- Typing "e", "d", or "x" while fuzzy-filtering the connection list adds
  those letters to the search query instead of triggering edit/delete.

## Out of scope

Renaming an existing connection (changing its name/array-key), and any
special handling for editing/deleting the connection the current session
is actively using.
