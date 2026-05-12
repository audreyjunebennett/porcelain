package rag

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lynn/claudia-gateway/internal/vectorstore"
)

// fakeStore is a minimal in-memory vectorstore.Store.
type fakeStore struct {
	mu          sync.Mutex
	collections map[string]int
	points      map[string][]vectorstore.Point
	deletedSrcs map[string][]string
	healthErr   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		collections: map[string]int{},
		points:      map[string][]vectorstore.Point{},
		deletedSrcs: map[string][]string{},
	}
}

func (s *fakeStore) EnsureCollection(_ context.Context, name string, dim int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collections[name] = dim
	return nil
}
func (s *fakeStore) Upsert(_ context.Context, c string, pts []vectorstore.Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points[c] = append(s.points[c], pts...)
	return nil
}
func (s *fakeStore) Search(_ context.Context, c string, _ []float32, k int, _ float32, _ *vectorstore.Coords) ([]vectorstore.Hit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pts := s.points[c]
	out := []vectorstore.Hit{}
	for i, p := range pts {
		if i >= k {
			break
		}
		out = append(out, vectorstore.Hit{ID: p.ID, Score: 0.9, Payload: p.Payload})
	}
	return out, nil
}
func (s *fakeStore) Health(context.Context) error { return s.healthErr }
func (s *fakeStore) Stats(_ context.Context, c string) (vectorstore.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return vectorstore.Stats{Collection: c, Points: int64(len(s.points[c])), VectorDim: s.collections[c]}, nil
}
func (s *fakeStore) DeleteBySource(_ context.Context, c, src string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedSrcs[c] = append(s.deletedSrcs[c], src)
	keep := s.points[c][:0]
	for _, p := range s.points[c] {
		if p.Payload.Source != src {
			keep = append(keep, p)
		}
	}
	s.points[c] = keep
	return nil
}

