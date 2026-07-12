# lazymongo — lazygit-style UI Design (v2)

Date: 2026-07-11
Status: Approved
Supersedes: the UI/navigation portions of `2026-07-11-lazymongo-design.md` (v1).
Everything in v1 NOT related to layout/navigation (scope, connections file format,
safety rules, tech stack, testing philosophy) still applies and is not repeated here.

## Overview

v1 shipped a `k9s`-style drill-down TUI (one view at a time: connections →
databases → collections → documents/indexes, `Esc` to go back). After using it,
the owner wants the actual `lazygit` look instead: several panels visible
**simultaneously** — a stack of small numbered panels on the left, one large
content panel on the right, a command log, and a keybinding footer — with
everything else (document detail, edit forms, confirmations) as floating
popups over that panel grid, exactly like lazygit's own confirmation popups.

This replaces `RootModel`'s navigation model (Task 16 of the v1 plan) almost
entirely. It does **not** change `internal/config` or `internal/mongo`, and it
reuses the *logic* (not necessarily the rendering) of most existing `internal/tui`
components.

## Layout

Five fixed panels stacked on the left (numbered `[1]`–`[5]`), one large panel on
the right (`[0]`), a command log below it, and a one-line keybinding footer at
the very bottom. All panels are drawn at all times — nothing is full-screen.

```
┌[1]─Status──────────────┐ ┌[0]─Documentos──────────────────────────┐
│ movatec-prod (rojo)    │ │ _id: 65f...  total: 42000  pagado      │
│ haddacloud-v2.orders   │ │ _id: 65e...  total: 15000  pendiente   │
├[2]─Databases───────────┤ │ ...                                     │
│ admin                  │ │                                         │
│ > haddacloud-v2        │ │                                         │
├[3]─Collections─────────┤ │                                         │
│ > orders               │ │                                         │
│   users                │ ├─Command log─────────────────────────────┤
├[4]─Indexes─────────────┤ │ Conectado a movatec-prod                │
│ > _id_                 │ │ Filtro aplicado: {estado: "pagado"}     │
│   email_1              │ │                                         │
├[5]─Conexiones──────────┤ └──────────────────────────────────────────┘
│   qa                   │
│ > prod                 │
└─────────────────────────┘
[1-5] panel  [j/k] mover  [Tab] documentos  [Enter] ver  [e] editar  [d] borrar  [/] filtro  [?] ayuda
```

### Panel-to-domain mapping

| Panel | Shows | Analogous lazygit panel |
|---|---|---|
| `[1]` Status | Current connection name (color-coded) + current `db.collection` breadcrumb | Status (current branch) |
| `[2]` Databases | Databases in the current connection | Files |
| `[3]` Collections | Collections in the selected database | Local branches |
| `[4]` Indexes | Indexes on the selected collection | Commits |
| `[5]` Conexiones | Named connections from `mongo-connections.sh`, color-coded | Stash |
| `[0]` Documentos | Paginated/filtered document list of the selected collection | Diff/main panel |
| Command log | Rolling history: connects, filters applied, confirmed writes, errors — every action, not just failures | Command log |

## Startup

The full 5-panel layout renders immediately on launch (empty where there's no
data yet). Focus starts on `[2] Databases` when launched with a connection
name argument (e.g. `lazymongo qa`), since the connection is already known
and there's no reason to force a `Conexiones` detour; focus starts on
`[5] Conexiones` only when launched with no argument, so the user can pick a
saved connection. Selecting a connection in `[5]` connects and cascades:
`[2]` populates with databases; selecting one populates `[3]`; selecting a
collection populates `[4]` and `[0]`. No separate "connection picker"
screen — it's just panel `[5]`, always reachable.

## Focus and navigation

- `1`–`5`: jump directly to that panel (matches lazygit).
- `j`/`k` (or arrows): move the cursor within the currently focused panel.
- `Tab` (or arrow toward the main panel): move focus to `[0]` Documentos.
- Moving the cursor in `[2]`/`[3]` (with `j`/`k`) immediately cascades: highlighting a database live-updates `[3]`'s collection list, highlighting a collection live-updates `[4]`'s indexes and `[0]`'s document list — no `Enter` needed to "confirm" a selection, since browsing (not checking out) is the whole point here. `Enter` is reserved for entering the focused panel's item (e.g. `Enter` on a document in `[0]` opens its detail popup).
- Selecting a connection in `[5]` (re)connects — this can happen at any time, not just at startup, so switching environments mid-session is just "press `5`, pick a different connection."

## Popups

Everything that isn't one of the 6 persistent panels is a floating, centered
popup drawn on top of the panel grid — directly modeled on lazygit's own
confirmation/input popups:

- Document detail (full JSON of a selected document from `[0]`)
- Inline field edit + its `y`/`n` confirmation
- Full-document edit via `$EDITOR` (suspends the screen the same way as v1)
- Insert-document flow (same `$EDITOR` mechanism, empty template)
- Delete-document confirmation
- Create-index form + its confirmation
- Drop-index confirmation
- Create-connection form (name/URI/color)
- Any error message

`Esc` closes the active popup and returns focus to whichever panel was active
before it opened — the panels behind it never lose their state.

The universal write-confirmation rule from v1 is unchanged: every mutating
action (insert, inline edit, full-document replace, delete, create index, drop
index) still shows a confirmation popup before executing, on every connection,
with no exceptions. Only the *presentation* of that confirmation changes (popup
instead of full-screen), not the rule itself.

## What's reused vs. rewritten

- **Unchanged, zero modification:** `internal/config` (connection resolution
  and writing), `internal/mongo` (the `Client` interface, `RealClient`,
  `FakeClient`). These packages have no knowledge of how the TUI renders.
- **Logic reused, presentation rewritten:** `confirmModel`'s confirm/cancel
  logic, the document filter/pagination state machine, the create-connection
  and create-index form logic, the `$EDITOR` suspend/resume mechanism. These
  keep their message types and update logic; they get re-wrapped to render as
  popups instead of full-screen views.
- **Rewritten:** `RootModel` (previously a single-active-view + navigation
  stack) becomes a persistent 6-panel layout with a focus-tracking model
  (which panel currently has the cursor) instead of a view stack. `listModel`
  (the generic Task 7 widget) is adapted into a smaller, panel-sized rendering
  mode (bounded height, always-visible border) rather than full-screen.

## Testing

Same approach as v1: Bubbletea `Update()` fed with `tea.Msg` sequences,
`mongo.FakeClient` for data. New coverage needed specifically for this
redesign:
- Focus model: pressing `1`–`5` moves focus to the right panel; `j`/`k` only
  affects the focused panel; `Tab` moves focus to `[0]`.
- Popup lifecycle: opening a popup preserves the panel layout underneath;
  closing it (`Esc` or after a confirmed/cancelled action) returns focus
  correctly without losing panel selection state.
- Cascading selection: picking a database populates collections; picking a
  collection populates indexes and documents.

## Out of scope (unchanged from v1)

Aggregation pipeline builder, schema viewer, editing/deleting existing
connections, GridFS, server/performance stats, validation rules — still all
deferred, per the v1 spec.
