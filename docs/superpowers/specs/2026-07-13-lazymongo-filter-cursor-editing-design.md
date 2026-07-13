# lazymongo — Cursor-based editing for the Documents filter

Date: 2026-07-13
Status: Approved

## Overview

Today `docListModel.filter` is append/backspace-only at the end of the
string — there's no way to move within it. The owner wants real cursor
movement (`←`/`→`) plus smart auto-closing of `{`/`"` (standard code-editor
behavior: type `{`, get `{}` with the cursor inside; type the same closing
character when it's already there, skip over it instead of duplicating it;
Backspace on an empty auto-closed pair removes both at once). This is
scoped to the Mongo query filter field only (`/` in `[0]` Documentos) — no
other text-entry field in the app (connection form, index create form,
fuzzy search boxes) changes.

This also fixes a related gap found while designing it: once a filter is
**applied** (`Enter` pressed), `Esc` currently does nothing to it — there's
no way to clear an applied filter and see all documents again without
reopening `/` and typing an empty query by hand.

## Text model

`docListModel` gains a new field, `filterCursor int` — a **rune** index
into `filter` (not a byte index, to avoid splitting multi-byte UTF-8
characters, e.g. accented Spanish text, mid-character). All edits operate
via `[]rune(m.filter)`:
- Typing a rune inserts it at `filterCursor`, then advances the cursor by
  one.
- Backspace removes the rune immediately before `filterCursor` (if any) and
  decrements the cursor.
- `←`/`→` move `filterCursor` by one, clamped to `[0, len([]rune(m.filter))]`.

## Auto-close, skip-over, and empty-pair backspace

Typing `{` or `"` inserts the matching closer (`}` or `"`) immediately and
places the cursor **between** the two characters. All of the following
rules are purely positional — none of them parse the JSON or track which
characters were auto-inserted versus typed by hand, which is what keeps
this simple:

- **Skip-over:** typing `}` (or `"`) when the character immediately to the
  right of the cursor is already that same character moves the cursor past
  it instead of inserting a duplicate. This is what lets closing an
  auto-inserted pair — or manually finishing a value string — feel normal:
  type `"Ana`, then `"` to close the value, and since the auto-closed quote
  is already sitting there, typing `"` just steps over it.
- **Empty-pair backspace:** Backspace when the character before the cursor
  is `{`/`"` and the character after it is the matching closer deletes both
  at once, as if the pair was never opened.

## Integration with the existing filter autocomplete

The field-name suggestion feature shipped previously
(`filterFieldSuggestion`, `internal/tui/document_list.go`) was built
assuming the cursor is always at the end of `filter` — its key-position
regex matches against the end of the whole string. With a real cursor, it
must instead match against the substring **before** the cursor
(`string([]rune(m.filter)[:m.filterCursor])`), ignoring whatever comes
after (e.g. an auto-closed `}`). Accepting a suggestion with `Tab` inserts
the missing letters at the cursor position (not appended to the end).
This is a direct extension of the existing detection logic, not a
redesign — same regex, same tie-breaking, just anchored to the cursor
instead of the string's end.

## Clearing an applied filter with Esc

When a filter is currently applied (`docList.filter != ""` and NOT
actively being typed, i.e. `m.docList.filtering == false`) and `Esc` is
pressed while focus is on the Documents panel, the filter clears and all
documents reload without it — the same effect as opening `/` and
submitting an empty filter, but in one keystroke. This is additive:
`Esc`/`h` with no applied filter keep their current behavior unchanged.
`Esc` while actively typing a filter already clears it today (unchanged).

## Rendering

The `_` cursor-blink marker — today always appended after the full typed
text — moves to `filterCursor`'s actual rune position: text before the
cursor, then `_`, then the dimmed suggestion (if any), then text after the
cursor. Same visual convention already in use, just positioned correctly
instead of always at the end. Nothing about the local fuzzy-find
(`Ctrl+f`) or any other panel changes.

## Testing

Same approach as the rest of the project: `Update()` fed `tea.Msg`
sequences, no terminal mocking.
- Typed runes insert at the cursor position, not just appended; Backspace
  removes the rune before the cursor; `←`/`→` move and clamp at both edges.
- Typing `{` or `"` inserts the pair with the cursor placed between them.
- Skip-over: typing the closer when it's already immediately to the right
  moves the cursor past it without duplicating.
- Backspace on an empty auto-closed pair removes both characters.
- The autocomplete suggestion is computed from the text before the cursor,
  ignoring text after it; `Tab` inserts the suggestion at the cursor
  position.
- `Esc` with an applied filter (not currently typing) clears it and
  triggers a document reload with no filter; `Esc` with no applied filter
  is unchanged.
- Multi-byte characters (e.g. accented Spanish text) are never split by an
  insert, Backspace, or cursor move.

## Out of scope

Cursor-based editing for any other text field in the app (connection form,
index create form, fuzzy search boxes), auto-closing `[`, a forward-delete
key, and reopening a previously-applied filter for editing when `/` is
pressed again (it still starts empty, unchanged from today).
