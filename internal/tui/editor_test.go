package tui

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestEditorCommand_UsesEnvVarWhenSet(t *testing.T) {
	t.Setenv("EDITOR", "code --wait")
	if got := editorCommand(); got != "code --wait" {
		t.Fatalf("expected 'code --wait', got %q", got)
	}
}

func TestEditorCommand_FallsBackToNvim(t *testing.T) {
	t.Setenv("EDITOR", "")
	if got := editorCommand(); got != "nvim" {
		t.Fatalf("expected fallback 'nvim', got %q", got)
	}
}

func TestWriteAndReadDocToTempFile_RoundTrips(t *testing.T) {
	doc := bson.M{"_id": "1", "name": "Ana", "age": int32(30)}

	path, cleanup, err := writeDocToTempFile(doc)
	if err != nil {
		t.Fatalf("writeDocToTempFile failed: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected temp file to exist: %v", err)
	}

	got, err := readDocFromTempFile(path)
	if err != nil {
		t.Fatalf("readDocFromTempFile failed: %v", err)
	}
	if got["name"] != "Ana" {
		t.Fatalf("expected name 'Ana' after round-trip, got %+v", got)
	}
}

func TestReadDocFromTempFile_InvalidJSONReturnsError(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.json")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.WriteString("not json"); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	f.Close()

	if _, err := readDocFromTempFile(f.Name()); err == nil {
		t.Fatal("expected an error for invalid JSON content")
	}
}

func TestEditFullRequestedMsg_TriggersExecProcessCmd(t *testing.T) {
	doc := bson.M{"_id": "1", "name": "Ana"}
	cmd := startEditFullFlow(doc)
	if cmd == nil {
		t.Fatal("expected a non-nil tea.Cmd from startEditFullFlow")
	}
	// startEditFullFlow must return a tea.ExecProcess-wrapped command; we only
	// assert it's non-nil here since tea.ExecProcess's returned Cmd type is
	// opaque and meant to be driven by the Bubbletea runtime, not called directly
	// in a unit test.
}
