//go:build integration

package mongo

import (
	"context"
	"os"
	"testing"
	"time"
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
