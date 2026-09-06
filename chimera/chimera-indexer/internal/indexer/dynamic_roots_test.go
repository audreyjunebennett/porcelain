package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestApplyRootsSnapshot_addsWatchPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rootA := filepath.Join(dir, "a")
	rootB := filepath.Join(dir, "b")
	if err := os.MkdirAll(rootA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootB, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := Resolved{Debounce: defaultDebounce}
	ix := New(cfg, nil, nil)
	ix.setRoots([]Root{{
		ID: rootSlug(rootA), AbsPath: rootA,
		Scope: ScopeFragment{WorkspaceID: "1", ProjectID: "p", FlavorID: "f"},
	}})

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ix.rootUpdates = make(chan rootsDelta, 1)
	if err := ix.registerInitialWatches(context.Background(), w, ix.getRoots()); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	next := []Root{
		{ID: rootSlug(rootA), AbsPath: rootA, Scope: ScopeFragment{WorkspaceID: "1", ProjectID: "p", FlavorID: "f"}},
		{ID: rootSlug(rootB), AbsPath: rootB, Scope: ScopeFragment{WorkspaceID: "1", ProjectID: "p", FlavorID: "f"}},
	}
	changed, err := ix.ApplyRootsSnapshot(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	select {
	case d := <-ix.rootUpdates:
		if err := ix.applyRootsDelta(ctx, w, d); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("expected root update on channel")
	}
	got := ix.getRoots()
	if len(got) != 2 {
		t.Fatalf("roots=%v", got)
	}
	ix.watchMu.Lock()
	nB := len(ix.watchedPaths[rootSlug(rootB)])
	ix.watchMu.Unlock()
	if nB < 1 {
		t.Fatal("expected fsnotify paths for new root")
	}
}

func TestApplyRootsSnapshot_prunesRemovedRootQueue(t *testing.T) {
	t.Parallel()
	removed := Root{ID: "old", AbsPath: `C:\old`, Scope: ScopeFragment{WorkspaceID: "1", ProjectID: "Old"}}
	ix := New(Resolved{Roots: []Root{removed}, QueueDepth: 20}, nil, nil)
	defer ix.Close()
	ix.setRoots([]Root{removed})
	oldScope := ScopeKey("Old", "")
	ix.incPendingBulk(oldScope)
	if !ix.queue.Enqueue(IngestEnqueue(Job{Root: removed, RelPath: "queued.txt"}, TierBulk, true, oldScope)) {
		t.Fatal("enqueue ingest")
	}
	if !ix.queue.Enqueue(WorkItem{
		Kind:     WorkFanoutList,
		Tier:     TierBulk,
		FanoutID: "old-fanout",
		Candidates: []TaggedCandidate{{
			Candidate: Candidate{Root: removed, RelPath: "later.txt"},
			Project:   "Old",
		}},
	}) {
		t.Fatal("enqueue fanout")
	}

	changed, err := ix.ApplyRootsSnapshot(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected root removal")
	}
	if got := ix.queue.Len(); got != 0 {
		t.Fatalf("queue length after root removal = %d, want 0", got)
	}
	if got := ix.pendingBulk(oldScope); got != 0 {
		t.Fatalf("pending bulk after root removal = %d, want 0", got)
	}
	if ix.rootIsActive(removed) {
		t.Fatal("removed root still active")
	}
}
