package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const pageSize = 20

type panelID int

const (
	panelStatus panelID = iota
	panelDatabases
	panelCollections
	panelIndexes
	panelConnections
	panelDocuments
)

type popupID int

const (
	popupNone popupID = iota
	popupDocDetail
	popupFieldEdit
	popupConfirmWrite
	popupDelete
	popupHelp
)

type RootModel struct {
	client mongo.Client

	conn   config.Connection
	db     string
	coll   string
	page   int64
	filter bson.M
	sort   bson.M

	connPicker connectionPickerModel
	dbList     dbListModel
	collList   collListModel
	docList    docListModel
	docDetail  docDetailModel
	fieldEdit  fieldEditModel
	idxList    idxListModel
	delete     deleteFlowModel

	// pending write awaiting confirmation via popupConfirmWrite
	confirmWrite         confirmModel
	editMode             string // "edit" | "insert", set before startEditFullFlow
	pendingDoc           bson.M
	pendingIndexKeysJSON string
	pendingIndexUnique   bool

	focus panelID
	popup popupID

	width  int
	height int
	log    []string

	err error
}

// NewRootModel builds the root model. If resolved is nil, focus starts on
// the Conexiones panel so the user can pick a saved connection; otherwise
// focus starts on Databases since the connection is already known.
func NewRootModel(client mongo.Client, resolved *config.Connection) RootModel {
	m := RootModel{client: client}
	if resolved != nil {
		m.conn = *resolved
		m.focus = panelDatabases
	} else {
		m.focus = panelConnections
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
		} else {
			m.connPicker = newConnectionPickerModel(conns)
		}
	}
	return m
}

// currentFilter returns the currently active document filter, defaulting to
// an empty (match-everything) filter when none has been set.
func (m RootModel) currentFilter() bson.M {
	if m.filter != nil {
		return m.filter
	}
	return bson.M{}
}

// currentSort returns the currently active document sort, defaulting to
// an empty sort when none has been set.
func (m RootModel) currentSort() bson.M {
	if m.sort != nil {
		return m.sort
	}
	return bson.M{}
}

// logf appends a formatted line to the command log, keeping only the most
// recent 50 entries.
func (m *RootModel) logf(format string, args ...any) {
	m.log = append(m.log, fmt.Sprintf(format, args...))
	if len(m.log) > 50 {
		m.log = m.log[len(m.log)-50:]
	}
}

func (m RootModel) Init() tea.Cmd {
	if m.focus != panelDatabases {
		return nil
	}
	return m.connectAndListDatabases()
}

type connectedMsg struct {
	Databases []string
	Err       error
}

func (m RootModel) connectAndListDatabases() tea.Cmd {
	client := m.client
	uri := m.conn.URI
	return func() tea.Msg {
		ctx := context.Background()
		if err := client.Connect(ctx, uri); err != nil {
			return connectedMsg{Err: err}
		}
		names, err := client.ListDatabases(ctx)
		if err != nil {
			return connectedMsg{Err: err}
		}
		return connectedMsg{Databases: names}
	}
}

type databasesLoadedMsg struct {
	Databases []string
	Err       error
	SelectDB  string
}

// loadDatabases refreshes the Databases panel without reconnecting
// (unlike connectAndListDatabases, used only for the initial connect).
// When selectDB is non-empty, the resulting handler positions the cursor
// on it and cascades into loadCollections, mirroring what moving the
// Databases cursor there by hand would do.
func (m RootModel) loadDatabases(selectDB string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		names, err := client.ListDatabases(context.Background())
		return databasesLoadedMsg{Databases: names, Err: err, SelectDB: selectDB}
	}
}

type collectionsLoadedMsg struct {
	Collections []string
	Err         error
	SelectName  string
}

// loadCollections refreshes the Collections panel. When selectName is
// non-empty, the resulting handler positions the cursor on it and
// cascades into loadIndexes/loadDocuments, mirroring what moving the
// Collections cursor there by hand would do. Manual cursor movement
// (case panelCollections below) always passes "" — it already does its
// own cascade inline.
func (m RootModel) loadCollections(selectName string) tea.Cmd {
	client, db := m.client, m.db
	return func() tea.Msg {
		names, err := client.ListCollections(context.Background(), db)
		return collectionsLoadedMsg{Collections: names, Err: err, SelectName: selectName}
	}
}

