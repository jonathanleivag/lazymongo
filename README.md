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
| `Tab` | jump to the Documents panel |
| `j`/`k`, arrows | move within the focused panel |
| `Enter` | view document / connect (in Conexiones) / enter |
| `Esc` | close the active popup |
| `/` | filter documents |
| `n`/`p` | next/previous page |
| `i`, `a` | insert document / create connection or index |
| `e` | edit field inline |
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
