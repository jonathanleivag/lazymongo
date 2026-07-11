package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func withConnectionsFile(t *testing.T, path string) {
	t.Helper()
	old := connectionsFile
	connectionsFile = path
	t.Cleanup(func() { connectionsFile = old })
}

func TestResolveConnection_Found(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	conn, err := ResolveConnection("qa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Connection{Name: "qa", URI: "mongodb://localhost:27017/test", Color: "verde"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestResolveConnection_NoColorAssigned(t *testing.T) {
	withConnectionsFile(t, "testdata/no_colors.sh")

	conn, err := ResolveConnection("solo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Connection{Name: "solo", URI: "mongodb://localhost:27017/solo", Color: ""}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestResolveConnection_NotFound(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	_, err := ResolveConnection("nope")
	if err == nil {
		t.Fatal("expected an error for an unknown connection name, got nil")
	}
}

func TestResolveConnection_RejectsShellInjectionAttempt(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	marker := filepath.Join(t.TempDir(), "lazymongo-pwned")
	malicious := `qa]}"; touch ` + marker + `; echo "`

	_, err := ResolveConnection(malicious)
	if err == nil {
		t.Fatal("expected an error for a malicious connection name, got nil")
	}

	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("injection executed: marker file was created at %s", marker)
	}
}

func TestResolveConnection_ValidNamesStillWork(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	for _, name := range []string{"qa", "movatec-prod", "movatec_prod", "QA123"} {
		if !isValidConnectionName(name) {
			t.Fatalf("expected %q to be recognized as a valid connection name", name)
		}
	}

	conn, err := ResolveConnection("qa")
	if err != nil {
		t.Fatalf("unexpected error resolving valid name %q: %v", "qa", err)
	}
	want := Connection{Name: "qa", URI: "mongodb://localhost:27017/test", Color: "verde"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestListConnections(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	conns, err := ListConnections()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Slice(conns, func(i, j int) bool { return conns[i].Name < conns[j].Name })

	if len(conns) != 2 {
		t.Fatalf("got %d connections, want 2: %+v", len(conns), conns)
	}
	if conns[0].Name != "prod" || conns[1].Name != "qa" {
		t.Fatalf("got names %q, %q; want prod, qa", conns[0].Name, conns[1].Name)
	}
}