type documentsLoadedMsg struct {
	Docs  []bson.M
	Total int64
	Err   error
}

func (m RootModel) loadDocuments(filter bson.M, sortDoc bson.M) tea.Cmd {
	client, db, coll, page := m.client, m.db, m.coll, m.page
	return func() tea.Msg {
		ctx := context.Background()
		docs, err := client.Find(ctx, db, coll, filter, sortDoc, page*pageSize, pageSize)
		if err != nil {
			return documentsLoadedMsg{Err: err}
		}
		total, err := client.CountDocuments(ctx, db, coll, filter)
		if err != nil {
			return documentsLoadedMsg{Err: err}
		}
		return documentsLoadedMsg{Docs: docs, Total: total}
	}
}

type indexesLoadedMsg struct {
	Indexes []mongo.IndexInfo
	Err     error
}

func (m RootModel) loadIndexes() tea.Cmd {
	client, db, coll := m.client, m.db, m.coll
	return func() tea.Msg {
		idxs, err := client.ListIndexes(context.Background(), db, coll)
		return indexesLoadedMsg{Indexes: idxs, Err: err}
	}
}

// docWriteCompletedMsg/indexWriteCompletedMsg report the result of a write
// that already went through the popupConfirmWrite confirmation step.
type docWriteCompletedMsg struct{ Err error }
type indexWriteCompletedMsg struct{ Err error }

type dbCreateCompletedMsg struct {
	Err    error
	DBName string
}
type dbDropCompletedMsg struct {
	Err  error
	Name string
}
type collCreateCompletedMsg struct {
	Err  error
	Name string
}
type collRenameCompletedMsg struct {
	Err     error
	NewName string
}
type collDropCompletedMsg struct {
	Err  error
	Name string
}

