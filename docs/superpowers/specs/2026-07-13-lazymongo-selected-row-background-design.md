# lazymongo — Background highlight for the selected row

Date: 2026-07-13
Status: Approved

## Overview

Today, the selected row in any panel is marked only with a `>` prefix and
bold text (`cursorStyle = lipgloss.NewStyle().Bold(true)`, in
`internal/tui/style.go`) — no color. The owner wants the selected row
marked with a background color, matching how real `lazygit` (this
project's visual reference) highlights its selected row with a colored bar
across the whole line.

## Change

`cursorStyle` gains `.Background(lipgloss.Color("6"))` (cyan — the same
tone already used for `focusedBorderColor` in `internal/tui/panel.go`),
keeping `Bold(true)`. Since every panel's selected-row rendering already
goes through this one shared style (`internal/tui/panel.go`'s
`visibleWindow`, which wraps the cursor line with `cursorStyle.Render(...)`),
this single change applies uniformly across all 6 panels — Status,
Databases, Collections, Indexes, Conexiones, and Documentos — with no other
file touched.

## Verified constraint: nested ANSI styling

Verified directly in Go (not assumed): when a line already contains an
embedded, separately-styled segment (e.g. a connection's own
`colorStyle(item.Color).Render(item.Label)`, or a BSON value's
`styleBSONValue(...)` in the expanded-document view) and that whole line is
then wrapped by an outer style's `.Render(...)`, the *inner* segment's own
closing ANSI reset code terminates the *outer* style too — any plain text
appearing **after** that inner segment within the same line loses the
outer's background for the remainder of the line.

Checked every line-building function that currently feeds into
`cursorStyle.Render(...)`: in all of them, an embedded styled segment (when
one exists at all) always sits at the very end of the line, with nothing
plain trailing after it. So this background change does not trigger the
bug for any line that exists in the codebase today. It is a fragile
assumption for future code, not a currently-live bug — a comment at
`cursorStyle`'s declaration documents this so a future change that adds
trailing plain text after an embedded styled segment doesn't silently lose
the highlight partway through the line without anyone knowing why.

## Testing

lipgloss emits no ANSI codes without a real TTY (already documented in this
codebase's `panel.go`), so there is no meaningful `go test` assertion for
this change beyond confirming the code still builds
(`go build ./... && go vet ./...`). Verification is visual, in a real
terminal.

## Out of scope

Restructuring how colored segments are built (Conexiones' per-connection
color, the expanded-document view's per-type BSON coloring) to make
background highlighting robust against future trailing-plain-text lines —
deferred, since no such line exists today.
