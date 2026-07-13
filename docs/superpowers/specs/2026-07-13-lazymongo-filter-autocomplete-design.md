# lazymongo — Field-name autocomplete for the Documents filter

Date: 2026-07-13
Status: Approved

## Overview

Today, the Mongo query filter in `[0]` Documentos (`/`) is a raw text field
— you type JSON like `{"name": "Ana"}` with no help. The owner wants field
names to autocomplete inline (shell-style ghost text) as they type, sourced
from the documents already loaded on the current page — the same
zero-extra-query spirit as the local fuzzy-find (`Ctrl+f`).

This only autocompletes field **names**, never values, and only top-level
fields — not nested paths. It's independent of the local fuzzy-find, the
expanded-document view, and the (separately paused) page-boundary
navigation feature.

## Trigger and matching

The filter field is append/backspace-only at the end of the string — there
is no left/right cursor movement within it — so "where you're typing" is
always the end of `m.filter`. A suggestion is offered when the text typed so
far ends in an **open quote immediately following `{` or `,`** (a JSON key
position), e.g. typing `{"nam` matches; the captured text after that quote
(`nam`) is the partial field name to match against known field names.

Known field names are the union of the top-level keys across every document
currently in `m.docs` (the loaded page) — not recursing into nested
`bson.M`/`bson.A` values, matching the same top-level-only boundary already
chosen for the expanded-document view's `Array (N)`/`Object` placeholders.

If more than one known field name starts with the partial text (e.g. `na`
matches both `name` and `nationality`), the alphabetically-first match is
suggested — simple and deterministic; typing further narrows it down.

## Display and acceptance

The suggested remainder renders in a faint/dim style immediately after the
typed text, before the cursor blink (`_`) — the same visual convention
already used for "Filtro: " and "Buscar: " indicators, just dimmer for the
part that hasn't been typed yet. `Tab` accepts it: appends the missing
letters of the field name to `m.filter` (not the closing quote — the user
still types that themselves). Any other key ignores the suggestion and
behaves exactly as it does today.

## Where this lives

- **`internal/tui/document_list.go`**: a new pure function/method inside
  `docListModel` computes the suggestion text from `m.filter` and `m.docs`
  (empty string when no key-position match applies). A new `tea.KeyTab`
  case is added to the existing `if m.filtering { switch keyMsg.Type {...} }`
  block, appending the current suggestion (if any) to `m.filter`. A new
  accessor, `FilterSuggestion() string`, follows the same naming convention
  already established by `FilterQuery()` (listModel/idxListModel) and
  `FuzzyQuery()` (docListModel itself).
- **`internal/tui/root.go`**: the existing line construction
  (`"Filtro: " + m.docList.filter + "_"`) changes to insert the
  dim-styled suggestion between the typed text and the cursor blink, via the
  new accessor. No other change to that line's construction, and no change
  to the pre-existing, out-of-scope cursor-misalignment issue with that
  prepended line.
- **Untouched:** the local fuzzy-find (`Ctrl+f`), the expanded-document
  rendering, `n`/`p` pagination, and the separately-paused page-boundary
  navigation spec.

## Testing

Same approach as the rest of the project: pure-function tests plus
`docListModel.Update()` fed `tea.Msg` sequences, no terminal mocking.
- The suggestion function returns the correct remainder for a partial key
  typed after `{` or `,`, matching the union of top-level field names across
  multiple documents.
- A tie between multiple matching field names resolves to the
  alphabetically-first one.
- `Tab` with an active suggestion appends it to `m.filter`; `Tab` with no
  active suggestion is a no-op that doesn't corrupt `m.filter`.
- Nested fields (keys inside a `bson.M`/`bson.A` value) are never suggested.
- Typing text that matches no known field name yields an empty suggestion.
- Typing outside a key position (e.g. inside a value's quotes, or after a
  `:`) yields no suggestion.

## Out of scope

Value autocompletion, nested/dot-notation field paths, a navigable
multi-suggestion list (arrow-key selection), and fetching an enlarged
sample from the collection independent of pagination — all deferred. The
page-boundary navigation feature raised in the same conversation is
unrelated and is being handled as its own, separately-paused spec.