func (s *fakeStore) ScrollPoints(_ context.Context, c string, filter *vectorstore.Coords, limit int, cursor string) (vectorstore.ScrollBatch, error) {
	if limit <= 0 {
		limit = 256
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows []vectorstore.PointPayload
	for _, p := range s.points[c] {
		if filter != nil {
			if filter.TenantID != "" && p.Payload.TenantID != filter.TenantID {
				continue
			}
			if filter.ProjectID != "" && p.Payload.ProjectID != filter.ProjectID {
				continue
			}
			if filter.FlavorID != "" && p.Payload.FlavorID != filter.FlavorID {
				continue
			}
		}
		rows = append(rows, vectorstore.PointPayload{ID: p.ID, Payload: p.Payload})
	}
	start := 0
	if cursor != "" {
		_, _ = fmt.Sscanf(cursor, "%d", &start)
	}
	if start >= len(rows) {
		return vectorstore.ScrollBatch{}, nil
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	next := ""
	if end < len(rows) {
		next = strconv.Itoa(end)
	}
	return vectorstore.ScrollBatch{Points: rows[start:end], NextCursor: next}, nil
}

// fakeEmbedder returns deterministic vectors of dim sized [i+1, 0, ..., 0].
type fakeEmbedder struct {
	dim   int
	model string
	calls int
}

func (e *fakeEmbedder) EmbedBatch(_ context.Context, in []string) ([][]float32, error) {
	e.calls++
	out := make([][]float32, len(in))
	for i := range in {
		v := make([]float32, e.dim)
		v[0] = float32(i + 1)
		out[i] = v
	}
	return out, nil
}
func (e *fakeEmbedder) EmbedOne(ctx context.Context, s string) ([]float32, error) {
	v, err := e.EmbedBatch(ctx, []string{s})
	if err != nil {
		return nil, err
	}
	return v[0], nil
}
func (e *fakeEmbedder) Model() string { return e.model }

func newSvc(t *testing.T) (*Service, *fakeStore, *fakeEmbedder) {
	t.Helper()
	st := newFakeStore()
	em := &fakeEmbedder{dim: 8, model: "test-embed"}
	s, err := New(Options{Store: st, Embedder: em, EmbeddingDim: 8, ChunkSize: 64, ChunkOverlap: 16, TopK: 4, ScoreThreshold: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	return s, st, em
}

func TestService_Ingest_HappyPath(t *testing.T) {
	s, st, em := newSvc(t)
	text := strings.Repeat("hello world ", 50)
	res, err := s.Ingest(context.Background(), IngestRequest{
		Coords: vectorstore.Coords{TenantID: "t1", ProjectID: "proj"},
		Source: "docs/readme.md",
		Text:   text,
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.Chunks < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", res.Chunks)
	}
	if !strings.HasPrefix(res.ContentHash, "sha256:") {
		t.Fatalf("expected sha256 hash, got %q", res.ContentHash)
	}
	if res.ContentSHA256 != res.ContentHash {
		t.Fatalf("content_sha256 should match content_hash, got %q vs %q", res.ContentSHA256, res.ContentHash)
	}
	pts := st.points[res.Collection]
	if len(pts) != res.Chunks {
		t.Fatalf("upsert count mismatch: %d vs %d", len(pts), res.Chunks)
	}
	if pts[0].Payload.TenantID != "t1" || pts[0].Payload.Source != "docs/readme.md" {
		t.Fatalf("payload missing fields: %+v", pts[0].Payload)
	}
	if pts[0].Payload.ContentSHA256 == "" {
		t.Fatalf("expected content_sha256 on payload, got %+v", pts[0].Payload)
	}
	if em.calls != 1 {
		t.Fatalf("embed should be batched once, got %d calls", em.calls)
	}
}

func TestService_Ingest_ReingestDeletesOld(t *testing.T) {
	s, st, _ := newSvc(t)
	req := IngestRequest{
		Coords: vectorstore.Coords{TenantID: "t1", ProjectID: "proj"},
		Source: "a.txt",
		Text:   strings.Repeat("x", 200),
	}
	if _, err := s.Ingest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	coll := vectorstore.CollectionName(req.Coords)
	if got := st.deletedSrcs[coll]; len(got) != 2 || got[0] != "a.txt" {
		t.Fatalf("expected delete-by-source called twice, got %v", got)
	}
}

func TestService_Ingest_Validation(t *testing.T) {
	s, _, _ := newSvc(t)
	cases := []struct {
		name string
		req  IngestRequest
		errs string
	}{
		{"empty source", IngestRequest{Coords: vectorstore.Coords{TenantID: "t"}, Text: "x"}, "empty source"},
		{"empty text", IngestRequest{Coords: vectorstore.Coords{TenantID: "t"}, Source: "a"}, "empty text"},
		{"empty tenant", IngestRequest{Source: "a", Text: "x"}, "empty tenant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Ingest(context.Background(), tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.errs) {
				t.Fatalf("got %v, want contains %q", err, tc.errs)
			}
		})
	}
}

func TestService_Retrieve_HappyPath(t *testing.T) {
	s, _, _ := newSvc(t)
	req := IngestRequest{
		Coords: vectorstore.Coords{TenantID: "t1", ProjectID: "proj"},
		Source: "doc.txt",
		Text:   strings.Repeat("alpha ", 50),
	}
	if _, err := s.Ingest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Retrieve(context.Background(), RetrieveRequest{
		Coords: req.Coords,
		Query:  "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	if hits[0].Payload.Source != "doc.txt" {
		t.Fatalf("payload: %+v", hits[0].Payload)
	}
}

func TestService_CorpusInventory(t *testing.T) {
	s, _, _ := newSvc(t)
	_, err := s.Ingest(context.Background(), IngestRequest{
		Coords:      vectorstore.Coords{TenantID: "t1", ProjectID: "proj"},
		Source:      "x.go",
		Text:        strings.Repeat("z", 200),
		ContentHash: "sha256:abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, next, err := s.CorpusInventory(context.Background(), vectorstore.Coords{TenantID: "t1", ProjectID: "proj"}, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || next != "" {
		t.Fatalf("entries=%v next=%q", entries, next)
	}
	if entries[0].Source != "x.go" || entries[0].ClientContentHash != "sha256:abc" || !strings.HasPrefix(entries[0].ContentSHA256, "sha256:") {
		t.Fatalf("entry: %+v", entries[0])
	}
}

func TestService_Retrieve_EmptyQuery(t *testing.T) {
	s, _, _ := newSvc(t)
	hits, err := s.Retrieve(context.Background(), RetrieveRequest{
		Coords: vectorstore.Coords{TenantID: "t"},
		Query:  "",
	})
	if err != nil || hits != nil {
		t.Fatalf("expected nil/nil for empty query, got %v %v", hits, err)
	}
}

func TestService_Retrieve_logContainsPrincipalId(t *testing.T) {
	st := newFakeStore()
	em := &fakeEmbedder{dim: 8, model: "test-embed"}
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s, err := New(Options{
		Store: st, Embedder: em, EmbeddingDim: 8, ChunkSize: 64, ChunkOverlap: 16, TopK: 4, ScoreThreshold: 0.5, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := vectorstore.Coords{TenantID: "tenant-principal-test", ProjectID: "p"}
	if _, err := s.Ingest(context.Background(), IngestRequest{Coords: c, Source: "s.txt", Text: strings.Repeat("w ", 100)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retrieve(context.Background(), RetrieveRequest{
		Coords:         c,
		Query:          "hello w",
		RequestID:      "rid-principal-test",
		ConversationID: "conv-principal-test",
	}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "principal_id=tenant-principal-test") {
		t.Fatalf("missing principal_id in RAG retrieve logs:\n%s", out)
	}
	if !strings.Contains(out, "request_id=rid-principal-test") || !strings.Contains(out, "conversation_id=conv-principal-test") {
		t.Fatalf("missing request/conversation correlation:\n%s", out)
	}
	if !strings.Contains(out, "msg=conversation.rag.span") || !strings.Contains(out, "window_ms=10000") {
		t.Fatalf("missing fallback lifecycle RAG span:\n%s", out)
	}
}
