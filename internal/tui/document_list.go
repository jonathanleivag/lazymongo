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
type filterClearedMsg struct{}
type sortSubmittedMsg struct{ Sort string }
type sortClearedMsg struct{}
type insertRequestedMsg struct{}
type switchToIndexesMsg struct{}

type docListModel struct {
	docs         []bson.M
	total        int64
	page         int64
	pageSize     int64
	cursor       int
	filtering    bool
	filter       string
	filterCursor int

	sorting    bool
	sort       string
	sortCursor int

	fuzzyFiltering bool
	fuzzyQuery     string
	allDocs        []bson.M
}

func newDocListModel(docs []bson.M, total, page, pageSize int64) docListModel {
	return docListModel{docs: docs, total: total, page: page, pageSize: pageSize}
}

func (m docListModel) FilterText() string { return m.filter }

// FilterCursor returns the current rune-index cursor position within the
// Mongo filter text.
func (m docListModel) FilterCursor() int { return m.filterCursor }

func (m docListModel) SortText() string { return m.sort }

// SortCursor returns the current rune-index cursor position within the
// Mongo sort text.
func (m docListModel) SortCursor() int { return m.sortCursor }

// FuzzyFiltering reports whether the local (non-Mongo) fuzzy-find over
// already-loaded rows is active, so RootModel.inTextEntry can keep global
// shortcuts (like "?", "1"-"5", "Tab") from stealing keystrokes meant for
// the search query.
func (m docListModel) FuzzyFiltering() bool { return m.fuzzyFiltering }

// FuzzyQuery returns the text typed so far into the active local fuzzy-find.
func (m docListModel) FuzzyQuery() string { return m.fuzzyQuery }

// textBeforeCursor returns the filter text up to (not including) the
// character at filterCursor.
func (m docListModel) textBeforeCursor() string {
	text := []rune(m.filter)
	if m.filterCursor > len(text) {
		return m.filter
	}
	return string(text[:m.filterCursor])
}

// textAfterCursor returns the filter text from filterCursor onward.
func (m docListModel) textAfterCursor() string {
	text := []rune(m.filter)
	if m.filterCursor > len(text) {
		return ""
	}
	return string(text[m.filterCursor:])
}

// FilterSuggestion returns the missing suffix of the best-matching
// top-level field name for whatever partial key is currently being typed
// into the Mongo filter, considering only the text before the cursor (text
// after it — e.g. an auto-closed "}" — is irrelevant to what key is being
// typed), or "" if none applies. See filterFieldSuggestion.
func (m docListModel) FilterSuggestion() string {
	return filterFieldSuggestion(m.textBeforeCursor(), m.docs)
}

// SortSuggestion returns the missing suffix of the best-matching
// top-level field name for whatever partial key is currently being typed
// into the Mongo sort.
func (m docListModel) SortSuggestion() string {
	return filterFieldSuggestion(m.sortTextBeforeCursor(), m.docs)
}

// FilterBeforeCursor and FilterAfterCursor split the filter text at the
// cursor for rendering: RootModel draws the cursor-blink marker at the real
// cursor position instead of always at the end of the string.
func (m docListModel) FilterBeforeCursor() string { return m.textBeforeCursor() }
func (m docListModel) FilterAfterCursor() string  { return m.textAfterCursor() }

func (m docListModel) sortTextBeforeCursor() string {
	text := []rune(m.sort)
	if m.sortCursor > len(text) {
		return m.sort
	}
	return string(text[:m.sortCursor])
}

func (m docListModel) sortTextAfterCursor() string {
	text := []rune(m.sort)
	if m.sortCursor > len(text) {
		return ""
	}
	return string(text[m.sortCursor:])
}

