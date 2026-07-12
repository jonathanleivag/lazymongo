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

// applyFilter drives a RootModel (already in viewDocumentList) through
// pressing "/" to open the filter prompt, typing filterText, and pressing
// Enter to submit it — mirroring real key presses instead of constructing
// filterSubmittedMsg by hand, since that message type is only ever produced
// internally by docListModel in response to actual tea.KeyMsg presses (see
// the comment on TestRootModel_FullDrillDownToDocumentDetail for the same
// rationale applied elsewhere in this file).
func applyFilter(t *testing.T, root RootModel, filterText string) RootModel {
	t.Helper()
	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	for _, r := range filterText {
		model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		root = model.(RootModel)
	}
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("applyFilter setup failed: expected a reload command after submitting the filter")
	}
	model, _ = root.Update(cmd())
	root = model.(RootModel)
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

// TestRootModel_FieldEditReloadsDocumentListNotEmpty proves that after a
// successful field edit, the document list is actually reloaded from Mongo
// (getting back the real docs/total) instead of the handler directly
// constructing a documentsLoadedMsg{Err: nil} with zero-value Docs/Total,
// which used to make the list appear completely empty even though the
// collection still has documents.
func TestRootModel_FieldEditReloadsDocumentListNotEmpty(t *testing.T) {
	root, fake := rootModelInDocList(t)
	fake.Databases["shop"]["orders"] = append(fake.Databases["shop"]["orders"], bson.M{"_id": "o2", "total": int32(20)})

	// drill into the first document's detail so m.docDetail.doc["_id"] is set
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if root.view != viewDocumentDetail {
		t.Fatalf("setup failed: expected viewDocumentDetail, got %v", root.view)
	}
	_ = cmd

	model, cmd = root.Update(fieldUpdateConfirmedMsg{Field: "total", NewValue: int32(99)})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command to perform the field update write")
	}
	writeResult := cmd()
	if _, ok := writeResult.(docWriteCompletedMsg); !ok {
		t.Fatalf("expected fieldUpdateConfirmedMsg handler to produce docWriteCompletedMsg (reused by the filter-aware reload path), got %T", writeResult)
	}

	model, cmd = root.Update(writeResult)
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected docWriteCompletedMsg to trigger a reload command")
	}
	reloaded := cmd()
	model, _ = root.Update(reloaded)
	root = model.(RootModel)

	if root.view != viewDocumentList {
		t.Fatalf("expected viewDocumentList after reload, got %v", root.view)
	}
	if len(root.docList.docs) != 2 {
		t.Fatalf("expected document list to be reloaded with 2 documents (not empty), got %d", len(root.docList.docs))
	}
	if root.docList.total != 2 {
		t.Fatalf("expected total to be 2, got %d", root.docList.total)
	}
}

// TestRootModel_DeleteReloadsDocumentListNotEmpty mirrors the field-edit
// case above for the delete flow.
func TestRootModel_DeleteReloadsDocumentListNotEmpty(t *testing.T) {
	root, fake := rootModelInDocList(t)
	fake.Databases["shop"]["orders"] = append(fake.Databases["shop"]["orders"], bson.M{"_id": "o2", "total": int32(20)})

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if root.view != viewDocumentDetail {
		t.Fatalf("setup failed: expected viewDocumentDetail, got %v", root.view)
	}

	model, cmd := root.Update(deleteConfirmedMsg{})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command to perform the delete write")
	}
	writeResult := cmd()
	if _, ok := writeResult.(docWriteCompletedMsg); !ok {
		t.Fatalf("expected deleteConfirmedMsg handler to produce docWriteCompletedMsg (reused by the filter-aware reload path), got %T", writeResult)
	}

	model, cmd = root.Update(writeResult)
	root = model.(RootModel)
	reloaded := cmd()
	model, _ = root.Update(reloaded)
	root = model.(RootModel)

	if root.view != viewDocumentList {
		t.Fatalf("expected viewDocumentList after reload, got %v", root.view)
	}
	if len(root.docList.docs) != 1 {
		t.Fatalf("expected document list to be reloaded with 1 remaining document (not empty), got %d", len(root.docList.docs))
	}
}

// TestRootModel_FilterPersistsAcrossPageChange proves that paging forward
// after applying a filter keeps m.filter set and reloads using it, instead
// of silently reverting to bson.M{} (the unfiltered collection) while the
// UI still shows "Filtro activo".
func TestRootModel_FilterPersistsAcrossPageChange(t *testing.T) {
	root, fake := rootModelInDocList(t)
	// give the collection enough documents that "n" (next page) is a no-op
	// pagination command rather than being swallowed for lack of a next page
	for i := 0; i < pageSize+1; i++ {
		fake.Databases["shop"]["orders"] = append(fake.Databases["shop"]["orders"], bson.M{"_id": fmt.Sprintf("extra-%d", i), "total": int32(10)})
	}
	root = applyFilter(t, root, `{"total":10}`)
	if root.filter == nil {
		t.Fatal("expected m.filter to be set after filterSubmittedMsg")
	}

	// pressing "n" makes docListModel emit pageChangedMsg, which RootModel's
	// viewDocumentList case immediately turns into a m.loadDocuments(...)
	// reload command (updating m.page synchronously along the way).
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a reload command after pressing 'n' to page forward")
	}
	model, _ = root.Update(cmd())
	root = model.(RootModel)

	if root.filter == nil {
		t.Fatal("expected m.filter to remain set after paging")
	}
	if root.filter["total"] != int32(10) {
		t.Fatalf("expected filter to still be {total:10}, got %+v", root.filter)
	}
	if root.page != 1 {
		t.Fatalf("expected page to have advanced to 1, got %d", root.page)
	}
}

// TestRootModel_FilterResetOnNewCollection proves entering a different
// collection resets any previously-active filter instead of carrying it
// over.
func TestRootModel_FilterResetOnNewCollection(t *testing.T) {
	root, _ := rootModelInDocList(t)
	root = applyFilter(t, root, `{"total":10}`)
	if root.filter == nil {
		t.Fatal("expected m.filter to be set after filterSubmittedMsg")
	}

	root.view = viewCollectionList
	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)

	if root.filter != nil {
		t.Fatalf("expected m.filter to be reset to nil after choosing a new collection, got %+v", root.filter)
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
