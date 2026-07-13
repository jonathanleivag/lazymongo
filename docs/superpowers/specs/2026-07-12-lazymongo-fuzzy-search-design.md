# lazymongo — Fuzzy-find search design

Date: 2026-07-12
Status: Approved

## Overview

Today, `/` only exists in the `[0]` Documents panel, where it opens a MongoDB
extended-JSON query filter sent to the database. The other list panels —
`[2]` Databases, `[3]` Collections, `[4]` Indexes, `[5]` Conexiones — have no
search at all: navigation is `j`/`k` only.

This adds fuzzy-find (fzf-style, live-as-you-type filtering) to those 4 list
panels, using `github.com/sahilm/fuzzy` (the same library `bubbles/list`,
from the same Charm ecosystem this project already depends on, uses for its
own fuzzy filtering). It also adds an independent local fuzzy-find over
already-loaded rows in `[0]` Documents, bound to `Ctrl+F`, which does not
touch or replace the existing Mongo query filter (`/`) — the two are
parallel mechanisms.

## Keybindings and interaction

- `[2]`/`[3]`/`[4]`/`[5]`: `/` enters search mode. Each keystroke filters the
  list live (fuzzy match, not exact substring). The cursor jumps to the
  first result, and the existing live-cascade behavior (Databases →
  Collections, Collections → Indexes/Documents) keeps firing normally, since
  it reacts to any cursor change regardless of whether it came from `j`/`k`
  or from typing into the filter. `Esc` clears the filter and restores the
  full list (cursor resets to 0). `Enter` is unchanged: still a no-op on
  Databases/Collections (existing behavior, cascade already happened live)
  and still confirms a selection in Conexiones.
- `[0]` Documents: `Ctrl+F` opens the local fuzzy-find (independent of the
  existing Mongo filter `/`). It filters the rows already fetched to the
  screen live — it never issues a new query against MongoDB. `Esc` closes it
  and restores the current page's full row set.
- Help footer (`?`): gains a line documenting `/` in the list panels and
  `Ctrl+F` in Documents.

**Coordination detail:** today, `a` in Conexiones ("new connection") and
`a`/`d` in Indexes ("create index"/"drop index") are intercepted before
reaching the generic list-handling code. If the user is typing a search and
their query happens to contain an "a" or a "d", that must NOT trigger those
actions — these shortcuts must be gated behind `!filtering`.

## Where this lives

- **`internal/tui/list.go`** (`listModel`, used by Databases/Collections/
  Conexiones): new fields `filtering bool`, `filterQuery string`. `Items`
  remains the source of truth that all existing code reads — while
  filtering, `Items` becomes the fuzzy-matched result (ordered by
  `sahilm/fuzzy` score) and `Cursor` keeps indexing into it, so existing
  code in `root.go` that reads `m.dbList.Items[m.dbList.Cursor]` keeps
  working unmodified. The original full set is kept separately so `Esc` can
  restore it.
- **`internal/tui/index_list.go`** (`idxListModel`, does not share code with
  `listModel`): same field pattern (`filtering`, `filterQuery`) applied to
  `m.indexes`, gating `a`/`d` behind `!m.filtering`.
- **`internal/tui/connection_picker.go`**: gate its own `a` interception
  behind `!m.list.filtering` before delegating to `m.list.Update`.
- **`internal/tui/document_list.go`**: new independent state
  (`fuzzyFiltering bool`, `fuzzyQuery string`) over the rows already in
  memory — does not interact with the existing Mongo query
  `filter`/`filtering` fields; these are two parallel mechanisms.
- **`internal/tui/root.go`**: `inTextEntry()` gains the new cases (`filtering`
  in any of the 4 list panels, `fuzzyFiltering` in Documents) so global keys
  like `?` don't interrupt an in-progress search. Footer/help text updated.

  **Second deviation (found while writing the implementation plan, approved
  by the owner):** today the global `1`-`5` panel-jump and `Tab` shortcuts
  are checked BEFORE dispatch to the focused panel's own `Update`, gated
  only by `m.popup == popupNone` — not by `inTextEntry()`. This is already a
  latent bug (typing a Mongo filter containing a digit 1-5 ejects you to a
  different panel instead of adding it to the filter text), but this feature
  makes it far more likely to bite: real names like `haddacloud-v2` or
  `email_1` (both used as examples in this project's own mockups) contain
  digits, so searching for them with the new fuzzy-find would eject the user
  mid-search. Fix: gate `1`-`5` and `Tab` behind `!m.inTextEntry()` too, the
  same protection `?` already has.
- **`go.mod`**: add `github.com/sahilm/fuzzy` as a direct dependency.

## Rendering

**Deviation from the original design (found while writing the implementation
plan, approved by the owner):** the existing Mongo filter indicator in
Documents has a pre-existing, out-of-scope bug — `root.go` prepends a
`"Filtro: ..."` line to the rendered rows without adjusting the cursor
index, so the `>` selection marker lands one row off whenever that filter is
active. Copying that convention into the 4 new list panels would reproduce
the same bug 4 more times. Instead: while filtering, the search text is shown
in the panel's **title** (e.g. `"Databases — Buscar: hdc_"`), not as a
prepended content line, so `Items`/`Cursor` indexing is never touched by the
indicator. An empty result set shows a single `(sin coincidencias)` line in
place of the item list (safe — there's no real selection to misalign when
the list is genuinely empty). This applies to the 4 list panels
(Databases/Collections/Indexes/Conexiones) and to the new local fuzzy-find
in Documents (its title-appended indicator is independent from, and does not
modify, the existing buggy `"Filtro: ..."` line). Moving focus away from a
filtering panel (e.g. pressing `1`-`5`) does NOT clear that panel's active
filter — consistent with the existing behavior of the Mongo filter in
Documents, not a new decision.

## Testing

Same approach as the rest of the project: `Update()` fed with `tea.Msg`
sequences, no terminal mocking. New coverage:
- `listModel`: typing text narrows `Items` to the expected fuzzy matches;
  `Esc` restores the full set; cursor resets to 0 when a filter starts;
  `a`/`d`/`enter` still work normally when NOT filtering.
- `connectionPickerModel` / `idxListModel`: typing "a" or "d" while filtering
  does NOT trigger create-connection / create-or-drop-index.
- `docListModel`: `Ctrl+F` opens local fuzzy-find over already-loaded rows,
  without calling `Find`/`CountDocuments` again (verifiable via
  `FakeClient` call counts); coexists with the existing Mongo filter without
  interfering with it.
- Filtering down to zero matches does not panic or index out of range.

## Out of scope

Visual highlighting of matched characters, recent-search history, and
fuzzy-matching over document *content* (nested fields) rather than the
already-rendered rows — all deferred to a future iteration if needed.
