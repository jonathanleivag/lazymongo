package tui

import (
	"strings"
	"testing"
)

func TestHelpModel_ContextSensitiveView(t *testing.T) {
	tests := []struct {
		focus       panelID
		expectedSub string
	}{
		{panelStatus, "Status"},
		{panelDatabases, "Bases de Datos"},
		{panelCollections, "Colecciones"},
		{panelIndexes, "Índices"},
		{panelConnections, "Conexiones"},
		{panelDocuments, "Documentos"},
	}

	for _, tt := range tests {
		m := helpModel{focus: tt.focus}
		view := m.View()
		if !strings.Contains(view, tt.expectedSub) {
			t.Errorf("expected view for panel %v to contain %q, got:\n%s", tt.focus, tt.expectedSub, view)
		}
	}
}
