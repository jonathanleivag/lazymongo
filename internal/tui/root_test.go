package tui

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestRootModel_DbCreateSubmittedMsgCreatesCollectionAndSelectsNewDatabase(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.dbList = newDbListModel([]string{"shop"})

	model, cmd := r.Update(dbCreateSubmittedMsg{DBName: "reports", CollName: "events"})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after dbCreateSubmittedMsg")
	}
	model, cmd = r.Update(cmd())
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after dbCreateCompletedMsg to reload databases")
	}
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["reports"]["events"]; !ok {
		t.Fatalf("expected FakeClient to have created reports.events, got %+v", fake.Databases)
	}
	found := false
	for _, item := range r.dbList.list.Items {
		if item.ID == "reports" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'reports' in the reloaded databases list, got %+v", r.dbList.list.Items)
	}
	if r.db != "reports" {
		t.Fatalf("expected active database to become 'reports', got %q", r.db)
	}
}

func TestRootModel_DbDropCompletedMsgOnActiveDatabaseClearsDependentPanels(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.dbList = newDbListModel([]string{"shop"})
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(dbDropConfirmedMsg{Name: "shop"})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after dbDropConfirmedMsg")
	}
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["shop"]; ok {
		t.Fatalf("expected FakeClient to have dropped 'shop', got %+v", fake.Databases)
	}
	if r.db != "" || r.coll != "" {
		t.Fatalf("expected db/coll cleared after dropping the active database, got db=%q coll=%q", r.db, r.coll)
	}
	if len(r.collList.list.Items) != 0 {
		t.Fatalf("expected collections list cleared, got %+v", r.collList.list.Items)
	}
}

func TestRootModel_DbDropCompletedMsgOnNonActiveDatabaseOnlyReloadsList(t *testing.T) {
	root, fake := newTestRootModel()
	fake.Databases["archive"] = map[string][]bson.M{"old": {}}
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.dbList = newDbListModel([]string{"shop", "archive"})
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(dbDropConfirmedMsg{Name: "archive"})
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if r.db != "shop" || r.coll != "orders" {
		t.Fatalf("expected db/coll untouched when dropping a non-active database, got db=%q coll=%q", r.db, r.coll)
	}
	if len(r.collList.list.Items) != 1 {
		t.Fatalf("expected collections list untouched, got %+v", r.collList.list.Items)
	}
}

func TestRootModel_CollCreateSubmittedMsgCreatesAndSelectsNewCollection(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.db = "shop"
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(collCreateSubmittedMsg{Name: "logs"})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after collCreateSubmittedMsg")
	}
	model, cmd = r.Update(cmd())
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after collCreateCompletedMsg to reload collections")
	}
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["shop"]["logs"]; !ok {
		t.Fatalf("expected FakeClient to have created shop.logs, got %+v", fake.Databases["shop"])
	}
	if r.coll != "logs" {
		t.Fatalf("expected active collection to become 'logs', got %q", r.coll)
	}
}

func TestRootModel_CollRenameSubmittedMsgRenamesAndFollowsCursor(t *testing.T) {
	root, fake := newTestRootModel()
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.collList = newCollListModel([]string{"orders"})

	model, cmd := r.Update(collRenameSubmittedMsg{OldName: "orders", NewName: "orders_v2"})
	r = model.(RootModel)
	model, cmd = r.Update(cmd())
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if _, ok := fake.Databases["shop"]["orders_v2"]; !ok {
		t.Fatalf("expected FakeClient to have renamed to shop.orders_v2, got %+v", fake.Databases["shop"])
	}
	if r.coll != "orders_v2" {
		t.Fatalf("expected active collection to follow the rename to 'orders_v2', got %q", r.coll)
	}
}

func TestRootModel_CollDropCompletedMsgOnActiveCollectionClearsDocumentsAndIndexes(t *testing.T) {
	root, _ := newTestRootModel()
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.collList = newCollListModel([]string{"orders"})
	r.docList = newDocListModel([]bson.M{{"_id": "o1"}}, 1, 0, pageSize)

	model, cmd := r.Update(collDropConfirmedMsg{Name: "orders"})
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if r.coll != "" {
		t.Fatalf("expected coll cleared after dropping the active collection, got %q", r.coll)
	}
	if len(r.docList.docs) != 0 {
		t.Fatalf("expected documents cleared, got %+v", r.docList.docs)
	}
}

