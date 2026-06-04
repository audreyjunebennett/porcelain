package indexer

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestTryAutoRepairMissingCollection_clearsSyncAndSchedulesScan(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSyncState(dir + "/sync.json")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	key := "root-a" + scopeKeySep + "a.go"
	if err := st.Put(key, SyncEntry{ClientSHA: "c", ServerSHA: "s", ChunkSchema: manifestChunkSchema}); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ix := &Indexer{
		cfg: Resolved{
			Roots: []Root{{
				ID:      "root-a",
				AbsPath: dir,
				Scope:   ScopeFragment{ProjectID: "proj", FlavorID: "fl"},
			}},
		},
		syncState: st,
		log:       log,
		queue:     NewQueue(8),
	}
	ix.initialScanCompleted.Store(true)
	gw := &IndexerConfig{TenantID: "t"}
	gw.Defaults.ProjectID = "proj"
	gw.Defaults.FlavorID = "fl"
	ix.lastGW.Store(gw)

	if !ix.tryAutoRepairMissingCollection(context.Background(), ScopeFragment{ProjectID: "proj", FlavorID: "fl"}, "qdrant GET 404 collection doesn't exist", IndexerKey("t", "proj", "fl")) {
		t.Fatal("expected repair")
	}
	if _, ok := st.Get(key); ok {
		t.Fatal("sync entry should be cleared")
	}
	if ix.queue.Len() < 1 {
		t.Fatal("expected scan scheduled")
	}
	if ix.tryAutoRepairMissingCollection(context.Background(), ScopeFragment{ProjectID: "proj", FlavorID: "fl"}, "404 not found", IndexerKey("t", "proj", "fl")) {
		t.Fatal("second repair should be no-op")
	}
}
