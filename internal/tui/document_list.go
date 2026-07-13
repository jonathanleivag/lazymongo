package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type documentChosenMsg struct{ Doc bson.M }
type pageChangedMsg struct{ Page int64 }
type filterSubmittedMsg struct{ Filter string }
type insertRequestedMsg struct{}
type switchToIndexesMsg struct{}

type docListModel struct {
	docs      []bson.M
	total     int64
	page      int64
	pageSize  int64
	cursor    int
	filtering bool
	filter    string

	fuzzyFiltering bool
	fuzzyQuery     string
	allDocs        []bson.M
}

func newDocListModel(docs []bson.M, total, page, pageSize int64) docListModel {
	return docListModel{docs: docs, total: total, page: page, pageSize: pageSize}
}

func (m docListModel) FilterText() string { return m.filter }

// FuzzyFiltering reports whether the local (non-Mongo) fuzzy-find over
// already-loaded rows is active, so RootModel.inTextEntry can keep global
// shortcuts (like "?", "1"-"5", "Tab") from stealing keystrokes meant for
// the search query.
func (m docListModel) FuzzyFiltering() bool { return m.fuzzyFiltering }

// FuzzyQuery returns the text typed so far into the active local fuzzy-find.
func (m docListModel) FuzzyQuery() string { return m.fuzzyQuery }

// FilterSuggestion returns the missing suffix of the best-matching
// top-level field name for whatever partial key is currently being typed
// into the Mongo filter, or "" if none applies. See filterFieldSuggestion.
func (m docListModel) FilterSuggestion() string {
	return filterFieldSuggestion(m.filter, m.docs)
}

func (m docListModel) Update(msg tea.Msg) (docListModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.filtering {
		switch keyMsg.Type {
		case tea.KeyEnter:
			m.filtering = false
			filter := m.filter
			return m, func() tea.Msg { return filterSubmittedMsg{Filter: filter} }
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
		case tea.KeyTab:
			m.filter += filterFieldSuggestion(m.filter, m.docs)
		case tea.KeyBackspace:
			if r := []rune(m.filter); len(r) > 0 {
				m.filter = string(r[:len(r)-1])
			}
		case tea.KeyRunes:
			m.filter += string(keyMsg.Runes)
		}
		return m, nil
	}

	if m.fuzzyFiltering {
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.fuzzyFiltering = false
			m.fuzzyQuery = ""
			m.docs = m.allDocs
			m.cursor = 0
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.docs)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			if len(m.docs) > 0 {
				doc := m.docs[m.cursor]
				m.exitFuzzyFiltering(doc["_id"])
				return m, func() tea.Msg { return documentChosenMsg{Doc: doc} }
			}
		case tea.KeyBackspace:
			if r := []rune(m.fuzzyQuery); len(r) > 0 {
				m.fuzzyQuery = string(r[:len(r)-1])
			}
			m.applyFuzzyFilter()
		case tea.KeyRunes:
			m.fuzzyQuery += string(keyMsg.Runes)
			m.applyFuzzyFilter()
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.docs)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.docs) > 0 {
			doc := m.docs[m.cursor]
			return m, func() tea.Msg { return documentChosenMsg{Doc: doc} }
		}
	case "/":
		m.filtering = true
		m.filter = ""
	case "ctrl+f":
		m.fuzzyFiltering = true
		m.fuzzyQuery = ""
		m.allDocs = m.docs
	case "n":
		if (m.page+1)*m.pageSize < m.total {
			page := m.page + 1
			return m, func() tea.Msg { return pageChangedMsg{Page: page} }
		}
	case "p":
		if m.page > 0 {
			page := m.page - 1
			return m, func() tea.Msg { return pageChangedMsg{Page: page} }
		}
	case "i", "a":
		return m, func() tea.Msg { return insertRequestedMsg{} }
	case "tab":
		return m, func() tea.Msg { return switchToIndexesMsg{} }
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

// exitFuzzyFiltering leaves local fuzzy-find mode and restores the full
// loaded row set, with cursor moved to selectedID's position in that full
// set. Used on Enter: without this, fuzzy-find stayed active after opening
// a document, so the next keystroke (e.g. a panel-jump digit, once the
// detail popup closes) kept being swallowed as literal query text instead
// of reaching RootModel's global shortcut handling.
func (m *docListModel) exitFuzzyFiltering(selectedID any) {
	m.fuzzyFiltering = false
	m.fuzzyQuery = ""
	m.docs = m.allDocs
	m.cursor = 0
	for i, doc := range m.docs {
		if doc["_id"] == selectedID {
			m.cursor = i
			break
		}
	}
}

// applyFuzzyFilter recomputes docs from allDocs using the current
// fuzzyQuery, matched against each document's rendered _id (the same text
// shown for every collapsed, non-highlighted row — not nested field
// content, per spec), ordered by fuzzy match quality, and resets cursor to
// the top result.
func (m *docListModel) applyFuzzyFilter() {
	labels := make([]string, len(m.allDocs))
	for i, doc := range m.allDocs {
		labels[i] = fmt.Sprintf("%v", doc["_id"])
	}
	idxs := fuzzyMatchIndexes(m.fuzzyQuery, labels)
	docs := make([]bson.M, len(idxs))
	for i, idx := range idxs {
		docs[i] = m.allDocs[idx]
	}
	m.docs = docs
	m.cursor = 0
}

func (m docListModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Documentos (%d total, página %d)", m.total, m.page+1)) + "\n\n")

	if m.filtering {
		b.WriteString("Filtro: " + m.filter + "_\n\n")
	} else if m.filter != "" {
		b.WriteString(helpHintStyle.Render("Filtro activo: "+m.filter) + "\n\n")
	}

	if len(m.docs) == 0 {
		b.WriteString("(sin documentos)\n")
	}
	for i, doc := range m.docs {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + fmt.Sprintf("%v\n", doc["_id"]))
	}
	b.WriteString("\n" + helpHintStyle.Render("[/] filtrar  [n/p] página  [Enter] ver  [i] insertar  [Tab] índices"))
	return b.String()
}

// filterKeyPositionPattern matches when the filter text ends in an open
// JSON key position: a "{" or "," (a value's closing quote and any
// whitespace already consumed), followed by an opening quote and the
// partial key text typed so far, with no further quotes in between (an
// interior quote would mean we're actually past the key, e.g. typing a
// value or already at a colon).
var filterKeyPositionPattern = regexp.MustCompile(`[{,]\s*"([^"]*)$`)

// filterFieldSuggestion returns the missing suffix of the best-matching
// top-level field name for the partial key currently being typed in
// filter, or "" if filter isn't in a JSON key position (see
// filterKeyPositionPattern) or no known field name starts with the partial
// text typed so far. Field names are the union of top-level keys across
// docs — nested fields inside a bson.M/bson.A value are never considered,
// matching the same top-level-only boundary used by docPanelLines'
// "Array (N)"/"Object" placeholders (see document_render.go). Ties between
// multiple matching field names resolve to the alphabetically-first one.
func filterFieldSuggestion(filter string, docs []bson.M) string {
	match := filterKeyPositionPattern.FindStringSubmatch(filter)
	if match == nil {
		return ""
	}
	partial := match[1]

	fieldSet := map[string]bool{}
	for _, doc := range docs {
		for field := range doc {
			fieldSet[field] = true
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		if strings.HasPrefix(field, partial) {
			return field[len(partial):]
		}
	}
	return ""
}