func TestRootModel_CollDropCompletedMsgOnNonActiveCollectionOnlyReloadsList(t *testing.T) {
	root, fake := newTestRootModel()
	fake.Databases["shop"]["logs"] = []bson.M{}
	r := *root
	r.db = "shop"
	r.coll = "orders"
	r.collList = newCollListModel([]string{"orders", "logs"})
	r.docList = newDocListModel([]bson.M{{"_id": "o1"}}, 1, 0, pageSize)

	model, cmd := r.Update(collDropConfirmedMsg{Name: "logs"})
	r = model.(RootModel)
	model, _ = r.Update(cmd())
	r = model.(RootModel)

	if r.coll != "orders" {
		t.Fatalf("expected coll untouched when dropping a non-active collection, got %q", r.coll)
	}
	if len(r.docList.docs) != 1 {
		t.Fatalf("expected documents untouched, got %+v", r.docList.docs)
	}
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
	if len(root.dbList.list.Items) != 1 || root.dbList.list.Items[0].ID != "shop" {
		t.Fatalf("expected dbList populated with 'shop', got %+v", root.dbList.list.Items)
	}
}

func TestRootModel_WindowSizeMsgUpdatesDimensions(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	root := model.(RootModel)
	if root.width != 120 || root.height != 40 {
		t.Fatalf("expected width=120 height=40, got width=%d height=%d", root.width, root.height)
	}
}

func TestRootModel_LogfAppendsAndCapsAt50(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	for i := 0; i < 60; i++ {
		root.logf("line %d", i)
	}
	if len(root.log) != 50 {
		t.Fatalf("expected log capped at 50 entries, got %d", len(root.log))
	}
	if root.log[len(root.log)-1] != "line 59" {
		t.Fatalf("expected most recent entry to be 'line 59', got %q", root.log[len(root.log)-1])
	}
}

func TestRootModel_DefaultFocus_WithResolvedConnection_IsDatabases(t *testing.T) {
	m, _ := newTestRootModel()
	if m.focus != panelDatabases {
		t.Fatalf("expected focus=panelDatabases when launched with a resolved connection, got %v", m.focus)
	}
}

func TestRootModel_DefaultFocus_NoArgLaunch_IsConnections(t *testing.T) {
	fake := mongo.NewFakeClient()
	m := NewRootModel(fake, nil)
	if m.focus != panelConnections {
		t.Fatalf("expected focus=panelConnections on no-argument launch, got %v", m.focus)
	}
}

func TestRootModel_NumberKeysSwitchFocus(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	cases := []struct {
		key   string
		panel panelID
	}{
		{"0", panelDocuments},
		{"1", panelStatus},
		{"2", panelDatabases},
		{"3", panelCollections},
		{"4", panelIndexes},
		{"5", panelConnections},
	}
	for _, c := range cases {
		// Reset focus to test each key independently
		root.focus = panelStatus
		if c.panel == panelStatus {
			root.focus = panelDatabases
		}
		model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(c.key)})
		root = model.(RootModel)
		if root.focus != c.panel {
			t.Fatalf("pressing %q: expected focus=%v, got %v", c.key, c.panel, root.focus)
		}
	}
}

// TestRootModel_ZeroFromDatabasesMovesToDocuments is a regression check that
// Zero's primary behavior (jump to Documents from any other panel) works.
func TestRootModel_ZeroFromDatabasesMovesToDocuments(t *testing.T) {
	root, _ := rootModelAtDatabasesFocus(t)
	if root.focus != panelDatabases {
		t.Fatalf("precondition failed: expected focus=panelDatabases, got %v", root.focus)
	}
	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("expected focus=panelDocuments after pressing '0' from panelDatabases, got %v", root.focus)
	}
}

// TestRootModel_ZeroWhileAlreadyOnDocuments_FallsThroughToSwitchToIndexes
// proves that when focus is ALREADY panelDocuments, Tab must NOT be swallowed by
// the global handler — it needs to reach docListModel.Update, which emits
// switchToIndexesMsg, handled by setting focus=panelIndexes and loading indexes.
func TestRootModel_ZeroWhileAlreadyOnDocuments_FallsThroughToSwitchToIndexes(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root := model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("precondition failed: expected focus=panelDocuments after first '0', got %v", root.focus)
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	if root.focus != panelIndexes {
		t.Fatalf("expected focus=panelIndexes after Tab while already on panelDocuments, got %v", root.focus)
	}
}

