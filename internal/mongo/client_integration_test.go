//go:build integration

package mongo

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestRealClient_ConnectAndListDatabases(t *testing.T) {
	uri := os.Getenv("LAZYMONGO_TEST_URI")
	if uri == "" {
		t.Skip("LAZYMONGO_TEST_URI not set; run via scripts/test-integration.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewRealClient()
	if err := client.Connect(ctx, uri); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect(ctx)

	names, err := client.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDatabases failed: %v", err)
	}
	found := false
	for _, n := range names {
		if n == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'admin' database to be present, got %v", names)
	}
}

func TestRealClient_DocumentCRUD(t *testing.T) {
	uri := os.Getenv("LAZYMONGO_TEST_URI")
	if uri == "" {
		t.Skip("LAZYMONGO_TEST_URI not set; run via scripts/test-integration.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewRealClient()
	if err := client.Connect(ctx, uri); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect(ctx)

	const db, coll = "lazymongo_test", "widgets"

	id, err := client.InsertOne(ctx, db, coll, bson.M{"name": "gizmo", "qty": 3})
	if err != nil {
		t.Fatalf("InsertOne failed: %v", err)
	}

	docs, err := client.Find(ctx, db, coll, bson.M{"name": "gizmo"}, 0, 10)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	count, err := client.CountDocuments(ctx, db, coll, bson.M{})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	if err := client.UpdateField(ctx, db, coll, id, "qty", 5); err != nil {
		t.Fatalf("UpdateField failed: %v", err)
	}
	docs, _ = client.Find(ctx, db, coll, bson.M{"_id": id}, 0, 1)
	if len(docs) != 1 || docs[0]["qty"] != int32(5) {
		t.Fatalf("expected qty updated to 5, got %+v", docs)
	}

	if err := client.ReplaceOne(ctx, db, coll, id, bson.M{"name": "gizmo-v2"}); err != nil {
		t.Fatalf("ReplaceOne failed: %v", err)
	}
	docs, _ = client.Find(ctx, db, coll, bson.M{"_id": id}, 0, 1)
	if len(docs) != 1 || docs[0]["name"] != "gizmo-v2" {
		t.Fatalf("expected replaced document, got %+v", docs)
	}

	if err := client.DeleteOne(ctx, db, coll, id); err != nil {
		t.Fatalf("DeleteOne failed: %v", err)
	}
	count, _ = client.CountDocuments(ctx, db, coll, bson.M{})
	if count != 0 {
		t.Fatalf("expected count 0 after delete, got %d", count)
	}
}

func TestRealClient_Indexes(t *testing.T) {
	uri := os.Getenv("LAZYMONGO_TEST_URI")
	if uri == "" {
		t.Skip("LAZYMONGO_TEST_URI not set; run via scripts/test-integration.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewRealClient()
	if err := client.Connect(ctx, uri); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect(ctx)

	const db, coll = "lazymongo_test", "indexed_widgets"
	_, _ = client.InsertOne(ctx, db, coll, bson.M{"sku": "abc"})

	name, err := client.CreateIndex(ctx, db, coll, bson.D{{Key: "sku", Value: 1}}, true)
	if err != nil {
		t.Fatalf("CreateIndex failed: %v", err)
	}
	if name == "" {
		t.Fatal("expected a non-empty index name")
	}

	indexes, err := client.ListIndexes(ctx, db, coll)
	if err != nil {
		t.Fatalf("ListIndexes failed: %v", err)
	}
	found := false
	for _, idx := range indexes {
		if idx.Name == name {
			found = true
			if !idx.Unique {
				t.Fatalf("expected index %q to be unique", name)
			}
		}
	}
	if !found {
		t.Fatalf("created index %q not found in ListIndexes result: %+v", name, indexes)
	}

	if err := client.DropIndex(ctx, db, coll, name); err != nil {
		t.Fatalf("DropIndex failed: %v", err)
	}
	indexes, _ = client.ListIndexes(ctx, db, coll)
	for _, idx := range indexes {
		if idx.Name == name {
			t.Fatalf("index %q still present after DropIndex", name)
		}
	}
}
