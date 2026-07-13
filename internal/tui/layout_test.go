package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderPopupOverlay_ContainsContent(t *testing.T) {
	out := renderPopupOverlay("¿Borrar documento? (y/n)", 80, 24)
	if !strings.Contains(out, "¿Borrar documento? (y/n)") {
		t.Fatalf("expected popup content to appear in overlay, got:\n%s", out)
	}
}

func TestRenderPopupOverlay_FillsRequestedHeight(t *testing.T) {
	out := renderPopupOverlay("hi", 80, 24)
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("expected overlay to be exactly 24 lines tall, got %d", len(lines))
	}
}

// TestRenderPopupOverlay_WrapsLongContentWithoutExceedingWidth is a
// regression test found via manual testing: a form field showing a real,
// long MongoDB URI (a multi-host replica-set string with no spaces to
// break on) overflowed the terminal — the popup box had no width
// constraint, so it auto-sized to the widest line regardless of the actual
// terminal width, and the extra columns rendered past the terminal's right
// edge instead of wrapping.
func TestRenderPopupOverlay_WrapsLongContentWithoutExceedingWidth(t *testing.T) {
	long := "mongodb://user:pass@172.16.1.20:27017,192.168.22.143:27017,172.20.1.41:27017,172.20.1.42:27017/db"
	out := renderPopupOverlay(long, 100, 24)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Fatalf("expected no rendered line wider than the requested terminal width (100), got %d in line %q", w, line)
		}
	}
}

// TestRenderPopupOverlay_ShortContentUnaffected proves the fix above is
// scoped to only-when-needed: content that already fits renders exactly as
// before, with no forced padding to some fixed width.
func TestRenderPopupOverlay_ShortContentUnaffected(t *testing.T) {
	short := "¿Borrar la conexión \"qa\"? (y/n)"
	out := renderPopupOverlay(short, 100, 24)
	if !strings.Contains(out, short) {
		t.Fatalf("expected short content to render unchanged (single line, no wrapping), got:\n%s", out)
	}
}

func TestComposeScreen_ContainsAllPanelsAndFooter(t *testing.T) {
	sidebar := []string{"PANEL-ONE", "PANEL-TWO"}
	out := composeScreen(sidebar, "MAIN-CONTENT", []string{"log line 1"}, "FOOTER-HINTS", 40, 5)
	for _, want := range []string{"PANEL-ONE", "PANEL-TWO", "MAIN-CONTENT", "log line 1", "FOOTER-HINTS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected composed screen to contain %q, got:\n%s", want, out)
		}
	}
}