// rootModelAtDatabasesFocus drives a RootModel to focus=panelDatabases with
// "shop" already loaded via Init(), landing on cursor 0 (the only database).
func rootModelAtDatabasesFocus(t *testing.T) (RootModel, *mongo.FakeClient) {
	t.Helper()
	m, fake := newTestRootModel()
	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	root = model.(RootModel)
	return root, fake
}

func TestRootModel_CursorMoveInDatabasesCascadesToCollections(t *testing.T) {
	root, fake := rootModelAtDatabasesFocus(t)
	fake.Databases["shop"]["users"] = []bson.M{{"_id": "u1"}}

	// cursor is already on "shop" (the only database) after Init; add a
	// second database ("admin") ahead of "shop" so a real cursor move ("j")
	// forces a cursor-change event, triggering the cascade to collections.
	model, _ := root.Update(connectedMsg{Databases: []string{"admin", "shop"}})
	root = model.(RootModel)

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command (collections load) after moving cursor to a new database")
	}
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if len(root.collList.list.Items) != 2 {
		t.Fatalf("expected 2 collections loaded for 'shop', got %+v", root.collList.list.Items)
	}
}

// TestRootModel_FuzzyFilterInDatabasesCascadesEvenWhenCursorStaysAtZero is a
// regression test for a bug reported after shipping fuzzy search: the
// Databases/Collections cascade used to fire only when the numeric cursor
// index changed. Fuzzy filtering always resets Cursor to 0 on every
// keystroke, so narrowing straight to a match at index 0 (a very common
// case — e.g. the first keystroke already narrows to the desired item) left
// the cursor index unchanged even though the highlighted database changed,
// so the cascade never fired: Collections/Documents never loaded for the
// searched-for database, and since Enter is intentionally a no-op on this
// panel (cascade is supposed to already have happened), the search appeared
// to do nothing at all.
func TestRootModel_FuzzyFilterInDatabasesCascadesEvenWhenCursorStaysAtZero(t *testing.T) {
	root, _ := rootModelAtDatabasesFocus(t)

	model, _ := root.Update(connectedMsg{Databases: []string{"admin", "shop"}})
	root = model.(RootModel)
	if root.dbList.list.Cursor != 0 || root.dbList.list.Items[0].ID != "admin" {
		t.Fatalf("precondition failed: expected cursor 0 on 'admin', got cursor=%d items=%+v", root.dbList.list.Cursor, root.dbList.list.Items)
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("shop")})
	root = model.(RootModel)
	if root.dbList.list.Cursor != 0 || len(root.dbList.list.Items) != 1 || root.dbList.list.Items[0].ID != "shop" {
		t.Fatalf("precondition failed: expected filter narrowed to 'shop' at cursor 0, got cursor=%d items=%+v", root.dbList.list.Cursor, root.dbList.list.Items)
	}
	if cmd == nil {
		t.Fatal("expected a command (collections load) after fuzzy-filtering to a different database, even though the cursor index stayed at 0")
	}

	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.db != "shop" {
		t.Fatalf("expected m.db updated to 'shop' after fuzzy-filtering, got %q", root.db)
	}
	if len(root.collList.list.Items) != 1 || root.collList.list.Items[0].ID != "orders" {
		t.Fatalf("expected 1 collection loaded for 'shop', got %+v", root.collList.list.Items)
	}
}

