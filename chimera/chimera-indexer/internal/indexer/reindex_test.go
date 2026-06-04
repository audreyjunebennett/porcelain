package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyWorkspacesReindex_BaselinesWithoutClearing(t *testing.T) {
	dir := t.TempDir()
	watchRoot := filepath.Join(dir, "w")
	if err := os.MkdirAll(watchRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := OpenSyncState(filepath.Join(dir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rootID := rootSlug(watchRoot)
	key := rootID + scopeKeySep + "main.go"
	if err := st.Put(key, SyncEntry{ClientSHA: "c", ServerSHA: "s"}); err != nil {
		t.Fatal(err)
	}

	resp := &WorkspacesAPIResponse{
		Workspaces: []WorkspaceAPIEntry{{
			WorkspaceID:       1,
			ProjectID:         "proj",
			ReindexGeneration: 3,
			Paths:             []WorkspacePathAPI{{PathID: 1, Path: watchRoot}},
		}},
	}
	tr := NewReindexTracker()
	if ApplyWorkspacesReindex(context.Background(), st, resp, tr, nil) {
		t.Fatal("first poll should baseline generation only")
	}
	if _, ok := st.Get(key); !ok {
		t.Fatal("sync entry should remain after baseline poll")
	}
	if ApplyWorkspacesReindex(context.Background(), st, resp, tr, nil) {
		t.Fatal("unchanged generation should not reindex")
	}
	if _, ok := st.Get(key); !ok {
		t.Fatal("sync entry should remain when generation unchanged")
	}

	resp.Workspaces[0].ReindexGeneration = 4
	if !ApplyWorkspacesReindex(context.Background(), st, resp, tr, nil) {
		t.Fatal("generation bump should trigger reindex")
	}
	if _, ok := st.Get(key); ok {
		t.Fatal("sync entry should be cleared after generation bump")
	}
}
