# lazymongo

Personal TUI for browsing/editing MongoDB, reusing `~/.config/mongo-connections.sh`
(the same file the `mgo` shell function uses). See
`docs/superpowers/specs/2026-07-11-lazymongo-design.md` for the design and
`docs/superpowers/plans/2026-07-11-lazymongo.md` for how it was built.

## Build

    go build -o lazymongo .

## Run

    ./lazymongo <connection-name>   # e.g. ./lazymongo qa
    ./lazymongo                     # shows a picker of available connections

## Keybindings

| Key | Action |
|---|---|
| `1`-`5` | jump to a panel (Status/Databases/Collections/Indexes/Conexiones) |
| `Tab` | jump to Documents panel (or to Indexes, if already on Documents) |
| `j`/`k`, arrows | move within the focused panel |
| `Enter` | view document / connect (in Conexiones) / enter |
| `Esc` | close the active popup / exit an active search (fuzzy or Mongo filter) |
| `/` | buscar/filtrar: fuzzy-search por nombre en Databases/Collections/Indexes/Conexiones; filtro de query Mongo en Documentos |
| `Ctrl+f` | fuzzy-search entre los documentos ya cargados en pantalla (no dispara una nueva query) |
| `n`/`p` | next/previous page |
| `i`, `a` | insert document / create connection or index |
| `e` | edit field inline (document detail popup) / edit a connection's URI and color (Conexiones panel — name stays fixed) |
| `E` | edit full document in `$EDITOR` |
| `d`, `x` | delete (always confirms) |
| `?` | help |
| `Ctrl+c` | quit |

Moving the cursor in the Databases/Collections panels immediately loads the
next panel's contents (live preview) — no `Enter` needed. Connecting from the
Conexiones panel still requires `Enter`, since it's a real network connect.

## Testing

    go test ./...                    # unit tests, no Docker required
    ./scripts/test-integration.sh    # integration tests against a disposable local Mongo (requires Docker Desktop running)

Integration tests never run against `qa`/`prod` — only a throwaway
`mongo:7` container on port 27018.

## Manual smoke test against qa

**Note:** the UI changed from a full-screen drill-down to a persistent
5-panel + Documents-panel layout (see the design spec
`docs/superpowers/specs/2026-07-11-lazymongo-lazygit-ui-design.md`). Redo this
checklist even if you validated the drill-down version before — panel focus,
popups, and live-preview cascading are all new interaction paths.

Run: `go build -o lazymongo . && ./lazymongo qa`

Walk through, confirming each works as expected:

- [ ] All 5 side panels + Documents panel render on launch, Status panel shows `qa` in its assigned color
- [ ] `2` focuses Databases; `j`/`k` moves the cursor; Collections panel live-updates as you move
- [ ] `3` focuses Collections; moving the cursor live-updates Indexes and Documents
- [ ] `2` focuses Databases; `/` opens fuzzy search, typing narrows the list live, `Esc` restores the full list, `Enter` selects the highlighted database
- [ ] `5` focuses Conexiones; `/` fuzzy-searches connection names without triggering `a` (create connection) if the query contains the letter "a"
- [ ] `Tab` focuses Documents; `/` filters, `n`/`p` paginate
- [ ] `Enter` on a document opens the detail popup; `Esc` closes it, panels underneath are unchanged
- [ ] `e` on a field in the detail popup opens the inline editor; confirming actually updates the field (verify with `mongosh`/`mgo qa` afterward)
- [ ] `E` opens the full document in `nvim`; saving+exiting shows the confirm popup, then replaces it
- [ ] `i` inserts a new document after confirmation
- [ ] `d` on a document opens the delete confirmation popup
- [ ] `4` focuses Indexes; `a` opens the create-index popup, `d` drops one (both confirm first)
- [ ] `5` focuses Conexiones; `Enter` on a different saved connection reconnects
- [ ] `e` on a connection opens the edit form pre-filled with its current URI/color; `Enter` saves, `Esc` cancels; the name field cannot be typed into
- [ ] `d`/`x` on a connection opens a delete confirmation; confirming removes it from the list
- [ ] `?` opens/closes help; `Ctrl+c` quits cleanly
- [ ] The command log shows connect/filter/write history

Only after this passes should `prod` be used with `lazymongo`.
