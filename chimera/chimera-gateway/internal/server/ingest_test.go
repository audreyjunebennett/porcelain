package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/internal/naming"
)

// inMemoryStore is a minimal vectorstore.Store for handler integration tests.
type inMemoryStore struct {
	mu          sync.Mutex
	collections map[string]int
	points      map[string][]vectorstore.Point
	healthErr   error
}

func newMemStore() *inMemoryStore {
	return &inMemoryStore{collections: map[string]int{}, points: map[string][]vectorstore.Point{}}
}

func (s *inMemoryStore) EnsureCollection(_ context.Context, name string, dim int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.collections[name]; !ok {
		s.collections[name] = dim
	}
	return nil
}
func (s *inMemoryStore) Upsert(_ context.Context, c string, pts []vectorstore.Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points[c] = append(s.points[c], pts...)
	return nil
}
func (s *inMemoryStore) Search(_ context.Context, c string, _ []float32, k int, _ float32, _ *vectorstore.Coords) ([]vectorstore.Hit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []vectorstore.Hit{}
	for i, p := range s.points[c] {
		if i >= k {
			break
		}
		out = append(out, vectorstore.Hit{ID: p.ID, Score: 0.95, Payload: p.Payload})
	}
	return out, nil
}
func (s *inMemoryStore) Health(context.Context) error { return s.healthErr }
func (s *inMemoryStore) Stats(_ context.Context, c string) (vectorstore.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return vectorstore.Stats{Collection: c, Points: int64(len(s.points[c])), VectorDim: s.collections[c]}, nil
}
func (s *inMemoryStore) DeleteBySource(_ context.Context, c, src string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := s.points[c][:0]
	for _, p := range s.points[c] {
		if p.Payload.Source != src {
			keep = append(keep, p)
		}
	}
	s.points[c] = keep
	return nil
}

func (s *inMemoryStore) ScrollPoints(_ context.Context, c string, filter *vectorstore.Coords, limit int, cursor string) (vectorstore.ScrollBatch, error) {
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
	slice := rows[start:end]
	next := ""
	if end < len(rows) {
		next = strconv.Itoa(end)
	}
	return vectorstore.ScrollBatch{Points: slice, NextCursor: next}, nil
}

// stubEmbedder yields deterministic dim-sized vectors.
type stubEmbedder struct{ dim int }

func (e stubEmbedder) EmbedBatch(_ context.Context, in []string) ([][]float32, error) {
	out := make([][]float32, len(in))
	for i := range in {
		v := make([]float32, e.dim)
		v[0] = float32(i + 1)
		out[i] = v
	}
	return out, nil
}
func (e stubEmbedder) EmbedOne(ctx context.Context, s string) ([]float32, error) {
	v, err := e.EmbedBatch(ctx, []string{s})
	if err != nil {
		return nil, err
	}
	return v[0], nil
}
func (e stubEmbedder) Model() string { return "test-embed" }

func testRepoOperatorMigrationsDir(t *testing.T) string {
	t.Helper()
	return testsupport.GatewayOperatorMigrationsDir(t)
}

// setupRAGServerWithLog wires NewRuntime + fake RAG like setupRAGServer; if lg is nil, uses testLog().
func setupRAGServerWithLog(t *testing.T, lg *slog.Logger) (*Runtime, *inMemoryStore, *httptest.Server) {
	t.Helper()
	if lg == nil {
		lg = testLog()
	}
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gwPath := filepath.Join(cfgDir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, upstream.URL, []string{"m"}, "http://127.0.0.1:1")
	opMig := testRepoOperatorMigrationsDir(t)
	opAppend := "\noperator:\n  migrations_dir: \"" + strings.ReplaceAll(filepath.ToSlash(opMig), `\`, `/`) + "\"\n"
	gwBytes, err := os.ReadFile(gwPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gwPath, append(gwBytes, []byte(opAppend)...), 0o644); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(cfgDir, "api-keys.yaml")
	writeTokens(t, tokPath, "ingest-tok", "tenantA")
	routePath := filepath.Join(cfgDir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, lg)
	if err != nil {
		t.Fatal(err)
	}
	store := newMemStore()
	svc, err := rag.New(rag.Options{
		Store:        store,
		Embedder:     stubEmbedder{dim: 8},
		ChunkSize:    128,
		ChunkOverlap: 32,
		TopK:         4,
		EmbeddingDim: 8,
		Log:          lg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetRAGForTest(svc)

	t.Cleanup(func() {
		rt.CloseOperator()
		rt.CloseMetrics()
	})

	srv := httptest.NewServer(NewMux(rt, lg, nil, nil))
	t.Cleanup(srv.Close)
	return rt, store, srv
}

func setupRAGServer(t *testing.T) (*Runtime, *inMemoryStore, *httptest.Server) {
	t.Helper()
	return setupRAGServerWithLog(t, nil)
}

func TestIngest_JSON(t *testing.T) {
	_, store, srv := setupRAGServer(t)
	body := `{"source":"docs/readme.md","text":"` + strings.Repeat("alpha ", 50) + `"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderProject, "myproj")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body %s", res.StatusCode, b)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["tenant_id"] != "tenantA" || doc["project_id"] != "myproj" {
		t.Fatalf("doc: %+v", doc)
	}
	if doc["chunks"].(float64) < 1 {
		t.Fatalf("doc: %+v", doc)
	}
	hash, _ := doc["content_hash"].(string)
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("content_hash: %q", hash)
	}
	coll, _ := doc["collection"].(string)
	if coll == "" || len(store.points[coll]) == 0 {
		t.Fatalf("no points stored in collection %q", coll)
	}
}

