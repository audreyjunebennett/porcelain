package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lynn/porcelain/internal/naming"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/rag"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

func TestIndexerConfig_HappyPath(t *testing.T) {
	_, _, srv := setupRAGServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/config", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["chunk_size"].(float64) != 128 || doc["chunk_overlap"].(float64) != 32 {
		t.Fatalf("doc: %+v", doc)
	}
	if doc["embedding_model"] != "test-embed" {
		t.Fatalf("model: %+v", doc["embedding_model"])
	}
	if doc["ingest_path"] != "/v1/ingest" {
		t.Fatalf("ingest_path: %+v", doc["ingest_path"])
	}
	if doc["max_whole_file_bytes"] == nil || doc["ingest_session_path"] == nil {
		t.Fatalf("expected v0.4 indexer fields: %+v", doc)
	}
	hdrs, _ := doc["optional_headers"].([]any)
	if len(hdrs) != 3 {
		t.Fatalf("optional_headers: %+v", hdrs)
	}
}

func TestIndexerWorkspaces_HappyPath_LegacyTenant(t *testing.T) {
	rt, _, srv := setupRAGServer(t)
	dir := t.TempDir()
	st := rt.OperatorStore()
	if st == nil {
		t.Fatal("operator store unavailable")
	}
	ctx := context.Background()
	if _, err := st.CreateWorkspace(ctx, "", "projW", "flW", []string{dir}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/workspaces", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc struct {
		Object     string `json:"object"`
		TenantID   string `json:"tenant_id"`
		Workspaces []struct {
			WorkspaceID int64  `json:"workspace_id"`
			ProjectID   string `json:"project_id"`
			FlavorID    string `json:"flavor_id"`
			Paths       []struct {
				PathID int64  `json:"path_id"`
				Path   string `json:"path"`
			} `json:"paths"`
		} `json:"workspaces"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.Object != "indexer.workspaces" {
		t.Fatalf("object=%q", doc.Object)
	}
	if len(doc.Workspaces) != 1 || doc.Workspaces[0].ProjectID != "projW" {
		t.Fatalf("workspaces=%+v", doc.Workspaces)
	}
	if len(doc.Workspaces[0].Paths) != 1 || filepath.Clean(doc.Workspaces[0].Paths[0].Path) != filepath.Clean(dir) {
		t.Fatalf("paths=%+v", doc.Workspaces[0].Paths)
	}
}

func TestIndexerHealth_OK(t *testing.T) {
	rt, _, srv := setupRAGServer(t)
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{"test-embed"}))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/health", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["ok"] != true || doc["status"] != "ok" {
		t.Fatalf("doc: %+v", doc)
	}
	checks, _ := doc["checks"].(map[string]any)
	if checks == nil {
		t.Fatal("missing checks")
	}
	vs, _ := checks["vectorstore"].(map[string]any)
	emb, _ := checks["embedding"].(map[string]any)
	if vs["ok"] != true || emb["ok"] != true {
		t.Fatalf("checks: %+v", checks)
	}
}

func TestIndexerHealth_Degraded(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	store.healthErr = errors.New("connection refused")
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{"test-embed"}))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/health", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["ok"] != false {
		t.Fatalf("doc: %+v", doc)
	}
	checks, _ := doc["checks"].(map[string]any)
	vs, _ := checks["vectorstore"].(map[string]any)
	if vs["ok"] != false || vs["reason_code"] != "vectorstore_unreachable" {
		t.Fatalf("vectorstore check: %+v", vs)
	}
}

func TestIndexerHealth_EmbedModelMissingFromCatalog(t *testing.T) {
	rt, _, srv := setupRAGServer(t)
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{"groq/llama3"}))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/health", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["ok"] != false {
		t.Fatalf("doc: %+v", doc)
	}
	checks, _ := doc["checks"].(map[string]any)
	vs, _ := checks["vectorstore"].(map[string]any)
	emb, _ := checks["embedding"].(map[string]any)
	if vs["ok"] != true {
		t.Fatalf("vectorstore: %+v", vs)
	}
	if emb["ok"] != false || emb["reason_code"] != "embed_model_not_in_catalog" {
		t.Fatalf("embedding: %+v", emb)
	}
}

func TestIndexerHealth_OllamaDownQdrantUp(t *testing.T) {
	rt, store, srv := setupRAGServerWithOllamaEmbedModel(t)
	store.healthErr = nil
	rt.SetCatalogSnapshot(catalog.NewTestSnapshotWithModels(time.Now(), []string{"groq/llama3"}))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/health", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["ok"] != false {
		t.Fatalf("doc: %+v", doc)
	}
	checks, _ := doc["checks"].(map[string]any)
	vs, _ := checks["vectorstore"].(map[string]any)
	emb, _ := checks["embedding"].(map[string]any)
	if vs["ok"] != true {
		t.Fatalf("vectorstore: %+v", vs)
	}
	if emb["ok"] != false || emb["reason_code"] == "" || emb["detail"] == "" {
		t.Fatalf("embedding: %+v", emb)
	}
}

func setupRAGServerWithOllamaEmbedModel(t *testing.T) (*Runtime, *inMemoryStore, *httptest.Server) {
	t.Helper()
	rt, store, srv := setupRAGServer(t)
	svc, err := rag.New(rag.Options{
		Store:        store,
		Embedder:     ollamaStubEmbedder{dim: 8},
		ChunkSize:    128,
		ChunkOverlap: 32,
		TopK:         4,
		EmbeddingDim: 8,
		Log:          testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetRAGForTest(svc)
	return rt, store, srv
}

type ollamaStubEmbedder struct{ dim int }

func (e ollamaStubEmbedder) EmbedBatch(ctx context.Context, in []string) ([][]float32, error) {
	out := make([][]float32, len(in))
	for i := range in {
		v := make([]float32, e.dim)
		v[0] = float32(i + 1)
		out[i] = v
	}
	return out, nil
}
func (e ollamaStubEmbedder) EmbedOne(ctx context.Context, s string) ([]float32, error) {
	v, err := e.EmbedBatch(ctx, []string{s})
	if err != nil {
		return nil, err
	}
	return v[0], nil
}
func (e ollamaStubEmbedder) Model() string { return "ollama/nomic-embed-text:latest" }

func TestIndexerStats_AfterIngest(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	_, err := rt.RAG().Ingest(context.Background(), rag.IngestRequest{
		Coords: vectorstore.Coords{TenantID: "tenantA", ProjectID: "proj"},
		Source: "docs/a.md",
		Text:   strings.Repeat("alpha ", 80),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = store

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/storage/stats", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set(HeaderProject, "proj")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if doc["points"].(float64) < 1 {
		t.Fatalf("doc: %+v", doc)
	}
	if doc["vector_dim"].(float64) != 8 {
		t.Fatalf("doc: %+v", doc)
	}
}

func TestIndexerCorpusInventory_AfterIngest(t *testing.T) {
	rt, _, srv := setupRAGServer(t)
	_, err := rt.RAG().Ingest(context.Background(), rag.IngestRequest{
		Coords:      vectorstore.Coords{TenantID: "tenantA", ProjectID: "proj"},
		Source:      "docs/b.md",
		Text:        strings.Repeat("beta ", 80),
		ContentHash: "sha256:clienthash",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/indexer/corpus/inventory?limit=50", nil)
	req.Header.Set("Authorization", "Bearer ingest-tok")
	req.Header.Set(HeaderProject, "proj")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc struct {
		Entries []struct {
			Source            string `json:"source"`
			ContentSHA256     string `json:"content_sha256"`
			ClientContentHash string `json:"client_content_hash"`
		} `json:"entries"`
		HasMore bool `json:"has_more"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) < 1 {
		t.Fatalf("entries=%v", doc.Entries)
	}
	found := false
	for _, e := range doc.Entries {
		if e.Source == "docs/b.md" && strings.HasPrefix(e.ContentSHA256, "sha256:") && e.ClientContentHash == "sha256:clienthash" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing expected entry: %+v", doc.Entries)
	}
}

func TestHealth_RAGProbeIncluded(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	_ = rt
	store.healthErr = nil
	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	checks, _ := doc["checks"].(map[string]any)
	if _, ok := checks["vectorstore"]; !ok {
		t.Fatalf("missing vectorstore check: %+v", checks)
	}
}

func TestHealth_RAGFailDegrades(t *testing.T) {
	rt, store, srv := setupRAGServer(t)
	store.healthErr = errors.New("nope")
	_ = rt
	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestHealth_NoRAGProbeWhenDisabled(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
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

	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	checks, _ := doc["checks"].(map[string]any)
	if _, ok := checks["vectorstore"]; ok {
		t.Fatalf("vectorstore check should not be present when RAG disabled: %+v", checks)
	}
}
