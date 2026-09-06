package indexer

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func TestQueue_DedupesAndBounds(t *testing.T) {
	q := NewQueue(2)
	j := IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "a"}, TierBulk, false, "")
	if !q.Enqueue(j) {
		t.Fatal("first enqueue should succeed")
	}
	if !q.Enqueue(j) {
		t.Fatal("dedup enqueue should still return true")
	}
	if q.Len() != 1 {
		t.Fatalf("len=%d, want 1", q.Len())
	}
	if !q.Enqueue(IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "b"}, TierBulk, false, "")) {
		t.Fatal("enqueue b")
	}
	if q.Enqueue(IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "c"}, TierBulk, false, "")) {
		t.Fatal("queue should be full")
	}
}

func TestQueue_DequeuePrefersInteractiveTier(t *testing.T) {
	q := NewQueue(10)
	if !q.Enqueue(IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "bulk"}, TierBulk, false, "")) {
		t.Fatal("enqueue bulk")
	}
	if !q.Enqueue(IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "live"}, TierInteractive, false, "")) {
		t.Fatal("enqueue interactive")
	}
	ctx := context.Background()
	w, ok := q.Dequeue(ctx)
	if !ok || w.Job.RelPath != "live" || w.Tier != TierInteractive {
		t.Fatalf("expected interactive first, got %+v ok=%v", w, ok)
	}
	w2, ok := q.Dequeue(ctx)
	if !ok || w2.Job.RelPath != "bulk" {
		t.Fatalf("expected bulk second, got %+v", w2)
	}
}

func TestQueue_IngestTierUpgrade(t *testing.T) {
	q := NewQueue(4)
	low := IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "x"}, TierBulk, false, "")
	high := IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "x"}, TierInteractive, false, "")
	if !q.Enqueue(low) {
		t.Fatal("enqueue low")
	}
	if !q.Enqueue(high) {
		t.Fatal("enqueue upgrade")
	}
	if q.Len() != 1 {
		t.Fatalf("want one pending item after upgrade, got %d", q.Len())
	}
	w, _ := q.Dequeue(context.Background())
	if w.Tier != TierInteractive {
		t.Fatalf("want tier 3, got %d", w.Tier)
	}
}

func TestQueue_DequeueBlocksThenReturns(t *testing.T) {
	q := NewQueue(4)
	got := make(chan WorkItem, 1)
	go func() {
		j, ok := q.Dequeue(context.Background())
		if ok {
			got <- j
		}
	}()
	time.Sleep(10 * time.Millisecond)
	q.Enqueue(IngestEnqueue(Job{Root: Root{ID: "r"}, RelPath: "a"}, TierBulk, false, ""))
	select {
	case j := <-got:
		if j.Job.RelPath != "a" {
			t.Fatalf("got %+v", j)
		}
	case <-time.After(time.Second):
		t.Fatal("dequeue did not return")
	}
}

func TestQueue_DequeueExitsOnContext(t *testing.T) {
	q := NewQueue(4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		_, ok := q.Dequeue(ctx)
		done <- ok
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected !ok on cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("dequeue did not unblock on cancel")
	}
}

func TestQueue_PruneRootsRemovesIngestAndFiltersFanout(t *testing.T) {
	q := NewQueue(20)
	removed := Root{ID: "old", AbsPath: `C:\old`, Scope: ScopeFragment{WorkspaceID: "1", ProjectID: "Old"}}
	kept := Root{ID: "new", AbsPath: `C:\new`, Scope: ScopeFragment{WorkspaceID: "2", ProjectID: "New"}}
	oldScope := ScopeKey("Old", "")

	if !q.Enqueue(IngestEnqueue(Job{Root: removed, RelPath: "old.txt"}, TierBulk, true, oldScope)) {
		t.Fatal("enqueue removed ingest")
	}
	if !q.Enqueue(IngestEnqueue(Job{Root: kept, RelPath: "new.txt"}, TierBulk, false, "")) {
		t.Fatal("enqueue retained ingest")
	}
	if !q.Enqueue(WorkItem{
		Kind:     WorkFanoutList,
		Tier:     TierBulk,
		FanoutID: "mixed",
		Candidates: []TaggedCandidate{
			{Candidate: Candidate{Root: removed, RelPath: "old-2.txt"}, Project: "Old"},
			{Candidate: Candidate{Root: kept, RelPath: "new-2.txt"}, Project: "New"},
		},
	}) {
		t.Fatal("enqueue fanout")
	}

	got := q.PruneRoots([]Root{removed})
	if got.Ingest != 1 || got.FanoutCandidates != 1 || got.PendingBulkByScope[oldScope] != 1 {
		t.Fatalf("unexpected prune result: %+v", got)
	}
	if q.Len() != 2 {
		t.Fatalf("queue length after prune = %d, want 2", q.Len())
	}

	ctx := context.Background()
	first, ok := q.Dequeue(ctx)
	if !ok || first.Kind != WorkIngest || first.Job.Root.ID != kept.ID {
		t.Fatalf("first retained work = %+v, ok=%v", first, ok)
	}
	second, ok := q.Dequeue(ctx)
	if !ok || second.Kind != WorkFanoutList || len(second.Candidates) != 1 || second.Candidates[0].Root.ID != kept.ID {
		t.Fatalf("filtered fanout = %+v, ok=%v", second, ok)
	}
}

func TestBackoff_GrowsAndCaps(t *testing.T) {
	base := 10 * time.Millisecond
	max := 80 * time.Millisecond
	d0 := Backoff(0, base, max, nil)
	d1 := Backoff(1, base, max, nil)
	d3 := Backoff(3, base, max, nil)
	d10 := Backoff(10, base, max, nil)
	if d0 != base || d1 != 2*base || d3 != 8*base || d10 != max {
		t.Fatalf("d0=%v d1=%v d3=%v d10=%v", d0, d1, d3, d10)
	}
	rng := rand.New(rand.NewSource(1))
	if got := Backoff(2, base, max, rng); got > 4*base {
		t.Fatalf("jittered backoff exceeded ceiling: %v", got)
	}
}
