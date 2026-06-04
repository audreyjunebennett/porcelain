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
	ichunk "github.com/lynn/porcelain/internal/chunk"
	"github.com/lynn/porcelain/internal/naming"
)

func manifestJSONForTest(source, text, clientHash string) string {
	norm := ichunk.NormalizeNewlines(text)
	segs := ichunk.Split(norm, 512, 128)
	sum := sha256.Sum256([]byte(norm))
	serverHash := "sha256:" + hex.EncodeToString(sum[:])
	if clientHash == "" {
		clientHash = serverHash
	}
	chunks := make([]rag.ManifestChunk, len(segs))
	for i, s := range segs {
		chunks[i] = rag.ManifestChunk{
			ChunkIndex: i, Text: s.Text, StartLine: s.StartLine, EndLine: s.EndLine,
			StartByte: s.StartByte, EndByte: s.EndByte, StartCh: s.StartCh, EndCh: s.EndCh,
			StartsMidLine: s.StartsMidLine,
		}
	}
	lineCount := 1
	if len(norm) > 0 {
		lineCount = strings.Count(norm, "\n") + 1
	}
	m := rag.IngestManifest{
		Object: "ingest.manifest", Source: source, ContentSHA256: serverHash,
		ClientContentHash: clientHash, ChunkSize: 512, ChunkOverlap: 128,
		ChunkSchema: ichunk.SchemaV2, LineCount: lineCount, FileBytes: len(norm), Chunks: chunks,
	}
	b, _ := json.Marshal(m)
	return string(b)
}

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

func (s *inMemoryStore) DeleteCollection(_ context.Context, c string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.collections, c)
	delete(s.points, c)
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

func (s *inMemoryStore) GetPoints(_ context.Context, c string, ids []string) ([]vectorstore.PointPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := map[string]vectorstore.Point{}
	for _, p := range s.points[c] {
		byID[p.ID] = p
	}
	var out []vectorstore.PointPayload
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			out = append(out, vectorstore.PointPayload{ID: p.ID, Payload: p.Payload})
		}
	}
	return out, nil
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
	tokPath := filepath.Join(cfgDir, "api-keys.yaml")
	writeTokens(t, tokPath, "ingest-tok", "tenantA")
	routePath := filepath.Join(cfgDir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntimeLog(t, gwPath, lg)
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
	text := strings.Repeat("alpha ", 50)
	body := manifestJSONForTest("docs/readme.md", text, "")
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

func TestIngest_MultipartRejected(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "main.go")
	_, _ = fw.Write([]byte("hello"))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", &buf)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d want 400", res.StatusCode)
	}
}

func TestIngest_Unauthorized(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	body := manifestJSONForTest("a", "hi", "")
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
	rt := mustRuntime(t, gwPath)
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
		{"missing manifest", "application/json", `{"object":"ingest.manifest"}`, http.StatusBadRequest},
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
	manifestBody := manifestJSONForTest("docs/chunked.txt", payload, "")

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest/session", strings.NewReader(`{"source":"docs/chunked.txt","content_hash":"sha256:00"}`))
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

	_ = maxChunk // manifest-first session: no transport chunks required.

	creq, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest/session/"+sid+"/complete", strings.NewReader(manifestBody))
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
	norm := ichunk.NormalizeNewlines(payload)
	sum := sha256.Sum256([]byte(norm))
	want := "sha256:" + hex.EncodeToString(sum[:])
	if doc["content_sha256"] != want || doc["content_hash"] != want {
		t.Fatalf("hashes: %+v want %s", doc, want)
	}
}

func TestIngest_JSON_logsConversationIDWhenHeaderPresent(t *testing.T) {
	var buf strings.Builder
	lg := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	_, _, srv := setupRAGServerWithLog(t, lg)
	body := manifestJSONForTest("docs/corr.md", strings.Repeat("alpha ", 50), "")
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
