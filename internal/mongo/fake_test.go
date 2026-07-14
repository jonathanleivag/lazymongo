package mongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFakeClient_CreateCollectionCreatesDatabaseAndCollection(t *testing.T) {
	f := NewFakeClient()
	if err := f.CreateCollection(context.Background(), "shop", "orders"); err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	if _, ok := f.Databases["shop"]["orders"]; !ok {
		t.Fatalf("expected shop.orders to exist, got %+v", f.Databases)
	}
}

func TestFakeClient_CreateCollectionRejectsExistingCollection(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {}}

	if err := f.CreateCollection(context.Background(), "shop", "orders"); err == nil {
		t.Fatal("expected an error creating a collection that already exists")
	}
}

func TestFakeClient_DropCollectionRemovesItAndItsIndexes(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {{"_id": "1"}}}
	f.Indexes["shop"] = map[string][]IndexInfo{"orders": {{Name: "email_1"}}}

	if err := f.DropCollection(context.Background(), "shop", "orders"); err != nil {
		t.Fatalf("DropCollection failed: %v", err)
	}
	if _, ok := f.Databases["shop"]["orders"]; ok {
		t.Fatal("expected shop.orders to be gone")
	}
	if _, ok := f.Indexes["shop"]["orders"]; ok {
		t.Fatal("expected shop.orders' indexes to be gone too")
	}
}

func TestFakeClient_DropCollectionRejectsMissingCollection(t *testing.T) {
	f := NewFakeClient()
	if err := f.DropCollection(context.Background(), "shop", "ghost"); err == nil {
		t.Fatal("expected an error dropping a collection that doesn't exist")
	}
}

func TestFakeClient_DropDatabaseRemovesAllItsCollectionsAndIndexes(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {{"_id": "1"}}}
	f.Indexes["shop"] = map[string][]IndexInfo{"orders": {{Name: "email_1"}}}

	if err := f.DropDatabase(context.Background(), "shop"); err != nil {
		t.Fatalf("DropDatabase failed: %v", err)
	}
	if _, ok := f.Databases["shop"]; ok {
		t.Fatal("expected 'shop' to be gone from Databases")
	}
	if _, ok := f.Indexes["shop"]; ok {
		t.Fatal("expected 'shop' to be gone from Indexes")
	}
}

func TestFakeClient_RenameCollectionMovesDocsAndIndexesToNewName(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {{"_id": "1"}}}
	f.Indexes["shop"] = map[string][]IndexInfo{"orders": {{Name: "email_1"}}}

	if err := f.RenameCollection(context.Background(), "shop", "orders", "orders_v2"); err != nil {
		t.Fatalf("RenameCollection failed: %v", err)
	}
	if _, ok := f.Databases["shop"]["orders"]; ok {
		t.Fatal("expected old name 'orders' to be gone")
	}
	docs, ok := f.Databases["shop"]["orders_v2"]
	if !ok || len(docs) != 1 {
		t.Fatalf("expected docs moved to 'orders_v2', got %+v", f.Databases["shop"])
	}
	idxs, ok := f.Indexes["shop"]["orders_v2"]
	if !ok || len(idxs) != 1 {
		t.Fatalf("expected indexes moved to 'orders_v2', got %+v", f.Indexes["shop"])
	}
}

func TestFakeClient_RenameCollectionRejectsMissingOldName(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{}

	if err := f.RenameCollection(context.Background(), "shop", "ghost", "new"); err == nil {
		t.Fatal("expected an error renaming a collection that doesn't exist")
	}
}

func TestFakeClient_RenameCollectionRejectsCollisionWithDifferentExistingCollection(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "1"}},
		"users":  {{"_id": "2"}},
	}

	if err := f.RenameCollection(context.Background(), "shop", "orders", "users"); err == nil {
		t.Fatal("expected an error renaming 'orders' to the already-existing 'users'")
	}
	if _, ok := f.Databases["shop"]["orders"]; !ok {
		t.Fatal("expected 'orders' to still exist after the rejected rename")
	}
	if len(f.Databases["shop"]["users"]) != 1 {
		t.Fatalf("expected 'users' to be untouched after the rejected rename, got %+v", f.Databases["shop"]["users"])
	}
}

func TestFakeClient_RenameCollectionToSameNameSucceedsAsNoOp(t *testing.T) {
	f := NewFakeClient()
	f.Databases["shop"] = map[string][]bson.M{"orders": {{"_id": "1"}}}
	f.Indexes["shop"] = map[string][]IndexInfo{"orders": {{Name: "email_1"}}}

	if err := f.RenameCollection(context.Background(), "shop", "orders", "orders"); err != nil {
		t.Fatalf("expected renaming 'orders' to itself to succeed as a no-op, got: %v", err)
	}
	docs, ok := f.Databases["shop"]["orders"]
	if !ok || len(docs) != 1 {
		t.Fatalf("expected 'orders' to still exist with its docs intact, got %+v", f.Databases["shop"])
	}
	idxs, ok := f.Indexes["shop"]["orders"]
	if !ok || len(idxs) != 1 {
		t.Fatalf("expected 'orders' indexes to still exist, got %+v", f.Indexes["shop"])
	}
}
