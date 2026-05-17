package rag

import (
	"testing"

	"github.com/lynn/porcelain/chimera/internal/vectorstore"
)

func TestHitsBySourceCount(t *testing.T) {
	hits := []vectorstore.Hit{
		{Payload: vectorstore.Payload{Source: "a.md"}},
		{Payload: vectorstore.Payload{Source: "b.md"}},
		{Payload: vectorstore.Payload{Source: "a.md"}},
		{Payload: vectorstore.Payload{Source: "  "}},
	}
	got := HitsBySourceCount(hits)
	if got["a.md"] != 2 || got["b.md"] != 1 || got["unknown"] != 1 {
		t.Fatalf("unexpected counts: %#v", got)
	}
	if len(SortedSources(got)) != 3 {
		t.Fatalf("sorted keys: %v", SortedSources(got))
	}
}
