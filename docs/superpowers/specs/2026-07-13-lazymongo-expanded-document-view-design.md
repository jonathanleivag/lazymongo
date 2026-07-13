# lazymongo — Expanded document view in the Documents panel

Date: 2026-07-13
Status: Approved

## Overview

Today, `[0]` Documentos shows only each document's `_id` as a single line;
seeing the rest of the fields requires pressing `Enter` to open the existing
detail popup (`docDetailModel` — per-field cursor, `e` edits a field, `E`
edits the full document in `$EDITOR`, `d`/`x` deletes). The owner wants a
Compass-style view where the currently highlighted document shows all of its
fields directly in the panel, without needing the popup.

This is a purely visual change to how `[0]`'s rows are rendered. It does not
touch `docDetailModel`, the popup, editing, deletion, or search (`/` /
`Ctrl+f`) in any way — `Enter` still opens the same popup it always has.

## Behavior

Only the document at `docList.cursor` expands to show every field, one per
line, colored by BSON type. Every other document in the current page stays
collapsed to its existing single `_id` line. Moving the cursor (`j`/`k`,
arrows, or a fuzzy-search match) changes which document is expanded — never
more than one at a time. A blank line follows the expanded block before the
next collapsed row, so it doesn't read as fused to what follows.

## Integration with the shared panel primitive

`renderPanel` (the primitive drawing all 6 panels — `internal/tui/panel.go`)
assumes one line of input maps to one displayed item, with a single `>`
cursor marker on that line. Two ways to reconcile a variable-height expanded
block with that:

- **A (chosen):** Flatten to lines + compute an "effective cursor" line
  index. The highlighted document expands into N lines (one per field); every
  other document still contributes exactly 1 line, as today. A new function
  computes the flat `[]string` for the whole page plus the physical line
  index where the expanded block starts, and that index — not
  `docList.cursor` directly — is what gets passed to `renderPanel`.
  `renderPanel` itself is not modified. The `>` marker lands only on the
  block's first line (the `_id` line) — the rest of the block's lines render
  as plain, unmarked, indented lines. This is consistent with how the other
  5 panels render (no risk to them) and needs no changes to shared code.
- **B (rejected):** Give Documents its own bespoke drawing path outside
  `renderPanel`, e.g. a bordered card per document matching the Compass
  screenshot exactly. Rejected because it duplicates border/title/focus-color
  logic that already lives in `panel.go`, and would make Documents visually
  inconsistent with the other 5 panels for no functional gain over A.

## Type-based coloring

A new style helper, separate from the existing `colorStyle` (which is for
per-connection colors — amarillo/rojo/verde — a different concept). Maps each
field's real decoded Go/BSON type to a color:

| Type | Color |
|---|---|
| `bson.ObjectID` | orange/yellow |
| `string` | green |
| `int32` / `int64` / `int` / `float64` / `float32` | blue |
| `bool` | magenta |
| `time.Time` | blue (same as numbers) |
| `nil` | gray/faint |
| `bson.A` (array) | gray/faint, rendered as `Array (N)` |
| `bson.M` (nested document) | gray/faint, rendered as `Object` |

Arrays and nested documents are shown as a collapsed placeholder
(`Array (N)` / `Object`) rather than recursively expanded — matching the
Compass screenshot (which itself shows `additionals: Array (1)` collapsed,
not expanded) and avoiding new interactive state for drilling into
sub-fields. Nested inspection is already covered by the existing detail
popup if needed.

Field order within the expanded block matches `docDetailModel`'s existing
convention: fields sorted alphabetically (`sort.Strings`), so the expanded
inline view and the popup show fields in the same order.

## Where this lives

- **New:** a pure function building the flat lines + effective-cursor pair
  for the Documents panel, and a value-to-styled-string helper implementing
  the type table above. Both are new, focused pieces of code — not additions
  to `docDetailModel` (which stays untouched) — since this is rendering logic
  for the panel-grid list, not the popup.
- **`internal/tui/root.go`:** the call site building `docLines` for panel
  `[0]` (currently `labelsFromDocs(m.docList.docs)`) switches to the new
  function, and the `cursor` argument passed to `renderPanel(0, ...)` becomes
  the computed effective line index instead of `m.docList.cursor` directly.
- **Untouched:** `internal/tui/document_list.go` (`docList.cursor` keeps
  meaning "index into `docs`", unchanged), `internal/tui/document_detail.go`
  (popup/editing/deletion), `internal/tui/root.go`'s search wiring (`/`,
  `Ctrl+f`), `internal/tui/panel.go` (`renderPanel`/`visibleWindow` — no
  changes).

## Testing

Same approach as the rest of the project: pure-function tests, no terminal
mocking.
- The highlighted document produces N lines (one per field, alphabetically
  sorted); every other document produces exactly 1 line.
- The effective cursor index passed to `renderPanel` lands on the expanded
  block's first line.
- Each BSON type in the table above renders with its assigned color, and
  arrays/nested documents render as `Array (N)`/`Object` rather than their
  raw value.
- Moving the cursor to a different document changes which block expands,
  without affecting `Enter`, the popup, editing, or deletion.
- A blank line separates the expanded block from the next collapsed row.

## Out of scope

Interactive expand/collapse of nested sub-fields, per-document bordered
cards (Approach B), and any change to the search UI or the existing detail
popup — all unchanged.
