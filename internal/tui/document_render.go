package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	bsonObjectIDStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // orange/yellow
	bsonStringStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	bsonNumberStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
	bsonBoolStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta
)

// styleBSONValue renders a document field's value as a single-line string,
// colored by its real Go/BSON type. Arrays and nested documents render as a
// collapsed placeholder ("Array (N)" / "Object") rather than their raw
// value — matching Mongo Compass's own collapsed display for nested data
// and avoiding new interactive state for drilling into sub-fields (the
// existing detail popup already covers that; see this plan's spec).
func styleBSONValue(v any) string {
	switch val := v.(type) {
	case nil:
		return helpHintStyle.Render("null")
	case bson.ObjectID:
		return bsonObjectIDStyle.Render(val.String())
	case string:
		return bsonStringStyle.Render(fmt.Sprintf("%q", val))
	case int32, int64, int, float64, float32:
		return bsonNumberStyle.Render(fmt.Sprintf("%v", val))
	case bool:
		return bsonBoolStyle.Render(fmt.Sprintf("%v", val))
	case time.Time:
		return bsonNumberStyle.Render(val.Format(time.RFC3339))
	case bson.A:
		return helpHintStyle.Render(fmt.Sprintf("Array (%d)", len(val)))
	case bson.M:
		return helpHintStyle.Render("Object")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// expandedDocLines renders one document's fields, one per line, colored by
// BSON type via styleBSONValue, in the same alphabetical field order
// docDetailModel already uses (see internal/tui/document_detail.go), so the
// inline expanded view and the detail popup agree on field order.
func expandedDocLines(doc bson.M) []string {
	fields := make([]string, 0, len(doc))
	for k := range doc {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	lines := make([]string, len(fields))
	for i, field := range fields {
		lines[i] = fmt.Sprintf("%s: %s", field, styleBSONValue(doc[field]))
	}
	return lines
}

// docPanelLines builds the flat display lines for panel [0], expanding only
// the document at cursor into all of its fields (via expandedDocLines)
// while every other document stays collapsed to its existing single _id
// line. It also returns the physical line index where the expanded block's
// first line lands: renderPanel (internal/tui/panel.go) takes a single
// line-index cursor and has no notion of multi-line items, so passing this
// computed index — not the raw docs-index cursor — is what makes the "> "
// marker land on the correct line once a document before the highlighted
// one may have expanded to more than one line.
func docPanelLines(docs []bson.M, cursor int) (lines []string, effectiveCursor int) {
	for i, doc := range docs {
		if i == cursor {
			effectiveCursor = len(lines)
			lines = append(lines, expandedDocLines(doc)...)
			lines = append(lines, "")
		} else {
			lines = append(lines, fmt.Sprintf("%v", doc["_id"]))
		}
	}
	return lines, effectiveCursor
}
