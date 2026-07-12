package tui

import tea "github.com/charmbracelet/bubbletea"

type databaseChosenMsg struct{ Name string }
type collectionChosenMsg struct{ Name string }

func newDatabaseListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Bases de datos", items, false)
}

func newCollectionListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Colecciones", items, false)
}

func translateDatabaseSelection(msg tea.Msg) (tea.Msg, bool) {
	selected, ok := msg.(itemSelectedMsg)
	if !ok {
		return nil, false
	}
	return databaseChosenMsg{Name: selected.Item.ID}, true
}

func translateCollectionSelection(msg tea.Msg) (tea.Msg, bool) {
	selected, ok := msg.(itemSelectedMsg)
	if !ok {
		return nil, false
	}
	return collectionChosenMsg{Name: selected.Item.ID}, true
}