// TestRootModel_EnterAfterFilteringDatabasesLetsDigitJumpToNextPanel is a
// regression test for a second bug reported right after the cascade fix
// above: the user searched Databases, pressed Enter (which used to leave
// filtering active), then pressed "3" wanting to jump to the Collections
// panel — but since filtering was still active, "3" got typed into the
// search query instead (visibly producing "ha3" with no matches).
func TestRootModel_EnterAfterFilteringDatabasesLetsDigitJumpToNextPanel(t *testing.T) {
	root, _ := rootModelAtDatabasesFocus(t)

	model, _ := root.Update(connectedMsg{Databases: []string{"admin", "haddacloud-v2"}})
	root = model.(RootModel)

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ha")})
	root = model.(RootModel)
	if cmd != nil {
		model, _ = root.Update(cmd())
		root = model.(RootModel)
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if root.dbList.list.Filtering() {
		t.Fatal("expected filtering to be false after Enter")
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	root = model.(RootModel)
	if root.focus != panelCollections {
		t.Fatalf("expected '3' after Enter to jump focus to panelCollections, got focus=%v (dbList filter query=%q)", root.focus, root.dbList.list.FilterQuery())
	}
}

// TestRootModel_FuzzyFilterInCollectionsCascadesEvenWhenCursorStaysAtZero is
// the Collections-panel counterpart of the Databases regression above — the
// exact scenario the project owner reported: searching for a collection by
// name and having nothing happen.
func TestRootModel_FuzzyFilterInCollectionsCascadesEvenWhenCursorStaysAtZero(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"logs":   {},
		"orders": {{"_id": "o1", "total": int32(10)}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.focus = panelCollections
	// FakeClient.ListCollections iterates a Go map, so its order is not
	// deterministic — build collList directly with a fixed order instead of
	// depending on that order to land "logs" at cursor 0.
	root.collList = newCollListModel([]string{"logs", "orders"})

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("orders")})
	root = model.(RootModel)
	if root.collList.list.Cursor != 0 || len(root.collList.list.Items) != 1 || root.collList.list.Items[0].ID != "orders" {
		t.Fatalf("precondition failed: expected filter narrowed to 'orders' at cursor 0, got cursor=%d items=%+v", root.collList.list.Cursor, root.collList.list.Items)
	}
	if cmd == nil {
		t.Fatal("expected a command (indexes+documents load) after fuzzy-filtering to a different collection, even though the cursor index stayed at 0")
	}

	for _, bm := range flattenBatchMsg(t, cmd()) {
		model, _ = root.Update(bm)
		root = model.(RootModel)
	}
	if root.coll != "orders" {
		t.Fatalf("expected m.coll updated to 'orders' after fuzzy-filtering, got %q", root.coll)
	}
	if len(root.docList.docs) != 1 || root.docList.docs[0]["_id"] != "o1" {
		t.Fatalf("expected 1 document loaded for 'orders', got %+v", root.docList.docs)
	}
}

func TestRootModel_PopupHelpTogglesOnQuestionMark(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	root := model.(RootModel)
	if root.popup != popupHelp {
		t.Fatalf("expected popup=popupHelp after '?', got %v", root.popup)
	}
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	root = model.(RootModel)
	if root.popup != popupNone {
		t.Fatalf("expected any key to close help popup, got popup=%v", root.popup)
	}
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
		t.Fatalf("expected status panel to show connection name 'qa', got view:\n%s", view)
	}
}

func TestRootModel_ErrorClearsOnAnyKeypress(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.err = fmt.Errorf("boom")
	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	root = model.(RootModel)
	if root.err != nil {
		t.Fatalf("expected err to be cleared after a keypress, got %v", root.err)
	}
}

