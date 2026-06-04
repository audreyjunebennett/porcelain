package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectStaleSources_detectsChangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("version one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := OpenSyncState(filepath.Join(dir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rootID := "r1"
	key := rootID + scopeKeySep + "a.go"
	if err := st.Put(key, SyncEntry{ClientSHA: "sha256:old", ServerSHA: "sha256:old"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("version two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Resolved{
		Roots: []Root{{
			ID: rootID, AbsPath: dir, Scope: ScopeFragment{ProjectID: "p", FlavorID: "f"},
		}},
	}
	stale := CollectStaleSources(cfg, nil, st, cfg.Roots)
	if len(stale) != 1 {
		t.Fatalf("len=%d want 1", len(stale))
	}
	if stale[0].Source != "a.go" {
		t.Fatalf("source=%q", stale[0].Source)
	}
	if stale[0].LiveSHA256 == stale[0].IndexedSHA256 {
		t.Fatal("expected different hashes")
	}
}
