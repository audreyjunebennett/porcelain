package rag

import (
	"context"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/corpusstale"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

func TestExpansionService_ContextAround_mergesOverlappingSegments(t *testing.T) {
	store := newFakeStore()
	coll := vectorstore.CollectionName(vectorstore.Coords{TenantID: "t", ProjectID: "p", FlavorID: "f"})
	pid := vectorstore.PointID(vectorstore.Coords{TenantID: "t", ProjectID: "p", FlavorID: "f"}, "src/a.go", 0)
	_ = store.Upsert(context.Background(), coll, []vectorstore.Point{{
		ID: pid, Vector: make([]float32, 8),
		Payload: vectorstore.Payload{TenantID: "t", ProjectID: "p", FlavorID: "f", Source: "src/a.go", Text: "line one\nline two"},
	}})
	op, err := operatorstore.Open(t.TempDir()+"/op.sqlite", testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Close()
	rows := []operatorstore.CorpusSegmentRow{{
		SegmentID: pid, TenantID: "t", ProjectID: "p", FlavorID: "f", Source: "src/a.go", ContentSHA256: "sha256:abc",
		ChunkIndex: 0, ChunkCount: 1, StartLine: 1, EndLine: 2, VectorPointID: pid,
	}}
	if err := op.ReplaceCorpusSegmentsForSource(context.Background(), "t", "p", "f", "src/a.go", rows); err != nil {
		t.Fatal(err)
	}
	svc, _ := New(Options{Store: store, Embedder: &fakeEmbedder{dim: 8}, EmbeddingDim: 8})
	stale := corpusstale.NewStore()
	exp := NewExpansionService(ExpansionOptions{RAG: svc, OperatorStore: op, StaleStore: stale})
	coords := vectorstore.Coords{TenantID: "t", ProjectID: "p", FlavorID: "f"}
	text, err := exp.ContextAround(context.Background(), coords, "src/a.go", "sha256:abc", 1, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if text == "" || !contains(text, "line one") {
		t.Fatalf("got %q", text)
	}
}

func TestExpansionService_AdjacentChunks_returnsNeighbors(t *testing.T) {
	store := newFakeStore()
	coll := vectorstore.CollectionName(vectorstore.Coords{TenantID: "t", ProjectID: "p", FlavorID: "f"})
	coords := vectorstore.Coords{TenantID: "t", ProjectID: "p", FlavorID: "f"}
	var pts []vectorstore.Point
	var rows []operatorstore.CorpusSegmentRow
	for i := 0; i < 3; i++ {
		pid := vectorstore.PointID(coords, "src/a.go", i)
		pts = append(pts, vectorstore.Point{
			ID: pid, Vector: make([]float32, 8),
			Payload: vectorstore.Payload{
				TenantID: "t", ProjectID: "p", FlavorID: "f",
				Source: "src/a.go", Text: "chunk " + string(rune('0'+i)),
			},
		})
		rows = append(rows, operatorstore.CorpusSegmentRow{
			SegmentID: pid, TenantID: "t", ProjectID: "p", FlavorID: "f", Source: "src/a.go",
			ContentSHA256: "sha256:abc", ChunkIndex: i, ChunkCount: 3,
			StartLine: i + 1, EndLine: i + 1, VectorPointID: pid,
		})
	}
	if err := store.Upsert(context.Background(), coll, pts); err != nil {
		t.Fatal(err)
	}
	op, err := operatorstore.Open(t.TempDir()+"/op.sqlite", testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Close()
	if err := op.ReplaceCorpusSegmentsForSource(context.Background(), "t", "p", "f", "src/a.go", rows); err != nil {
		t.Fatal(err)
	}
	svc, _ := New(Options{Store: store, Embedder: &fakeEmbedder{dim: 8}, EmbeddingDim: 8})
	exp := NewExpansionService(ExpansionOptions{RAG: svc, OperatorStore: op, StaleStore: corpusstale.NewStore()})
	mid := rows[1].VectorPointID
	hits, err := exp.AdjacentChunks(context.Background(), coords, mid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("want 3 adjacent hits, got %d", len(hits))
	}
	indices := map[int]bool{}
	for _, h := range hits {
		indices[h.Segment.ChunkIndex] = true
		if h.Text == "" {
			t.Fatalf("missing text for chunk %d", h.Segment.ChunkIndex)
		}
	}
	for _, want := range []int{0, 1, 2} {
		if !indices[want] {
			t.Fatalf("missing chunk_index %d in %+v", want, hits)
		}
	}
}

func TestExpansionService_strictRejectsStale(t *testing.T) {
	store := newFakeStore()
	op, err := operatorstore.Open(t.TempDir()+"/op.sqlite", testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer op.Close()
	svc, _ := New(Options{Store: store, Embedder: &fakeEmbedder{dim: 8}, EmbeddingDim: 8})
	stale := corpusstale.NewStore()
	stale.ReplaceScope("t", "p", "f", []corpusstale.Entry{{
		Source: "src/a.go", IndexedSHA256: "sha256:old", LiveSHA256: "sha256:new",
	}})
	exp := NewExpansionService(ExpansionOptions{
		RAG: svc, OperatorStore: op, StaleStore: stale,
		Resolved: nil,
	})
	exp.coherence = "strict"
	_, err = exp.ListSegments(context.Background(), vectorstore.Coords{TenantID: "t", ProjectID: "p", FlavorID: "f"}, "src/a.go", "sha256:old")
	if err == nil {
		t.Fatal("expected stale error")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