// TestRootModel_DocumentEnterOpensDocDetailPopup drives a full, real cascade:
// two databases are seeded so moving the cursor in Databases triggers a
// genuine cursor-change event (the cascade in this implementation fires on
// cursor movement, not on initial population — see the Update code for
// panelDatabases/panelCollections), which loads Collections; then moving the
// cursor in Collections cascades to load both Indexes and Documents. Once
// documents are loaded, focusing Documents (Tab) and pressing Enter on the
// only document must open the doc-detail popup.
func TestRootModel_DocumentEnterOpensDocDetailPopup(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["admin"] = map[string][]bson.M{}
	fake.Databases["shop"] = map[string][]bson.M{
		"logs":   {},
		"orders": {{"_id": "o1", "total": int32(10)}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	// Init connects and lists databases. FakeClient.ListDatabases/
	// ListCollections iterate over a Go map, so their order is NOT
	// deterministic across runs — the cascade in this implementation fires
	// only on an actual cursor *change*, so driveCursorToItem (below) is
	// used to land the cursor deterministically on "shop"/"orders" and
	// force a real cascade regardless of which index the fake happened to
	// put them at, keeping this test hermetic instead of flaky.
	model, _ := m.Update(m.Init()())
	root := model.(RootModel)

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	root = model.(RootModel)
	root = driveCursorToItem(t, root, root.dbList.list, "shop")
	if root.db != "shop" {
		t.Fatalf("expected cursor to land on 'shop', got db=%q", root.db)
	}
	if len(root.collList.list.Items) != 2 {
		t.Fatalf("expected 2 collections loaded for 'shop', got %+v", root.collList.list.Items)
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	root = model.(RootModel)
	root = driveCursorToItem(t, root, root.collList.list, "orders")
	if root.coll != "orders" {
		t.Fatalf("expected cursor to land on 'orders', got coll=%q", root.coll)
	}
	if len(root.docList.docs) != 1 {
		t.Fatalf("expected 1 document loaded for 'orders', got %+v", root.docList.docs)
	}

	// Focus Documents and press Enter on the only document: this must open
	// the doc-detail popup.
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("expected focus=panelDocuments after pressing '0', got %v", root.focus)
	}
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if root.popup != popupDocDetail {
		t.Fatalf("expected popup=popupDocDetail after Enter on a document, got %v", root.popup)
	}
	if root.docDetail.doc["_id"] != "o1" {
		t.Fatalf("expected docDetail to hold document 'o1', got %+v", root.docDetail.doc)
	}
}

// driveCursorToItem presses "j"/"k" on root (whose currently-focused panel
// backs the given list) until its cursor lands on the item with the given
// ID, applying each resulting cascade command (and flattening any
// tea.Batch) along the way, and returns the updated root. It always forces
// at least one real cursor-change event — even when the target is already
// at cursor 0 — by first moving down to a different index (when the list
// has more than one item) and then back up, since this implementation's
// database/collection cascade fires only on an actual cursor change, not on
// initial population. This keeps tests deterministic regardless of the
// order FakeClient happens to return names in (map iteration is unordered).
func driveCursorToItem(t *testing.T, root RootModel, list listModel, id string) RootModel {
	t.Helper()
	target := -1
	for i, item := range list.Items {
		if item.ID == id {
			target = i
			break
		}
	}
	if target == -1 {
		t.Fatalf("item %q not found in list %+v", id, list.Items)
	}

	apply := func(key string) {
		model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		root = model.(RootModel)
		if cmd == nil {
			return
		}
		for _, bm := range flattenBatchMsg(t, cmd()) {
			model, _ = root.Update(bm)
			root = model.(RootModel)
		}
	}

	if target == 0 {
		if len(list.Items) > 1 {
			apply("j")
			apply("k")
		}
		return root
	}
	for i := 0; i < target; i++ {
		apply("j")
	}
	return root
}

// flattenBatchMsg resolves a tea.Cmd's result, which may be a tea.BatchMsg
// (a []tea.Cmd) produced by tea.Batch, into the concrete messages each
// sub-command yields. Bubbletea's own runtime does this internally; tests
// driving Update() by hand need to do it explicitly.
func flattenBatchMsg(t *testing.T, msg tea.Msg) []tea.Msg {
	t.Helper()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		if c == nil {
			continue
		}
		out = append(out, flattenBatchMsg(t, c())...)
	}
	return out
}

func TestRootModel_InTextEntry_TrueWhileFilteringDatabasesPanel(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDbListModel([]string{"shop", "admin"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	r := model.(RootModel)
	if !r.inTextEntry() {
		t.Fatal("expected inTextEntry to be true while filtering the Databases panel")
	}
}

func TestRootModel_DigitDuringFilterAddsToQueryInsteadOfSwitchingFocus(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDbListModel([]string{"shop-v1", "shop-v2"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v2")})
	r := model.(RootModel)
	if r.focus != panelDatabases {
		t.Fatalf("expected focus to remain on Databases while its filter query contains a digit, got %v", r.focus)
	}
	if len(r.dbList.list.Items) != 1 || r.dbList.list.Items[0].ID != "shop-v2" {
		t.Fatalf("expected filter to narrow to 'shop-v2', got %+v", r.dbList.list.Items)
	}
}

func TestRootModel_QuestionMarkDuringFilterDoesNotOpenHelp(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDbListModel([]string{"shop"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	r := model.(RootModel)
	if r.popup == popupHelp {
		t.Fatal("expected '?' typed into an active filter query to NOT open help")
	}
	if r.dbList.list.FilterQuery() != "?" {
		t.Fatalf("expected '?' to be added to the filter query, got %q", r.dbList.list.FilterQuery())
	}
}

func TestRootModel_EscDuringDatabaseFilterRestoresFullListWithoutLeavingPanel(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDbListModel([]string{"shop", "admin"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adm")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyEsc})
	r := model.(RootModel)
	if r.focus != panelDatabases {
		t.Fatalf("expected Esc to only clear the filter, not change focus, got focus=%v", r.focus)
	}
	if len(r.dbList.list.Items) != 2 {
		t.Fatalf("expected full database list restored, got %d items", len(r.dbList.list.Items))
	}
}

func TestRootModel_CtrlFOnDocumentsPanelIsGuardedByInTextEntry(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.focus = panelDocuments
	root.docList = newDocListModel([]bson.M{{"_id": "o1"}, {"_id": "o2"}}, 2, 0, 20)

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	r := model.(RootModel)
	if !r.inTextEntry() {
		t.Fatal("expected inTextEntry true after Ctrl+f on Documents")
	}

	model, _ = r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	r2 := model.(RootModel)
	if r2.popup == popupHelp {
		t.Fatal("expected '?' typed during local fuzzy-find to NOT open help")
	}
}

func TestRootModel_DocumentsPanelExpandsOnlyHighlightedDocument(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {
			{"_id": "o1", "total": int32(10)},
			{"_id": "o2", "total": int32(20)},
		},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	model, _ = root.Update(documentsLoadedMsg{Docs: fake.Databases["shop"]["orders"], Total: 2})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("precondition failed: expected focus=panelDocuments, got %v", root.focus)
	}

	view := root.View()
	if !strings.Contains(view, `total: `) {
		t.Fatalf("expected the highlighted document's fields to appear in the rendered view, got:\n%s", view)
	}
	if !strings.Contains(view, "o1") {
		t.Fatalf("expected the highlighted document's _id ('o1') to appear, got:\n%s", view)
	}
}

func TestRootModel_DocumentsFilterShowsInlineSuggestion(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "name": "Ana"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	model, _ = root.Update(documentsLoadedMsg{Docs: fake.Databases["shop"]["orders"], Total: 1})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	for _, r := range `{"nam` {
		model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		root = model.(RootModel)
	}

	view := root.View()
	if !strings.Contains(view, `Filtro: {"nam`) {
		t.Fatalf("expected the typed filter text to appear, got:\n%s", view)
	}
	if !strings.Contains(view, "nam_e") {
		t.Fatalf("expected the cursor marker '_' followed by the suggestion 'e' right after the typed text, got:\n%s", view)
	}
}

func TestRootModel_EscWithAppliedFilterClearsAndReloadsDocuments(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "name": "Ana"}, {"_id": "o2", "name": "Beto"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	root.filter = bson.M{"name": "Ana"}
	model, _ = root.Update(documentsLoadedMsg{Docs: []bson.M{{"_id": "o1", "name": "Ana"}}, Total: 1})
	root = model.(RootModel)
	root.docList.filter = `{"name":"Ana"}`
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("precondition failed: expected focus=panelDocuments, got %v", root.focus)
	}

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEsc})
	root = model.(RootModel)
	if root.docList.FilterText() != "" {
		t.Fatalf("expected the applied filter text cleared, got %q", root.docList.FilterText())
	}
	if cmd == nil {
		t.Fatal("expected a command to reload documents")
	}
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if len(root.docList.docs) != 2 {
		t.Fatalf("expected all 2 documents reloaded without the filter, got %+v", root.docList.docs)
	}
}

func TestRootModel_DocumentsFilterCursorMarkerAtRealPosition(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "name": "Ana"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	model, _ = root.Update(documentsLoadedMsg{Docs: fake.Databases["shop"]["orders"], Total: 1})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	root = model.(RootModel)

	view := root.View()
	if !strings.Contains(view, "Filtro: {_}") {
		t.Fatalf("expected the cursor marker between the auto-closed braces, got:\n%s", view)
	}
}

// TestRootModel_AppliedFilterSurvivesTheReloadItTriggers is a regression
// test for a bug found via manual testing: applying a filter (Enter) sets
// docList.filter correctly, but the moment the resulting async reload
// completes, documentsLoadedMsg's handler used to replace m.docList with a
// brand-new newDocListModel(...) that never carries the filter text over —
// wiping it back to "" within a render or two of applying it. Since Esc
// clearing an applied filter depends on docList.filter being non-empty,
// this made Esc appear to do nothing: by the time a user reacted and
// pressed it, the filter text was already gone.
func TestRootModel_AppliedFilterSurvivesTheReloadItTriggers(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "name": "Ana"}, {"_id": "o2", "name": "Beto"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	model, _ = root.Update(documentsLoadedMsg{Docs: fake.Databases["shop"]["orders"], Total: 2})
	root = model.(RootModel)
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	for _, r := range `{"name":"Ana"}` {
		model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		root = model.(RootModel)
	}

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command (document reload) after applying the filter")
	}
	// This is the actual async reload command completing — the same
	// round-trip that happens for real once bubbletea runs the command.
	model, _ = root.Update(cmd())
	root = model.(RootModel)

	if root.docList.FilterText() != `{"name":"Ana"}` {
		t.Fatalf("expected the applied filter text to survive the reload, got %q", root.docList.FilterText())
	}

	// Now Esc must actually see a non-empty filter and clear it.
	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEsc})
	root = model.(RootModel)
	if root.docList.FilterText() != "" {
		t.Fatalf("expected Esc to clear the (now correctly non-empty) applied filter, got %q", root.docList.FilterText())
	}
	if cmd == nil {
		t.Fatal("expected a command to reload documents without the filter")
	}
}

// TestRootModel_FilterTextSurvivesPagination is a regression test for the
// same root cause as above: paginating (n/p) also reloads documents via
// documentsLoadedMsg, and used to wipe the filter indicator even though the
// underlying query (m.currentFilter()) correctly kept using it.
func TestRootModel_FilterTextSurvivesPagination(t *testing.T) {
	fake := mongo.NewFakeClient()
	docs := make([]bson.M, 25)
	for i := range docs {
		docs[i] = bson.M{"_id": fmt.Sprintf("o%d", i), "name": "Ana"}
	}
	fake.Databases["shop"] = map[string][]bson.M{"orders": docs}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.db = "shop"
	root.coll = "orders"
	root.filter = bson.M{"name": "Ana"}
	root.docList = newDocListModel(docs[:20], 25, 0, 20)
	root.docList.filter = `{"name":"Ana"}`
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	root = model.(RootModel)

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command (next page load)")
	}
	model, _ = root.Update(cmd())
	root = model.(RootModel)

	if root.docList.FilterText() != `{"name":"Ana"}` {
		t.Fatalf("expected the filter text to survive pagination, got %q", root.docList.FilterText())
	}
}

func TestRootModel_ConnectionUpdatedMsgReloadsConnectionsList(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mongo-connections.sh")
	fixture := "declare -A MONGO_CONNECTIONS=(\n  [qa]=\"mongodb://localhost:27017/test\"\n)\n\ndeclare -A MONGO_CONNECTION_COLORS=(\n  [qa]=\"verde\"\n)\n"
	if err := os.WriteFile(tmp, []byte(fixture), 0600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	restore := config.SetConnectionsFileForTesting(tmp)
	defer restore()

	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionUpdatedMsg{Conn: config.Connection{Name: "qa", URI: "mongodb://localhost:27017/test", Color: "verde"}})
	root = model.(RootModel)
	if len(root.connPicker.list.Items) != 1 || root.connPicker.list.Items[0].ID != "qa" {
		t.Fatalf("expected connections list reloaded with 'qa', got %+v", root.connPicker.list.Items)
	}
}

func TestRootModel_ConnectionUpdatedMsgMovesCursorToTheEditedConnection(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mongo-connections.sh")
	fixture := "declare -A MONGO_CONNECTIONS=(\n  [aaa]=\"mongodb://localhost:27017/aaa\"\n  [zzz]=\"mongodb://localhost:27017/zzz\"\n)\n\ndeclare -A MONGO_CONNECTION_COLORS=(\n  [aaa]=\"verde\"\n  [zzz]=\"verde\"\n)\n"
	if err := os.WriteFile(tmp, []byte(fixture), 0600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	restore := config.SetConnectionsFileForTesting(tmp)
	defer restore()

	m, _ := newTestRootModel()
	root := *m

	// connectionUpdatedMsg.Conn.Name is always the *new* name, regardless of
	// whether it came from a plain update or a rename — this message names
	// "zzz", the connection that sorts alphabetically last in the fixture,
	// to prove the handler positions the cursor by name rather than
	// leaving it at whatever index newConnectionPickerModel defaults to.
	model, _ := root.Update(connectionUpdatedMsg{Conn: config.Connection{Name: "zzz", URI: "mongodb://localhost:27017/zzz", Color: "verde"}})
	root = model.(RootModel)

	if got := root.connPicker.list.Cursor; got != 1 {
		t.Fatalf("expected cursor at index 1 ('zzz', alphabetically last), got %d (items: %+v)", got, root.connPicker.list.Items)
	}
	if got := root.connPicker.list.Items[root.connPicker.list.Cursor].ID; got != "zzz" {
		t.Fatalf("expected cursor to point at 'zzz', got %q", got)
	}
}

func TestRootModel_ConnectionDeletedMsgReloadsConnectionsList(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "mongo-connections.sh")
	fixture := "declare -A MONGO_CONNECTIONS=(\n  [staging]=\"mongodb://localhost:27017/staging\"\n)\n"
	if err := os.WriteFile(tmp, []byte(fixture), 0600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	restore := config.SetConnectionsFileForTesting(tmp)
	defer restore()

	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionDeletedMsg{Name: "qa"})
	root = model.(RootModel)
	if len(root.connPicker.list.Items) != 1 || root.connPicker.list.Items[0].ID != "staging" {
		t.Fatalf("expected connections list reloaded with only 'staging', got %+v", root.connPicker.list.Items)
	}
}

func TestRootModel_ConnectionUpdateErrMsgSetsErr(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionUpdateErrMsg{Err: fmt.Errorf("boom")})
	root = model.(RootModel)
	if root.err == nil {
		t.Fatal("expected root.err to be set after connectionUpdateErrMsg")
	}
}

