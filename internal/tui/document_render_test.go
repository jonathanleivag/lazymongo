package tui

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestStyleBSONValue_ObjectID(t *testing.T) {
	id, err := bson.ObjectIDFromHex("640a53bfe6ef34dde9a6b4ba")
	if err != nil {
		t.Fatalf("unexpected error building ObjectID: %v", err)
	}
	out := styleBSONValue(id)
	if !strings.Contains(out, `ObjectID("640a53bfe6ef34dde9a6b4ba")`) {
		t.Fatalf("expected ObjectID hex representation, got %q", out)
	}
}

func TestStyleBSONValue_String(t *testing.T) {
	out := styleBSONValue("Ana")
	if !strings.Contains(out, `"Ana"`) {
		t.Fatalf("expected quoted string, got %q", out)
	}
}

func TestStyleBSONValue_Numbers(t *testing.T) {
	if !strings.Contains(styleBSONValue(int32(10)), "10") {
		t.Fatalf("expected '10' in rendering of int32(10), got %q", styleBSONValue(int32(10)))
	}
	if !strings.Contains(styleBSONValue(int64(20)), "20") {
		t.Fatalf("expected '20' in rendering of int64(20), got %q", styleBSONValue(int64(20)))
	}
	if !strings.Contains(styleBSONValue(3.5), "3.5") {
		t.Fatalf("expected '3.5' in rendering of float64(3.5), got %q", styleBSONValue(3.5))
	}
}

func TestStyleBSONValue_Bool(t *testing.T) {
	if !strings.Contains(styleBSONValue(true), "true") {
		t.Fatalf("expected 'true' in rendering, got %q", styleBSONValue(true))
	}
}

func TestStyleBSONValue_Nil(t *testing.T) {
	if !strings.Contains(styleBSONValue(nil), "null") {
		t.Fatalf("expected 'null' in rendering, got %q", styleBSONValue(nil))
	}
}

func TestStyleBSONValue_Time(t *testing.T) {
	ts := time.Date(2026, 7, 11, 18, 30, 43, 0, time.UTC)
	out := styleBSONValue(ts)
	if !strings.Contains(out, "2026-07-11") {
		t.Fatalf("expected date in rendering, got %q", out)
	}
}

func TestStyleBSONValue_Array(t *testing.T) {
	out := styleBSONValue(bson.A{"a", "b", "c"})
	if !strings.Contains(out, "Array (3)") {
		t.Fatalf("expected 'Array (3)' placeholder, got %q", out)
	}
}

func TestStyleBSONValue_NestedDocument(t *testing.T) {
	out := styleBSONValue(bson.M{"foo": "bar"})
	if !strings.Contains(out, "Object") {
		t.Fatalf("expected 'Object' placeholder, got %q", out)
	}
}

func TestExpandedDocLines_OneLinePerFieldSortedAlphabetically(t *testing.T) {
	doc := bson.M{"name": "Ana", "_id": "1", "age": int32(30)}
	lines := expandedDocLines(doc)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (one per field), got %d: %+v", len(lines), lines)
	}
	// "_id" sorts before "age" and "name" (ASCII '_' < 'a').
	if !strings.HasPrefix(lines[0], "_id:") {
		t.Fatalf("expected first line to be '_id:...', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "age:") {
		t.Fatalf("expected second line to be 'age:...', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "name:") {
		t.Fatalf("expected third line to be 'name:...', got %q", lines[2])
	}
}

func TestDocPanelLines_OnlyHighlightedDocumentExpands(t *testing.T) {
	docs := []bson.M{
		{"_id": "1", "name": "Ana"},
		{"_id": "2", "name": "Beto"},
		{"_id": "3", "name": "Caro"},
	}
	lines, effectiveCursor := docPanelLines(docs, 1)

	// doc 0: 1 collapsed line. doc 1 (highlighted): 2 field lines + 1 blank
	// separator. doc 2: 1 collapsed line. Total 5.
	if len(lines) != 5 {
		t.Fatalf("expected 5 total lines, got %d: %+v", len(lines), lines)
	}
	if effectiveCursor != 1 {
		t.Fatalf("expected effective cursor at line 1 (right after doc 0's single collapsed line), got %d", effectiveCursor)
	}
	if !strings.Contains(lines[1], "_id") {
		t.Fatalf("expected line 1 (start of the expanded block) to be a field line, got %q", lines[1])
	}
	if lines[3] != "" {
		t.Fatalf("expected a blank separator line after the expanded block, got %q", lines[3])
	}
}

func TestDocPanelLines_NonHighlightedDocsStayCollapsedToID(t *testing.T) {
	docs := []bson.M{
		{"_id": "1", "name": "Ana", "age": int32(30)},
		{"_id": "2", "name": "Beto"},
	}
	lines, _ := docPanelLines(docs, 1)

	if lines[0] != "1" {
		t.Fatalf("expected doc 0 collapsed to just its _id, got %q", lines[0])
	}
}

func TestDocPanelLines_EmptyDocsProducesNoLines(t *testing.T) {
	lines, effectiveCursor := docPanelLines(nil, 0)
	if len(lines) != 0 {
		t.Fatalf("expected no lines for an empty doc list, got %+v", lines)
	}
	if effectiveCursor != 0 {
		t.Fatalf("expected effective cursor 0 for an empty doc list, got %d", effectiveCursor)
	}
}
