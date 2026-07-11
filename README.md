# lazymongo

Personal TUI for browsing/editing MongoDB. See `docs/superpowers/specs/2026-07-11-lazymongo-design.md`
for the design, and `docs/superpowers/plans/2026-07-11-lazymongo.md` for the implementation plan.

## Build

    go build -o lazymongo .

## Run

    ./lazymongo <connection-name>   # resolves from ~/.config/mongo-connections.sh
    ./lazymongo                     # shows a picker of available connections
