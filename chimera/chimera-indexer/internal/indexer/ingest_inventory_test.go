package indexer

import "testing"

func TestCorpusInventoryKey_scopedPaths(t *testing.T) {
	t.Parallel()
	k1 := CorpusInventoryKey("porcelain", "", "docs/a.md")
	k2 := CorpusInventoryKey("assistants", "", "docs/a.md")
	if k1 == k2 {
		t.Fatal("same rel path in different projects must not collide")
	}
	if CorpusInventoryKey("porcelain", "", "a.go") != ScopeKey("porcelain", "")+scopeKeySep+"a.go" {
		t.Fatal("unexpected key format")
	}
}

func TestLoadRemoteCorpusInventory_requiresRoots(t *testing.T) {
	ix := &Indexer{
		cfg:    Resolved{},
		client: NewGatewayClient("http://127.0.0.1:1", "tok", 0),
	}
	ix.lastGW.Store(&IndexerConfig{CorpusInventoryPath: "/v1/indexer/corpus/inventory"})
	if err := ix.loadRemoteCorpusInventory(t.Context()); err != nil {
		t.Fatalf("no roots: %v", err)
	}
	if ix.remoteInv != nil {
		t.Fatal("expected nil remoteInv with no roots")
	}
}
