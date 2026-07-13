package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type listItem struct {
	ID    string
	Label string
	Color string
}

type itemSelectedMsg struct{ Item listItem }
type listBackMsg struct{}

type listModel struct {
	Title     string
	Items     []listItem
	Cursor    int
	CanCreate bool

	filtering   bool
	filterQuery string
	allItems    []listItem
}

func newListModel(title string, items []listItem, canCreate bool) listModel {
	return listModel{Title: title, Items: items, CanCreate: canCreate}
}

// Filtering reports whether this list is in an active fuzzy-search
// text-entry state, so RootModel.inTextEntry can keep global shortcuts
// (like "?", "1"-"5", "Tab") from stealing keystrokes meant for the query.
func (m listModel) Filtering() bool { return m.filtering }

// FilterQuery returns the text typed so far into the active fuzzy search.
func (m listModel) FilterQuery() string { return m.filterQuery }

func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.filtering {
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.filtering = false
			m.filterQuery = ""
			m.Items = m.allItems
			m.Cursor = 0
		case tea.KeyUp:
			if m.Cursor > 0 {
				m.Cursor--
			}
		case tea.KeyDown:
			if m.Cursor < len(m.Items)-1 {
				m.Cursor++
			}
		case tea.KeyEnter:
			if len(m.Items) > 0 {
				item := m.Items[m.Cursor]
				m.exitFiltering(item.ID)
				return m, func() tea.Msg { return itemSelectedMsg{Item: item} }
			}
		case tea.KeyBackspace:
			if r := []rune(m.filterQuery); len(r) > 0 {
				m.filterQuery = string(r[:len(r)-1])
			}
			m.applyFilter()
		case tea.KeyRunes:
			m.filterQuery += string(keyMsg.Runes)
			m.applyFilter()
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "j", "down":
		if m.Cursor < len(m.Items)-1 {
			m.Cursor++
		}
	case "k", "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "enter":
		if len(m.Items) > 0 {
			item := m.Items[m.Cursor]
			return m, func() tea.Msg { return itemSelectedMsg{Item: item} }
		}
	case "/":
		m.filtering = true
		m.filterQuery = ""
		m.allItems = m.Items
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

// exitFiltering leaves filtering mode and restores the full item set, with
// Cursor moved to selectedID's position in that full set. Used on Enter:
// without this, filtering stayed active after a selection, so the next
// keystroke (e.g. a panel-jump digit) kept being swallowed as literal query
// text instead of reaching RootModel's global shortcut handling.
func (m *listModel) exitFiltering(selectedID string) {
	m.filtering = false
	m.filterQuery = ""
	m.Items = m.allItems
	m.Cursor = 0
	for i, item := range m.Items {
		if item.ID == selectedID {
			m.Cursor = i
			break
		}
	}
}

// applyFilter recomputes Items from allItems using the current filterQuery,
// ordered by fuzzy match quality (best first), and resets Cursor to the top
// result.
func (m *listModel) applyFilter() {
	labels := make([]string, len(m.allItems))
	for i, item := range m.allItems {
		labels[i] = item.Label
	}
	idxs := fuzzyMatchIndexes(m.filterQuery, labels)
	items := make([]listItem, len(idxs))
	for i, idx := range idxs {
		items[i] = m.allItems[idx]
	}
	m.Items = items
	m.Cursor = 0
}

func (m listModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.Title) + "\n\n")
	if len(m.Items) == 0 {
		b.WriteString("(vacío)\n")
	}
	for i, item := range m.Items {
		prefix := "  "
		if i == m.Cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + colorStyle(item.Color).Render(item.Label) + "\n")
	}
	if m.CanCreate {
		b.WriteString("\n" + helpHintStyle.Render("[a] nueva conexión"))
	}
	return b.String()
}