func TestRootModel_ConnectionDeleteErrMsgSetsErr(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m

	model, _ := root.Update(connectionDeleteErrMsg{Err: fmt.Errorf("boom")})
	root = model.(RootModel)
	if root.err == nil {
		t.Fatal("expected root.err to be set after connectionDeleteErrMsg")
	}
}

func TestRootModel_InTextEntry_TrueWhileEditingConnection(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.focus = panelConnections
	root.connPicker.editing = true

	if !root.inTextEntry() {
		t.Fatal("expected inTextEntry to be true while editing a connection")
	}
}

// TestRootModel_SwitchingCollectionsClearsStaleFilterText is a regression
// test guarding the fix above from over-preserving: once documentsLoadedMsg
// stops wiping the filter unconditionally, switching to a different
// collection must still explicitly clear docList.filter itself (not just
// RootModel's m.filter) — otherwise a filter applied on collection A would
// incorrectly still show as "active" after cascading to collection B.
func TestRootModel_SwitchingCollectionsClearsStaleFilterText(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "name": "Ana"}},
		"users":  {{"_id": "u1", "name": "Beto"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.collList = newCollListModel([]string{"orders", "users"})
	root.docList.filter = `{"name":"Ana"}`
	root.focus = panelCollections

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command (indexes+documents load) after moving to a different collection")
	}
	if root.docList.FilterText() != "" {
		t.Fatalf("expected the stale filter text from the previous collection to be cleared immediately, got %q", root.docList.FilterText())
	}
}

