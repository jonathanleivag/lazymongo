package tui

import "testing"

func TestNewDatabaseListModel_ListsGivenNames(t *testing.T) {
	m := newDatabaseListModel([]string{"admin", "haddacloud-v2"})
	if len(m.Items) != 2 || m.Items[0].Label != "admin" || m.Items[1].Label != "haddacloud-v2" {
		t.Fatalf("unexpected items: %+v", m.Items)
	}
}