// inTextEntry reports whether the current focus/popup is in an active
// text-entry sub-state, where printable keys like "?" must be typed
// literally rather than treated as a global shortcut.
func (m RootModel) inTextEntry() bool {
	switch {
	case m.focus == panelConnections && m.connPicker.creating:
		return true
	case m.focus == panelConnections && m.connPicker.editing:
		return true
	case m.focus == panelConnections && m.connPicker.list.Filtering():
		return true
	case m.focus == panelDatabases && m.dbList.list.Filtering():
		return true
	case m.focus == panelDatabases && m.dbList.creating:
		return true
	case m.focus == panelCollections && m.collList.list.Filtering():
		return true
	case m.focus == panelCollections && m.collList.creating:
		return true
	case m.focus == panelCollections && m.collList.editing:
		return true
	case m.focus == panelIndexes && m.idxList.creating:
		return true
	case m.focus == panelIndexes && m.idxList.Filtering():
		return true
	case m.focus == panelDocuments && m.docList.filtering:
		return true
	case m.focus == panelDocuments && m.docList.sorting:
		return true
	case m.focus == panelDocuments && m.docList.FuzzyFiltering():
		return true
	case m.popup == popupFieldEdit && !m.fieldEdit.confirming:
		return true
	}
	return false
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.err != nil {
			m.err = nil
			return m, nil
		}
		if keyMsg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.popup == popupHelp {
			m.popup = popupNone
			return m, nil
		}
		if keyMsg.String() == "?" && !m.inTextEntry() {
			m.popup = popupHelp
			return m, nil
		}
		if m.popup == popupNone && !m.inTextEntry() {
			switch keyMsg.String() {
			case "r":
				m.logf("Refrescando datos...")
				switch m.focus {
				case panelDatabases:
					return m, m.loadDatabases("")
				case panelCollections:
					return m, m.loadCollections("")
				case panelDocuments:
					return m, m.loadDocuments(m.currentFilter(), m.currentSort())
				case panelIndexes:
					return m, m.loadIndexes()
				case panelConnections:
					conns, err := config.ListConnections()
					if err != nil {
						m.err = err
						return m, nil
					}
					m.connPicker = newConnectionPickerModel(conns)
					return m, nil
				}
			case "1":
				m.focus = panelStatus
				return m, nil
			case "2":
				m.focus = panelDatabases
				return m, nil
			case "3":
				m.focus = panelCollections
				return m, nil
			case "4":
				m.focus = panelIndexes
				return m, nil
			case "5":
				m.focus = panelConnections
				return m, nil
			case "0":
				if m.focus != panelDocuments {
					m.focus = panelDocuments
					return m, nil
				}
			}
		}
	}

	switch msg := msg.(type) {
	case connectedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.dbList = newDbListModel(msg.Databases)
		m.logf("Conectado a %s", m.conn.Name)
		if len(msg.Databases) > 0 {
			m.db = msg.Databases[0]
			return m, m.loadCollections("")
		}
		return m, nil

	case connectionChosenMsg:
		conn, err := config.ResolveConnection(msg.Conn.Name)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.conn = conn
		m.focus = panelDatabases
		return m, m.connectAndListDatabases()

	case connectionCreatedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		return m, nil

	case connectionCreateErrMsg:
		m.err = msg.Err
		return m, nil

	case connectionUpdatedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		for i, item := range m.connPicker.list.Items {
			if item.ID == msg.Conn.Name {
				m.connPicker.list.Cursor = i
				break
			}
		}
		return m, nil

	case connectionUpdateErrMsg:
		m.err = msg.Err
		return m, nil

	case connectionDeletedMsg:
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		return m, nil

	case connectionDeleteErrMsg:
		m.err = msg.Err
		return m, nil

	case databasesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.dbList = newDbListModel(msg.Databases)
		if msg.SelectDB == "" {
			return m, nil
		}
		for i, item := range m.dbList.list.Items {
			if item.ID == msg.SelectDB {
				m.dbList.list.Cursor = i
				break
			}
		}
		m.db = msg.SelectDB
		return m, m.loadCollections("")

	case dbCreateSubmittedMsg:
		client, dbName, collName := m.client, msg.DBName, msg.CollName
		m.logf("Creando database %q (collection inicial %q)", dbName, collName)
		return m, func() tea.Msg {
			err := client.CreateCollection(context.Background(), dbName, collName)
			return dbCreateCompletedMsg{Err: err, DBName: dbName}
		}

	case dbCreateCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadDatabases(msg.DBName)

	case dbDropConfirmedMsg:
		client, name := m.client, msg.Name
		m.logf("Borrando database %q", name)
		return m, func() tea.Msg {
			err := client.DropDatabase(context.Background(), name)
			return dbDropCompletedMsg{Err: err, Name: name}
		}

	case dbDropCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		if msg.Name == m.db {
			m.db = ""
			m.coll = ""
			m.page = 0
			m.filter = nil
			m.collList = newCollListModel(nil)
			m.docList = newDocListModel(nil, 0, 0, pageSize)
			m.idxList = newIdxListModel(nil)
		}
		return m, m.loadDatabases("")

	case collectionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.collList = newCollListModel(msg.Collections)
		selectName := msg.SelectName
		if selectName == "" && len(msg.Collections) > 0 {
			selectName = msg.Collections[0]
		}
		if selectName == "" {
			return m, nil
		}
		for i, item := range m.collList.list.Items {
			if item.ID == selectName {
				m.collList.list.Cursor = i
				break
			}
		}
		m.coll = selectName
		m.page = 0
		m.filter = nil
		m.docList.filter = ""
		m.docList.filterCursor = 0
		m.sort = nil
		m.docList.sort = ""
		m.docList.sortCursor = 0
		return m, tea.Batch(m.loadIndexes(), m.loadDocuments(bson.M{}, bson.M{}))

	case collCreateSubmittedMsg:
		client, db, name := m.client, m.db, msg.Name
		m.logf("Creando collection %q en %q", name, db)
		return m, func() tea.Msg {
			err := client.CreateCollection(context.Background(), db, name)
			return collCreateCompletedMsg{Err: err, Name: name}
		}

	case collCreateCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadCollections(msg.Name)

	case collRenameSubmittedMsg:
		client, db, oldName, newName := m.client, m.db, msg.OldName, msg.NewName
		m.logf("Renombrando collection %q a %q en %q", oldName, newName, db)
		return m, func() tea.Msg {
			err := client.RenameCollection(context.Background(), db, oldName, newName)
			return collRenameCompletedMsg{Err: err, NewName: newName}
		}

	case collRenameCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadCollections(msg.NewName)

	case collDropConfirmedMsg:
		client, db, name := m.client, m.db, msg.Name
		m.logf("Borrando collection %q en %q", name, db)
		return m, func() tea.Msg {
			err := client.DropCollection(context.Background(), db, name)
			return collDropCompletedMsg{Err: err, Name: name}
		}

	case collDropCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		if msg.Name == m.coll {
			m.coll = ""
			m.page = 0
			m.filter = nil
			m.sort = nil
			m.docList = newDocListModel(nil, 0, 0, pageSize)
			m.idxList = newIdxListModel(nil)
		}
		return m, m.loadCollections("")

	case documentsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		newList := newDocListModel(msg.Docs, msg.Total, m.page, pageSize)
		// Every reload used to replace docList wholesale, silently wiping
		// filter/filterCursor even when the reload was the direct result of
		// applying that same filter (or just paginating with it still
		// active). Carrying them over here keeps the "Filtro activo: ..."
		// indicator — and the Esc-clears-it behavior, which depends on
		// filter being non-empty — correct after the reload completes.
		// Callers that should show no filter (e.g. switching collections)
		// clear m.docList.filter themselves before triggering the reload.
		newList.filter = m.docList.filter
		newList.filterCursor = m.docList.filterCursor
		newList.sort = m.docList.sort
		newList.sortCursor = m.docList.sortCursor
		m.docList = newList
		return m, nil

	case indexesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.idxList = newIdxListModel(msg.Indexes)
		return m, nil

	case fieldSelectedMsg:
		m.fieldEdit = newFieldEditModel(msg.Field, msg.Value)
		m.popup = popupFieldEdit
		return m, nil

	case deleteRequestedMsg:
		id := m.docDetail.doc["_id"]
		m.delete = newDeleteFlowModel(id)
		m.popup = popupDelete
		return m, nil

	case deleteConfirmedMsg:
		id := m.docDetail.doc["_id"]
		client, db, coll := m.client, m.db, m.coll
		m.popup = popupNone
		m.logf("Borrando documento %v", id)
		return m, func() tea.Msg {
			err := client.DeleteOne(context.Background(), db, coll, id)
			return docWriteCompletedMsg{Err: err}
		}

	case fieldUpdateConfirmedMsg:
		client, db, coll, id := m.client, m.db, m.coll, m.docDetail.doc["_id"]
		field, value := msg.Field, msg.NewValue
		m.popup = popupNone
		m.logf("Actualizando campo %q del documento %v", field, id)
		return m, func() tea.Msg {
			err := client.UpdateField(context.Background(), db, coll, id, field, value)
			return docWriteCompletedMsg{Err: err}
		}

	case editFullRequestedMsg:
		m.editMode = "edit"
		return m, startEditFullFlow(m.docDetail.doc)

	case editFullDoneMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.pendingDoc = msg.Doc
		m.confirmWrite = confirmModel{Message: "¿Guardar estos cambios en Mongo?"}
		m.popup = popupConfirmWrite
		return m, nil

	case indexCreateSubmittedMsg:
		m.pendingIndexKeysJSON = msg.KeysJSON
		m.pendingIndexUnique = msg.Unique
		m.confirmWrite = confirmModel{Message: "¿Crear este índice?"}
		m.popup = popupConfirmWrite
		return m, nil

	case indexDropConfirmedMsg:
		client, db, coll, name := m.client, m.db, m.coll, msg.Name
		m.logf("Borrando índice %q", name)
		return m, func() tea.Msg {
			err := client.DropIndex(context.Background(), db, coll, name)
			return indexWriteCompletedMsg{Err: err}
		}

	case docWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.pendingDoc = nil
		return m, m.loadDocuments(m.currentFilter(), m.currentSort())

	case indexWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadIndexes()

	case valueCopiedMsg:
		m.logf("Copiado al portapapeles: %s", msg.Text)
		return m, nil

	case listBackMsg:
		// with no view stack, "back" just closes whatever popup is open.
		m.popup = popupNone
		return m, nil
	}

	if m.popup != popupNone {
		var cmd tea.Cmd
		switch m.popup {
		case popupDocDetail:
			var docCmd tea.Cmd
			m.docDetail, docCmd = m.docDetail.Update(msg)
			if docCmd != nil {
				return m.dispatch(docCmd())
			}
		case popupFieldEdit:
			var feCmd tea.Cmd
			m.fieldEdit, feCmd = m.fieldEdit.Update(msg)
			if feCmd != nil {
				return m.dispatch(feCmd())
			}
		case popupDelete:
			var delCmd tea.Cmd
			m.delete, delCmd = m.delete.Update(msg)
			if delCmd != nil {
				return m.dispatch(delCmd())
			}
		case popupConfirmWrite:
			var cwCmd tea.Cmd
			m.confirmWrite, cwCmd = m.confirmWrite.Update(msg)
			if cwCmd != nil {
				result, ok := cwCmd().(confirmResultMsg)
				if !ok {
					return m, cwCmd
				}
				m.popup = popupNone
				if !result.Confirmed {
					m.pendingDoc = nil
					m.pendingIndexKeysJSON = ""
					m.pendingIndexUnique = false
					return m, nil
				}
				return m.executePendingWrite()
			}
		}
		return m, cmd
	}

	switch m.focus {
	case panelDatabases:
		// listModel.Update still emits itemSelectedMsg on Enter, but we intentionally
		// don't handle it here: cursor movement above already cascades into loading
		// collections for the highlighted database, so Enter has nothing left to do.
		// Do not "fix" this into an itemSelectedMsg handler.
		beforeID := highlightedItemID(m.dbList.list.Items, m.dbList.list.Cursor)
		var listCmd tea.Cmd
		m.dbList, listCmd = m.dbList.Update(msg)
		afterID := highlightedItemID(m.dbList.list.Items, m.dbList.list.Cursor)
		if afterID != "" && afterID != beforeID {
			m.db = afterID
			return m, m.loadCollections("")
		}
		return m, listCmd

	case panelCollections:
		// Same as panelDatabases above: Enter is a silent no-op by design, since
		// cursor movement already cascades into loading indexes/documents for the
		// highlighted collection.
		beforeID := highlightedItemID(m.collList.list.Items, m.collList.list.Cursor)
		var listCmd tea.Cmd
		m.collList, listCmd = m.collList.Update(msg)
		afterID := highlightedItemID(m.collList.list.Items, m.collList.list.Cursor)
		if afterID != "" && afterID != beforeID {
			m.coll = afterID
			m.page = 0
			m.filter = nil
			m.sort = nil
			// documentsLoadedMsg now carries docList.filter across a reload
			// (see its handler) so a stale filter from the previous
			// collection doesn't linger — clear it explicitly here too.
			m.docList.filter = ""
			m.docList.filterCursor = 0
			m.docList.sort = ""
			m.docList.sortCursor = 0
			return m, tea.Batch(m.loadIndexes(), m.loadDocuments(bson.M{}, bson.M{}))
		}
		return m, listCmd

	case panelIndexes:
		var idxCmd tea.Cmd
		m.idxList, idxCmd = m.idxList.Update(msg)
		if idxCmd != nil {
			return m.dispatch(idxCmd())
		}
		return m, nil

	case panelConnections:
		var cmd tea.Cmd
		m.connPicker, cmd = m.connPicker.Update(msg)
		return m, cmd

	case panelDocuments:
		var cmd tea.Cmd
		m.docList, cmd = m.docList.Update(msg)
		if cmd != nil {
			switch out := cmd().(type) {
			case pageChangedMsg:
				m.page = out.Page
				return m, m.loadDocuments(m.currentFilter(), m.currentSort())
			case filterSubmittedMsg:
				m.page = 0
				var filter bson.M
				if err := bson.UnmarshalExtJSON([]byte(out.Filter), false, &filter); err != nil {
					m.err = fmt.Errorf("filtro inválido: %w", err)
					return m, nil
				}
				m.filter = filter
				m.logf("Filtro aplicado: %s", out.Filter)
				return m, m.loadDocuments(filter, m.currentSort())
			case filterClearedMsg:
				m.filter = nil
				m.page = 0
				m.logf("Filtro removido")
				return m, m.loadDocuments(bson.M{}, m.currentSort())
			case sortSubmittedMsg:
				m.page = 0
				var sortDoc bson.M
				if err := bson.UnmarshalExtJSON([]byte(out.Sort), false, &sortDoc); err != nil {
					m.err = fmt.Errorf("orden inválido: %w", err)
					return m, nil
				}
				m.sort = sortDoc
				m.logf("Orden aplicado: %s", out.Sort)
				return m, m.loadDocuments(m.currentFilter(), sortDoc)
			case sortClearedMsg:
				m.sort = nil
				m.page = 0
				m.logf("Orden removido")
				return m, m.loadDocuments(m.currentFilter(), bson.M{})
			case documentChosenMsg:
				m.docDetail = newDocDetailModel(out.Doc)
				m.popup = popupDocDetail
				return m, nil
			case insertRequestedMsg:
				m.editMode = "insert"
				return m, startEditFullFlow(bson.M{})
			case switchToIndexesMsg:
				m.focus = panelIndexes
				return m, m.loadIndexes()
			}
			return m, cmd
		}
	}
	return m, nil
}

