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
| `j`/`k`, arrows | move |
| `Enter` | drill down / select |
| `Esc`, `h` | go back |
| `/` | filter documents |
| `Tab` | switch Documents/Indexes |
| `n`/`p` | next/previous page |
| `i`, `a` | insert document / create connection or index |
| `e` | edit field inline |
| `E` | edit full document in `$EDITOR` |
| `d`, `x` | delete (always confirms) |
| `?` | help |
| `q`, `Ctrl+c` | quit |

## Testing

    go test ./...                    # unit tests, no Docker required
    ./scripts/test-integration.sh    # integration tests against a disposable local Mongo (requires Docker Desktop running)

Integration tests never run against `qa`/`prod` — only a throwaway
`mongo:7` container on port 27018.
