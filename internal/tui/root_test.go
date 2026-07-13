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
	if len(root.dbList.Items) != 1 || root.dbList.Items[0].ID != "shop" {
		t.Fatalf("expected dbList populated with 'shop', got %+v", root.dbList.Items)
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
		{"1", panelStatus},
		{"2", panelDatabases},
		{"3", panelCollections},
		{"4", panelIndexes},
		{"5", panelConnections},
	}
	for _, c := range cases {
		model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(c.key)})
		root = model.(RootModel)
		if root.focus != c.panel {
			t.Fatalf("pressing %q: expected focus=%v, got %v", c.key, c.panel, root.focus)
		}
	}
}

func TestRootModel_TabSwitchesFocusToDocuments(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	root := model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("expected focus=panelDocuments after Tab, got %v", root.focus)
	}
}

// TestRootModel_TabFromDatabasesMovesToDocuments is a regression check that
// Tab's primary behavior (jump to Documents from any other panel) still
// works after the fix that lets Tab fall through to per-panel routing when
// focus is already panelDocuments.
func TestRootModel_TabFromDatabasesMovesToDocuments(t *testing.T) {
	root, _ := rootModelAtDatabasesFocus(t)
	if root.focus != panelDatabases {
		t.Fatalf("precondition failed: expected focus=panelDatabases, got %v", root.focus)
	}
	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("expected focus=panelDocuments after Tab from panelDatabases, got %v", root.focus)
	}
}

// TestRootModel_TabWhileAlreadyOnDocuments_FallsThroughToSwitchToIndexes
// proves the old v1 behavior is restored: when focus is ALREADY
// panelDocuments, Tab must NOT be swallowed by the global handler — it needs
// to reach docListModel.Update, which (per its own internal Tab handling)
// emits switchToIndexesMsg, handled by the panelDocuments branch by setting
// focus=panelIndexes and loading indexes.
func TestRootModel_TabWhileAlreadyOnDocuments_FallsThroughToSwitchToIndexes(t *testing.T) {
	m, _ := newTestRootModel()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	root := model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("precondition failed: expected focus=panelDocuments after first Tab, got %v", root.focus)
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
	if len(root.collList.Items) != 2 {
		t.Fatalf("expected 2 collections loaded for 'shop', got %+v", root.collList.Items)
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
	if root.dbList.Cursor != 0 || root.dbList.Items[0].ID != "admin" {
		t.Fatalf("precondition failed: expected cursor 0 on 'admin', got cursor=%d items=%+v", root.dbList.Cursor, root.dbList.Items)
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("shop")})
	root = model.(RootModel)
	if root.dbList.Cursor != 0 || len(root.dbList.Items) != 1 || root.dbList.Items[0].ID != "shop" {
		t.Fatalf("precondition failed: expected filter narrowed to 'shop' at cursor 0, got cursor=%d items=%+v", root.dbList.Cursor, root.dbList.Items)
	}
	if cmd == nil {
		t.Fatal("expected a command (collections load) after fuzzy-filtering to a different database, even though the cursor index stayed at 0")
	}

	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.db != "shop" {
		t.Fatalf("expected m.db updated to 'shop' after fuzzy-filtering, got %q", root.db)
	}
	if len(root.collList.Items) != 1 || root.collList.Items[0].ID != "orders" {
		t.Fatalf("expected 1 collection loaded for 'shop', got %+v", root.collList.Items)
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
	if root.dbList.Filtering() {
		t.Fatal("expected filtering to be false after Enter")
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	root = model.(RootModel)
	if root.focus != panelCollections {
		t.Fatalf("expected '3' after Enter to jump focus to panelCollections, got focus=%v (dbList filter query=%q)", root.focus, root.dbList.FilterQuery())
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
	root.collList = newCollectionListModel([]string{"logs", "orders"})

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	root = model.(RootModel)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("orders")})
	root = model.(RootModel)
	if root.collList.Cursor != 0 || len(root.collList.Items) != 1 || root.collList.Items[0].ID != "orders" {
		t.Fatalf("precondition failed: expected filter narrowed to 'orders' at cursor 0, got cursor=%d items=%+v", root.collList.Cursor, root.collList.Items)
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
	root = driveCursorToItem(t, root, root.dbList, "shop")
	if root.db != "shop" {
		t.Fatalf("expected cursor to land on 'shop', got db=%q", root.db)
	}
	if len(root.collList.Items) != 2 {
		t.Fatalf("expected 2 collections loaded for 'shop', got %+v", root.collList.Items)
	}

	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	root = model.(RootModel)
	root = driveCursorToItem(t, root, root.collList, "orders")
	if root.coll != "orders" {
		t.Fatalf("expected cursor to land on 'orders', got coll=%q", root.coll)
	}
	if len(root.docList.docs) != 1 {
		t.Fatalf("expected 1 document loaded for 'orders', got %+v", root.docList.docs)
	}

	// Focus Documents and press Enter on the only document: this must open
	// the doc-detail popup.
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	if root.focus != panelDocuments {
		t.Fatalf("expected focus=panelDocuments after Tab, got %v", root.focus)
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
	root.dbList = newDatabaseListModel([]string{"shop", "admin"})
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
	root.dbList = newDatabaseListModel([]string{"shop-v1", "shop-v2"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v2")})
	r := model.(RootModel)
	if r.focus != panelDatabases {
		t.Fatalf("expected focus to remain on Databases while its filter query contains a digit, got %v", r.focus)
	}
	if len(r.dbList.Items) != 1 || r.dbList.Items[0].ID != "shop-v2" {
		t.Fatalf("expected filter to narrow to 'shop-v2', got %+v", r.dbList.Items)
	}
}

func TestRootModel_QuestionMarkDuringFilterDoesNotOpenHelp(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDatabaseListModel([]string{"shop"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	r := model.(RootModel)
	if r.popup == popupHelp {
		t.Fatal("expected '?' typed into an active filter query to NOT open help")
	}
	if r.dbList.FilterQuery() != "?" {
		t.Fatalf("expected '?' to be added to the filter query, got %q", r.dbList.FilterQuery())
	}
}

func TestRootModel_EscDuringDatabaseFilterRestoresFullListWithoutLeavingPanel(t *testing.T) {
	m, _ := newTestRootModel()
	root := *m
	root.dbList = newDatabaseListModel([]string{"shop", "admin"})
	root.focus = panelDatabases

	model, _ := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("adm")})
	model, _ = model.(RootModel).Update(tea.KeyMsg{Type: tea.KeyEsc})
	r := model.(RootModel)
	if r.focus != panelDatabases {
		t.Fatalf("expected Esc to only clear the filter, not change focus, got focus=%v", r.focus)
	}
	if len(r.dbList.Items) != 2 {
		t.Fatalf("expected full database list restored, got %d items", len(r.dbList.Items))
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
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyTab})
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
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyTab})
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
	if !strings.Contains(view, `{"name`) {
		t.Fatalf("expected the suggestion 'e' to appear appended right after the typed text, got:\n%s", view)
	}
}
