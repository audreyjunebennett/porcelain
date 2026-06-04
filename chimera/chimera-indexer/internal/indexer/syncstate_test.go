package indexer

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestResolveSyncStateSQLitePath(t *testing.T) {
	t.Parallel()
	json := filepath.Join("data", "gateway", "indexer.sync-state.json")
	want := filepath.Join("data", "gateway", syncStateSQLiteFile)
	if got := ResolveSyncStateSQLitePath(json); got != want {
		t.Fatalf("json path: got %q want %q", got, want)
	}
	explicit := filepath.Join("tmp", "custom.sqlite")
	if got := ResolveSyncStateSQLitePath(explicit); got != explicit {
		t.Fatalf("sqlite path: got %q want %q", got, explicit)
	}
	if got := ResolveSyncStateSQLitePath(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}

func TestOpenSyncState_GetPut(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "indexer.sync-state.json")
	st, err := OpenSyncState(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.StorePath() != filepath.Join(dir, syncStateSQLiteFile) {
		t.Fatalf("StorePath=%q", st.StorePath())
	}
	key := "myroot" + scopeKeySep + "src/a.go"
	if _, ok := st.Get(key); ok {
		t.Fatal("expected miss")
	}
	ent := SyncEntry{ClientSHA: "sha256:client", ServerSHA: "sha256:server", ChunkCount: 3, ChunkSchema: 2}
	if err := st.Put(key, ent); err != nil {
		t.Fatal(err)
	}
	got, ok := st.Get(key)
	if !ok {
		t.Fatal("expected hit")
	}
	if got != ent {
		t.Fatalf("got %+v want %+v", got, ent)
	}
	st2, err := OpenSyncState(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	got2, ok := st2.Get(key)
	if !ok || got2 != ent {
		t.Fatalf("reopen: ok=%v got=%+v", ok, got2)
	}
}

func TestSyncState_DeleteByRoot(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSyncState(filepath.Join(dir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	keys := []struct {
		root, rel string
	}{
		{"root-a", "one.go"},
		{"root-a", "two.go"},
		{"root-b", "three.go"},
	}
	for _, k := range keys {
		key := k.root + scopeKeySep + k.rel
		if err := st.Put(key, SyncEntry{ClientSHA: "c", ServerSHA: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.DeleteByRoot("root-a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Get("root-a" + scopeKeySep + "one.go"); ok {
		t.Fatal("root-a should be gone")
	}
	if _, ok := st.Get("root-b" + scopeKeySep + "three.go"); !ok {
		t.Fatal("root-b should remain")
	}
}

func TestSyncState_DeleteAll(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSyncState(filepath.Join(dir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Put("r"+scopeKeySep+"x", SyncEntry{ClientSHA: "c", ServerSHA: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Get("r" + scopeKeySep + "x"); ok {
		t.Fatal("expected empty store")
	}
}

func TestSyncState_ConcurrentPut(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSyncState(filepath.Join(dir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := "root" + scopeKeySep + filepath.Join("pkg", string(rune('a'+i))+".go")
			_ = st.Put(key, SyncEntry{ClientSHA: "c", ServerSHA: "s"})
		}()
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		key := "root" + scopeKeySep + filepath.Join("pkg", string(rune('a'+i))+".go")
		if _, ok := st.Get(key); !ok {
			t.Fatalf("missing %s", key)
		}
	}
}
