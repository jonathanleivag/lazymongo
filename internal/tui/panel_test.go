package tui

import (
	"strings"
	"testing"
)

func TestRenderPanel_ShowsNumberedTitle(t *testing.T) {
	out := renderPanel(2, "Databases", []string{"admin", "test"}, 0, false, 30, 5, false)
	if !strings.Contains(out, "[2] Databases") {
		t.Fatalf("expected title with panel number, got:\n%s", out)
	}
}

func TestRenderPanel_HighlightsCursorItem(t *testing.T) {
	plain := renderPanel(2, "Databases", []string{"admin", "test"}, 0, false, 30, 5, false)
	cursorOnSecond := renderPanel(2, "Databases", []string{"admin", "test"}, 1, false, 30, 5, false)
	if plain == cursorOnSecond {
		t.Fatal("expected rendering to differ when cursor moves to a different item")
	}
}

func TestRenderPanel_FocusedDiffersFromUnfocused(t *testing.T) {
	unfocused := renderPanel(1, "Status", []string{"a"}, 0, false, 30, 5, false)
	focused := renderPanel(1, "Status", []string{"a"}, 0, true, 30, 5, false)
	if unfocused == focused {
		t.Fatal("expected focused and unfocused rendering to differ")
	}
	if !strings.Contains(focused, "▶") {
		t.Fatalf("expected a focus marker in the focused panel's heading, got:\n%s", focused)
	}
}

func TestRenderPanel_TruncatesToHeightKeepingCursorVisible(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	// innerHeight for height=5 leaves room for border+title; cursor near the end
	// must still be visible in the rendered output.
	out := renderPanel(2, "Databases", items, 7, false, 30, 5, false)
	if !strings.Contains(out, "h") {
		t.Fatalf("expected the item at the cursor to remain visible after truncation, got:\n%s", out)
	}
}

func TestRenderPanel_EmptyItemsDoesNotPanic(t *testing.T) {
	out := renderPanel(3, "Collections", nil, 0, false, 30, 5, false)
	if !strings.Contains(out, "[3] Collections") {
		t.Fatalf("expected title even with no items, got:\n%s", out)
	}
}

func TestRenderPanel_CursorStripsAnsi(t *testing.T) {
	coloredLine := "\x1b[32mpawmatch\x1b[0m"
	out := renderPanel(5, "Conexiones", []string{coloredLine}, 0, false, 30, 5, false)
	
	expectedPlain := cursorStyle.Render("pawmatch")
	if !strings.Contains(out, expectedPlain) {
		t.Fatalf("expected output to contain stripped clean highlighted selection %q, got:\n%q", expectedPlain, out)
	}
}
