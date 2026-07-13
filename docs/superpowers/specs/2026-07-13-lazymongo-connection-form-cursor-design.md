# lazymongo — Cursor-based editing for the connection form

Date: 2026-07-13
Status: Approved

## Overview

`connectionForm` (shared by both "new connection" (`a`) and "edit connection"
(`e`) in `[5]` Conexiones) currently edits Name and URI append/backspace-only
at the end of the string, the same limitation the Mongo filter field had
before cursor-based editing was added there. The owner wants `←`/`→` to move
within Name/URI, matching that same pattern. The Color field is unchanged —
it already uses `h`/`l`/arrows to cycle through `amarillo`/`rojo`/`verde`,
not to move within text, and has no cursor concept.

This is scoped to arrow-key movement only — no auto-close of `{`/`"` like
the Mongo filter got, since a connection name or a MongoDB URI is not JSON
and has no bracket/quote pairs to auto-close.

## Cursor model

`connectionForm` gains a `cursor int` (rune index) representing the position
within whichever of Name (field 0) or URI (field 1) is currently active.
- `←`/`→` move the cursor by one rune, clamped to `[0, len([]rune(active field text))]`.
- Typing inserts the rune at `cursor`, then advances it by one.
- `Backspace` removes the rune immediately before `cursor`.
- Switching fields with `Tab`/`Shift+Tab` moves the cursor to the **end** of
  whichever field (Name or URI) becomes active — matching the convention of
  tabbing into a normal form input and landing ready to keep typing, rather
  than always resetting to the start.
- The Color field (2) is completely unaffected — its existing `h`/`l`/arrow
  handling for cycling colors is untouched, and it has no cursor.

## Rendering

While Name or URI is the active field, its line shows
`text-before-cursor` + `_` + `text-after-cursor` instead of the flat field
value — the same convention already used for the Mongo filter's cursor. The
trailing ` <` marker that already indicates "this is the active field" stays
for all three fields (Color still needs it, since it has no cursor of its
own).

## Where this lives

Entirely within `internal/tui/connection_picker.go` — `connectionForm` gains
`cursor`, and its `update`/rendering logic follow the same shape already
established for `docListModel.filter`/`filterCursor` in
`internal/tui/document_list.go` (insert-at-cursor, backspace-before-cursor,
arrow-clamping, before/after-cursor split for rendering). No other file
changes; both the create and edit flows share this form, so both benefit
without any change to `connectionPickerModel` itself.

## Testing

Same approach as the rest of the project: `Update()`/`update()` fed `tea.Msg`
sequences, no terminal mocking.
- Typing inserts at the cursor position, not just appended; `Backspace`
  removes the rune before the cursor; `←`/`→` move and clamp at both edges,
  for both Name and URI.
- Switching fields with `Tab`/`Shift+Tab` puts the cursor at the end of the
  newly active field's current text.
- The Color field's existing `h`/`l`/arrow cycling behavior is unaffected.
- Multi-byte characters (e.g. accented Spanish text) are never split by an
  insert, `Backspace`, or cursor move.
- The edit form (`locked = true`) still only ever lands on field 1 (URI) or
  2 (Color) when tabbing — Name (field 0) stays unreachable and read-only,
  unaffected by this change.

## Out of scope

Auto-closing `{`/`"` (not applicable — Name/URI aren't JSON), any change to
the Color field's cycling, and cursor-based editing for any other text field
in the app beyond what's already shipped (the Mongo filter) and this form.
