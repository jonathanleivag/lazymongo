package tui

import "testing"

func TestNewDatabaseListModel_ListsGivenNames(t *testing.T) {
	m := newDatabaseListModel([]string{"admin", "haddacloud-v2"})
	if len(m.Items) != 2 || m.Items[0].Label != "admin" || m.Items[1].Label != "haddacloud-v2" {
		t.Fatalf("unexpected items: %+v", m.Items)
	}
}

func TestTranslateDatabaseSelection(t *testing.T) {
	msg := itemSelectedMsg{Item: listItem{ID: "admin", Label: "admin"}}
	translated, ok := translateDatabaseSelection(msg)
	if !ok {
		t.Fatal("expected translation to succeed for itemSelectedMsg")
	}
	chosen, ok := translated.(databaseChosenMsg)
	if !ok || chosen.Name != "admin" {
		t.Fatalf("expected databaseChosenMsg{Name:\"admin\"}, got %#v", translated)
	}

	_, ok = translateDatabaseSelection(listBackMsg{})
	if ok {
		t.Fatal("expected translation to fail for a non-selection message")
	}
}

func TestTranslateCollectionSelection(t *testing.T) {
	msg := itemSelectedMsg{Item: listItem{ID: "users", Label: "users"}}
	translated, ok := translateCollectionSelection(msg)
	if !ok {
		t.Fatal("expected translation to succeed for itemSelectedMsg")
	}
	chosen, ok := translated.(collectionChosenMsg)
	if !ok || chosen.Name != "users" {
		t.Fatalf("expected collectionChosenMsg{Name:\"users\"}, got %#v", translated)
	}
}