// executePendingWrite performs the write that was just confirmed via
// popupConfirmWrite: either a document insert/replace (set up by
// insertRequestedMsg/editFullRequestedMsg) or an index creation (set up by
// indexCreateSubmittedMsg).
func (m RootModel) executePendingWrite() (tea.Model, tea.Cmd) {
	client, db, coll := m.client, m.db, m.coll

	if m.pendingIndexKeysJSON != "" {
		var keys bson.D
		if err := bson.UnmarshalExtJSON([]byte(m.pendingIndexKeysJSON), false, &keys); err != nil {
			m.err = fmt.Errorf("keys de índice inválidas: %w", err)
			return m, nil
		}
		unique := m.pendingIndexUnique
		m.pendingIndexKeysJSON = ""
		return m, func() tea.Msg {
			_, err := client.CreateIndex(context.Background(), db, coll, keys, unique)
			return indexWriteCompletedMsg{Err: err}
		}
	}

	doc, mode := m.pendingDoc, m.editMode
	switch mode {
	case "insert":
		return m, func() tea.Msg {
			_, err := client.InsertOne(context.Background(), db, coll, doc)
			return docWriteCompletedMsg{Err: err}
		}
	case "edit":
		id := m.docDetail.doc["_id"]
		return m, func() tea.Msg {
			err := client.ReplaceOne(context.Background(), db, coll, id, doc)
			return docWriteCompletedMsg{Err: err}
		}
	}
	return m, nil
}

