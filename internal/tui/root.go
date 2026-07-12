package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type viewID int

const (
	viewConnectionPicker viewID = iota
	viewDatabaseList
	viewCollectionList
	viewDocumentList
	viewDocumentDetail
	viewFieldEdit
	viewIndexList
	viewDelete
	viewConfirmWrite
	viewHelp
)

const pageSize = 20

type RootModel struct {
	client mongo.Client

	view      viewID
	prevViews []viewID

	conn   config.Connection
	db     string
	coll   string
	page   int64
	filter bson.M

	connPicker connectionPickerModel
	dbList     listModel
	collList   listModel
	docList    docListModel
	docDetail  docDetailModel
	fieldEdit  fieldEditModel
	idxList    idxListModel
	delete     deleteFlowModel

	// pending write awaiting confirmation via viewConfirmWrite
	confirmWrite         confirmModel
	editMode             string // "edit" | "insert", set before startEditFullFlow
	pendingDoc           bson.M
	pendingIndexKeysJSON string
	pendingIndexUnique   bool

	err error
}

// NewRootModel builds the root model. If resolved is nil, the app starts on
// the connection picker, populated with the user's saved connections;
// otherwise it connects immediately.
func NewRootModel(client mongo.Client, resolved *config.Connection) RootModel {
	m := RootModel{client: client}
	if resolved != nil {
		m.conn = *resolved
		m.view = viewDatabaseList
	} else {
		m.view = viewConnectionPicker
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

func (m RootModel) pushView(v viewID) RootModel {
	m.prevViews = append(m.prevViews, m.view)
	m.view = v
	return m
}

func (m RootModel) popView() RootModel {
	if len(m.prevViews) == 0 {
		return m
	}
	m.view = m.prevViews[len(m.prevViews)-1]
	m.prevViews = m.prevViews[:len(m.prevViews)-1]
	return m
}

func (m RootModel) Init() tea.Cmd {
	if m.view != viewDatabaseList {
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

type collectionsLoadedMsg struct {
	Collections []string
	Err         error
}

func (m RootModel) loadCollections() tea.Cmd {
	client, db := m.client, m.db
	return func() tea.Msg {
		names, err := client.ListCollections(context.Background(), db)
		return collectionsLoadedMsg{Collections: names, Err: err}
	}
}

type documentsLoadedMsg struct {
	Docs  []bson.M
	Total int64
	Err   error
}

func (m RootModel) loadDocuments(filter bson.M) tea.Cmd {
	client, db, coll, page := m.client, m.db, m.coll, m.page
	return func() tea.Msg {
		ctx := context.Background()
		docs, err := client.Find(ctx, db, coll, filter, page*pageSize, pageSize)
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
// that already went through the viewConfirmWrite confirmation step.
type docWriteCompletedMsg struct{ Err error }
type indexWriteCompletedMsg struct{ Err error }

// inTextEntry reports whether the current view is in an active text-entry
// sub-state, where printable keys like "q" or "?" must be typed literally
// rather than treated as global shortcuts.
func (m RootModel) inTextEntry() bool {
	switch {
	case m.view == viewConnectionPicker && m.connPicker.creating:
		return true
	case m.view == viewDocumentList && m.docList.filtering:
		return true
	case m.view == viewFieldEdit && !m.fieldEdit.confirming:
		return true
	case m.view == viewIndexList && m.idxList.creating:
		return true
	}
	return false
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.err != nil {
			m.err = nil
			return m, nil
		}
		switch keyMsg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		if keyMsg.String() == "?" && !m.inTextEntry() {
			if m.view != viewHelp {
				m = m.pushView(viewHelp)
				return m, nil
			}
		}
		if m.view == viewHelp {
			m = m.popView()
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case connectedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.dbList = newDatabaseListModel(msg.Databases)
		m.view = viewDatabaseList
		return m, nil

	case connectionChosenMsg:
		conn, err := config.ResolveConnection(msg.Conn.Name)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.conn = conn
		return m, m.connectAndListDatabases()

	case connectionCreatedMsg:
		// re-render the picker with the latest connections list
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

	case collectionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.collList = newCollectionListModel(msg.Collections)
		m.view = viewCollectionList
		return m, nil

	case documentsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.docList = newDocListModel(msg.Docs, msg.Total, m.page, pageSize)
		m.view = viewDocumentList
		return m, nil

	case indexesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.idxList = newIdxListModel(msg.Indexes)
		m.view = viewIndexList
		return m, nil

	case fieldSelectedMsg:
		m.fieldEdit = newFieldEditModel(msg.Field, msg.Value)
		m = m.pushView(viewFieldEdit)
		return m, nil

	case deleteRequestedMsg:
		id := m.docDetail.doc["_id"]
		m.delete = newDeleteFlowModel(id)
		m = m.pushView(viewDelete)
		return m, nil

	case deleteConfirmedMsg:
		id := m.docDetail.doc["_id"]
		client, db, coll := m.client, m.db, m.coll
		m = m.popView()
		return m, func() tea.Msg {
			err := client.DeleteOne(context.Background(), db, coll, id)
			return docWriteCompletedMsg{Err: err}
		}

	case fieldUpdateConfirmedMsg:
		client, db, coll, id := m.client, m.db, m.coll, m.docDetail.doc["_id"]
		field, value := msg.Field, msg.NewValue
		m = m.popView()
		return m, func() tea.Msg {
			err := client.UpdateField(context.Background(), db, coll, id, field, value)
			return docWriteCompletedMsg{Err: err}
		}

	case insertRequestedMsg:
		m.editMode = "insert"
		return m, startEditFullFlow(bson.M{})

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
		m = m.pushView(viewConfirmWrite)
		return m, nil

	case switchToIndexesMsg:
		return m, m.loadIndexes()

	case indexCreateSubmittedMsg:
		m.pendingIndexKeysJSON = msg.KeysJSON
		m.pendingIndexUnique = msg.Unique
		m.confirmWrite = confirmModel{Message: "¿Crear este índice?"}
		m = m.pushView(viewConfirmWrite)
		return m, nil

	case indexDropConfirmedMsg:
		client, db, coll, name := m.client, m.db, m.coll, msg.Name
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
		return m, m.loadDocuments(m.currentFilter())

	case indexWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadIndexes()

	case listBackMsg:
		switch m.view {
		case viewDatabaseList:
			m.view = viewConnectionPicker
		case viewCollectionList:
			m.view = viewDatabaseList
		case viewDocumentList:
			m.view = viewCollectionList
		case viewIndexList:
			m.view = viewDocumentList
		default:
			m = m.popView()
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.view {
	case viewConnectionPicker:
		m.connPicker, cmd = m.connPicker.Update(msg)
	case viewDatabaseList:
		var listCmd tea.Cmd
		m.dbList, listCmd = m.dbList.Update(msg)
		if listCmd != nil {
			if translated, ok := translateDatabaseSelection(listCmd()); ok {
				if chosen, ok := translated.(databaseChosenMsg); ok {
					m.db = chosen.Name
					return m, m.loadCollections()
				}
			}
			return m, listCmd
		}
	case viewCollectionList:
		var listCmd tea.Cmd
		m.collList, listCmd = m.collList.Update(msg)
		if listCmd != nil {
			if translated, ok := translateCollectionSelection(listCmd()); ok {
				if chosen, ok := translated.(collectionChosenMsg); ok {
					m.coll = chosen.Name
					m.page = 0
					m.filter = nil
					return m, m.loadDocuments(bson.M{})
				}
			}
			return m, listCmd
		}
	case viewDocumentList:
		m.docList, cmd = m.docList.Update(msg)
		if cmd != nil {
			switch out := cmd().(type) {
			case pageChangedMsg:
				m.page = out.Page
				return m, m.loadDocuments(m.currentFilter())
			case filterSubmittedMsg:
				m.page = 0
				var filter bson.M
				if err := bson.UnmarshalExtJSON([]byte(out.Filter), false, &filter); err != nil {
					m.err = fmt.Errorf("filtro inválido: %w", err)
					return m, nil
				}
				m.filter = filter
				return m, m.loadDocuments(filter)
			case documentChosenMsg:
				m.docDetail = newDocDetailModel(out.Doc)
				m = m.pushView(viewDocumentDetail)
				return m, nil
			}
			return m, cmd
		}
	case viewDocumentDetail:
		var docCmd tea.Cmd
		m.docDetail, docCmd = m.docDetail.Update(msg)
		if docCmd != nil {
			return m.dispatch(docCmd())
		}
	case viewFieldEdit:
		var feCmd tea.Cmd
		m.fieldEdit, feCmd = m.fieldEdit.Update(msg)
		if feCmd != nil {
			return m.dispatch(feCmd())
		}
	case viewIndexList:
		var idxCmd tea.Cmd
		m.idxList, idxCmd = m.idxList.Update(msg)
		if idxCmd != nil {
			return m.dispatch(idxCmd())
		}
	case viewDelete:
		var delCmd tea.Cmd
		m.delete, delCmd = m.delete.Update(msg)
		if delCmd != nil {
			return m.dispatch(delCmd())
		}
	case viewConfirmWrite:
		var cwCmd tea.Cmd
		m.confirmWrite, cwCmd = m.confirmWrite.Update(msg)
		if cwCmd != nil {
			result, ok := cwCmd().(confirmResultMsg)
			if !ok {
				return m, cwCmd
			}
			m = m.popView()
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

// executePendingWrite performs the write that was just confirmed via
// viewConfirmWrite: either a document insert/replace (set up by
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

// statusBar renders the current connection's name in its assigned color, so
// it's always visible which environment (e.g. qa vs. prod) is being edited.
func (m RootModel) statusBar() string {
	if m.conn.Name == "" {
		return ""
	}
	label := fmt.Sprintf(" %s ", m.conn.Name)
	return colorStyle(m.conn.Color).Reverse(true).Render(label) + "\n\n"
}

func (m RootModel) View() string {
	if m.err != nil {
		return m.statusBar() + fmt.Sprintf("Error: %v\n\n[cualquier tecla] continuar", m.err)
	}

	var content string
	switch m.view {
	case viewConnectionPicker:
		return m.connPicker.View() // no connection chosen yet, no status bar
	case viewDatabaseList:
		content = m.dbList.View()
	case viewCollectionList:
		content = m.collList.View()
	case viewDocumentList:
		content = m.docList.View()
	case viewDocumentDetail:
		content = m.docDetail.View()
	case viewFieldEdit:
		content = m.fieldEdit.View()
	case viewIndexList:
		content = m.idxList.View()
	case viewDelete:
		content = m.delete.View()
	case viewConfirmWrite:
		content = m.confirmWrite.View()
	case viewHelp:
		content = helpModel{}.View()
	}
	return m.statusBar() + content
}

// Run starts the Bubbletea program.
func Run(client mongo.Client, resolved *config.Connection) error {
	model := NewRootModel(client, resolved)
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
