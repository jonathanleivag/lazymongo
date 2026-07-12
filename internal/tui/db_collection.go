package tui

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
