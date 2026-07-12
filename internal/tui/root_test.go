package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func newTestRootModel() (*RootModel, *mongo.FakeClient) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "total": int32(10)}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)
	return &m, fake
}

func TestRootModel_InitConnectsAndLoadsDatabases(t *testing.T) {
	m, _ := newTestRootModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a command that connects and lists databases")
	}
	msg := cmd()
	newModel, _ := m.Update(msg)
	root := newModel.(RootModel)
	if root.view != viewDatabaseList {
		t.Fatalf("expected view to become viewDatabaseList after connecting, got %v", root.view)
	}
}

func TestRootModel_FullDrillDownToDocumentDetail(t *testing.T) {
	// newTestRootModel's fake has exactly one database ("shop") and one
	// collection ("orders"), so pressing Enter always drills into that
	// single item — this simulates real key presses rather than
	// constructing itemSelectedMsg/documentChosenMsg by hand, since those
	// message types are only ever produced internally by listModel/docListModel
	// in response to an actual tea.KeyMsg, never accepted as external input.
	m, _ := newTestRootModel()

	cmd := m.Init()
	model, _ := m.Update(cmd())
	root := model.(RootModel)

	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewCollectionList {
		t.Fatalf("expected viewCollectionList, got %v", root.view)
	}

	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewDocumentList {
		t.Fatalf("expected viewDocumentList, got %v", root.view)
	}

	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if root.view != viewDocumentDetail {
		t.Fatalf("expected viewDocumentDetail, got %v", root.view)
	}
	_ = cmd
}

func TestRootModel_QuitsOnCtrlC(t *testing.T) {
	m, _ := newTestRootModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a quit command on Ctrl+C")
	}
}

func TestRootModel_ViewShowsConnectionNameInStatusBar(t *testing.T) {
	m, _ := newTestRootModel()
	view := m.View()
	if !strings.Contains(view, "qa") {
		t.Fatalf("expected status bar to show connection name 'qa', got view:\n%s", view)
	}
}

// rootModelInDocList drives a RootModel from init through selecting the
// "shop" database and "orders" collection, landing in viewDocumentList.
func rootModelInDocList(t *testing.T) (RootModel, *mongo.FakeClient) {
	t.Helper()
	m, fake := newTestRootModel()

	// newTestRootModel's fake has exactly one database and one collection,
	// so pressing Enter twice always drills into "shop" then "orders" —
	// see the comment on TestRootModel_FullDrillDownToDocumentDetail for
	// why real key presses are used instead of constructing itemSelectedMsg
	// by hand.
	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewDocumentList {
		t.Fatalf("setup failed: expected viewDocumentList, got %v", root.view)
	}
	return root, fake
}

// switchToIndexList drives a RootModel from viewDocumentList to viewIndexList
// by pressing Tab. This takes three Update calls, not one: Tab makes
// docListModel emit switchToIndexesMsg (1), which RootModel turns into an
// async m.loadIndexes() command (2), whose result (indexesLoadedMsg) is what
// actually sets m.view = viewIndexList (3).
func switchToIndexList(t *testing.T, root RootModel) RootModel {
	t.Helper()
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	model, cmd = root.Update(cmd())
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewIndexList {
		t.Fatalf("switchToIndexList setup failed: expected viewIndexList, got %v", root.view)
	}
	return root
}

func TestRootModel_InsertGoesThroughConfirmationBeforeWriting(t *testing.T) {
	root, fake := rootModelInDocList(t)

	model, _ := root.Update(insertRequestedMsg{})
	root = model.(RootModel)
	if root.editMode != "insert" {
		t.Fatalf("expected editMode 'insert', got %q", root.editMode)
	}

	// simulate the editor returning a new document (skipping the real
	// tea.ExecProcess invocation, which needs a real terminal/editor)
	model, _ = root.Update(editFullDoneMsg{Doc: bson.M{"total": int32(99)}})
	root = model.(RootModel)
	if root.view != viewConfirmWrite {
		t.Fatalf("expected viewConfirmWrite after editFullDoneMsg, got %v", root.view)
	}

	before := len(fake.Databases["shop"]["orders"])
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)

	if len(fake.Databases["shop"]["orders"]) != before+1 {
		t.Fatalf("expected InsertOne to add a document, got %d (was %d)", len(fake.Databases["shop"]["orders"]), before)
	}
}

func TestRootModel_InsertCancelledDoesNotWrite(t *testing.T) {
	root, fake := rootModelInDocList(t)

	model, _ := root.Update(insertRequestedMsg{})
	root = model.(RootModel)
	model, _ = root.Update(editFullDoneMsg{Doc: bson.M{"total": int32(99)}})
	root = model.(RootModel)

	before := len(fake.Databases["shop"]["orders"])
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	root = model.(RootModel)

	if len(fake.Databases["shop"]["orders"]) != before {
		t.Fatalf("expected no document added after cancelling, got %d (was %d)", len(fake.Databases["shop"]["orders"]), before)
	}
	if root.view != viewDocumentList {
		t.Fatalf("expected to return to viewDocumentList after cancelling, got %v", root.view)
	}
}

func TestRootModel_TabSwitchesToIndexList(t *testing.T) {
	root, fake := rootModelInDocList(t)
	fake.Indexes["shop"] = map[string][]mongo.IndexInfo{
		"orders": {{Name: "_id_", Key: bson.M{"_id": 1}, Unique: true}},
	}

	root = switchToIndexList(t, root)

	if root.view != viewIndexList {
		t.Fatalf("expected viewIndexList after Tab, got %v", root.view)
	}
	if len(root.idxList.indexes) != 1 {
		t.Fatalf("expected 1 index loaded, got %d", len(root.idxList.indexes))
	}
}

