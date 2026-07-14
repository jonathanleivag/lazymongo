# lazymongo

Personal TUI for browsing/editing MongoDB, reusing `~/.config/mongo-connections.sh`
(the same file the `mgo` shell function uses). See
`docs/superpowers/specs/2026-07-11-lazymongo-design.md` for the design and
`docs/superpowers/plans/2026-07-11-lazymongo.md` for how it was built.

## Build & Install

To compile the application:

    go build -o lazymongo .

To install it globally to your `~/go/bin` directory:

    go install .

### Aliases (ZSH)
We have configured aliases in your `~/.zshrc` to run the application globally. You can launch it using any of the following:

    lazymongo
    lezymongo
    lm

## Keybindings

| Key | Action |
|---|---|
| `1`-`5` | Jump to a panel (Status/Databases/Collections/Indexes/Conexiones) |
| `Tab` | Jump to Documents panel (or to Indexes, if already on Documents) / Autocomplete field names (in filter/sort entry) |
| `j`/`k`, arrows | Move within the focused panel (or navigate cursor left/right within text inputs) |
| `Enter` | View document / connect (in Conexiones) / submit form, filter, or sort |
| `Esc` | Close active popup / exit active search / clear active filter or sort |
| `/` | Search/Filter: fuzzy-search in side panels; Mongo query filter in Documents |
| `s` | Sort: edit Mongo sort query in Documents (e.g. `{"createdAt": -1}`) |
| `Ctrl+f` | Quick local fuzzy-search within currently loaded documents |
| `n`/`p` | Next/previous page (in Documents) |
| `i`, `a` | Insert document / create connection, index, database, or collection |
| `e` | Edit field inline (document detail popup) / edit connection name/URI/color / rename collection |
| `y`, `c` | Copy selected field value to clipboard (in document detail popup; copies ObjectID as clean hex) |
| `E` | Edit full document in `$EDITOR` |
| `d`, `x` | Delete item (always confirms) |
| `?` | Context-sensitive help (shows hotkeys tailored to the active panel) |
| `Ctrl+c` | Quit |

*Note: The footer status line at the bottom of the screen updates dynamically to show only the hotkeys relevant to your active panel.*

## AI Commit Integration (lazygit)

We have configured custom commands inside your `lazygit` config to integrate with **Antigravity (`agy` CLI)**. The configuration is stored in the repository at [lazygit-config.yml](file:///Users/jonathanleivag/Development/jonathanleivag/lazymongo/lazygit-config.yml) (and applied in your local `~/Library/Application Support/lazygit/config.yml`).

### Commands inside `lazygit`:
- **Press `g` in Files Panel**: Generates a conventional commit message for your staged changes using Antigravity AI and commits them automatically.
- **Press `x` in Files Panel**: Explains the changes made in the highlighted file.
- **Press `x` in Commits Panel**: Explains what the highlighted commit does.
- **Press `x` in Branches Panel**: Summarizes the changes in the selected local branch compared to `main`.

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