func TestRootModel_SortingAppliesAndCarriesOver(t *testing.T) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "total": int32(10)}, {"_id": "o2", "total": int32(20)}},
		"users":  {{"_id": "u1", "name": "Beto"}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	root.collList = newCollListModel([]string{"orders", "users"})
	root.coll = "orders"
	root.focus = panelDocuments

	// Apply sort by typing and pressing Enter
	root.docList.sorting = true
	root.docList.sort = `{"total": -1}`
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected command to reload documents after sort submission")
	}

	if root.sort == nil || root.sort["total"] != int32(-1) {
		t.Fatalf("expected root.sort to be applied, got %+v", root.sort)
	}

	// Load documents (simulate reload complete)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.docList.SortText() != `{"total": -1}` {
		t.Fatalf("expected sort query to carry over, got %q", root.docList.SortText())
	}

	// Switch to different collection "users"
	root.focus = panelCollections
	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // moves to "users"
	root = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected a command after moving to different collection")
	}
	if root.sort != nil || root.docList.SortText() != "" {
		t.Fatalf("expected sort to be cleared when switching collections, got sort=%+v SortText=%q", root.sort, root.docList.SortText())
	}
}

func TestRootModel_FooterTextChangesWithFocus(t *testing.T) {
	fake := mongo.NewFakeClient()
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)

	tests := []struct {
		focus       panelID
		expectedSub string
	}{
		{panelStatus, "mover log"},
		{panelDatabases, "crear DB"},
		{panelCollections, "renombrar"},
		{panelIndexes, "crear índice"},
		{panelConnections, "conectar"},
		{panelDocuments, "ordenar"},
	}

	for _, tt := range tests {
		m.focus = tt.focus
		footer := m.footerText()
		if !strings.Contains(footer, tt.expectedSub) {
			t.Errorf("expected footer for panel %v to contain %q, got %q", tt.focus, tt.expectedSub, footer)
		}
	}
}

func TestRootModel_NoArgLaunch_RendersConnectionPickerAsModal(t *testing.T) {
	fake := mongo.NewFakeClient()
	m := NewRootModel(fake, nil)
	view := m.View()
	if !strings.Contains(view, "Seleccionar Conexión") {
		t.Fatalf("expected startup connection picker modal to be rendered, got:\n%s", view)
	}
}