// dispatch re-enters Update with an already-produced message, used by
// sub-models whose Update signature returns a concrete child type instead
// of tea.Model.
func (m RootModel) dispatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.Update(msg)
}

func (m RootModel) View() string {
	if m.err != nil {
		return renderPopupOverlay(fmt.Sprintf("Error: %v\n\n[cualquier tecla] continuar", m.err), m.width, m.height)
	}

	if m.conn.Name == "" {
		var pickerContent string
		if m.connPicker.creating || m.connPicker.editing || m.connPicker.confirmingDelete {
			pickerContent = m.connPicker.View()
		} else {
			var b strings.Builder
			b.WriteString(titleStyle.Render("lazymongo — Seleccionar Conexión") + "\n\n")
			b.WriteString("Usa [j/k] para moverte, [Enter] para conectar.\n")
			b.WriteString("[a] crear nueva, [e] editar, [d] borrar.\n\n")
			b.WriteString(m.connPicker.View())
			pickerContent = b.String()
		}
		return renderPopupOverlay(pickerContent, m.width, m.height)
	}

	switch m.popup {
	case popupDocDetail:
		return renderPopupOverlay(m.docDetail.View(), m.width, m.height)
	case popupFieldEdit:
		return renderPopupOverlay(m.fieldEdit.View(), m.width, m.height)
	case popupConfirmWrite:
		return renderPopupOverlay(m.confirmWrite.View(), m.width, m.height)
	case popupDelete:
		return renderPopupOverlay(m.delete.View(), m.width, m.height)
	case popupHelp:
		return renderPopupOverlay(helpModel{focus: m.focus}.View(), m.width, m.height)
	}

	if m.focus == panelConnections && (m.connPicker.creating || m.connPicker.editing || m.connPicker.confirmingDelete) {
		return renderPopupOverlay(m.connPicker.View(), m.width, m.height)
	}
	if m.focus == panelIndexes && (m.idxList.creating || m.idxList.confirmingDrop) {
		return renderPopupOverlay(m.idxList.View(), m.width, m.height)
	}
	if m.focus == panelDatabases && (m.dbList.creating || m.dbList.confirmingDrop) {
		return renderPopupOverlay(m.dbList.View(), m.width, m.height)
	}
	if m.focus == panelCollections && (m.collList.creating || m.collList.editing || m.collList.confirmingDrop) {
		return renderPopupOverlay(m.collList.View(), m.width, m.height)
	}

	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 30
	}

	sidebarWidth := width / 3
	if sidebarWidth < 24 {
		sidebarWidth = 24
	}
	mainWidth := width - sidebarWidth - 2
	if mainWidth < 30 {
		mainWidth = 30
	}
	panelHeight := 5

	statusLines := []string{colorStyle(m.conn.Color).Render(m.conn.Name), fmt.Sprintf("%s.%s", m.db, m.coll)}

	dbTitle := "Databases"
	if m.dbList.list.Filtering() {
		dbTitle = "Databases — Buscar: " + m.dbList.list.FilterQuery() + "_"
	}
	collTitle := "Collections"
	if m.collList.list.Filtering() {
		collTitle = "Collections — Buscar: " + m.collList.list.FilterQuery() + "_"
	}
	idxTitle := "Indexes"
	if m.idxList.Filtering() {
		idxTitle = "Indexes — Buscar: " + m.idxList.FilterQuery() + "_"
	}
	connTitle := "Conexiones"
	if m.connPicker.list.Filtering() {
		connTitle = "Conexiones — Buscar: " + m.connPicker.list.FilterQuery() + "_"
	}

	p1 := renderPanel(1, "Status", statusLines, 0, m.focus == panelStatus, sidebarWidth, panelHeight)
	p2 := renderPanel(2, dbTitle, labelsFromListModel(m.dbList.list), m.dbList.list.Cursor, m.focus == panelDatabases, sidebarWidth, panelHeight)
	p3 := renderPanel(3, collTitle, labelsFromListModel(m.collList.list), m.collList.list.Cursor, m.focus == panelCollections, sidebarWidth, panelHeight)
	p4 := renderPanel(4, idxTitle, labelsFromIndexes(m.idxList), m.idxList.cursor, m.focus == panelIndexes, sidebarWidth, panelHeight)
	p5 := renderPanel(5, connTitle, labelsFromListModel(m.connPicker.list), m.connPicker.list.Cursor, m.focus == panelConnections, sidebarWidth, panelHeight)

	docTitle := fmt.Sprintf("Documentos (%d total, pág %d)", m.docList.total, m.docList.page+1)
	if m.docList.FuzzyFiltering() {
		docTitle += " — Buscar: " + m.docList.FuzzyQuery() + "_"
	}
	docLines, docCursor := docPanelLines(m.docList.docs, m.docList.cursor)
	offset := 0
	if m.docList.sorting {
		suggestion := helpHintStyle.Render(m.docList.SortSuggestion())
		line := "Orden:  " + m.docList.SortBeforeCursor() + "_" + suggestion + m.docList.SortAfterCursor()
		docLines = append([]string{line}, docLines...)
		offset++
	} else if m.docList.sort != "" {
		docLines = append([]string{"Orden activo:  " + m.docList.sort}, docLines...)
		offset++
	}
	if m.docList.filtering {
		suggestion := helpHintStyle.Render(m.docList.FilterSuggestion())
		line := "Filtro: " + m.docList.FilterBeforeCursor() + "_" + suggestion + m.docList.FilterAfterCursor()
		docLines = append([]string{line}, docLines...)
		offset++
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
		offset++
	}
	docCursor += offset
	mainHeight := panelHeight*5 - 5
	main := renderPanel(0, docTitle, docLines, docCursor, m.focus == panelDocuments, mainWidth, mainHeight)

	footer := m.footerText() + helpHintStyle.Render("  |  Creado por jonathanleivag")

	return composeScreen([]string{p1, p2, p3, p4, p5}, main, lastLogLines(m.log, 4), footer, mainWidth, 4)
}

