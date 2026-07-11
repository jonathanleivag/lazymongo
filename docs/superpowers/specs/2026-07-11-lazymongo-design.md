# lazymongo — Design Spec

Date: 2026-07-11
Status: Approved (v1 scope)

## Overview

`lazymongo` is a personal, keyboard-driven TUI for browsing and editing MongoDB
databases, in the spirit of `lazygit`/`lazysql`/`lazydocker`. It replaces (for
TUI-style browsing) the earlier attempt to use `vi-mongo`, which hit an
unresolved input bug under Ghostty (Enter/click not registering on its
changelog modal — traced to its `tview` fork predating Ghostty support in
`tcell`, but the actual button-focus root cause was never conclusively fixed
upstream). `mongosh` remains the tool for ad-hoc queries; `lazymongo` is for
visual browsing/editing.

Scope: **personal use only.** Private repo, no distribution concerns, no
requirement to support connections/environments other than the owner's own.

## Motivation

- `vi-mongo`'s TUI (Go, custom `tview` fork) has an input bug on Ghostty that
  blocks even the startup changelog modal. Investigated in depth (confirmed
  via a scripted pty repro) but not fixed in time to be usable.
- The owner already has a working connection-naming convention
  (`~/.config/mongo-connections.sh`, used by the `mgo` shell function) and
  wants a TUI that reuses it rather than introducing a second config format.
- Aggregation pipeline building is explicitly out of scope for v1 — the owner
  only needs document browsing/editing and index management day-to-day.

## Scope (v1)

**In scope:**
- Browse databases → collections → documents (drill-down navigation)
- Filter/query documents (JSON filter, like `db.coll.find({...})`)
- Paginate document results
- Create, edit (inline field or full-document via `$EDITOR`), and delete documents
- View, create, and drop indexes on a collection
- Reuse `~/.config/mongo-connections.sh` for named connections
- Create new named connections from within the TUI, including a color tag
  (yellow/red/green) persisted back into the same file
- Universal confirmation prompt before any write (insert/update/delete/index
  create/drop), on every connection, regardless of color

