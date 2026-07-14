# lazymongo — Page-boundary navigation in the Documents panel

Date: 2026-07-13
Status: Approved

## Overview

Today, in `[0]` Documentos, `j`/`k` move the cursor within the current
page's loaded rows and clamp at the edges — pressing `j` on the last
document, or `k` on the first, does nothing. Advancing pages requires the
separate `n`/`p` keys. The owner wants `j`/`k` to fall through into a page
change at those edges, so scrolling a long collection feels continuous
instead of hitting a wall at each page boundary.

## Behavior

- `j` on the **last** document of the current page: if a next page exists
  (`(page+1)*pageSize < total` — the same check `n` already uses), advances
  to the next page and lands on its **first** document (cursor 0, the
  existing default when a page reloads).
- `k` on the **first** document of a page that isn't the first: if
  `page > 0` (the same check `p` already uses), goes back to the previous
  page and lands on its **last** document.
- At the absolute edges (last page with no next, or page 0 with no
  previous), nothing happens — identical to today's `n`/`p` behavior at
  those edges.
- This does not change `n`/`p` themselves, the Mongo query filter (`/`),
  the local fuzzy-find (`Ctrl+f`), or the expanded-document rendering
  shipped in the previous feature — only what `j`/`k` do at the two
  boundary positions.

## Where this lives

- **`internal/tui/document_list.go`**: the boundary check lives in
  `docListModel.Update`'s existing `"j"`/`"k"` cases, since `docListModel`
  already holds `total`/`pageSize`/`page` (the same fields `"n"`/`"p"`
  already read). At the boundary, instead of a no-op, it emits the existing
  `pageChangedMsg{Page: ...}` — extended with one new field,
  `LandOnLast bool`, set `true` only for the backward (`k`) case.
- **`internal/tui/root.go`**: the existing `case pageChangedMsg:` handler
  reads the new `LandOnLast` field. Since the actual page reload is
  asynchronous (`m.loadDocuments(...)` returns a `tea.Cmd`, and the loaded
  rows only arrive later via `documentsLoadedMsg`), `LandOnLast` can't be
  acted on immediately — it's stashed on `RootModel` in a new field,
  `landCursorAtEnd bool`, following the same pattern the codebase already
  uses for `editMode` (remembering context across an async round-trip). The
  existing `case documentsLoadedMsg:` handler checks and consumes that flag
  once, right after building the new `docListModel`: if set, it moves the
  freshly-loaded page's cursor to `len(docs)-1` (guarded against an empty
  page) instead of leaving it at the default 0, then clears the flag.

## Testing

Same approach as the rest of the project: `Update()` fed with `tea.Msg`
sequences, `mongo.FakeClient` for data.
- `j` on the last document of a page with a next page emits
  `pageChangedMsg{Page: page+1, LandOnLast: false}`.
- `j` on the last document of the last page (no next page) is a no-op, as
  today.
- `k` on the first document of page > 0 emits
  `pageChangedMsg{Page: page-1, LandOnLast: true}`.
- `k` on the first document of page 0 is a no-op, as today.
- At the `RootModel` level: simulating the full round-trip (`k` at a
  page-0-boundary → `pageChangedMsg` → `documentsLoadedMsg`) lands the
  cursor at `len(docs)-1` on the reloaded previous page; the equivalent
  forward round-trip (`j` → `pageChangedMsg` → `documentsLoadedMsg`) lands
  the cursor at 0, matching existing default behavior.
- `n`/`p` continue to work exactly as before (regression check that the new
  `LandOnLast` field defaults to `false` for their existing call sites and
  doesn't change their behavior).

## Out of scope

Any change to `n`/`p`, the Mongo query filter, the local fuzzy-find, or the
expanded-document view. A separate, larger feature (autocomplete for the
Mongo filter's `{}` query, suggesting field names/values from the loaded
documents) was raised in the same conversation but is being brainstormed
and spec'd separately — it is unrelated to this page-boundary navigation
change.
