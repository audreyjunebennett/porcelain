package indexer

import "testing"

func TestQueue_HasPendingKey(t *testing.T) {
	q := NewQueue(32)
	root := Root{ID: "r1", AbsPath: "/srv"}
	job := Job{Root: root, RelPath: "a.txt", AbsPath: "/srv/a.txt"}
	if q.HasPendingKey(job.Key()) {
		t.Fatal("unexpected pending")
	}
	if !q.Enqueue(IngestEnqueue(job, TierBulk, false, "")) {
		t.Fatal("enqueue failed")
	}
	if !q.HasPendingKey(job.Key()) {
		t.Fatal("expected pending")
	}
}

func TestQueue_TallyScopeQueues_fanoutAndIngest(t *testing.T) {
	r := Resolved{
		DefaultScope: ScopeFragment{ProjectID: "projA", FlavorID: "flA"},
		Roots: []Root{
			{ID: "r1", AbsPath: "/srv"},
		},
	}
	root := r.Roots[0]
	q := NewQueue(50)

	job := Job{Root: root, RelPath: "x.go", AbsPath: "/srv/x.go"}
	if !q.Enqueue(IngestEnqueue(job, TierWrite, false, "")) {
		t.Fatal("enqueue ingest")
	}

	fan := WorkItem{
		Kind:     WorkFanoutList,
		Tier:     TierBulk,
		FanoutID: nextFanoutID(),
		Candidates: []TaggedCandidate{
			{Project: "projB", Flavor: "flB", Candidate: Candidate{Root: root, RelPath: "a", AbsPath: "/srv/a"}},
			{Project: "projB", Flavor: "flB", Candidate: Candidate{Root: root, RelPath: "b", AbsPath: "/srv/b"}},
		},
	}
	if !q.Enqueue(fan) {
		t.Fatal("enqueue fanout")
	}

	ingest, fanout := q.TallyScopeQueues(r)
	skA := ScopeKey("projA", "flA")
	skB := ScopeKey("projB", "flB")
	if ingest[skA] != 1 {
		t.Fatalf("ingest projA=%v map=%v", ingest[skA], ingest)
	}
	if fanout[skB] != 2 {
		t.Fatalf("fanout projB=%v map=%v", fanout[skB], fanout)
	}
}