func TestIngest_Multipart(t *testing.T) {
	_, store, srv := setupRAGServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	w, _ := mw.CreateFormFile("file", "main.go")
	_, _ = w.Write([]byte(strings.Repeat("hello ", 100)))
	_ = mw.WriteField("source", "src/main.go")
	_ = mw.WriteField("content_hash", "sha256:client-supplied")
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", &buf)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(HeaderFlavor, "branch-foo")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body %s", res.StatusCode, b)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["flavor_id"] != "branch-foo" {
		t.Fatalf("flavor: %+v", doc)
	}
	if doc["source"] != "src/main.go" {
		t.Fatalf("source should be the explicit form field 'src/main.go', got: %+v", doc)
	}
	text := strings.Repeat("hello ", 100)
	sum := sha256.Sum256([]byte(text))
	want := "sha256:" + hex.EncodeToString(sum[:])
	if doc["content_hash"] != want || doc["content_sha256"] != want {
		t.Fatalf("expected server sha for file bytes: content_hash=%v content_sha256=%v want %q", doc["content_hash"], doc["content_sha256"], want)
	}
	if doc["client_content_hash"] != "sha256:client-supplied" {
		t.Fatalf("client_content_hash: %+v", doc["client_content_hash"])
	}
	coll, _ := doc["collection"].(string)
	if len(store.points[coll]) == 0 {
		t.Fatal("expected points stored")
	}
}

func TestIngest_Unauthorized(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	body := `{"source":"a","text":"hi"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestIngest_RAGDisabled_503(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(upstream.Close)
	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, upstream.URL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "tok", "ten")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	_ = os.WriteFile(routePath, []byte("rules: []\n"), 0o644)
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewMux(rt, testLog(), nil, nil))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", strings.NewReader(`{"source":"a","text":"x"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestIngest_BadBody(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	cases := []struct {
		name string
		ct   string
		body string
		want int
	}{
		{"empty json", "application/json", `{}`, http.StatusBadRequest},
		{"missing source", "application/json", `{"text":"x"}`, http.StatusBadRequest},
		{"bad ct", "text/plain", "hello", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer ingest-tok")
			req.Header.Set("Content-Type", tc.ct)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("got %d want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestIngest_ChunkedSession(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	payload := strings.Repeat("chunkline\n", 500)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest/session", strings.NewReader(`{"source":"docs/chunked.txt"}`))
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("start status %d: %s", res.StatusCode, b)
	}
	var start struct {
		SessionID     string  `json:"session_id"`
		MaxChunkBytes float64 `json:"max_chunk_bytes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&start); err != nil {
		t.Fatal(err)
	}
	if start.SessionID == "" || start.MaxChunkBytes <= 0 {
		t.Fatalf("bad start: %+v", start)
	}
	maxChunk := int64(start.MaxChunkBytes)
	sid := start.SessionID

	off := 0
	for idx := 0; off < len(payload); idx++ {
		end := off + int(maxChunk)
		if end > len(payload) {
			end = len(payload)
		}
		chunk := payload[off:end]
		preq, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/ingest/session/"+sid+"/chunk", strings.NewReader(chunk))
		preq.Header.Set("Authorization", "Bearer ingest-tok")
		preq.Header.Set(naming.HeaderChunkIndexTarget, strconv.Itoa(idx))
		preq.ContentLength = int64(len(chunk))
		pres, err := http.DefaultClient.Do(preq)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(pres.Body)
		pres.Body.Close()
		if pres.StatusCode != http.StatusOK {
			t.Fatalf("chunk %d status %d: %s", idx, pres.StatusCode, b)
		}
		off = end
	}

	creq, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest/session/"+sid+"/complete", strings.NewReader("{}"))
	creq.Header.Set("Authorization", "Bearer ingest-tok")
	creq.Header.Set("Content-Type", "application/json")
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cres.Body.Close()
	if cres.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(cres.Body)
		t.Fatalf("complete status %d: %s", cres.StatusCode, b)
	}
	var doc map[string]any
	if err := json.NewDecoder(cres.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(payload))
	want := "sha256:" + hex.EncodeToString(sum[:])
	if doc["content_sha256"] != want || doc["content_hash"] != want {
		t.Fatalf("hashes: %+v want %s", doc, want)
	}
}

func TestIngest_JSON_logsConversationIDWhenHeaderPresent(t *testing.T) {
	var buf strings.Builder
	lg := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	_, _, srv := setupRAGServerWithLog(t, lg)
	body := `{"source":"docs/corr.md","text":"` + strings.Repeat("alpha ", 50) + `"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderProject, "myproj")
	req.Header.Set(headerConversationID, "ingest-linked-conv-1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body %s", res.StatusCode, b)
	}
	out := buf.String()
	if !strings.Contains(out, "conversation_id=ingest-linked-conv-1") {
		t.Fatalf("expected conversation_id in ingest logs:\n%s", out)
	}
	if !strings.Contains(out, "msg=ingest.complete") {
		t.Fatalf("expected ingest.complete:\n%s", out)
	}
}