func TestRootModel_CreateIndexGoesThroughConfirmationBeforeWriting(t *testing.T) {
	root, fake := rootModelInDocList(t)
	root = switchToIndexList(t, root)

	model, _ := root.Update(indexCreateSubmittedMsg{KeysJSON: `{"total":1}`, Unique: false})
	root = model.(RootModel)
	if root.view != viewConfirmWrite {
		t.Fatalf("expected viewConfirmWrite after indexCreateSubmittedMsg, got %v", root.view)
	}

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	root = model.(RootModel)
	model, _ = root.Update(cmd())

	if len(fake.Indexes["shop"]["orders"]) != 1 {
		t.Fatalf("expected 1 index created, got %d", len(fake.Indexes["shop"]["orders"]))
	}
}

func TestRootModel_ErrorScreenDismissedByKeypress(t *testing.T) {
	m, _ := newTestRootModel()
	m.err = fmt.Errorf("boom")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	root := model.(RootModel)

	if root.err != nil {
		t.Fatalf("expected err to be cleared after keypress, got %v", root.err)
	}
}

func TestRootModel_ErrorScreenNotClearedByNonKeyMsg(t *testing.T) {
	m, _ := newTestRootModel()
	m.err = fmt.Errorf("boom")

	// a non-key message (e.g. an in-flight async command's result) should
	// not dismiss the error screen
	model, _ := m.Update(documentsLoadedMsg{Docs: nil, Total: 0})
	root := model.(RootModel)

	if root.err == nil {
		t.Fatal("expected err to remain set after a non-key message")
	}
}

func TestRootModel_QDoesNotQuitWhileCreatingConnection(t *testing.T) {
	m, _ := newTestRootModel()
	m.view = viewConnectionPicker
	m.connPicker.creating = true

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	root := model.(RootModel)

	if root.view != viewConnectionPicker {
		t.Fatalf("expected to stay on viewConnectionPicker, got %v", root.view)
	}
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("expected 'q' to NOT quit the app while creating a connection")
		}
	}
}

func TestRootModel_CtrlCStillQuitsOutsideTextEntry(t *testing.T) {
	m, _ := newTestRootModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a quit command on Ctrl+C")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("expected Ctrl+C to produce tea.Quit")
	}
}

// TestRootModel_NoArgLaunchPopulatesConnectionPickerFromConfig guards against
// the connection picker staying a permanently-empty zero-value model when
// NewRootModel is called with resolved == nil (the primary, no-CLI-args
// launch path). internal/config's connectionsFile var is unexported, so this
// test points it at a fixture via the exported config.SetConnectionsFileForTesting
// test helper (mirrors the httptest style: a small exported setter meant only
// for tests) instead of reading the real machine's ~/.config/mongo-
// connections.sh. This keeps the test hermetic: it asserts against the exact
// connections defined in internal/config/testdata/basic.sh ("prod", "qa"),
// not whatever happens to exist on the machine running it. Before the fix,
// m.connPicker was always a zero-value connectionPickerModel{} (empty list),
// so this test would fail regardless of which fixture is used.
func TestRootModel_NoArgLaunchPopulatesConnectionPickerFromConfig(t *testing.T) {
	restore := config.SetConnectionsFileForTesting("../config/testdata/basic.sh")
	t.Cleanup(restore)

	wantConns, err := config.ListConnections()
	if err != nil {
		t.Fatalf("test precondition failed: config.ListConnections() returned an error: %v", err)
	}
	if len(wantConns) != 2 {
		t.Fatalf("test precondition failed: expected fixture to define 2 connections, got %d: %+v", len(wantConns), wantConns)
	}

	fake := mongo.NewFakeClient()
	m := NewRootModel(fake, nil)

	if m.view != viewConnectionPicker {
		t.Fatalf("expected viewConnectionPicker, got %v", m.view)
	}
	if m.err != nil {
		t.Fatalf("expected no error, got %v", m.err)
	}
	if len(m.connPicker.list.Items) != len(wantConns) {
		t.Fatalf("expected picker to have %d connections (from fixture testdata/basic.sh), got %d: %+v",
			len(wantConns), len(m.connPicker.list.Items), m.connPicker.list.Items)
	}
	if !m.connPicker.list.CanCreate {
		t.Fatal("expected picker to allow creating a new connection")
	}

	view := m.View()
	for _, name := range []string{"qa", "prod"} {
		if !strings.Contains(view, name) {
			t.Fatalf("expected picker view to contain connection name %q (from fixture testdata/basic.sh), got view:\n%s", name, view)
		}
	}
}

func TestRootModel_DropIndexWritesImmediately(t *testing.T) {
	root, fake := rootModelInDocList(t)
	fake.Indexes["shop"] = map[string][]mongo.IndexInfo{
		"orders": {{Name: "total_1", Key: bson.M{"total": 1}}},
	}
	root = switchToIndexList(t, root)

	// idxListModel already confirms internally before emitting
	// indexDropConfirmedMsg, so RootModel executes it directly.
	model, cmd := root.Update(indexDropConfirmedMsg{Name: "total_1"})
	root = model.(RootModel)
	_, _ = root.Update(cmd())

	if len(fake.Indexes["shop"]["orders"]) != 0 {
		t.Fatalf("expected index dropped, got %+v", fake.Indexes["shop"]["orders"])
	}
}