func (m RootModel) footerText() string {
	switch m.focus {
	case panelStatus:
		return "[0-5] panel  [j/k] mover log  [?] ayuda  [Ctrl+c] salir"
	case panelDatabases:
		return "[0-5] panel  [j/k] mover  [/] buscar  [a] crear DB  [d] borrar DB  [?] ayuda  [Ctrl+c] salir"
	case panelCollections:
		return "[0-5] panel  [j/k] mover  [/] buscar  [a] crear  [e] renombrar  [d] borrar  [?] ayuda  [Ctrl+c] salir"
	case panelIndexes:
		return "[0-5] panel  [j/k] mover  [/] buscar  [a] crear índice  [d] borrar índice  [?] ayuda  [Ctrl+c] salir"
	case panelConnections:
		return "[0-5] panel  [j/k] mover  [Enter] conectar  [/] buscar  [a] crear  [e] editar  [d] borrar  [?] ayuda  [Ctrl+c] salir"
	case panelDocuments:
		return "[1-5] panel  [Tab] índices  [j/k] mover  [Enter] ver  [/] filtro  [s] ordenar  [Ctrl+f] buscar en docs  [i] insertar  [d] borrar  [?] ayuda  [Ctrl+c] salir"
	default:
		return "[0-5] panel  [j/k] mover  [?] ayuda  [Ctrl+c] salir"
	}
}

