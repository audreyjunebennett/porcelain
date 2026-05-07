package indexer

import (
	"testing"
)

func TestComputePerScopeBudget(t *testing.T) {
	cfg := Resolved{QueueDepth: 10, QueueFanoutHWMPercent: 75}
	ix := New(cfg, nil, nil)
	if g := ix.computePerScopeBudget(2); g != 3 {
		t.Fatalf("N=2: got %d want 3 (floor(10*0.75/2))", g)
	}
	if g := ix.computePerScopeBudget(1); g != 7 {
		t.Fatalf("N=1: got %d want 7 (floor(10*0.75/1))", g)
	}
}

func TestComputePerScopeBudget_UnboundedQueue(t *testing.T) {
	cfg := Resolved{QueueDepth: 0, QueueFanoutHWMPercent: 75}
	ix := New(cfg, nil, nil)
	if g := ix.computePerScopeBudget(5); g < 1000 {
		t.Fatalf("unbounded queue should yield large budget, got %d", g)
	}
}

func TestInterleaveTaggedCandidatesByScope_roundRobin(t *testing.T) {
	// Scope keys sort: "a\x00" < "b\x00"
	in := []TaggedCandidate{
		{Project: "a", Flavor: ""},
		{Project: "a", Flavor: ""},
		{Project: "a", Flavor: ""},
		{Project: "b", Flavor: ""},
		{Project: "b", Flavor: ""},
	}
	got := interleaveTaggedCandidatesByScope(in)
	if len(got) != len(in) {
		t.Fatalf("len %d want %d", len(got), len(in))
	}
	want := []string{"a", "b", "a", "b", "a"}
	for i, tc := range got {
		if tc.Project != want[i] {
			t.Fatalf("position %d: project %q want %q", i, tc.Project, want[i])
		}
	}
}

func TestInterleaveTaggedCandidatesByScope_singleScopeNoop(t *testing.T) {
	in := []TaggedCandidate{
		{Project: "x", Flavor: "f"},
		{Project: "x", Flavor: "f"},
	}
	got := interleaveTaggedCandidatesByScope(in)
	if len(got) != 2 || got[0].Project != "x" {
		t.Fatalf("single scope should preserve order, got %+v", got)
	}
}

func TestInterleaveTaggedCandidatesByScope_empty(t *testing.T) {
	if got := interleaveTaggedCandidatesByScope(nil); got != nil {
		t.Fatalf("nil in should give nil out, got %v", got)
	}
	if got := interleaveTaggedCandidatesByScope([]TaggedCandidate{}); len(got) != 0 {
		t.Fatalf("empty in should give empty out")
	}
}
