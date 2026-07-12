package tui

import (
	"context"
	"fmt"

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

	connPicker connectionPickerModel
	dbList     listModel
	collList   listModel
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
// that already went through the popupConfirmWrite confirmation step.
type docWriteCompletedMsg struct{ Err error }
type indexWriteCompletedMsg struct{ Err error }

// inTextEntry reports whether the current focus/popup is in an active
// text-entry sub-state, where printable keys like "?" must be typed
// literally rather than treated as a global shortcut.
func (m RootModel) inTextEntry() bool {
	switch {
	case m.focus == panelConnections && m.connPicker.creating:
		return true
	case m.focus == panelDocuments && m.docList.filtering:
		return true
	case m.popup == popupFieldEdit && !m.fieldEdit.confirming:
		return true
	case m.focus == panelIndexes && m.idxList.creating:
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
		if m.popup == popupNone {
			switch keyMsg.String() {
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
			case "tab":
				m.focus = panelDocuments
				return m, nil
			}
		}
	}

	switch msg := msg.(type) {
	case connectedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.dbList = newDatabaseListModel(msg.Databases)
		m.logf("Conectado a %s", m.conn.Name)
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

	case collectionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.collList = newCollectionListModel(msg.Collections)
		return m, nil

	case documentsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.docList = newDocListModel(msg.Docs, msg.Total, m.page, pageSize)
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
		return m, m.loadDocuments(m.currentFilter())

	case indexWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadIndexes()

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
		before := m.dbList.Cursor
		var listCmd tea.Cmd
		m.dbList, listCmd = m.dbList.Update(msg)
		if m.dbList.Cursor != before && len(m.dbList.Items) > 0 {
			m.db = m.dbList.Items[m.dbList.Cursor].ID
			return m, m.loadCollections()
		}
		return m, listCmd

	case panelCollections:
		before := m.collList.Cursor
		var listCmd tea.Cmd
		m.collList, listCmd = m.collList.Update(msg)
		if m.collList.Cursor != before && len(m.collList.Items) > 0 {
			m.coll = m.collList.Items[m.collList.Cursor].ID
			m.page = 0
			m.filter = nil
			return m, tea.Batch(m.loadIndexes(), m.loadDocuments(bson.M{}))
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
				return m, m.loadDocuments(m.currentFilter())
			case filterSubmittedMsg:
				m.page = 0
				var filter bson.M
				if err := bson.UnmarshalExtJSON([]byte(out.Filter), false, &filter); err != nil {
					m.err = fmt.Errorf("filtro inválido: %w", err)
					return m, nil
				}
				m.filter = filter
				m.logf("Filtro aplicado: %s", out.Filter)
				return m, m.loadDocuments(filter)
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
		return renderPopupOverlay(fmt.Sprintf("Error: %v\n\n[cualquier tecla] continuar", m.err), m.width, m.height)
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
		return renderPopupOverlay(helpModel{}.View(), m.width, m.height)
	}

	if m.focus == panelConnections && m.connPicker.creating {
		return renderPopupOverlay(m.connPicker.View(), m.width, m.height)
	}
	if m.focus == panelIndexes && (m.idxList.creating || m.idxList.confirmingDrop) {
		return renderPopupOverlay(m.idxList.View(), m.width, m.height)
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
	p1 := renderPanel(1, "Status", statusLines, 0, m.focus == panelStatus, sidebarWidth, panelHeight)
	p2 := renderPanel(2, "Databases", labelsFromListModel(m.dbList), m.dbList.Cursor, m.focus == panelDatabases, sidebarWidth, panelHeight)
	p3 := renderPanel(3, "Collections", labelsFromListModel(m.collList), m.collList.Cursor, m.focus == panelCollections, sidebarWidth, panelHeight)
	p4 := renderPanel(4, "Indexes", labelsFromIndexes(m.idxList.indexes), m.idxList.cursor, m.focus == panelIndexes, sidebarWidth, panelHeight)
	p5 := renderPanel(5, "Conexiones", labelsFromListModel(m.connPicker.list), m.connPicker.list.Cursor, m.focus == panelConnections, sidebarWidth, panelHeight)

	docTitle := fmt.Sprintf("Documentos (%d total, pág %d)", m.docList.total, m.docList.page+1)
	docLines := labelsFromDocs(m.docList.docs)
	if m.docList.filtering {
		docLines = append([]string{"Filtro: " + m.docList.filter + "_"}, docLines...)
	} else if m.docList.filter != "" {
		docLines = append([]string{"Filtro activo: " + m.docList.filter}, docLines...)
	}
	mainHeight := panelHeight*5 - 5
	main := renderPanel(0, docTitle, docLines, m.docList.cursor, m.focus == panelDocuments, mainWidth, mainHeight)

	footer := "[1-5] panel  [j/k] mover  [Tab] documentos  [Enter] ver  [e] editar  [d] borrar  [/] filtro  [?] ayuda  [Ctrl+c] salir"

	return composeScreen([]string{p1, p2, p3, p4, p5}, main, lastLogLines(m.log, 4), footer, mainWidth, 4)
}

// labelsFromListModel renders each item's colored label as a plain line for
// panel display (no cursor/border chrome — renderPanel adds that).
func labelsFromListModel(m listModel) []string {
	labels := make([]string, len(m.Items))
	for i, item := range m.Items {
		labels[i] = colorStyle(item.Color).Render(item.Label)
	}
	return labels
}

func labelsFromIndexes(indexes []mongo.IndexInfo) []string {
	labels := make([]string, len(indexes))
	for i, idx := range indexes {
		unique := ""
		if idx.Unique {
			unique = " (unique)"
		}
		labels[i] = fmt.Sprintf("%s %v%s", idx.Name, idx.Key, unique)
	}
	return labels
}

func labelsFromDocs(docs []bson.M) []string {
	labels := make([]string, len(docs))
	for i, doc := range docs {
		labels[i] = fmt.Sprintf("%v", doc["_id"])
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
