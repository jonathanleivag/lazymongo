package tui

import (
	"strings"
	"testing"
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

func TestComposeScreen_ContainsAllPanelsAndFooter(t *testing.T) {
	sidebar := []string{"PANEL-ONE", "PANEL-TWO"}
	out := composeScreen(sidebar, "MAIN-CONTENT", []string{"log line 1"}, "FOOTER-HINTS", 40, 5)
	for _, want := range []string{"PANEL-ONE", "PANEL-TWO", "MAIN-CONTENT", "log line 1", "FOOTER-HINTS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected composed screen to contain %q, got:\n%s", want, out)
		}
	}
}
