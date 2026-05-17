package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/vectorstore"
)

// fakeQdrant is a tiny stub of the routes used by Client.
type fakeQdrant struct {
	mu          sync.Mutex
	collections map[string]int // name -> dim
	points      map[string][]map[string]any
	failHealth  bool
}

func newFake() *fakeQdrant {
	return &fakeQdrant{collections: map[string]int{}, points: map[string][]map[string]any{}}
}

func (f *fakeQdrant) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		fail := f.failHealth
		f.mu.Unlock()
		if fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"title":"qdrant - vector search engine"}`)
	})
	mux.HandleFunc("/collections/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/collections/")
		f.mu.Lock()
		defer f.mu.Unlock()
		// Order from most-specific to least.
		switch {
		case strings.HasSuffix(rest, "/points/search") && r.Method == http.MethodPost:
			coll := strings.TrimSuffix(rest, "/points/search")
			pts := f.points[coll]
			out := []map[string]any{}
			for _, p := range pts {
				out = append(out, map[string]any{
					"id":      p["__id"],
					"score":   float32(0.9),
					"payload": p,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": out})
		case strings.HasSuffix(rest, "/points/delete") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"status": "completed"}})
		case strings.HasSuffix(rest, "/points/scroll") && r.Method == http.MethodPost:
			coll := strings.TrimSuffix(rest, "/points/scroll")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			limit := 100
			if v, ok := body["limit"].(float64); ok && int(v) > 0 {
				limit = int(v)
			}
			off := 0
			if v, ok := body["offset"]; ok && v != nil {
				switch t := v.(type) {
				case float64:
					off = int(t)
				}
			}
			pts := f.points[coll]
			end := off + limit
			if end > len(pts) {
				end = len(pts)
			}
			var chunk []map[string]any
			if off < len(pts) {
				chunk = pts[off:end]
			}
			var next any
			if end < len(pts) {
				next = end
			}
			outPts := make([]map[string]any, 0, len(chunk))
			for _, p := range chunk {
				id := p["__id"]
				cp := map[string]any{}
				for k, v := range p {
					if k == "__id" || k == "__vector" {
						continue
					}
					cp[k] = v
				}
				outPts = append(outPts, map[string]any{"id": id, "payload": cp})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"points": outPts, "next_page_offset": next}})
		case strings.HasSuffix(rest, "/points") && r.Method == http.MethodPut:
			coll := strings.TrimSuffix(rest, "/points")
			var body struct {
				Points []map[string]any `json:"points"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, p := range body.Points {
				payload, _ := p["payload"].(map[string]any)
				if payload == nil {
					payload = map[string]any{}
				}
				payload["__id"] = p["id"]
				payload["__vector"] = p["vector"]
				f.points[coll] = append(f.points[coll], payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"status": "completed"}})
		case strings.HasSuffix(rest, "/index") && r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{"result": true})
		case !strings.Contains(rest, "/") && r.Method == http.MethodGet:
			dim, ok := f.collections[rest]
			if !ok {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"points_count": int64(len(f.points[rest])),
					"config": map[string]any{
						"params": map[string]any{
							"vectors": map[string]any{"size": dim, "distance": "Cosine"},
						},
					},
				},
			})
		case !strings.Contains(rest, "/") && r.Method == http.MethodPut:
			var body struct {
				Vectors struct {
					Size int `json:"size"`
				} `json:"vectors"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.collections[rest] = body.Vectors.Size
			_ = json.NewEncoder(w).Encode(map[string]any{"result": true})
		default:
			http.Error(w, "not implemented in fake: "+r.Method+" "+r.URL.Path, http.StatusNotImplemented)
		}
	})
	return mux
}

func TestClient_EnsureUpsertSearch(t *testing.T) {
	f := newFake()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	c := New(srv.URL, "")
	ctx := context.Background()
	if err := c.EnsureCollection(ctx, "chimera-t-p-_-abcd1234", 4); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	// Idempotent.
	if err := c.EnsureCollection(ctx, "chimera-t-p-_-abcd1234", 4); err != nil {
		t.Fatalf("ensure idempotent: %v", err)
	}

	pts := []vectorstore.Point{
		{ID: "11111111-1111-1111-1111-111111111111", Vector: []float32{1, 0, 0, 0}, Payload: vectorstore.Payload{TenantID: "t", ProjectID: "p", Text: "hello", Source: "a.txt"}},
		{ID: "22222222-2222-2222-2222-222222222222", Vector: []float32{0, 1, 0, 0}, Payload: vectorstore.Payload{TenantID: "t", ProjectID: "p", Text: "world", Source: "b.txt", FlavorID: "main"}},
	}
	if err := c.Upsert(ctx, "chimera-t-p-_-abcd1234", pts); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	hits, err := c.Search(ctx, "chimera-t-p-_-abcd1234", []float32{1, 0, 0, 0}, 10, 0, &vectorstore.Coords{TenantID: "t", ProjectID: "p"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits=%d, want 2", len(hits))
	}
	if hits[0].Payload.Text == "" {
		t.Fatalf("missing payload.text in hit")
	}

	st, err := c.Stats(ctx, "chimera-t-p-_-abcd1234")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.VectorDim != 4 || st.Points != 2 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestClient_ScrollPoints_Paginates(t *testing.T) {
	f := newFake()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := New(srv.URL, "")
	ctx := context.Background()
	coll := "chimera-t-p-_-abcd1234"
	if err := c.EnsureCollection(ctx, coll, 4); err != nil {
		t.Fatal(err)
	}
	var pts []vectorstore.Point
	for i := 0; i < 5; i++ {
		pts = append(pts, vectorstore.Point{
			ID:     fmt.Sprintf("00000000-0000-0000-0000-%012d", i),
			Vector: []float32{1, 0, 0, 0},
			Payload: vectorstore.Payload{
				TenantID: "t", ProjectID: "p", Text: "x", Source: fmt.Sprintf("f%d.txt", i),
				ContentSHA256: fmt.Sprintf("sha256:%d", i),
			},
		})
	}
	if err := c.Upsert(ctx, coll, pts); err != nil {
		t.Fatal(err)
	}
	b1, err := c.ScrollPoints(ctx, coll, &vectorstore.Coords{TenantID: "t", ProjectID: "p"}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(b1.Points) != 2 || b1.NextCursor == "" {
		t.Fatalf("batch1: %+v", b1)
	}
	b2, err := c.ScrollPoints(ctx, coll, &vectorstore.Coords{TenantID: "t", ProjectID: "p"}, 2, b1.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(b2.Points) != 2 {
		t.Fatalf("batch2: %+v", b2)
	}
}

func TestClient_Health(t *testing.T) {
	f := newFake()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := New(srv.URL, "")
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("health ok: %v", err)
	}
	f.mu.Lock()
	f.failHealth = true
	f.mu.Unlock()
	if err := c.Health(context.Background()); err == nil {
		t.Fatalf("expected error when down")
	}
}

func TestClient_Search_MissingCollectionReturnsEmpty(t *testing.T) {
	f := newFake()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := New(srv.URL, "")
	hits, err := c.Search(context.Background(), "no-such", []float32{1, 0, 0, 0}, 5, 0, nil)
	// Our fake returns empty rather than 404 here, but the production code path
	// also tolerates 404 / "doesn't exist". Accept either nil err and 0 hits.
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits, got %d", len(hits))
	}
}