func (m docListModel) SortBeforeCursor() string { return m.sortTextBeforeCursor() }
func (m docListModel) SortAfterCursor() string  { return m.sortTextAfterCursor() }

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
			m.filterCursor = 0
			return m, func() tea.Msg { return filterSubmittedMsg{Filter: filter} }
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
			m.filterCursor = 0
		case tea.KeyTab:
			suggestion := []rune(filterFieldSuggestion(m.textBeforeCursor(), m.docs))
			if len(suggestion) > 0 {
				text := []rune(m.filter)
				newText := make([]rune, 0, len(text)+len(suggestion))
				newText = append(newText, text[:m.filterCursor]...)
				newText = append(newText, suggestion...)
				newText = append(newText, text[m.filterCursor:]...)
				m.filter = string(newText)
				m.filterCursor += len(suggestion)
			}
		case tea.KeyLeft:
			if m.filterCursor > 0 {
				m.filterCursor--
			}
		case tea.KeyRight:
			if m.filterCursor < len([]rune(m.filter)) {
				m.filterCursor++
			}
		case tea.KeyBackspace:
			m.backspaceFilter()
		case tea.KeyRunes:
			m.insertFilterRunes(keyMsg.Runes)
		}
		return m, nil
	}

	if m.sorting {
		switch keyMsg.Type {
		case tea.KeyEnter:
			m.sorting = false
			sortQuery := m.sort
			m.sortCursor = 0
			return m, func() tea.Msg { return sortSubmittedMsg{Sort: sortQuery} }
		case tea.KeyEsc:
			m.sorting = false
			m.sort = ""
			m.sortCursor = 0
		case tea.KeyTab:
			suggestion := []rune(filterFieldSuggestion(m.sortTextBeforeCursor(), m.docs))
			if len(suggestion) > 0 {
				text := []rune(m.sort)
				newText := make([]rune, 0, len(text)+len(suggestion))
				newText = append(newText, text[:m.sortCursor]...)
				newText = append(newText, suggestion...)
				newText = append(newText, text[m.sortCursor:]...)
				m.sort = string(newText)
				m.sortCursor += len(suggestion)
			}
		case tea.KeyLeft:
			if m.sortCursor > 0 {
				m.sortCursor--
			}
		case tea.KeyRight:
			if m.sortCursor < len([]rune(m.sort)) {
				m.sortCursor++
			}
		case tea.KeyBackspace:
			m.backspaceSort()
		case tea.KeyRunes:
			m.insertSortRunes(keyMsg.Runes)
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
		m.filterCursor = 0
	case "s":
		m.sorting = true
		m.sort = ""
		m.sortCursor = 0
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
	case "d", "x":
		if len(m.docs) > 0 {
			return m, func() tea.Msg { return deleteRequestedMsg{} }
		}
	case "i", "a":
		return m, func() tea.Msg { return insertRequestedMsg{} }
	case "tab":
		return m, func() tea.Msg { return switchToIndexesMsg{} }
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.filterCursor = 0
			return m, func() tea.Msg { return filterClearedMsg{} }
		}
		if m.sort != "" {
			m.sort = ""
			m.sortCursor = 0
			return m, func() tea.Msg { return sortClearedMsg{} }
		}
		return m, func() tea.Msg { return listBackMsg{} }
	case "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

// insertFilterRunes inserts each typed rune at filterCursor in order,
// applying auto-close and skip-over for "{" and "\"" (see
// insertFilterRune).
func (m *docListModel) insertFilterRunes(runes []rune) {
	for _, r := range runes {
		m.insertFilterRune(r)
	}
}

// insertFilterRune inserts one rune into filter at filterCursor. Typing "{"
// or "\"" auto-inserts the matching closer immediately after it, leaving
// the cursor between the two. Typing a closer ("}" or "\"") that is already
// sitting immediately to the right of the cursor moves the cursor past it
// instead of inserting a duplicate ("skip-over") — this is purely
// positional (it doesn't parse the JSON or track which characters were
// auto-inserted), which is what makes it work identically whether you're
// closing an auto-inserted pair or finishing a value string by hand.
func (m *docListModel) insertFilterRune(r rune) {
	text := []rune(m.filter)

	if (r == '}' || r == '"') && m.filterCursor < len(text) && text[m.filterCursor] == r {
		m.filterCursor++
		return
	}

	var closer rune
	switch r {
	case '{':
		closer = '}'
	case '"':
		closer = '"'
	}

	newText := make([]rune, 0, len(text)+2)
	newText = append(newText, text[:m.filterCursor]...)
	newText = append(newText, r)
	if closer != 0 {
		newText = append(newText, closer)
	}
	newText = append(newText, text[m.filterCursor:]...)

	m.filter = string(newText)
	m.filterCursor++
}

// backspaceFilter removes the rune immediately before filterCursor. If that
// rune is "{" or "\"" and the rune immediately after the cursor is its
// matching closer with nothing typed in between (an empty auto-closed
// pair), both characters are removed at once instead of leaving an orphan
// closer behind.
func (m *docListModel) backspaceFilter() {
	if m.filterCursor == 0 {
		return
	}
	text := []rune(m.filter)
	before := text[m.filterCursor-1]

	emptyPair := false
	if m.filterCursor < len(text) {
		after := text[m.filterCursor]
		emptyPair = (before == '{' && after == '}') || (before == '"' && after == '"')
	}

	if emptyPair {
		newText := make([]rune, 0, len(text)-2)
		newText = append(newText, text[:m.filterCursor-1]...)
		newText = append(newText, text[m.filterCursor+1:]...)
		m.filter = string(newText)
		m.filterCursor--
		return
	}

	newText := make([]rune, 0, len(text)-1)
	newText = append(newText, text[:m.filterCursor-1]...)
	newText = append(newText, text[m.filterCursor:]...)
	m.filter = string(newText)
	m.filterCursor--
}

func (m *docListModel) insertSortRunes(runes []rune) {
	for _, r := range runes {
		m.insertSortRune(r)
	}
}

func (m *docListModel) insertSortRune(r rune) {
	text := []rune(m.sort)

	if (r == '}' || r == '"') && m.sortCursor < len(text) && text[m.sortCursor] == r {
		m.sortCursor++
		return
	}

	var closer rune
	switch r {
	case '{':
		closer = '}'
	case '"':
		closer = '"'
	}

	newText := make([]rune, 0, len(text)+2)
	newText = append(newText, text[:m.sortCursor]...)
	newText = append(newText, r)
	if closer != 0 {
		newText = append(newText, closer)
	}
	newText = append(newText, text[m.sortCursor:]...)

	m.sort = string(newText)
	m.sortCursor++
}

func (m *docListModel) backspaceSort() {
	if m.sortCursor == 0 {
		return
	}
	text := []rune(m.sort)
	before := text[m.sortCursor-1]

	emptyPair := false
	if m.sortCursor < len(text) {
		after := text[m.sortCursor]
		emptyPair = (before == '{' && after == '}') || (before == '"' && after == '"')
	}

	if emptyPair {
		newText := make([]rune, 0, len(text)-2)
		newText = append(newText, text[:m.sortCursor-1]...)
		newText = append(newText, text[m.sortCursor+1:]...)
		m.sort = string(newText)
		m.sortCursor--
		return
	}

	newText := make([]rune, 0, len(text)-1)
	newText = append(newText, text[:m.sortCursor-1]...)
	newText = append(newText, text[m.sortCursor:]...)
	m.sort = string(newText)
	m.sortCursor--
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

	if m.sorting {
		b.WriteString("Orden:  " + m.sort + "_\n\n")
	} else if m.sort != "" {
		b.WriteString(helpHintStyle.Render("Orden activo:  "+m.sort) + "\n\n")
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
	b.WriteString("\n" + helpHintStyle.Render("[/] filtrar  [s] ordenar  [n/p] página  [Enter] ver  [i] insertar  [Tab] índices"))
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