**Explicitly out of scope for v1:**
- Aggregation pipeline builder
- Schema inference/visualization (like Compass's schema tab)
- Editing/deleting existing connections from the TUI (only creating new ones —
  editing/removing stays a manual edit of `mongo-connections.sh`)
- GridFS, performance/server-stats views, validation rules
- Any read-only/lockdown mode for specific connections (the owner wants full
  read/write on every connection, including `prod` — the color tag plus the
  universal confirmation prompt are the intended safety net, not a hard block)

## Tech Stack

- **Language:** Go
- **TUI framework:** [Bubbletea](https://github.com/charmbracelet/bubbletea) +
  [Lipgloss](https://github.com/charmbracelet/lipgloss) (Charm ecosystem).
  Chosen specifically over a `tview`/`tcell`-based approach because Bubbletea
  is the framework `mongotui` (a comparable tool) already uses successfully
  under Ghostty, and it's the de facto standard for modern Go TUIs (`glow`,
  `gum`, `soft-serve`, `crush`, etc.) — avoiding the exact class of bug hit
  with `vi-mongo`.
- **MongoDB driver:** official `go.mongodb.org/mongo-driver`
- **Repo location:** `~/Development/jonathanleivag/lazymongo` (private,
  personal — not part of the `terminal-stack` repo)

## Architecture

```
lazymongo/
  main.go                    # entry point: resolve connection, start Bubbletea program
  internal/mongo/             # wrapper around go.mongodb.org/mongo-driver
                               # (interface MongoClient, for testability)
  internal/config/            # resolves a connection name -> URI + color,
                               # and appends new connections, by editing
                               # ~/.config/mongo-connections.sh directly
  internal/tui/                # Bubbletea models, one per view
  testdata/                    # fixture mongo-connections.sh files for config tests
```

### Connection resolution

`~/.config/mongo-connections.sh` is bash, not something `lazymongo` should
re-implement a parser for. To **read** a connection, `lazymongo` shells out to
bash itself:

```
bash -c 'source ~/.config/mongo-connections.sh; echo "${MONGO_CONNECTIONS[$1]}"; echo "${MONGO_CONNECTION_COLORS[$1]}"' _ <name>
```

This guarantees `lazymongo` and the `mgo` shell function always agree on what
a connection resolves to — there is exactly one source of truth.

Running `lazymongo` with no argument shows a `ConnectionPicker` view listing
every name currently in `MONGO_CONNECTIONS`, colored per
`MONGO_CONNECTION_COLORS` (default/neutral color if a name has no color entry
yet — e.g. the owner's existing `qa`/`prod` connections, added before this
tool existed).

### Connection file format (extended)

```bash
declare -A MONGO_CONNECTIONS=(
  [qa]="mongodb://..."
  [prod]="mongodb://..."
)

declare -A MONGO_CONNECTION_COLORS=(
  [qa]="verde"
  [prod]="rojo"
)
```

The colors array is new; existing files won't have it yet. `lazymongo` must
create the `MONGO_CONNECTION_COLORS=( ... )` block if it doesn't exist when
the owner creates their first connection through the TUI.

### Writing a new connection (from the TUI)

Triggered by pressing `a` in `ConnectionPicker`. Form fields, in order:
1. Name
2. Full MongoDB URI
3. Color — fixed 3-way choice: amarillo / rojo / verde

On confirm, `internal/config` edits `~/.config/mongo-connections.sh` as text:
- Locates the `declare -A MONGO_CONNECTIONS=(` block, inserts
  `  [name]="uri"` immediately before its closing `)`.
- Locates (or creates, if absent) the `declare -A MONGO_CONNECTION_COLORS=(`
  block, inserts `  [name]="color"` before its closing `)`.
- Writes the file back, then re-validates it by shelling out to
  `bash -n` — if that fails, the write is aborted and the original file
  content is restored (never leave the file in a broken state).

## Views & Navigation

Stack-based drill-down (`lazygit`/`k9s`-style): `Enter` goes a level deeper,
`Esc`/`h` goes back one level.

```
ConnectionPicker (only shown if launched with no argument)
  → DatabaseList
     → CollectionList
        → DocumentList (paginated table + JSON filter bar)
           → DocumentDetail (full JSON of one document)
        → IndexList (parallel tab within a collection, via Tab)
```

### Keybindings

| Key | Action |
|---|---|
| `j`/`k` or arrows | move selection |
| `Enter` | drill down |
| `Esc` / `h` | go back one level |
| `/` | filter (JSON query in DocumentList, text elsewhere) |
| `Tab` | switch between Documents/Indexes within a collection |
| `n` / `p` | next/previous page (DocumentList) |
| `i` / `a` | insert new document (DocumentList) / new connection (ConnectionPicker) |
| `e` | edit a field inline (DocumentDetail) |
| `E` | edit full document in `$EDITOR` (DocumentDetail) |
| `dd` / `x` | delete (document, index, or connection-scoped item) — always confirms |
| `?` | help / full keybinding list |
| `q` / `Ctrl+c` | quit |

## Editing

- **Inline (`e`):** in `DocumentDetail`, select a scalar field (string,
  number, bool, date), a small overlay input appears, `Enter` confirms →
  `UpdateOne({_id}, {$set: {field: value}})`. The field's original BSON type
  is preserved (no coercion to string).
- **Full document (`E`):** the document is dumped as pretty JSON to a temp
  file; Bubbletea suspends its screen (`tea.ExecProcess`, same mechanism
  `lazygit` uses to shell out to an editor) and opens `$EDITOR` (falls back to
  `nvim` if `$EDITOR` is unset — matches the existing `vi` alias). On save +
  exit, the file is parsed as JSON; if changed, `ReplaceOne` is called.

## Safety

- **Universal write confirmation:** every mutating action — insert, inline
  edit, full-document replace, delete document, create index, drop index —
  shows a `y`/`n` confirmation modal summarizing the change, on **every**
  connection (not just ones colored red). No fast-path/skip-confirmation mode.
- **Color-coded status bar:** the current connection's name and assigned
  color (amarillo/rojo/verde) are always visible in the status bar, so it's
  never ambiguous which environment is being edited.
- Full read/write access is intentionally allowed on every connection,
  including `prod` — there is no read-only lockdown mode. The confirmation
  prompt + color tag are the deliberate safety net, not a hard technical
  block.

## Testing

Personal tool — testing effort is targeted, not exhaustive:

- **Bubbletea `Update()` logic:** unit-tested by feeding sequences of
  `tea.Msg` and asserting resulting `Model` state — no real terminal needed.
  `internal/mongo` is abstracted behind a `MongoClient` interface so these
  tests inject a fake instead of hitting a real database.
- **`mongo-connections.sh` writing logic:** given a real bug already happened
  once from a stray comma in this exact file (hand-edited), the
  connection-insertion code gets fixture-based tests (`testdata/*.sh`)
  covering: no color block yet, existing comments, hyphenated names, etc. —
  every case asserts the resulting file still passes `bash -n` and that
  `MONGO_CONNECTIONS`/`MONGO_CONNECTION_COLORS` are re-readable afterward.
- **`internal/mongo` integration tests:** run against a disposable local
  MongoDB (`docker run --rm mongo:7`), never against `qa`/`prod`.
- **Manual validation:** new features are smoke-tested by hand against `qa`
  before being trusted near `prod`.

## Future ideas (explicitly not v1)

- Aggregation pipeline builder with stage-by-stage preview
- Schema inference view (field types / presence %, like Compass)
- Editing or deleting existing connections from within the TUI
- GridFS browsing, server/performance stats, validation rules