// highlightedItemID returns the ID of the item at cursor, or "" if items is
// empty or cursor is out of range. The Databases/Collections cascade compares
// this across an Update call rather than comparing the raw cursor index,
// because fuzzy filtering resets Cursor to 0 on every keystroke — comparing
// cursor values alone would miss every case where the item at index 0
// changes underneath a cursor that never numerically moves.
func highlightedItemID(items []listItem, cursor int) string {
	if cursor < 0 || cursor >= len(items) {
		return ""
	}
	return items[cursor].ID
}

// labelsFromListModel renders each item's colored label as a plain line for
// panel display (no cursor/border chrome — renderPanel adds that). While
// filtering with no matches, a single placeholder line is shown instead of
// an empty list — safe because there's no real selection to misalign
// against an empty item set.
func labelsFromListModel(m listModel) []string {
	if m.Filtering() && len(m.Items) == 0 {
		return []string{helpHintStyle.Render("(sin coincidencias)")}
	}
	labels := make([]string, len(m.Items))
	for i, item := range m.Items {
		labels[i] = colorStyle(item.Color).Render(item.Label)
	}
	return labels
}

func labelsFromIndexes(m idxListModel) []string {
	if m.Filtering() && len(m.indexes) == 0 {
		return []string{helpHintStyle.Render("(sin coincidencias)")}
	}
	labels := make([]string, len(m.indexes))
	for i, idx := range m.indexes {
		unique := ""
		if idx.Unique {
			unique = " (unique)"
		}
		labels[i] = fmt.Sprintf("%s %v%s", idx.Name, idx.Key, unique)
	}
	return labels
}

// lastLogLines returns at most n of the most recent log entries.
func lastLogLines(log []string, n int) []string {
	if len(log) <= n {
		return log
	}
	return log[len(log)-n:]
}

// Run starts the Bubbletea program.
func Run(client mongo.Client, resolved *config.Connection) error {
	model := NewRootModel(client, resolved)
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
