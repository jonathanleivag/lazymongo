# lazymongo — Selected row background highlight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mark the selected row in every panel with a cyan background, matching lazygit's own selection highlighting.

**Architecture:** A single shared style (`cursorStyle` in `internal/tui/style.go`) already wraps the selected row's text in every panel (`internal/tui/panel.go`'s `visibleWindow`) — adding a background there applies uniformly with no other file touched.

**Tech Stack:** Go, `github.com/charmbracelet/lipgloss` (already a dependency).

## Global Constraints

- `cursorStyle` is the ONLY thing that changes. No other file is modified.
- Verified directly in Go (not assumed): wrapping an already-styled inner segment (e.g. `colorStyle(item.Color).Render(item.Label)`, or a BSON value from `styleBSONValue(...)`) with an outer style works correctly ONLY when the inner segment sits at the very end of the line, with no plain text trailing after it — the inner segment's own ANSI reset otherwise kills the outer style for anything after it. Every line in this codebase that currently reaches `cursorStyle.Render(...)` satisfies this (checked: `labelsFromListModel`, `labelsFromIndexes`, `docPanelLines`/`expandedDocLines`, the Status panel's connection-name line) — this is not a live bug today, but it is documented in a comment so a future change doesn't silently reintroduce it.
- lipgloss emits no ANSI without a real TTY (already documented in `panel.go`) — there is no meaningful `go test` assertion for a pure color change; verification is visual, in a real terminal.

---

### Task 1: Add a background to the selected-row style

**Files:**
- Modify: `internal/tui/style.go`

**Interfaces:**
- None — this is a self-contained style constant change with no new functions or signatures.

- [ ] **Step 1: Make the change**

In `internal/tui/style.go`, replace:

```go
var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	cursorStyle   = lipgloss.NewStyle().Bold(true)
	helpHintStyle = lipgloss.NewStyle().Faint(true)
)
```

with:

```go
var (
	titleStyle = lipgloss.NewStyle().Bold(true)

	// cursorStyle marks the selected row in every panel (applied by
	// panel.go's visibleWindow) with a cyan background, matching lazygit's
	// own selection highlighting — cyan is the same tone already used for
	// focusedBorderColor in panel.go.
	//
	// This only renders correctly today because every line that reaches
	// cursorStyle.Render(...) either has no embedded lipgloss styling at
	// all, or has its embedded styled segment (e.g. a connection's own
	// colorStyle(...).Render(...), or a BSON value from styleBSONValue(...)
	// in the expanded-document view) sitting at the very end of the line
	// with nothing plain trailing after it. Verified directly: when an
	// inner styled segment is followed by more plain text within the same
	// outer-wrapped line, the inner segment's own closing ANSI reset code
	// kills the outer background for everything after it — so if a future
	// change adds trailing plain text after an embedded styled segment on
	// a cursor-highlighted line, the background will visibly cut off
	// partway through. This is why, not a bug to "fix" here.
	cursorStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("6"))

	helpHintStyle = lipgloss.NewStyle().Faint(true)
)
```

- [ ] **Step 2: Verify it builds and vets clean**

Run: `cd ~/Development/jonathanleivag/lazymongo && go build ./... && go vet ./...`
Expected: no errors (this is a pure style-constant change; there is no unit test to write since lipgloss emits no ANSI without a real terminal — see this plan's Global Constraints)

- [ ] **Step 3: Run the full test suite to confirm no regressions**

Run: `go test ./... -count=1`
Expected: PASS — every pre-existing test still passes (this change only alters *how* the cursor line is styled, never *which* line is selected or what text it contains, so no test that asserts on line content or cursor position should be affected)

- [ ] **Step 4: Commit**

```bash
git add internal/tui/style.go
git commit -m "feat: highlight the selected row with a cyan background in every panel"
```

---

## Final manual smoke test

Run: `go build -o lazymongo . && ./lazymongo qa`, then confirm, across all 6 panels (Status, Databases, Collections, Indexes, Conexiones, Documentos):
- The row under the cursor shows a cyan background across the row
- Moving the cursor (`j`/`k`) moves the highlighted background correctly
- Conexiones' per-connection text color (amarillo/rojo/verde) is still legible on the cyan background when that connection is selected
- The expanded document view's highlighted field lines (e.g. `_id: ObjectID(...)`) show the background correctly, without any visible cutoff
