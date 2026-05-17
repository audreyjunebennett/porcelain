package indexer

import (
	"testing"
)

func TestIndexerKey_Stability(t *testing.T) {
	a := IndexerKey("tenant-a", "proj1", "flav1")
	b := IndexerKey("tenant-a", "proj1", "flav1")
	if a != b {
		t.Fatalf("expected stable key, got %q vs %q", a, b)
	}
	if len(a) < 8 {
		t.Fatalf("unexpected key: %q", a)
	}
	if IndexerKey("tenant-a", "p1", "f") == IndexerKey("tenant-a", "p2", "f") {
		t.Fatal("expected distinct keys for different projects")
	}
	if IndexerKey("", "", "") == IndexerKey("z", "", "") {
		t.Fatal("expected distinct keys for different tenants")
	}
}
